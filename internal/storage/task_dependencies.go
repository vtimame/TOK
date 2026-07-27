package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

type TaskDependency struct {
	ID            int64
	EdgeType      string
	BlockerTaskID int64
	BlockedTaskID int64
	CreatedAt     string
}

func (s *Store) AddTaskDependency(ctx context.Context, edgeType string, blockerTaskID, blockedTaskID int64) (TaskDependency, error) {
	if err := validateTaskDependency(edgeType, blockerTaskID, blockedTaskID); err != nil {
		return TaskDependency{}, err
	}

	blocker, err := s.GetTask(ctx, blockerTaskID)
	if err != nil {
		return TaskDependency{}, fmt.Errorf("get blocker task: %w", err)
	}
	blocked, err := s.GetTask(ctx, blockedTaskID)
	if err != nil {
		return TaskDependency{}, fmt.Errorf("get blocked task: %w", err)
	}
	if blocker.ProjectID != blocked.ProjectID {
		return TaskDependency{}, errors.New("task dependency must stay within one project")
	}

	res, err := s.db.ExecContext(ctx, `
		INSERT INTO task_dependencies (edge_type, blocker_task_id, blocked_task_id)
		VALUES (?, ?, ?)
	`, edgeType, blockerTaskID, blockedTaskID)
	if err != nil {
		return TaskDependency{}, fmt.Errorf("add task dependency: %w", err)
	}

	id, err := res.LastInsertId()
	if err != nil {
		return TaskDependency{}, fmt.Errorf("read task dependency id: %w", err)
	}

	return s.GetTaskDependency(ctx, id)
}

func (s *Store) RemoveTaskDependency(ctx context.Context, edgeType string, blockerTaskID, blockedTaskID int64) error {
	if err := validateTaskDependency(edgeType, blockerTaskID, blockedTaskID); err != nil {
		return err
	}

	res, err := s.db.ExecContext(ctx, `
		DELETE FROM task_dependencies
		WHERE edge_type = ? AND blocker_task_id = ? AND blocked_task_id = ?
	`, edgeType, blockerTaskID, blockedTaskID)
	if err != nil {
		return fmt.Errorf("remove task dependency: %w", err)
	}

	removed, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("read removed task dependency count: %w", err)
	}
	if removed == 0 {
		return sql.ErrNoRows
	}

	return nil
}

func (s *Store) GetTaskDependency(ctx context.Context, id int64) (TaskDependency, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, edge_type, blocker_task_id, blocked_task_id, created_at
		FROM task_dependencies
		WHERE id = ?
	`, id)
	return scanTaskDependency(row)
}

func (s *Store) ListTaskDependencies(ctx context.Context, projectID, taskID int64) ([]TaskDependency, error) {
	if projectID <= 0 {
		return nil, errors.New("dependency project id is required")
	}
	if taskID <= 0 {
		return nil, errors.New("dependency task id is required")
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT d.id, d.edge_type, d.blocker_task_id, d.blocked_task_id, d.created_at
		FROM task_dependencies d
		JOIN tasks blocker ON blocker.id = d.blocker_task_id
		JOIN tasks blocked ON blocked.id = d.blocked_task_id
		WHERE (d.blocker_task_id = ? OR d.blocked_task_id = ?)
		  AND blocker.project_id = ?
		  AND blocked.project_id = ?
		ORDER BY d.id
	`, taskID, taskID, projectID, projectID)
	if err != nil {
		return nil, fmt.Errorf("list task dependencies: %w", err)
	}
	defer rows.Close()

	var dependencies []TaskDependency
	for rows.Next() {
		dependency, err := scanTaskDependency(rows)
		if err != nil {
			return nil, fmt.Errorf("scan task dependency: %w", err)
		}
		dependencies = append(dependencies, dependency)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate task dependencies: %w", err)
	}

	return dependencies, nil
}

func validateTaskDependency(edgeType string, blockerTaskID, blockedTaskID int64) error {
	if edgeType != "blocks" {
		return fmt.Errorf("invalid task dependency edge type %q", edgeType)
	}
	if blockerTaskID <= 0 {
		return errors.New("blocker task id is required")
	}
	if blockedTaskID <= 0 {
		return errors.New("blocked task id is required")
	}
	if blockerTaskID == blockedTaskID {
		return errors.New("task cannot block itself")
	}
	return nil
}
