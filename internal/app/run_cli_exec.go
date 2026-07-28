package app

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	tokservice "s26.sh/tok/internal/service"
	"s26.sh/tok/internal/storage"
)

type runExecResult struct {
	RunStatus string
	Summary   string
}

type runCommandExecutionResult struct {
	Status         string
	RunStatus      string
	Summary        string
	ExitCode       int
	Duration       time.Duration
	TimedOut       bool
	Signal         string
	PID            int
	ProcessGroupID int
	SessionID      int
}

func executeRunCommand(ctx context.Context, store *storage.Store, dataDir string, run storage.Run, task storage.Task, project storage.Project, opts runExecOptions, actor storage.ActorRef) (runExecResult, error) {
	redactor := newRunMetadataRedactor(os.Environ())
	commandEnv := defaultRunCommandEnv(os.Environ(), run, task, project)
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

	artifacts, err := store.ListRunArtifacts(ctx, run.ID)
	if err != nil {
		return runExecResult{}, err
	}
	streams, err := newRunStreamArtifactWriters(dataDir, run.ID, len(artifacts)+1, opts.limitBytes)
	if err != nil {
		return runExecResult{}, err
	}
	defer streams.closeQuietly()

	cmd := exec.Command(opts.command[0], opts.command[1:]...)
	cmd.Dir = project.Path
	cmd.Env = commandEnv
	cmd.Stdout = streams.StdoutWriter
	cmd.Stderr = streams.StderrWriter
	configureRunProcessGroup(cmd)

	start := time.Now()
	if err := cmd.Start(); err != nil {
		streams.closeQuietly()
		streams.removeFiles()
		return runExecResult{}, fmt.Errorf("start run exec command: %w", err)
	}

	pid := cmd.Process.Pid
	pgid := runProcessGroupID(pid)
	sessionID := 0
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

	var waitErr error
	timedOut := false
	forwardedSignal := ""
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
	duration := time.Since(start)

	stdoutArtifact, stderrArtifact, err := recordRunStreamArtifacts(ctx, store, run.ID, "run exec", opts.limitBytes, streams, actor)
	if err != nil {
		return runExecResult{}, err
	}

	execution := runCommandExecutionResult{
		Status:         "passed",
		RunStatus:      "succeeded",
		Summary:        "Exec succeeded.",
		ExitCode:       0,
		Duration:       duration,
		TimedOut:       timedOut,
		Signal:         forwardedSignal,
		PID:            pid,
		ProcessGroupID: pgid,
		SessionID:      sessionID,
	}
	if timedOut {
		execution.Status = "cancelled"
		execution.RunStatus = "cancelled"
		execution.ExitCode = -1
		execution.Summary = fmt.Sprintf("Exec timed out after %s.", opts.timeout)
	} else if forwardedSignal != "" {
		execution.Status = "cancelled"
		execution.RunStatus = "cancelled"
		execution.ExitCode = -1
		execution.Summary = fmt.Sprintf("Exec interrupted by %s.", forwardedSignal)
	} else if waitErr != nil {
		execution.Status = "failed"
		execution.RunStatus = "failed"
		execution.ExitCode = -1
		if cmd.ProcessState != nil {
			execution.ExitCode = cmd.ProcessState.ExitCode()
		}
		execution.Summary = fmt.Sprintf("Exec failed with exit code %d.", execution.ExitCode)
	}

	validationStatus := "failed"
	if execution.Status == "passed" {
		validationStatus = "passed"
	}
	validationMetadata, err := executedValidationArtifactMetadata(runValidateOptions{
		command:        opts.command,
		timeout:        opts.timeout,
		limitBytes:     opts.limitBytes,
		allowDangerous: opts.allowDangerous,
	}, redactor, safety, validationStatus, execution.ExitCode, duration, timedOut, stdoutArtifact.ID, stderrArtifact.ID)
	if err != nil {
		return runExecResult{}, err
	}
	if _, err := tokservice.NewRunService(store).RecordValidationArtifact(ctx, storage.AddRunArtifactInput{
		RunID:    run.ID,
		Metadata: validationMetadata,
		Actor:    actor,
	}); err != nil {
		return runExecResult{}, err
	}

	metadata, err := runExecArtifactMetadata(opts, redactor, safety, execution, stdoutArtifact.ID, stderrArtifact.ID)
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

func defaultRunCommandEnv(baseEnv []string, run storage.Run, task storage.Task, project storage.Project) []string {
	allowed := map[string]bool{
		"HOME":           true,
		"PATH":           true,
		"SHELL":          true,
		"USER":           true,
		"LOGNAME":        true,
		"LANG":           true,
		"LANGUAGE":       true,
		"TERM":           true,
		"TMPDIR":         true,
		"TMP":            true,
		"TEMP":           true,
		"TZ":             true,
		"GOCACHE":        true,
		"GOMODCACHE":     true,
		"GOPATH":         true,
		"XDG_CACHE_HOME": true,
	}
	envByName := map[string]string{}
	for _, item := range baseEnv {
		name, _, ok := strings.Cut(item, "=")
		if !ok || name == "" {
			continue
		}
		if allowed[name] || strings.HasPrefix(name, "LC_") {
			envByName[name] = item
		}
	}
	envByName["PWD"] = "PWD=" + project.Path
	envByName["TOK_RUN_ID"] = "TOK_RUN_ID=" + strconv.FormatInt(run.ID, 10)
	envByName["TOK_TASK_ID"] = "TOK_TASK_ID=" + strconv.FormatInt(task.ID, 10)
	envByName["TOK_PROJECT_NAME"] = "TOK_PROJECT_NAME=" + project.Name

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

func runEnvNames(env []string) []string {
	names := make([]string, 0, len(env))
	for _, item := range env {
		name, _, ok := strings.Cut(item, "=")
		if ok && name != "" {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names
}

func dangerousRunCommandReason(command []string) string {
	if len(command) == 0 {
		return ""
	}
	name := strings.ToLower(filepath.Base(command[0]))
	switch name {
	case "sudo", "su", "doas":
		return "privilege escalation command"
	case "mkfs", "mkfs.ext4", "mkfs.xfs", "fdisk", "parted", "shutdown", "reboot":
		return "host destructive command"
	case "dd":
		for _, arg := range command[1:] {
			if strings.HasPrefix(arg, "of=/dev/") || strings.HasPrefix(arg, "of=/") {
				return "raw disk write command"
			}
		}
	case "rm":
		if rmCommandRecursive(command[1:]) {
			return "recursive remove command"
		}
	case "sh", "bash", "zsh", "fish":
		script := shellCommandScript(command[1:])
		if script != "" {
			return dangerousShellScriptReason(script)
		}
	}
	return ""
}

func rmCommandRecursive(args []string) bool {
	for _, arg := range args {
		if arg == "-r" || arg == "-R" || arg == "--recursive" {
			return true
		}
		if strings.HasPrefix(arg, "-") && strings.Contains(arg, "r") {
			return true
		}
	}
	return false
}

func shellCommandScript(args []string) string {
	for i, arg := range args {
		if arg == "-c" && i+1 < len(args) {
			return args[i+1]
		}
	}
	return ""
}

func dangerousShellScriptReason(script string) string {
	lower := strings.ToLower(script)
	switch {
	case strings.Contains(lower, "rm -rf") || strings.Contains(lower, "rm -fr"):
		return "recursive remove shell command"
	case strings.Contains(lower, "mkfs"):
		return "filesystem formatting shell command"
	case strings.Contains(lower, "curl ") && strings.Contains(lower, "| sh"):
		return "download pipe to shell command"
	case strings.Contains(lower, "wget ") && strings.Contains(lower, "| sh"):
		return "download pipe to shell command"
	case strings.Contains(lower, "dd ") && strings.Contains(lower, " of=/"):
		return "raw write shell command"
	default:
		return ""
	}
}
