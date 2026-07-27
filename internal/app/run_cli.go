package app

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	contextpkg "s26.sh/tok/internal/context"
	"s26.sh/tok/internal/retrieval"
	"s26.sh/tok/internal/storage"
)

const defaultRunLeaseTTL = 15 * time.Minute
const defaultRunArtifactLimitBytes int64 = 1024 * 1024
const defaultRunValidationTimeout = 10 * time.Minute
const defaultRunExecTimeout = 30 * time.Minute
const runExecTerminationGrace = 2 * time.Second
const agentAdapterContractV0 = "tok.agent_adapter.v0"

func (c *CLI) runRun(ctx context.Context, opts runtimeOptions) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if len(opts.args) < 2 {
		return &UsageError{
			Message: fmt.Sprintf("missing run command\n\nRun '%s help' for usage.", commandName),
			Code:    2,
		}
	}

	cfg, _, store, err := c.runtimeStore(ctx, opts)
	if err != nil {
		return err
	}
	defer store.Close()

	switch opts.args[1] {
	case "list":
		return c.runRunList(ctx, store, opts.args[2:])
	case "start":
		return c.runRunStart(ctx, store, opts.args[2:])
	case "exec":
		return c.runRunExec(ctx, store, cfg.DataDir, opts.args[2:])
	case "agent":
		return c.runRunAgent(ctx, store, cfg.DataDir, opts.args[2:])
	case "show":
		return c.runRunShow(ctx, store, opts.args[2:])
	case "record-validation":
		return c.runRunRecordValidation(ctx, store, opts.args[2:])
	case "record-artifact":
		return c.runRunRecordArtifact(ctx, store, cfg.DataDir, opts.args[2:])
	case "validate":
		return c.runRunValidate(ctx, store, cfg.DataDir, opts.args[2:])
	case "heartbeat":
		return c.runRunHeartbeat(ctx, store, opts.args[2:])
	case "recover":
		return c.runRunRecover(ctx, store, opts.args[2:])
	case "cancel":
		return c.runRunCancel(ctx, store, opts.args[2:])
	case "finish":
		return c.runRunFinish(ctx, store, opts.args[2:])
	default:
		return &UsageError{
			Message: fmt.Sprintf("unknown run command %q\n\nRun '%s help' for usage.", opts.args[1], commandName),
			Code:    2,
		}
	}
}

type runStartOptions struct {
	taskID         int64
	retrievalLimit int
	handoffOutput  string
	allowActive    bool
	json           bool
}

type runExecOptions struct {
	taskID         int64
	retrievalLimit int
	command        []string
	timeout        time.Duration
	limitBytes     int64
	allowDangerous bool
	allowActive    bool
	json           bool
}

type runAgentOptions struct {
	taskID           int64
	retrievalLimit   int
	command          []string
	contextMode      string
	timeout          time.Duration
	limitBytes       int64
	allowDangerous   bool
	allowActive      bool
	allowUnvalidated bool
	overrideReason   string
	json             bool
}

type runListOptions struct {
	projectName string
	taskID      int64
	status      string
	json        bool
}

type runShowOptions struct {
	runID int64
	json  bool
}

type runCancelOptions struct {
	runID   int64
	summary string
	json    bool
}

type runHeartbeatOptions struct {
	runID int64
	owner string
	ttl   time.Duration
	json  bool
}

type runRecoverOptions struct {
	now     string
	summary string
	json    bool
}

type runFinishOptions struct {
	runID            int64
	status           string
	summary          string
	allowUnvalidated bool
	overrideReason   string
	json             bool
}

type runRecordValidationOptions struct {
	runID   int64
	command string
	status  string
	summary string
	json    bool
}

type runRecordArtifactOptions struct {
	runID      int64
	kind       string
	inputPath  string
	limitBytes int64
	json       bool
}

type runValidateOptions struct {
	runID          int64
	command        []string
	timeout        time.Duration
	limitBytes     int64
	allowDangerous bool
	json           bool
}

func (c *CLI) runRunStart(ctx context.Context, store *storage.Store, args []string) error {
	startOpts, err := parseRunStartOptions(args)
	if err != nil {
		return err
	}

	task, err := store.GetTask(ctx, startOpts.taskID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("task not found: %d", startOpts.taskID)
		}
		return err
	}
	project, err := store.GetProjectByID(ctx, task.ProjectID)
	if err != nil {
		return err
	}
	actor, err := currentLocalHumanActor(ctx, store)
	if err != nil {
		return err
	}
	gitState := contextpkg.CommandGitInspector{}.Inspect(ctx, project.Path)
	now := time.Now().UTC()

	run, err := store.CreateRun(ctx, storage.CreateRunInput{
		TaskID:                 startOpts.taskID,
		Status:                 "in_progress",
		HandoffContractVersion: contextpkg.HandoffContractV0,
		RetrievalLimit:         startOpts.retrievalLimit,
		BaseBranch:             gitState.Branch,
		BaseHead:               gitState.Head,
		LeaseOwner:             runLeaseOwner(actor),
		HeartbeatAt:            formatRunTimestamp(now),
		ExpiresAt:              formatRunTimestamp(now.Add(defaultRunLeaseTTL)),
		AllowActive:            startOpts.allowActive,
		Actor:                  actor,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("task not found: %d", startOpts.taskID)
		}
		if errors.Is(err, storage.ErrActiveRunExists) {
			return fmt.Errorf("active run already exists for task %d; use --allow-active to override", startOpts.taskID)
		}
		return err
	}

	var artifacts []storage.RunArtifact
	if startOpts.handoffOutput != "" {
		artifact, err := writeRunHandoffArtifact(ctx, store, project, task, run, startOpts, actor)
		if err != nil {
			return err
		}
		artifacts = append(artifacts, artifact)
	}

	if startOpts.json {
		return printRunJSON(c.out, run, artifacts)
	}
	printRun(c.out, run)
	return nil
}

