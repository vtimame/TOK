package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
)

type ContextSource struct {
	ID        int64
	ProjectID int64
	Kind      string
	URI       string
	Metadata  string
	CreatedAt string
	UpdatedAt string
}

type UpsertContextSourceInput struct {
	ProjectID int64
	Kind      string
	URI       string
	Metadata  string
}

type IndexMetadata struct {
	ID        int64
	ProjectID int64
	SourceID  sql.NullInt64
	Key       string
	Value     string
	UpdatedAt string
}

type IndexFileManifestInput struct {
	ProjectID     int64
	Path          string
	SizeBytes     int64
	ModTime       string
	ContentHash   string
	IndexedChunks int
	SkippedReason string
}

type IndexFileManifest struct {
	ProjectID     int64
	Path          string
	SizeBytes     int64
	ModTime       string
	ContentHash   string
	IndexedChunks int
	SkippedReason string
	UpdatedAt     string
}

type IndexPolicy struct {
	ProjectID       int64
	IncludePatterns []string
	IgnorePatterns  []string
	CreatedAt       string
	UpdatedAt       string
}

type IndexedDocumentInput struct {
	ProjectID  int64
	Path       string
	Provenance string
	SizeBytes  int64
	Content    string
}

type IndexedDocument struct {
	Path       string
	Provenance string
	Content    string
}

func (s *Store) UpsertContextSource(ctx context.Context, input UpsertContextSourceInput) (ContextSource, error) {
	if input.ProjectID <= 0 {
		return ContextSource{}, errors.New("context source project id is required")
	}
	if strings.TrimSpace(input.Kind) == "" {
		return ContextSource{}, errors.New("context source kind is required")
	}
	if strings.TrimSpace(input.URI) == "" {
		return ContextSource{}, errors.New("context source uri is required")
	}
	if input.Metadata == "" {
		input.Metadata = "{}"
	}

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO context_sources (project_id, kind, uri, metadata)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(project_id, kind, uri) DO UPDATE SET
			metadata = excluded.metadata,
			updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
	`, input.ProjectID, input.Kind, input.URI, input.Metadata)
	if err != nil {
		return ContextSource{}, fmt.Errorf("upsert context source: %w", err)
	}

	row := s.db.QueryRowContext(ctx, `
		SELECT id, project_id, kind, uri, metadata, created_at, updated_at
		FROM context_sources
		WHERE project_id = ? AND kind = ? AND uri = ?
	`, input.ProjectID, input.Kind, input.URI)
	return scanContextSource(row)
}

func (s *Store) UpsertIndexMetadata(ctx context.Context, projectID int64, sourceID sql.NullInt64, key, value string) (IndexMetadata, error) {
	if projectID <= 0 {
		return IndexMetadata{}, errors.New("index metadata project id is required")
	}
	if strings.TrimSpace(key) == "" {
		return IndexMetadata{}, errors.New("index metadata key is required")
	}

	if !sourceID.Valid {
		var id int64
		err := s.db.QueryRowContext(ctx, `
			SELECT id
			FROM index_metadata
			WHERE project_id = ? AND source_id IS NULL AND key = ?
		`, projectID, key).Scan(&id)
		if err == nil {
			if _, err := s.db.ExecContext(ctx, `
				UPDATE index_metadata
				SET value = ?, updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
				WHERE id = ?
			`, value, id); err != nil {
				return IndexMetadata{}, fmt.Errorf("update index metadata: %w", err)
			}
			row := s.db.QueryRowContext(ctx, `
				SELECT id, project_id, source_id, key, value, updated_at
				FROM index_metadata
				WHERE id = ?
			`, id)
			return scanIndexMetadata(row)
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return IndexMetadata{}, fmt.Errorf("find index metadata: %w", err)
		}
	}

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO index_metadata (project_id, source_id, key, value)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(project_id, source_id, key) DO UPDATE SET
			value = excluded.value,
			updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
	`, projectID, sourceID, key, value)
	if err != nil {
		return IndexMetadata{}, fmt.Errorf("upsert index metadata: %w", err)
	}
	row := s.db.QueryRowContext(ctx, `
		SELECT id, project_id, source_id, key, value, updated_at
		FROM index_metadata
		WHERE project_id = ? AND source_id IS ? AND key = ?
	`, projectID, sourceID, key)
	return scanIndexMetadata(row)
}

