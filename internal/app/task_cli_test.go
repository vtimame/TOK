package app

import (
	"bytes"
	"context"
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