func (c *CLI) runRunExec(ctx context.Context, store *storage.Store, dataDir string, args []string) error {
	execOpts, err := parseRunExecOptions(args)
	if err != nil {
		return err
	}

	task, err := store.GetTask(ctx, execOpts.taskID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("task not found: %d", execOpts.taskID)
		}
		return err
	}
	project, err := store.GetProjectByID(ctx, task.ProjectID)
	if err != nil {
		return err
	}
	actor, err := currentLocalHumanActor(ctx, store)
	if err != nil {
		return err
	}
	gitState := contextpkg.CommandGitInspector{}.Inspect(ctx, project.Path)
	now := time.Now().UTC()

	run, err := store.CreateRun(ctx, storage.CreateRunInput{
		TaskID:                 execOpts.taskID,
		Status:                 "in_progress",
		HandoffContractVersion: contextpkg.HandoffContractV0,
		RetrievalLimit:         execOpts.retrievalLimit,
		BaseBranch:             gitState.Branch,
		BaseHead:               gitState.Head,
		LeaseOwner:             runLeaseOwner(actor),
		HeartbeatAt:            formatRunTimestamp(now),
		ExpiresAt:              formatRunTimestamp(now.Add(defaultRunLeaseTTL)),
		AllowActive:            execOpts.allowActive,
		Actor:                  actor,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("task not found: %d", execOpts.taskID)
		}
		if errors.Is(err, storage.ErrActiveRunExists) {
			return fmt.Errorf("active run already exists for task %d; use --allow-active to override", execOpts.taskID)
		}
		return err
	}

	if _, err := writeRunManagedHandoffArtifact(ctx, store, dataDir, project, task, run, execOpts.retrievalLimit, actor); err != nil {
		return err
	}
	result, err := executeRunCommand(ctx, store, dataDir, run, task, project, execOpts, actor)
	if err != nil {
		return err
	}

	finished, err := store.FinishRun(ctx, storage.FinishRunInput{
		ID:            run.ID,
		Status:        result.RunStatus,
		ResultSummary: result.Summary,
		Actor:         actor,
	})
	if err != nil {
		return err
	}
	artifacts, err := store.ListRunArtifacts(ctx, finished.ID)
	if err != nil {
		return err
	}
	if execOpts.json {
		return printRunJSON(c.out, finished, artifacts)
	}
	printRun(c.out, finished)
	return nil
}

func (c *CLI) runRunAgent(ctx context.Context, store *storage.Store, dataDir string, args []string) error {
	agentOpts, err := parseRunAgentOptions(args)
	if err != nil {
		return err
	}

	task, err := store.GetTask(ctx, agentOpts.taskID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("task not found: %d", agentOpts.taskID)
		}
		return err
	}
	project, err := store.GetProjectByID(ctx, task.ProjectID)
	if err != nil {
		return err
	}
	actor, err := currentLocalHumanActor(ctx, store)
	if err != nil {
		return err
	}
	gitState := contextpkg.CommandGitInspector{}.Inspect(ctx, project.Path)
	now := time.Now().UTC()

	run, err := store.CreateRun(ctx, storage.CreateRunInput{
		TaskID:                 agentOpts.taskID,
		Status:                 "in_progress",
		HandoffContractVersion: contextpkg.HandoffContractV0,
		RetrievalLimit:         agentOpts.retrievalLimit,
		BaseBranch:             gitState.Branch,
		BaseHead:               gitState.Head,
		LeaseOwner:             runLeaseOwner(actor),
		HeartbeatAt:            formatRunTimestamp(now),
		ExpiresAt:              formatRunTimestamp(now.Add(defaultRunLeaseTTL)),
		AllowActive:            agentOpts.allowActive,
		Actor:                  actor,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("task not found: %d", agentOpts.taskID)
		}
		if errors.Is(err, storage.ErrActiveRunExists) {
			return fmt.Errorf("active run already exists for task %d; use --allow-active to override", agentOpts.taskID)
		}
		return err
	}

	handoffArtifact, handoffText, err := writeRunManagedHandoffTextArtifact(ctx, store, dataDir, project, task, run, agentOpts.retrievalLimit, actor, "run agent")
	if err != nil {
		return err
	}
	result, err := executeAgentCommand(ctx, store, dataDir, run, task, project, agentOpts, actor, handoffArtifact, handoffText)
	if err != nil {
		return err
	}

	finished, err := store.FinishRun(ctx, storage.FinishRunInput{
		ID:               run.ID,
		Status:           result.RunStatus,
		ResultSummary:    result.Summary,
		AllowUnvalidated: agentOpts.allowUnvalidated,
		OverrideReason:   agentOpts.overrideReason,
		Actor:            actor,
	})
	if err != nil {
		return err
	}
	artifacts, err := store.ListRunArtifacts(ctx, finished.ID)
	if err != nil {
		return err
	}
	if agentOpts.json {
		return printRunJSON(c.out, finished, artifacts)
	}
	printRun(c.out, finished)
	return nil
}

func (c *CLI) runRunList(ctx context.Context, store *storage.Store, args []string) error {
	listOpts, err := parseRunListOptions(args)
	if err != nil {
		return err
	}

	var projectID int64
	if listOpts.projectName != "" {
		project, err := store.GetProject(ctx, listOpts.projectName)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return fmt.Errorf("project not found: %s", listOpts.projectName)
			}
			return err
		}
		projectID = project.ID
	}

	runs, err := store.ListRuns(ctx, storage.ListRunsOptions{
		ProjectID: projectID,
		TaskID:    listOpts.taskID,
		Status:    listOpts.status,
	})
	if err != nil {
		return err
	}

	if listOpts.json {
		return printRunsJSON(c.out, runs)
	}
	if len(runs) == 0 {
		fmt.Fprintln(c.out, "no runs")
		return nil
	}

	rows := [][]string{{"id", "task_id", "status", "started_at", "finished_at", "summary"}}
	for _, run := range runs {
		rows = append(rows, []string{
			strconv.FormatInt(run.ID, 10),
			strconv.FormatInt(run.TaskID, 10),
			run.Status,
			run.StartedAt,
			run.FinishedAt,
			run.ResultSummary,
		})
	}
	return printTerminalTable(c.out, rows)
}

func (c *CLI) runRunShow(ctx context.Context, store *storage.Store, args []string) error {
	showOpts, err := parseRunShowOptions(args)
	if err != nil {
		return err
	}

	run, err := store.GetRun(ctx, showOpts.runID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("run not found: %d", showOpts.runID)
		}
		return err
	}

	artifacts, err := store.ListRunArtifacts(ctx, run.ID)
	if err != nil {
		return err
	}
	if showOpts.json {
		return printRunJSON(c.out, run, artifacts)
	}
	printRun(c.out, run)
	return nil
}

func (c *CLI) runRunCancel(ctx context.Context, store *storage.Store, args []string) error {
	cancelOpts, err := parseRunCancelOptions(args)
	if err != nil {
		return err
	}

	actor, err := currentLocalHumanActor(ctx, store)
	if err != nil {
		return err
	}

	run, err := store.FinishRun(ctx, storage.FinishRunInput{
		ID:            cancelOpts.runID,
		Status:        "cancelled",
		ResultSummary: cancelOpts.summary,
		Actor:         actor,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("run not found: %d", cancelOpts.runID)
		}
		if errors.Is(err, storage.ErrInvalidRunTransition) {
			return fmt.Errorf("run cannot be cancelled from current status")
		}
		if errors.Is(err, storage.ErrRunResultSummaryEmpty) {
			return fmt.Errorf("run cancel requires --summary")
		}
		return err
	}

	artifacts, err := store.ListRunArtifacts(ctx, run.ID)
	if err != nil {
		return err
	}
	if cancelOpts.json {
		return printRunJSON(c.out, run, artifacts)
	}
	printRun(c.out, run)
	return nil
}

