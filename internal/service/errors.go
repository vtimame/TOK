package service

import "errors"

var (
	ErrRunValidationRequired          = errors.New("passed validation evidence is required")
	ErrTaskCompletionEvidenceRequired = errors.New("task completion evidence run with passed validation is required")
	ErrOverrideReasonRequired         = errors.New("override reason is required")
)
