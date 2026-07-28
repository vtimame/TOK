package storage

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
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

	for _, table := range []string{"projects", "tasks", "task_events", "context_sources", "index_metadata", "retrieval_documents", "runs", "run_artifacts", "actors", "index_file_manifest", "index_policies", "project_instructions"} {
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
	if applied != 15 {
		t.Fatalf("expected 15 applied migrations, got %d", applied)
	}
}

func TestInitMigratesV021DatabaseFixture(t *testing.T) {
	ctx := context.Background()
	legacyPath := createV021MigrationFixture(t, ctx)
	migratedPath := filepath.Join(t.TempDir(), DatabaseFileName)
	copyFile(t, legacyPath, migratedPath)

	store, err := Open(ctx, migratedPath)
	if err != nil {
		t.Fatalf("Open migrated fixture returned error: %v", err)
	}
	if err := store.Init(ctx); err != nil {
		t.Fatalf("Init migrated fixture returned error: %v", err)
	}

	assertCurrentMigrationVersion(t, ctx, store, 15)
	assertColumnExists(t, ctx, store, "task_events", "evidence_run_id")
	assertColumnExists(t, ctx, store, "task_events", "evidence_artifact_id")
	assertColumnExists(t, ctx, store, "tasks", "source")
	assertColumnExists(t, ctx, store, "tasks", "external_id")
	assertColumnExists(t, ctx, store, "tasks", "external_url")
	assertColumnExists(t, ctx, store, "tasks", "external_revision")

	statusChangedTask, err := store.GetTask(ctx, 1)
	if err != nil {
		t.Fatalf("GetTask legacy status_changed task returned error: %v", err)
	}
	if statusChangedTask.Status != "done" {
		t.Fatalf("expected legacy status_changed task to remain done, got %q", statusChangedTask.Status)
	}
	if statusChangedTask.Source != "local" || statusChangedTask.ExternalID != "" || statusChangedTask.ExternalURL != "" || statusChangedTask.ExternalRevision != "" {
		t.Fatalf("unexpected migrated external reference defaults: %+v", statusChangedTask)
	}

	statusChangedEvents, err := store.ListTaskEvents(ctx, statusChangedTask.ID)
	if err != nil {
		t.Fatalf("ListTaskEvents legacy status_changed task returned error: %v", err)
	}
	assertTaskEvents(t, statusChangedEvents, []TaskEvent{
		{Type: "created", Body: "Legacy status_changed completion", ToStatus: "open"},
		{Type: "status_changed", FromStatus: "open", ToStatus: "in_progress"},
		{Type: "status_changed", FromStatus: "in_progress", ToStatus: "done"},
	})

	overrideTask, err := store.GetTask(ctx, 2)
	if err != nil {
		t.Fatalf("GetTask legacy override task returned error: %v", err)
	}
	if overrideTask.Status != "done" {
		t.Fatalf("expected legacy override task to remain done, got %q", overrideTask.Status)
	}
	if overrideTask.Source != "local" || overrideTask.ExternalID != "" || overrideTask.ExternalURL != "" || overrideTask.ExternalRevision != "" {
		t.Fatalf("unexpected migrated override task external reference defaults: %+v", overrideTask)
	}

	overrideEvents, err := store.ListTaskEvents(ctx, overrideTask.ID)
	if err != nil {
		t.Fatalf("ListTaskEvents legacy override task returned error: %v", err)
	}
	assertTaskEvents(t, overrideEvents, []TaskEvent{
		{Type: "created", Body: "Legacy override completion", ToStatus: "open"},
		{Type: "status_changed", FromStatus: "open", ToStatus: "in_progress"},
		{Type: "completed", Body: "legacy completion note", FromStatus: "in_progress", ToStatus: "done"},
		{Type: "completion_override", Body: "legacy override reason", FromStatus: "in_progress", ToStatus: "done"},
	})
	for _, event := range append(statusChangedEvents, overrideEvents...) {
		if event.EvidenceRunID != 0 || event.EvidenceArtifactID != 0 {
			t.Fatalf("expected migrated event evidence defaults to be zero, got %+v", event)
		}
	}

	if err := store.Close(); err != nil {
		t.Fatalf("Close migrated fixture returned error: %v", err)
	}

	reopened, err := Open(ctx, migratedPath)
	if err != nil {
		t.Fatalf("Open migrated fixture after close returned error: %v", err)
	}
	defer func() {
		if err := reopened.Close(); err != nil {
			t.Fatalf("Close reopened fixture returned error: %v", err)
		}
	}()
	if err := reopened.Init(ctx); err != nil {
		t.Fatalf("second Init migrated fixture returned error: %v", err)
	}
	assertCurrentMigrationVersion(t, ctx, reopened, 15)

	tasks, err := reopened.ListTasks(ctx, 1)
	if err != nil {
		t.Fatalf("ListTasks after second Init returned error: %v", err)
	}
	if len(tasks) != 2 || tasks[0].Status != "done" || tasks[1].Status != "done" {
		t.Fatalf("unexpected migrated tasks after second Init: %+v", tasks)
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
	if task.Source != "local" || task.ExternalID != "" || task.ExternalURL != "" || task.ExternalRevision != "" {
		t.Fatalf("unexpected default task source: %+v", task)
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

func TestStoreTaskExternalReferenceCreateReadAndUpdate(t *testing.T) {
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

	task, err := store.CreateTask(ctx, CreateTaskInput{
		ProjectID:        project.ID,
		Title:            "Implement linked issue",
		Source:           "github",
		ExternalID:       "42",
		ExternalURL:      "https://github.com/vtimame/TOK/issues/42",
		ExternalRevision: "2026-07-27T19:00:00Z",
	})
	if err != nil {
		t.Fatalf("CreateTask returned error: %v", err)
	}
	if task.Source != "github" || task.ExternalID != "42" || task.ExternalURL != "https://github.com/vtimame/TOK/issues/42" || task.ExternalRevision != "2026-07-27T19:00:00Z" {
		t.Fatalf("unexpected created external reference: %+v", task)
	}

	got, err := store.GetTask(ctx, task.ID)
	if err != nil {
		t.Fatalf("GetTask returned error: %v", err)
	}
	if got.Source != task.Source || got.ExternalID != task.ExternalID || got.ExternalURL != task.ExternalURL || got.ExternalRevision != task.ExternalRevision {
		t.Fatalf("unexpected read external reference: %+v", got)
	}

	updated, err := store.UpdateTaskExternalReference(ctx, UpdateTaskExternalReferenceInput{
		ID:               task.ID,
		Source:           "linear",
		ExternalID:       "TOK-42",
		ExternalURL:      "https://linear.app/tok/issue/TOK-42",
		ExternalRevision: "rev-2",
	})
	if err != nil {
		t.Fatalf("UpdateTaskExternalReference returned error: %v", err)
	}
	if updated.Source != "linear" || updated.ExternalID != "TOK-42" || updated.ExternalURL != "https://linear.app/tok/issue/TOK-42" || updated.ExternalRevision != "rev-2" {
		t.Fatalf("unexpected updated external reference: %+v", updated)
	}

	events, err := store.ListTaskEvents(ctx, task.ID)
	if err != nil {
		t.Fatalf("ListTaskEvents returned error: %v", err)
	}
	if len(events) != 2 || events[1].Type != "source_updated" || !strings.Contains(events[1].Body, "source=linear") {
		t.Fatalf("unexpected source update event: %+v", events)
	}

	local, err := store.UpdateTaskExternalReference(ctx, UpdateTaskExternalReferenceInput{
		ID:     task.ID,
		Source: "local",
	})
	if err != nil {
		t.Fatalf("UpdateTaskExternalReference local returned error: %v", err)
	}
	if local.Source != "local" || local.ExternalID != "" || local.ExternalURL != "" || local.ExternalRevision != "" {
		t.Fatalf("unexpected local external reference reset: %+v", local)
	}
}

func TestStoreTaskExternalReferenceValidation(t *testing.T) {
	ctx := context.Background()
	store := openInitializedTestStore(t)

	project, err := store.CreateProject(ctx, CreateProjectInput{Name: "tok", DisplayName: "TOK", Path: "/tmp/tok"})
	if err != nil {
		t.Fatalf("CreateProject returned error: %v", err)
	}

	for _, input := range []CreateTaskInput{
		{ProjectID: project.ID, Title: "Invalid source", Source: "trello", ExternalID: "1", ExternalURL: "https://example.com/1"},
		{ProjectID: project.ID, Title: "Missing id", Source: "github", ExternalURL: "https://github.com/vtimame/TOK/issues/1"},
		{ProjectID: project.ID, Title: "Missing url", Source: "github", ExternalID: "1"},
		{ProjectID: project.ID, Title: "Local with external", Source: "local", ExternalID: "1"},
		{ProjectID: project.ID, Title: "JavaScript url", Source: "github", ExternalID: "1", ExternalURL: "javascript:alert(1)"},
		{ProjectID: project.ID, Title: "Data url", Source: "github", ExternalID: "1", ExternalURL: "data:text/html,<p>x</p>"},
		{ProjectID: project.ID, Title: "Invalid url", Source: "github", ExternalID: "1", ExternalURL: "://not-a-url"},
	} {
		if _, err := store.CreateTask(ctx, input); err == nil {
			t.Fatalf("CreateTask(%+v) succeeded, want validation error", input)
		}
	}

	created, err := store.CreateTask(ctx, CreateTaskInput{
		ProjectID:   project.ID,
		Title:       "HTTP external url",
		Source:      "github",
		ExternalID:  "2",
		ExternalURL: "http://example.com/2",
	})
	if err != nil {
		t.Fatalf("CreateTask with http external url returned error: %v", err)
	}
	if created.ExternalURL != "http://example.com/2" {
		t.Fatalf("unexpected http external url: %+v", created)
	}

	for _, externalURL := range []string{"javascript:alert(1)", "data:text/html,<p>x</p>", "://not-a-url"} {
		_, err := store.UpdateTaskExternalReference(ctx, UpdateTaskExternalReferenceInput{
			ID:          created.ID,
			Source:      "github",
			ExternalID:  "2",
			ExternalURL: externalURL,
		})
		if !errors.Is(err, ErrInvalidTaskExternalReference) {
			t.Fatalf("UpdateTaskExternalReference(%q) error = %v, want ErrInvalidTaskExternalReference", externalURL, err)
		}
	}
}

func TestStoreUpdatesAndDeletesProjects(t *testing.T) {
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
	task, err := store.CreateTask(ctx, CreateTaskInput{
		ProjectID: project.ID,
		Title:     "Project-owned task",
	})
	if err != nil {
		t.Fatalf("CreateTask returned error: %v", err)
	}

	updated, err := store.UpdateProject(ctx, project.ID, UpdateProjectInput{
		Name:        "tok-renamed",
		DisplayName: "TOK Renamed",
		Path:        "/tmp/tok-renamed",
	})
	if err != nil {
		t.Fatalf("UpdateProject returned error: %v", err)
	}
	if updated.ID != project.ID || updated.Name != "tok-renamed" || updated.DisplayName != "TOK Renamed" || updated.Path != "/tmp/tok-renamed" {
		t.Fatalf("unexpected updated project: %+v", updated)
	}
	if _, err := store.GetProject(ctx, "tok"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("GetProject old name error = %v, want sql.ErrNoRows", err)
	}

	if err := store.DeleteProject(ctx, updated.ID); err != nil {
		t.Fatalf("DeleteProject returned error: %v", err)
	}
	if _, err := store.GetProject(ctx, "tok-renamed"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("GetProject deleted error = %v, want sql.ErrNoRows", err)
	}
	if _, err := store.GetTask(ctx, task.ID); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("GetTask after project delete error = %v, want sql.ErrNoRows", err)
	}
}

func TestProjectInstructionsCRUDAndOrdering(t *testing.T) {
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

	normal, err := store.CreateProjectInstruction(ctx, CreateProjectInstructionInput{
		ProjectID: project.ID,
		Title:     "Use Context7",
		Body:      "Use Context7 for library documentation.",
	})
	if err != nil {
		t.Fatalf("CreateProjectInstruction normal returned error: %v", err)
	}
	critical, err := store.CreateProjectInstruction(ctx, CreateProjectInstructionInput{
		ProjectID: project.ID,
		Title:     "Never skip tests",
		Body:      "Run focused tests before reporting completion.",
		Priority:  "critical",
		Source:    "manual",
	})
	if err != nil {
		t.Fatalf("CreateProjectInstruction critical returned error: %v", err)
	}
	if normal.Priority != "normal" || normal.Scope != "project" || !normal.Enabled || normal.Source != "manual" {
		t.Fatalf("unexpected normal instruction defaults: %+v", normal)
	}

	instructions, err := store.ListProjectInstructions(ctx, ListProjectInstructionsOptions{ProjectID: project.ID})
	if err != nil {
		t.Fatalf("ListProjectInstructions returned error: %v", err)
	}
	if len(instructions) != 2 || instructions[0].ID != critical.ID || instructions[1].ID != normal.ID {
		t.Fatalf("unexpected instruction order: %+v", instructions)
	}

	disabled, err := store.SetProjectInstructionEnabled(ctx, project.ID, critical.ID, false)
	if err != nil {
		t.Fatalf("SetProjectInstructionEnabled returned error: %v", err)
	}
	if disabled.Enabled {
		t.Fatalf("expected disabled instruction: %+v", disabled)
	}
	enabledOnly, err := store.ListProjectInstructions(ctx, ListProjectInstructionsOptions{ProjectID: project.ID})
	if err != nil {
		t.Fatalf("ListProjectInstructions enabled returned error: %v", err)
	}
	if len(enabledOnly) != 1 || enabledOnly[0].ID != normal.ID {
		t.Fatalf("unexpected enabled instructions: %+v", enabledOnly)
	}
	withDisabled, err := store.ListProjectInstructions(ctx, ListProjectInstructionsOptions{ProjectID: project.ID, IncludeDisabled: true})
	if err != nil {
		t.Fatalf("ListProjectInstructions disabled returned error: %v", err)
	}
	if len(withDisabled) != 2 {
		t.Fatalf("expected disabled instructions in listing: %+v", withDisabled)
	}

	if err := store.DeleteProjectInstruction(ctx, project.ID, normal.ID); err != nil {
		t.Fatalf("DeleteProjectInstruction returned error: %v", err)
	}
	if _, err := store.GetProjectInstruction(ctx, project.ID, normal.ID); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("GetProjectInstruction deleted error = %v, want sql.ErrNoRows", err)
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

	if _, err := store.UpdateTaskStatus(ctx, blocker.ID, "in_progress"); err != nil {
		t.Fatalf("UpdateTaskStatus in_progress returned error: %v", err)
	}
	if _, err := store.CompleteTaskWithOptions(ctx, CompleteTaskInput{
		ID:             blocker.ID,
		Mode:           CompletionOverride,
		Note:           "Dependency blocker resolved.",
		OverrideReason: "Ready task fixture override.",
	}); err != nil {
		t.Fatalf("CompleteTaskWithOptions override returned error: %v", err)
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

func TestClaimTasksAtomicallyMarksReadyTaskInProgress(t *testing.T) {
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
	if _, err := store.AddTaskDependency(ctx, "blocks", blocker.ID, blocked.ID); err != nil {
		t.Fatalf("AddTaskDependency returned error: %v", err)
	}

	claimed, err := store.ClaimNextReadyTask(ctx, project.ID)
	if err != nil {
		t.Fatalf("ClaimNextReadyTask returned error: %v", err)
	}
	if claimed.ID != blocker.ID || claimed.Status != "in_progress" {
		t.Fatalf("expected blocker to be claimed, got %+v", claimed)
	}

	events, err := store.ListTaskEvents(ctx, claimed.ID)
	if err != nil {
		t.Fatalf("ListTaskEvents returned error: %v", err)
	}
	if len(events) != 2 || events[1].Type != "claimed" || events[1].FromStatus != "open" || events[1].ToStatus != "in_progress" {
		t.Fatalf("unexpected claimed events: %+v", events)
	}

	if _, err := store.ClaimTask(ctx, project.ID, blocker.ID); !errors.Is(err, ErrTaskNotReady) {
		t.Fatalf("expected already claimed task to be not ready, got %v", err)
	}
	if _, err := store.ClaimTask(ctx, project.ID, blocked.ID); !errors.Is(err, ErrTaskNotReady) {
		t.Fatalf("expected blocked task to be not ready, got %v", err)
	}

	if _, err := store.CompleteTaskWithOptions(ctx, CompleteTaskInput{
		ID:             blocker.ID,
		Mode:           CompletionOverride,
		Note:           "Dependency blocker resolved.",
		OverrideReason: "Claim task fixture override.",
	}); err != nil {
		t.Fatalf("CompleteTaskWithOptions override returned error: %v", err)
	}
	claimedBlocked, err := store.ClaimTask(ctx, project.ID, blocked.ID)
	if err != nil {
		t.Fatalf("ClaimTask blocked after blocker done returned error: %v", err)
	}
	if claimedBlocked.ID != blocked.ID || claimedBlocked.Status != "in_progress" {
		t.Fatalf("expected blocked task to be claimed after blocker done, got %+v", claimedBlocked)
	}

	if _, err := store.ClaimNextReadyTask(ctx, project.ID); !errors.Is(err, ErrNoReadyTask) {
		t.Fatalf("expected no ready task, got %v", err)
	}
}

func TestRunLifecycle(t *testing.T) {
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
	task, err := store.CreateTask(ctx, CreateTaskInput{
		ProjectID: project.ID,
		Title:     "Run task",
	})
	if err != nil {
		t.Fatalf("CreateTask returned error: %v", err)
	}

	run, err := store.CreateRun(ctx, CreateRunInput{
		TaskID:                 task.ID,
		Status:                 "in_progress",
		HandoffContractVersion: "tok.handoff.v0",
		RetrievalLimit:         3,
		BaseBranch:             "main",
		BaseHead:               "abc1234",
	})
	if err != nil {
		t.Fatalf("CreateRun returned error: %v", err)
	}
	if run.ID == 0 || run.TaskID != task.ID || run.Status != "in_progress" || run.StartedAt == "" {
		t.Fatalf("unexpected created run: %+v", run)
	}
	if run.HandoffContractVersion != "tok.handoff.v0" || run.RetrievalLimit != 3 || run.BaseBranch != "main" || run.BaseHead != "abc1234" {
		t.Fatalf("unexpected run snapshot fields: %+v", run)
	}
	if run.FinishedAt != "" || run.ResultSummary != "" {
		t.Fatalf("new run should not be finished: %+v", run)
	}

	got, err := store.GetRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("GetRun returned error: %v", err)
	}
	if got.ID != run.ID || got.TaskID != task.ID {
		t.Fatalf("unexpected run from GetRun: %+v", got)
	}

	_, err = store.AddRunArtifact(ctx, AddRunArtifactInput{
		RunID:    run.ID,
		Kind:     "validation",
		Metadata: `{"status":"passed","command":"go test ./..."}`,
	})
	if err != nil {
		t.Fatalf("AddRunArtifact validation returned error: %v", err)
	}
	finished, err := store.FinishRun(ctx, FinishRunInput{
		ID:            run.ID,
		Status:        "succeeded",
		ResultSummary: "Implemented and tests pass.",
	})
	if err != nil {
		t.Fatalf("FinishRun returned error: %v", err)
	}
	if finished.Status != "succeeded" || finished.FinishedAt == "" || finished.ResultSummary != "Implemented and tests pass." {
		t.Fatalf("unexpected finished run: %+v", finished)
	}

	_, err = store.FinishRun(ctx, FinishRunInput{
		ID:            run.ID,
		Status:        "failed",
		ResultSummary: "Should not change.",
	})
	if !errors.Is(err, ErrInvalidRunTransition) {
		t.Fatalf("expected invalid run transition, got %v", err)
	}
}

func TestListRunsFiltersByProjectTaskAndStatus(t *testing.T) {
	ctx := context.Background()
	store := openInitializedTestStore(t)

	project, err := store.CreateProject(ctx, CreateProjectInput{
		Name:        "tok",
		DisplayName: "TOK",
		Path:        "/tmp/tok",
	})
	if err != nil {
		t.Fatalf("CreateProject tok returned error: %v", err)
	}
	otherProject, err := store.CreateProject(ctx, CreateProjectInput{
		Name:        "other",
		DisplayName: "Other",
		Path:        "/tmp/other",
	})
	if err != nil {
		t.Fatalf("CreateProject other returned error: %v", err)
	}
	firstTask, err := store.CreateTask(ctx, CreateTaskInput{ProjectID: project.ID, Title: "First"})
	if err != nil {
		t.Fatalf("CreateTask first returned error: %v", err)
	}
	secondTask, err := store.CreateTask(ctx, CreateTaskInput{ProjectID: project.ID, Title: "Second"})
	if err != nil {
		t.Fatalf("CreateTask second returned error: %v", err)
	}
	otherTask, err := store.CreateTask(ctx, CreateTaskInput{ProjectID: otherProject.ID, Title: "Other"})
	if err != nil {
		t.Fatalf("CreateTask other returned error: %v", err)
	}

	firstRun, err := store.CreateRun(ctx, CreateRunInput{
		TaskID:                 firstTask.ID,
		Status:                 "in_progress",
		HandoffContractVersion: "tok.handoff.v0",
	})
	if err != nil {
		t.Fatalf("CreateRun first returned error: %v", err)
	}
	secondRun, err := store.CreateRun(ctx, CreateRunInput{
		TaskID:                 secondTask.ID,
		Status:                 "in_progress",
		HandoffContractVersion: "tok.handoff.v0",
	})
	if err != nil {
		t.Fatalf("CreateRun second returned error: %v", err)
	}
	if _, err := store.FinishRun(ctx, FinishRunInput{ID: secondRun.ID, Status: "failed", ResultSummary: "Tests failed."}); err != nil {
		t.Fatalf("FinishRun second returned error: %v", err)
	}
	otherRun, err := store.CreateRun(ctx, CreateRunInput{
		TaskID:                 otherTask.ID,
		Status:                 "in_progress",
		HandoffContractVersion: "tok.handoff.v0",
	})
	if err != nil {
		t.Fatalf("CreateRun other returned error: %v", err)
	}

	projectRuns, err := store.ListRuns(ctx, ListRunsOptions{ProjectID: project.ID})
	if err != nil {
		t.Fatalf("ListRuns project returned error: %v", err)
	}
	if len(projectRuns) != 2 || projectRuns[0].ID != secondRun.ID || projectRuns[1].ID != firstRun.ID {
		t.Fatalf("unexpected project runs: %+v", projectRuns)
	}

	taskRuns, err := store.ListRuns(ctx, ListRunsOptions{TaskID: firstTask.ID})
	if err != nil {
		t.Fatalf("ListRuns task returned error: %v", err)
	}
	if len(taskRuns) != 1 || taskRuns[0].ID != firstRun.ID {
		t.Fatalf("unexpected task runs: %+v", taskRuns)
	}

	activeRuns, err := store.ListRuns(ctx, ListRunsOptions{Status: "in_progress"})
	if err != nil {
		t.Fatalf("ListRuns status returned error: %v", err)
	}
	if len(activeRuns) != 2 || activeRuns[0].ID != otherRun.ID || activeRuns[1].ID != firstRun.ID {
		t.Fatalf("unexpected active runs: %+v", activeRuns)
	}

	_, err = store.ListRuns(ctx, ListRunsOptions{Status: "paused"})
	if err == nil || !strings.Contains(err.Error(), `invalid run status "paused"`) {
		t.Fatalf("expected invalid run status error, got %v", err)
	}
}

func TestRunLeaseHeartbeatAndRecovery(t *testing.T) {
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
	task, err := store.CreateTask(ctx, CreateTaskInput{ProjectID: project.ID, Title: "Lease task"})
	if err != nil {
		t.Fatalf("CreateTask returned error: %v", err)
	}

	run, err := store.CreateRun(ctx, CreateRunInput{
		TaskID:                 task.ID,
		Status:                 "in_progress",
		HandoffContractVersion: "tok.handoff.v0",
		LeaseOwner:             "agent/one",
		HeartbeatAt:            "2026-07-25T12:00:00.000Z",
		ExpiresAt:              "2026-07-25T12:15:00.000Z",
	})
	if err != nil {
		t.Fatalf("CreateRun returned error: %v", err)
	}
	if run.LeaseOwner != "agent/one" || run.HeartbeatAt != "2026-07-25T12:00:00.000Z" || run.ExpiresAt != "2026-07-25T12:15:00.000Z" {
		t.Fatalf("unexpected run lease fields: %+v", run)
	}

	_, err = store.CreateRun(ctx, CreateRunInput{
		TaskID:                 task.ID,
		Status:                 "in_progress",
		HandoffContractVersion: "tok.handoff.v0",
	})
	if !errors.Is(err, ErrActiveRunExists) {
		t.Fatalf("expected active run guard, got %v", err)
	}

	overrideRun, err := store.CreateRun(ctx, CreateRunInput{
		TaskID:                 task.ID,
		Status:                 "in_progress",
		HandoffContractVersion: "tok.handoff.v0",
		AllowActive:            true,
	})
	if err != nil {
		t.Fatalf("CreateRun allow active returned error: %v", err)
	}
	if overrideRun.ID == run.ID {
		t.Fatalf("expected distinct override run, got %+v", overrideRun)
	}
	if _, err := store.FinishRun(ctx, FinishRunInput{ID: overrideRun.ID, Status: "failed", ResultSummary: "Override failed."}); err != nil {
		t.Fatalf("FinishRun override returned error: %v", err)
	}

	heartbeat, err := store.HeartbeatRun(ctx, HeartbeatRunInput{
		ID:        run.ID,
		Owner:     "agent/two",
		Now:       "2026-07-25T12:10:00.000Z",
		ExpiresAt: "2026-07-25T12:25:00.000Z",
	})
	if err != nil {
		t.Fatalf("HeartbeatRun returned error: %v", err)
	}
	if heartbeat.LeaseOwner != "agent/two" || heartbeat.HeartbeatAt != "2026-07-25T12:10:00.000Z" || heartbeat.ExpiresAt != "2026-07-25T12:25:00.000Z" {
		t.Fatalf("unexpected heartbeat run: %+v", heartbeat)
	}

	notStale, err := store.RecoverStaleRuns(ctx, RecoverStaleRunsInput{
		Now:           "2026-07-25T12:24:59.000Z",
		ResultSummary: "Recovered stale run.",
	})
	if err != nil {
		t.Fatalf("RecoverStaleRuns before expiry returned error: %v", err)
	}
	if len(notStale) != 0 {
		t.Fatalf("expected no stale runs before expiry, got %+v", notStale)
	}

	recovered, err := store.RecoverStaleRuns(ctx, RecoverStaleRunsInput{
		Now:           "2026-07-25T12:25:01.000Z",
		ResultSummary: "Recovered stale run.",
	})
	if err != nil {
		t.Fatalf("RecoverStaleRuns returned error: %v", err)
	}
	if len(recovered) != 1 || recovered[0].ID != run.ID || recovered[0].Status != "cancelled" || recovered[0].ResultSummary != "Recovered stale run." {
		t.Fatalf("unexpected recovered runs: %+v", recovered)
	}

	_, err = store.HeartbeatRun(ctx, HeartbeatRunInput{
		ID:        run.ID,
		Owner:     "agent/three",
		Now:       "2026-07-25T12:26:00.000Z",
		ExpiresAt: "2026-07-25T12:41:00.000Z",
	})
	if !errors.Is(err, ErrInvalidRunTransition) {
		t.Fatalf("expected heartbeat terminal transition error, got %v", err)
	}

	terminalOnly, err := store.RecoverStaleRuns(ctx, RecoverStaleRunsInput{
		Now:           "2026-07-25T13:00:00.000Z",
		ResultSummary: "Should not change terminal runs.",
	})
	if err != nil {
		t.Fatalf("RecoverStaleRuns terminal-only returned error: %v", err)
	}
	if len(terminalOnly) != 0 {
		t.Fatalf("expected terminal runs to be ignored, got %+v", terminalOnly)
	}
}

func TestRunLifecycleValidation(t *testing.T) {
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
	task, err := store.CreateTask(ctx, CreateTaskInput{
		ProjectID: project.ID,
		Title:     "Run validation",
	})
	if err != nil {
		t.Fatalf("CreateTask returned error: %v", err)
	}

	created, err := store.CreateRun(ctx, CreateRunInput{
		TaskID:                 task.ID,
		HandoffContractVersion: "tok.handoff.v0",
	})
	if err != nil {
		t.Fatalf("CreateRun default status returned error: %v", err)
	}
	if created.Status != "created" {
		t.Fatalf("expected default created status, got %+v", created)
	}
	if created.RetrievalLimit != 5 {
		t.Fatalf("expected default retrieval limit, got %+v", created)
	}

	_, err = store.CreateRun(ctx, CreateRunInput{
		TaskID:                 task.ID,
		Status:                 "paused",
		HandoffContractVersion: "tok.handoff.v0",
	})
	if err == nil || !strings.Contains(err.Error(), `invalid run status "paused"`) {
		t.Fatalf("expected invalid run status error, got %v", err)
	}

	_, err = store.CreateRun(ctx, CreateRunInput{
		TaskID:                 task.ID,
		Status:                 "succeeded",
		HandoffContractVersion: "tok.handoff.v0",
	})
	if !errors.Is(err, ErrInvalidRunTransition) {
		t.Fatalf("expected invalid create terminal transition, got %v", err)
	}

	_, err = store.FinishRun(ctx, FinishRunInput{
		ID:            created.ID,
		Status:        "in_progress",
		ResultSummary: "Still running.",
	})
	if err == nil || !strings.Contains(err.Error(), `invalid terminal run status "in_progress"`) {
		t.Fatalf("expected invalid terminal status error, got %v", err)
	}

	_, err = store.FinishRun(ctx, FinishRunInput{
		ID:     created.ID,
		Status: "failed",
	})
	if !errors.Is(err, ErrRunResultSummaryEmpty) {
		t.Fatalf("expected empty run summary error, got %v", err)
	}
}

func TestFinishRunRecordsOverrideAuditArtifact(t *testing.T) {
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
	task, err := store.CreateTask(ctx, CreateTaskInput{
		ProjectID: project.ID,
		Title:     "Validated run",
	})
	if err != nil {
		t.Fatalf("CreateTask returned error: %v", err)
	}
	run, err := store.CreateRun(ctx, CreateRunInput{
		TaskID:                 task.ID,
		Status:                 "in_progress",
		HandoffContractVersion: "tok.handoff.v0",
	})
	if err != nil {
		t.Fatalf("CreateRun returned error: %v", err)
	}

	finished, err := store.FinishRun(ctx, FinishRunInput{
		ID:            run.ID,
		Status:        "succeeded",
		ResultSummary: "Storage records terminal state.",
	})
	if err != nil {
		t.Fatalf("FinishRun returned error: %v", err)
	}
	if finished.Status != "succeeded" {
		t.Fatalf("unexpected finished run: %+v", finished)
	}

	overrideTask, err := store.CreateTask(ctx, CreateTaskInput{
		ProjectID: project.ID,
		Title:     "Override run",
	})
	if err != nil {
		t.Fatalf("CreateTask override returned error: %v", err)
	}
	overrideRun, err := store.CreateRun(ctx, CreateRunInput{
		TaskID:                 overrideTask.ID,
		Status:                 "in_progress",
		HandoffContractVersion: "tok.handoff.v0",
	})
	if err != nil {
		t.Fatalf("CreateRun override returned error: %v", err)
	}
	overridden, err := store.FinishRun(ctx, FinishRunInput{
		ID:             overrideRun.ID,
		Status:         "succeeded",
		ResultSummary:  "Explicit override.",
		OverrideReason: "Manual operator override for characterization.",
	})
	if err != nil {
		t.Fatalf("FinishRun override returned error: %v", err)
	}
	if overridden.Status != "succeeded" {
		t.Fatalf("unexpected override run: %+v", overridden)
	}
	artifacts, err := store.ListRunArtifacts(ctx, overrideRun.ID)
	if err != nil {
		t.Fatalf("ListRunArtifacts override returned error: %v", err)
	}
	if len(artifacts) != 1 || artifacts[0].Kind != "log" || !strings.Contains(artifacts[0].Metadata, "Manual operator override") {
		t.Fatalf("expected override audit artifact, got %+v", artifacts)
	}
}

func TestCompleteTaskRejectsActiveRun(t *testing.T) {
	ctx := context.Background()
	store := openInitializedTestStore(t)

	_, task := createStorageProjectTask(t, ctx, store, "Completion guard")
	if _, err := store.UpdateTaskStatus(ctx, task.ID, "in_progress"); err != nil {
		t.Fatalf("UpdateTaskStatus returned error: %v", err)
	}
	run, err := store.CreateRun(ctx, CreateRunInput{
		TaskID:                 task.ID,
		Status:                 "in_progress",
		HandoffContractVersion: "tok.handoff.v0",
	})
	if err != nil {
		t.Fatalf("CreateRun returned error: %v", err)
	}

	_, err = store.CompleteTaskWithOptions(ctx, CompleteTaskInput{
		ID:             task.ID,
		Mode:           CompletionOverride,
		Note:           "Done too early.",
		OverrideReason: "Active run guard test.",
	})
	if !errors.Is(err, ErrActiveRunExists) {
		t.Fatalf("expected active run completion guard, got %v", err)
	}
	_, err = store.UpdateTaskStatus(ctx, task.ID, "done")
	if !errors.Is(err, ErrTaskStatusDoneUnsupported) {
		t.Fatalf("expected direct done status rejection, got %v", err)
	}

	if _, err := store.FinishRun(ctx, FinishRunInput{
		ID:            run.ID,
		Status:        "failed",
		ResultSummary: "Run failed.",
	}); err != nil {
		t.Fatalf("FinishRun failed returned error: %v", err)
	}
	_, err = store.CompleteTaskWithOptions(ctx, CompleteTaskInput{
		ID:             task.ID,
		Mode:           CompletionOverride,
		Note:           "Closed after terminal run.",
		OverrideReason: "Terminal run guard test override.",
	})
	if err != nil {
		t.Fatalf("CompleteTaskWithOptions after terminal run returned error: %v", err)
	}
}

func TestCompleteTaskRequiresExplicitModeAndEvidenceOrOverride(t *testing.T) {
	ctx := context.Background()
	store := openInitializedTestStore(t)

	_, task := createStorageProjectTask(t, ctx, store, "Missing evidence completion")
	if _, err := store.UpdateTaskStatus(ctx, task.ID, "in_progress"); err != nil {
		t.Fatalf("UpdateTaskStatus returned error: %v", err)
	}

	_, err := store.CompleteTaskWithOptions(ctx, CompleteTaskInput{
		ID:   task.ID,
		Note: "Completed without explicit mode.",
	})
	if !errors.Is(err, ErrInvalidTaskCompletionMode) {
		t.Fatalf("expected invalid completion mode error, got %v", err)
	}

	_, err = store.CompleteTaskWithOptions(ctx, CompleteTaskInput{
		ID:   task.ID,
		Mode: CompletionOverride,
		Note: "Completed without override reason.",
	})
	if !errors.Is(err, ErrTaskCompletionOverrideRequired) {
		t.Fatalf("expected override reason required error, got %v", err)
	}

	_, err = store.CompleteTaskWithOptions(ctx, CompleteTaskInput{
		ID:   task.ID,
		Mode: CompletionValidated,
		Note: "Completed without run evidence.",
	})
	if !errors.Is(err, ErrTaskCompletionEvidenceRequired) {
		t.Fatalf("expected evidence-required completion error, got %v", err)
	}
}

func TestCompleteTaskAllowsTerminalRunWithoutEvidencePolicy(t *testing.T) {
	ctx := context.Background()
	store := openInitializedTestStore(t)

	_, task := createStorageProjectTask(t, ctx, store, "Failed run completion")
	if _, err := store.UpdateTaskStatus(ctx, task.ID, "in_progress"); err != nil {
		t.Fatalf("UpdateTaskStatus returned error: %v", err)
	}
	run, err := store.CreateRun(ctx, CreateRunInput{
		TaskID:                 task.ID,
		Status:                 "in_progress",
		HandoffContractVersion: "tok.handoff.v0",
	})
	if err != nil {
		t.Fatalf("CreateRun returned error: %v", err)
	}
	if _, err := store.FinishRun(ctx, FinishRunInput{
		ID:            run.ID,
		Status:        "failed",
		ResultSummary: "Validation failed.",
	}); err != nil {
		t.Fatalf("FinishRun failed returned error: %v", err)
	}

	_, err = store.CompleteTaskWithOptions(ctx, CompleteTaskInput{
		ID:   task.ID,
		Mode: CompletionValidated,
		Note: "Completed after failed run.",
	})
	if !errors.Is(err, ErrTaskCompletionEvidenceRequired) {
		t.Fatalf("expected evidence-required completion error, got %v", err)
	}

	finished, err := store.GetRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("GetRun returned error: %v", err)
	}
	if finished.Status != "failed" {
		t.Fatalf("unexpected evidence run status: %+v", finished)
	}
}

func TestCompleteTaskWithOptionsStoresCompletionEvidenceRunAndArtifact(t *testing.T) {
	ctx := context.Background()
	store := openInitializedTestStore(t)

	_, task := createStorageProjectTask(t, ctx, store, "Validated completion")
	if _, err := store.UpdateTaskStatus(ctx, task.ID, "in_progress"); err != nil {
		t.Fatalf("UpdateTaskStatus returned error: %v", err)
	}
	run, err := store.CreateRun(ctx, CreateRunInput{
		TaskID:                 task.ID,
		Status:                 "in_progress",
		HandoffContractVersion: "tok.handoff.v0",
	})
	if err != nil {
		t.Fatalf("CreateRun returned error: %v", err)
	}
	validationArtifact, err := store.AddRunArtifact(ctx, AddRunArtifactInput{
		RunID:    run.ID,
		Kind:     "validation",
		Metadata: `{"status":"passed","command":"go test ./..."}`,
	})
	if err != nil {
		t.Fatalf("AddRunArtifact returned error: %v", err)
	}
	if _, err := store.FinishRun(ctx, FinishRunInput{
		ID:            run.ID,
		Status:        "succeeded",
		ResultSummary: "Validation passed.",
	}); err != nil {
		t.Fatalf("FinishRun returned error: %v", err)
	}

	done, err := store.CompleteTaskWithOptions(ctx, CompleteTaskInput{
		ID:   task.ID,
		Mode: CompletionValidated,
		Note: "Implemented and tests pass.",
	})
	if err != nil {
		t.Fatalf("CompleteTaskWithOptions returned error: %v", err)
	}
	if done.Status != "done" {
		t.Fatalf("expected done task, got %+v", done)
	}

	events, err := store.ListTaskEvents(ctx, task.ID)
	if err != nil {
		t.Fatalf("ListTaskEvents returned error: %v", err)
	}
	last := events[len(events)-1]
	if last.Type != "completed" || last.Body != "Implemented and tests pass." || last.FromStatus != "in_progress" || last.ToStatus != "done" {
		t.Fatalf("unexpected completion event: %+v", last)
	}
	if last.EvidenceRunID != run.ID {
		t.Fatalf("expected completion evidence run id %d, got %d", run.ID, last.EvidenceRunID)
	}
	if last.EvidenceArtifactID != validationArtifact.ID {
		t.Fatalf("expected completion evidence artifact id %d, got %d", validationArtifact.ID, last.EvidenceArtifactID)
	}
}

func TestCompleteTaskWithOptionsValidationCheckIsAtomic(t *testing.T) {
	ctx := context.Background()
	store := openInitializedTestStore(t)

	_, task := createStorageProjectTask(t, ctx, store, "Missing validation atomicity")
	if _, err := store.UpdateTaskStatus(ctx, task.ID, "in_progress"); err != nil {
		t.Fatalf("UpdateTaskStatus returned error: %v", err)
	}
	run, err := store.CreateRun(ctx, CreateRunInput{
		TaskID:                 task.ID,
		Status:                 "in_progress",
		HandoffContractVersion: "tok.handoff.v0",
	})
	if err != nil {
		t.Fatalf("CreateRun returned error: %v", err)
	}
	if _, err := store.FinishRun(ctx, FinishRunInput{
		ID:            run.ID,
		Status:        "failed",
		ResultSummary: "Validation failed.",
	}); err != nil {
		t.Fatalf("FinishRun failed returned error: %v", err)
	}

	_, err = store.CompleteTaskWithOptions(ctx, CompleteTaskInput{
		ID:   task.ID,
		Mode: CompletionValidated,
		Note: "Should stay in progress",
	})
	if !errors.Is(err, ErrTaskCompletionEvidenceRequired) {
		t.Fatalf("expected evidence-required completion error, got %v", err)
	}

	taskInProgress, err := store.GetTask(ctx, task.ID)
	if err != nil {
		t.Fatalf("GetTask returned error: %v", err)
	}
	if taskInProgress.Status != "in_progress" {
		t.Fatalf("expected task status in_progress, got %s", taskInProgress.Status)
	}
	events, err := store.ListTaskEvents(ctx, task.ID)
	if err != nil {
		t.Fatalf("ListTaskEvents returned error: %v", err)
	}
	for _, event := range events {
		if event.Type == "completed" {
			t.Fatalf("expected no completion event when evidence validation fails, got %+v", event)
		}
	}
}

func TestRunArtifacts(t *testing.T) {
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
	task, err := store.CreateTask(ctx, CreateTaskInput{
		ProjectID: project.ID,
		Title:     "Artifact task",
	})
	if err != nil {
		t.Fatalf("CreateTask returned error: %v", err)
	}
	run, err := store.CreateRun(ctx, CreateRunInput{
		TaskID:                 task.ID,
		Status:                 "in_progress",
		HandoffContractVersion: "tok.handoff.v0",
	})
	if err != nil {
		t.Fatalf("CreateRun returned error: %v", err)
	}

	handoff, err := store.AddRunArtifact(ctx, AddRunArtifactInput{
		RunID:       run.ID,
		Kind:        "handoff",
		Path:        "/tmp/context.md",
		ContentHash: "sha256:abc123",
		Metadata:    `{"format":"text"}`,
	})
	if err != nil {
		t.Fatalf("AddRunArtifact handoff returned error: %v", err)
	}
	if handoff.ID == 0 || handoff.RunID != run.ID || handoff.Kind != "handoff" || handoff.CreatedAt == "" {
		t.Fatalf("unexpected handoff artifact: %+v", handoff)
	}

	note, err := store.AddRunArtifact(ctx, AddRunArtifactInput{
		RunID: run.ID,
		Kind:  "note",
	})
	if err != nil {
		t.Fatalf("AddRunArtifact note returned error: %v", err)
	}
	if note.Metadata != "{}" {
		t.Fatalf("expected default artifact metadata, got %+v", note)
	}
	stdout, err := store.AddRunArtifact(ctx, AddRunArtifactInput{
		RunID:       run.ID,
		Kind:        "stdout",
		Path:        "/tmp/artifacts/run-1/stdout.txt",
		ContentHash: "sha256:def456",
		SizeBytes:   12,
		Truncated:   true,
		Metadata:    `{"format":"text","limit_bytes":12}`,
	})
	if err != nil {
		t.Fatalf("AddRunArtifact stdout returned error: %v", err)
	}
	if stdout.SizeBytes != 12 || !stdout.Truncated {
		t.Fatalf("unexpected stdout artifact file metadata: %+v", stdout)
	}

	artifacts, err := store.ListRunArtifacts(ctx, run.ID)
	if err != nil {
		t.Fatalf("ListRunArtifacts returned error: %v", err)
	}
	if len(artifacts) != 3 || artifacts[0].ID != handoff.ID || artifacts[1].ID != note.ID || artifacts[2].ID != stdout.ID {
		t.Fatalf("unexpected artifacts: %+v", artifacts)
	}
}

func TestRunArtifactValidation(t *testing.T) {
	ctx := context.Background()
	store := openInitializedTestStore(t)

	_, err := store.AddRunArtifact(ctx, AddRunArtifactInput{
		RunID: 1,
		Kind:  "binary",
	})
	if !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("expected missing run error before kind validation, got %v", err)
	}

	project, err := store.CreateProject(ctx, CreateProjectInput{
		Name:        "tok",
		DisplayName: "TOK",
		Path:        "/tmp/tok",
	})
	if err != nil {
		t.Fatalf("CreateProject returned error: %v", err)
	}
	task, err := store.CreateTask(ctx, CreateTaskInput{
		ProjectID: project.ID,
		Title:     "Artifact validation",
	})
	if err != nil {
		t.Fatalf("CreateTask returned error: %v", err)
	}
	run, err := store.CreateRun(ctx, CreateRunInput{
		TaskID:                 task.ID,
		HandoffContractVersion: "tok.handoff.v0",
	})
	if err != nil {
		t.Fatalf("CreateRun returned error: %v", err)
	}

	_, err = store.AddRunArtifact(ctx, AddRunArtifactInput{
		RunID: run.ID,
		Kind:  "binary",
	})
	if err == nil || !strings.Contains(err.Error(), `invalid run artifact kind "binary"`) {
		t.Fatalf("expected invalid kind error, got %v", err)
	}

	_, err = store.AddRunArtifact(ctx, AddRunArtifactInput{
		RunID:     run.ID,
		Kind:      "stdout",
		SizeBytes: -1,
	})
	if err == nil || !strings.Contains(err.Error(), "size bytes cannot be negative") {
		t.Fatalf("expected invalid size error, got %v", err)
	}
}

func TestCompleteTaskRequiresInProgressAndRecordsCompletionEvent(t *testing.T) {
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

	task, err := store.CreateTask(ctx, CreateTaskInput{
		ProjectID: project.ID,
		Title:     "Complete me",
	})
	if err != nil {
		t.Fatalf("CreateTask returned error: %v", err)
	}

	if _, err := store.CompleteTaskWithOptions(ctx, CompleteTaskInput{
		ID:   task.ID,
		Mode: CompletionValidated,
		Note: "too early",
	}); !errors.Is(err, ErrInvalidTaskTransition) {
		t.Fatalf("expected invalid transition for open task, got %v", err)
	}

	claimed, err := store.ClaimTask(ctx, project.ID, task.ID)
	if err != nil {
		t.Fatalf("ClaimTask returned error: %v", err)
	}
	if claimed.Status != "in_progress" {
		t.Fatalf("expected claimed task to be in_progress, got %+v", claimed)
	}
	run, err := store.CreateRun(ctx, CreateRunInput{
		TaskID:                 task.ID,
		Status:                 "in_progress",
		HandoffContractVersion: "tok.handoff.v0",
	})
	if err != nil {
		t.Fatalf("CreateRun returned error: %v", err)
	}
	validationArtifact, err := store.AddRunArtifact(ctx, AddRunArtifactInput{
		RunID:    run.ID,
		Kind:     "validation",
		Metadata: `{"status":"passed","command":"go test ./..."}`,
	})
	if err != nil {
		t.Fatalf("AddRunArtifact returned error: %v", err)
	}
	if _, err := store.FinishRun(ctx, FinishRunInput{
		ID:            run.ID,
		Status:        "succeeded",
		ResultSummary: "Validation passed.",
	}); err != nil {
		t.Fatalf("FinishRun returned error: %v", err)
	}

	done, err := store.CompleteTaskWithOptions(ctx, CompleteTaskInput{
		ID:   task.ID,
		Mode: CompletionValidated,
		Note: "Implemented and tests pass.",
	})
	if err != nil {
		t.Fatalf("CompleteTask returned error: %v", err)
	}
	if done.Status != "done" {
		t.Fatalf("expected done task, got %+v", done)
	}

	events, err := store.ListTaskEvents(ctx, task.ID)
	if err != nil {
		t.Fatalf("ListTaskEvents returned error: %v", err)
	}
	last := events[len(events)-1]
	if last.Type != "completed" || last.Body != "Implemented and tests pass." || last.FromStatus != "in_progress" || last.ToStatus != "done" {
		t.Fatalf("unexpected completion event: %+v", last)
	}
	if last.EvidenceRunID != run.ID {
		t.Fatalf("expected completion evidence run id %d, got %d", run.ID, last.EvidenceRunID)
	}
	if last.EvidenceArtifactID != validationArtifact.ID {
		t.Fatalf("expected completion evidence artifact id %d, got %d", validationArtifact.ID, last.EvidenceArtifactID)
	}

	if _, err := store.CompleteTaskWithOptions(ctx, CompleteTaskInput{
		ID:   task.ID,
		Mode: CompletionValidated,
		Note: "again",
	}); !errors.Is(err, ErrInvalidTaskTransition) {
		t.Fatalf("expected invalid transition for done task, got %v", err)
	}
	if _, err := store.CompleteTaskWithOptions(ctx, CompleteTaskInput{
		ID:   task.ID,
		Mode: CompletionValidated,
		Note: " ",
	}); !errors.Is(err, ErrTaskCompletionNoteEmpty) {
		t.Fatalf("expected empty completion note error, got %v", err)
	}
}

func TestUpdateTaskStatusByActorRejectsDoneToInProgressOrOpenOrBlocked(t *testing.T) {
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
	task, err := store.CreateTask(ctx, CreateTaskInput{
		ProjectID: project.ID,
		Title:     "Reopen guard",
	})
	if err != nil {
		t.Fatalf("CreateTask returned error: %v", err)
	}

	if _, err := store.ClaimTask(ctx, project.ID, task.ID); err != nil {
		t.Fatalf("ClaimTask returned error: %v", err)
	}
	done, err := store.CompleteTaskWithOptions(ctx, CompleteTaskInput{
		ID:             task.ID,
		Mode:           CompletionOverride,
		Note:           "Completed.",
		OverrideReason: "Task status policy.",
	})
	if err != nil {
		t.Fatalf("CompleteTaskWithOptions override returned error: %v", err)
	}
	if done.Status != "done" {
		t.Fatalf("expected done task, got %+v", done)
	}

	if _, err := store.UpdateTaskStatusByActor(ctx, task.ID, "open", ActorRef{}); !errors.Is(err, ErrInvalidTaskTransition) {
		t.Fatalf("expected done->open invalid transition error, got %v", err)
	}
	if _, err := store.UpdateTaskStatusByActor(ctx, task.ID, "in_progress", ActorRef{}); !errors.Is(err, ErrInvalidTaskTransition) {
		t.Fatalf("expected done->in_progress invalid transition error, got %v", err)
	}
	if _, err := store.UpdateTaskStatusByActor(ctx, task.ID, "blocked", ActorRef{}); !errors.Is(err, ErrInvalidTaskTransition) {
		t.Fatalf("expected done->blocked invalid transition error, got %v", err)
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

func createV021MigrationFixture(t *testing.T, ctx context.Context) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), DatabaseFileName)
	store, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("Open legacy fixture store returned error: %v", err)
	}

	if err := ensureMigrationsTable(ctx, store.db); err != nil {
		t.Fatalf("ensure legacy migrations table: %v", err)
	}
	migrations, err := loadMigrations()
	if err != nil {
		t.Fatalf("load embedded migrations: %v", err)
	}

	applied := 0
	for _, migration := range migrations {
		if migration.version > 13 {
			continue
		}
		if err := applyMigration(ctx, store.db, migration); err != nil {
			t.Fatalf("apply legacy migration %d: %v", migration.version, err)
		}
		applied++
	}
	if applied != 13 {
		t.Fatalf("expected to apply 13 legacy migrations, applied %d", applied)
	}

	if _, err := store.db.ExecContext(ctx, `
		INSERT INTO projects (id, name, display_name, path)
		VALUES (1, 'legacy', 'Legacy', '/tmp/legacy')
	`); err != nil {
		t.Fatalf("insert legacy project: %v", err)
	}
	if _, err := store.db.ExecContext(ctx, `
		INSERT INTO tasks (id, project_id, status, title, description, acceptance_criteria, notes)
		VALUES
			(1, 1, 'done', 'Legacy status_changed completion', '', '', ''),
			(2, 1, 'done', 'Legacy override completion', '', '', '')
	`); err != nil {
		t.Fatalf("insert legacy tasks: %v", err)
	}
	if _, err := store.db.ExecContext(ctx, `
		INSERT INTO task_events (task_id, type, body, from_status, to_status, actor_id, actor_kind, actor_name)
		VALUES
			(1, 'created', 'Legacy status_changed completion', '', 'open', 0, '', ''),
			(1, 'status_changed', '', 'open', 'in_progress', 0, '', ''),
			(1, 'status_changed', '', 'in_progress', 'done', 0, '', ''),
			(2, 'created', 'Legacy override completion', '', 'open', 0, '', ''),
			(2, 'status_changed', '', 'open', 'in_progress', 0, '', ''),
			(2, 'completed', 'legacy completion note', 'in_progress', 'done', 0, '', ''),
			(2, 'completion_override', 'legacy override reason', 'in_progress', 'done', 0, '', '')
	`); err != nil {
		t.Fatalf("insert legacy task events: %v", err)
	}

	if err := store.Close(); err != nil {
		t.Fatalf("Close legacy fixture store returned error: %v", err)
	}
	return path
}

