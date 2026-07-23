package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"testing"
)

func TestCLITaskCreateListShowStatusAndComment(t *testing.T) {
	ctx := context.Background()
	dataDir := t.TempDir()
	projectDir := t.TempDir()

	projectCLI := newProjectTestCLI(dataDir, &bytes.Buffer{})
	if err := projectCLI.Run(ctx, []string{"project", "add", projectDir, "--name", "tok", "--display-name", "TOK"}); err != nil {
		t.Fatalf("project add returned error: %v", err)
	}

	var createOut bytes.Buffer
	createCLI := newProjectTestCLI(dataDir, &createOut)
	if err := createCLI.Run(ctx, []string{
		"task", "create",
		"--project", "tok",
		"--title", "Implement task store",
		"--description", "Create issue-like task CLI.",
		"--acceptance-criteria", "- create\n- list\n- show\n- status",
		"--notes", "Keep it local.",
	}); err != nil {
		t.Fatalf("task create returned error: %v", err)
	}

	createOutput := createOut.String()
	taskID := mustExtractNumericField(t, createOutput, "id")
	for _, want := range []string{
		"status: open",
		"title: Implement task store",
		"description: Create issue-like task CLI.",
		"acceptance_criteria: - create",
		"notes: Keep it local.",
		"created_at:",
		"updated_at:",
	} {
		if !strings.Contains(createOutput, want) {
			t.Fatalf("task create output missing %q:\n%s", want, createOutput)
		}
	}

	var listOut bytes.Buffer
	listCLI := newProjectTestCLI(dataDir, &listOut)
	if err := listCLI.Run(ctx, []string{"task", "list", "--project", "tok"}); err != nil {
		t.Fatalf("task list returned error: %v", err)
	}
	for _, want := range []string{
		"id\tstatus\ttitle",
		strconv.FormatInt(taskID, 10) + "\topen\tImplement task store",
	} {
		if !strings.Contains(listOut.String(), want) {
			t.Fatalf("task list output missing %q:\n%s", want, listOut.String())
		}
	}

	var statusOut bytes.Buffer
	statusCLI := newProjectTestCLI(dataDir, &statusOut)
	if err := statusCLI.Run(ctx, []string{"task", "status", strconv.FormatInt(taskID, 10), "blocked"}); err != nil {
		t.Fatalf("task status returned error: %v", err)
	}
	if !strings.Contains(statusOut.String(), "status: blocked") {
		t.Fatalf("unexpected task status output:\n%s", statusOut.String())
	}

	var commentOut bytes.Buffer
	commentCLI := newProjectTestCLI(dataDir, &commentOut)
	if err := commentCLI.Run(ctx, []string{"task", "comment", strconv.FormatInt(taskID, 10), "--body", "Needs dependency review."}); err != nil {
		t.Fatalf("task comment returned error: %v", err)
	}
	for _, want := range []string{
		"type: commented",
		"body: Needs dependency review.",
	} {
		if !strings.Contains(commentOut.String(), want) {
			t.Fatalf("task comment output missing %q:\n%s", want, commentOut.String())
		}
	}

	var showOut bytes.Buffer
	showCLI := newProjectTestCLI(dataDir, &showOut)
	if err := showCLI.Run(ctx, []string{"task", "show", strconv.FormatInt(taskID, 10)}); err != nil {
		t.Fatalf("task show returned error: %v", err)
	}
	for _, want := range []string{
		"status: blocked",
		"title: Implement task store",
		"events:",
		"type: created",
		"type: status_changed from: open to: blocked",
		"type: commented body: Needs dependency review.",
	} {
		if !strings.Contains(showOut.String(), want) {
			t.Fatalf("task show output missing %q:\n%s", want, showOut.String())
		}
	}
}

