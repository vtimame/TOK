package storage

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

func (s *Store) AddTaskProgress(ctx context.Context, taskID int64, body string) (TaskEvent, error) {
	return s.AddTaskProgressByActor(ctx, taskID, body, ActorRef{})
}

func (s *Store) AddTaskProgressByActor(ctx context.Context, taskID int64, body string, actor ActorRef) (TaskEvent, error) {
	return s.addTaskNoteEvent(ctx, taskID, "progress", body, actor)
}

func (s *Store) AddTaskComment(ctx context.Context, taskID int64, body string) (TaskEvent, error) {
	return s.AddTaskCommentByActor(ctx, taskID, body, ActorRef{})
}

func (s *Store) AddTaskCommentByActor(ctx context.Context, taskID int64, body string, actor ActorRef) (TaskEvent, error) {
	return s.addTaskNoteEvent(ctx, taskID, "commented", body, actor)
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
		SELECT id, task_id, type, body, from_status, to_status, actor_id, actor_kind, actor_name, evidence_run_id, evidence_artifact_id, created_at
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
		if err := rows.Scan(
			&event.ID,
			&event.TaskID,
			&event.Type,
			&event.Body,
			&event.FromStatus,
			&event.ToStatus,
			&event.ActorID,
			&event.ActorKind,
			&event.ActorName,
			&event.EvidenceRunID,
			&event.EvidenceArtifactID,
			&event.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan task event: %w", err)
		}
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate task events: %w", err)
	}

	return events, nil
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

func (s *Store) GetTaskEvent(ctx context.Context, id int64) (TaskEvent, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, task_id, type, body, from_status, to_status, actor_id, actor_kind, actor_name, evidence_run_id, evidence_artifact_id, created_at
		FROM task_events
		WHERE id = ?
	`, id)

	var event TaskEvent
	if err := row.Scan(
		&event.ID,
		&event.TaskID,
		&event.Type,
		&event.Body,
		&event.FromStatus,
		&event.ToStatus,
		&event.ActorID,
		&event.ActorKind,
		&event.ActorName,
		&event.EvidenceRunID,
		&event.EvidenceArtifactID,
		&event.CreatedAt,
	); err != nil {
		return TaskEvent{}, err
	}
	return event, nil
}
