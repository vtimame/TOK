package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	_ "github.com/mattn/go-sqlite3"
)

const DatabaseFileName = "tok.db"

var (
	ErrNoReadyTask                    = errors.New("no ready task")
	ErrTaskNotReady                   = errors.New("task is not ready to claim")
	ErrInvalidTaskTransition          = errors.New("invalid task status transition")
	ErrTaskCompletionNoteEmpty        = errors.New("task completion note is required")
	ErrTaskNoteEmpty                  = errors.New("task note is required")
	ErrInvalidRunTransition           = errors.New("invalid run status transition")
	ErrRunResultSummaryEmpty          = errors.New("run result summary is required")
	ErrActiveRunExists                = errors.New("active run already exists for task")
	ErrTaskCompletionEvidenceRequired = errors.New("task completion evidence run with passed validation is required")
	ErrTaskCompletionOverrideRequired = errors.New("task completion override reason is required")
	ErrInvalidTaskCompletionMode      = errors.New("invalid task completion mode")
	ErrTaskStatusDoneUnsupported      = errors.New("use CompleteTaskWithOptions to complete a task")
	ErrInvalidTaskSource              = errors.New("invalid task source")
	ErrInvalidTaskExternalReference   = errors.New("invalid task external reference")
)

// Store persists and retrieves application state.
type Store struct {
	db   *sql.DB
	path string
}

func DatabasePath(dataDir string) string {
	return filepath.Join(dataDir, DatabaseFileName)
}

func Open(ctx context.Context, path string) (*Store, error) {
	if strings.TrimSpace(path) == "" {
		return nil, errors.New("open sqlite database: path is empty")
	}
	if path != ":memory:" {
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			return nil, fmt.Errorf("create database directory: %w", err)
		}
	}

	db, err := sql.Open("sqlite3", sqliteDSN(path))
	if err != nil {
		return nil, fmt.Errorf("open sqlite database: %w", err)
	}
	db.SetMaxOpenConns(1)

	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping sqlite database: %w", err)
	}

	return &Store{db: db, path: path}, nil
}

func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *Store) DataDir() string {
	if s == nil || s.path == "" || s.path == ":memory:" {
		return ""
	}
	return filepath.Dir(s.path)
}

func (s *Store) Init(ctx context.Context) error {
	if s == nil || s.db == nil {
		return errors.New("initialize storage: store is nil")
	}
	if err := ensureMigrationsTable(ctx, s.db); err != nil {
		return err
	}

	migrations, err := loadMigrations()
	if err != nil {
		return err
	}

	for _, migration := range migrations {
		applied, err := migrationApplied(ctx, s.db, migration.version)
		if err != nil {
			return err
		}
		if applied {
			continue
		}
		if err := applyMigration(ctx, s.db, migration); err != nil {
			return err
		}
	}

	return nil
}