func TestCLITaskDependencyReadyAndClaimFlow(t *testing.T) {
	ctx := context.Background()
	dataDir := t.TempDir()
	projectDir := t.TempDir()

	projectCLI := newProjectTestCLI(dataDir, &bytes.Buffer{})
	if err := projectCLI.Run(ctx, []string{"project", "add", projectDir, "--name", "tok"}); err != nil {
		t.Fatalf("project add returned error: %v", err)
	}

	blockerID := createTaskForTest(t, ctx, dataDir, "tok", "Blocker")
	blockedID := createTaskForTest(t, ctx, dataDir, "tok", "Blocked")
	claimedID := createTaskForTest(t, ctx, dataDir, "tok", "Claimed")

	statusCLI := newProjectTestCLI(dataDir, &bytes.Buffer{})
	if err := statusCLI.Run(ctx, []string{"task", "status", strconv.FormatInt(claimedID, 10), "in_progress"}); err != nil {
		t.Fatalf("task status returned error: %v", err)
	}

	var depOut bytes.Buffer
	depCLI := newProjectTestCLI(dataDir, &depOut)
	if err := depCLI.Run(ctx, []string{"task", "dependency", "add", strconv.FormatInt(blockerID, 10), strconv.FormatInt(blockedID, 10), "--type", "blocks"}); err != nil {
		t.Fatalf("task dependency add returned error: %v", err)
	}
	for _, want := range []string{
		"edge_type: blocks",
		"blocker_task_id: " + strconv.FormatInt(blockerID, 10),
		"blocked_task_id: " + strconv.FormatInt(blockedID, 10),
	} {
		if !strings.Contains(depOut.String(), want) {
			t.Fatalf("dependency add output missing %q:\n%s", want, depOut.String())
		}
	}

	var readyOut bytes.Buffer
	readyCLI := newProjectTestCLI(dataDir, &readyOut)
	if err := readyCLI.Run(ctx, []string{"task", "ready", "--project", "tok"}); err != nil {
		t.Fatalf("task ready returned error: %v", err)
	}
	if !strings.Contains(readyOut.String(), strconv.FormatInt(blockerID, 10)+"\topen\tBlocker") {
		t.Fatalf("expected blocker to be ready:\n%s", readyOut.String())
	}
	if strings.Contains(readyOut.String(), "Blocked") || strings.Contains(readyOut.String(), "Claimed") {
		t.Fatalf("blocked or in-progress task should not be ready:\n%s", readyOut.String())
	}

	var readyJSONOut bytes.Buffer
	readyJSONCLI := newProjectTestCLI(dataDir, &readyJSONOut)
	if err := readyJSONCLI.Run(ctx, []string{"task", "ready", "--project", "tok", "--json"}); err != nil {
		t.Fatalf("task ready --json returned error: %v", err)
	}
	var readyTasks []readyTaskOutput
	if err := json.Unmarshal(readyJSONOut.Bytes(), &readyTasks); err != nil {
		t.Fatalf("parse ready JSON: %v\n%s", err, readyJSONOut.String())
	}
	if len(readyTasks) != 1 || readyTasks[0].ID != blockerID || readyTasks[0].Title != "Blocker" {
		t.Fatalf("unexpected ready JSON: %+v", readyTasks)
	}

	statusCLI = newProjectTestCLI(dataDir, &bytes.Buffer{})
	if err := statusCLI.Run(ctx, []string{"task", "status", strconv.FormatInt(blockerID, 10), "done"}); err != nil {
		t.Fatalf("task status done returned error: %v", err)
	}

	readyOut.Reset()
	readyCLI = newProjectTestCLI(dataDir, &readyOut)
	if err := readyCLI.Run(ctx, []string{"task", "ready", "--project", "tok"}); err != nil {
		t.Fatalf("task ready after done returned error: %v", err)
	}
	if !strings.Contains(readyOut.String(), strconv.FormatInt(blockedID, 10)+"\topen\tBlocked") {
		t.Fatalf("expected blocked task to become ready:\n%s", readyOut.String())
	}
	if strings.Contains(readyOut.String(), "Blocker") || strings.Contains(readyOut.String(), "Claimed") {
		t.Fatalf("done or in-progress task should not be ready:\n%s", readyOut.String())
	}

	var removeOut bytes.Buffer
	removeCLI := newProjectTestCLI(dataDir, &removeOut)
	if err := removeCLI.Run(ctx, []string{"task", "dep", "remove", strconv.FormatInt(blockerID, 10), strconv.FormatInt(blockedID, 10)}); err != nil {
		t.Fatalf("task dependency remove returned error: %v", err)
	}
	if !strings.Contains(removeOut.String(), "removed dependency") {
		t.Fatalf("unexpected dependency remove output:\n%s", removeOut.String())
	}

	var claimOut bytes.Buffer
	claimCLI := newProjectTestCLI(dataDir, &claimOut)
	if err := claimCLI.Run(ctx, []string{"task", "claim"}); err != nil {
		t.Fatalf("task claim returned error: %v", err)
	}
	if !strings.Contains(claimOut.String(), "future `tok task claim --project <name> [<task-id>]`") {
		t.Fatalf("unexpected claim flow output:\n%s", claimOut.String())
	}
}

func TestCLITaskCreateRejectsMissingTitle(t *testing.T) {
	cli := newProjectTestCLI(t.TempDir(), &bytes.Buffer{})

	err := cli.Run(context.Background(), []string{"task", "create", "--project", "tok"})
	if err == nil {
		t.Fatal("expected error")
	}

	var usageErr *UsageError
	if !errors.As(err, &usageErr) {
		t.Fatalf("expected UsageError, got %T: %v", err, err)
	}
	if usageErr.Code != 2 || !strings.Contains(usageErr.Message, "requires --title") {
		t.Fatalf("unexpected usage error: %+v", usageErr)
	}
}

func TestCLITaskCreateRejectsUnknownProject(t *testing.T) {
	cli := newProjectTestCLI(t.TempDir(), &bytes.Buffer{})

	err := cli.Run(context.Background(), []string{"task", "create", "--project", "missing", "--title", "Task"})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "project not found: missing") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCLITaskStatusRejectsInvalidStatus(t *testing.T) {
	cli := newProjectTestCLI(t.TempDir(), &bytes.Buffer{})

	err := cli.Run(context.Background(), []string{"task", "status", "1", "paused"})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), `invalid task status "paused"`) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func mustExtractNumericField(t *testing.T, output, field string) int64 {
	t.Helper()

	prefix := field + ": "
	for _, line := range strings.Split(output, "\n") {
		if strings.HasPrefix(line, prefix) {
			value, err := strconv.ParseInt(strings.TrimSpace(strings.TrimPrefix(line, prefix)), 10, 64)
			if err != nil {
				t.Fatalf("parse %s from %q: %v", field, line, err)
			}
			return value
		}
	}

	t.Fatalf("missing field %q in output:\n%s", field, output)
	return 0
}

func createTaskForTest(t *testing.T, ctx context.Context, dataDir, projectName, title string) int64 {
	t.Helper()

	var out bytes.Buffer
	cli := newProjectTestCLI(dataDir, &out)
	if err := cli.Run(ctx, []string{"task", "create", "--project", projectName, "--title", title}); err != nil {
		t.Fatalf("task create %q returned error: %v", title, err)
	}
	return mustExtractNumericField(t, out.String(), "id")
}
