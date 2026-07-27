package app

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	contextpkg "s26.sh/tok/internal/context"
)

func parseRunStartOptions(args []string) (runStartOptions, error) {
	var opts runStartOptions

	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--task":
			i++
			if i >= len(args) {
				return runStartOptions{}, &UsageError{Message: "--task requires a value", Code: 2}
			}
			taskID, err := parseTaskID(args[i])
			if err != nil {
				return runStartOptions{}, err
			}
			opts.taskID = taskID
		case strings.HasPrefix(arg, "--task="):
			taskID, err := parseTaskID(strings.TrimPrefix(arg, "--task="))
			if err != nil {
				return runStartOptions{}, err
			}
			opts.taskID = taskID
		case arg == "--limit":
			i++
			if i >= len(args) {
				return runStartOptions{}, &UsageError{Message: "--limit requires a value", Code: 2}
			}
			limit, err := parseContextLimit(args[i])
			if err != nil {
				return runStartOptions{}, err
			}
			opts.retrievalLimit = limit
		case strings.HasPrefix(arg, "--limit="):
			limit, err := parseContextLimit(strings.TrimPrefix(arg, "--limit="))
			if err != nil {
				return runStartOptions{}, err
			}
			opts.retrievalLimit = limit
		case arg == "--handoff-output":
			i++
			if i >= len(args) {
				return runStartOptions{}, &UsageError{Message: "--handoff-output requires a path", Code: 2}
			}
			opts.handoffOutput = args[i]
			if strings.TrimSpace(opts.handoffOutput) == "" {
				return runStartOptions{}, &UsageError{Message: "--handoff-output requires a path", Code: 2}
			}
		case strings.HasPrefix(arg, "--handoff-output="):
			opts.handoffOutput = strings.TrimPrefix(arg, "--handoff-output=")
			if strings.TrimSpace(opts.handoffOutput) == "" {
				return runStartOptions{}, &UsageError{Message: "--handoff-output requires a path", Code: 2}
			}
		case arg == "--allow-active":
			opts.allowActive = true
		case arg == "--json":
			opts.json = true
		default:
			return runStartOptions{}, &UsageError{Message: fmt.Sprintf("unknown run start option %q", arg), Code: 2}
		}
	}

	if opts.taskID == 0 {
		return runStartOptions{}, &UsageError{Message: "run start requires --task", Code: 2}
	}
	if opts.retrievalLimit == 0 {
		opts.retrievalLimit = contextpkg.DefaultRetrievalLimit
	}
	opts.handoffOutput = strings.TrimSpace(opts.handoffOutput)
	return opts, nil
}