func copyFile(t *testing.T, src, dst string) {
	t.Helper()

	data, err := os.ReadFile(src)
	if err != nil {
		t.Fatalf("read fixture source %s: %v", src, err)
	}
	if err := os.WriteFile(dst, data, 0o600); err != nil {
		t.Fatalf("write fixture copy %s: %v", dst, err)
	}
}

func assertCurrentMigrationVersion(t *testing.T, ctx context.Context, store *Store, want int) {
	t.Helper()

	var count int
	var maxVersion int
	if err := store.db.QueryRowContext(ctx, "SELECT COUNT(*), MAX(version) FROM schema_migrations").Scan(&count, &maxVersion); err != nil {
		t.Fatalf("read schema migrations: %v", err)
	}
	if count != want || maxVersion != want {
		t.Fatalf("expected %d migrations through version %d, got count=%d max=%d", want, want, count, maxVersion)
	}
}

func assertColumnExists(t *testing.T, ctx context.Context, store *Store, table, column string) {
	t.Helper()

	rows, err := store.db.QueryContext(ctx, "PRAGMA table_info("+table+")")
	if err != nil {
		t.Fatalf("read columns for %s: %v", table, err)
	}
	defer rows.Close()

	for rows.Next() {
		var cid int
		var name, columnType string
		var notNull int
		var defaultValue any
		var primaryKey int
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &primaryKey); err != nil {
			t.Fatalf("scan column for %s: %v", table, err)
		}
		if name == column {
			return
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate columns for %s: %v", table, err)
	}
	t.Fatalf("expected column %s.%s to exist", table, column)
}

