package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

type ProjectInstruction struct {
	ID        int64
	ProjectID int64
	Scope     string
	Title     string
	Body      string
	Priority  string
	Enabled   bool
	Source    string
	CreatedAt string
	UpdatedAt string
}

type CreateProjectInstructionInput struct {
	ProjectID int64
	Title     string
	Body      string
	Priority  string
	Source    string
}

type ListProjectInstructionsOptions struct {
	ProjectID       int64
	IncludeDisabled bool
}

func validateProjectInstructionInput(input CreateProjectInstructionInput) error {
	if input.ProjectID <= 0 {
		return errors.New("project instruction project id is required")
	}
	if strings.TrimSpace(input.Title) == "" {
		return errors.New("project instruction title is required")
	}
	if strings.TrimSpace(input.Body) == "" {
		return errors.New("project instruction body is required")
	}
	if !validInstructionPriority(normalizeInstructionPriority(input.Priority)) {
		return fmt.Errorf("invalid project instruction priority: %s", input.Priority)
	}
	return nil
}

func normalizeInstructionPriority(priority string) string {
	priority = strings.TrimSpace(strings.ToLower(priority))
	if priority == "" {
		return "normal"
	}
	return priority
}

func validInstructionPriority(priority string) bool {
	switch priority {
	case "low", "normal", "high", "critical":
		return true
	default:
		return false
	}
}

func (s *Store) CreateProjectInstruction(ctx context.Context, input CreateProjectInstructionInput) (ProjectInstruction, error) {
	if err := validateProjectInstructionInput(input); err != nil {
		return ProjectInstruction{}, err
	}
	priority := normalizeInstructionPriority(input.Priority)
	source := strings.TrimSpace(input.Source)
	if source == "" {
		source = "manual"
	}

	res, err := s.db.ExecContext(ctx, `
		INSERT INTO project_instructions (project_id, scope, title, body, priority, enabled, source)
		VALUES (?, 'project', ?, ?, ?, 1, ?)
	`, input.ProjectID, strings.TrimSpace(input.Title), strings.TrimSpace(input.Body), priority, source)
	if err != nil {
		return ProjectInstruction{}, fmt.Errorf("create project instruction: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return ProjectInstruction{}, fmt.Errorf("read created project instruction id: %w", err)
	}
	return s.GetProjectInstruction(ctx, input.ProjectID, id)
}

func (s *Store) GetProjectInstruction(ctx context.Context, projectID, id int64) (ProjectInstruction, error) {
	if projectID <= 0 {
		return ProjectInstruction{}, errors.New("project instruction project id is required")
	}
	if id <= 0 {
		return ProjectInstruction{}, errors.New("project instruction id is required")
	}
	row := s.db.QueryRowContext(ctx, `
		SELECT id, project_id, scope, title, body, priority, enabled, source, created_at, updated_at
		FROM project_instructions
		WHERE project_id = ? AND id = ?
	`, projectID, id)
	return scanProjectInstruction(row)
}

func (s *Store) ListProjectInstructions(ctx context.Context, opts ListProjectInstructionsOptions) ([]ProjectInstruction, error) {
	if opts.ProjectID <= 0 {
		return nil, errors.New("project instruction project id is required")
	}
	query := `
		SELECT id, project_id, scope, title, body, priority, enabled, source, created_at, updated_at
		FROM project_instructions
		WHERE project_id = ?
	`
	args := []any{opts.ProjectID}
	if !opts.IncludeDisabled {
		query += " AND enabled = 1"
	}
	query += `
		ORDER BY
			CASE priority
				WHEN 'critical' THEN 0
				WHEN 'high' THEN 1
				WHEN 'normal' THEN 2
				WHEN 'low' THEN 3
				ELSE 4
			END,
			created_at,
			id
	`

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list project instructions: %w", err)
	}
	defer rows.Close()

	var instructions []ProjectInstruction
	for rows.Next() {
		instruction, err := scanProjectInstruction(rows)
		if err != nil {
			return nil, fmt.Errorf("scan project instruction: %w", err)
		}
		instructions = append(instructions, instruction)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate project instructions: %w", err)
	}
	return instructions, nil
}

func (s *Store) SetProjectInstructionEnabled(ctx context.Context, projectID, id int64, enabled bool) (ProjectInstruction, error) {
	if projectID <= 0 {
		return ProjectInstruction{}, errors.New("project instruction project id is required")
	}
	if id <= 0 {
		return ProjectInstruction{}, errors.New("project instruction id is required")
	}
	enabledValue := 0
	if enabled {
		enabledValue = 1
	}
	res, err := s.db.ExecContext(ctx, `
		UPDATE project_instructions
		SET enabled = ?, updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
		WHERE project_id = ? AND id = ?
	`, enabledValue, projectID, id)
	if err != nil {
		return ProjectInstruction{}, fmt.Errorf("set project instruction enabled: %w", err)
	}
	updated, err := res.RowsAffected()
	if err != nil {
		return ProjectInstruction{}, fmt.Errorf("read updated project instruction count: %w", err)
	}
	if updated == 0 {
		return ProjectInstruction{}, sql.ErrNoRows
	}
	return s.GetProjectInstruction(ctx, projectID, id)
}

func (s *Store) DeleteProjectInstruction(ctx context.Context, projectID, id int64) error {
	if projectID <= 0 {
		return errors.New("project instruction project id is required")
	}
	if id <= 0 {
		return errors.New("project instruction id is required")
	}
	res, err := s.db.ExecContext(ctx, `
		DELETE FROM project_instructions
		WHERE project_id = ? AND id = ?
	`, projectID, id)
	if err != nil {
		return fmt.Errorf("delete project instruction: %w", err)
	}
	deleted, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("read deleted project instruction count: %w", err)
	}
	if deleted == 0 {
		return sql.ErrNoRows
	}
	return nil
}
