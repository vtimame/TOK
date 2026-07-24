package indexwatch

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"s26.sh/tok/internal/retrieval"
	"s26.sh/tok/internal/storage"
)

func TestWatchIndexesProjectsAddedAfterStartup(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	store := newTestStore(t)
	service, events := newTestWatchService(t, store, Config{
		Debounce:         50 * time.Millisecond,
		RegistryInterval: 50 * time.Millisecond,
	})
	runWatchService(t, ctx, service)

	projectDir := t.TempDir()
	writeWatchFixtureFile(t, projectDir, "main.go", "package main\n\nfunc initialWatchToken() {}\n")
	project, err := store.CreateProject(ctx, storage.CreateProjectInput{
		Name:        "watch-added",
		DisplayName: "Watch Added",
		Path:        projectDir,
	})
	if err != nil {
		t.Fatalf("CreateProject returned error: %v", err)
	}

	waitForWatchEvent(t, events, func(event Event) bool {
		return event.Type == EventIndexCompleted && event.ProjectName == project.Name
	})
	waitForSearchResult(t, retrieval.NewService(store), project, "initialWatchToken")
}

func TestWatchDebouncesFileChangesAndReindexesProject(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	store := newTestStore(t)
	projectDir := t.TempDir()
	writeWatchFixtureFile(t, projectDir, "main.go", "package main\n\nfunc beforeWatchToken() {}\n")
	project, err := store.CreateProject(ctx, storage.CreateProjectInput{
		Name:        "watch-change",
		DisplayName: "Watch Change",
		Path:        projectDir,
	})
	if err != nil {
		t.Fatalf("CreateProject returned error: %v", err)
	}

	service, events := newTestWatchService(t, store, Config{
		Debounce:         50 * time.Millisecond,
		RegistryInterval: time.Hour,
	})
	runWatchService(t, ctx, service)

	waitForWatchEvent(t, events, func(event Event) bool {
		return event.Type == EventIndexCompleted && event.ProjectName == project.Name
	})
	retrievalService := retrieval.NewService(store)
	waitForSearchResult(t, retrievalService, project, "beforeWatchToken")

	writeWatchFixtureFile(t, projectDir, "main.go", "package main\n\nfunc afterWatchToken() {}\n")
	writeWatchFixtureFile(t, projectDir, "extra.go", "package main\n\nfunc extraWatchToken() {}\n")

	waitForWatchEvent(t, events, func(event Event) bool {
		return event.Type == EventIndexCompleted &&
			event.ProjectName == project.Name &&
			event.Summary != nil &&
			event.Summary.IndexedDocuments == 2
	})
	waitForSearchResult(t, retrievalService, project, "afterWatchToken")
	waitForSearchResult(t, retrievalService, project, "extraWatchToken")
}