func parseRunExecOptions(args []string) (runExecOptions, error) {
	opts := runExecOptions{
		timeout:    defaultRunExecTimeout,
		limitBytes: defaultRunArtifactLimitBytes,
	}
	commandStart := -1
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--":
			commandStart = i + 1
			i = len(args)
		case arg == "--task":
			i++
			if i >= len(args) {
				return runExecOptions{}, &UsageError{Message: "--task requires a value", Code: 2}
			}
			taskID, err := parseTaskID(args[i])
			if err != nil {
				return runExecOptions{}, err
			}
			opts.taskID = taskID
		case strings.HasPrefix(arg, "--task="):
			taskID, err := parseTaskID(strings.TrimPrefix(arg, "--task="))
			if err != nil {
				return runExecOptions{}, err
			}
			opts.taskID = taskID
		case arg == "--limit":
			i++
			if i >= len(args) {
				return runExecOptions{}, &UsageError{Message: "--limit requires a value", Code: 2}
			}
			limit, err := parseContextLimit(args[i])
			if err != nil {
				return runExecOptions{}, err
			}
			opts.retrievalLimit = limit
		case strings.HasPrefix(arg, "--limit="):
			limit, err := parseContextLimit(strings.TrimPrefix(arg, "--limit="))
			if err != nil {
				return runExecOptions{}, err
			}
			opts.retrievalLimit = limit
		case arg == "--timeout":
			i++
			if i >= len(args) {
				return runExecOptions{}, &UsageError{Message: "--timeout requires a duration", Code: 2}
			}
			timeout, err := parseRunTTL(args[i])
			if err != nil {
				return runExecOptions{}, err
			}
			opts.timeout = timeout
		case strings.HasPrefix(arg, "--timeout="):
			timeout, err := parseRunTTL(strings.TrimPrefix(arg, "--timeout="))
			if err != nil {
				return runExecOptions{}, err
			}
			opts.timeout = timeout
		case arg == "--limit-bytes":
			i++
			if i >= len(args) {
				return runExecOptions{}, &UsageError{Message: "--limit-bytes requires a value", Code: 2}
			}
			limit, err := parseRunArtifactLimit(args[i])
			if err != nil {
				return runExecOptions{}, err
			}
			opts.limitBytes = limit
		case strings.HasPrefix(arg, "--limit-bytes="):
			limit, err := parseRunArtifactLimit(strings.TrimPrefix(arg, "--limit-bytes="))
			if err != nil {
				return runExecOptions{}, err
			}
			opts.limitBytes = limit
		case arg == "--allow-dangerous":
			opts.allowDangerous = true
		case arg == "--allow-active":
			opts.allowActive = true
		case arg == "--json":
			opts.json = true
		default:
			return runExecOptions{}, &UsageError{Message: fmt.Sprintf("unknown run exec option %q", arg), Code: 2}
		}
	}

	if opts.taskID == 0 {
		return runExecOptions{}, &UsageError{Message: "run exec requires --task", Code: 2}
	}
	if opts.retrievalLimit == 0 {
		opts.retrievalLimit = contextpkg.DefaultRetrievalLimit
	}
	if commandStart < 0 || commandStart >= len(args) {
		return runExecOptions{}, &UsageError{Message: "run exec requires -- <command...>", Code: 2}
	}
	opts.command = args[commandStart:]
	if strings.TrimSpace(opts.command[0]) == "" {
		return runExecOptions{}, &UsageError{Message: "run exec requires -- <command...>", Code: 2}
	}
	if !opts.allowDangerous {
		if reason := dangerousRunCommandReason(opts.command); reason != "" {
			return runExecOptions{}, &UsageError{Message: fmt.Sprintf("run exec rejected dangerous command: %s; use --allow-dangerous to override", reason), Code: 2}
		}
	}
	return opts, nil
}