func (c *CLI) runRunHeartbeat(ctx context.Context, store *storage.Store, args []string) error {
	heartbeatOpts, err := parseRunHeartbeatOptions(args)
	if err != nil {
		return err
	}

	actor, err := currentLocalHumanActor(ctx, store)
	if err != nil {
		return err
	}
	owner := heartbeatOpts.owner
	if owner == "" {
		owner = runLeaseOwner(actor)
	}
	now := time.Now().UTC()
	run, err := store.HeartbeatRun(ctx, storage.HeartbeatRunInput{
		ID:        heartbeatOpts.runID,
		Owner:     owner,
		Now:       formatRunTimestamp(now),
		ExpiresAt: formatRunTimestamp(now.Add(heartbeatOpts.ttl)),
		Actor:     actor,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("run not found: %d", heartbeatOpts.runID)
		}
		if errors.Is(err, storage.ErrInvalidRunTransition) {
			return fmt.Errorf("run cannot be heartbeated from current status")
		}
		return err
	}

	artifacts, err := store.ListRunArtifacts(ctx, run.ID)
	if err != nil {
		return err
	}
	if heartbeatOpts.json {
		return printRunJSON(c.out, run, artifacts)
	}
	printRun(c.out, run)
	return nil
}

func (c *CLI) runRunRecover(ctx context.Context, store *storage.Store, args []string) error {
	recoverOpts, err := parseRunRecoverOptions(args)
	if err != nil {
		return err
	}

	actor, err := currentLocalHumanActor(ctx, store)
	if err != nil {
		return err
	}
	now := recoverOpts.now
	if now == "" {
		now = formatRunTimestamp(time.Now().UTC())
	}
	runs, err := store.RecoverStaleRuns(ctx, storage.RecoverStaleRunsInput{
		Now:           now,
		ResultSummary: recoverOpts.summary,
		Actor:         actor,
	})
	if err != nil {
		if errors.Is(err, storage.ErrRunResultSummaryEmpty) {
			return fmt.Errorf("run recover requires --summary")
		}
		return err
	}

	if recoverOpts.json {
		return printRunsJSON(c.out, runs)
	}
	if len(runs) == 0 {
		fmt.Fprintln(c.out, "no stale runs")
		return nil
	}
	rows := [][]string{{"id", "task_id", "status", "finished_at", "summary"}}
	for _, run := range runs {
		rows = append(rows, []string{
			strconv.FormatInt(run.ID, 10),
			strconv.FormatInt(run.TaskID, 10),
			run.Status,
			run.FinishedAt,
			run.ResultSummary,
		})
	}
	return printTerminalTable(c.out, rows)
}

func (c *CLI) runRunFinish(ctx context.Context, store *storage.Store, args []string) error {
	finishOpts, err := parseRunFinishOptions(args)
	if err != nil {
		return err
	}

	actor, err := currentLocalHumanActor(ctx, store)
	if err != nil {
		return err
	}

	run, err := store.FinishRun(ctx, storage.FinishRunInput{
		ID:               finishOpts.runID,
		Status:           finishOpts.status,
		ResultSummary:    finishOpts.summary,
		AllowUnvalidated: finishOpts.allowUnvalidated,
		OverrideReason:   finishOpts.overrideReason,
		Actor:            actor,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("run not found: %d", finishOpts.runID)
		}
		if errors.Is(err, storage.ErrInvalidRunTransition) {
			return fmt.Errorf("run cannot be finished from current status")
		}
		if errors.Is(err, storage.ErrRunResultSummaryEmpty) {
			return fmt.Errorf("run finish requires --summary")
		}
		if errors.Is(err, storage.ErrRunValidationRequired) {
			return fmt.Errorf("run finish succeeded requires passed validation evidence; use run record-validation or --allow-unvalidated with --override-reason")
		}
		if errors.Is(err, storage.ErrOverrideReasonRequired) {
			return fmt.Errorf("run finish --allow-unvalidated requires --override-reason")
		}
		return err
	}

	artifacts, err := store.ListRunArtifacts(ctx, run.ID)
	if err != nil {
		return err
	}
	if finishOpts.json {
		return printRunJSON(c.out, run, artifacts)
	}
	printRun(c.out, run)
	return nil
}

func (c *CLI) runRunRecordValidation(ctx context.Context, store *storage.Store, args []string) error {
	recordOpts, err := parseRunRecordValidationOptions(args)
	if err != nil {
		return err
	}

	metadata, err := validationArtifactMetadata(recordOpts, newRunMetadataRedactor(os.Environ()))
	if err != nil {
		return err
	}
	actor, err := currentLocalHumanActor(ctx, store)
	if err != nil {
		return err
	}

	artifact, err := store.AddRunArtifact(ctx, storage.AddRunArtifactInput{
		RunID:    recordOpts.runID,
		Kind:     "validation",
		Metadata: metadata,
		Actor:    actor,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("run not found: %d", recordOpts.runID)
		}
		return err
	}

	if recordOpts.json {
		return printRunArtifactJSON(c.out, artifact)
	}
	printRunArtifact(c.out, artifact)
	return nil
}

func (c *CLI) runRunRecordArtifact(ctx context.Context, store *storage.Store, dataDir string, args []string) error {
	recordOpts, err := parseRunRecordArtifactOptions(args)
	if err != nil {
		return err
	}

	actor, err := currentLocalHumanActor(ctx, store)
	if err != nil {
		return err
	}

	artifact, err := writeRunFileArtifact(ctx, store, dataDir, recordOpts, actor)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("run not found: %d", recordOpts.runID)
		}
		return err
	}

	if recordOpts.json {
		return printRunArtifactJSON(c.out, artifact)
	}
	printRunArtifact(c.out, artifact)
	return nil
}

func (c *CLI) runRunValidate(ctx context.Context, store *storage.Store, dataDir string, args []string) error {
	validateOpts, err := parseRunValidateOptions(args)
	if err != nil {
		return err
	}

	actor, err := currentLocalHumanActor(ctx, store)
	if err != nil {
		return err
	}

	artifact, err := executeRunValidation(ctx, store, dataDir, validateOpts, actor)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("run not found: %d", validateOpts.runID)
		}
		return err
	}

	if validateOpts.json {
		return printRunArtifactJSON(c.out, artifact)
	}
	printRunArtifact(c.out, artifact)
	return nil
}

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

type runOutput struct {
	ID                     int64               `json:"id"`
	TaskID                 int64               `json:"task_id"`
	Status                 string              `json:"status"`
	HandoffContractVersion string              `json:"handoff_contract_version"`
	RetrievalLimit         int                 `json:"retrieval_limit"`
	StartedAt              string              `json:"started_at"`
	FinishedAt             string              `json:"finished_at"`
	BaseBranch             string              `json:"base_branch"`
	BaseHead               string              `json:"base_head"`
	ResultSummary          string              `json:"result_summary"`
	LeaseOwner             string              `json:"lease_owner"`
	HeartbeatAt            string              `json:"heartbeat_at"`
	ExpiresAt              string              `json:"expires_at"`
	StartedBy              *actorOutput        `json:"started_by,omitempty"`
	FinishedBy             *actorOutput        `json:"finished_by,omitempty"`
	Artifacts              []runArtifactOutput `json:"artifacts"`
}

