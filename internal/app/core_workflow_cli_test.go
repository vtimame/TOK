package app

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
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
	if !sameRunArtifactOutput(finishedRun.Artifacts[1], validationArtifact) {
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
	if err := doneBlockedCLI.Run(ctx, []string{
		"task", "done",
		strconv.FormatInt(blockedID, 10),
		"--note", "Workflow completed.",
		"--allow-unvalidated",
		"--override-reason", "Workflow test covers block and unblock, not validation.",
	}); err != nil {
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
	wantEvents := []string{"created", "claimed", "blocked", "unblocked", "claimed", "completed", "completion_override"}
	if len(shown.Events) != len(wantEvents) {
		t.Fatalf("unexpected final task events: %+v", shown.Events)
	}
	for idx, want := range wantEvents {
		if shown.Events[idx].Type != want {
			t.Fatalf("unexpected final task event %d: got %+v want %s", idx, shown.Events[idx], want)
		}
	}
}

func TestCLIRunnerProductionSmokeEndToEnd(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skipf("sh is required for production runner smoke: %v", err)
	}

	t.Run("exec artifacts validate finish task done", func(t *testing.T) {
		ctx := context.Background()
		dataDir := t.TempDir()
		projectDir := t.TempDir()
		initialHead := initRunTestGitRepo(t, projectDir)
		writeContextFixtureFile(t, projectDir, "runner.txt", "production runner smoke\n")

		projectCLI := newProjectTestCLI(dataDir, &bytes.Buffer{})
		if err := projectCLI.Run(ctx, []string{"project", "add", projectDir, "--name", "tok"}); err != nil {
			t.Fatalf("project add returned error: %v", err)
		}
		taskID := createTaskForTest(t, ctx, dataDir, "tok", "Production runner task")
		claimCLI := newProjectTestCLI(dataDir, &bytes.Buffer{})
		if err := claimCLI.Run(ctx, []string{"task", "claim", "--project", "tok", strconv.FormatInt(taskID, 10)}); err != nil {
			t.Fatalf("task claim returned error: %v", err)
		}

		var execOut bytes.Buffer
		execCLI := newProjectTestCLI(dataDir, &execOut)
		if err := execCLI.Run(ctx, []string{
			"run", "exec",
			"--task", strconv.FormatInt(taskID, 10),
			"--limit", "2",
			"--timeout", "2s",
			"--json",
			"--",
			"sh", "-c", "printf prod-out; printf prod-err >&2; test \"$PWD\" = '" + projectDir + "'",
		}); err != nil {
			t.Fatalf("run exec returned error: %v", err)
		}
		var execRun runOutput
		if err := json.Unmarshal(execOut.Bytes(), &execRun); err != nil {
			t.Fatalf("parse run exec JSON: %v\n%s", err, execOut.String())
		}
		if execRun.Status != "succeeded" || execRun.ResultSummary != "Exec succeeded." || execRun.BaseHead != initialHead {
			t.Fatalf("unexpected exec run: %+v", execRun)
		}
		if len(execRun.Artifacts) != 5 || execRun.Artifacts[0].Kind != "handoff" || execRun.Artifacts[1].Kind != "stdout" || execRun.Artifacts[2].Kind != "stderr" || execRun.Artifacts[3].Kind != "validation" || execRun.Artifacts[4].Kind != "log" {
			t.Fatalf("exec run did not record handoff/stdout/stderr/validation/log artifacts: %+v", execRun.Artifacts)
		}
		assertFileContent(t, execRun.Artifacts[1].Path, "prod-out")
		assertFileContent(t, execRun.Artifacts[2].Path, "prod-err")

		manualRun := startRunForTestWithArgs(t, ctx, dataDir, []string{"run", "start", "--task", strconv.FormatInt(taskID, 10), "--allow-active", "--json"})
		var validationOut bytes.Buffer
		validationCLI := newProjectTestCLI(dataDir, &validationOut)
		if err := validationCLI.Run(ctx, []string{
			"run", "validate",
			strconv.FormatInt(manualRun.ID, 10),
			"--timeout", "2s",
			"--json",
			"--",
			"sh", "-c", "printf validation-out; printf validation-err >&2",
		}); err != nil {
			t.Fatalf("run validate returned error: %v", err)
		}
		var validation runArtifactOutput
		if err := json.Unmarshal(validationOut.Bytes(), &validation); err != nil {
			t.Fatalf("parse validation JSON: %v\n%s", err, validationOut.String())
		}
		if validation.Kind != "validation" || validation.RunID != manualRun.ID {
			t.Fatalf("unexpected validation artifact: %+v", validation)
		}

		var finishOut bytes.Buffer
		finishCLI := newProjectTestCLI(dataDir, &finishOut)
		if err := finishCLI.Run(ctx, []string{
			"run", "finish",
			strconv.FormatInt(manualRun.ID, 10),
			"--status", "succeeded",
			"--summary", "Production smoke validated.",
			"--json",
		}); err != nil {
			t.Fatalf("run finish returned error: %v", err)
		}
		var finished runOutput
		if err := json.Unmarshal(finishOut.Bytes(), &finished); err != nil {
			t.Fatalf("parse run finish JSON: %v\n%s", err, finishOut.String())
		}
		if finished.Status != "succeeded" || finished.ResultSummary != "Production smoke validated." || len(finished.Artifacts) != 3 {
			t.Fatalf("unexpected finished validation run: %+v", finished)
		}

		doneCLI := newProjectTestCLI(dataDir, &bytes.Buffer{})
		if err := doneCLI.Run(ctx, []string{"task", "done", strconv.FormatInt(taskID, 10), "--note", "Production run smoke passed."}); err != nil {
			t.Fatalf("task done returned error: %v", err)
		}
		assertTaskStatus(t, ctx, dataDir, taskID, "done")
	})

	t.Run("cancel long running exec via timeout", func(t *testing.T) {
		ctx := context.Background()
		dataDir := t.TempDir()
		projectDir := t.TempDir()
		projectCLI := newProjectTestCLI(dataDir, &bytes.Buffer{})
		if err := projectCLI.Run(ctx, []string{"project", "add", projectDir, "--name", "tok"}); err != nil {
			t.Fatalf("project add returned error: %v", err)
		}
		taskID := createTaskForTest(t, ctx, dataDir, "tok", "Cancellable runner task")

		var execOut bytes.Buffer
		execCLI := newProjectTestCLI(dataDir, &execOut)
		if err := execCLI.Run(ctx, []string{
			"run", "exec",
			"--task", strconv.FormatInt(taskID, 10),
			"--timeout", "50ms",
			"--json",
			"--",
			"sh", "-c", "trap 'printf term >&2; exit 143' TERM; sleep 10",
		}); err != nil {
			t.Fatalf("cancelled run exec should return run JSON, got error: %v", err)
		}
		var cancelled runOutput
		if err := json.Unmarshal(execOut.Bytes(), &cancelled); err != nil {
			t.Fatalf("parse cancelled exec JSON: %v\n%s", err, execOut.String())
		}
		if cancelled.Status != "cancelled" || cancelled.ResultSummary != "Exec timed out after 50ms." {
			t.Fatalf("unexpected cancelled exec run: %+v", cancelled)
		}
		if len(cancelled.Artifacts) != 5 || cancelled.Artifacts[2].Kind != "stderr" || cancelled.Artifacts[3].Kind != "validation" || cancelled.Artifacts[4].Kind != "log" {
			t.Fatalf("cancelled exec missing expected artifacts: %+v", cancelled.Artifacts)
		}
		stderrContent, err := os.ReadFile(cancelled.Artifacts[2].Path)
		if err != nil {
			t.Fatalf("read cancelled stderr artifact: %v", err)
		}
		if !strings.Contains(string(stderrContent), "term") {
			t.Fatalf("cancelled process did not receive TERM, stderr=%q", string(stderrContent))
		}
	})

	t.Run("stale heartbeat recovery", func(t *testing.T) {
		ctx := context.Background()
		dataDir := t.TempDir()
		projectDir := t.TempDir()
		projectCLI := newProjectTestCLI(dataDir, &bytes.Buffer{})
		if err := projectCLI.Run(ctx, []string{"project", "add", projectDir, "--name", "tok"}); err != nil {
			t.Fatalf("project add returned error: %v", err)
		}
		taskID := createTaskForTest(t, ctx, dataDir, "tok", "Recoverable runner task")
		started := startRunForTest(t, ctx, dataDir, taskID)

		heartbeatCLI := newProjectTestCLI(dataDir, &bytes.Buffer{})
		if err := heartbeatCLI.Run(ctx, []string{
			"run", "heartbeat",
			strconv.FormatInt(started.ID, 10),
			"--owner", "production-smoke",
			"--ttl", "1ms",
		}); err != nil {
			t.Fatalf("run heartbeat returned error: %v", err)
		}

		var recoverOut bytes.Buffer
		recoverCLI := newProjectTestCLI(dataDir, &recoverOut)
		if err := recoverCLI.Run(ctx, []string{
			"run", "recover",
			"--now", "9999-01-01T00:00:00.000Z",
			"--summary", "Recovered by production smoke.",
			"--json",
		}); err != nil {
			t.Fatalf("run recover returned error: %v", err)
		}
		var recovered []runOutput
		if err := json.Unmarshal(recoverOut.Bytes(), &recovered); err != nil {
			t.Fatalf("parse recovered JSON: %v\n%s", err, recoverOut.String())
		}
		if len(recovered) != 1 || recovered[0].ID != started.ID || recovered[0].Status != "cancelled" || recovered[0].ResultSummary != "Recovered by production smoke." {
			t.Fatalf("unexpected recovered runs: %+v", recovered)
		}
	})

	t.Run("failed validation prevents silent success", func(t *testing.T) {
		ctx := context.Background()
		dataDir := t.TempDir()
		projectDir := t.TempDir()
		projectCLI := newProjectTestCLI(dataDir, &bytes.Buffer{})
		if err := projectCLI.Run(ctx, []string{"project", "add", projectDir, "--name", "tok"}); err != nil {
			t.Fatalf("project add returned error: %v", err)
		}
		taskID := createTaskForTest(t, ctx, dataDir, "tok", "Failed validation runner task")
		started := startRunForTest(t, ctx, dataDir, taskID)

		var validationOut bytes.Buffer
		validationCLI := newProjectTestCLI(dataDir, &validationOut)
		if err := validationCLI.Run(ctx, []string{
			"run", "validate",
			strconv.FormatInt(started.ID, 10),
			"--json",
			"--",
			"sh", "-c", "printf failed-validation; exit 7",
		}); err != nil {
			t.Fatalf("failed run validate should record evidence without command error: %v", err)
		}
		var validation runArtifactOutput
		if err := json.Unmarshal(validationOut.Bytes(), &validation); err != nil {
			t.Fatalf("parse failed validation JSON: %v\n%s", err, validationOut.String())
		}
		var metadata struct {
			Status   string `json:"status"`
			ExitCode int    `json:"exit_code"`
		}
		if err := json.Unmarshal([]byte(validation.Metadata), &metadata); err != nil {
			t.Fatalf("parse failed validation metadata: %v\n%s", err, validation.Metadata)
		}
		if metadata.Status != "failed" || metadata.ExitCode != 7 {
			t.Fatalf("unexpected failed validation metadata: %+v", metadata)
		}

		finishCLI := newProjectTestCLI(dataDir, &bytes.Buffer{})
		err := finishCLI.Run(ctx, []string{
			"run", "finish",
			strconv.FormatInt(started.ID, 10),
			"--status", "succeeded",
			"--summary", "This should not silently succeed.",
		})
		if err == nil || !strings.Contains(err.Error(), "requires passed validation evidence") {
			t.Fatalf("expected failed validation to block success, got %v", err)
		}
	})
}

func assertFileContent(t *testing.T, path, want string) {
	t.Helper()

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read file %q: %v", path, err)
	}
	if string(content) != want {
		t.Fatalf("unexpected file content for %q: got %q want %q", path, string(content), want)
	}
}

func assertTaskStatus(t *testing.T, ctx context.Context, dataDir string, taskID int64, want string) {
	t.Helper()

	var out bytes.Buffer
	cli := newProjectTestCLI(dataDir, &out)
	if err := cli.Run(ctx, []string{"task", "show", strconv.FormatInt(taskID, 10), "--json"}); err != nil {
		t.Fatalf("task show returned error: %v", err)
	}
	var shown taskShowOutput
	if err := json.Unmarshal(out.Bytes(), &shown); err != nil {
		t.Fatalf("parse task show JSON: %v\n%s", err, out.String())
	}
	if shown.Task.Status != want {
		t.Fatalf("unexpected task status: got %s want %s", shown.Task.Status, want)
	}
}
