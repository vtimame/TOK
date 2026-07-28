package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"strings"
)

type Task struct {
	ID                 int64
	ProjectID          int64
	Status             string
	Title              string
	Description        string
	AcceptanceCriteria string
	Notes              string
	Source             string
	ExternalID         string
	ExternalURL        string
	ExternalRevision   string
	CreatedAt          string
	UpdatedAt          string
}

type CreateTaskInput struct {
	ProjectID          int64
	Title              string
	Description        string
	AcceptanceCriteria string
	Notes              string
	Source             string
	ExternalID         string
	ExternalURL        string
	ExternalRevision   string
	Actor              ActorRef
}

type ListTasksOptions struct {
	Status    string
	Statuses  []string
	ProjectID int64
	Limit     int
	Cursor    int64
}

type TaskEvent struct {
	ID                 int64
	TaskID             int64
	Type               string
	Body               string
	FromStatus         string
	ToStatus           string
	ActorID            int64
	ActorKind          string
	ActorName          string
	EvidenceRunID      int64
	EvidenceArtifactID int64
	CreatedAt          string
}

type CompleteTaskInput struct {
	ID             int64
	Mode           CompletionMode
	Note           string
	EvidenceRunID  int64
	OverrideReason string
	Actor          ActorRef
}

type CompletionMode string

const (
	CompletionValidated CompletionMode = "validated"
	CompletionOverride  CompletionMode = "override"
)

type UpdateTaskExternalReferenceInput struct {
	ID               int64
	Source           string
	ExternalID       string
	ExternalURL      string
	ExternalRevision string
	Actor            ActorRef
}

