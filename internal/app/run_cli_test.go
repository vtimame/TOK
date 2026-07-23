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

func TestCLIRunStartShowFinish(t *testing.T) {
	ctx := context.Background()
	dataDir := t.TempDir()
	projectDir := t.TempDir()

	projectCLI := newProjectTestCLI(dataDir, &bytes.Buffer{})
	if err := projectCLI.Run(ctx, []string{"project", "add", projectDir, "--name", "tok"}); err != nil {
		t.Fatalf("project add returned error: %v", err)
	}

	taskID := createTaskForTest(t, ctx, dataDir, "tok", "Run handoff task")
	claimCLI := newProjectTestCLI(dataDir, &bytes.Buffer{})
	if err := claimCLI.Run(ctx, []string{"task", "claim", "--project", "tok", strconv.FormatInt(taskID, 10)}); err != nil {
		t.Fatalf("task claim returned error: %v", err)
	}

	var startOut bytes.Buffer
	startCLI := newProjectTestCLI(dataDir, &startOut)
	if err := startCLI.Run(ctx, []string{"run", "start", "--task", strconv.FormatInt(taskID, 10), "--json"}); err != nil {
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
	} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("run start output missing %q:\n%s", want, out.String())
		}
	}
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
