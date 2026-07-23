package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestCLIRunStartShowFinish(t *testing.T) {
	ctx := context.Background()
	dataDir := t.TempDir()
	projectDir := t.TempDir()
	initialHead := initRunTestGitRepo(t, projectDir)

	projectCLI := newProjectTestCLI(dataDir, &bytes.Buffer{})
	if err := projectCLI.Run(ctx, []string{"project", "add", projectDir, "--name", "tok"}); err != nil {
		t.Fatalf("project add returned error: %v", err)
	}

	taskID := createTaskForTest(t, ctx, dataDir, "tok", "Run handoff task")
	claimCLI := newProjectTestCLI(dataDir, &bytes.Buffer{})
	if err := claimCLI.Run(ctx, []string{"task", "claim", "--project", "tok", strconv.FormatInt(taskID, 10)}); err != nil {
		t.Fatalf("task claim returned error: %v", err)
	}

	indexCLI := newProjectTestCLI(dataDir, &bytes.Buffer{})
	if err := indexCLI.Run(ctx, []string{"index", "update", "--project", "tok"}); err != nil {
		t.Fatalf("index update returned error: %v", err)
	}

	handoffPath := filepath.Join(t.TempDir(), "handoff.md")
	var startOut bytes.Buffer
	startCLI := newProjectTestCLI(dataDir, &startOut)
	if err := startCLI.Run(ctx, []string{
		"run", "start",
		"--task", strconv.FormatInt(taskID, 10),
		"--limit", "3",
		"--handoff-output", handoffPath,
		"--json",
	}); err != nil {
		t.Fatalf("run start --json returned error: %v", err)
	}
	var started runOutput
	if err := json.Unmarshal(startOut.Bytes(), &started); err != nil {
		t.Fatalf("parse run start JSON: %v\n%s", err, startOut.String())
	}
	if started.ID == 0 || started.TaskID != taskID || started.Status != "in_progress" {
		t.Fatalf("unexpected started run: %+v", started)
	}
	if started.HandoffContractVersion != "tok.handoff.v0" || started.StartedAt == "" || started.FinishedAt != "" {
		t.Fatalf("unexpected started run contract fields: %+v", started)
	}
	if started.RetrievalLimit != 3 || started.BaseBranch != "main" || started.BaseHead != initialHead {
		t.Fatalf("unexpected started run snapshot: %+v", started)
	}
	handoffContent, err := os.ReadFile(handoffPath)
	if err != nil {
		t.Fatalf("ReadFile handoff path returned error: %v", err)
	}
	for _, want := range []string{
		"# TOK Context Package",
		"contract_version: tok.handoff.v0",
		"title: Run handoff task",
	} {
		if !strings.Contains(string(handoffContent), want) {
			t.Fatalf("handoff output missing %q:\n%s", want, string(handoffContent))
		}
	}
	assertHandoffArtifactOutput(t, started.Artifacts, started.ID, handoffPath, sha256ContentHash(string(handoffContent)))

	secondHead := commitRunTestGitChange(t, projectDir, "second.txt", "second")
	if secondHead == initialHead {
		t.Fatalf("expected changed git head after second commit")
	}

	var showOut bytes.Buffer
	showCLI := newProjectTestCLI(dataDir, &showOut)
	if err := showCLI.Run(ctx, []string{"run", "show", strconv.FormatInt(started.ID, 10), "--json"}); err != nil {
		t.Fatalf("run show --json returned error: %v", err)
	}
	var shown runOutput
	if err := json.Unmarshal(showOut.Bytes(), &shown); err != nil {
		t.Fatalf("parse run show JSON: %v\n%s", err, showOut.String())
	}
	if shown.ID != started.ID || shown.Status != "in_progress" {
		t.Fatalf("unexpected shown run: %+v", shown)
	}
	if shown.BaseHead != initialHead || shown.RetrievalLimit != 3 {
		t.Fatalf("run show should preserve start snapshot, got %+v", shown)
	}
	assertHandoffArtifactOutput(t, shown.Artifacts, started.ID, handoffPath, sha256ContentHash(string(handoffContent)))

	var finishOut bytes.Buffer
	finishCLI := newProjectTestCLI(dataDir, &finishOut)
	if err := finishCLI.Run(ctx, []string{
		"run", "finish",
		strconv.FormatInt(started.ID, 10),
		"--status", "succeeded",
		"--summary", "Implemented and tests pass.",
		"--json",
	}); err != nil {
		t.Fatalf("run finish --json returned error: %v", err)
	}
	var finished runOutput
	if err := json.Unmarshal(finishOut.Bytes(), &finished); err != nil {
		t.Fatalf("parse run finish JSON: %v\n%s", err, finishOut.String())
	}
	if finished.Status != "succeeded" || finished.FinishedAt == "" || finished.ResultSummary != "Implemented and tests pass." {
		t.Fatalf("unexpected finished run: %+v", finished)
	}
	assertHandoffArtifactOutput(t, finished.Artifacts, started.ID, handoffPath, sha256ContentHash(string(handoffContent)))

	var taskOut bytes.Buffer
	taskCLI := newProjectTestCLI(dataDir, &taskOut)
	if err := taskCLI.Run(ctx, []string{"task", "show", strconv.FormatInt(taskID, 10), "--json"}); err != nil {
		t.Fatalf("task show --json returned error: %v", err)
	}
	var task taskShowOutput
	if err := json.Unmarshal(taskOut.Bytes(), &task); err != nil {
		t.Fatalf("parse task show JSON: %v\n%s", err, taskOut.String())
	}
	if task.Task.Status != "in_progress" {
		t.Fatalf("run finish should not close task, got %+v", task.Task)
	}
}