func (s *Store) CreateTask(ctx context.Context, input CreateTaskInput) (Task, error) {
	if strings.TrimSpace(input.Title) == "" {
		return Task{}, errors.New("task title is required")
	}
	if input.ProjectID <= 0 {
		return Task{}, errors.New("task project id is required")
	}
	source, externalID, externalURL, externalRevision, err := normalizeTaskExternalReference(input.Source, input.ExternalID, input.ExternalURL, input.ExternalRevision)
	if err != nil {
		return Task{}, err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Task{}, fmt.Errorf("begin create task transaction: %w", err)
	}
	defer rollback(tx)

	res, err := tx.ExecContext(ctx, `
		INSERT INTO tasks (project_id, title, description, acceptance_criteria, notes, source, external_id, external_url, external_revision)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, input.ProjectID, input.Title, input.Description, input.AcceptanceCriteria, input.Notes, source, externalID, externalURL, externalRevision)
	if err != nil {
		return Task{}, fmt.Errorf("create task: %w", err)
	}

	id, err := res.LastInsertId()
	if err != nil {
		return Task{}, fmt.Errorf("read created task id: %w", err)
	}

	actor := sanitizeActorRef(input.Actor)
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO task_events (task_id, type, body, to_status, actor_id, actor_kind, actor_name)
		VALUES (?, 'created', ?, 'open', ?, ?, ?)
	`, id, input.Title, actor.ID, actor.Kind, actor.Name); err != nil {
		return Task{}, fmt.Errorf("record task create event: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return Task{}, fmt.Errorf("commit create task transaction: %w", err)
	}

	return s.GetTask(ctx, id)
}

func (s *Store) GetTask(ctx context.Context, id int64) (Task, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, project_id, status, title, description, acceptance_criteria, notes, source, external_id, external_url, external_revision, created_at, updated_at
		FROM tasks
		WHERE id = ?
	`, id)
	return scanTask(row)
}

func (s *Store) ListTasks(ctx context.Context, projectID int64) ([]Task, error) {
	return s.ListTasksWithOptions(ctx, projectID, ListTasksOptions{})
}

func (s *Store) ListAllTasksWithOptions(ctx context.Context, opts ListTasksOptions) ([]Task, error) {
	return s.listTasksWithOptions(ctx, opts.ProjectID, opts, "DESC")
}

func (s *Store) ListTasksWithOptions(ctx context.Context, projectID int64, opts ListTasksOptions) ([]Task, error) {
	if projectID <= 0 {
		return nil, errors.New("task project id is required")
	}
	return s.listTasksWithOptions(ctx, projectID, opts, "ASC")
}

func (s *Store) listTasksWithOptions(ctx context.Context, projectID int64, opts ListTasksOptions, direction string) ([]Task, error) {
	statuses, err := normalizeTaskStatuses(opts)
	if err != nil {
		return nil, err
	}

	query := `
		SELECT id, project_id, status, title, description, acceptance_criteria, notes, source, external_id, external_url, external_revision, created_at, updated_at
		FROM tasks
	`
	args := []any{}
	where, baseArgs := taskWhereClauses(projectID, statuses)
	args = append(args, baseArgs...)
	if direction != "DESC" {
		direction = "ASC"
	}
	if opts.Cursor > 0 {
		op := ">"
		if direction == "DESC" {
			op = "<"
		}
		where = append(where, "id "+op+" ?")
		args = append(args, opts.Cursor)
	}
	if len(where) > 0 {
		query += " WHERE " + strings.Join(where, " AND ")
	}
	query += " ORDER BY id " + direction
	if opts.Limit > 0 {
		query += " LIMIT ?"
		args = append(args, opts.Limit)
	}

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list tasks: %w", err)
	}
	defer rows.Close()

	return scanTaskRows(rows, "scan task", "iterate tasks")
}

func (s *Store) CountTasks(ctx context.Context) (int, error) {
	return s.CountTasksWithOptions(ctx, 0, ListTasksOptions{})
}

func (s *Store) CountTasksWithOptions(ctx context.Context, projectID int64, opts ListTasksOptions) (int, error) {
	statuses, err := normalizeTaskStatuses(opts)
	if err != nil {
		return 0, err
	}

	query := "SELECT COUNT(*) FROM tasks"
	where, args := taskWhereClauses(projectID, statuses)
	if len(where) > 0 {
		query += " WHERE " + strings.Join(where, " AND ")
	}

	var count int
	if err := s.db.QueryRowContext(ctx, query, args...).Scan(&count); err != nil {
		return 0, fmt.Errorf("count tasks: %w", err)
	}

	return count, nil
}

func taskWhereClauses(projectID int64, statuses []string) ([]string, []any) {
	where := []string{}
	args := []any{}

	if projectID > 0 {
		where = append(where, "project_id = ?")
		args = append(args, projectID)
	}
	switch len(statuses) {
	case 0:
	case 1:
		where = append(where, "status = ?")
		args = append(args, statuses[0])
	default:
		where = append(where, "status IN ("+queryPlaceholders(len(statuses))+")")
		for _, status := range statuses {
			args = append(args, status)
		}
	}

	return where, args
}

func (s *Store) UpdateTaskStatus(ctx context.Context, id int64, status string) (Task, error) {
	return s.UpdateTaskStatusByActor(ctx, id, status, ActorRef{})
}

func (s *Store) UpdateTaskStatusByActor(ctx context.Context, id int64, status string, actor ActorRef) (Task, error) {
	return s.updateTaskStatusByActor(ctx, id, status, actor)
}

func (s *Store) updateTaskStatusByActor(ctx context.Context, id int64, status string, actor ActorRef) (Task, error) {
	if !validTaskStatus(status) {
		return Task{}, fmt.Errorf("invalid task status %q", status)
	}
	if status == "done" {
		return Task{}, ErrTaskStatusDoneUnsupported
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Task{}, fmt.Errorf("begin update task status transaction: %w", err)
	}
	defer rollback(tx)

	current, err := getTaskInTx(ctx, tx, id)
	if err != nil {
		return Task{}, err
	}
	if current.Status == status {
		return current, nil
	}
	if current.Status == "done" {
		return Task{}, ErrInvalidTaskTransition
	}

	res, err := tx.ExecContext(ctx, `
		UPDATE tasks
		SET status = ?, updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
		WHERE id = ?
	`, status, id)
	if err != nil {
		return Task{}, fmt.Errorf("update task status: %w", err)
	}
	updated, err := res.RowsAffected()
	if err != nil {
		return Task{}, fmt.Errorf("read task status count: %w", err)
	}
	if updated == 0 {
		return Task{}, ErrInvalidTaskTransition
	}

	actor = sanitizeActorRef(actor)
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO task_events (task_id, type, from_status, to_status, actor_id, actor_kind, actor_name)
		VALUES (?, 'status_changed', ?, ?, ?, ?, ?)
	`, id, current.Status, status, actor.ID, actor.Kind, actor.Name); err != nil {
		return Task{}, fmt.Errorf("record task status event: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return Task{}, fmt.Errorf("commit update task status transaction: %w", err)
	}

	return s.GetTask(ctx, id)
}

func (s *Store) CompleteTaskWithOptions(ctx context.Context, input CompleteTaskInput) (Task, error) {
	input.Note = strings.TrimSpace(input.Note)
	input.OverrideReason = strings.TrimSpace(input.OverrideReason)
	if input.ID <= 0 {
		return Task{}, errors.New("task id is required")
	}
	if input.Note == "" {
		return Task{}, ErrTaskCompletionNoteEmpty
	}
	switch input.Mode {
	case CompletionValidated:
	case CompletionOverride:
		if input.OverrideReason == "" {
			return Task{}, ErrTaskCompletionOverrideRequired
		}
	default:
		return Task{}, ErrInvalidTaskCompletionMode
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Task{}, fmt.Errorf("begin complete task transaction: %w", err)
	}
	defer rollback(tx)

	current, err := getTaskInTx(ctx, tx, input.ID)
	if err != nil {
		return Task{}, err
	}
	if current.Status != "in_progress" {
		return Task{}, ErrInvalidTaskTransition
	}
	activeRun, err := s.hasActiveRunForTaskInTx(ctx, tx, input.ID)
	if err != nil {
		return Task{}, err
	}
	if activeRun {
		return Task{}, ErrActiveRunExists
	}
	evidenceRunID := int64(0)
	evidenceArtifactID := int64(0)
	if input.Mode == CompletionValidated {
		evidenceRunID, evidenceArtifactID, err = s.validateTaskCompletionEvidenceInTx(ctx, tx, input.ID, input.EvidenceRunID)
		if err != nil {
			return Task{}, err
		}
		if evidenceRunID == 0 {
			return Task{}, ErrTaskCompletionEvidenceRequired
		}
	}

	res, err := tx.ExecContext(ctx, `
		UPDATE tasks
		SET status = 'done', updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
		WHERE id = ? AND status = 'in_progress'
	`, input.ID)
	if err != nil {
		return Task{}, fmt.Errorf("complete task: %w", err)
	}
	completed, err := res.RowsAffected()
	if err != nil {
		return Task{}, fmt.Errorf("read completed task count: %w", err)
	}
	if completed == 0 {
		return Task{}, ErrInvalidTaskTransition
	}

	actor := sanitizeActorRef(input.Actor)
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO task_events (task_id, type, body, from_status, to_status, actor_id, actor_kind, actor_name, evidence_run_id, evidence_artifact_id)
		VALUES (?, 'completed', ?, 'in_progress', 'done', ?, ?, ?, ?, ?)
	`, input.ID, input.Note, actor.ID, actor.Kind, actor.Name, evidenceRunID, evidenceArtifactID); err != nil {
		return Task{}, fmt.Errorf("record task completion event: %w", err)
	}
	if input.OverrideReason != "" {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO task_events (task_id, type, body, from_status, to_status, actor_id, actor_kind, actor_name)
			VALUES (?, 'completion_override', ?, 'in_progress', 'done', ?, ?, ?)
		`, input.ID, input.OverrideReason, actor.ID, actor.Kind, actor.Name); err != nil {
			return Task{}, fmt.Errorf("record task completion override event: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return Task{}, fmt.Errorf("commit complete task transaction: %w", err)
	}

	return s.GetTask(ctx, input.ID)
}

func (s *Store) UpdateTaskExternalReference(ctx context.Context, input UpdateTaskExternalReferenceInput) (Task, error) {
	if input.ID <= 0 {
		return Task{}, errors.New("task id is required")
	}
	source, externalID, externalURL, externalRevision, err := normalizeTaskExternalReference(input.Source, input.ExternalID, input.ExternalURL, input.ExternalRevision)
	if err != nil {
		return Task{}, err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Task{}, fmt.Errorf("begin update task source transaction: %w", err)
	}
	defer rollback(tx)

	current, err := getTaskInTx(ctx, tx, input.ID)
	if err != nil {
		return Task{}, err
	}
	if current.Source == source && current.ExternalID == externalID && current.ExternalURL == externalURL && current.ExternalRevision == externalRevision {
		return current, nil
	}

	res, err := tx.ExecContext(ctx, `
		UPDATE tasks
		SET source = ?, external_id = ?, external_url = ?, external_revision = ?,
		    updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
		WHERE id = ?
	`, source, externalID, externalURL, externalRevision, input.ID)
	if err != nil {
		return Task{}, fmt.Errorf("update task external reference: %w", err)
	}
	updated, err := res.RowsAffected()
	if err != nil {
		return Task{}, fmt.Errorf("read task external reference count: %w", err)
	}
	if updated == 0 {
		return Task{}, sql.ErrNoRows
	}

	actor := sanitizeActorRef(input.Actor)
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO task_events (task_id, type, body, actor_id, actor_kind, actor_name)
		VALUES (?, 'source_updated', ?, ?, ?, ?)
	`, input.ID, taskExternalReferenceEventBody(source, externalID, externalURL, externalRevision), actor.ID, actor.Kind, actor.Name); err != nil {
		return Task{}, fmt.Errorf("record task external reference event: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return Task{}, fmt.Errorf("commit update task source transaction: %w", err)
	}

	return s.GetTask(ctx, input.ID)
}

func (s *Store) ListReadyTasks(ctx context.Context, projectID int64) ([]Task, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT t.id, t.project_id, t.status, t.title, t.description, t.acceptance_criteria, t.notes, t.source, t.external_id, t.external_url, t.external_revision, t.created_at, t.updated_at
		FROM tasks t
		WHERE t.project_id = ?
		  AND t.status = 'open'
		  AND NOT EXISTS (
			SELECT 1
			FROM task_dependencies d
			JOIN tasks blocker ON blocker.id = d.blocker_task_id
			WHERE d.blocked_task_id = t.id
			  AND d.edge_type = 'blocks'
			  AND blocker.status <> 'done'
		  )
		ORDER BY t.id
	`, projectID)
	if err != nil {
		return nil, fmt.Errorf("list ready tasks: %w", err)
	}
	defer rows.Close()

	return scanTaskRows(rows, "scan ready task", "iterate ready tasks")
}

