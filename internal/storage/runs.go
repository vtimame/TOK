package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

type ListRunsOptions struct {
	ProjectID int64
	TaskID    int64
	Status    string
	Statuses  []string
	Limit     int
	Offset    int
}

type Run struct {
	ID                     int64
	TaskID                 int64
	Status                 string
	HandoffContractVersion string
	RetrievalLimit         int
	StartedAt              string
	FinishedAt             string
	BaseBranch             string
	BaseHead               string
	ResultSummary          string
	LeaseOwner             string
	HeartbeatAt            string
	ExpiresAt              string
	ActorID                int64
	ActorKind              string
	ActorName              string
	FinishedActorID        int64
	FinishedActorKind      string
	FinishedActorName      string
}

type CreateRunInput struct {
	TaskID                 int64
	Status                 string
	HandoffContractVersion string
	RetrievalLimit         int
	BaseBranch             string
	BaseHead               string
	LeaseOwner             string
	HeartbeatAt            string
	ExpiresAt              string
	AllowActive            bool
	Actor                  ActorRef
}

type FinishRunInput struct {
	ID             int64
	Status         string
	ResultSummary  string
	OverrideReason string
	Actor          ActorRef
}

type HeartbeatRunInput struct {
	ID        int64
	Owner     string
	Now       string
	ExpiresAt string
	Actor     ActorRef
}

type RecoverStaleRunsInput struct {
	Now           string
	ResultSummary string
	Actor         ActorRef
}

func (s *Store) CreateRun(ctx context.Context, input CreateRunInput) (Run, error) {
	if input.TaskID <= 0 {
		return Run{}, errors.New("run task id is required")
	}
	status := strings.TrimSpace(input.Status)
	if status == "" {
		status = "created"
	}
	if !validRunStatus(status) {
		return Run{}, fmt.Errorf("invalid run status %q", status)
	}
	if runStatusTerminal(status) {
		return Run{}, ErrInvalidRunTransition
	}
	input.HandoffContractVersion = strings.TrimSpace(input.HandoffContractVersion)
	if input.HandoffContractVersion == "" {
		return Run{}, errors.New("run handoff contract version is required")
	}
	if input.RetrievalLimit <= 0 {
		input.RetrievalLimit = 5
	}
	input.LeaseOwner = strings.TrimSpace(input.LeaseOwner)
	input.HeartbeatAt = strings.TrimSpace(input.HeartbeatAt)
	input.ExpiresAt = strings.TrimSpace(input.ExpiresAt)

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Run{}, fmt.Errorf("begin create run transaction: %w", err)
	}
	defer rollback(tx)

	if err := lockInProgressTaskForRun(ctx, tx, input.TaskID); err != nil {
		return Run{}, err
	}
	if !input.AllowActive {
		active, err := s.hasActiveRunForTaskInTx(ctx, tx, input.TaskID)
		if err != nil {
			return Run{}, err
		}
		if active {
			return Run{}, ErrActiveRunExists
		}
	}

	actor := sanitizeActorRef(input.Actor)
	res, err := tx.ExecContext(ctx, `
		INSERT INTO runs (task_id, status, handoff_contract_version, retrieval_limit, base_branch, base_head, lease_owner, heartbeat_at, expires_at, actor_id, actor_kind, actor_name)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, input.TaskID, status, input.HandoffContractVersion, input.RetrievalLimit, input.BaseBranch, input.BaseHead, input.LeaseOwner, input.HeartbeatAt, input.ExpiresAt, actor.ID, actor.Kind, actor.Name)
	if err != nil {
		return Run{}, fmt.Errorf("create run: %w", err)
	}

	id, err := res.LastInsertId()
	if err != nil {
		return Run{}, fmt.Errorf("read created run id: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return Run{}, fmt.Errorf("commit create run transaction: %w", err)
	}

	return s.GetRun(ctx, id)
}

func lockInProgressTaskForRun(ctx context.Context, tx *sql.Tx, taskID int64) error {
	res, err := tx.ExecContext(ctx, `
		UPDATE tasks
		SET updated_at = updated_at
		WHERE id = ? AND status = 'in_progress'
	`, taskID)
	if err != nil {
		return fmt.Errorf("lock task for run: %w", err)
	}
	updated, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("read locked task count: %w", err)
	}
	if updated == 1 {
		return nil
	}
	if _, err := getTaskInTx(ctx, tx, taskID); err != nil {
		return err
	}
	return ErrTaskRunRequiresInProgress
}

func (s *Store) HasActiveRunForTask(ctx context.Context, taskID int64) (bool, error) {
	var id int64
	err := s.db.QueryRowContext(ctx, `
		SELECT id
		FROM runs
		WHERE task_id = ?
		  AND status IN ('created', 'in_progress')
		ORDER BY id
		LIMIT 1
	`, taskID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("find active run: %w", err)
	}
	return true, nil
}

func (s *Store) GetRun(ctx context.Context, id int64) (Run, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, task_id, status, handoff_contract_version, retrieval_limit, started_at, finished_at, base_branch, base_head, result_summary, lease_owner, heartbeat_at, expires_at, actor_id, actor_kind, actor_name, finished_actor_id, finished_actor_kind, finished_actor_name
		FROM runs
		WHERE id = ?
	`, id)
	return scanRun(row)
}

