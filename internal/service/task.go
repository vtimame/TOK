package service

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"s26.sh/tok/internal/storage"
)

type TaskService struct {
	store *storage.Store
}

func NewTaskService(store *storage.Store) *TaskService {
	return &TaskService{store: store}
}

type CompleteTaskInput struct {
	ID               int64
	Note             string
	EvidenceRunID    int64
	AllowUnvalidated bool
	OverrideReason   string
	Actor            storage.ActorRef
}

func (s *TaskService) UpdateStatus(ctx context.Context, id int64, status string, actor storage.ActorRef) (storage.Task, error) {
	status = strings.TrimSpace(status)
	if status != "done" {
		return s.store.UpdateTaskStatusByActor(ctx, id, status, actor)
	}
	if err := s.requireTaskCompletionEvidence(ctx, id, 0); err != nil {
		return storage.Task{}, err
	}
	return s.store.UpdateTaskStatusByActor(ctx, id, status, actor)
}

func (s *TaskService) CompleteTask(ctx context.Context, input CompleteTaskInput) (storage.Task, error) {
	input.Note = strings.TrimSpace(input.Note)
	input.OverrideReason = strings.TrimSpace(input.OverrideReason)
	if input.ID <= 0 {
		return storage.Task{}, errors.New("task id is required")
	}
	if input.Note == "" {
		return storage.Task{}, storage.ErrTaskCompletionNoteEmpty
	}

	if input.AllowUnvalidated {
		if input.OverrideReason == "" {
			return storage.Task{}, ErrOverrideReasonRequired
		}
		return s.store.CompleteTaskWithOptions(ctx, storage.CompleteTaskInput{
			ID:             input.ID,
			Note:           input.Note,
			OverrideReason: input.OverrideReason,
			Actor:          input.Actor,
		})
	}

	if err := s.requireTaskCompletionEvidence(ctx, input.ID, input.EvidenceRunID); err != nil {
		return storage.Task{}, err
	}
	return s.store.CompleteTaskWithOptions(ctx, storage.CompleteTaskInput{
		ID:    input.ID,
		Note:  input.Note,
		Actor: input.Actor,
	})
}

func (s *TaskService) requireTaskCompletionEvidence(ctx context.Context, taskID, evidenceRunID int64) error {
	if taskID <= 0 {
		return errors.New("task id is required")
	}
	task, err := s.store.GetTask(ctx, taskID)
	if err != nil {
		return err
	}
	if task.Status != "in_progress" {
		return storage.ErrInvalidTaskTransition
	}
	activeRun, err := s.store.HasActiveRunForTask(ctx, taskID)
	if err != nil {
		return err
	}
	if activeRun {
		return storage.ErrActiveRunExists
	}

	if evidenceRunID == 0 {
		evidenceRunID, err = s.latestValidatedSucceededRunID(ctx, taskID)
		if err != nil {
			return err
		}
	}
	if evidenceRunID == 0 {
		return ErrTaskCompletionEvidenceRequired
	}
	valid, err := s.isTaskCompletionEvidenceRun(ctx, taskID, evidenceRunID)
	if err != nil {
		return err
	}
	if !valid {
		return ErrTaskCompletionEvidenceRequired
	}
	return nil
}

func (s *TaskService) latestValidatedSucceededRunID(ctx context.Context, taskID int64) (int64, error) {
	runs, err := s.store.ListRuns(ctx, storage.ListRunsOptions{
		TaskID: taskID,
		Status: "succeeded",
	})
	if err != nil {
		return 0, err
	}
	for _, run := range runs {
		hasValidation, err := s.store.HasPassedValidationArtifact(ctx, run.ID)
		if err != nil {
			return 0, err
		}
		if hasValidation {
			return run.ID, nil
		}
	}
	return 0, nil
}

func (s *TaskService) isTaskCompletionEvidenceRun(ctx context.Context, taskID, runID int64) (bool, error) {
	run, err := s.store.GetRun(ctx, runID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	if run.TaskID != taskID || run.Status != "succeeded" {
		return false, nil
	}
	return s.store.HasPassedValidationArtifact(ctx, runID)
}
