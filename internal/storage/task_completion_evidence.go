package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

type runValidationMetadata struct {
	Status string `json:"status"`
}

func (s *Store) validateTaskCompletionEvidenceInTx(ctx context.Context, tx *sql.Tx, taskID int64, evidenceRunID int64) (int64, int64, error) {
	if evidenceRunID > 0 {
		run, err := getRunInTx(ctx, tx, evidenceRunID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return 0, 0, nil
			}
			return 0, 0, fmt.Errorf("lookup evidence run: %w", err)
		}
		if run.TaskID != taskID || run.Status != "succeeded" {
			return 0, 0, nil
		}
		artifactID, err := s.findPassedValidationArtifactInRunInTx(ctx, tx, evidenceRunID)
		if err != nil {
			return 0, 0, err
		}
		if artifactID == 0 {
			return 0, 0, nil
		}
		return evidenceRunID, artifactID, nil
	}

	rows, err := tx.QueryContext(ctx, `
		SELECT id
		FROM runs
		WHERE task_id = ?
		  AND status = 'succeeded'
		ORDER BY id DESC
	`, taskID)
	if err != nil {
		return 0, 0, fmt.Errorf("list succeeded runs for evidence: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var runID int64
		if err := rows.Scan(&runID); err != nil {
			return 0, 0, fmt.Errorf("scan evidence run id: %w", err)
		}
		artifactID, err := s.findPassedValidationArtifactInRunInTx(ctx, tx, runID)
		if err != nil {
			return 0, 0, err
		}
		if artifactID == 0 {
			continue
		}
		return runID, artifactID, nil
	}
	if err := rows.Err(); err != nil {
		return 0, 0, fmt.Errorf("iterate evidence runs: %w", err)
	}
	return 0, 0, nil
}

func (s *Store) findPassedValidationArtifactInRunInTx(ctx context.Context, tx *sql.Tx, runID int64) (int64, error) {
	rows, err := tx.QueryContext(ctx, `
		SELECT id, metadata
		FROM run_artifacts
		WHERE run_id = ? AND kind = 'validation'
		ORDER BY id DESC
	`, runID)
	if err != nil {
		return 0, fmt.Errorf("list validation artifacts: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var id int64
		var metadata string
		if err := rows.Scan(&id, &metadata); err != nil {
			return 0, fmt.Errorf("scan validation artifact: %w", err)
		}
		var parsed runValidationMetadata
		if err := json.Unmarshal([]byte(metadata), &parsed); err != nil {
			continue
		}
		if strings.TrimSpace(parsed.Status) == "passed" {
			return id, nil
		}
	}
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("iterate validation artifacts: %w", err)
	}
	return 0, nil
}

func getRunInTx(ctx context.Context, tx *sql.Tx, id int64) (Run, error) {
	row := tx.QueryRowContext(ctx, `
		SELECT id, task_id, status, handoff_contract_version, retrieval_limit, started_at, finished_at, base_branch, base_head, result_summary, lease_owner, heartbeat_at, expires_at, actor_id, actor_kind, actor_name, finished_actor_id, finished_actor_kind, finished_actor_name
		FROM runs
		WHERE id = ?
	`, id)
	return scanRun(row)
}

func (s *Store) hasActiveRunForTaskInTx(ctx context.Context, tx *sql.Tx, taskID int64) (bool, error) {
	var id int64
	err := tx.QueryRowContext(ctx, `
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
		return false, fmt.Errorf("find active run in tx: %w", err)
	}
	return true, nil
}
