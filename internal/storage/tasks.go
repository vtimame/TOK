package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
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
	CreatedAt          string
	UpdatedAt          string
}

type CreateTaskInput struct {
	ProjectID          int64
	Title              string
	Description        string
	AcceptanceCriteria string
	Notes              string
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
	ID         int64
	TaskID     int64
	Type       string
	Body       string
	FromStatus string
	ToStatus   string
	ActorID    int64
	ActorKind  string
	ActorName  string
	CreatedAt  string
}

type CompleteTaskInput struct {
	ID             int64
	Note           string
	OverrideReason string
	Actor          ActorRef
}

func (s *Store) CreateTask(ctx context.Context, input CreateTaskInput) (Task, error) {
	if strings.TrimSpace(input.Title) == "" {
		return Task{}, errors.New("task title is required")
	}
	if input.ProjectID <= 0 {
		return Task{}, errors.New("task project id is required")
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Task{}, fmt.Errorf("begin create task transaction: %w", err)
	}
	defer rollback(tx)

	res, err := tx.ExecContext(ctx, `
		INSERT INTO tasks (project_id, title, description, acceptance_criteria, notes)
		VALUES (?, ?, ?, ?, ?)
	`, input.ProjectID, input.Title, input.Description, input.AcceptanceCriteria, input.Notes)
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
		SELECT id, project_id, status, title, description, acceptance_criteria, notes, created_at, updated_at
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
		SELECT id, project_id, status, title, description, acceptance_criteria, notes, created_at, updated_at
		FROM tasks
	`
	args := []any{}
	where := []string{}
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

	var tasks []Task
	for rows.Next() {
		var task Task
		if err := rows.Scan(&task.ID, &task.ProjectID, &task.Status, &task.Title, &task.Description, &task.AcceptanceCriteria, &task.Notes, &task.CreatedAt, &task.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan task: %w", err)
		}
		tasks = append(tasks, task)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate tasks: %w", err)
	}

	return tasks, nil
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
	args := []any{}
	where := []string{}
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
	if len(where) > 0 {
		query += " WHERE " + strings.Join(where, " AND ")
	}

	var count int
	if err := s.db.QueryRowContext(ctx, query, args...).Scan(&count); err != nil {
		return 0, fmt.Errorf("count tasks: %w", err)
	}
	return count, nil
}

func (s *Store) UpdateTaskStatus(ctx context.Context, id int64, status string) (Task, error) {
	return s.UpdateTaskStatusByActor(ctx, id, status, ActorRef{})
}

func (s *Store) UpdateTaskStatusByActor(ctx context.Context, id int64, status string, actor ActorRef) (Task, error) {
	if !validTaskStatus(status) {
		return Task{}, fmt.Errorf("invalid task status %q", status)
	}

	current, err := s.GetTask(ctx, id)
	if err != nil {
		return Task{}, err
	}
	if current.Status == status {
		return current, nil
	}
	if status == "done" {
		activeRun, err := s.hasActiveRunForTask(ctx, id)
		if err != nil {
			return Task{}, err
		}
		if activeRun {
			return Task{}, ErrActiveRunExists
		}
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Task{}, fmt.Errorf("begin update task status transaction: %w", err)
	}
	defer rollback(tx)

	if _, err := tx.ExecContext(ctx, `
		UPDATE tasks
		SET status = ?, updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
		WHERE id = ?
	`, status, id); err != nil {
		return Task{}, fmt.Errorf("update task status: %w", err)
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

func (s *Store) CompleteTask(ctx context.Context, id int64, note string) (Task, error) {
	return s.CompleteTaskByActor(ctx, id, note, ActorRef{})
}

func (s *Store) CompleteTaskByActor(ctx context.Context, id int64, note string, actor ActorRef) (Task, error) {
	return s.CompleteTaskWithOptions(ctx, CompleteTaskInput{
		ID:    id,
		Note:  note,
		Actor: actor,
	})
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

	current, err := s.GetTask(ctx, input.ID)
	if err != nil {
		return Task{}, err
	}
	if current.Status != "in_progress" {
		return Task{}, ErrInvalidTaskTransition
	}
	activeRun, err := s.hasActiveRunForTask(ctx, input.ID)
	if err != nil {
		return Task{}, err
	}
	if activeRun {
		return Task{}, ErrActiveRunExists
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Task{}, fmt.Errorf("begin complete task transaction: %w", err)
	}
	defer rollback(tx)

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
		INSERT INTO task_events (task_id, type, body, from_status, to_status, actor_id, actor_kind, actor_name)
		VALUES (?, 'completed', ?, 'in_progress', 'done', ?, ?, ?)
	`, input.ID, input.Note, actor.ID, actor.Kind, actor.Name); err != nil {
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

func (s *Store) AddTaskProgress(ctx context.Context, taskID int64, body string) (TaskEvent, error) {
	return s.AddTaskProgressByActor(ctx, taskID, body, ActorRef{})
}

func (s *Store) AddTaskProgressByActor(ctx context.Context, taskID int64, body string, actor ActorRef) (TaskEvent, error) {
	return s.addTaskNoteEvent(ctx, taskID, "progress", body, actor)
}

func (s *Store) BlockTask(ctx context.Context, id int64, reason string) (Task, error) {
	return s.BlockTaskByActor(ctx, id, reason, ActorRef{})
}

func (s *Store) BlockTaskByActor(ctx context.Context, id int64, reason string, actor ActorRef) (Task, error) {
	return s.transitionTaskWithNote(ctx, id, "blocked", "blocked", reason, actor)
}

func (s *Store) UnblockTask(ctx context.Context, id int64, note string) (Task, error) {
	return s.UnblockTaskByActor(ctx, id, note, ActorRef{})
}

func (s *Store) UnblockTaskByActor(ctx context.Context, id int64, note string, actor ActorRef) (Task, error) {
	note = strings.TrimSpace(note)
	if id <= 0 {
		return Task{}, errors.New("task id is required")
	}
	if note == "" {
		return Task{}, ErrTaskNoteEmpty
	}

	current, err := s.GetTask(ctx, id)
	if err != nil {
		return Task{}, err
	}
	if current.Status != "blocked" {
		return Task{}, ErrInvalidTaskTransition
	}

	return s.transitionTaskWithNote(ctx, id, "open", "unblocked", note, actor)
}

func (s *Store) ListTaskEvents(ctx context.Context, taskID int64) ([]TaskEvent, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, task_id, type, body, from_status, to_status, actor_id, actor_kind, actor_name, created_at
		FROM task_events
		WHERE task_id = ?
		ORDER BY id
	`, taskID)
	if err != nil {
		return nil, fmt.Errorf("list task events: %w", err)
	}
	defer rows.Close()

	var events []TaskEvent
	for rows.Next() {
		var event TaskEvent
		if err := rows.Scan(&event.ID, &event.TaskID, &event.Type, &event.Body, &event.FromStatus, &event.ToStatus, &event.ActorID, &event.ActorKind, &event.ActorName, &event.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan task event: %w", err)
		}
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate task events: %w", err)
	}

	return events, nil
}

func (s *Store) ListReadyTasks(ctx context.Context, projectID int64) ([]Task, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT t.id, t.project_id, t.status, t.title, t.description, t.acceptance_criteria, t.notes, t.created_at, t.updated_at
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

	var tasks []Task
	for rows.Next() {
		var task Task
		if err := rows.Scan(&task.ID, &task.ProjectID, &task.Status, &task.Title, &task.Description, &task.AcceptanceCriteria, &task.Notes, &task.CreatedAt, &task.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan ready task: %w", err)
		}
		tasks = append(tasks, task)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate ready tasks: %w", err)
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

func (s *Store) AddTaskComment(ctx context.Context, taskID int64, body string) (TaskEvent, error) {
	return s.AddTaskCommentByActor(ctx, taskID, body, ActorRef{})
}

func (s *Store) AddTaskCommentByActor(ctx context.Context, taskID int64, body string, actor ActorRef) (TaskEvent, error) {
	return s.addTaskNoteEvent(ctx, taskID, "commented", body, actor)
}

func (s *Store) addTaskNoteEvent(ctx context.Context, taskID int64, eventType, body string, actor ActorRef) (TaskEvent, error) {
	if taskID <= 0 {
		return TaskEvent{}, errors.New("task id is required")
	}
	body = strings.TrimSpace(body)
	if body == "" {
		return TaskEvent{}, ErrTaskNoteEmpty
	}

	if _, err := s.GetTask(ctx, taskID); err != nil {
		return TaskEvent{}, err
	}

	actor = sanitizeActorRef(actor)
	res, err := s.db.ExecContext(ctx, `
		INSERT INTO task_events (task_id, type, body, actor_id, actor_kind, actor_name)
		VALUES (?, ?, ?, ?, ?, ?)
	`, taskID, eventType, body, actor.ID, actor.Kind, actor.Name)
	if err != nil {
		return TaskEvent{}, fmt.Errorf("record task %s event: %w", eventType, err)
	}

	id, err := res.LastInsertId()
	if err != nil {
		return TaskEvent{}, fmt.Errorf("read task %s event id: %w", eventType, err)
	}

	return s.GetTaskEvent(ctx, id)
}

func (s *Store) transitionTaskWithNote(ctx context.Context, id int64, status, eventType, body string, actor ActorRef) (Task, error) {
	body = strings.TrimSpace(body)
	if id <= 0 {
		return Task{}, errors.New("task id is required")
	}
	if body == "" {
		return Task{}, ErrTaskNoteEmpty
	}

	current, err := s.GetTask(ctx, id)
	if err != nil {
		return Task{}, err
	}
	if current.Status == "done" {
		return Task{}, ErrInvalidTaskTransition
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Task{}, fmt.Errorf("begin task %s transaction: %w", eventType, err)
	}
	defer rollback(tx)

	if _, err := tx.ExecContext(ctx, `
		UPDATE tasks
		SET status = ?, updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
		WHERE id = ?
	`, status, id); err != nil {
		return Task{}, fmt.Errorf("update task %s status: %w", eventType, err)
	}

	actor = sanitizeActorRef(actor)
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO task_events (task_id, type, body, from_status, to_status, actor_id, actor_kind, actor_name)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, id, eventType, body, current.Status, status, actor.ID, actor.Kind, actor.Name); err != nil {
		return Task{}, fmt.Errorf("record task %s event: %w", eventType, err)
	}

	if err := tx.Commit(); err != nil {
		return Task{}, fmt.Errorf("commit task %s transaction: %w", eventType, err)
	}

	return s.GetTask(ctx, id)
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
		SELECT id, project_id, status, title, description, acceptance_criteria, notes, created_at, updated_at
		FROM tasks
		WHERE id = ?
	`, id)
	return scanTask(row)
}

func (s *Store) GetTaskEvent(ctx context.Context, id int64) (TaskEvent, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, task_id, type, body, from_status, to_status, actor_id, actor_kind, actor_name, created_at
		FROM task_events
		WHERE id = ?
	`, id)

	var event TaskEvent
	if err := row.Scan(&event.ID, &event.TaskID, &event.Type, &event.Body, &event.FromStatus, &event.ToStatus, &event.ActorID, &event.ActorKind, &event.ActorName, &event.CreatedAt); err != nil {
		return TaskEvent{}, err
	}
	return event, nil
}