func TestWatchHonorsStoredIgnorePolicy(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	store := newTestStore(t)
	projectDir := t.TempDir()
	writeWatchFixtureFile(t, projectDir, ".gitignore", "generated/**\n")
	writeWatchFixtureFile(t, projectDir, "keep.go", "package main\n\nconst keptMarker = \"kkk111\"\n")
	writeWatchFixtureFile(t, projectDir, "generated/skip.go", "package main\n\nconst ignoredMarker = \"zzz999\"\n")
	project, err := store.CreateProject(ctx, storage.CreateProjectInput{
		Name:        "watch-ignore",
		DisplayName: "Watch Ignore",
		Path:        projectDir,
	})
	if err != nil {
		t.Fatalf("CreateProject returned error: %v", err)
	}

	service, events := newTestWatchService(t, store, Config{
		Debounce:         50 * time.Millisecond,
		RegistryInterval: time.Hour,
	})
	runWatchService(t, ctx, service)

	waitForWatchEvent(t, events, func(event Event) bool {
		return event.Type == EventIndexCompleted && event.ProjectName == project.Name
	})

	retrievalService := retrieval.NewService(store)
	waitForSearchResult(t, retrievalService, project, "kkk111")
	results, err := retrievalService.Search(ctx, project, "zzz999", 10)
	if err != nil {
		t.Fatalf("Search ignored token returned error: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("ignored file was indexed: %+v", results)
	}
}

func TestWatchMissingPathDoesNotStopRegistryLoop(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	store := newTestStore(t)
	missingPath := filepath.Join(t.TempDir(), "missing")
	project, err := store.CreateProject(ctx, storage.CreateProjectInput{
		Name:        "watch-missing",
		DisplayName: "Watch Missing",
		Path:        missingPath,
	})
	if err != nil {
		t.Fatalf("CreateProject returned error: %v", err)
	}

	service, events := newTestWatchService(t, store, Config{
		Debounce:         50 * time.Millisecond,
		RegistryInterval: 50 * time.Millisecond,
	})
	runWatchService(t, ctx, service)

	waitForWatchEvent(t, events, func(event Event) bool {
		return event.Type == EventPathMissing && event.ProjectName == project.Name
	})

	if err := os.MkdirAll(missingPath, 0o755); err != nil {
		t.Fatalf("MkdirAll returned error: %v", err)
	}
	writeWatchFixtureFile(t, missingPath, "main.go", "package main\n\nfunc restoredWatchToken() {}\n")

	waitForWatchEvent(t, events, func(event Event) bool {
		return event.Type == EventIndexCompleted && event.ProjectName == project.Name
	})
	waitForSearchResult(t, retrieval.NewService(store), project, "restoredWatchToken")
}

func newTestStore(t *testing.T) *storage.Store {
	t.Helper()

	ctx := context.Background()
	store, err := storage.Open(ctx, filepath.Join(t.TempDir(), storage.DatabaseFileName))
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Fatalf("Close returned error: %v", err)
		}
	})
	if err := store.Init(ctx); err != nil {
		t.Fatalf("Init returned error: %v", err)
	}
	return store
}

func newTestWatchService(t *testing.T, store *storage.Store, config Config) (*Service, <-chan Event) {
	t.Helper()

	events := make(chan Event, 256)
	config.Store = store
	config.Indexer = retrieval.NewService(store)
	config.Events = events
	service, err := New(config)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	return service, events
}

func runWatchService(t *testing.T, ctx context.Context, service *Service) {
	t.Helper()

	errs := make(chan error, 1)
	go func() {
		errs <- service.Run(ctx)
	}()
	t.Cleanup(func() {
		select {
		case err := <-errs:
			if err != nil && !errorsIsContextCanceled(err) {
				t.Fatalf("Run returned error: %v", err)
			}
		case <-time.After(100 * time.Millisecond):
		}
	})
}

func waitForWatchEvent(t *testing.T, events <-chan Event, match func(Event) bool) Event {
	t.Helper()

	deadline := time.After(5 * time.Second)
	for {
		select {
		case event := <-events:
			if event.Err != nil && event.Type != EventPathMissing {
				t.Fatalf("unexpected watch event error: %+v", event)
			}
			if match(event) {
				return event
			}
		case <-deadline:
			t.Fatal("timed out waiting for watch event")
		}
	}
}

func waitForSearchResult(t *testing.T, service *retrieval.Service, project storage.Project, query string) {
	t.Helper()

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		results, err := service.Search(context.Background(), project, query, 10)
		if err != nil {
			t.Fatalf("Search returned error: %v", err)
		}
		for _, result := range results {
			if strings.Contains(result.Snippet, query) || strings.Contains(result.Excerpt, query) {
				return
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for search result %q", query)
}

func writeWatchFixtureFile(t *testing.T, root, relPath, content string) {
	t.Helper()

	path := filepath.Join(root, relPath)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll returned error: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}
}

func errorsIsContextCanceled(err error) bool {
	return err == context.Canceled || err == context.DeadlineExceeded
}
