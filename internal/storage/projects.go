package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

type Project struct {
	ID          int64
	Name        string
	DisplayName string
	Path        string
	CreatedAt   string
	UpdatedAt   string
}

type CreateProjectInput struct {
	Name        string
	DisplayName string
	Path        string
}

type UpdateProjectInput struct {
	Name        string
	DisplayName string
	Path        string
}

type ListProjectsOptions struct {
	Limit  int
	Cursor string
}

func validateProjectInput(input CreateProjectInput) error {
	if strings.TrimSpace(input.Name) == "" {
		return errors.New("project name is required")
	}
	if strings.TrimSpace(input.DisplayName) == "" {
		return errors.New("project display name is required")
	}
	if strings.TrimSpace(input.Path) == "" {
		return errors.New("project path is required")
	}
	return nil
}

func (s *Store) CreateProject(ctx context.Context, input CreateProjectInput) (Project, error) {
	if err := validateProjectInput(input); err != nil {
		return Project{}, err
	}

	res, err := s.db.ExecContext(ctx, `
		INSERT INTO projects (name, display_name, path)
		VALUES (?, ?, ?)
	`, input.Name, input.DisplayName, input.Path)
	if err != nil {
		return Project{}, fmt.Errorf("create project %q: %w", input.Name, err)
	}

	id, err := res.LastInsertId()
	if err != nil {
		return Project{}, fmt.Errorf("read created project id: %w", err)
	}

	return s.GetProjectByID(ctx, id)
}

func (s *Store) GetProject(ctx context.Context, name string) (Project, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, name, display_name, path, created_at, updated_at
		FROM projects
		WHERE name = ?
	`, name)
	return scanProject(row)
}

func (s *Store) GetProjectByID(ctx context.Context, id int64) (Project, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, name, display_name, path, created_at, updated_at
		FROM projects
		WHERE id = ?
	`, id)
	return scanProject(row)
}

func (s *Store) UpdateProject(ctx context.Context, id int64, input UpdateProjectInput) (Project, error) {
	if id <= 0 {
		return Project{}, errors.New("project id is required")
	}
	if err := validateProjectInput(CreateProjectInput(input)); err != nil {
		return Project{}, err
	}

	res, err := s.db.ExecContext(ctx, `
		UPDATE projects
		SET name = ?,
			display_name = ?,
			path = ?,
			updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
		WHERE id = ?
	`, input.Name, input.DisplayName, input.Path, id)
	if err != nil {
		return Project{}, fmt.Errorf("update project %d: %w", id, err)
	}
	updated, err := res.RowsAffected()
	if err != nil {
		return Project{}, fmt.Errorf("read updated project count: %w", err)
	}
	if updated == 0 {
		return Project{}, sql.ErrNoRows
	}

	return s.GetProjectByID(ctx, id)
}

func (s *Store) DeleteProject(ctx context.Context, id int64) error {
	if id <= 0 {
		return errors.New("project id is required")
	}
	res, err := s.db.ExecContext(ctx, "DELETE FROM projects WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("delete project %d: %w", id, err)
	}
	deleted, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("read deleted project count: %w", err)
	}
	if deleted == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Store) ListProjects(ctx context.Context) ([]Project, error) {
	return s.ListProjectsWithOptions(ctx, ListProjectsOptions{})
}

func (s *Store) ListProjectsWithOptions(ctx context.Context, opts ListProjectsOptions) ([]Project, error) {
	query := `
		SELECT id, name, display_name, path, created_at, updated_at
		FROM projects
	`
	args := []any{}
	if opts.Cursor != "" {
		query += " WHERE name > ?"
		args = append(args, opts.Cursor)
	}
	query += " ORDER BY name"
	if opts.Limit > 0 {
		query += " LIMIT ?"
		args = append(args, opts.Limit)
	}

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list projects: %w", err)
	}
	defer rows.Close()

	var projects []Project
	for rows.Next() {
		var project Project
		if err := rows.Scan(&project.ID, &project.Name, &project.DisplayName, &project.Path, &project.CreatedAt, &project.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan project: %w", err)
		}
		projects = append(projects, project)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate projects: %w", err)
	}

	return projects, nil
}

func (s *Store) CountProjects(ctx context.Context) (int, error) {
	var count int
	if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM projects").Scan(&count); err != nil {
		return 0, fmt.Errorf("count projects: %w", err)
	}
	return count, nil
}
