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
		"path",
		"line",
		"score",
		"provenance",
		"snippet",
		"internal/auth/token.go",
		"3",
		"project_file",
		"func refreshToken() string {",
	} {
		if !strings.Contains(searchOut.String(), want) {
			t.Fatalf("search output missing %q:\n%s", want, searchOut.String())
		}
	}
	if strings.Contains(searchOut.String(), ".env") {
		t.Fatalf("excluded file appeared in search output:\n%s", searchOut.String())
	}

	var searchJSONOut bytes.Buffer
	searchJSONCLI := newProjectTestCLI(dataDir, &searchJSONOut)
	if err := searchJSONCLI.Run(ctx, []string{"search", "--project", "tok", "refresh token", "--limit", "3", "--json"}); err != nil {
		t.Fatalf("search --json returned error: %v", err)
	}
	var searchJSON searchListOutput
	if err := json.Unmarshal(searchJSONOut.Bytes(), &searchJSON); err != nil {
		t.Fatalf("parse search JSON: %v\n%s", err, searchJSONOut.String())
	}
	if len(searchJSON.Results) != 1 {
		t.Fatalf("unexpected search JSON result count: %+v", searchJSON)
	}
	result := searchJSON.Results[0]
	if result.Path != "internal/auth/token.go" || result.Line != 3 || result.LineStart == 0 || result.LineEnd == 0 || result.Provenance != "project_file" {
		t.Fatalf("unexpected search JSON result: %+v", result)
	}
	if !strings.Contains(result.Snippet, "refreshToken") || !strings.Contains(result.Excerpt, "refreshToken") {
		t.Fatalf("search JSON result missing snippet/excerpt: %+v", result)
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

func TestCLIIndexStatusAllReportsMissingProjectPath(t *testing.T) {
	ctx := context.Background()
	dataDir := t.TempDir()
	projectDir := t.TempDir()

	projectCLI := newProjectTestCLI(dataDir, &bytes.Buffer{})
	if err := projectCLI.Run(ctx, []string{"project", "add", projectDir, "--name", "missing"}); err != nil {
		t.Fatalf("project add returned error: %v", err)
	}
	if err := os.RemoveAll(projectDir); err != nil {
		t.Fatalf("RemoveAll returned error: %v", err)
	}

	var statusOut bytes.Buffer
	statusCLI := newProjectTestCLI(dataDir, &statusOut)
	if err := statusCLI.Run(ctx, []string{"index", "status", "--all", "--json"}); err != nil {
		t.Fatalf("index status --all --json returned error: %v", err)
	}

	var statuses []indexSummaryOutput
	if err := json.Unmarshal(statusOut.Bytes(), &statuses); err != nil {
		t.Fatalf("parse index status all JSON: %v\n%s", err, statusOut.String())
	}
	if len(statuses) != 1 || statuses[0].ProjectName != "missing" || statuses[0].State != "path_missing" || statuses[0].PathExists {
		t.Fatalf("unexpected index status all JSON: %+v", statuses)
	}
}

func TestCLIIndexIgnoreListAddRemove(t *testing.T) {
	ctx := context.Background()
	dataDir := t.TempDir()
	projectDir := t.TempDir()
	writeAppFixtureFile(t, projectDir, ".gitignore", "typed-router.d.ts\n")

	projectCLI := newProjectTestCLI(dataDir, &bytes.Buffer{})
	if err := projectCLI.Run(ctx, []string{"project", "add", projectDir, "--name", "tok"}); err != nil {
		t.Fatalf("project add returned error: %v", err)
	}

	var listOut bytes.Buffer
	listCLI := newProjectTestCLI(dataDir, &listOut)
	if err := listCLI.Run(ctx, []string{"index", "ignore", "list", "--project", "tok", "--json"}); err != nil {
		t.Fatalf("index ignore list returned error: %v", err)
	}
	var listed indexIgnorePolicyOutput
	if err := json.Unmarshal(listOut.Bytes(), &listed); err != nil {
		t.Fatalf("parse ignore list JSON: %v\n%s", err, listOut.String())
	}
	if len(listed.IgnorePatterns) != 1 || listed.IgnorePatterns[0] != "typed-router.d.ts" || !listed.SeededFromIgnore {
		t.Fatalf("unexpected seeded ignore policy: %+v", listed)
	}

	addCLI := newProjectTestCLI(dataDir, &bytes.Buffer{})
	if err := addCLI.Run(ctx, []string{"index", "ignore", "add", "--project", "tok", "src/generated/**"}); err != nil {
		t.Fatalf("index ignore add returned error: %v", err)
	}
	removeCLI := newProjectTestCLI(dataDir, &bytes.Buffer{})
	if err := removeCLI.Run(ctx, []string{"index", "ignore", "remove", "--project", "tok", "typed-router.d.ts"}); err != nil {
		t.Fatalf("index ignore remove returned error: %v", err)
	}

	var finalOut bytes.Buffer
	finalCLI := newProjectTestCLI(dataDir, &finalOut)
	if err := finalCLI.Run(ctx, []string{"index", "ignore", "list", "--project", "tok", "--json"}); err != nil {
		t.Fatalf("final index ignore list returned error: %v", err)
	}
	var final indexIgnorePolicyOutput
	if err := json.Unmarshal(finalOut.Bytes(), &final); err != nil {
		t.Fatalf("parse final ignore list JSON: %v\n%s", err, finalOut.String())
	}
	if len(final.IgnorePatterns) != 1 || final.IgnorePatterns[0] != "src/generated/**" {
		t.Fatalf("unexpected final ignore policy: %+v", final)
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
