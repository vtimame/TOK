package storage

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	_ "github.com/mattn/go-sqlite3"
)

const DatabaseFileName = "tok.db"

var (
	ErrNoReadyTask             = errors.New("no ready task")
	ErrTaskNotReady            = errors.New("task is not ready to claim")
	ErrInvalidTaskTransition   = errors.New("invalid task status transition")
	ErrTaskCompletionNoteEmpty = errors.New("task completion note is required")
	ErrTaskNoteEmpty           = errors.New("task note is required")
	ErrInvalidRunTransition    = errors.New("invalid run status transition")
	ErrRunResultSummaryEmpty   = errors.New("run result summary is required")
)

//go:embed migrations/*.sql
var migrationFiles embed.FS

type Store struct {
	db *sql.DB
}

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
}

type TaskEvent struct {
	ID         int64
	TaskID     int64
	Type       string
	Body       string
	FromStatus string
	ToStatus   string
	CreatedAt  string
}

type TaskDependency struct {
	ID            int64
	EdgeType      string
	BlockerTaskID int64
	BlockedTaskID int64
	CreatedAt     string
}

type Run struct {
	ID                     int64
	TaskID                 int64
	Status                 string
	HandoffContractVersion string
	RetrievalLimit         int
	StartedAt              string
	FinishedAt             string
	BaseBranch             string
	BaseHead               string
	ResultSummary          string
}

type RunArtifact struct {
	ID          int64
	RunID       int64
	Kind        string
	Path        string
	ContentHash string
	Metadata    string
	CreatedAt   string
}

type CreateRunInput struct {
	TaskID                 int64
	Status                 string
	HandoffContractVersion string
	RetrievalLimit         int
	BaseBranch             string
	BaseHead               string
}

type FinishRunInput struct {
	ID            int64
	Status        string
	ResultSummary string
}

type AddRunArtifactInput struct {
	RunID       int64
	Kind        string
	Path        string
	ContentHash string
	Metadata    string
}

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

	return &Store{db: db}, nil
}

func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
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

func (s *Store) ListProjects(ctx context.Context) ([]Project, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, name, display_name, path, created_at, updated_at
		FROM projects
		ORDER BY name
	`)
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

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO task_events (task_id, type, body, to_status)
		VALUES (?, 'created', ?, 'open')
	`, id, input.Title); err != nil {
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
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, project_id, status, title, description, acceptance_criteria, notes, created_at, updated_at
		FROM tasks
		WHERE project_id = ?
		ORDER BY id
	`, projectID)
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

func (s *Store) UpdateTaskStatus(ctx context.Context, id int64, status string) (Task, error) {
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

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO task_events (task_id, type, from_status, to_status)
		VALUES (?, 'status_changed', ?, ?)
	`, id, current.Status, status); err != nil {
		return Task{}, fmt.Errorf("record task status event: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return Task{}, fmt.Errorf("commit update task status transaction: %w", err)
	}

	return s.GetTask(ctx, id)
}

