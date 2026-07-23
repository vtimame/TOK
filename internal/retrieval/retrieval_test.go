package retrieval

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"s26.sh/tok/internal/storage"
)

func TestTokenizeLowercasesAndDeduplicates(t *testing.T) {
	got := Tokenize("Refresh refreshToken token auth_token")
	want := []string{"refresh", "refreshtoken", "token", "auth_token", "auth"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected tokens: got %+v want %+v", got, want)
	}
}

func TestIndexAndSearchFixtureProject(t *testing.T) {
	ctx := context.Background()
	projectDir := t.TempDir()
	writeFixtureFile(t, projectDir, "auth.go", "package auth\n\nfunc refreshToken() string {\n\treturn \"value\"\n}\n")
	writeFixtureFile(t, projectDir, "README.md", "# Fixture\n\nToken refresh notes live here.\n")
	writeFixtureFile(t, projectDir, ".env", "SECRET=skip\n")
	writeFixtureFile(t, projectDir, "image.png", "\x00\x01\x02")

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

	project, err := store.CreateProject(ctx, storage.CreateProjectInput{
		Name:        "fixture",
		DisplayName: "Fixture",
		Path:        projectDir,
	})
	if err != nil {
		t.Fatalf("CreateProject returned error: %v", err)
	}

	service := NewService(store)
	summary, err := service.IndexProject(ctx, project)
	if err != nil {
		t.Fatalf("IndexProject returned error: %v", err)
	}
	if summary.IndexedDocuments != 2 {
		t.Fatalf("expected 2 indexed documents, got %+v", summary)
	}
	if summary.SkippedFiles != 2 {
		t.Fatalf("expected 2 skipped files, got %+v", summary)
	}

	results, err := service.Search(ctx, project, "refresh token", 5)
	if err != nil {
		t.Fatalf("Search returned error: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected search results")
	}

	var foundAuth bool
	for _, result := range results {
		if result.Path == "auth.go" {
			foundAuth = true
			if result.Line != 3 || result.Snippet != "func refreshToken() string {" {
				t.Fatalf("unexpected auth snippet: %+v", result)
			}
			for _, want := range []string{"2:", "3: func refreshToken() string {", "4: return \"value\""} {
				if !strings.Contains(result.Excerpt, want) {
					t.Fatalf("auth excerpt missing %q: %+v", want, result)
				}
			}
			if result.Provenance != "project_file" {
				t.Fatalf("unexpected provenance: %+v", result)
			}
		}
		if result.Path == ".env" || result.Path == "image.png" {
			t.Fatalf("excluded file was indexed: %+v", result)
		}
	}
	if !foundAuth {
		t.Fatalf("expected auth.go in results: %+v", results)
	}
}

func writeFixtureFile(t *testing.T, root, relPath, content string) {
	t.Helper()

	path := filepath.Join(root, relPath)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll returned error: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}
}
