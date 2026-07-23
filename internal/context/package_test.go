package context

import (
	stdctx "context"
	"path/filepath"
	"strings"
	"testing"

	"s26.sh/tok/internal/retrieval"
	"s26.sh/tok/internal/storage"
)

func TestBuilderBuildsDeterministicTextPackage(t *testing.T) {
	ctx := stdctx.Background()
	store := openContextTestStore(t)

	project, err := store.CreateProject(ctx, storage.CreateProjectInput{
		Name:        "tok",
		DisplayName: "TOK",
		Path:        t.TempDir(),
	})
	if err != nil {
		t.Fatalf("CreateProject returned error: %v", err)
	}
	task, err := store.CreateTask(ctx, storage.CreateTaskInput{
		ProjectID:          project.ID,
		Title:              "Refresh token retrieval",
		Description:        "Find refreshToken implementation.",
		AcceptanceCriteria: "- package includes retrieval results",
		Notes:              "Keep output deterministic.",
	})
	if err != nil {
		t.Fatalf("CreateTask returned error: %v", err)
	}
	if _, err := store.ReplaceIndexedDocuments(ctx, project.ID, []storage.IndexedDocumentInput{
		{
			ProjectID:  project.ID,
			Path:       "internal/auth/token.go",
			Provenance: "project_file",
			SizeBytes:  64,
			Content:    "package auth\n\nfunc refreshToken() string {\n\treturn \"value\"\n}\n",
		},
	}); err != nil {
		t.Fatalf("ReplaceIndexedDocuments returned error: %v", err)
	}

	builder := NewBuilder(store, retrieval.NewService(store))
	builder.git = fixedGitInspector{state: GitState{
		Available: true,
		Branch:    "main",
		Head:      "abc1234",
		Status:    []string{"M internal/context/package.go"},
	}}

	pkg, err := builder.Build(ctx, BuildInput{
		Project:        project,
		Task:           task,
		RetrievalLimit: 3,
	})
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}

	text := pkg.RenderText()
	for _, want := range []string{
		"# TOK Context Package",
		"## Project",
		"name: tok",
		"## Task",
		"status: open",
		"title: Refresh token retrieval",
		"acceptance_criteria:",
		"  - package includes retrieval results",
		"## Task Events",
		"type: created",
		"## Retrieval Results",
		"1. path: internal/auth/token.go",
		"provenance: project_file",
		"snippet: func refreshToken() string {",
		"excerpt:",
		"3: func refreshToken() string {",
		"## Repository State",
		"available: true",
		"branch: main",
		"head: abc1234",
		"- M internal/context/package.go",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("package text missing %q:\n%s", want, text)
		}
	}
}

func openContextTestStore(t *testing.T) *storage.Store {
	t.Helper()

	store, err := storage.Open(stdctx.Background(), filepath.Join(t.TempDir(), storage.DatabaseFileName))
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Fatalf("Close returned error: %v", err)
		}
	})
	if err := store.Init(stdctx.Background()); err != nil {
		t.Fatalf("Init returned error: %v", err)
	}
	return store
}

type fixedGitInspector struct {
	state GitState
}

func (f fixedGitInspector) Inspect(stdctx.Context, string) GitState {
	return f.state
}