func (s *Store) CompleteTask(ctx context.Context, id int64, note string) (Task, error) {
	note = strings.TrimSpace(note)
	if id <= 0 {
		return Task{}, errors.New("task id is required")
	}
	if note == "" {
		return Task{}, ErrTaskCompletionNoteEmpty
	}

	current, err := s.GetTask(ctx, id)
	if err != nil {
		return Task{}, err
	}
	if current.Status != "in_progress" {
		return Task{}, ErrInvalidTaskTransition
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
	`, id)
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

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO task_events (task_id, type, body, from_status, to_status)
		VALUES (?, 'completed', ?, 'in_progress', 'done')
	`, id, note); err != nil {
		return Task{}, fmt.Errorf("record task completion event: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return Task{}, fmt.Errorf("commit complete task transaction: %w", err)
	}

	return s.GetTask(ctx, id)
}

func (s *Store) AddTaskProgress(ctx context.Context, taskID int64, body string) (TaskEvent, error) {
	return s.addTaskNoteEvent(ctx, taskID, "progress", body)
}

func (s *Store) BlockTask(ctx context.Context, id int64, reason string) (Task, error) {
	return s.transitionTaskWithNote(ctx, id, "blocked", "blocked", reason)
}

func (s *Store) UnblockTask(ctx context.Context, id int64, note string) (Task, error) {
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

	return s.transitionTaskWithNote(ctx, id, "open", "unblocked", note)
}

func (s *Store) ListTaskEvents(ctx context.Context, taskID int64) ([]TaskEvent, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, task_id, type, body, from_status, to_status, created_at
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
		if err := rows.Scan(&event.ID, &event.TaskID, &event.Type, &event.Body, &event.FromStatus, &event.ToStatus, &event.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan task event: %w", err)
		}
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate task events: %w", err)
	}

	return events, nil
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

func (s *Store) CreateRun(ctx context.Context, input CreateRunInput) (Run, error) {
	if input.TaskID <= 0 {
		return Run{}, errors.New("run task id is required")
	}
	if _, err := s.GetTask(ctx, input.TaskID); err != nil {
		return Run{}, err
	}
	status := strings.TrimSpace(input.Status)
	if status == "" {
		status = "created"
	}
	if !validRunStatus(status) {
		return Run{}, fmt.Errorf("invalid run status %q", status)
	}
	if runStatusTerminal(status) {
		return Run{}, ErrInvalidRunTransition
	}
	input.HandoffContractVersion = strings.TrimSpace(input.HandoffContractVersion)
	if input.HandoffContractVersion == "" {
		return Run{}, errors.New("run handoff contract version is required")
	}
	if input.RetrievalLimit <= 0 {
		input.RetrievalLimit = 5
	}

	res, err := s.db.ExecContext(ctx, `
		INSERT INTO runs (task_id, status, handoff_contract_version, retrieval_limit, base_branch, base_head)
		VALUES (?, ?, ?, ?, ?, ?)
	`, input.TaskID, status, input.HandoffContractVersion, input.RetrievalLimit, input.BaseBranch, input.BaseHead)
	if err != nil {
		return Run{}, fmt.Errorf("create run: %w", err)
	}

	id, err := res.LastInsertId()
	if err != nil {
		return Run{}, fmt.Errorf("read created run id: %w", err)
	}

	return s.GetRun(ctx, id)
}

func (s *Store) GetRun(ctx context.Context, id int64) (Run, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, task_id, status, handoff_contract_version, retrieval_limit, started_at, finished_at, base_branch, base_head, result_summary
		FROM runs
		WHERE id = ?
	`, id)
	return scanRun(row)
}

func (s *Store) FinishRun(ctx context.Context, input FinishRunInput) (Run, error) {
	if input.ID <= 0 {
		return Run{}, errors.New("run id is required")
	}
	input.Status = strings.TrimSpace(input.Status)
	if !runStatusTerminal(input.Status) {
		return Run{}, fmt.Errorf("invalid terminal run status %q", input.Status)
	}
	input.ResultSummary = strings.TrimSpace(input.ResultSummary)
	if input.ResultSummary == "" {
		return Run{}, ErrRunResultSummaryEmpty
	}

	current, err := s.GetRun(ctx, input.ID)
	if err != nil {
		return Run{}, err
	}
	if runStatusTerminal(current.Status) {
		return Run{}, ErrInvalidRunTransition
	}

	res, err := s.db.ExecContext(ctx, `
		UPDATE runs
		SET status = ?,
			finished_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now'),
			result_summary = ?
		WHERE id = ?
		  AND status IN ('created', 'in_progress')
	`, input.Status, input.ResultSummary, input.ID)
	if err != nil {
		return Run{}, fmt.Errorf("finish run: %w", err)
	}
	updated, err := res.RowsAffected()
	if err != nil {
		return Run{}, fmt.Errorf("read finished run count: %w", err)
	}
	if updated == 0 {
		return Run{}, ErrInvalidRunTransition
	}

	return s.GetRun(ctx, input.ID)
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
	input.Metadata = strings.TrimSpace(input.Metadata)
	if input.Metadata == "" {
		input.Metadata = "{}"
	}

	res, err := s.db.ExecContext(ctx, `
		INSERT INTO run_artifacts (run_id, kind, path, content_hash, metadata)
		VALUES (?, ?, ?, ?, ?)
	`, input.RunID, input.Kind, input.Path, input.ContentHash, input.Metadata)
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
		SELECT id, run_id, kind, path, content_hash, metadata, created_at
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
		SELECT id, run_id, kind, path, content_hash, metadata, created_at
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

	task, err := claimTaskInTx(ctx, tx, projectID, taskID)
	if err != nil {
		return Task{}, err
	}
	if err := tx.Commit(); err != nil {
		return Task{}, fmt.Errorf("commit claim next task transaction: %w", err)
	}

	return task, nil
}

func (s *Store) ClaimTask(ctx context.Context, projectID, taskID int64) (Task, error) {
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

	task, err := claimTaskInTx(ctx, tx, projectID, taskID)
	if err != nil {
		return Task{}, err
	}
	if err := tx.Commit(); err != nil {
		return Task{}, fmt.Errorf("commit claim task transaction: %w", err)
	}

	return task, nil
}

func (s *Store) AddTaskComment(ctx context.Context, taskID int64, body string) (TaskEvent, error) {
	return s.addTaskNoteEvent(ctx, taskID, "commented", body)
}

func (s *Store) addTaskNoteEvent(ctx context.Context, taskID int64, eventType, body string) (TaskEvent, error) {
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

	res, err := s.db.ExecContext(ctx, `
		INSERT INTO task_events (task_id, type, body)
		VALUES (?, ?, ?)
	`, taskID, eventType, body)
	if err != nil {
		return TaskEvent{}, fmt.Errorf("record task %s event: %w", eventType, err)
	}

	id, err := res.LastInsertId()
	if err != nil {
		return TaskEvent{}, fmt.Errorf("read task %s event id: %w", eventType, err)
	}

	return s.GetTaskEvent(ctx, id)
}

func (s *Store) transitionTaskWithNote(ctx context.Context, id int64, status, eventType, body string) (Task, error) {
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

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO task_events (task_id, type, body, from_status, to_status)
		VALUES (?, ?, ?, ?, ?)
	`, id, eventType, body, current.Status, status); err != nil {
		return Task{}, fmt.Errorf("record task %s event: %w", eventType, err)
	}

	if err := tx.Commit(); err != nil {
		return Task{}, fmt.Errorf("commit task %s transaction: %w", eventType, err)
	}

	return s.GetTask(ctx, id)
}

