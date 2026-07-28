package service

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"s26.sh/tok/internal/storage"
)

func TestRunServiceFinishSucceededRequiresPassedValidationOrOverride(t *testing.T) {
	ctx := context.Background()
	store := openInitializedTestStore(t)
	runSvc := NewRunService(store)
	_, task := createProjectTask(t, ctx, store, "Validated run")
	run := createRun(t, ctx, store, task.ID)

	_, err := runSvc.FinishRun(ctx, FinishRunInput{
		ID:            run.ID,
		Status:        "succeeded",
		ResultSummary: "No validation yet.",
	})
	if !errors.Is(err, ErrRunValidationRequired) {
		t.Fatalf("expected validation required error, got %v", err)
	}

	addValidation(t, ctx, store, run.ID, "failed")
	_, err = runSvc.FinishRun(ctx, FinishRunInput{
		ID:            run.ID,
		Status:        "succeeded",
		ResultSummary: "Validation failed.",
	})
	if !errors.Is(err, ErrRunValidationRequired) {
		t.Fatalf("expected failed validation to block success, got %v", err)
	}

	addValidation(t, ctx, store, run.ID, "passed")
	finished, err := runSvc.FinishRun(ctx, FinishRunInput{
		ID:            run.ID,
		Status:        "succeeded",
		ResultSummary: "Validation passed.",
	})
	if err != nil {
		t.Fatalf("FinishRun with passed validation returned error: %v", err)
	}
	if finished.Status != "succeeded" {
		t.Fatalf("unexpected finished run: %+v", finished)
	}

	overrideTask, err := store.CreateTask(ctx, storage.CreateTaskInput{
		ProjectID: task.ProjectID,
		Title:     "Override run",
	})
	if err != nil {
		t.Fatalf("CreateTask override returned error: %v", err)
	}
	overrideRun := createRun(t, ctx, store, overrideTask.ID)
	_, err = runSvc.FinishRun(ctx, FinishRunInput{
		ID:               overrideRun.ID,
		Status:           "succeeded",
		ResultSummary:    "Explicit override.",
		AllowUnvalidated: true,
	})
	if !errors.Is(err, ErrOverrideReasonRequired) {
		t.Fatalf("expected override reason error, got %v", err)
	}
	overridden, err := runSvc.FinishRun(ctx, FinishRunInput{
		ID:               overrideRun.ID,
		Status:           "succeeded",
		ResultSummary:    "Explicit override.",
		AllowUnvalidated: true,
		OverrideReason:   "Manual operator override.",
	})
	if err != nil {
		t.Fatalf("FinishRun override returned error: %v", err)
	}
	if overridden.Status != "succeeded" {
		t.Fatalf("unexpected override run: %+v", overridden)
	}
	artifacts, err := store.ListRunArtifacts(ctx, overrideRun.ID)
	if err != nil {
		t.Fatalf("ListRunArtifacts override returned error: %v", err)
	}
	if len(artifacts) != 1 || artifacts[0].Kind != "log" || !strings.Contains(artifacts[0].Metadata, "Manual operator override") {
		t.Fatalf("expected override audit artifact, got %+v", artifacts)
	}
}