func (s *Store) ListRuns(ctx context.Context, opts ListRunsOptions) ([]Run, error) {
	statuses, err := normalizeRunStatuses(opts)
	if err != nil {
		return nil, err
	}
	if opts.TaskID < 0 {
		return nil, errors.New("run task id must be positive")
	}
	if opts.ProjectID < 0 {
		return nil, errors.New("run project id must be positive")
	}

	query := `
		SELECT r.id, r.task_id, r.status, r.handoff_contract_version, r.retrieval_limit, r.started_at, r.finished_at, r.base_branch, r.base_head, r.result_summary, r.lease_owner, r.heartbeat_at, r.expires_at, r.actor_id, r.actor_kind, r.actor_name, r.finished_actor_id, r.finished_actor_kind, r.finished_actor_name
		FROM runs r
	`
	args := []any{}
	where := []string{}
	if opts.ProjectID > 0 {
		query += " JOIN tasks t ON t.id = r.task_id"
		where = append(where, "t.project_id = ?")
		args = append(args, opts.ProjectID)
	}
	if opts.TaskID > 0 {
		where = append(where, "r.task_id = ?")
		args = append(args, opts.TaskID)
	}
	switch len(statuses) {
	case 0:
	case 1:
		where = append(where, "r.status = ?")
		args = append(args, statuses[0])
	default:
		where = append(where, "r.status IN ("+queryPlaceholders(len(statuses))+")")
		for _, status := range statuses {
			args = append(args, status)
		}
	}
	if len(where) > 0 {
		query += " WHERE " + strings.Join(where, " AND ")
	}
	query += " ORDER BY r.id DESC"
	if opts.Limit > 0 {
		query += " LIMIT ? OFFSET ?"
		args = append(args, opts.Limit, max(opts.Offset, 0))
	}

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list runs: %w", err)
	}
	defer rows.Close()

	var runs []Run
	for rows.Next() {
		run, err := scanRun(rows)
		if err != nil {
			return nil, fmt.Errorf("scan run: %w", err)
		}
		runs = append(runs, run)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate runs: %w", err)
	}

	return runs, nil
}