func (s *Store) ListProjectIndexMetadata(ctx context.Context, projectID int64) ([]IndexMetadata, error) {
	if projectID <= 0 {
		return nil, errors.New("index metadata project id is required")
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT id, project_id, source_id, key, value, updated_at
		FROM index_metadata
		WHERE project_id = ? AND source_id IS NULL
		ORDER BY key
	`, projectID)
	if err != nil {
		return nil, fmt.Errorf("list project index metadata: %w", err)
	}
	defer rows.Close()

	var metadata []IndexMetadata
	for rows.Next() {
		item, err := scanIndexMetadata(rows)
		if err != nil {
			return nil, fmt.Errorf("scan project index metadata: %w", err)
		}
		metadata = append(metadata, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate project index metadata: %w", err)
	}

	return metadata, nil
}

func (s *Store) ReplaceIndexedDocuments(ctx context.Context, projectID int64, docs []IndexedDocumentInput) (int, error) {
	if projectID <= 0 {
		return 0, errors.New("indexed document project id is required")
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin replace indexed documents transaction: %w", err)
	}
	defer rollback(tx)

	if _, err := tx.ExecContext(ctx, "DELETE FROM retrieval_documents WHERE project_id = ?", projectID); err != nil {
		return 0, fmt.Errorf("delete indexed documents: %w", err)
	}

	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO retrieval_documents (project_id, path, provenance, size_bytes, content)
		VALUES (?, ?, ?, ?, ?)
	`)
	if err != nil {
		return 0, fmt.Errorf("prepare indexed document insert: %w", err)
	}
	defer stmt.Close()

	for _, doc := range docs {
		if doc.ProjectID != projectID {
			return 0, errors.New("indexed document project id mismatch")
		}
		if strings.TrimSpace(doc.Path) == "" {
			return 0, errors.New("indexed document path is required")
		}
		if doc.Provenance == "" {
			doc.Provenance = "project_file"
		}
		if _, err := stmt.ExecContext(ctx, doc.ProjectID, doc.Path, doc.Provenance, doc.SizeBytes, doc.Content); err != nil {
			return 0, fmt.Errorf("insert indexed document %q: %w", doc.Path, err)
		}
	}

	if _, err := tx.ExecContext(ctx, `
		DELETE FROM index_metadata
		WHERE project_id = ? AND source_id IS NULL AND key = 'retrieval_documents'
	`, projectID); err != nil {
		return 0, fmt.Errorf("delete indexed document count metadata: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO index_metadata (project_id, source_id, key, value)
		VALUES (?, NULL, 'retrieval_documents', ?)
	`, projectID, strconv.Itoa(len(docs))); err != nil {
		return 0, fmt.Errorf("record indexed document count: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit replace indexed documents transaction: %w", err)
	}

	return len(docs), nil
}

func (s *Store) ReplaceIndexFileManifest(ctx context.Context, projectID int64, files []IndexFileManifestInput) error {
	if projectID <= 0 {
		return errors.New("index file manifest project id is required")
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin replace index file manifest transaction: %w", err)
	}
	defer rollback(tx)

	if _, err := tx.ExecContext(ctx, "DELETE FROM index_file_manifest WHERE project_id = ?", projectID); err != nil {
		return fmt.Errorf("delete index file manifest: %w", err)
	}

	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO index_file_manifest (project_id, path, size_bytes, mod_time, content_hash, indexed_chunks, skipped_reason)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return fmt.Errorf("prepare index file manifest insert: %w", err)
	}
	defer stmt.Close()

	for _, file := range files {
		if file.ProjectID != projectID {
			return errors.New("index file manifest project id mismatch")
		}
		if strings.TrimSpace(file.Path) == "" {
			return errors.New("index file manifest path is required")
		}
		if _, err := stmt.ExecContext(ctx, file.ProjectID, file.Path, file.SizeBytes, file.ModTime, file.ContentHash, file.IndexedChunks, file.SkippedReason); err != nil {
			return fmt.Errorf("insert index file manifest %q: %w", file.Path, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit replace index file manifest transaction: %w", err)
	}
	return nil
}

func (s *Store) UpsertIndexPolicy(ctx context.Context, projectID int64, includePatterns, ignorePatterns []string) (IndexPolicy, error) {
	if projectID <= 0 {
		return IndexPolicy{}, errors.New("index policy project id is required")
	}
	includeJSON, err := encodeStringList(normalizeStringList(includePatterns))
	if err != nil {
		return IndexPolicy{}, fmt.Errorf("encode include patterns: %w", err)
	}
	ignoreJSON, err := encodeStringList(normalizeStringList(ignorePatterns))
	if err != nil {
		return IndexPolicy{}, fmt.Errorf("encode ignore patterns: %w", err)
	}

	_, err = s.db.ExecContext(ctx, `
		INSERT INTO index_policies (project_id, include_patterns, ignore_patterns)
		VALUES (?, ?, ?)
		ON CONFLICT(project_id) DO UPDATE SET
			include_patterns = excluded.include_patterns,
			ignore_patterns = excluded.ignore_patterns,
			updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
	`, projectID, includeJSON, ignoreJSON)
	if err != nil {
		return IndexPolicy{}, fmt.Errorf("upsert index policy: %w", err)
	}
	return s.GetIndexPolicy(ctx, projectID)
}

func (s *Store) GetIndexPolicy(ctx context.Context, projectID int64) (IndexPolicy, error) {
	if projectID <= 0 {
		return IndexPolicy{}, errors.New("index policy project id is required")
	}
	row := s.db.QueryRowContext(ctx, `
		SELECT project_id, include_patterns, ignore_patterns, created_at, updated_at
		FROM index_policies
		WHERE project_id = ?
	`, projectID)
	return scanIndexPolicy(row)
}

func (s *Store) ListIndexFileManifest(ctx context.Context, projectID int64) ([]IndexFileManifest, error) {
	if projectID <= 0 {
		return nil, errors.New("index file manifest project id is required")
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT project_id, path, size_bytes, mod_time, content_hash, indexed_chunks, skipped_reason, updated_at
		FROM index_file_manifest
		WHERE project_id = ?
		ORDER BY path
	`, projectID)
	if err != nil {
		return nil, fmt.Errorf("list index file manifest: %w", err)
	}
	defer rows.Close()

	var files []IndexFileManifest
	for rows.Next() {
		var file IndexFileManifest
		if err := rows.Scan(&file.ProjectID, &file.Path, &file.SizeBytes, &file.ModTime, &file.ContentHash, &file.IndexedChunks, &file.SkippedReason, &file.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan index file manifest: %w", err)
		}
		files = append(files, file)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate index file manifest: %w", err)
	}
	return files, nil
}

func scanIndexPolicy(row interface {
	Scan(dest ...any) error
}) (IndexPolicy, error) {
	var policy IndexPolicy
	var includeJSON, ignoreJSON string
	if err := row.Scan(&policy.ProjectID, &includeJSON, &ignoreJSON, &policy.CreatedAt, &policy.UpdatedAt); err != nil {
		return IndexPolicy{}, err
	}
	var err error
	policy.IncludePatterns, err = decodeStringList(includeJSON)
	if err != nil {
		return IndexPolicy{}, fmt.Errorf("decode include patterns: %w", err)
	}
	policy.IgnorePatterns, err = decodeStringList(ignoreJSON)
	if err != nil {
		return IndexPolicy{}, fmt.Errorf("decode ignore patterns: %w", err)
	}
	return policy, nil
}

func encodeStringList(values []string) (string, error) {
	data, err := json.Marshal(values)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func decodeStringList(value string) ([]string, error) {
	if strings.TrimSpace(value) == "" {
		return nil, nil
	}
	var values []string
	if err := json.Unmarshal([]byte(value), &values); err != nil {
		return nil, err
	}
	return normalizeStringList(values), nil
}

func normalizeStringList(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(filepath.ToSlash(value))
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func (s *Store) ListIndexedDocuments(ctx context.Context, projectID int64) ([]IndexedDocument, error) {
	if projectID <= 0 {
		return nil, errors.New("indexed document project id is required")
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT path, provenance, content
		FROM retrieval_documents
		WHERE project_id = ?
		ORDER BY path
	`, projectID)
	if err != nil {
		return nil, fmt.Errorf("list indexed documents: %w", err)
	}
	defer rows.Close()

	var docs []IndexedDocument
	for rows.Next() {
		var doc IndexedDocument
		if err := rows.Scan(&doc.Path, &doc.Provenance, &doc.Content); err != nil {
			return nil, fmt.Errorf("scan indexed document: %w", err)
		}
		docs = append(docs, doc)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate indexed documents: %w", err)
	}

	return docs, nil
}