func TestTaskServiceCompleteTaskRequiresValidatedEvidenceOrOverride(t *testing.T) {
	ctx := context.Background()
	store := openInitializedTestStore(t)
	taskSvc := NewTaskService(store)

	_, task := createProjectTask(t, ctx, store, "Missing evidence completion")
	claimTask(t, ctx, store, task.ProjectID, task.ID)
	_, err := taskSvc.CompleteTask(ctx, CompleteTaskInput{
		ID:   task.ID,
		Note: "Completed without run evidence.",
	})
	if !errors.Is(err, ErrTaskCompletionEvidenceRequired) {
		t.Fatalf("expected missing evidence error, got %v", err)
	}
	_, err = taskSvc.UpdateStatus(ctx, task.ID, "done", storage.ActorRef{})
	if !errors.Is(err, ErrTaskStatusDoneUnsupported) {
		t.Fatalf("expected direct status done rejection, got %v", err)
	}

	_, failedTask := createProjectTask(t, ctx, store, "Failed run completion")
	claimTask(t, ctx, store, failedTask.ProjectID, failedTask.ID)
	failedRun := createRun(t, ctx, store, failedTask.ID)
	if _, err := store.FinishRun(ctx, storage.FinishRunInput{
		ID:            failedRun.ID,
		Status:        "failed",
		ResultSummary: "Run failed.",
	}); err != nil {
		t.Fatalf("FinishRun failed returned error: %v", err)
	}
	_, err = taskSvc.CompleteTask(ctx, CompleteTaskInput{
		ID:   failedTask.ID,
		Note: "Completed after failed run.",
	})
	if !errors.Is(err, ErrTaskCompletionEvidenceRequired) {
		t.Fatalf("expected failed run evidence rejection, got %v", err)
	}

	_, validatedTask := createProjectTask(t, ctx, store, "Validated completion")
	claimTask(t, ctx, store, validatedTask.ProjectID, validatedTask.ID)
	validatedRun := createRun(t, ctx, store, validatedTask.ID)
	addValidation(t, ctx, store, validatedRun.ID, "passed")
	if _, err := store.FinishRun(ctx, storage.FinishRunInput{
		ID:            validatedRun.ID,
		Status:        "succeeded",
		ResultSummary: "Validated.",
	}); err != nil {
		t.Fatalf("FinishRun validated returned error: %v", err)
	}
	done, err := taskSvc.CompleteTask(ctx, CompleteTaskInput{
		ID:            validatedTask.ID,
		Note:          "Implemented and validated.",
		EvidenceRunID: validatedRun.ID,
	})
	if err != nil {
		t.Fatalf("CompleteTask validated returned error: %v", err)
	}
	if done.Status != "done" {
		t.Fatalf("unexpected completed task: %+v", done)
	}
	events, err := store.ListTaskEvents(ctx, validatedTask.ID)
	if err != nil {
		t.Fatalf("ListTaskEvents returned error: %v", err)
	}
	last := events[len(events)-1]
	if last.Type != "completed" {
		t.Fatalf("unexpected completion event: %+v", last)
	}
	if last.EvidenceRunID != validatedRun.ID {
		t.Fatalf("expected completion evidence run id %d, got %d", validatedRun.ID, last.EvidenceRunID)
	}
	if last.EvidenceArtifactID == 0 {
		t.Fatalf("expected completion evidence artifact id for validated run")
	}

	_, overrideTask := createProjectTask(t, ctx, store, "Override completion")
	claimTask(t, ctx, store, overrideTask.ProjectID, overrideTask.ID)
	_, err = taskSvc.CompleteTask(ctx, CompleteTaskInput{
		ID:               overrideTask.ID,
		Note:             "Override completion.",
		AllowUnvalidated: true,
	})
	if !errors.Is(err, ErrOverrideReasonRequired) {
		t.Fatalf("expected override reason error, got %v", err)
	}
	overridden, err := taskSvc.CompleteTask(ctx, CompleteTaskInput{
		ID:               overrideTask.ID,
		Note:             "Override completion.",
		AllowUnvalidated: true,
		OverrideReason:   "Manual task completion override.",
	})
	if err != nil {
		t.Fatalf("CompleteTask override returned error: %v", err)
	}
	if overridden.Status != "done" {
		t.Fatalf("unexpected override task: %+v", overridden)
	}
	events, err = store.ListTaskEvents(ctx, overrideTask.ID)
	if err != nil {
		t.Fatalf("ListTaskEvents override returned error: %v", err)
	}
	if len(events) < 1 || events[len(events)-1].Type != "completion_override" || events[len(events)-1].Body != "Manual task completion override." {
		t.Fatalf("expected completion override audit event, got %+v", events)
	}
}

