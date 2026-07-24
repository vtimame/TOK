package app

import (
	"bytes"
	"context"
	"encoding/json"
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
	writeContextFixtureFile(t, projectDir, "custom.md", "# Custom\n\ncustom marker lives here.\n")

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
	blockerID := createTaskForTest(t, ctx, dataDir, "tok", "Prepare source index")

	depCLI := newProjectTestCLI(dataDir, &bytes.Buffer{})
	if err := depCLI.Run(ctx, []string{"task", "dependency", "add", strconv.FormatInt(blockerID, 10), strconv.FormatInt(taskID, 10)}); err != nil {
		t.Fatalf("task dependency add returned error: %v", err)
	}

	indexCLI := newProjectTestCLI(dataDir, &bytes.Buffer{})
	if err := indexCLI.Run(ctx, []string{"index", "update", "--project", "tok"}); err != nil {
		t.Fatalf("index update returned error: %v", err)
	}
	instructionCLI := newProjectTestCLI(dataDir, &bytes.Buffer{})
	if err := instructionCLI.Run(ctx, []string{
		"project", "instruction", "add",
		"--project", "tok",
		"--title", "Документация библиотек",
		"--body", "Используй Context7 для поиска документации библиотек.",
		"--priority", "high",
	}); err != nil {
		t.Fatalf("project instruction add returned error: %v", err)
	}

	var contextOut bytes.Buffer
	contextCLI := newProjectTestCLI(dataDir, &contextOut)
	if err := contextCLI.Run(ctx, []string{"context", "build", "--project", "tok", "--task", strconv.FormatInt(taskID, 10), "--limit", "2"}); err != nil {
		t.Fatalf("context build returned error: %v", err)
	}

	output := contextOut.String()
	for _, want := range []string{
		"# TOK Context Package",
		"## Handoff Contract",
		"contract_version: tok.handoff.v0",
		"retrieval_limit: 2",
		"## Task",
		"title: Refresh token package",
		"acceptance_criteria:",
		"  - include retrieval results",
		"## Current State",
		"active_blockers: 1",
		"repository_available: false",
		"## Project",
		"name: tok",
		"display_name: TOK",
		"## Project Instructions",
		"priority: high title: Документация библиотек",
		"Используй Context7 для поиска документации библиотек.",
		"## Task Dependencies",
		"blocker_task_id: " + strconv.FormatInt(blockerID, 10) + " blocked_task_id: " + strconv.FormatInt(taskID, 10),
		"## Task Events",
		"type: created",
		"## Relevant Files",
		"1. path: internal/auth/token.go",
		"provenance: project_file",
		"snippet: func refreshToken() string {",
		"excerpt:",
		"3: func refreshToken() string {",
		"## Repository State",
		"available: false",
		"error: not a git worktree",
		"## Commands",
		"tok task show " + strconv.FormatInt(taskID, 10) + " --json",
		"## Open Questions",
		"none",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("context package output missing %q:\n%s", want, output)
		}
	}

	outputPath := filepath.Join(t.TempDir(), "context.md")
	var fileOut bytes.Buffer
	fileCLI := newProjectTestCLI(dataDir, &fileOut)
	if err := fileCLI.Run(ctx, []string{
		"context", "build",
		"--project", "tok",
		"--task", strconv.FormatInt(taskID, 10),
		"--query", "custom marker",
		"--output", outputPath,
	}); err != nil {
		t.Fatalf("context build --output returned error: %v", err)
	}
	if !strings.Contains(fileOut.String(), "wrote context package: "+outputPath) {
		t.Fatalf("unexpected context build --output stdout:\n%s", fileOut.String())
	}

	fileContent, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("ReadFile output path returned error: %v", err)
	}
	fileText := string(fileContent)
	for _, want := range []string{
		"## Relevant Files",
		"1. path: custom.md",
		"snippet: custom marker lives here.",
		"excerpt:",
	} {
		if !strings.Contains(fileText, want) {
			t.Fatalf("context package file missing %q:\n%s", want, fileText)
		}
	}

	var jsonOut bytes.Buffer
	jsonCLI := newProjectTestCLI(dataDir, &jsonOut)
	if err := jsonCLI.Run(ctx, []string{
		"context", "build",
		"--project", "tok",
		"--task", strconv.FormatInt(taskID, 10),
		"--query", "custom marker",
		"--json",
	}); err != nil {
		t.Fatalf("context build --json returned error: %v", err)
	}
	var packageJSON contextPackageOutput
	if err := json.Unmarshal(jsonOut.Bytes(), &packageJSON); err != nil {
		t.Fatalf("parse context package JSON: %v\n%s", err, jsonOut.String())
	}
	if packageJSON.Project.Name != "tok" || packageJSON.Task.ID != taskID || packageJSON.Task.Title != "Refresh token package" {
		t.Fatalf("unexpected context package JSON metadata: %+v", packageJSON)
	}
	if packageJSON.ContractVersion != "tok.handoff.v0" {
		t.Fatalf("unexpected context package contract version: %+v", packageJSON)
	}
	if packageJSON.RetrievalLimit != 5 {
		t.Fatalf("unexpected context package retrieval limit: %+v", packageJSON)
	}
	if len(packageJSON.Dependencies) != 1 || packageJSON.Dependencies[0].BlockerTaskID != blockerID || packageJSON.Dependencies[0].BlockedTaskID != taskID {
		t.Fatalf("unexpected context package JSON dependencies: %+v", packageJSON.Dependencies)
	}
	if len(packageJSON.ProjectInstructions) != 1 || packageJSON.ProjectInstructions[0].Title != "Документация библиотек" || packageJSON.ProjectInstructions[0].Priority != "high" {
		t.Fatalf("unexpected context package JSON project instructions: %+v", packageJSON.ProjectInstructions)
	}
	if len(packageJSON.Blockers) != 1 || packageJSON.Blockers[0].BlockerTaskID != blockerID {
		t.Fatalf("unexpected context package JSON blockers: %+v", packageJSON.Blockers)
	}
	if len(packageJSON.SuggestedCommands) == 0 || !strings.Contains(packageJSON.SuggestedCommands[0], strconv.FormatInt(taskID, 10)) {
		t.Fatalf("unexpected context package JSON suggested commands: %+v", packageJSON.SuggestedCommands)
	}
	if len(packageJSON.Events) != 1 || packageJSON.Events[0].Type != "created" {
		t.Fatalf("unexpected context package JSON events: %+v", packageJSON.Events)
	}
	if len(packageJSON.RetrievalResults) == 0 || packageJSON.RetrievalResults[0].Path != "custom.md" || packageJSON.RetrievalResults[0].Excerpt == "" {
		t.Fatalf("unexpected context package JSON retrieval results: %+v", packageJSON.RetrievalResults)
	}
	if packageJSON.RepositoryState.Available {
		t.Fatalf("expected fixture project to have unavailable repository state: %+v", packageJSON.RepositoryState)
	}
	if len(packageJSON.RepositoryState.DiffSummary) != 0 {
		t.Fatalf("expected unavailable repository state to have empty diff summary: %+v", packageJSON.RepositoryState)
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