func parseRunAgentOptions(args []string) (runAgentOptions, error) {
	opts := runAgentOptions{
		contextMode: "file",
		timeout:     defaultRunExecTimeout,
		limitBytes:  defaultRunArtifactLimitBytes,
	}
	commandStart := -1
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--":
			commandStart = i + 1
			i = len(args)
		case arg == "--task":
			i++
			if i >= len(args) {
				return runAgentOptions{}, &UsageError{Message: "--task requires a value", Code: 2}
			}
			taskID, err := parseTaskID(args[i])
			if err != nil {
				return runAgentOptions{}, err
			}
			opts.taskID = taskID
		case strings.HasPrefix(arg, "--task="):
			taskID, err := parseTaskID(strings.TrimPrefix(arg, "--task="))
			if err != nil {
				return runAgentOptions{}, err
			}
			opts.taskID = taskID
		case arg == "--limit":
			i++
			if i >= len(args) {
				return runAgentOptions{}, &UsageError{Message: "--limit requires a value", Code: 2}
			}
			limit, err := parseContextLimit(args[i])
			if err != nil {
				return runAgentOptions{}, err
			}
			opts.retrievalLimit = limit
		case strings.HasPrefix(arg, "--limit="):
			limit, err := parseContextLimit(strings.TrimPrefix(arg, "--limit="))
			if err != nil {
				return runAgentOptions{}, err
			}
			opts.retrievalLimit = limit
		case arg == "--context":
			i++
			if i >= len(args) {
				return runAgentOptions{}, &UsageError{Message: "--context requires file, stdin or env", Code: 2}
			}
			opts.contextMode = args[i]
		case strings.HasPrefix(arg, "--context="):
			opts.contextMode = strings.TrimPrefix(arg, "--context=")
		case arg == "--timeout":
			i++
			if i >= len(args) {
				return runAgentOptions{}, &UsageError{Message: "--timeout requires a duration", Code: 2}
			}
			timeout, err := parseRunTTL(args[i])
			if err != nil {
				return runAgentOptions{}, err
			}
			opts.timeout = timeout
		case strings.HasPrefix(arg, "--timeout="):
			timeout, err := parseRunTTL(strings.TrimPrefix(arg, "--timeout="))
			if err != nil {
				return runAgentOptions{}, err
			}
			opts.timeout = timeout
		case arg == "--limit-bytes":
			i++
			if i >= len(args) {
				return runAgentOptions{}, &UsageError{Message: "--limit-bytes requires a value", Code: 2}
			}
			limit, err := parseRunArtifactLimit(args[i])
			if err != nil {
				return runAgentOptions{}, err
			}
			opts.limitBytes = limit
		case strings.HasPrefix(arg, "--limit-bytes="):
			limit, err := parseRunArtifactLimit(strings.TrimPrefix(arg, "--limit-bytes="))
			if err != nil {
				return runAgentOptions{}, err
			}
			opts.limitBytes = limit
		case arg == "--allow-dangerous":
			opts.allowDangerous = true
		case arg == "--allow-active":
			opts.allowActive = true
		case arg == "--allow-unvalidated":
			opts.allowUnvalidated = true
		case arg == "--override-reason":
			i++
			if i >= len(args) {
				return runAgentOptions{}, &UsageError{Message: "--override-reason requires a value", Code: 2}
			}
			opts.overrideReason = args[i]
		case strings.HasPrefix(arg, "--override-reason="):
			opts.overrideReason = strings.TrimPrefix(arg, "--override-reason=")
			if opts.overrideReason == "" {
				return runAgentOptions{}, &UsageError{Message: "--override-reason requires a value", Code: 2}
			}
		case arg == "--json":
			opts.json = true
		default:
			return runAgentOptions{}, &UsageError{Message: fmt.Sprintf("unknown run agent option %q", arg), Code: 2}
		}
	}

	if opts.taskID == 0 {
		return runAgentOptions{}, &UsageError{Message: "run agent requires --task", Code: 2}
	}
	if opts.retrievalLimit == 0 {
		opts.retrievalLimit = contextpkg.DefaultRetrievalLimit
	}
	opts.contextMode = strings.TrimSpace(opts.contextMode)
	if opts.contextMode != "file" && opts.contextMode != "stdin" && opts.contextMode != "env" {
		return runAgentOptions{}, &UsageError{Message: "run agent requires --context file, stdin or env", Code: 2}
	}
	if commandStart < 0 || commandStart >= len(args) {
		return runAgentOptions{}, &UsageError{Message: "run agent requires -- <adapter-command...>", Code: 2}
	}
	opts.command = args[commandStart:]
	if strings.TrimSpace(opts.command[0]) == "" {
		return runAgentOptions{}, &UsageError{Message: "run agent requires -- <adapter-command...>", Code: 2}
	}
	if !opts.allowDangerous {
		if reason := dangerousRunCommandReason(opts.command); reason != "" {
			return runAgentOptions{}, &UsageError{Message: fmt.Sprintf("run agent rejected dangerous command: %s; use --allow-dangerous to override", reason), Code: 2}
		}
	}
	opts.overrideReason = strings.TrimSpace(opts.overrideReason)
	if opts.allowUnvalidated && opts.overrideReason == "" {
		return runAgentOptions{}, &UsageError{Message: "run agent --allow-unvalidated requires --override-reason", Code: 2}
	}
	return opts, nil
}

func parseRunListOptions(args []string) (runListOptions, error) {
	var opts runListOptions

	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--project":
			i++
			if i >= len(args) {
				return runListOptions{}, &UsageError{Message: "--project requires a value", Code: 2}
			}
			opts.projectName = args[i]
		case strings.HasPrefix(arg, "--project="):
			opts.projectName = strings.TrimPrefix(arg, "--project=")
			if opts.projectName == "" {
				return runListOptions{}, &UsageError{Message: "--project requires a value", Code: 2}
			}
		case arg == "--task":
			i++
			if i >= len(args) {
				return runListOptions{}, &UsageError{Message: "--task requires a value", Code: 2}
			}
			taskID, err := parseTaskID(args[i])
			if err != nil {
				return runListOptions{}, err
			}
			opts.taskID = taskID
		case strings.HasPrefix(arg, "--task="):
			taskID, err := parseTaskID(strings.TrimPrefix(arg, "--task="))
			if err != nil {
				return runListOptions{}, err
			}
			opts.taskID = taskID
		case arg == "--status":
			i++
			if i >= len(args) {
				return runListOptions{}, &UsageError{Message: "--status requires a value", Code: 2}
			}
			opts.status = args[i]
		case strings.HasPrefix(arg, "--status="):
			opts.status = strings.TrimPrefix(arg, "--status=")
			if opts.status == "" {
				return runListOptions{}, &UsageError{Message: "--status requires a value", Code: 2}
			}
		case arg == "--json":
			opts.json = true
		default:
			return runListOptions{}, &UsageError{Message: fmt.Sprintf("unknown run list option %q", arg), Code: 2}
		}
	}

	opts.projectName = strings.TrimSpace(opts.projectName)
	opts.status = strings.TrimSpace(opts.status)
	if opts.status != "" && !validRunStatusOption(opts.status) {
		return runListOptions{}, &UsageError{Message: fmt.Sprintf("invalid run status %q", opts.status), Code: 2}
	}

	return opts, nil
}