func TestTaskServiceUpdateStatusRejectsDoneToOpenOrInProgress(t *testing.T) {
	ctx := context.Background()
	store := openInitializedTestStore(t)
	taskSvc := NewTaskService(store)

	_, task := createProjectTask(t, ctx, store, "Service no reopen")
	claimTask(t, ctx, store, task.ProjectID, task.ID)
	done, err := taskSvc.CompleteTask(ctx, CompleteTaskInput{
		ID:               task.ID,
		Note:             "Service completion override.",
		AllowUnvalidated: true,
		OverrideReason:   "Test policy.",
	})
	if err != nil {
		t.Fatalf("CompleteTask override returned error: %v", err)
	}
	if done.Status != "done" {
		t.Fatalf("expected task done, got %+v", done)
	}

	if _, err := taskSvc.UpdateStatus(ctx, task.ID, "open", storage.ActorRef{}); !errors.Is(err, storage.ErrInvalidTaskTransition) {
		t.Fatalf("expected done->open invalid transition, got %v", err)
	}
	if _, err := taskSvc.UpdateStatus(ctx, task.ID, "in_progress", storage.ActorRef{}); !errors.Is(err, storage.ErrInvalidTaskTransition) {
		t.Fatalf("expected done->in_progress invalid transition, got %v", err)
	}
}

func openInitializedTestStore(t *testing.T) *storage.Store {
	t.Helper()

	store, err := storage.Open(context.Background(), filepath.Join(t.TempDir(), storage.DatabaseFileName))
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Fatalf("Close returned error: %v", err)
		}
	})
	if err := store.Init(context.Background()); err != nil {
		t.Fatalf("Init returned error: %v", err)
	}
	return store
}

func createProjectTask(t *testing.T, ctx context.Context, store *storage.Store, title string) (storage.Project, storage.Task) {
	t.Helper()

	project, err := store.GetProject(ctx, "tok")
	if errors.Is(err, sql.ErrNoRows) {
		project, err = store.CreateProject(ctx, storage.CreateProjectInput{
			Name:        "tok",
			DisplayName: "TOK",
			Path:        "/tmp/tok",
		})
		if err != nil {
			t.Fatalf("CreateProject returned error: %v", err)
		}
	} else if err != nil {
		t.Fatalf("GetProject returned error: %v", err)
	}
	task, err := store.CreateTask(ctx, storage.CreateTaskInput{
		ProjectID: project.ID,
		Title:     title,
	})
	if err != nil {
		t.Fatalf("CreateTask returned error: %v", err)
	}
	return project, task
}

func claimTask(t *testing.T, ctx context.Context, store *storage.Store, projectID, taskID int64) {
	t.Helper()

	if _, err := store.ClaimTask(ctx, projectID, taskID); err != nil {
		t.Fatalf("ClaimTask returned error: %v", err)
	}
}

func createRun(t *testing.T, ctx context.Context, store *storage.Store, taskID int64) storage.Run {
	t.Helper()

	run, err := store.CreateRun(ctx, storage.CreateRunInput{
		TaskID:                 taskID,
		Status:                 "in_progress",
		HandoffContractVersion: "tok.handoff.v0",
		AllowActive:            true,
	})
	if err != nil {
		t.Fatalf("CreateRun returned error: %v", err)
	}
	return run
}

func addValidation(t *testing.T, ctx context.Context, store *storage.Store, runID int64, status string) {
	t.Helper()

	if _, err := store.AddRunArtifact(ctx, storage.AddRunArtifactInput{
		RunID:    runID,
		Kind:     "validation",
		Metadata: `{"status":"` + status + `","command":"go test ./..."}`,
	}); err != nil {
		t.Fatalf("AddRunArtifact %s returned error: %v", status, err)
	}
}
