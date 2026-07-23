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

	handoffPath := filepath.Join(t.TempDir(), "run-handoff.md")
	var runStartOut bytes.Buffer
	runStartCLI := newProjectTestCLI(dataDir, &runStartOut)
	if err := runStartCLI.Run(ctx, []string{
		"run", "start",
		"--task", strconv.FormatInt(blockerID, 10),
		"--limit", "3",
		"--handoff-output", handoffPath,
		"--json",
	}); err != nil {
		t.Fatalf("run start --json returned error: %v", err)
	}
	var startedRun runOutput
	if err := json.Unmarshal(runStartOut.Bytes(), &startedRun); err != nil {
		t.Fatalf("parse run start JSON: %v\n%s", err, runStartOut.String())
	}
	if startedRun.ID == 0 || startedRun.TaskID != blockerID || startedRun.Status != "in_progress" || startedRun.HandoffContractVersion != "tok.handoff.v0" {
		t.Fatalf("unexpected started run: %+v", startedRun)
	}
	if startedRun.RetrievalLimit != 3 {
		t.Fatalf("unexpected run retrieval limit: %+v", startedRun)
	}
	handoffContent, err := os.ReadFile(handoffPath)
	if err != nil {
		t.Fatalf("ReadFile run handoff path returned error: %v", err)
	}
	if !strings.Contains(string(handoffContent), "workflow.go") || !strings.Contains(string(handoffContent), "buildContextPackage") {
		t.Fatalf("run handoff artifact missing workflow retrieval result:\n%s", string(handoffContent))
	}
	handoffHash := sha256ContentHash(string(handoffContent))
	assertHandoffArtifactOutput(t, startedRun.Artifacts, startedRun.ID, handoffPath, handoffHash)

	var runShowOut bytes.Buffer
	runShowCLI := newProjectTestCLI(dataDir, &runShowOut)
	if err := runShowCLI.Run(ctx, []string{"run", "show", strconv.FormatInt(startedRun.ID, 10), "--json"}); err != nil {
		t.Fatalf("run show --json returned error: %v", err)
	}
	var shownRun runOutput
	if err := json.Unmarshal(runShowOut.Bytes(), &shownRun); err != nil {
		t.Fatalf("parse run show JSON: %v\n%s", err, runShowOut.String())
	}
	if shownRun.ID != startedRun.ID || shownRun.TaskID != blockerID || shownRun.RetrievalLimit != startedRun.RetrievalLimit {
		t.Fatalf("unexpected shown run: %+v", shownRun)
	}
	assertHandoffArtifactOutput(t, shownRun.Artifacts, startedRun.ID, handoffPath, handoffHash)

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

	var validationOut bytes.Buffer
	validationCLI := newProjectTestCLI(dataDir, &validationOut)
	if err := validationCLI.Run(ctx, []string{
		"run", "record-validation",
		strconv.FormatInt(startedRun.ID, 10),
		"--command", "go test ./...",
		"--status", "passed",
		"--summary", "Smoke tests pass.",
		"--json",
	}); err != nil {
		t.Fatalf("run record-validation --json returned error: %v", err)
	}
	var validationArtifact runArtifactOutput
	if err := json.Unmarshal(validationOut.Bytes(), &validationArtifact); err != nil {
		t.Fatalf("parse validation artifact JSON: %v\n%s", err, validationOut.String())
	}
	if validationArtifact.Kind != "validation" || validationArtifact.RunID != startedRun.ID {
		t.Fatalf("unexpected validation artifact: %+v", validationArtifact)
	}
	var validationMetadata struct {
		Command string `json:"command"`
		Status  string `json:"status"`
		Summary string `json:"summary"`
	}
	if err := json.Unmarshal([]byte(validationArtifact.Metadata), &validationMetadata); err != nil {
		t.Fatalf("parse validation artifact metadata: %v\n%s", err, validationArtifact.Metadata)
	}
	if validationMetadata.Command != "go test ./..." || validationMetadata.Status != "passed" || validationMetadata.Summary != "Smoke tests pass." {
		t.Fatalf("unexpected validation artifact metadata: %+v", validationMetadata)
	}

	var runFinishOut bytes.Buffer
	runFinishCLI := newProjectTestCLI(dataDir, &runFinishOut)
	if err := runFinishCLI.Run(ctx, []string{
		"run", "finish",
		strconv.FormatInt(startedRun.ID, 10),
		"--status", "succeeded",
		"--summary", "Handoff package prepared.",
		"--json",
	}); err != nil {
		t.Fatalf("run finish --json returned error: %v", err)
	}
	var finishedRun runOutput
	if err := json.Unmarshal(runFinishOut.Bytes(), &finishedRun); err != nil {
		t.Fatalf("parse run finish JSON: %v\n%s", err, runFinishOut.String())
	}
	if finishedRun.Status != "succeeded" || finishedRun.FinishedAt == "" || finishedRun.ResultSummary != "Handoff package prepared." {
		t.Fatalf("unexpected finished run: %+v", finishedRun)
	}
	if len(finishedRun.Artifacts) != 2 {
		t.Fatalf("expected handoff and validation artifacts on finished run, got %+v", finishedRun.Artifacts)
	}
	assertHandoffArtifactOutput(t, finishedRun.Artifacts[:1], startedRun.ID, handoffPath, handoffHash)
	if finishedRun.Artifacts[1] != validationArtifact {
		t.Fatalf("finished run validation artifact mismatch: %+v", finishedRun.Artifacts[1])
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