func parseRunRecordValidationOptions(args []string) (runRecordValidationOptions, error) {
	if len(args) == 0 {
		return runRecordValidationOptions{}, &UsageError{Message: "run record-validation requires a run id", Code: 2}
	}

	runID, err := parseRunID(args[0])
	if err != nil {
		return runRecordValidationOptions{}, err
	}

	opts := runRecordValidationOptions{runID: runID}
	for i := 1; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--command":
			i++
			if i >= len(args) {
				return runRecordValidationOptions{}, &UsageError{Message: "--command requires a value", Code: 2}
			}
			opts.command = args[i]
		case strings.HasPrefix(arg, "--command="):
			opts.command = strings.TrimPrefix(arg, "--command=")
		case arg == "--status":
			i++
			if i >= len(args) {
				return runRecordValidationOptions{}, &UsageError{Message: "--status requires a value", Code: 2}
			}
			opts.status = args[i]
		case strings.HasPrefix(arg, "--status="):
			opts.status = strings.TrimPrefix(arg, "--status=")
		case arg == "--summary":
			i++
			if i >= len(args) {
				return runRecordValidationOptions{}, &UsageError{Message: "--summary requires a value", Code: 2}
			}
			opts.summary = args[i]
		case strings.HasPrefix(arg, "--summary="):
			opts.summary = strings.TrimPrefix(arg, "--summary=")
		case arg == "--json":
			opts.json = true
		default:
			return runRecordValidationOptions{}, &UsageError{Message: fmt.Sprintf("unknown run record-validation option %q", arg), Code: 2}
		}
	}

	opts.command = strings.TrimSpace(opts.command)
	if opts.command == "" {
		return runRecordValidationOptions{}, &UsageError{Message: "run record-validation requires --command", Code: 2}
	}
	opts.status = strings.TrimSpace(opts.status)
	if opts.status != "passed" && opts.status != "failed" {
		return runRecordValidationOptions{}, &UsageError{Message: "run record-validation requires --status passed or failed", Code: 2}
	}
	opts.summary = strings.TrimSpace(opts.summary)
	if opts.summary == "" {
		return runRecordValidationOptions{}, &UsageError{Message: "run record-validation requires --summary", Code: 2}
	}
	return opts, nil
}

func parseRunRecordArtifactOptions(args []string) (runRecordArtifactOptions, error) {
	if len(args) == 0 {
		return runRecordArtifactOptions{}, &UsageError{Message: "run record-artifact requires a run id", Code: 2}
	}

	runID, err := parseRunID(args[0])
	if err != nil {
		return runRecordArtifactOptions{}, err
	}

	opts := runRecordArtifactOptions{
		runID:      runID,
		limitBytes: defaultRunArtifactLimitBytes,
	}
	for i := 1; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--kind":
			i++
			if i >= len(args) {
				return runRecordArtifactOptions{}, &UsageError{Message: "--kind requires a value", Code: 2}
			}
			opts.kind = args[i]
		case strings.HasPrefix(arg, "--kind="):
			opts.kind = strings.TrimPrefix(arg, "--kind=")
		case arg == "--input":
			i++
			if i >= len(args) {
				return runRecordArtifactOptions{}, &UsageError{Message: "--input requires a path or -", Code: 2}
			}
			opts.inputPath = args[i]
		case strings.HasPrefix(arg, "--input="):
			opts.inputPath = strings.TrimPrefix(arg, "--input=")
		case arg == "--limit-bytes":
			i++
			if i >= len(args) {
				return runRecordArtifactOptions{}, &UsageError{Message: "--limit-bytes requires a value", Code: 2}
			}
			limit, err := parseRunArtifactLimit(args[i])
			if err != nil {
				return runRecordArtifactOptions{}, err
			}
			opts.limitBytes = limit
		case strings.HasPrefix(arg, "--limit-bytes="):
			limit, err := parseRunArtifactLimit(strings.TrimPrefix(arg, "--limit-bytes="))
			if err != nil {
				return runRecordArtifactOptions{}, err
			}
			opts.limitBytes = limit
		case arg == "--json":
			opts.json = true
		default:
			return runRecordArtifactOptions{}, &UsageError{Message: fmt.Sprintf("unknown run record-artifact option %q", arg), Code: 2}
		}
	}

	opts.kind = strings.TrimSpace(opts.kind)
	if !validFileRunArtifactKind(opts.kind) {
		return runRecordArtifactOptions{}, &UsageError{Message: "run record-artifact requires --kind stdout, stderr, log or patch", Code: 2}
	}
	opts.inputPath = strings.TrimSpace(opts.inputPath)
	if opts.inputPath == "" {
		return runRecordArtifactOptions{}, &UsageError{Message: "run record-artifact requires --input", Code: 2}
	}
	return opts, nil
}