func assertTaskEvents(t *testing.T, got, want []TaskEvent) {
	t.Helper()

	if len(got) != len(want) {
		t.Fatalf("expected %d task events, got %d: %+v", len(want), len(got), got)
	}
	for i := range want {
		if got[i].Type != want[i].Type || got[i].Body != want[i].Body || got[i].FromStatus != want[i].FromStatus || got[i].ToStatus != want[i].ToStatus {
			t.Fatalf("unexpected task event %d: got %+v, want %+v", i, got[i], want[i])
		}
	}
}

func openInitializedTestStore(t *testing.T) *Store {
	t.Helper()

	store := openTestStore(t)
	if err := store.Init(context.Background()); err != nil {
		t.Fatalf("Init returned error: %v", err)
	}

	return store
}

func createStorageProjectTask(t *testing.T, ctx context.Context, store *Store, title string) (Project, Task) {
	t.Helper()

	project, err := store.CreateProject(ctx, CreateProjectInput{
		Name:        "tok",
		DisplayName: "TOK",
		Path:        "/tmp/tok",
	})
	if err != nil {
		t.Fatalf("CreateProject returned error: %v", err)
	}
	task, err := store.CreateTask(ctx, CreateTaskInput{
		ProjectID: project.ID,
		Title:     title,
	})
	if err != nil {
		t.Fatalf("CreateTask returned error: %v", err)
	}
	return project, task
}