func scanTaskRows(rows *sql.Rows, scanLabel, iterateLabel string) ([]Task, error) {
	var tasks []Task
	for rows.Next() {
		task, err := scanTask(rows)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", scanLabel, err)
		}
		tasks = append(tasks, task)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("%s: %w", iterateLabel, err)
	}

	return tasks, nil
}

func (s *Store) ClaimNextReadyTask(ctx context.Context, projectID int64) (Task, error) {
	return s.ClaimNextReadyTaskByActor(ctx, projectID, ActorRef{})
}

func (s *Store) ClaimNextReadyTaskByActor(ctx context.Context, projectID int64, actor ActorRef) (Task, error) {
	if projectID <= 0 {
		return Task{}, errors.New("claim project id is required")
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Task{}, fmt.Errorf("begin claim next task transaction: %w", err)
	}
	defer rollback(tx)

	var taskID int64
	err = tx.QueryRowContext(ctx, `
		SELECT t.id
		FROM tasks t
		WHERE t.project_id = ?
		  AND t.status = 'open'
		  AND NOT EXISTS (
			SELECT 1
			FROM task_dependencies d
			JOIN tasks blocker ON blocker.id = d.blocker_task_id
			WHERE d.blocked_task_id = t.id
			  AND d.edge_type = 'blocks'
			  AND blocker.status <> 'done'
		  )
		ORDER BY t.id
		LIMIT 1
	`, projectID).Scan(&taskID)
	if errors.Is(err, sql.ErrNoRows) {
		return Task{}, ErrNoReadyTask
	}
	if err != nil {
		return Task{}, fmt.Errorf("select next ready task: %w", err)
	}

	task, err := claimTaskInTx(ctx, tx, projectID, taskID, actor)
	if err != nil {
		return Task{}, err
	}
	if err := tx.Commit(); err != nil {
		return Task{}, fmt.Errorf("commit claim next task transaction: %w", err)
	}

	return task, nil
}