func parseRunValidateOptions(args []string) (runValidateOptions, error) {
	if len(args) == 0 {
		return runValidateOptions{}, &UsageError{Message: "run validate requires a run id", Code: 2}
	}

	runID, err := parseRunID(args[0])
	if err != nil {
		return runValidateOptions{}, err
	}

	opts := runValidateOptions{
		runID:      runID,
		timeout:    defaultRunValidationTimeout,
		limitBytes: defaultRunArtifactLimitBytes,
	}
	commandStart := -1
	for i := 1; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--":
			commandStart = i + 1
			i = len(args)
		case arg == "--timeout":
			i++
			if i >= len(args) {
				return runValidateOptions{}, &UsageError{Message: "--timeout requires a duration", Code: 2}
			}
			timeout, err := parseRunTTL(args[i])
			if err != nil {
				return runValidateOptions{}, err
			}
			opts.timeout = timeout
		case strings.HasPrefix(arg, "--timeout="):
			timeout, err := parseRunTTL(strings.TrimPrefix(arg, "--timeout="))
			if err != nil {
				return runValidateOptions{}, err
			}
			opts.timeout = timeout
		case arg == "--limit-bytes":
			i++
			if i >= len(args) {
				return runValidateOptions{}, &UsageError{Message: "--limit-bytes requires a value", Code: 2}
			}
			limit, err := parseRunArtifactLimit(args[i])
			if err != nil {
				return runValidateOptions{}, err
			}
			opts.limitBytes = limit
		case strings.HasPrefix(arg, "--limit-bytes="):
			limit, err := parseRunArtifactLimit(strings.TrimPrefix(arg, "--limit-bytes="))
			if err != nil {
				return runValidateOptions{}, err
			}
			opts.limitBytes = limit
		case arg == "--allow-dangerous":
			opts.allowDangerous = true
		case arg == "--json":
			opts.json = true
		default:
			return runValidateOptions{}, &UsageError{Message: fmt.Sprintf("unknown run validate option %q", arg), Code: 2}
		}
	}

	if commandStart < 0 || commandStart >= len(args) {
		return runValidateOptions{}, &UsageError{Message: "run validate requires -- <command...>", Code: 2}
	}
	opts.command = args[commandStart:]
	if strings.TrimSpace(opts.command[0]) == "" {
		return runValidateOptions{}, &UsageError{Message: "run validate requires -- <command...>", Code: 2}
	}
	if !opts.allowDangerous {
		if reason := dangerousRunCommandReason(opts.command); reason != "" {
			return runValidateOptions{}, &UsageError{Message: fmt.Sprintf("run validate rejected dangerous command: %s; use --allow-dangerous to override", reason), Code: 2}
		}
	}
	return opts, nil
}

func parseRunShowOptions(args []string) (runShowOptions, error) {
	var opts runShowOptions

	for _, arg := range args {
		switch {
		case arg == "--json":
			opts.json = true
		default:
			if opts.runID != 0 {
				return runShowOptions{}, &UsageError{Message: "run show accepts exactly one run id", Code: 2}
			}
			runID, err := parseRunID(arg)
			if err != nil {
				return runShowOptions{}, err
			}
			opts.runID = runID
		}
	}

	if opts.runID == 0 {
		return runShowOptions{}, &UsageError{Message: "run show requires a run id", Code: 2}
	}
	return opts, nil
}

