package app

import (
	"context"
	"os"
	"os/exec"
	"time"

	tokservice "s26.sh/tok/internal/service"
	"s26.sh/tok/internal/storage"
)

func executeRunValidation(ctx context.Context, store *storage.Store, dataDir string, opts runValidateOptions, actor storage.ActorRef) (storage.RunArtifact, error) {
	run, err := store.GetRun(ctx, opts.runID)
	if err != nil {
		return storage.RunArtifact{}, err
	}
	task, err := store.GetTask(ctx, run.TaskID)
	if err != nil {
		return storage.RunArtifact{}, err
	}
	project, err := store.GetProjectByID(ctx, task.ProjectID)
	if err != nil {
		return storage.RunArtifact{}, err
	}
	redactor := newRunMetadataRedactor(os.Environ())
	commandEnv := defaultRunCommandEnv(os.Environ(), run, task, project)
	envNames := append([]string(nil), runEnvNames(commandEnv)...)
	dangerousOverride := ""
	if opts.allowDangerous {
		dangerousOverride = dangerousRunCommandReason(opts.command)
		if dangerousOverride == "" {
			dangerousOverride = "explicit operator override"
		}
	}
	safety := runCommandSafetyMetadata{
		EnvPolicy:         "filtered",
		EnvNames:          envNames,
		RedactionEnabled:  true,
		AllowDangerous:    opts.allowDangerous,
		DangerousOverride: dangerousOverride,
	}
	artifacts, err := store.ListRunArtifacts(ctx, run.ID)
	if err != nil {
		return storage.RunArtifact{}, err
	}

	streams, err := newRunStreamArtifactWriters(dataDir, run.ID, len(artifacts)+1, opts.limitBytes)
	if err != nil {
		return storage.RunArtifact{}, err
	}
	defer streams.closeQuietly()

	commandCtx, cancel := context.WithTimeout(ctx, opts.timeout)
	defer cancel()
	cmd := exec.CommandContext(commandCtx, opts.command[0], opts.command[1:]...)
	cmd.Dir = project.Path
	cmd.Env = commandEnv
	cmd.Stdout = streams.StdoutWriter
	cmd.Stderr = streams.StderrWriter

	start := time.Now()
	runErr := cmd.Run()
	duration := time.Since(start)
	timedOut := commandCtx.Err() == context.DeadlineExceeded
	exitCode := 0
	status := "passed"
	if runErr != nil || timedOut {
		status = "failed"
		exitCode = -1
		if cmd.ProcessState != nil {
			exitCode = cmd.ProcessState.ExitCode()
		}
	}

	stdoutArtifact, stderrArtifact, err := recordRunStreamArtifacts(ctx, store, run.ID, "run validate", opts.limitBytes, streams, actor)
	if err != nil {
		return storage.RunArtifact{}, err
	}

	metadata, err := executedValidationArtifactMetadata(opts, redactor, safety, status, exitCode, duration, timedOut, stdoutArtifact.ID, stderrArtifact.ID)
	if err != nil {
		return storage.RunArtifact{}, err
	}
	return tokservice.NewRunService(store).RecordValidationArtifact(ctx, storage.AddRunArtifactInput{
		RunID:    run.ID,
		Metadata: metadata,
		Actor:    actor,
	})
}