func (s *Store) FinishRun(ctx context.Context, input FinishRunInput) (Run, error) {
	if input.ID <= 0 {
		return Run{}, errors.New("run id is required")
	}
	input.Status = strings.TrimSpace(input.Status)
	if !runStatusTerminal(input.Status) {
		return Run{}, fmt.Errorf("invalid terminal run status %q", input.Status)
	}
	input.ResultSummary = strings.TrimSpace(input.ResultSummary)
	if input.ResultSummary == "" {
		return Run{}, ErrRunResultSummaryEmpty
	}
	input.OverrideReason = strings.TrimSpace(input.OverrideReason)

	current, err := s.GetRun(ctx, input.ID)
	if err != nil {
		return Run{}, err
	}
	if runStatusTerminal(current.Status) {
		return Run{}, ErrInvalidRunTransition
	}

	actor := sanitizeActorRef(input.Actor)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Run{}, fmt.Errorf("begin finish run transaction: %w", err)
	}
	defer rollback(tx)

	res, err := tx.ExecContext(ctx, `
		UPDATE runs
		SET status = ?,
			finished_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now'),
			result_summary = ?,
			finished_actor_id = ?,
			finished_actor_kind = ?,
			finished_actor_name = ?
		WHERE id = ?
		  AND status IN ('created', 'in_progress')
	`, input.Status, input.ResultSummary, actor.ID, actor.Kind, actor.Name, input.ID)
	if err != nil {
		return Run{}, fmt.Errorf("finish run: %w", err)
	}
	updated, err := res.RowsAffected()
	if err != nil {
		return Run{}, fmt.Errorf("read finished run count: %w", err)
	}
	if updated == 0 {
		return Run{}, ErrInvalidRunTransition
	}
	if input.Status == "succeeded" && input.OverrideReason != "" {
		metadata, err := runValidationOverrideMetadata(input.ResultSummary, input.OverrideReason)
		if err != nil {
			return Run{}, err
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO run_artifacts (run_id, kind, metadata, actor_id, actor_kind, actor_name)
			VALUES (?, 'log', ?, ?, ?, ?)
		`, input.ID, metadata, actor.ID, actor.Kind, actor.Name); err != nil {
			return Run{}, fmt.Errorf("record run validation override artifact: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return Run{}, fmt.Errorf("commit finish run transaction: %w", err)
	}

	return s.GetRun(ctx, input.ID)
}

func (s *Store) HasPassedValidationArtifact(ctx context.Context, runID int64) (bool, error) {
	artifacts, err := s.ListRunArtifacts(ctx, runID)
	if err != nil {
		return false, err
	}
	for _, artifact := range artifacts {
		if artifact.Kind != "validation" {
			continue
		}
		var metadata struct {
			Status string `json:"status"`
		}
		if err := json.Unmarshal([]byte(artifact.Metadata), &metadata); err != nil {
			continue
		}
		if strings.TrimSpace(metadata.Status) == "passed" {
			return true, nil
		}
	}
	return false, nil
}

func runValidationOverrideMetadata(summary, reason string) (string, error) {
	raw, err := json.Marshal(struct {
		Source  string `json:"source"`
		Status  string `json:"status"`
		Summary string `json:"summary"`
		Reason  string `json:"reason"`
	}{
		Source:  "run finish override",
		Status:  "succeeded",
		Summary: summary,
		Reason:  reason,
	})
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

func (s *Store) HeartbeatRun(ctx context.Context, input HeartbeatRunInput) (Run, error) {
	if input.ID <= 0 {
		return Run{}, errors.New("run id is required")
	}
	input.Owner = strings.TrimSpace(input.Owner)
	if input.Owner == "" {
		return Run{}, errors.New("run heartbeat owner is required")
	}
	input.Now = strings.TrimSpace(input.Now)
	if input.Now == "" {
		return Run{}, errors.New("run heartbeat timestamp is required")
	}
	input.ExpiresAt = strings.TrimSpace(input.ExpiresAt)
	if input.ExpiresAt == "" {
		return Run{}, errors.New("run heartbeat expiration is required")
	}

	current, err := s.GetRun(ctx, input.ID)
	if err != nil {
		return Run{}, err
	}
	if runStatusTerminal(current.Status) {
		return Run{}, ErrInvalidRunTransition
	}

	res, err := s.db.ExecContext(ctx, `
		UPDATE runs
		SET lease_owner = ?,
			heartbeat_at = ?,
			expires_at = ?
		WHERE id = ?
		  AND status IN ('created', 'in_progress')
	`, input.Owner, input.Now, input.ExpiresAt, input.ID)
	if err != nil {
		return Run{}, fmt.Errorf("heartbeat run: %w", err)
	}
	updated, err := res.RowsAffected()
	if err != nil {
		return Run{}, fmt.Errorf("read heartbeat run count: %w", err)
	}
	if updated == 0 {
		return Run{}, ErrInvalidRunTransition
	}

	return s.GetRun(ctx, input.ID)
}

func (s *Store) RecoverStaleRuns(ctx context.Context, input RecoverStaleRunsInput) ([]Run, error) {
	input.Now = strings.TrimSpace(input.Now)
	if input.Now == "" {
		return nil, errors.New("run recovery timestamp is required")
	}
	input.ResultSummary = strings.TrimSpace(input.ResultSummary)
	if input.ResultSummary == "" {
		return nil, ErrRunResultSummaryEmpty
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT id
		FROM runs
		WHERE status IN ('created', 'in_progress')
		  AND expires_at != ''
		  AND expires_at < ?
		ORDER BY id
	`, input.Now)
	if err != nil {
		return nil, fmt.Errorf("list stale runs: %w", err)
	}
	defer rows.Close()

	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan stale run id: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate stale runs: %w", err)
	}

	recovered := make([]Run, 0, len(ids))
	for _, id := range ids {
		run, err := s.FinishRun(ctx, FinishRunInput{
			ID:            id,
			Status:        "cancelled",
			ResultSummary: input.ResultSummary,
			Actor:         input.Actor,
		})
		if err != nil {
			return nil, err
		}
		recovered = append(recovered, run)
	}
	return recovered, nil
}