func parseRunCancelOptions(args []string) (runCancelOptions, error) {
	if len(args) == 0 {
		return runCancelOptions{}, &UsageError{Message: "run cancel requires a run id", Code: 2}
	}

	runID, err := parseRunID(args[0])
	if err != nil {
		return runCancelOptions{}, err
	}

	opts := runCancelOptions{runID: runID}
	for i := 1; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--summary":
			i++
			if i >= len(args) {
				return runCancelOptions{}, &UsageError{Message: "--summary requires a value", Code: 2}
			}
			opts.summary = args[i]
		case strings.HasPrefix(arg, "--summary="):
			opts.summary = strings.TrimPrefix(arg, "--summary=")
			if opts.summary == "" {
				return runCancelOptions{}, &UsageError{Message: "--summary requires a value", Code: 2}
			}
		case arg == "--json":
			opts.json = true
		default:
			return runCancelOptions{}, &UsageError{Message: fmt.Sprintf("unknown run cancel option %q", arg), Code: 2}
		}
	}

	opts.summary = strings.TrimSpace(opts.summary)
	if opts.summary == "" {
		return runCancelOptions{}, &UsageError{Message: "run cancel requires --summary", Code: 2}
	}
	return opts, nil
}

func parseRunHeartbeatOptions(args []string) (runHeartbeatOptions, error) {
	if len(args) == 0 {
		return runHeartbeatOptions{}, &UsageError{Message: "run heartbeat requires a run id", Code: 2}
	}

	runID, err := parseRunID(args[0])
	if err != nil {
		return runHeartbeatOptions{}, err
	}

	opts := runHeartbeatOptions{runID: runID, ttl: defaultRunLeaseTTL}
	for i := 1; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--owner":
			i++
			if i >= len(args) {
				return runHeartbeatOptions{}, &UsageError{Message: "--owner requires a value", Code: 2}
			}
			opts.owner = args[i]
		case strings.HasPrefix(arg, "--owner="):
			opts.owner = strings.TrimPrefix(arg, "--owner=")
			if opts.owner == "" {
				return runHeartbeatOptions{}, &UsageError{Message: "--owner requires a value", Code: 2}
			}
		case arg == "--ttl":
			i++
			if i >= len(args) {
				return runHeartbeatOptions{}, &UsageError{Message: "--ttl requires a value", Code: 2}
			}
			ttl, err := parseRunTTL(args[i])
			if err != nil {
				return runHeartbeatOptions{}, err
			}
			opts.ttl = ttl
		case strings.HasPrefix(arg, "--ttl="):
			ttl, err := parseRunTTL(strings.TrimPrefix(arg, "--ttl="))
			if err != nil {
				return runHeartbeatOptions{}, err
			}
			opts.ttl = ttl
		case arg == "--json":
			opts.json = true
		default:
			return runHeartbeatOptions{}, &UsageError{Message: fmt.Sprintf("unknown run heartbeat option %q", arg), Code: 2}
		}
	}

	opts.owner = strings.TrimSpace(opts.owner)
	return opts, nil
}

func parseRunRecoverOptions(args []string) (runRecoverOptions, error) {
	var opts runRecoverOptions

	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--summary":
			i++
			if i >= len(args) {
				return runRecoverOptions{}, &UsageError{Message: "--summary requires a value", Code: 2}
			}
			opts.summary = args[i]
		case strings.HasPrefix(arg, "--summary="):
			opts.summary = strings.TrimPrefix(arg, "--summary=")
			if opts.summary == "" {
				return runRecoverOptions{}, &UsageError{Message: "--summary requires a value", Code: 2}
			}
		case arg == "--now":
			i++
			if i >= len(args) {
				return runRecoverOptions{}, &UsageError{Message: "--now requires a value", Code: 2}
			}
			opts.now = args[i]
		case strings.HasPrefix(arg, "--now="):
			opts.now = strings.TrimPrefix(arg, "--now=")
			if opts.now == "" {
				return runRecoverOptions{}, &UsageError{Message: "--now requires a value", Code: 2}
			}
		case arg == "--json":
			opts.json = true
		default:
			return runRecoverOptions{}, &UsageError{Message: fmt.Sprintf("unknown run recover option %q", arg), Code: 2}
		}
	}

	opts.now = strings.TrimSpace(opts.now)
	opts.summary = strings.TrimSpace(opts.summary)
	if opts.summary == "" {
		return runRecoverOptions{}, &UsageError{Message: "run recover requires --summary", Code: 2}
	}
	return opts, nil
}

