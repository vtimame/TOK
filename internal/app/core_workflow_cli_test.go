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

func TestCLICoreWorkflowEndToEnd(t *testing.T) {
	ctx := context.Background()
	dataDir := t.TempDir()
	projectDir := t.TempDir()
	writeContextFixtureFile(t, projectDir, "workflow.go", "package workflow\n\nfunc buildContextPackage() string {\n\treturn \"workflow context\"\n}\n")

	projectCLI := newProjectTestCLI(dataDir, &bytes.Buffer{})
	if err := projectCLI.Run(ctx, []string{"project", "add", projectDir, "--name", "tok"}); err != nil {
		t.Fatalf("project add returned error: %v", err)
	}

	blockerID := createTaskForTest(t, ctx, dataDir, "tok", "Prepare context package")
	blockedID := createTaskForTest(t, ctx, dataDir, "tok", "Use context package")

	depCLI := newProjectTestCLI(dataDir, &bytes.Buffer{})
	if err := depCLI.Run(ctx, []string{"task", "dependency", "add", strconv.FormatInt(blockerID, 10), strconv.FormatInt(blockedID, 10)}); err != nil {
		t.Fatalf("task dependency add returned error: %v", err)
	}

	var readyOut bytes.Buffer
	readyCLI := newProjectTestCLI(dataDir, &readyOut)
	if err := readyCLI.Run(ctx, []string{"task", "ready", "--project", "tok", "--json"}); err != nil {
		t.Fatalf("task ready --json returned error: %v", err)
	}
	var readyTasks []readyTaskOutput
	if err := json.Unmarshal(readyOut.Bytes(), &readyTasks); err != nil {
		t.Fatalf("parse ready JSON: %v\n%s", err, readyOut.String())
	}
	if len(readyTasks) != 1 || readyTasks[0].ID != blockerID {
		t.Fatalf("expected only blocker to be ready, got %+v", readyTasks)
	}

	var claimOut bytes.Buffer
	claimCLI := newProjectTestCLI(dataDir, &claimOut)
	if err := claimCLI.Run(ctx, []string{"task", "claim", "--project", "tok", "--json"}); err != nil {
		t.Fatalf("task claim --json returned error: %v", err)
	}
	var claimed readyTaskOutput
	if err := json.Unmarshal(claimOut.Bytes(), &claimed); err != nil {
		t.Fatalf("parse claimed JSON: %v\n%s", err, claimOut.String())
	}
	if claimed.ID != blockerID || claimed.Status != "in_progress" {
		t.Fatalf("unexpected claimed task: %+v", claimed)
	}

	indexCLI := newProjectTestCLI(dataDir, &bytes.Buffer{})
	if err := indexCLI.Run(ctx, []string{"index", "update", "--project", "tok"}); err != nil {
		t.Fatalf("index update returned error: %v", err)
	}

	contextPath := filepath.Join(t.TempDir(), "context.md")
	var contextOut bytes.Buffer
	contextCLI := newProjectTestCLI(dataDir, &contextOut)
	if err := contextCLI.Run(ctx, []string{
		"context", "build",
		"--project", "tok",
		"--task", strconv.FormatInt(blockerID, 10),
		"--query", "workflow context",
		"--output", contextPath,
	}); err != nil {
		t.Fatalf("context build --output returned error: %v", err)
	}
	if !strings.Contains(contextOut.String(), "wrote context package: "+contextPath) {
		t.Fatalf("unexpected context build stdout:\n%s", contextOut.String())
	}
	contextContent, err := os.ReadFile(contextPath)
	if err != nil {
		t.Fatalf("ReadFile context path returned error: %v", err)
	}
	if !strings.Contains(string(contextContent), "workflow.go") || !strings.Contains(string(contextContent), "buildContextPackage") {
		t.Fatalf("context package missing workflow retrieval result:\n%s", string(contextContent))
	}
	for _, want := range []string{
		"contract_version: tok.handoff.v0",
		"## Current State",
		"## Relevant Files",
		"## Commands",
		"## Open Questions",
	} {
		if !strings.Contains(string(contextContent), want) {
			t.Fatalf("context package missing handoff section %q:\n%s", want, string(contextContent))
		}
	}

	var contextJSONOut bytes.Buffer
	contextJSONCLI := newProjectTestCLI(dataDir, &contextJSONOut)
	if err := contextJSONCLI.Run(ctx, []string{
		"context", "build",
		"--project", "tok",
		"--task", strconv.FormatInt(blockerID, 10),
		"--query", "workflow context",
		"--json",
	}); err != nil {
		t.Fatalf("context build --json returned error: %v", err)
	}
	var handoff contextPackageOutput
	if err := json.Unmarshal(contextJSONOut.Bytes(), &handoff); err != nil {
		t.Fatalf("parse context package JSON: %v\n%s", err, contextJSONOut.String())
	}
	if handoff.ContractVersion != "tok.handoff.v0" || handoff.Task.ID != blockerID || len(handoff.RetrievalResults) == 0 || len(handoff.SuggestedCommands) == 0 {
		t.Fatalf("unexpected handoff JSON: %+v", handoff)
	}

	progressCLI := newProjectTestCLI(dataDir, &bytes.Buffer{})
	if err := progressCLI.Run(ctx, []string{"task", "progress", strconv.FormatInt(blockerID, 10), "--body", "Handoff package reviewed."}); err != nil {
		t.Fatalf("task progress returned error: %v", err)
	}

	doneCLI := newProjectTestCLI(dataDir, &bytes.Buffer{})
	if err := doneCLI.Run(ctx, []string{"task", "done", strconv.FormatInt(blockerID, 10), "--note", "Prepared context package."}); err != nil {
		t.Fatalf("task done blocker returned error: %v", err)
	}

	readyOut.Reset()
	readyCLI = newProjectTestCLI(dataDir, &readyOut)
	if err := readyCLI.Run(ctx, []string{"task", "ready", "--project", "tok", "--json"}); err != nil {
		t.Fatalf("task ready after blocker done returned error: %v", err)
	}
	if err := json.Unmarshal(readyOut.Bytes(), &readyTasks); err != nil {
		t.Fatalf("parse ready JSON after blocker done: %v\n%s", err, readyOut.String())
	}
	if len(readyTasks) != 1 || readyTasks[0].ID != blockedID {
		t.Fatalf("expected blocked task to become ready, got %+v", readyTasks)
	}

	claimBlockedCLI := newProjectTestCLI(dataDir, &bytes.Buffer{})
	if err := claimBlockedCLI.Run(ctx, []string{"task", "claim", "--project", "tok", strconv.FormatInt(blockedID, 10)}); err != nil {
		t.Fatalf("claim unblocked task returned error: %v", err)
	}

	blockCLI := newProjectTestCLI(dataDir, &bytes.Buffer{})
	if err := blockCLI.Run(ctx, []string{"task", "block", strconv.FormatInt(blockedID, 10), "--reason", "Need handoff review."}); err != nil {
		t.Fatalf("task block returned error: %v", err)
	}
	readyOut.Reset()
	readyCLI = newProjectTestCLI(dataDir, &readyOut)
	if err := readyCLI.Run(ctx, []string{"task", "ready", "--project", "tok", "--json"}); err != nil {
		t.Fatalf("task ready after block returned error: %v", err)
	}
	if err := json.Unmarshal(readyOut.Bytes(), &readyTasks); err != nil {
		t.Fatalf("parse ready JSON after block: %v\n%s", err, readyOut.String())
	}
	if len(readyTasks) != 0 {
		t.Fatalf("blocked task should not be ready, got %+v", readyTasks)
	}

	unblockCLI := newProjectTestCLI(dataDir, &bytes.Buffer{})
	if err := unblockCLI.Run(ctx, []string{"task", "unblock", strconv.FormatInt(blockedID, 10), "--note", "Review complete."}); err != nil {
		t.Fatalf("task unblock returned error: %v", err)
	}
	claimBlockedAgainCLI := newProjectTestCLI(dataDir, &bytes.Buffer{})
	if err := claimBlockedAgainCLI.Run(ctx, []string{"task", "claim", "--project", "tok", strconv.FormatInt(blockedID, 10)}); err != nil {
		t.Fatalf("claim unblocked task again returned error: %v", err)
	}

	doneBlockedCLI := newProjectTestCLI(dataDir, &bytes.Buffer{})
	if err := doneBlockedCLI.Run(ctx, []string{"task", "done", strconv.FormatInt(blockedID, 10), "--note", "Workflow completed."}); err != nil {
		t.Fatalf("task done blocked returned error: %v", err)
	}

	var showOut bytes.Buffer
	showCLI := newProjectTestCLI(dataDir, &showOut)
	if err := showCLI.Run(ctx, []string{"task", "show", strconv.FormatInt(blockedID, 10), "--json"}); err != nil {
		t.Fatalf("task show --json returned error: %v", err)
	}
	var shown taskShowOutput
	if err := json.Unmarshal(showOut.Bytes(), &shown); err != nil {
		t.Fatalf("parse task show JSON: %v\n%s", err, showOut.String())
	}
	if shown.Task.Status != "done" {
		t.Fatalf("expected final task to be done, got %+v", shown.Task)
	}
	wantEvents := []string{"created", "claimed", "blocked", "unblocked", "claimed", "completed"}
	if len(shown.Events) != len(wantEvents) {
		t.Fatalf("unexpected final task events: %+v", shown.Events)
	}
	for idx, want := range wantEvents {
		if shown.Events[idx].Type != want {
			t.Fatalf("unexpected final task event %d: got %+v want %s", idx, shown.Events[idx], want)
		}
	}
}