type runArtifactOutput struct {
	ID          int64        `json:"id"`
	RunID       int64        `json:"run_id"`
	Kind        string       `json:"kind"`
	Path        string       `json:"path"`
	ContentHash string       `json:"content_hash"`
	SizeBytes   int64        `json:"size_bytes"`
	Truncated   bool         `json:"truncated"`
	Metadata    string       `json:"metadata"`
	Actor       *actorOutput `json:"actor,omitempty"`
	CreatedAt   string       `json:"created_at"`
}

func printRun(out io.Writer, run storage.Run) {
	fmt.Fprintf(out, "id: %d\n", run.ID)
	fmt.Fprintf(out, "task_id: %d\n", run.TaskID)
	fmt.Fprintf(out, "status: %s\n", run.Status)
	fmt.Fprintf(out, "handoff_contract_version: %s\n", run.HandoffContractVersion)
	fmt.Fprintf(out, "retrieval_limit: %d\n", run.RetrievalLimit)
	fmt.Fprintf(out, "started_at: %s\n", run.StartedAt)
	fmt.Fprintf(out, "finished_at: %s\n", run.FinishedAt)
	fmt.Fprintf(out, "base_branch: %s\n", run.BaseBranch)
	fmt.Fprintf(out, "base_head: %s\n", run.BaseHead)
	fmt.Fprintf(out, "result_summary: %s\n", run.ResultSummary)
	fmt.Fprintf(out, "lease_owner: %s\n", run.LeaseOwner)
	fmt.Fprintf(out, "heartbeat_at: %s\n", run.HeartbeatAt)
	fmt.Fprintf(out, "expires_at: %s\n", run.ExpiresAt)
	if run.ActorName != "" {
		fmt.Fprintf(out, "started_by: %s/%s\n", run.ActorKind, run.ActorName)
	}
	if run.FinishedActorName != "" {
		fmt.Fprintf(out, "finished_by: %s/%s\n", run.FinishedActorKind, run.FinishedActorName)
	}
}

func printRunArtifact(out io.Writer, artifact storage.RunArtifact) {
	fmt.Fprintf(out, "id: %d\n", artifact.ID)
	fmt.Fprintf(out, "run_id: %d\n", artifact.RunID)
	fmt.Fprintf(out, "kind: %s\n", artifact.Kind)
	fmt.Fprintf(out, "path: %s\n", artifact.Path)
	fmt.Fprintf(out, "content_hash: %s\n", artifact.ContentHash)
	fmt.Fprintf(out, "size_bytes: %d\n", artifact.SizeBytes)
	fmt.Fprintf(out, "truncated: %t\n", artifact.Truncated)
	fmt.Fprintf(out, "metadata: %s\n", artifact.Metadata)
	if artifact.ActorName != "" {
		fmt.Fprintf(out, "actor: %s/%s\n", artifact.ActorKind, artifact.ActorName)
	}
	fmt.Fprintf(out, "created_at: %s\n", artifact.CreatedAt)
}

func printRunJSON(out io.Writer, run storage.Run, artifacts []storage.RunArtifact) error {
	encoder := json.NewEncoder(out)
	encoder.SetIndent("", "  ")
	return encoder.Encode(runOutputFromStorage(run, artifacts))
}

func printRunsJSON(out io.Writer, runs []storage.Run) error {
	outputs := make([]runOutput, 0, len(runs))
	for _, run := range runs {
		outputs = append(outputs, runOutputFromStorage(run, nil))
	}
	encoder := json.NewEncoder(out)
	encoder.SetIndent("", "  ")
	return encoder.Encode(outputs)
}

func printRunArtifactJSON(out io.Writer, artifact storage.RunArtifact) error {
	encoder := json.NewEncoder(out)
	encoder.SetIndent("", "  ")
	return encoder.Encode(runArtifactOutputFromStorage(artifact))
}

func runOutputFromStorage(run storage.Run, artifacts []storage.RunArtifact) runOutput {
	artifactOutputs := make([]runArtifactOutput, 0, len(artifacts))
	for _, artifact := range artifacts {
		artifactOutputs = append(artifactOutputs, runArtifactOutputFromStorage(artifact))
	}

	return runOutput{
		ID:                     run.ID,
		TaskID:                 run.TaskID,
		Status:                 run.Status,
		HandoffContractVersion: run.HandoffContractVersion,
		RetrievalLimit:         run.RetrievalLimit,
		StartedAt:              run.StartedAt,
		FinishedAt:             run.FinishedAt,
		BaseBranch:             run.BaseBranch,
		BaseHead:               run.BaseHead,
		ResultSummary:          run.ResultSummary,
		LeaseOwner:             run.LeaseOwner,
		HeartbeatAt:            run.HeartbeatAt,
		ExpiresAt:              run.ExpiresAt,
		StartedBy:              actorOutputFromSnapshot(run.ActorID, run.ActorKind, run.ActorName),
		FinishedBy:             actorOutputFromSnapshot(run.FinishedActorID, run.FinishedActorKind, run.FinishedActorName),
		Artifacts:              artifactOutputs,
	}
}