func parseRunFinishOptions(args []string) (runFinishOptions, error) {
	if len(args) == 0 {
		return runFinishOptions{}, &UsageError{Message: "run finish requires a run id", Code: 2}
	}

	runID, err := parseRunID(args[0])
	if err != nil {
		return runFinishOptions{}, err
	}

	opts := runFinishOptions{runID: runID}
	for i := 1; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--status":
			i++
			if i >= len(args) {
				return runFinishOptions{}, &UsageError{Message: "--status requires a value", Code: 2}
			}
			opts.status = args[i]
		case strings.HasPrefix(arg, "--status="):
			opts.status = strings.TrimPrefix(arg, "--status=")
			if opts.status == "" {
				return runFinishOptions{}, &UsageError{Message: "--status requires a value", Code: 2}
			}
		case arg == "--summary":
			i++
			if i >= len(args) {
				return runFinishOptions{}, &UsageError{Message: "--summary requires a value", Code: 2}
			}
			opts.summary = args[i]
		case strings.HasPrefix(arg, "--summary="):
			opts.summary = strings.TrimPrefix(arg, "--summary=")
			if opts.summary == "" {
				return runFinishOptions{}, &UsageError{Message: "--summary requires a value", Code: 2}
			}
		case arg == "--allow-unvalidated":
			opts.allowUnvalidated = true
		case arg == "--override-reason":
			i++
			if i >= len(args) {
				return runFinishOptions{}, &UsageError{Message: "--override-reason requires a value", Code: 2}
			}
			opts.overrideReason = args[i]
		case strings.HasPrefix(arg, "--override-reason="):
			opts.overrideReason = strings.TrimPrefix(arg, "--override-reason=")
			if opts.overrideReason == "" {
				return runFinishOptions{}, &UsageError{Message: "--override-reason requires a value", Code: 2}
			}
		case arg == "--json":
			opts.json = true
		default:
			return runFinishOptions{}, &UsageError{Message: fmt.Sprintf("unknown run finish option %q", arg), Code: 2}
		}
	}

	opts.status = strings.TrimSpace(opts.status)
	if opts.status == "" {
		return runFinishOptions{}, &UsageError{Message: "run finish requires --status", Code: 2}
	}
	opts.summary = strings.TrimSpace(opts.summary)
	if opts.summary == "" {
		return runFinishOptions{}, &UsageError{Message: "run finish requires --summary", Code: 2}
	}
	opts.overrideReason = strings.TrimSpace(opts.overrideReason)
	if opts.allowUnvalidated && opts.overrideReason == "" {
		return runFinishOptions{}, &UsageError{Message: "run finish --allow-unvalidated requires --override-reason", Code: 2}
	}
	return opts, nil
}

func parseRunID(value string) (int64, error) {
	id, err := strconv.ParseInt(value, 10, 64)
	if err != nil || id <= 0 {
		return 0, &UsageError{Message: fmt.Sprintf("invalid run id: %s", value), Code: 2}
	}
	return id, nil
}

func parseRunTTL(value string) (time.Duration, error) {
	ttl, err := time.ParseDuration(strings.TrimSpace(value))
	if err != nil || ttl <= 0 {
		return 0, &UsageError{Message: fmt.Sprintf("invalid run ttl: %s", value), Code: 2}
	}
	return ttl, nil
}

func validRunStatusOption(status string) bool {
	switch status {
	case "created", "in_progress", "succeeded", "failed", "blocked", "cancelled":
		return true
	default:
		return false
	}
}

func validFileRunArtifactKind(kind string) bool {
	switch kind {
	case "stdout", "stderr", "log", "patch":
		return true
	default:
		return false
	}
}

func parseRunArtifactLimit(value string) (int64, error) {
	limit, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	if err != nil || limit <= 0 {
		return 0, &UsageError{Message: fmt.Sprintf("invalid run artifact size limit: %s", value), Code: 2}
	}
	return limit, nil
}
