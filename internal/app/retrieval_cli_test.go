package app

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCLIIndexUpdateAndSearch(t *testing.T) {
	ctx := context.Background()
	dataDir := t.TempDir()
	projectDir := t.TempDir()
	writeAppFixtureFile(t, projectDir, "internal/auth/token.go", "package auth\n\nfunc refreshToken() string {\n\treturn \"value\"\n}\n")
	writeAppFixtureFile(t, projectDir, ".env", "SECRET=skip\n")

	projectCLI := newProjectTestCLI(dataDir, &bytes.Buffer{})
	if err := projectCLI.Run(ctx, []string{"project", "add", projectDir, "--name", "tok"}); err != nil {
		t.Fatalf("project add returned error: %v", err)
	}

	var indexOut bytes.Buffer
	indexCLI := newProjectTestCLI(dataDir, &indexOut)
	if err := indexCLI.Run(ctx, []string{"index", "update", "--project", "tok"}); err != nil {
		t.Fatalf("index update returned error: %v", err)
	}
	for _, want := range []string{
		"project: tok",
		"indexed_documents: 1",
		"skipped_files: 1",
		"skipped_reasons:",
		"- env_file: 1",
		"updated_at:",
	} {
		if !strings.Contains(indexOut.String(), want) {
			t.Fatalf("index output missing %q:\n%s", want, indexOut.String())
		}
	}

	var statusOut bytes.Buffer
	statusCLI := newProjectTestCLI(dataDir, &statusOut)
	if err := statusCLI.Run(ctx, []string{"index", "status", "--project", "tok", "--json"}); err != nil {
		t.Fatalf("index status --json returned error: %v", err)
	}
	var status indexSummaryOutput
	if err := json.Unmarshal(statusOut.Bytes(), &status); err != nil {
		t.Fatalf("parse index status JSON: %v\n%s", err, statusOut.String())
	}
	if status.ProjectName != "tok" || status.IndexedDocuments != 1 || status.SkippedFiles != 1 || status.UpdatedAt == "" {
		t.Fatalf("unexpected index status JSON: %+v", status)
	}
	if status.SkippedReasons["env_file"] != 1 {
		t.Fatalf("unexpected status skipped reasons: %+v", status.SkippedReasons)
	}

	var searchOut bytes.Buffer
	searchCLI := newProjectTestCLI(dataDir, &searchOut)
	if err := searchCLI.Run(ctx, []string{"search", "--project", "tok", "refresh token", "--limit", "3"}); err != nil {
		t.Fatalf("search returned error: %v", err)
	}
	for _, want := range []string{
		"path\tline\tscore\tprovenance\tsnippet",
		"internal/auth/token.go\t3\t",
		"project_file\tfunc refreshToken() string {",
	} {
		if !strings.Contains(searchOut.String(), want) {
			t.Fatalf("search output missing %q:\n%s", want, searchOut.String())
		}
	}
	if strings.Contains(searchOut.String(), ".env") {
		t.Fatalf("excluded file appeared in search output:\n%s", searchOut.String())
	}
}

func TestCLISearchRejectsMissingProject(t *testing.T) {
	cli := newProjectTestCLI(t.TempDir(), &bytes.Buffer{})

	err := cli.Run(context.Background(), []string{"search", "token"})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "search requires --project") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func writeAppFixtureFile(t *testing.T, root, relPath, content string) {
	t.Helper()

	path := filepath.Join(root, relPath)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll returned error: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}
}
