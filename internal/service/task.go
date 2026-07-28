package service

import (
	"context"
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
	if status == "done" {
		return storage.Task{}, ErrTaskStatusDoneUnsupported
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
		task, err := s.store.CompleteTaskWithOptions(ctx, storage.CompleteTaskInput{
			ID:             input.ID,
			Mode:           storage.CompletionOverride,
			Note:           input.Note,
			OverrideReason: input.OverrideReason,
			Actor:          input.Actor,
		})
		if errors.Is(err, storage.ErrTaskCompletionEvidenceRequired) {
			return storage.Task{}, ErrTaskCompletionEvidenceRequired
		}
		if errors.Is(err, storage.ErrTaskCompletionOverrideRequired) {
			return storage.Task{}, ErrOverrideReasonRequired
		}
		return task, err
	}
	task, err := s.store.CompleteTaskWithOptions(ctx, storage.CompleteTaskInput{
		ID:            input.ID,
		Mode:          storage.CompletionValidated,
		Note:          input.Note,
		EvidenceRunID: input.EvidenceRunID,
		Actor:         input.Actor,
	})
	if errors.Is(err, storage.ErrTaskCompletionEvidenceRequired) {
		return storage.Task{}, ErrTaskCompletionEvidenceRequired
	}
	return task, err
}