func runArtifactOutputFromStorage(artifact storage.RunArtifact) runArtifactOutput {
	return runArtifactOutput{
		ID:          artifact.ID,
		RunID:       artifact.RunID,
		Kind:        artifact.Kind,
		Path:        artifact.Path,
		ContentHash: artifact.ContentHash,
		SizeBytes:   artifact.SizeBytes,
		Truncated:   artifact.Truncated,
		Metadata:    artifact.Metadata,
		Actor:       actorOutputFromSnapshot(artifact.ActorID, artifact.ActorKind, artifact.ActorName),
		CreatedAt:   artifact.CreatedAt,
	}
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

func formatRunTimestamp(t time.Time) string {
	return t.UTC().Format("2006-01-02T15:04:05.000Z")
}

func runLeaseOwner(actor storage.ActorRef) string {
	if actor.Kind != "" && actor.Name != "" {
		return actor.Kind + "/" + actor.Name
	}
	if actor.Name != "" {
		return actor.Name
	}
	if actor.Kind != "" {
		return actor.Kind
	}
	return "local"
}

func writeRunHandoffArtifact(ctx context.Context, store *storage.Store, project storage.Project, task storage.Task, run storage.Run, opts runStartOptions, actor storage.ActorRef) (storage.RunArtifact, error) {
	outputPath := opts.handoffOutput
	if !filepath.IsAbs(outputPath) {
		absPath, err := filepath.Abs(outputPath)
		if err != nil {
			return storage.RunArtifact{}, fmt.Errorf("resolve handoff output path %q: %w", outputPath, err)
		}
		outputPath = absPath
	}

	builder := contextpkg.NewBuilder(store, retrieval.NewService(store))
	pkg, err := builder.Build(ctx, contextpkg.BuildInput{
		Project:        project,
		Task:           task,
		RetrievalLimit: opts.retrievalLimit,
	})
	if err != nil {
		return storage.RunArtifact{}, err
	}

	text := pkg.RenderText()
	if err := writeContextPackage(outputPath, text); err != nil {
		return storage.RunArtifact{}, err
	}

	return store.AddRunArtifact(ctx, storage.AddRunArtifactInput{
		RunID:       run.ID,
		Kind:        "handoff",
		Path:        outputPath,
		ContentHash: sha256ContentHash(text),
		Metadata:    `{"format":"text"}`,
		Actor:       actor,
	})
}

func writeRunManagedHandoffArtifact(ctx context.Context, store *storage.Store, dataDir string, project storage.Project, task storage.Task, run storage.Run, retrievalLimit int, actor storage.ActorRef) (storage.RunArtifact, error) {
	artifact, _, err := writeRunManagedHandoffTextArtifact(ctx, store, dataDir, project, task, run, retrievalLimit, actor, "run exec")
	return artifact, err
}

func writeRunManagedHandoffTextArtifact(ctx context.Context, store *storage.Store, dataDir string, project storage.Project, task storage.Task, run storage.Run, retrievalLimit int, actor storage.ActorRef, source string) (storage.RunArtifact, string, error) {
	artifacts, err := store.ListRunArtifacts(ctx, run.ID)
	if err != nil {
		return storage.RunArtifact{}, "", err
	}
	outputPath, _, err := nextRunArtifactPathWithExt(dataDir, run.ID, "handoff", ".md", len(artifacts)+1)
	if err != nil {
		return storage.RunArtifact{}, "", err
	}

	builder := contextpkg.NewBuilder(store, retrieval.NewService(store))
	pkg, err := builder.Build(ctx, contextpkg.BuildInput{
		Project:        project,
		Task:           task,
		RetrievalLimit: retrievalLimit,
	})
	if err != nil {
		return storage.RunArtifact{}, "", err
	}

	text := pkg.RenderText()
	if err := writeContextPackage(outputPath, text); err != nil {
		return storage.RunArtifact{}, "", err
	}
	metadata, err := json.Marshal(struct {
		Format string `json:"format"`
		Source string `json:"source"`
	}{
		Format: "text",
		Source: source,
	})
	if err != nil {
		return storage.RunArtifact{}, "", err
	}

	artifact, err := store.AddRunArtifact(ctx, storage.AddRunArtifactInput{
		RunID:       run.ID,
		Kind:        "handoff",
		Path:        outputPath,
		ContentHash: sha256ContentHash(text),
		SizeBytes:   int64(len([]byte(text))),
		Metadata:    string(metadata),
		Actor:       actor,
	})
	if err != nil {
		_ = os.Remove(outputPath)
		return storage.RunArtifact{}, "", err
	}
	return artifact, text, nil
}

func sha256ContentHash(value string) string {
	sum := sha256.Sum256([]byte(value))
	return fmt.Sprintf("sha256:%x", sum)
}

type runFileArtifactWriteResult struct {
	ContentHash       string
	SizeBytes         int64
	OriginalSizeBytes int64
	Truncated         bool
}

type boundedRunArtifactWriter struct {
	file       *os.File
	hasher     hashWriter
	path       string
	limitBytes int64
	written    int64
	original   int64
}

type hashWriter interface {
	Write([]byte) (int, error)
	Sum([]byte) []byte
}

type runCommandSafetyMetadata struct {
	EnvPolicy         string   `json:"env_policy"`
	EnvNames          []string `json:"env_names"`
	RedactionEnabled  bool     `json:"redaction_enabled"`
	AllowDangerous    bool     `json:"allow_dangerous"`
	DangerousOverride string   `json:"dangerous_override,omitempty"`
}

type runMetadataRedactor struct {
	values   []string
	patterns []string
}

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

	stdoutPath, nextOrdinal, err := nextRunArtifactPath(dataDir, run.ID, "stdout", len(artifacts)+1)
	if err != nil {
		return runExecResult{}, err
	}
	stderrPath, _, err := nextRunArtifactPath(dataDir, run.ID, "stderr", nextOrdinal)
	if err != nil {
		return runExecResult{}, err
	}

	stdoutWriter, err := newBoundedRunArtifactWriter(stdoutPath, opts.limitBytes)
	if err != nil {
		return runExecResult{}, err
	}
	defer func() { _, _ = stdoutWriter.Close() }()
	stderrWriter, err := newBoundedRunArtifactWriter(stderrPath, opts.limitBytes)
	if err != nil {
		_, _ = stdoutWriter.Close()
		_ = os.Remove(stdoutPath)
		return runExecResult{}, err
	}
	defer func() { _, _ = stderrWriter.Close() }()

	cmd := exec.Command(opts.command[0], opts.command[1:]...)
	cmd.Dir = project.Path
	cmd.Env = commandEnv
	if opts.contextMode == "stdin" {
		cmd.Stdin = strings.NewReader(handoffText)
	}
	cmd.Stdout = stdoutWriter
	cmd.Stderr = stderrWriter
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

	stdoutResult, err := stdoutWriter.Close()
	if err != nil {
		_, _ = stderrWriter.Close()
		_ = os.Remove(stdoutPath)
		_ = os.Remove(stderrPath)
		return runExecResult{}, err
	}
	stderrResult, err := stderrWriter.Close()
	if err != nil {
		_ = os.Remove(stdoutPath)
		_ = os.Remove(stderrPath)
		return runExecResult{}, err
	}

	stdoutMetadata, err := streamArtifactMetadata("run agent", "stdout", opts.limitBytes, stdoutResult)
	if err != nil {
		_ = os.Remove(stdoutPath)
		_ = os.Remove(stderrPath)
		return runExecResult{}, err
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
		return runExecResult{}, err
	}

	stderrMetadata, err := streamArtifactMetadata("run agent", "stderr", opts.limitBytes, stderrResult)
	if err != nil {
		_ = os.Remove(stderrPath)
		return runExecResult{}, err
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

func writeRunFileArtifact(ctx context.Context, store *storage.Store, dataDir string, opts runRecordArtifactOptions, actor storage.ActorRef) (storage.RunArtifact, error) {
	run, err := store.GetRun(ctx, opts.runID)
	if err != nil {
		return storage.RunArtifact{}, err
	}

	artifacts, err := store.ListRunArtifacts(ctx, run.ID)
	if err != nil {
		return storage.RunArtifact{}, err
	}

	var input io.Reader
	sourcePath := opts.inputPath
	var inputFile *os.File
	if opts.inputPath == "-" {
		input = os.Stdin
	} else {
		absPath, err := filepath.Abs(opts.inputPath)
		if err != nil {
			return storage.RunArtifact{}, fmt.Errorf("resolve artifact input path %q: %w", opts.inputPath, err)
		}
		sourcePath = absPath
		inputFile, err = os.Open(absPath)
		if err != nil {
			return storage.RunArtifact{}, fmt.Errorf("open artifact input %q: %w", absPath, err)
		}
		defer inputFile.Close()
		input = inputFile
	}

	outputPath, _, err := nextRunArtifactPath(dataDir, run.ID, opts.kind, len(artifacts)+1)
	if err != nil {
		return storage.RunArtifact{}, err
	}

	result, err := writeBoundedRunArtifactFile(outputPath, input, opts.limitBytes)
	if err != nil {
		return storage.RunArtifact{}, err
	}

	metadata, err := fileRunArtifactMetadata(opts, sourcePath, result)
	if err != nil {
		return storage.RunArtifact{}, err
	}

	artifact, err := store.AddRunArtifact(ctx, storage.AddRunArtifactInput{
		RunID:       run.ID,
		Kind:        opts.kind,
		Path:        outputPath,
		ContentHash: result.ContentHash,
		SizeBytes:   result.SizeBytes,
		Truncated:   result.Truncated,
		Metadata:    metadata,
		Actor:       actor,
	})
	if err != nil {
		_ = os.Remove(outputPath)
		return storage.RunArtifact{}, err
	}
	return artifact, nil
}

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
	return store.AddRunArtifact(ctx, storage.AddRunArtifactInput{
		RunID:    run.ID,
		Kind:     "validation",
		Metadata: metadata,
		Actor:    actor,
	})
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
	stdoutPath, nextOrdinal, err := nextRunArtifactPath(dataDir, run.ID, "stdout", len(artifacts)+1)
	if err != nil {
		return runExecResult{}, err
	}
	stderrPath, _, err := nextRunArtifactPath(dataDir, run.ID, "stderr", nextOrdinal)
	if err != nil {
		return runExecResult{}, err
	}

	stdoutWriter, err := newBoundedRunArtifactWriter(stdoutPath, opts.limitBytes)
	if err != nil {
		return runExecResult{}, err
	}
	defer func() { _, _ = stdoutWriter.Close() }()
	stderrWriter, err := newBoundedRunArtifactWriter(stderrPath, opts.limitBytes)
	if err != nil {
		_, _ = stdoutWriter.Close()
		_ = os.Remove(stdoutPath)
		return runExecResult{}, err
	}
	defer func() { _, _ = stderrWriter.Close() }()

	cmd := exec.Command(opts.command[0], opts.command[1:]...)
	cmd.Dir = project.Path
	cmd.Env = commandEnv
	cmd.Stdout = stdoutWriter
	cmd.Stderr = stderrWriter
	configureRunProcessGroup(cmd)

	start := time.Now()
	if err := cmd.Start(); err != nil {
		_, _ = stdoutWriter.Close()
		_, _ = stderrWriter.Close()
		_ = os.Remove(stdoutPath)
		_ = os.Remove(stderrPath)
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

	stdoutResult, err := stdoutWriter.Close()
	if err != nil {
		_, _ = stderrWriter.Close()
		_ = os.Remove(stdoutPath)
		_ = os.Remove(stderrPath)
		return runExecResult{}, err
	}
	stderrResult, err := stderrWriter.Close()
	if err != nil {
		_ = os.Remove(stdoutPath)
		_ = os.Remove(stderrPath)
		return runExecResult{}, err
	}

	stdoutMetadata, err := streamArtifactMetadata("run exec", "stdout", opts.limitBytes, stdoutResult)
	if err != nil {
		_ = os.Remove(stdoutPath)
		_ = os.Remove(stderrPath)
		return runExecResult{}, err
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
		return runExecResult{}, err
	}

	stderrMetadata, err := streamArtifactMetadata("run exec", "stderr", opts.limitBytes, stderrResult)
	if err != nil {
		_ = os.Remove(stderrPath)
		return runExecResult{}, err
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
	if _, err := store.AddRunArtifact(ctx, storage.AddRunArtifactInput{
		RunID:    run.ID,
		Kind:     "validation",
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

func nextRunArtifactPath(dataDir string, runID int64, kind string, startOrdinal int) (string, int, error) {
	return nextRunArtifactPathWithExt(dataDir, runID, kind, ".txt", startOrdinal)
}

func nextRunArtifactPathWithExt(dataDir string, runID int64, kind, ext string, startOrdinal int) (string, int, error) {
	outputDir := filepath.Join(dataDir, "run-artifacts", fmt.Sprintf("run-%d", runID))
	for ordinal := startOrdinal; ; ordinal++ {
		outputPath := filepath.Join(outputDir, fmt.Sprintf("%04d-%s%s", ordinal, kind, ext))
		_, err := os.Stat(outputPath)
		if errors.Is(err, os.ErrNotExist) {
			return outputPath, ordinal + 1, nil
		}
		if err != nil {
			return "", 0, fmt.Errorf("stat artifact path %q: %w", outputPath, err)
		}
	}
}

func newBoundedRunArtifactWriter(path string, limitBytes int64) (*boundedRunArtifactWriter, error) {
	if limitBytes <= 0 {
		return nil, errors.New("run artifact limit bytes must be positive")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("create run artifact directory: %w", err)
	}
	file, err := os.Create(path)
	if err != nil {
		return nil, fmt.Errorf("create run artifact %q: %w", path, err)
	}
	return &boundedRunArtifactWriter{
		file:       file,
		hasher:     sha256.New(),
		path:       path,
		limitBytes: limitBytes,
	}, nil
}

func (w *boundedRunArtifactWriter) Write(p []byte) (int, error) {
	if w.file == nil {
		return 0, errors.New("run artifact writer is closed")
	}
	w.original += int64(len(p))
	if w.written >= w.limitBytes {
		return len(p), nil
	}

	remaining := w.limitBytes - w.written
	writeN := int64(len(p))
	if writeN > remaining {
		writeN = remaining
	}
	chunk := p[:writeN]
	if _, err := w.file.Write(chunk); err != nil {
		return 0, fmt.Errorf("write run artifact %q: %w", w.path, err)
	}
	if _, err := w.hasher.Write(chunk); err != nil {
		return 0, fmt.Errorf("hash run artifact %q: %w", w.path, err)
	}
	w.written += writeN
	return len(p), nil
}

func (w *boundedRunArtifactWriter) Close() (runFileArtifactWriteResult, error) {
	if w.file != nil {
		if err := w.file.Close(); err != nil {
			w.file = nil
			return runFileArtifactWriteResult{}, fmt.Errorf("close run artifact %q: %w", w.path, err)
		}
		w.file = nil
	}
	return runFileArtifactWriteResult{
		ContentHash:       fmt.Sprintf("sha256:%x", w.hasher.Sum(nil)),
		SizeBytes:         w.written,
		OriginalSizeBytes: w.original,
		Truncated:         w.original > w.written,
	}, nil
}

func writeBoundedRunArtifactFile(path string, input io.Reader, limitBytes int64) (runFileArtifactWriteResult, error) {
	if limitBytes <= 0 {
		return runFileArtifactWriteResult{}, errors.New("run artifact limit bytes must be positive")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return runFileArtifactWriteResult{}, fmt.Errorf("create run artifact directory: %w", err)
	}

	output, err := os.Create(path)
	if err != nil {
		return runFileArtifactWriteResult{}, fmt.Errorf("create run artifact %q: %w", path, err)
	}

	hasher := sha256.New()
	var written int64
	var original int64
	var writeErr error
	buffer := make([]byte, 32*1024)
	for {
		n, readErr := input.Read(buffer)
		if n > 0 {
			original += int64(n)
			if written < limitBytes {
				remaining := limitBytes - written
				writeN := int64(n)
				if writeN > remaining {
					writeN = remaining
				}
				chunk := buffer[:writeN]
				if _, err := output.Write(chunk); err != nil {
					writeErr = fmt.Errorf("write run artifact %q: %w", path, err)
					break
				}
				if _, err := hasher.Write(chunk); err != nil {
					writeErr = fmt.Errorf("hash run artifact %q: %w", path, err)
					break
				}
				written += writeN
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			writeErr = fmt.Errorf("read run artifact input: %w", readErr)
			break
		}
	}

	if closeErr := output.Close(); writeErr == nil && closeErr != nil {
		writeErr = fmt.Errorf("close run artifact %q: %w", path, closeErr)
	}
	if writeErr != nil {
		_ = os.Remove(path)
		return runFileArtifactWriteResult{}, writeErr
	}

	return runFileArtifactWriteResult{
		ContentHash:       fmt.Sprintf("sha256:%x", hasher.Sum(nil)),
		SizeBytes:         written,
		OriginalSizeBytes: original,
		Truncated:         original > written,
	}, nil
}

func fileRunArtifactMetadata(opts runRecordArtifactOptions, sourcePath string, result runFileArtifactWriteResult) (string, error) {
	raw, err := json.Marshal(struct {
		Format            string `json:"format"`
		SourcePath        string `json:"source_path,omitempty"`
		SizeBytes         int64  `json:"size_bytes"`
		OriginalSizeBytes int64  `json:"original_size_bytes"`
		LimitBytes        int64  `json:"limit_bytes"`
		Truncated         bool   `json:"truncated"`
	}{
		Format:            "text",
		SourcePath:        sourcePath,
		SizeBytes:         result.SizeBytes,
		OriginalSizeBytes: result.OriginalSizeBytes,
		LimitBytes:        opts.limitBytes,
		Truncated:         result.Truncated,
	})
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

func validationStreamArtifactMetadata(stream string, limitBytes int64, result runFileArtifactWriteResult) (string, error) {
	return streamArtifactMetadata("run validate", stream, limitBytes, result)
}

func streamArtifactMetadata(source, stream string, limitBytes int64, result runFileArtifactWriteResult) (string, error) {
	raw, err := json.Marshal(struct {
		Format            string `json:"format"`
		Source            string `json:"source"`
		Stream            string `json:"stream"`
		SizeBytes         int64  `json:"size_bytes"`
		OriginalSizeBytes int64  `json:"original_size_bytes"`
		LimitBytes        int64  `json:"limit_bytes"`
		Truncated         bool   `json:"truncated"`
	}{
		Format:            "text",
		Source:            source,
		Stream:            stream,
		SizeBytes:         result.SizeBytes,
		OriginalSizeBytes: result.OriginalSizeBytes,
		LimitBytes:        limitBytes,
		Truncated:         result.Truncated,
	})
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

func runExecArtifactMetadata(opts runExecOptions, redactor runMetadataRedactor, safety runCommandSafetyMetadata, execution runCommandExecutionResult, stdoutArtifactID, stderrArtifactID int64) (string, error) {
	redactedArgs := redactor.redactArgs(opts.command)
	raw, err := json.Marshal(struct {
		Source           string                   `json:"source"`
		Command          string                   `json:"command"`
		Args             []string                 `json:"args"`
		Status           string                   `json:"status"`
		RunStatus        string                   `json:"run_status"`
		Summary          string                   `json:"summary"`
		ExitCode         int                      `json:"exit_code"`
		DurationMS       int64                    `json:"duration_ms"`
		TimedOut         bool                     `json:"timed_out"`
		TimeoutMS        int64                    `json:"timeout_ms"`
		Signal           string                   `json:"signal,omitempty"`
		PID              int                      `json:"pid"`
		ProcessGroupID   int                      `json:"process_group_id"`
		SessionID        int                      `json:"session_id"`
		ProcessGroup     bool                     `json:"process_group"`
		ForwardedSignals []string                 `json:"forwarded_signals"`
		StdoutArtifactID int64                    `json:"stdout_artifact_id"`
		StderrArtifactID int64                    `json:"stderr_artifact_id"`
		Safety           runCommandSafetyMetadata `json:"safety"`
	}{
		Source:           "run exec",
		Command:          strings.Join(redactedArgs, " "),
		Args:             redactedArgs,
		Status:           execution.Status,
		RunStatus:        execution.RunStatus,
		Summary:          redactor.redactString(execution.Summary),
		ExitCode:         execution.ExitCode,
		DurationMS:       execution.Duration.Milliseconds(),
		TimedOut:         execution.TimedOut,
		TimeoutMS:        opts.timeout.Milliseconds(),
		Signal:           execution.Signal,
		PID:              execution.PID,
		ProcessGroupID:   execution.ProcessGroupID,
		SessionID:        execution.SessionID,
		ProcessGroup:     execution.ProcessGroupID != 0,
		ForwardedSignals: []string{"SIGINT", "SIGTERM"},
		StdoutArtifactID: stdoutArtifactID,
		StderrArtifactID: stderrArtifactID,
		Safety:           safety,
	})
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

func runAgentArtifactMetadata(opts runAgentOptions, redactor runMetadataRedactor, safety runCommandSafetyMetadata, execution runCommandExecutionResult, handoffArtifactID, stdoutArtifactID, stderrArtifactID int64, artifactDir, contextFile, resultFile string, resultRead bool, resultErr error) (string, error) {
	redactedArgs := redactor.redactArgs(opts.command)
	resultError := ""
	if resultErr != nil {
		resultError = resultErr.Error()
	}
	raw, err := json.Marshal(struct {
		Source            string                   `json:"source"`
		AdapterContract   string                   `json:"adapter_contract"`
		Command           string                   `json:"command"`
		Args              []string                 `json:"args"`
		Status            string                   `json:"status"`
		RunStatus         string                   `json:"run_status"`
		Summary           string                   `json:"summary"`
		ExitCode          int                      `json:"exit_code"`
		DurationMS        int64                    `json:"duration_ms"`
		TimedOut          bool                     `json:"timed_out"`
		TimeoutMS         int64                    `json:"timeout_ms"`
		Signal            string                   `json:"signal,omitempty"`
		PID               int                      `json:"pid"`
		ProcessGroupID    int                      `json:"process_group_id"`
		SessionID         int                      `json:"session_id"`
		ProcessGroup      bool                     `json:"process_group"`
		ForwardedSignals  []string                 `json:"forwarded_signals"`
		ContextMode       string                   `json:"context_mode"`
		ContextFile       string                   `json:"context_file,omitempty"`
		ResultFile        string                   `json:"result_file"`
		ResultRead        bool                     `json:"result_read"`
		ResultError       string                   `json:"result_error,omitempty"`
		ArtifactDir       string                   `json:"artifact_dir"`
		HandoffArtifactID int64                    `json:"handoff_artifact_id"`
		StdoutArtifactID  int64                    `json:"stdout_artifact_id"`
		StderrArtifactID  int64                    `json:"stderr_artifact_id"`
		Safety            runCommandSafetyMetadata `json:"safety"`
	}{
		Source:            "run agent",
		AdapterContract:   agentAdapterContractV0,
		Command:           strings.Join(redactedArgs, " "),
		Args:              redactedArgs,
		Status:            execution.Status,
		RunStatus:         execution.RunStatus,
		Summary:           redactor.redactString(execution.Summary),
		ExitCode:          execution.ExitCode,
		DurationMS:        execution.Duration.Milliseconds(),
		TimedOut:          execution.TimedOut,
		TimeoutMS:         opts.timeout.Milliseconds(),
		Signal:            execution.Signal,
		PID:               execution.PID,
		ProcessGroupID:    execution.ProcessGroupID,
		SessionID:         execution.SessionID,
		ProcessGroup:      execution.ProcessGroupID != 0,
		ForwardedSignals:  []string{"SIGINT", "SIGTERM"},
		ContextMode:       opts.contextMode,
		ContextFile:       contextFile,
		ResultFile:        resultFile,
		ResultRead:        resultRead,
		ResultError:       redactor.redactString(resultError),
		ArtifactDir:       artifactDir,
		HandoffArtifactID: handoffArtifactID,
		StdoutArtifactID:  stdoutArtifactID,
		StderrArtifactID:  stderrArtifactID,
		Safety:            safety,
	})
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

func validationArtifactMetadata(opts runRecordValidationOptions, redactor runMetadataRedactor) (string, error) {
	raw, err := json.Marshal(struct {
		Command          string `json:"command"`
		Status           string `json:"status"`
		Summary          string `json:"summary"`
		RedactionEnabled bool   `json:"redaction_enabled"`
	}{
		Command:          redactor.redactString(opts.command),
		Status:           opts.status,
		Summary:          redactor.redactString(opts.summary),
		RedactionEnabled: true,
	})
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

func executedValidationArtifactMetadata(opts runValidateOptions, redactor runMetadataRedactor, safety runCommandSafetyMetadata, status string, exitCode int, duration time.Duration, timedOut bool, stdoutArtifactID, stderrArtifactID int64) (string, error) {
	summary := validationSummary(status, exitCode, timedOut, opts.timeout)
	redactedArgs := redactor.redactArgs(opts.command)
	raw, err := json.Marshal(struct {
		Command          string                   `json:"command"`
		Args             []string                 `json:"args"`
		Status           string                   `json:"status"`
		Summary          string                   `json:"summary"`
		ExitCode         int                      `json:"exit_code"`
		DurationMS       int64                    `json:"duration_ms"`
		TimedOut         bool                     `json:"timed_out"`
		TimeoutMS        int64                    `json:"timeout_ms"`
		StdoutArtifactID int64                    `json:"stdout_artifact_id"`
		StderrArtifactID int64                    `json:"stderr_artifact_id"`
		Safety           runCommandSafetyMetadata `json:"safety"`
	}{
		Command:          strings.Join(redactedArgs, " "),
		Args:             redactedArgs,
		Status:           status,
		Summary:          redactor.redactString(summary),
		ExitCode:         exitCode,
		DurationMS:       duration.Milliseconds(),
		TimedOut:         timedOut,
		TimeoutMS:        opts.timeout.Milliseconds(),
		StdoutArtifactID: stdoutArtifactID,
		StderrArtifactID: stderrArtifactID,
		Safety:           safety,
	})
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

func validationSummary(status string, exitCode int, timedOut bool, timeout time.Duration) string {
	if timedOut {
		return fmt.Sprintf("Validation timed out after %s.", timeout)
	}
	if status == "passed" {
		return "Validation passed."
	}
	return fmt.Sprintf("Validation failed with exit code %d.", exitCode)
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

func newRunMetadataRedactor(env []string) runMetadataRedactor {
	seen := map[string]bool{}
	var values []string
	for _, item := range env {
		name, value, ok := strings.Cut(item, "=")
		if !ok || value == "" || len(value) < 4 || !secretLikeName(name) || seen[value] {
			continue
		}
		seen[value] = true
		values = append(values, value)
	}

	var patterns []string
	for _, pattern := range strings.Split(os.Getenv("TOK_SECRET_PATTERNS"), ",") {
		pattern = strings.TrimSpace(pattern)
		if pattern != "" && !seen[pattern] {
			seen[pattern] = true
			patterns = append(patterns, pattern)
		}
	}
	return runMetadataRedactor{values: values, patterns: patterns}
}

func (r runMetadataRedactor) redactArgs(args []string) []string {
	redacted := make([]string, 0, len(args))
	for _, arg := range args {
		redacted = append(redacted, r.redactArg(arg))
	}
	return redacted
}

func (r runMetadataRedactor) redactArg(arg string) string {
	name, value, ok := strings.Cut(arg, "=")
	if ok && value != "" && secretLikeName(name) {
		return name + "=[REDACTED]"
	}
	return r.redactString(arg)
}

func (r runMetadataRedactor) redactString(value string) string {
	redacted := value
	for _, secret := range r.values {
		redacted = strings.ReplaceAll(redacted, secret, "[REDACTED]")
	}
	for _, pattern := range r.patterns {
		redacted = strings.ReplaceAll(redacted, pattern, "[REDACTED]")
	}
	return redacted
}

func secretLikeName(name string) bool {
	upper := strings.ToUpper(strings.TrimSpace(name))
	for _, marker := range []string{"SECRET", "TOKEN", "PASSWORD", "PASS", "API_KEY", "PRIVATE", "CREDENTIAL", "AUTH"} {
		if strings.Contains(upper, marker) {
			return true
		}
	}
	return false
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
		return 0, &UsageError{Message: fmt.Sprintf("invalid run artifact limit: %s", value), Code: 2}
	}
	return limit, nil
}
