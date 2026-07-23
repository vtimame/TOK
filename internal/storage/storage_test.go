package storage

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
)

func TestInitAppliesEmbeddedMigrations(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)

	if err := store.Init(ctx); err != nil {
		t.Fatalf("Init returned error: %v", err)
	}
	if err := store.Init(ctx); err != nil {
		t.Fatalf("second Init returned error: %v", err)
	}

	for _, table := range []string{"projects", "tasks", "task_events", "context_sources", "index_metadata"} {
		var name string
		err := store.db.QueryRowContext(ctx, `
			SELECT name
			FROM sqlite_master
			WHERE type = 'table' AND name = ?
		`, table).Scan(&name)
		if err != nil {
			t.Fatalf("expected table %s: %v", table, err)
		}
	}

	var applied int
	if err := store.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM schema_migrations").Scan(&applied); err != nil {
		t.Fatalf("count migrations: %v", err)
	}
	if applied != 1 {
		t.Fatalf("expected 1 applied migration, got %d", applied)
	}
}

func TestStoreBasicCRUDPaths(t *testing.T) {
	ctx := context.Background()
	store := openInitializedTestStore(t)

	project, err := store.CreateProject(ctx, CreateProjectInput{
		Name:        "tok",
		DisplayName: "TOK",
		Path:        "/tmp/tok",
	})
	if err != nil {
		t.Fatalf("CreateProject returned error: %v", err)
	}
	if project.ID == 0 || project.CreatedAt == "" || project.UpdatedAt == "" {
		t.Fatalf("created project missing generated fields: %+v", project)
	}

	gotProject, err := store.GetProject(ctx, "tok")
	if err != nil {
		t.Fatalf("GetProject returned error: %v", err)
	}
	if gotProject.ID != project.ID || gotProject.Path != "/tmp/tok" {
		t.Fatalf("unexpected project: %+v", gotProject)
	}

	projects, err := store.ListProjects(ctx)
	if err != nil {
		t.Fatalf("ListProjects returned error: %v", err)
	}
	if len(projects) != 1 || projects[0].Name != "tok" {
		t.Fatalf("unexpected project list: %+v", projects)
	}

	task, err := store.CreateTask(ctx, CreateTaskInput{
		ProjectID:          project.ID,
		Title:              "Add storage",
		Description:        "Create SQLite storage layer.",
		AcceptanceCriteria: "- migrations\n- CRUD",
		Notes:              "First storage slice.",
	})
	if err != nil {
		t.Fatalf("CreateTask returned error: %v", err)
	}
	if task.Status != "open" || task.ProjectID != project.ID {
		t.Fatalf("unexpected created task: %+v", task)
	}

	task, err = store.UpdateTaskStatus(ctx, task.ID, "in_progress")
	if err != nil {
		t.Fatalf("UpdateTaskStatus returned error: %v", err)
	}
	if task.Status != "in_progress" {
		t.Fatalf("unexpected task status: %s", task.Status)
	}

	tasks, err := store.ListTasks(ctx, project.ID)
	if err != nil {
		t.Fatalf("ListTasks returned error: %v", err)
	}
	if len(tasks) != 1 || tasks[0].Title != "Add storage" {
		t.Fatalf("unexpected task list: %+v", tasks)
	}

	events, err := store.ListTaskEvents(ctx, task.ID)
	if err != nil {
		t.Fatalf("ListTaskEvents returned error: %v", err)
	}
	if len(events) != 2 || events[0].Type != "created" || events[1].Type != "status_changed" {
		t.Fatalf("unexpected task events: %+v", events)
	}

	source, err := store.UpsertContextSource(ctx, UpsertContextSourceInput{
		ProjectID: project.ID,
		Kind:      "filesystem",
		URI:       "/tmp/tok",
		Metadata:  `{"include":"**/*.go"}`,
	})
	if err != nil {
		t.Fatalf("UpsertContextSource returned error: %v", err)
	}
	if source.ID == 0 || source.Metadata == "" {
		t.Fatalf("unexpected context source: %+v", source)
	}

	metadata, err := store.UpsertIndexMetadata(ctx, project.ID, sql.NullInt64{Int64: source.ID, Valid: true}, "last_path", "README.md")
	if err != nil {
		t.Fatalf("UpsertIndexMetadata returned error: %v", err)
	}
	if metadata.Key != "last_path" || metadata.Value != "README.md" || !metadata.SourceID.Valid {
		t.Fatalf("unexpected index metadata: %+v", metadata)
	}
}

func TestOpenCreatesDatabaseDirectory(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "nested", DatabaseFileName)

	store, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	defer store.Close()

	if err := store.Init(ctx); err != nil {
		t.Fatalf("Init returned error: %v", err)
	}
}

func openTestStore(t *testing.T) *Store {
	t.Helper()

	path := filepath.Join(t.TempDir(), DatabaseFileName)
	store, err := Open(context.Background(), path)
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Fatalf("Close returned error: %v", err)
		}
	})

	return store
}

func openInitializedTestStore(t *testing.T) *Store {
	t.Helper()

	store := openTestStore(t)
	if err := store.Init(context.Background()); err != nil {
		t.Fatalf("Init returned error: %v", err)
	}

	return store
}
