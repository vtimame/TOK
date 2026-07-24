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

	contextpkg "s26.sh/tok/internal/context"
	"s26.sh/tok/internal/retrieval"
	"s26.sh/tok/internal/storage"
)

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
	case "start":
		return c.runRunStart(ctx, store, opts.args[2:])
	case "show":
		return c.runRunShow(ctx, store, opts.args[2:])
	case "record-validation":
		return c.runRunRecordValidation(ctx, store, opts.args[2:])
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
	json           bool
}

type runShowOptions struct {
	runID int64
	json  bool
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

	run, err := store.CreateRun(ctx, storage.CreateRunInput{
		TaskID:                 startOpts.taskID,
		Status:                 "in_progress",
		HandoffContractVersion: contextpkg.HandoffContractV0,
		RetrievalLimit:         startOpts.retrievalLimit,
		BaseBranch:             gitState.Branch,
		BaseHead:               gitState.Head,
		Actor:                  actor,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("task not found: %d", startOpts.taskID)
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