func claimTaskInTx(ctx context.Context, tx *sql.Tx, projectID, taskID int64) (Task, error) {
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

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO task_events (task_id, type, from_status, to_status)
		VALUES (?, 'claimed', 'open', 'in_progress')
	`, taskID); err != nil {
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
		SELECT id, task_id, type, body, from_status, to_status, created_at
		FROM task_events
		WHERE id = ?
	`, id)

	var event TaskEvent
	if err := row.Scan(&event.ID, &event.TaskID, &event.Type, &event.Body, &event.FromStatus, &event.ToStatus, &event.CreatedAt); err != nil {
		return TaskEvent{}, err
	}
	return event, nil
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

type migration struct {
	version int64
	name    string
	sql     string
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

type scanner interface {
	Scan(dest ...any) error
}

func scanProject(row scanner) (Project, error) {
	var project Project
	if err := row.Scan(&project.ID, &project.Name, &project.DisplayName, &project.Path, &project.CreatedAt, &project.UpdatedAt); err != nil {
		return Project{}, err
	}
	return project, nil
}

func scanTask(row scanner) (Task, error) {
	var task Task
	if err := row.Scan(&task.ID, &task.ProjectID, &task.Status, &task.Title, &task.Description, &task.AcceptanceCriteria, &task.Notes, &task.CreatedAt, &task.UpdatedAt); err != nil {
		return Task{}, err
	}
	return task, nil
}

func scanTaskDependency(row scanner) (TaskDependency, error) {
	var dependency TaskDependency
	if err := row.Scan(&dependency.ID, &dependency.EdgeType, &dependency.BlockerTaskID, &dependency.BlockedTaskID, &dependency.CreatedAt); err != nil {
		return TaskDependency{}, err
	}
	return dependency, nil
}

func scanRun(row scanner) (Run, error) {
	var run Run
	if err := row.Scan(
		&run.ID,
		&run.TaskID,
		&run.Status,
		&run.HandoffContractVersion,
		&run.RetrievalLimit,
		&run.StartedAt,
		&run.FinishedAt,
		&run.BaseBranch,
		&run.BaseHead,
		&run.ResultSummary,
	); err != nil {
		return Run{}, err
	}
	return run, nil
}

func scanRunArtifact(row scanner) (RunArtifact, error) {
	var artifact RunArtifact
	if err := row.Scan(
		&artifact.ID,
		&artifact.RunID,
		&artifact.Kind,
		&artifact.Path,
		&artifact.ContentHash,
		&artifact.Metadata,
		&artifact.CreatedAt,
	); err != nil {
		return RunArtifact{}, err
	}
	return artifact, nil
}

func scanContextSource(row scanner) (ContextSource, error) {
	var source ContextSource
	if err := row.Scan(&source.ID, &source.ProjectID, &source.Kind, &source.URI, &source.Metadata, &source.CreatedAt, &source.UpdatedAt); err != nil {
		return ContextSource{}, err
	}
	return source, nil
}

func scanIndexMetadata(row scanner) (IndexMetadata, error) {
	var metadata IndexMetadata
	if err := row.Scan(&metadata.ID, &metadata.ProjectID, &metadata.SourceID, &metadata.Key, &metadata.Value, &metadata.UpdatedAt); err != nil {
		return IndexMetadata{}, err
	}
	return metadata, nil
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

func validTaskStatus(status string) bool {
	switch status {
	case "open", "in_progress", "blocked", "done":
		return true
	default:
		return false
	}
}

func validRunStatus(status string) bool {
	switch status {
	case "created", "in_progress", "succeeded", "failed", "blocked", "cancelled":
		return true
	default:
		return false
	}
}

func runStatusTerminal(status string) bool {
	switch status {
	case "succeeded", "failed", "blocked", "cancelled":
		return true
	default:
		return false
	}
}

func validRunArtifactKind(kind string) bool {
	switch kind {
	case "handoff", "validation", "log", "patch", "note":
		return true
	default:
		return false
	}
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

func sqliteDSN(path string) string {
	if path == ":memory:" {
		return "file::memory:?cache=shared&_foreign_keys=on&_busy_timeout=5000"
	}
	values := url.Values{}
	values.Set("_foreign_keys", "on")
	values.Set("_busy_timeout", "5000")
	return (&url.URL{
		Scheme:   "file",
		Path:     filepath.ToSlash(path),
		RawQuery: values.Encode(),
	}).String()
}

func rollback(tx *sql.Tx) {
	if tx != nil {
		_ = tx.Rollback()
	}
}