func assertHandoffArtifactOutput(t *testing.T, artifacts []runArtifactOutput, runID int64, path, contentHash string) {
	t.Helper()

	if len(artifacts) != 1 {
		t.Fatalf("expected one run artifact, got %+v", artifacts)
	}
	artifact := artifacts[0]
	if artifact.ID == 0 || artifact.RunID != runID {
		t.Fatalf("unexpected handoff artifact ids: %+v", artifact)
	}
	if artifact.Kind != "handoff" || artifact.Path != path || artifact.ContentHash != contentHash {
		t.Fatalf("unexpected handoff artifact: %+v", artifact)
	}
	if artifact.Metadata != `{"format":"text"}` || artifact.CreatedAt == "" {
		t.Fatalf("unexpected handoff artifact metadata: %+v", artifact)
	}
}

func TestCLIRunStartPrintsTextOutput(t *testing.T) {
	ctx := context.Background()
	dataDir := t.TempDir()
	projectDir := t.TempDir()

	projectCLI := newProjectTestCLI(dataDir, &bytes.Buffer{})
	if err := projectCLI.Run(ctx, []string{"project", "add", projectDir, "--name", "tok"}); err != nil {
		t.Fatalf("project add returned error: %v", err)
	}
	taskID := createTaskForTest(t, ctx, dataDir, "tok", "Run text task")

	var out bytes.Buffer
	cli := newProjectTestCLI(dataDir, &out)
	if err := cli.Run(ctx, []string{"run", "start", "--task", strconv.FormatInt(taskID, 10)}); err != nil {
		t.Fatalf("run start returned error: %v", err)
	}
	for _, want := range []string{
		"id:",
		"task_id: " + strconv.FormatInt(taskID, 10),
		"status: in_progress",
		"handoff_contract_version: tok.handoff.v0",
		"retrieval_limit: 5",
	} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("run start output missing %q:\n%s", want, out.String())
		}
	}
}

func TestCLIRunRecordValidationArtifact(t *testing.T) {
	ctx := context.Background()
	dataDir := t.TempDir()
	projectDir := t.TempDir()

	projectCLI := newProjectTestCLI(dataDir, &bytes.Buffer{})
	if err := projectCLI.Run(ctx, []string{"project", "add", projectDir, "--name", "tok"}); err != nil {
		t.Fatalf("project add returned error: %v", err)
	}
	taskID := createTaskForTest(t, ctx, dataDir, "tok", "Validation artifact task")

	var startOut bytes.Buffer
	startCLI := newProjectTestCLI(dataDir, &startOut)
	if err := startCLI.Run(ctx, []string{"run", "start", "--task", strconv.FormatInt(taskID, 10), "--json"}); err != nil {
		t.Fatalf("run start --json returned error: %v", err)
	}
	var started runOutput
	if err := json.Unmarshal(startOut.Bytes(), &started); err != nil {
		t.Fatalf("parse run start JSON: %v\n%s", err, startOut.String())
	}

	var recordOut bytes.Buffer
	recordCLI := newProjectTestCLI(dataDir, &recordOut)
	if err := recordCLI.Run(ctx, []string{
		"run", "record-validation",
		strconv.FormatInt(started.ID, 10),
		"--command", "go test ./...",
		"--status", "passed",
		"--summary", "All tests pass.",
		"--json",
	}); err != nil {
		t.Fatalf("run record-validation --json returned error: %v", err)
	}
	var artifact runArtifactOutput
	if err := json.Unmarshal(recordOut.Bytes(), &artifact); err != nil {
		t.Fatalf("parse run record-validation JSON: %v\n%s", err, recordOut.String())
	}
	if artifact.ID == 0 || artifact.RunID != started.ID || artifact.Kind != "validation" {
		t.Fatalf("unexpected validation artifact: %+v", artifact)
	}
	if artifact.Path != "" || artifact.ContentHash != "" || artifact.CreatedAt == "" {
		t.Fatalf("unexpected validation artifact file fields: %+v", artifact)
	}
	var metadata struct {
		Command string `json:"command"`
		Status  string `json:"status"`
		Summary string `json:"summary"`
	}
	if err := json.Unmarshal([]byte(artifact.Metadata), &metadata); err != nil {
		t.Fatalf("parse validation metadata: %v\n%s", err, artifact.Metadata)
	}
	if metadata.Command != "go test ./..." || metadata.Status != "passed" || metadata.Summary != "All tests pass." {
		t.Fatalf("unexpected validation metadata: %+v", metadata)
	}

	var showOut bytes.Buffer
	showCLI := newProjectTestCLI(dataDir, &showOut)
	if err := showCLI.Run(ctx, []string{"run", "show", strconv.FormatInt(started.ID, 10), "--json"}); err != nil {
		t.Fatalf("run show --json returned error: %v", err)
	}
	var shown runOutput
	if err := json.Unmarshal(showOut.Bytes(), &shown); err != nil {
		t.Fatalf("parse run show JSON: %v\n%s", err, showOut.String())
	}
	if len(shown.Artifacts) != 1 || shown.Artifacts[0] != artifact {
		t.Fatalf("run show did not include validation artifact: %+v", shown.Artifacts)
	}

	invalidStatusCLI := newProjectTestCLI(dataDir, &bytes.Buffer{})
	err := invalidStatusCLI.Run(ctx, []string{
		"run", "record-validation",
		strconv.FormatInt(started.ID, 10),
		"--command", "go test ./...",
		"--status", "skipped",
		"--summary", "Skipped.",
	})
	if err == nil {
		t.Fatal("expected invalid validation status error")
	}
	var usageErr *UsageError
	if !errors.As(err, &usageErr) || !strings.Contains(usageErr.Message, "passed or failed") {
		t.Fatalf("unexpected invalid validation status error: %T %v", err, err)
	}
}

