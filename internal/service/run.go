package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"s26.sh/tok/internal/storage"
)

type RunService struct {
	store *storage.Store
}

func NewRunService(store *storage.Store) *RunService {
	return &RunService{store: store}
}

type FinishRunInput struct {
	ID               int64
	Status           string
	ResultSummary    string
	AllowUnvalidated bool
	OverrideReason   string
	Actor            storage.ActorRef
}

func (s *RunService) FinishRun(ctx context.Context, input FinishRunInput) (storage.Run, error) {
	if input.ID <= 0 {
		return storage.Run{}, errors.New("run id is required")
	}
	input.Status = strings.TrimSpace(input.Status)
	if !terminalRunStatus(input.Status) {
		return storage.Run{}, fmt.Errorf("invalid terminal run status %q", input.Status)
	}
	input.ResultSummary = strings.TrimSpace(input.ResultSummary)
	input.OverrideReason = strings.TrimSpace(input.OverrideReason)
	if input.ResultSummary == "" {
		return storage.Run{}, storage.ErrRunResultSummaryEmpty
	}

	current, err := s.store.GetRun(ctx, input.ID)
	if err != nil {
		return storage.Run{}, err
	}
	if terminalRunStatus(current.Status) {
		return storage.Run{}, storage.ErrInvalidRunTransition
	}

	if input.Status == "succeeded" {
		if input.AllowUnvalidated {
			if input.OverrideReason == "" {
				return storage.Run{}, ErrOverrideReasonRequired
			}
		} else {
			hasValidation, err := s.store.HasPassedValidationArtifact(ctx, input.ID)
			if err != nil {
				return storage.Run{}, err
			}
			if !hasValidation {
				return storage.Run{}, ErrRunValidationRequired
			}
		}
	}

	overrideReason := ""
	if input.Status == "succeeded" && input.AllowUnvalidated {
		overrideReason = input.OverrideReason
	}
	return s.store.FinishRun(ctx, storage.FinishRunInput{
		ID:             input.ID,
		Status:         input.Status,
		ResultSummary:  input.ResultSummary,
		OverrideReason: overrideReason,
		Actor:          input.Actor,
	})
}

func (s *RunService) RecordValidationArtifact(ctx context.Context, input storage.AddRunArtifactInput) (storage.RunArtifact, error) {
	input.Kind = "validation"
	return s.store.AddRunArtifact(ctx, input)
}

func terminalRunStatus(status string) bool {
	switch status {
	case "succeeded", "failed", "blocked", "cancelled":
		return true
	default:
		return false
	}
}
