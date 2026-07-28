package app

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"strconv"
	"time"

	contextpkg "s26.sh/tok/internal/context"
	tokservice "s26.sh/tok/internal/service"
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
		if errors.Is(err, storage.ErrTaskRunRequiresInProgress) {
			return fmt.Errorf("run start requires task %d to be in_progress", startOpts.taskID)
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
		if errors.Is(err, storage.ErrTaskRunRequiresInProgress) {
			return fmt.Errorf("run exec requires task %d to be in_progress", execOpts.taskID)
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

	finished, err := tokservice.NewRunService(store).FinishRun(ctx, tokservice.FinishRunInput{
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
		if errors.Is(err, storage.ErrTaskRunRequiresInProgress) {
			return fmt.Errorf("run agent requires task %d to be in_progress", agentOpts.taskID)
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

	finished, err := tokservice.NewRunService(store).FinishRun(ctx, tokservice.FinishRunInput{
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

	run, err := tokservice.NewRunService(store).FinishRun(ctx, tokservice.FinishRunInput{
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

	run, err := tokservice.NewRunService(store).FinishRun(ctx, tokservice.FinishRunInput{
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
		if errors.Is(err, tokservice.ErrRunValidationRequired) {
			return fmt.Errorf("run finish succeeded requires passed validation evidence; use run record-validation or --allow-unvalidated with --override-reason")
		}
		if errors.Is(err, tokservice.ErrOverrideReasonRequired) {
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

	artifact, err := tokservice.NewRunService(store).RecordValidationArtifact(ctx, storage.AddRunArtifactInput{
		RunID:    recordOpts.runID,
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