func (s *Store) ClaimTask(ctx context.Context, projectID, taskID int64) (Task, error) {
	return s.ClaimTaskByActor(ctx, projectID, taskID, ActorRef{})
}

func (s *Store) ClaimTaskByActor(ctx context.Context, projectID, taskID int64, actor ActorRef) (Task, error) {
	if projectID <= 0 {
		return Task{}, errors.New("claim project id is required")
	}
	if taskID <= 0 {
		return Task{}, errors.New("claim task id is required")
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Task{}, fmt.Errorf("begin claim task transaction: %w", err)
	}
	defer rollback(tx)

	task, err := claimTaskInTx(ctx, tx, projectID, taskID, actor)
	if err != nil {
		return Task{}, err
	}
	if err := tx.Commit(); err != nil {
		return Task{}, fmt.Errorf("commit claim task transaction: %w", err)
	}

	return task, nil
}

func claimTaskInTx(ctx context.Context, tx *sql.Tx, projectID, taskID int64, actor ActorRef) (Task, error) {
	current, err := getTaskInTx(ctx, tx, taskID)
	if err != nil {
		return Task{}, err
	}
	if current.ProjectID != projectID {
		return Task{}, sql.ErrNoRows
	}
	if current.Status != "open" {
		return Task{}, ErrTaskNotReady
	}

	var activeBlockers int
	if err := tx.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM task_dependencies d
		JOIN tasks blocker ON blocker.id = d.blocker_task_id
		WHERE d.blocked_task_id = ?
		  AND d.edge_type = 'blocks'
		  AND blocker.status <> 'done'
	`, taskID).Scan(&activeBlockers); err != nil {
		return Task{}, fmt.Errorf("count active task blockers: %w", err)
	}
	if activeBlockers > 0 {
		return Task{}, ErrTaskNotReady
	}

	res, err := tx.ExecContext(ctx, `
		UPDATE tasks
		SET status = 'in_progress', updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
		WHERE id = ?
		  AND project_id = ?
		  AND status = 'open'
		  AND NOT EXISTS (
			SELECT 1
			FROM task_dependencies d
			JOIN tasks blocker ON blocker.id = d.blocker_task_id
			WHERE d.blocked_task_id = tasks.id
			  AND d.edge_type = 'blocks'
			  AND blocker.status <> 'done'
		  )
	`, taskID, projectID)
	if err != nil {
		return Task{}, fmt.Errorf("claim task: %w", err)
	}
	claimed, err := res.RowsAffected()
	if err != nil {
		return Task{}, fmt.Errorf("read claimed task count: %w", err)
	}
	if claimed == 0 {
		return Task{}, ErrTaskNotReady
	}

	actor = sanitizeActorRef(actor)
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO task_events (task_id, type, from_status, to_status, actor_id, actor_kind, actor_name)
		VALUES (?, 'claimed', 'open', 'in_progress', ?, ?, ?)
	`, taskID, actor.ID, actor.Kind, actor.Name); err != nil {
		return Task{}, fmt.Errorf("record task claim event: %w", err)
	}

	return getTaskInTx(ctx, tx, taskID)
}

