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

	for _, table := range []string{"projects", "tasks", "task_events", "context_sources", "index_metadata", "retrieval_documents"} {
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
	if applied != 3 {
		t.Fatalf("expected 3 applied migrations, got %d", applied)
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

	comment, err := store.AddTaskComment(ctx, task.ID, "Needs review.")
	if err != nil {
		t.Fatalf("AddTaskComment returned error: %v", err)
	}
	if comment.Type != "commented" || comment.Body != "Needs review." {
		t.Fatalf("unexpected task comment event: %+v", comment)
	}

	events, err = store.ListTaskEvents(ctx, task.ID)
	if err != nil {
		t.Fatalf("ListTaskEvents returned error after comment: %v", err)
	}
	if len(events) != 3 || events[2].Type != "commented" {
		t.Fatalf("unexpected task events after comment: %+v", events)
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

func TestListReadyTasksExcludesActiveBlockedAndNonOpenTasks(t *testing.T) {
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

	blocker, err := store.CreateTask(ctx, CreateTaskInput{
		ProjectID: project.ID,
		Title:     "Blocker",
	})
	if err != nil {
		t.Fatalf("CreateTask blocker returned error: %v", err)
	}
	blocked, err := store.CreateTask(ctx, CreateTaskInput{
		ProjectID: project.ID,
		Title:     "Blocked",
	})
	if err != nil {
		t.Fatalf("CreateTask blocked returned error: %v", err)
	}
	inProgress, err := store.CreateTask(ctx, CreateTaskInput{
		ProjectID: project.ID,
		Title:     "Already claimed",
	})
	if err != nil {
		t.Fatalf("CreateTask in progress returned error: %v", err)
	}
	if _, err := store.UpdateTaskStatus(ctx, inProgress.ID, "in_progress"); err != nil {
		t.Fatalf("UpdateTaskStatus returned error: %v", err)
	}

	dependency, err := store.AddTaskDependency(ctx, "blocks", blocker.ID, blocked.ID)
	if err != nil {
		t.Fatalf("AddTaskDependency returned error: %v", err)
	}
	if dependency.EdgeType != "blocks" || dependency.BlockerTaskID != blocker.ID || dependency.BlockedTaskID != blocked.ID {
		t.Fatalf("unexpected dependency: %+v", dependency)
	}

	ready, err := store.ListReadyTasks(ctx, project.ID)
	if err != nil {
		t.Fatalf("ListReadyTasks returned error: %v", err)
	}
	if len(ready) != 1 || ready[0].ID != blocker.ID {
		t.Fatalf("expected only blocker to be ready, got %+v", ready)
	}

	if _, err := store.UpdateTaskStatus(ctx, blocker.ID, "done"); err != nil {
		t.Fatalf("UpdateTaskStatus done returned error: %v", err)
	}

	ready, err = store.ListReadyTasks(ctx, project.ID)
	if err != nil {
		t.Fatalf("ListReadyTasks after closing blocker returned error: %v", err)
	}
	if len(ready) != 1 || ready[0].ID != blocked.ID {
		t.Fatalf("expected blocked task to become ready, got %+v", ready)
	}

	if err := store.RemoveTaskDependency(ctx, "blocks", blocker.ID, blocked.ID); err != nil {
		t.Fatalf("RemoveTaskDependency returned error: %v", err)
	}
}

func TestReplaceIndexedDocumentsRebuildsProjectIndex(t *testing.T) {
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

	indexed, err := store.ReplaceIndexedDocuments(ctx, project.ID, []IndexedDocumentInput{
		{
			ProjectID:  project.ID,
			Path:       "README.md",
			Provenance: "project_file",
			SizeBytes:  14,
			Content:    "first content",
		},
	})
	if err != nil {
		t.Fatalf("ReplaceIndexedDocuments returned error: %v", err)
	}
	if indexed != 1 {
		t.Fatalf("expected 1 indexed document, got %d", indexed)
	}

	indexed, err = store.ReplaceIndexedDocuments(ctx, project.ID, []IndexedDocumentInput{
		{
			ProjectID:  project.ID,
			Path:       "internal/app/cli.go",
			Provenance: "project_file",
			SizeBytes:  15,
			Content:    "second content",
		},
	})
	if err != nil {
		t.Fatalf("second ReplaceIndexedDocuments returned error: %v", err)
	}
	if indexed != 1 {
		t.Fatalf("expected 1 indexed document after rebuild, got %d", indexed)
	}

	docs, err := store.ListIndexedDocuments(ctx, project.ID)
	if err != nil {
		t.Fatalf("ListIndexedDocuments returned error: %v", err)
	}
	if len(docs) != 1 || docs[0].Path != "internal/app/cli.go" || docs[0].Content != "second content" {
		t.Fatalf("unexpected indexed documents after rebuild: %+v", docs)
	}

	var metadataRows int
	if err := store.db.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM index_metadata
		WHERE project_id = ? AND source_id IS NULL AND key = 'retrieval_documents'
	`, project.ID).Scan(&metadataRows); err != nil {
		t.Fatalf("count index metadata rows: %v", err)
	}
	if metadataRows != 1 {
		t.Fatalf("expected 1 metadata row, got %d", metadataRows)
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