func initRunTestGitRepo(t *testing.T, dir string) string {
	t.Helper()

	if _, err := exec.LookPath("git"); err != nil {
		t.Skipf("git is required for run snapshot test: %v", err)
	}
	runGitCommand(t, dir, "init")
	runGitCommand(t, dir, "checkout", "-b", "main")
	runGitCommand(t, dir, "config", "user.email", "tok@example.test")
	runGitCommand(t, dir, "config", "user.name", "TOK Test")
	return commitRunTestGitChange(t, dir, "README.md", "# TOK\n")
}

func commitRunTestGitChange(t *testing.T, dir, name, content string) string {
	t.Helper()

	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatalf("write git fixture file: %v", err)
	}
	runGitCommand(t, dir, "add", name)
	runGitCommand(t, dir, "commit", "-m", "test commit")
	return strings.TrimSpace(runGitCommand(t, dir, "rev-parse", "--short", "HEAD"))
}

func runGitCommand(t *testing.T, dir string, args ...string) string {
	t.Helper()

	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s failed: %v\n%s", strings.Join(args, " "), err, string(out))
	}
	return string(out)
}

func TestCLIRunFinishValidation(t *testing.T) {
	ctx := context.Background()
	dataDir := t.TempDir()
	projectDir := t.TempDir()

	projectCLI := newProjectTestCLI(dataDir, &bytes.Buffer{})
	if err := projectCLI.Run(ctx, []string{"project", "add", projectDir, "--name", "tok"}); err != nil {
		t.Fatalf("project add returned error: %v", err)
	}
	taskID := createTaskForTest(t, ctx, dataDir, "tok", "Run validation task")

	var startOut bytes.Buffer
	startCLI := newProjectTestCLI(dataDir, &startOut)
	if err := startCLI.Run(ctx, []string{"run", "start", "--task", strconv.FormatInt(taskID, 10), "--json"}); err != nil {
		t.Fatalf("run start --json returned error: %v", err)
	}
	var started runOutput
	if err := json.Unmarshal(startOut.Bytes(), &started); err != nil {
		t.Fatalf("parse run start JSON: %v\n%s", err, startOut.String())
	}

	missingSummaryCLI := newProjectTestCLI(dataDir, &bytes.Buffer{})
	err := missingSummaryCLI.Run(ctx, []string{"run", "finish", strconv.FormatInt(started.ID, 10), "--status", "failed"})
	if err == nil {
		t.Fatal("expected missing summary error")
	}
	var usageErr *UsageError
	if !errors.As(err, &usageErr) || !strings.Contains(usageErr.Message, "requires --summary") {
		t.Fatalf("unexpected missing summary error: %T %v", err, err)
	}

	invalidStatusCLI := newProjectTestCLI(dataDir, &bytes.Buffer{})
	err = invalidStatusCLI.Run(ctx, []string{
		"run", "finish",
		strconv.FormatInt(started.ID, 10),
		"--status", "in_progress",
		"--summary", "Still running.",
	})
	if err == nil || !strings.Contains(err.Error(), `invalid terminal run status "in_progress"`) {
		t.Fatalf("expected invalid terminal status error, got %v", err)
	}
}
