package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"s26.sh/tok/internal/storage"
	"sort"
	"strings"
	"time"
)

type agentAdapterResult struct {
	Status  string `json:"status"`
	Summary string `json:"summary"`
}

func executeAgentCommand(ctx context.Context, store *storage.Store, dataDir string, run storage.Run, task storage.Task, project storage.Project, opts runAgentOptions, actor storage.ActorRef, handoffArtifact storage.RunArtifact, handoffText string) (runExecResult, error) {
	redactor := newRunMetadataRedactor(os.Environ())
	artifacts, err := store.ListRunArtifacts(ctx, run.ID)
	if err != nil {
		return runExecResult{}, err
	}
	resultPath, _, err := nextRunArtifactPathWithExt(dataDir, run.ID, "agent-result", ".json", len(artifacts)+1)
	if err != nil {
		return runExecResult{}, err
	}
	artifactDir := filepath.Dir(resultPath)
	if err := os.MkdirAll(artifactDir, 0o755); err != nil {
		return runExecResult{}, fmt.Errorf("create run artifact directory: %w", err)
	}

	commandEnv := agentAdapterCommandEnv(os.Environ(), run, task, project, opts, artifactDir, handoffArtifact.Path, resultPath, handoffText)
	envNames := runEnvNames(commandEnv)
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

	streams, err := newRunStreamArtifactWriters(dataDir, run.ID, len(artifacts)+1, opts.limitBytes)
	if err != nil {
		return runExecResult{}, err
	}
	defer streams.closeQuietly()

	cmd := exec.Command(opts.command[0], opts.command[1:]...)
	cmd.Dir = project.Path
	cmd.Env = commandEnv
	if opts.contextMode == "stdin" {
		cmd.Stdin = strings.NewReader(handoffText)
	}
	cmd.Stdout = streams.StdoutWriter
	cmd.Stderr = streams.StderrWriter
	configureRunProcessGroup(cmd)

	start := time.Now()
	startErr := cmd.Start()
	pid := 0
	pgid := 0
	sessionID := 0
	var waitErr error
	timedOut := false
	forwardedSignal := ""
	if startErr == nil {
		pid = cmd.Process.Pid
		pgid = runProcessGroupID(pid)
		if sid, err := getRunProcessSessionID(pid); err == nil {
			sessionID = sid
		}

		waitCh := make(chan error, 1)
		go func() {
			waitCh <- cmd.Wait()
		}()

		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, os.Interrupt, runProcessTerminationSignal())
		defer signal.Stop(sigCh)

		timeout := time.NewTimer(opts.timeout)
		defer timeout.Stop()

		select {
		case waitErr = <-waitCh:
		case sig := <-sigCh:
			forwardedSignal = sig.String()
			forwardRunProcessSignal(pgid, sig)
			waitErr = waitForRunProcessExit(waitCh, pgid, runExecTerminationGrace)
		case <-timeout.C:
			timedOut = true
			forwardedSignal = runProcessTerminationSignalName()
			forwardRunProcessSignal(pgid, runProcessTerminationSignal())
			waitErr = waitForRunProcessExit(waitCh, pgid, runExecTerminationGrace)
		case <-ctx.Done():
			forwardedSignal = runProcessTerminationSignalName()
			forwardRunProcessSignal(pgid, runProcessTerminationSignal())
			waitErr = waitForRunProcessExit(waitCh, pgid, runExecTerminationGrace)
		}
	} else {
		waitErr = startErr
	}
	duration := time.Since(start)

	stdoutArtifact, stderrArtifact, err := recordRunStreamArtifacts(ctx, store, run.ID, "run agent", opts.limitBytes, streams, actor)
	if err != nil {
		return runExecResult{}, err
	}

	adapterResult, resultRead, resultErr := readAgentAdapterResult(resultPath)
	execution := agentCommandExecutionResult(opts, redactor, adapterResult, resultRead, resultErr, waitErr, startErr, duration, timedOut, forwardedSignal, pid, pgid, sessionID)
	metadata, err := runAgentArtifactMetadata(opts, redactor, safety, execution, handoffArtifact.ID, stdoutArtifact.ID, stderrArtifact.ID, artifactDir, handoffArtifact.Path, resultPath, resultRead, resultErr)
	if err != nil {
		return runExecResult{}, err
	}
	if _, err := store.AddRunArtifact(ctx, storage.AddRunArtifactInput{
		RunID:    run.ID,
		Kind:     "log",
		Metadata: metadata,
		Actor:    actor,
	}); err != nil {
		return runExecResult{}, err
	}

	return runExecResult{
		RunStatus: execution.RunStatus,
		Summary:   execution.Summary,
	}, nil
}