func getTaskInTx(ctx context.Context, tx *sql.Tx, id int64) (Task, error) {
	row := tx.QueryRowContext(ctx, `
		SELECT id, project_id, status, title, description, acceptance_criteria, notes, source, external_id, external_url, external_revision, created_at, updated_at
		FROM tasks
		WHERE id = ?
	`, id)
	return scanTask(row)
}

func normalizeTaskExternalReference(source, externalID, externalURL, externalRevision string) (string, string, string, string, error) {
	source = strings.TrimSpace(source)
	externalID = strings.TrimSpace(externalID)
	externalURL = strings.TrimSpace(externalURL)
	externalRevision = strings.TrimSpace(externalRevision)
	if source == "" {
		source = "local"
	}
	if !validTaskSource(source) {
		return "", "", "", "", fmt.Errorf("%w: %q", ErrInvalidTaskSource, source)
	}
	if source == "local" {
		if externalID != "" || externalURL != "" || externalRevision != "" {
			return "", "", "", "", fmt.Errorf("%w: local source cannot include external reference fields", ErrInvalidTaskExternalReference)
		}
		return source, "", "", "", nil
	}
	if externalID == "" {
		return "", "", "", "", fmt.Errorf("%w: external source requires external id", ErrInvalidTaskExternalReference)
	}
	if externalURL == "" {
		return "", "", "", "", fmt.Errorf("%w: external source requires external url", ErrInvalidTaskExternalReference)
	}
	if !validTaskExternalURL(externalURL) {
		return "", "", "", "", fmt.Errorf("%w: external url must use http or https", ErrInvalidTaskExternalReference)
	}
	return source, externalID, externalURL, externalRevision, nil
}

func validTaskExternalURL(externalURL string) bool {
	parsed, err := url.ParseRequestURI(externalURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return false
	}
	switch strings.ToLower(parsed.Scheme) {
	case "http", "https":
		return true
	default:
		return false
	}
}

func validTaskSource(source string) bool {
	switch source {
	case "local", "github", "linear", "jira":
		return true
	default:
		return false
	}
}

func taskExternalReferenceEventBody(source, externalID, externalURL, externalRevision string) string {
	if source == "local" {
		return "source=local"
	}
	parts := []string{
		"source=" + source,
		"external_id=" + externalID,
		"external_url=" + externalURL,
	}
	if externalRevision != "" {
		parts = append(parts, "external_revision="+externalRevision)
	}
	return strings.Join(parts, " ")
}
