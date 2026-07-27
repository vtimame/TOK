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

	stdoutPath, nextOrdinal, err := nextRunArtifactPath(dataDir, run.ID, "stdout", len(artifacts)+1)
	if err != nil {
		return storage.RunArtifact{}, err
	}
	stderrPath, _, err := nextRunArtifactPath(dataDir, run.ID, "stderr", nextOrdinal)
	if err != nil {
		return storage.RunArtifact{}, err
	}

	stdoutWriter, err := newBoundedRunArtifactWriter(stdoutPath, opts.limitBytes)
	if err != nil {
		return storage.RunArtifact{}, err
	}
	defer func() { _, _ = stdoutWriter.Close() }()
	stderrWriter, err := newBoundedRunArtifactWriter(stderrPath, opts.limitBytes)
	if err != nil {
		_, _ = stdoutWriter.Close()
		_ = os.Remove(stdoutPath)
		return storage.RunArtifact{}, err
	}
	defer func() { _, _ = stderrWriter.Close() }()

	commandCtx, cancel := context.WithTimeout(ctx, opts.timeout)
	defer cancel()
	cmd := exec.CommandContext(commandCtx, opts.command[0], opts.command[1:]...)
	cmd.Dir = project.Path
	cmd.Env = commandEnv
	cmd.Stdout = stdoutWriter
	cmd.Stderr = stderrWriter

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

	stdoutResult, err := stdoutWriter.Close()
	if err != nil {
		_, _ = stderrWriter.Close()
		_ = os.Remove(stdoutPath)
		_ = os.Remove(stderrPath)
		return storage.RunArtifact{}, err
	}
	stderrResult, err := stderrWriter.Close()
	if err != nil {
		_ = os.Remove(stdoutPath)
		_ = os.Remove(stderrPath)
		return storage.RunArtifact{}, err
	}

	stdoutMetadata, err := validationStreamArtifactMetadata("stdout", opts.limitBytes, stdoutResult)
	if err != nil {
		_ = os.Remove(stdoutPath)
		_ = os.Remove(stderrPath)
		return storage.RunArtifact{}, err
	}
	stdoutArtifact, err := store.AddRunArtifact(ctx, storage.AddRunArtifactInput{
		RunID:       run.ID,
		Kind:        "stdout",
		Path:        stdoutPath,
		ContentHash: stdoutResult.ContentHash,
		SizeBytes:   stdoutResult.SizeBytes,
		Truncated:   stdoutResult.Truncated,
		Metadata:    stdoutMetadata,
		Actor:       actor,
	})
	if err != nil {
		_ = os.Remove(stdoutPath)
		_ = os.Remove(stderrPath)
		return storage.RunArtifact{}, err
	}

	stderrMetadata, err := validationStreamArtifactMetadata("stderr", opts.limitBytes, stderrResult)
	if err != nil {
		_ = os.Remove(stderrPath)
		return storage.RunArtifact{}, err
	}
	stderrArtifact, err := store.AddRunArtifact(ctx, storage.AddRunArtifactInput{
		RunID:       run.ID,
		Kind:        "stderr",
		Path:        stderrPath,
		ContentHash: stderrResult.ContentHash,
		SizeBytes:   stderrResult.SizeBytes,
		Truncated:   stderrResult.Truncated,
		Metadata:    stderrMetadata,
		Actor:       actor,
	})
	if err != nil {
		_ = os.Remove(stderrPath)
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