func agentAdapterCommandEnv(baseEnv []string, run storage.Run, task storage.Task, project storage.Project, opts runAgentOptions, artifactDir, handoffPath, resultPath, handoffText string) []string {
	envByName := map[string]string{}
	for _, item := range defaultRunCommandEnv(baseEnv, run, task, project) {
		name, _, ok := strings.Cut(item, "=")
		if ok && name != "" {
			envByName[name] = item
		}
	}
	envByName["TOK_PROJECT_PATH"] = "TOK_PROJECT_PATH=" + project.Path
	envByName["TOK_AGENT_ADAPTER_CONTRACT"] = "TOK_AGENT_ADAPTER_CONTRACT=" + agentAdapterContractV0
	envByName["TOK_AGENT_CONTEXT_MODE"] = "TOK_AGENT_CONTEXT_MODE=" + opts.contextMode
	envByName["TOK_AGENT_HANDOFF_ARTIFACT_FILE"] = "TOK_AGENT_HANDOFF_ARTIFACT_FILE=" + handoffPath
	envByName["TOK_AGENT_RESULT_FILE"] = "TOK_AGENT_RESULT_FILE=" + resultPath
	envByName["TOK_RUN_ARTIFACT_DIR"] = "TOK_RUN_ARTIFACT_DIR=" + artifactDir
	if opts.contextMode == "file" {
		envByName["TOK_AGENT_CONTEXT_FILE"] = "TOK_AGENT_CONTEXT_FILE=" + handoffPath
	}
	if opts.contextMode == "env" {
		envByName["TOK_AGENT_CONTEXT"] = "TOK_AGENT_CONTEXT=" + handoffText
	}

	names := make([]string, 0, len(envByName))
	for name := range envByName {
		names = append(names, name)
	}
	sort.Strings(names)
	env := make([]string, 0, len(names))
	for _, name := range names {
		env = append(env, envByName[name])
	}
	return env
}

func readAgentAdapterResult(path string) (agentAdapterResult, bool, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return agentAdapterResult{}, false, nil
		}
		return agentAdapterResult{}, false, err
	}
	var result agentAdapterResult
	if err := json.Unmarshal(raw, &result); err != nil {
		return agentAdapterResult{}, true, err
	}
	result.Status = strings.TrimSpace(result.Status)
	result.Summary = strings.TrimSpace(result.Summary)
	if !validAgentAdapterResultStatus(result.Status) {
		return agentAdapterResult{}, true, fmt.Errorf("invalid adapter result status %q", result.Status)
	}
	if result.Summary == "" {
		return agentAdapterResult{}, true, errors.New("adapter result summary is required")
	}
	return result, true, nil
}

func validAgentAdapterResultStatus(status string) bool {
	switch status {
	case "succeeded", "failed", "blocked", "cancelled":
		return true
	default:
		return false
	}
}

func agentCommandExecutionResult(opts runAgentOptions, redactor runMetadataRedactor, adapterResult agentAdapterResult, resultRead bool, resultErr error, waitErr, startErr error, duration time.Duration, timedOut bool, forwardedSignal string, pid, pgid, sessionID int) runCommandExecutionResult {
	execution := runCommandExecutionResult{
		Status:         "failed",
		RunStatus:      "failed",
		Summary:        "Agent adapter did not write a result.",
		ExitCode:       0,
		Duration:       duration,
		TimedOut:       timedOut,
		Signal:         forwardedSignal,
		PID:            pid,
		ProcessGroupID: pgid,
		SessionID:      sessionID,
	}
	if startErr != nil {
		execution.ExitCode = -1
		execution.Summary = "Agent adapter failed to start."
		return execution
	}
	if timedOut {
		execution.Status = "cancelled"
		execution.RunStatus = "cancelled"
		execution.ExitCode = -1
		execution.Summary = fmt.Sprintf("Agent adapter timed out after %s.", opts.timeout)
		return execution
	}
	if forwardedSignal != "" {
		execution.Status = "cancelled"
		execution.RunStatus = "cancelled"
		execution.ExitCode = -1
		execution.Summary = fmt.Sprintf("Agent adapter interrupted by %s.", forwardedSignal)
		return execution
	}
	if waitErr != nil {
		execution.ExitCode = -1
	}
	if resultRead && resultErr == nil {
		execution.Status = adapterResult.Status
		execution.RunStatus = adapterResult.Status
		execution.Summary = redactor.redactString(adapterResult.Summary)
		if waitErr != nil && adapterResult.Status == "succeeded" {
			execution.Status = "failed"
			execution.RunStatus = "failed"
			execution.Summary = "Agent adapter exited unsuccessfully after reporting success."
		}
		if execution.ExitCode == -1 && waitErr != nil && execution.Summary == "" {
			execution.Summary = "Agent adapter exited unsuccessfully."
		}
	} else if resultErr != nil {
		execution.Summary = "Agent adapter wrote an invalid result."
	}
	if waitErr != nil && execution.ExitCode == -1 {
		// exec.ExitError exposes the command exit code via ProcessState.
		var exitErr *exec.ExitError
		if errors.As(waitErr, &exitErr) {
			execution.ExitCode = exitErr.ExitCode()
		}
	}
	return execution
}
