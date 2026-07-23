package app

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestCLIContextBuild(t *testing.T) {
	ctx := context.Background()
	dataDir := t.TempDir()
	projectDir := t.TempDir()
	writeContextFixtureFile(t, projectDir, "internal/auth/token.go", "package auth\n\nfunc refreshToken() string {\n\treturn \"value\"\n}\n")

	projectCLI := newProjectTestCLI(dataDir, &bytes.Buffer{})
	if err := projectCLI.Run(ctx, []string{"project", "add", projectDir, "--name", "tok", "--display-name", "TOK"}); err != nil {
		t.Fatalf("project add returned error: %v", err)
	}

	var taskOut bytes.Buffer
	taskCLI := newProjectTestCLI(dataDir, &taskOut)
	if err := taskCLI.Run(ctx, []string{
		"task", "create",
		"--project", "tok",
		"--title", "Refresh token package",
		"--description", "Find refreshToken implementation.",
		"--acceptance-criteria", "- include retrieval results",
	}); err != nil {
		t.Fatalf("task create returned error: %v", err)
	}
	taskID := mustExtractNumericField(t, taskOut.String(), "id")

	indexCLI := newProjectTestCLI(dataDir, &bytes.Buffer{})
	if err := indexCLI.Run(ctx, []string{"index", "update", "--project", "tok"}); err != nil {
		t.Fatalf("index update returned error: %v", err)
	}

	var contextOut bytes.Buffer
	contextCLI := newProjectTestCLI(dataDir, &contextOut)
	if err := contextCLI.Run(ctx, []string{"context", "build", "--project", "tok", "--task", strconv.FormatInt(taskID, 10), "--limit", "2"}); err != nil {
		t.Fatalf("context build returned error: %v", err)
	}

	output := contextOut.String()
	for _, want := range []string{
		"# TOK Context Package",
		"## Project",
		"name: tok",
		"display_name: TOK",
		"## Task",
		"title: Refresh token package",
		"acceptance_criteria:",
		"  - include retrieval results",
		"## Task Events",
		"type: created",
		"## Retrieval Results",
		"1. path: internal/auth/token.go",
		"provenance: project_file",
		"snippet: func refreshToken() string {",
		"## Repository State",
		"available: false",
		"error: not a git worktree",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("context package output missing %q:\n%s", want, output)
		}
	}
}

func writeContextFixtureFile(t *testing.T, root, relPath, content string) {
	t.Helper()

	path := filepath.Join(root, relPath)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll returned error: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}
}
