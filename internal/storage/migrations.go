package storage

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

//go:embed migrations/*.sql
var migrationFiles embed.FS

type migration struct {
	version int64
	name    string
	sql     string
}

func ensureMigrationsTable(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version INTEGER PRIMARY KEY,
			name TEXT NOT NULL,
			applied_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
		)
	`)
	if err != nil {
		return fmt.Errorf("ensure schema migrations table: %w", err)
	}
	return nil
}

func loadMigrations() ([]migration, error) {
	entries, err := fs.ReadDir(migrationFiles, "migrations")
	if err != nil {
		return nil, fmt.Errorf("read embedded migrations: %w", err)
	}

	migrations := make([]migration, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}

		versionPart, _, ok := strings.Cut(entry.Name(), "_")
		if !ok {
			return nil, fmt.Errorf("migration %q must start with numeric version", entry.Name())
		}
		version, err := strconv.ParseInt(versionPart, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("parse migration version from %q: %w", entry.Name(), err)
		}

		data, err := migrationFiles.ReadFile(filepath.ToSlash(filepath.Join("migrations", entry.Name())))
		if err != nil {
			return nil, fmt.Errorf("read migration %q: %w", entry.Name(), err)
		}

		migrations = append(migrations, migration{
			version: version,
			name:    entry.Name(),
			sql:     string(data),
		})
	}

	sort.Slice(migrations, func(i, j int) bool {
		return migrations[i].version < migrations[j].version
	})

	return migrations, nil
}

func migrationApplied(ctx context.Context, db *sql.DB, version int64) (bool, error) {
	var exists int
	err := db.QueryRowContext(ctx, "SELECT 1 FROM schema_migrations WHERE version = ?", version).Scan(&exists)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("check migration %d: %w", version, err)
	}
	return true, nil
}

func applyMigration(ctx context.Context, db *sql.DB, migration migration) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin migration %s: %w", migration.name, err)
	}
	defer rollback(tx)

	if _, err := tx.ExecContext(ctx, migration.sql); err != nil {
		return fmt.Errorf("apply migration %s: %w", migration.name, err)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO schema_migrations (version, name)
		VALUES (?, ?)
	`, migration.version, migration.name); err != nil {
		return fmt.Errorf("record migration %s: %w", migration.name, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit migration %s: %w", migration.name, err)
	}

	return nil
}
