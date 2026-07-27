package storage

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

type RunArtifact struct {
	ID          int64
	RunID       int64
	Kind        string
	Path        string
	ContentHash string
	SizeBytes   int64
	Truncated   bool
	Metadata    string
	ActorID     int64
	ActorKind   string
	ActorName   string
	CreatedAt   string
}

type AddRunArtifactInput struct {
	RunID       int64
	Kind        string
	Path        string
	ContentHash string
	SizeBytes   int64
	Truncated   bool
	Metadata    string
	Actor       ActorRef
}

func (s *Store) AddRunArtifact(ctx context.Context, input AddRunArtifactInput) (RunArtifact, error) {
	if input.RunID <= 0 {
		return RunArtifact{}, errors.New("run artifact run id is required")
	}
	if _, err := s.GetRun(ctx, input.RunID); err != nil {
		return RunArtifact{}, err
	}
	input.Kind = strings.TrimSpace(input.Kind)
	if !validRunArtifactKind(input.Kind) {
		return RunArtifact{}, fmt.Errorf("invalid run artifact kind %q", input.Kind)
	}
	input.Path = strings.TrimSpace(input.Path)
	input.ContentHash = strings.TrimSpace(input.ContentHash)
	if input.SizeBytes < 0 {
		return RunArtifact{}, errors.New("run artifact size bytes cannot be negative")
	}
	input.Metadata = strings.TrimSpace(input.Metadata)
	if input.Metadata == "" {
		input.Metadata = "{}"
	}

	actor := sanitizeActorRef(input.Actor)
	res, err := s.db.ExecContext(ctx, `
		INSERT INTO run_artifacts (run_id, kind, path, content_hash, size_bytes, truncated, metadata, actor_id, actor_kind, actor_name)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, input.RunID, input.Kind, input.Path, input.ContentHash, input.SizeBytes, boolToInt(input.Truncated), input.Metadata, actor.ID, actor.Kind, actor.Name)
	if err != nil {
		return RunArtifact{}, fmt.Errorf("add run artifact: %w", err)
	}

	id, err := res.LastInsertId()
	if err != nil {
		return RunArtifact{}, fmt.Errorf("read run artifact id: %w", err)
	}

	return s.GetRunArtifact(ctx, id)
}

func (s *Store) GetRunArtifact(ctx context.Context, id int64) (RunArtifact, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, run_id, kind, path, content_hash, size_bytes, truncated, metadata, actor_id, actor_kind, actor_name, created_at
		FROM run_artifacts
		WHERE id = ?
	`, id)
	return scanRunArtifact(row)
}

func (s *Store) ListRunArtifacts(ctx context.Context, runID int64) ([]RunArtifact, error) {
	if runID <= 0 {
		return nil, errors.New("run artifact run id is required")
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT id, run_id, kind, path, content_hash, size_bytes, truncated, metadata, actor_id, actor_kind, actor_name, created_at
		FROM run_artifacts
		WHERE run_id = ?
		ORDER BY id
	`, runID)
	if err != nil {
		return nil, fmt.Errorf("list run artifacts: %w", err)
	}
	defer rows.Close()

	var artifacts []RunArtifact
	for rows.Next() {
		artifact, err := scanRunArtifact(rows)
		if err != nil {
			return nil, fmt.Errorf("scan run artifact: %w", err)
		}
		artifacts = append(artifacts, artifact)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate run artifacts: %w", err)
	}

	return artifacts, nil
}
