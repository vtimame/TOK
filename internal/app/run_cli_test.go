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

func sameRunArtifactOutput(a, b runArtifactOutput) bool {
	if a.ID != b.ID || a.RunID != b.RunID || a.Kind != b.Kind || a.Path != b.Path || a.ContentHash != b.ContentHash || a.Metadata != b.Metadata || a.CreatedAt != b.CreatedAt {
		return false
	}
	if a.Actor == nil || b.Actor == nil {
		return a.Actor == nil && b.Actor == nil
	}
	return *a.Actor == *b.Actor
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

func TestCLIRunListAndCancel(t *testing.T) {
	ctx := context.Background()
	dataDir := t.TempDir()
	projectDir := t.TempDir()
	otherProjectDir := t.TempDir()

	projectCLI := newProjectTestCLI(dataDir, &bytes.Buffer{})
	if err := projectCLI.Run(ctx, []string{"project", "add", projectDir, "--name", "tok"}); err != nil {
		t.Fatalf("project add tok returned error: %v", err)
	}
	otherProjectCLI := newProjectTestCLI(dataDir, &bytes.Buffer{})
	if err := otherProjectCLI.Run(ctx, []string{"project", "add", otherProjectDir, "--name", "other"}); err != nil {
		t.Fatalf("project add other returned error: %v", err)
	}

	firstTaskID := createTaskForTest(t, ctx, dataDir, "tok", "First run task")
	secondTaskID := createTaskForTest(t, ctx, dataDir, "tok", "Second run task")
	otherTaskID := createTaskForTest(t, ctx, dataDir, "other", "Other run task")

	firstRun := startRunForTest(t, ctx, dataDir, firstTaskID)
	secondRun := startRunForTest(t, ctx, dataDir, secondTaskID)
	otherRun := startRunForTest(t, ctx, dataDir, otherTaskID)

	var finishOut bytes.Buffer
	finishCLI := newProjectTestCLI(dataDir, &finishOut)
	if err := finishCLI.Run(ctx, []string{
		"run", "finish",
		strconv.FormatInt(secondRun.ID, 10),
		"--status", "failed",
		"--summary", "Tests failed.",
		"--json",
	}); err != nil {
		t.Fatalf("run finish failed run returned error: %v", err)
	}

	var textListOut bytes.Buffer
	textListCLI := newProjectTestCLI(dataDir, &textListOut)
	if err := textListCLI.Run(ctx, []string{"run", "list", "--project", "tok"}); err != nil {
		t.Fatalf("run list text returned error: %v", err)
	}
	for _, want := range []string{
		"id | task_id | status",
		"started_at",
		"finished_at",
		"summary",
		strconv.FormatInt(firstRun.ID, 10),
		strconv.FormatInt(secondRun.ID, 10),
		"failed",
		"Tests failed.",
	} {
		if !strings.Contains(textListOut.String(), want) {
			t.Fatalf("run list text output missing %q:\n%s", want, textListOut.String())
		}
	}

	var projectListOut bytes.Buffer
	projectListCLI := newProjectTestCLI(dataDir, &projectListOut)
	if err := projectListCLI.Run(ctx, []string{"run", "list", "--project", "tok", "--json"}); err != nil {
		t.Fatalf("run list --project --json returned error: %v", err)
	}
	var projectRuns []runOutput
	if err := json.Unmarshal(projectListOut.Bytes(), &projectRuns); err != nil {
		t.Fatalf("parse project run list JSON: %v\n%s", err, projectListOut.String())
	}
	if len(projectRuns) != 2 || projectRuns[0].ID != secondRun.ID || projectRuns[1].ID != firstRun.ID {
		t.Fatalf("unexpected project run list: %+v", projectRuns)
	}
	if projectRuns[0].Artifacts == nil || len(projectRuns[0].Artifacts) != 0 {
		t.Fatalf("run list JSON should include empty artifacts array, got %+v", projectRuns[0].Artifacts)
	}

	var taskListOut bytes.Buffer
	taskListCLI := newProjectTestCLI(dataDir, &taskListOut)
	if err := taskListCLI.Run(ctx, []string{"run", "list", "--task", strconv.FormatInt(firstTaskID, 10), "--json"}); err != nil {
		t.Fatalf("run list --task --json returned error: %v", err)
	}
	var taskRuns []runOutput
	if err := json.Unmarshal(taskListOut.Bytes(), &taskRuns); err != nil {
		t.Fatalf("parse task run list JSON: %v\n%s", err, taskListOut.String())
	}
	if len(taskRuns) != 1 || taskRuns[0].ID != firstRun.ID {
		t.Fatalf("unexpected task run list: %+v", taskRuns)
	}

	var activeListOut bytes.Buffer
	activeListCLI := newProjectTestCLI(dataDir, &activeListOut)
	if err := activeListCLI.Run(ctx, []string{"run", "list", "--status=in_progress", "--json"}); err != nil {
		t.Fatalf("run list --status --json returned error: %v", err)
	}
	var activeRuns []runOutput
	if err := json.Unmarshal(activeListOut.Bytes(), &activeRuns); err != nil {
		t.Fatalf("parse active run list JSON: %v\n%s", err, activeListOut.String())
	}
	if len(activeRuns) != 2 || activeRuns[0].ID != otherRun.ID || activeRuns[1].ID != firstRun.ID {
		t.Fatalf("unexpected active run list: %+v", activeRuns)
	}

	var cancelOut bytes.Buffer
	cancelCLI := newProjectTestCLI(dataDir, &cancelOut)
	if err := cancelCLI.Run(ctx, []string{
		"run", "cancel",
		strconv.FormatInt(firstRun.ID, 10),
		"--summary", "Stopped by operator.",
		"--json",
	}); err != nil {
		t.Fatalf("run cancel --json returned error: %v", err)
	}
	var cancelled runOutput
	if err := json.Unmarshal(cancelOut.Bytes(), &cancelled); err != nil {
		t.Fatalf("parse cancelled run JSON: %v\n%s", err, cancelOut.String())
	}
	if cancelled.Status != "cancelled" || cancelled.FinishedAt == "" || cancelled.ResultSummary != "Stopped by operator." || cancelled.FinishedBy == nil {
		t.Fatalf("unexpected cancelled run: %+v", cancelled)
	}

	var cancelledListOut bytes.Buffer
	cancelledListCLI := newProjectTestCLI(dataDir, &cancelledListOut)
	if err := cancelledListCLI.Run(ctx, []string{"run", "list", "--project=tok", "--status", "cancelled", "--json"}); err != nil {
		t.Fatalf("run list cancelled returned error: %v", err)
	}
	var cancelledRuns []runOutput
	if err := json.Unmarshal(cancelledListOut.Bytes(), &cancelledRuns); err != nil {
		t.Fatalf("parse cancelled run list JSON: %v\n%s", err, cancelledListOut.String())
	}
	if len(cancelledRuns) != 1 || cancelledRuns[0].ID != firstRun.ID {
		t.Fatalf("unexpected cancelled run list: %+v", cancelledRuns)
	}

	cancelTerminalCLI := newProjectTestCLI(dataDir, &bytes.Buffer{})
	err := cancelTerminalCLI.Run(ctx, []string{"run", "cancel", strconv.FormatInt(firstRun.ID, 10), "--summary", "Again."})
	if err == nil || !strings.Contains(err.Error(), "run cannot be cancelled from current status") {
		t.Fatalf("expected terminal cancel error, got %v", err)
	}

	cancelMissingCLI := newProjectTestCLI(dataDir, &bytes.Buffer{})
	err = cancelMissingCLI.Run(ctx, []string{"run", "cancel", "999999", "--summary", "Missing."})
	if err == nil || !strings.Contains(err.Error(), "run not found: 999999") {
		t.Fatalf("expected missing run error, got %v", err)
	}
}

func startRunForTest(t *testing.T, ctx context.Context, dataDir string, taskID int64) runOutput {
	t.Helper()

	var out bytes.Buffer
	cli := newProjectTestCLI(dataDir, &out)
	if err := cli.Run(ctx, []string{"run", "start", "--task", strconv.FormatInt(taskID, 10), "--json"}); err != nil {
		t.Fatalf("run start --json returned error: %v", err)
	}
	var run runOutput
	if err := json.Unmarshal(out.Bytes(), &run); err != nil {
		t.Fatalf("parse run start JSON: %v\n%s", err, out.String())
	}
	return run
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
	if len(shown.Artifacts) != 1 || !sameRunArtifactOutput(shown.Artifacts[0], artifact) {
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
