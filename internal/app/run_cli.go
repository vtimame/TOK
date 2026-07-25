package app

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	contextpkg "s26.sh/tok/internal/context"
	"s26.sh/tok/internal/retrieval"
	"s26.sh/tok/internal/storage"
)

const defaultRunLeaseTTL = 15 * time.Minute

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

	_, _, store, err := c.runtimeStore(ctx, opts)
	if err != nil {
		return err
	}
	defer store.Close()

	switch opts.args[1] {
	case "list":
		return c.runRunList(ctx, store, opts.args[2:])
	case "start":
		return c.runRunStart(ctx, store, opts.args[2:])
	case "show":
		return c.runRunShow(ctx, store, opts.args[2:])
	case "record-validation":
		return c.runRunRecordValidation(ctx, store, opts.args[2:])
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
	runID   int64
	status  string
	summary string
	json    bool
}

type runRecordValidationOptions struct {
	runID   int64
	command string
	status  string
	summary string
	json    bool
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
		ID:            finishOpts.runID,
		Status:        finishOpts.status,
		ResultSummary: finishOpts.summary,
		Actor:         actor,
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

	metadata, err := validationArtifactMetadata(recordOpts)
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

func sha256ContentHash(value string) string {
	sum := sha256.Sum256([]byte(value))
	return fmt.Sprintf("sha256:%x", sum)
}

func validationArtifactMetadata(opts runRecordValidationOptions) (string, error) {
	raw, err := json.Marshal(struct {
		Command string `json:"command"`
		Status  string `json:"status"`
		Summary string `json:"summary"`
	}{
		Command: opts.command,
		Status:  opts.status,
		Summary: opts.summary,
	})
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

func validRunStatusOption(status string) bool {
	switch status {
	case "created", "in_progress", "succeeded", "failed", "blocked", "cancelled":
		return true
	default:
		return false
	}
}
