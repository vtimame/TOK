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
	if started.LeaseOwner == "" || started.HeartbeatAt == "" || started.ExpiresAt == "" {
		t.Fatalf("started run missing lease fields: %+v", started)
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
		"--allow-unvalidated",
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

func TestCLIRunExecSuccess(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skipf("sh is required for run exec test: %v", err)
	}
	ctx := context.Background()
	dataDir := t.TempDir()
	projectDir := t.TempDir()
	initialHead := initRunTestGitRepo(t, projectDir)

	projectCLI := newProjectTestCLI(dataDir, &bytes.Buffer{})
	if err := projectCLI.Run(ctx, []string{"project", "add", projectDir, "--name", "tok"}); err != nil {
		t.Fatalf("project add returned error: %v", err)
	}
	taskID := createTaskForTest(t, ctx, dataDir, "tok", "Run exec task")

	var execOut bytes.Buffer
	execCLI := newProjectTestCLI(dataDir, &execOut)
	if err := execCLI.Run(ctx, []string{
		"run", "exec",
		"--task", strconv.FormatInt(taskID, 10),
		"--limit", "3",
		"--timeout", "2s",
		"--limit-bytes", "64",
		"--json",
		"--",
		"sh", "-c", "printf exec-out; printf exec-err >&2",
	}); err != nil {
		t.Fatalf("run exec --json returned error: %v", err)
	}
	var run runOutput
	if err := json.Unmarshal(execOut.Bytes(), &run); err != nil {
		t.Fatalf("parse run exec JSON: %v\n%s", err, execOut.String())
	}
	if run.ID == 0 || run.TaskID != taskID || run.Status != "succeeded" || run.FinishedAt == "" {
		t.Fatalf("unexpected run exec output: %+v", run)
	}
	if run.BaseBranch != "main" || run.BaseHead != initialHead || run.RetrievalLimit != 3 {
		t.Fatalf("run exec did not preserve git/context snapshot: %+v", run)
	}
	if run.ResultSummary != "Exec succeeded." {
		t.Fatalf("unexpected run exec summary: %+v", run)
	}
	if len(run.Artifacts) != 4 {
		t.Fatalf("expected handoff, stdout, stderr and log artifacts, got %+v", run.Artifacts)
	}
	if run.Artifacts[0].Kind != "handoff" || run.Artifacts[1].Kind != "stdout" || run.Artifacts[2].Kind != "stderr" || run.Artifacts[3].Kind != "log" {
		t.Fatalf("unexpected run exec artifacts: %+v", run.Artifacts)
	}
	handoffContent, err := os.ReadFile(run.Artifacts[0].Path)
	if err != nil {
		t.Fatalf("read run exec handoff artifact: %v", err)
	}
	if !strings.Contains(string(handoffContent), "Run exec task") || run.Artifacts[0].ContentHash != sha256ContentHash(string(handoffContent)) {
		t.Fatalf("unexpected handoff artifact: %+v content=%s", run.Artifacts[0], string(handoffContent))
	}
	stdoutContent, err := os.ReadFile(run.Artifacts[1].Path)
	if err != nil {
		t.Fatalf("read run exec stdout artifact: %v", err)
	}
	if string(stdoutContent) != "exec-out" || run.Artifacts[1].ContentHash != sha256ContentHash("exec-out") {
		t.Fatalf("unexpected stdout artifact: %+v content=%q", run.Artifacts[1], string(stdoutContent))
	}
	stderrContent, err := os.ReadFile(run.Artifacts[2].Path)
	if err != nil {
		t.Fatalf("read run exec stderr artifact: %v", err)
	}
	if string(stderrContent) != "exec-err" || run.Artifacts[2].ContentHash != sha256ContentHash("exec-err") {
		t.Fatalf("unexpected stderr artifact: %+v content=%q", run.Artifacts[2], string(stderrContent))
	}
	var metadata struct {
		Source           string `json:"source"`
		Status           string `json:"status"`
		RunStatus        string `json:"run_status"`
		ExitCode         int    `json:"exit_code"`
		TimedOut         bool   `json:"timed_out"`
		PID              int    `json:"pid"`
		ProcessGroupID   int    `json:"process_group_id"`
		SessionID        int    `json:"session_id"`
		ProcessGroup     bool   `json:"process_group"`
		StdoutArtifactID int64  `json:"stdout_artifact_id"`
		StderrArtifactID int64  `json:"stderr_artifact_id"`
		Safety           struct {
			EnvPolicy string `json:"env_policy"`
		} `json:"safety"`
	}
	if err := json.Unmarshal([]byte(run.Artifacts[3].Metadata), &metadata); err != nil {
		t.Fatalf("parse run exec metadata: %v\n%s", err, run.Artifacts[3].Metadata)
	}
	if metadata.Source != "run exec" || metadata.Status != "passed" || metadata.RunStatus != "succeeded" || metadata.ExitCode != 0 || metadata.TimedOut {
		t.Fatalf("unexpected run exec metadata: %+v", metadata)
	}
	if metadata.PID <= 0 || metadata.ProcessGroupID <= 0 || !metadata.ProcessGroup || metadata.StdoutArtifactID != run.Artifacts[1].ID || metadata.StderrArtifactID != run.Artifacts[2].ID {
		t.Fatalf("run exec metadata missing process/artifact references: %+v", metadata)
	}
	if metadata.Safety.EnvPolicy != "filtered" {
		t.Fatalf("run exec did not record safety metadata: %+v", metadata)
	}

	var taskOut bytes.Buffer
	taskCLI := newProjectTestCLI(dataDir, &taskOut)
	if err := taskCLI.Run(ctx, []string{"task", "show", strconv.FormatInt(taskID, 10), "--json"}); err != nil {
		t.Fatalf("task show returned error: %v", err)
	}
	var task taskShowOutput
	if err := json.Unmarshal(taskOut.Bytes(), &task); err != nil {
		t.Fatalf("parse task show JSON: %v\n%s", err, taskOut.String())
	}
	if task.Task.Status != "open" {
		t.Fatalf("run exec should not close task, got %+v", task.Task)
	}
}

func TestCLIRunExecCurrentBehaviorSucceedsWithoutValidationArtifactExpectedToChange(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skipf("sh is required for run exec test: %v", err)
	}
	ctx := context.Background()
	dataDir := t.TempDir()
	projectDir := t.TempDir()
	initRunTestGitRepo(t, projectDir)

	projectCLI := newProjectTestCLI(dataDir, &bytes.Buffer{})
	if err := projectCLI.Run(ctx, []string{"project", "add", projectDir, "--name", "tok"}); err != nil {
		t.Fatalf("project add returned error: %v", err)
	}
	taskID := createTaskForTest(t, ctx, dataDir, "tok", "Run exec hidden bypass")

	var execOut bytes.Buffer
	execCLI := newProjectTestCLI(dataDir, &execOut)
	if err := execCLI.Run(ctx, []string{
		"run", "exec",
		"--task", strconv.FormatInt(taskID, 10),
		"--json",
		"--",
		"sh", "-c", "true",
	}); err != nil {
		t.Fatalf("current run exec returned error: %v", err)
	}
	var run runOutput
	if err := json.Unmarshal(execOut.Bytes(), &run); err != nil {
		t.Fatalf("parse run exec JSON: %v\n%s", err, execOut.String())
	}
	if run.Status != "succeeded" {
		t.Fatalf("unexpected current run exec status: %+v", run)
	}
	for _, artifact := range run.Artifacts {
		if artifact.Kind == "validation" {
			t.Fatalf("current characterization expected no validation artifact, got %+v", run.Artifacts)
		}
	}

	// Expected-to-change in task 154: run exec should become a
	// validation-producing command instead of relying on hidden
	// AllowUnvalidated.
	if len(run.Artifacts) != 4 || run.Artifacts[3].Kind != "log" {
		t.Fatalf("unexpected current run exec artifacts: %+v", run.Artifacts)
	}
}

func TestCLIRunExecFailureAndTimeout(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skipf("sh is required for run exec test: %v", err)
	}
	ctx := context.Background()
	dataDir := t.TempDir()
	projectDir := t.TempDir()

	projectCLI := newProjectTestCLI(dataDir, &bytes.Buffer{})
	if err := projectCLI.Run(ctx, []string{"project", "add", projectDir, "--name", "tok"}); err != nil {
		t.Fatalf("project add returned error: %v", err)
	}
	failureTaskID := createTaskForTest(t, ctx, dataDir, "tok", "Run exec failure task")
	var failureOut bytes.Buffer
	failureCLI := newProjectTestCLI(dataDir, &failureOut)
	if err := failureCLI.Run(ctx, []string{
		"run", "exec",
		"--task", strconv.FormatInt(failureTaskID, 10),
		"--json",
		"--",
		"sh", "-c", "printf bad; exit 7",
	}); err != nil {
		t.Fatalf("run exec failure should return run JSON, got error: %v", err)
	}
	var failed runOutput
	if err := json.Unmarshal(failureOut.Bytes(), &failed); err != nil {
		t.Fatalf("parse failed run exec JSON: %v\n%s", err, failureOut.String())
	}
	if failed.Status != "failed" || failed.ResultSummary != "Exec failed with exit code 7." || len(failed.Artifacts) != 4 {
		t.Fatalf("unexpected failed run exec output: %+v", failed)
	}
	var failedMetadata struct {
		Status    string `json:"status"`
		RunStatus string `json:"run_status"`
		ExitCode  int    `json:"exit_code"`
	}
	if err := json.Unmarshal([]byte(failed.Artifacts[3].Metadata), &failedMetadata); err != nil {
		t.Fatalf("parse failed run exec metadata: %v\n%s", err, failed.Artifacts[3].Metadata)
	}
	if failedMetadata.Status != "failed" || failedMetadata.RunStatus != "failed" || failedMetadata.ExitCode != 7 {
		t.Fatalf("unexpected failed run exec metadata: %+v", failedMetadata)
	}

	timeoutTaskID := createTaskForTest(t, ctx, dataDir, "tok", "Run exec timeout task")
	var timeoutOut bytes.Buffer
	timeoutCLI := newProjectTestCLI(dataDir, &timeoutOut)
	if err := timeoutCLI.Run(ctx, []string{
		"run", "exec",
		"--task", strconv.FormatInt(timeoutTaskID, 10),
		"--timeout", "50ms",
		"--json",
		"--",
		"sh", "-c", "trap 'printf term >&2; exit 143' TERM; sleep 2",
	}); err != nil {
		t.Fatalf("run exec timeout should return run JSON, got error: %v", err)
	}
	var timedOut runOutput
	if err := json.Unmarshal(timeoutOut.Bytes(), &timedOut); err != nil {
		t.Fatalf("parse timeout run exec JSON: %v\n%s", err, timeoutOut.String())
	}
	if timedOut.Status != "cancelled" || timedOut.ResultSummary != "Exec timed out after 50ms." || len(timedOut.Artifacts) != 4 {
		t.Fatalf("unexpected timeout run exec output: %+v", timedOut)
	}
	var timeoutMetadata struct {
		Status    string `json:"status"`
		RunStatus string `json:"run_status"`
		TimedOut  bool   `json:"timed_out"`
		Signal    string `json:"signal"`
	}
	if err := json.Unmarshal([]byte(timedOut.Artifacts[3].Metadata), &timeoutMetadata); err != nil {
		t.Fatalf("parse timeout run exec metadata: %v\n%s", err, timedOut.Artifacts[3].Metadata)
	}
	if timeoutMetadata.Status != "cancelled" || timeoutMetadata.RunStatus != "cancelled" || !timeoutMetadata.TimedOut || timeoutMetadata.Signal != "SIGTERM" {
		t.Fatalf("unexpected timeout run exec metadata: %+v", timeoutMetadata)
	}
	stderrContent, err := os.ReadFile(timedOut.Artifacts[2].Path)
	if err != nil {
		t.Fatalf("read timeout stderr artifact: %v", err)
	}
	if !strings.Contains(string(stderrContent), "term") {
		t.Fatalf("timeout should forward SIGTERM to process group, stderr=%q", string(stderrContent))
	}
}

func TestCLIRunAgentAdapterContextModesAndResultMapping(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skipf("sh is required for run agent test: %v", err)
	}
	ctx := context.Background()
	fakeAdapter := writeFakeAgentAdapter(t)

	for _, mode := range []string{"file", "stdin", "env"} {
		t.Run(mode, func(t *testing.T) {
			dataDir := t.TempDir()
			projectDir := t.TempDir()
			initialHead := initRunTestGitRepo(t, projectDir)

			projectCLI := newProjectTestCLI(dataDir, &bytes.Buffer{})
			if err := projectCLI.Run(ctx, []string{"project", "add", projectDir, "--name", "tok"}); err != nil {
				t.Fatalf("project add returned error: %v", err)
			}
			title := "Agent adapter " + mode + " task"
			taskID := createTaskForTest(t, ctx, dataDir, "tok", title)

			var agentOut bytes.Buffer
			agentCLI := newProjectTestCLI(dataDir, &agentOut)
			if err := agentCLI.Run(ctx, []string{
				"run", "agent",
				"--task", strconv.FormatInt(taskID, 10),
				"--limit", "2",
				"--context", mode,
				"--timeout", "2s",
				"--limit-bytes", "64",
				"--json",
				"--",
				fakeAdapter, title, "succeeded", "Adapter completed.", "0",
			}); err != nil {
				t.Fatalf("run agent --json returned error: %v", err)
			}
			var run runOutput
			if err := json.Unmarshal(agentOut.Bytes(), &run); err != nil {
				t.Fatalf("parse run agent JSON: %v\n%s", err, agentOut.String())
			}
			if run.ID == 0 || run.TaskID != taskID || run.Status != "succeeded" || run.ResultSummary != "Adapter completed." {
				t.Fatalf("unexpected run agent output: %+v", run)
			}
			if run.BaseBranch != "main" || run.BaseHead != initialHead || run.RetrievalLimit != 2 {
				t.Fatalf("run agent did not preserve git/context snapshot: %+v", run)
			}
			if len(run.Artifacts) != 4 {
				t.Fatalf("expected handoff, stdout, stderr and log artifacts, got %+v", run.Artifacts)
			}
			if run.Artifacts[0].Kind != "handoff" || run.Artifacts[1].Kind != "stdout" || run.Artifacts[2].Kind != "stderr" || run.Artifacts[3].Kind != "log" {
				t.Fatalf("unexpected run agent artifacts: %+v", run.Artifacts)
			}
			stdoutContent, err := os.ReadFile(run.Artifacts[1].Path)
			if err != nil {
				t.Fatalf("read run agent stdout artifact: %v", err)
			}
			if string(stdoutContent) != "adapter-out" {
				t.Fatalf("unexpected run agent stdout: %q", string(stdoutContent))
			}
			stderrContent, err := os.ReadFile(run.Artifacts[2].Path)
			if err != nil {
				t.Fatalf("read run agent stderr artifact: %v", err)
			}
			if string(stderrContent) != "adapter-err" {
				t.Fatalf("unexpected run agent stderr: %q", string(stderrContent))
			}

			var metadata struct {
				Source           string `json:"source"`
				AdapterContract  string `json:"adapter_contract"`
				Status           string `json:"status"`
				RunStatus        string `json:"run_status"`
				ContextMode      string `json:"context_mode"`
				ContextFile      string `json:"context_file"`
				ResultFile       string `json:"result_file"`
				ResultRead       bool   `json:"result_read"`
				ArtifactDir      string `json:"artifact_dir"`
				HandoffArtifact  int64  `json:"handoff_artifact_id"`
				StdoutArtifactID int64  `json:"stdout_artifact_id"`
				StderrArtifactID int64  `json:"stderr_artifact_id"`
				Safety           struct {
					EnvNames []string `json:"env_names"`
				} `json:"safety"`
			}
			if err := json.Unmarshal([]byte(run.Artifacts[3].Metadata), &metadata); err != nil {
				t.Fatalf("parse run agent metadata: %v\n%s", err, run.Artifacts[3].Metadata)
			}
			if metadata.Source != "run agent" || metadata.AdapterContract != "tok.agent_adapter.v0" || metadata.Status != "succeeded" || metadata.RunStatus != "succeeded" || metadata.ContextMode != mode || !metadata.ResultRead {
				t.Fatalf("unexpected run agent metadata: %+v", metadata)
			}
			if metadata.ArtifactDir == "" || metadata.ResultFile == "" || metadata.HandoffArtifact != run.Artifacts[0].ID || metadata.StdoutArtifactID != run.Artifacts[1].ID || metadata.StderrArtifactID != run.Artifacts[2].ID {
				t.Fatalf("run agent metadata missing contract paths/artifact refs: %+v", metadata)
			}
			if mode == "file" && metadata.ContextFile != run.Artifacts[0].Path {
				t.Fatalf("run agent file context should reference handoff path, got %+v", metadata)
			}
			for _, want := range []string{"TOK_AGENT_ADAPTER_CONTRACT", "TOK_AGENT_CONTEXT_MODE", "TOK_AGENT_RESULT_FILE", "TOK_RUN_ARTIFACT_DIR", "TOK_PROJECT_PATH"} {
				if !containsString(metadata.Safety.EnvNames, want) {
					t.Fatalf("expected env %s in adapter env: %+v", want, metadata.Safety.EnvNames)
				}
			}
		})
	}

	dataDir := t.TempDir()
	projectDir := t.TempDir()
	projectCLI := newProjectTestCLI(dataDir, &bytes.Buffer{})
	if err := projectCLI.Run(ctx, []string{"project", "add", projectDir, "--name", "tok"}); err != nil {
		t.Fatalf("project add returned error: %v", err)
	}
	taskID := createTaskForTest(t, ctx, dataDir, "tok", "Blocked adapter task")
	var blockedOut bytes.Buffer
	blockedCLI := newProjectTestCLI(dataDir, &blockedOut)
	if err := blockedCLI.Run(ctx, []string{
		"run", "agent",
		"--task", strconv.FormatInt(taskID, 10),
		"--json",
		"--",
		fakeAdapter, "Blocked adapter task", "blocked", "Adapter requested follow-up.", "17",
	}); err != nil {
		t.Fatalf("run agent blocked result returned error: %v", err)
	}
	var blocked runOutput
	if err := json.Unmarshal(blockedOut.Bytes(), &blocked); err != nil {
		t.Fatalf("parse blocked run agent JSON: %v\n%s", err, blockedOut.String())
	}
	if blocked.Status != "blocked" || blocked.ResultSummary != "Adapter requested follow-up." {
		t.Fatalf("adapter JSON result should map run outcome without parsing stdout/stderr: %+v", blocked)
	}
}

func TestCLIRunAgentCurrentBehaviorSucceedsWithoutValidationArtifactExpectedToChange(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skipf("sh is required for run agent test: %v", err)
	}
	ctx := context.Background()
	dataDir := t.TempDir()
	projectDir := t.TempDir()
	initRunTestGitRepo(t, projectDir)
	fakeAdapter := writeFakeAgentAdapter(t)

	projectCLI := newProjectTestCLI(dataDir, &bytes.Buffer{})
	if err := projectCLI.Run(ctx, []string{"project", "add", projectDir, "--name", "tok"}); err != nil {
		t.Fatalf("project add returned error: %v", err)
	}
	taskID := createTaskForTest(t, ctx, dataDir, "tok", "Run agent hidden bypass")

	var agentOut bytes.Buffer
	agentCLI := newProjectTestCLI(dataDir, &agentOut)
	if err := agentCLI.Run(ctx, []string{
		"run", "agent",
		"--task", strconv.FormatInt(taskID, 10),
		"--json",
		"--",
		fakeAdapter, "Run agent hidden bypass", "succeeded", "Adapter completed.", "0",
	}); err != nil {
		t.Fatalf("current run agent returned error: %v", err)
	}
	var run runOutput
	if err := json.Unmarshal(agentOut.Bytes(), &run); err != nil {
		t.Fatalf("parse run agent JSON: %v\n%s", err, agentOut.String())
	}
	if run.Status != "succeeded" {
		t.Fatalf("unexpected current run agent status: %+v", run)
	}
	for _, artifact := range run.Artifacts {
		if artifact.Kind == "validation" {
			t.Fatalf("current characterization expected no validation artifact, got %+v", run.Artifacts)
		}
	}

	// Expected-to-change in task 154: agent adapter success should not be
	// treated as validation evidence by itself.
	if len(run.Artifacts) != 4 || run.Artifacts[3].Kind != "log" {
		t.Fatalf("unexpected current run agent artifacts: %+v", run.Artifacts)
	}
}

func writeFakeAgentAdapter(t *testing.T) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "fake-agent-adapter.sh")
	script := `#!/bin/sh
set -eu
want="$1"
status="$2"
summary="$3"
exit_code="$4"

[ "$TOK_AGENT_ADAPTER_CONTRACT" = "tok.agent_adapter.v0" ] || exit 21
[ "$PWD" = "$TOK_PROJECT_PATH" ] || exit 22
[ -d "$TOK_RUN_ARTIFACT_DIR" ] || exit 23
[ -n "$TOK_AGENT_RESULT_FILE" ] || exit 24

case "$TOK_AGENT_CONTEXT_MODE" in
  file)
    [ -f "$TOK_AGENT_CONTEXT_FILE" ] || exit 25
    context="$(cat "$TOK_AGENT_CONTEXT_FILE")"
    ;;
  stdin)
    context="$(cat)"
    ;;
  env)
    context="$TOK_AGENT_CONTEXT"
    ;;
  *)
    exit 26
    ;;
esac

case "$context" in
  *"$want"*) ;;
  *) printf 'missing expected context' >&2; exit 27 ;;
esac

printf 'adapter-out'
printf 'adapter-err' >&2
printf '{"status":"%s","summary":"%s"}\n' "$status" "$summary" > "$TOK_AGENT_RESULT_FILE"
exit "$exit_code"
`
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake adapter: %v", err)
	}
	return path
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
	if a.ID != b.ID || a.RunID != b.RunID || a.Kind != b.Kind || a.Path != b.Path || a.ContentHash != b.ContentHash || a.SizeBytes != b.SizeBytes || a.Truncated != b.Truncated || a.Metadata != b.Metadata || a.CreatedAt != b.CreatedAt {
		return false
	}
	if a.Actor == nil || b.Actor == nil {
		return a.Actor == nil && b.Actor == nil
	}
	return *a.Actor == *b.Actor
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func TestCLIRunRecordFileArtifacts(t *testing.T) {
	ctx := context.Background()
	dataDir := t.TempDir()
	projectDir := t.TempDir()

	projectCLI := newProjectTestCLI(dataDir, &bytes.Buffer{})
	if err := projectCLI.Run(ctx, []string{"project", "add", projectDir, "--name", "tok"}); err != nil {
		t.Fatalf("project add returned error: %v", err)
	}
	taskID := createTaskForTest(t, ctx, dataDir, "tok", "File artifact task")
	started := startRunForTest(t, ctx, dataDir, taskID)

	inputDir := t.TempDir()
	stdoutInput := filepath.Join(inputDir, "stdout.txt")
	if err := os.WriteFile(stdoutInput, []byte("hello world"), 0o644); err != nil {
		t.Fatalf("write stdout input: %v", err)
	}
	stderrInput := filepath.Join(inputDir, "stderr.txt")
	if err := os.WriteFile(stderrInput, []byte("err"), 0o644); err != nil {
		t.Fatalf("write stderr input: %v", err)
	}

	var stdoutOut bytes.Buffer
	stdoutCLI := newProjectTestCLI(dataDir, &stdoutOut)
	if err := stdoutCLI.Run(ctx, []string{
		"run", "record-artifact",
		strconv.FormatInt(started.ID, 10),
		"--kind", "stdout",
		"--input", stdoutInput,
		"--limit-bytes", "5",
		"--json",
	}); err != nil {
		t.Fatalf("run record-artifact stdout returned error: %v", err)
	}
	var stdoutArtifact runArtifactOutput
	if err := json.Unmarshal(stdoutOut.Bytes(), &stdoutArtifact); err != nil {
		t.Fatalf("parse stdout artifact JSON: %v\n%s", err, stdoutOut.String())
	}
	artifactDir := filepath.Join(dataDir, "run-artifacts", "run-"+strconv.FormatInt(started.ID, 10))
	if stdoutArtifact.Kind != "stdout" || stdoutArtifact.Path != filepath.Join(artifactDir, "0001-stdout.txt") {
		t.Fatalf("unexpected stdout artifact path fields: %+v", stdoutArtifact)
	}
	if stdoutArtifact.ContentHash != sha256ContentHash("hello") || stdoutArtifact.SizeBytes != 5 || !stdoutArtifact.Truncated {
		t.Fatalf("unexpected stdout artifact metadata fields: %+v", stdoutArtifact)
	}
	stdoutContent, err := os.ReadFile(stdoutArtifact.Path)
	if err != nil {
		t.Fatalf("read stdout artifact: %v", err)
	}
	if string(stdoutContent) != "hello" {
		t.Fatalf("unexpected truncated stdout content %q", string(stdoutContent))
	}
	var stdoutMetadata struct {
		Format            string `json:"format"`
		SourcePath        string `json:"source_path"`
		SizeBytes         int64  `json:"size_bytes"`
		OriginalSizeBytes int64  `json:"original_size_bytes"`
		LimitBytes        int64  `json:"limit_bytes"`
		Truncated         bool   `json:"truncated"`
	}
	if err := json.Unmarshal([]byte(stdoutArtifact.Metadata), &stdoutMetadata); err != nil {
		t.Fatalf("parse stdout artifact metadata: %v\n%s", err, stdoutArtifact.Metadata)
	}
	if stdoutMetadata.Format != "text" || stdoutMetadata.SizeBytes != 5 || stdoutMetadata.OriginalSizeBytes != 11 || stdoutMetadata.LimitBytes != 5 || !stdoutMetadata.Truncated {
		t.Fatalf("unexpected stdout artifact metadata: %+v", stdoutMetadata)
	}

	var stderrOut bytes.Buffer
	stderrCLI := newProjectTestCLI(dataDir, &stderrOut)
	if err := stderrCLI.Run(ctx, []string{
		"run", "record-artifact",
		strconv.FormatInt(started.ID, 10),
		"--kind", "stderr",
		"--input", stderrInput,
		"--json",
	}); err != nil {
		t.Fatalf("run record-artifact stderr returned error: %v", err)
	}
	var stderrArtifact runArtifactOutput
	if err := json.Unmarshal(stderrOut.Bytes(), &stderrArtifact); err != nil {
		t.Fatalf("parse stderr artifact JSON: %v\n%s", err, stderrOut.String())
	}
	if stderrArtifact.Kind != "stderr" || stderrArtifact.Path != filepath.Join(artifactDir, "0002-stderr.txt") {
		t.Fatalf("unexpected stderr artifact path fields: %+v", stderrArtifact)
	}
	if stderrArtifact.ContentHash != sha256ContentHash("err") || stderrArtifact.SizeBytes != 3 || stderrArtifact.Truncated {
		t.Fatalf("unexpected stderr artifact metadata fields: %+v", stderrArtifact)
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
	if len(shown.Artifacts) != 2 || !sameRunArtifactOutput(shown.Artifacts[0], stdoutArtifact) || !sameRunArtifactOutput(shown.Artifacts[1], stderrArtifact) {
		t.Fatalf("run show did not preserve artifact order and metadata: %+v", shown.Artifacts)
	}
	if strings.Contains(showOut.String(), "hello world") {
		t.Fatalf("run show JSON should not inline artifact content:\n%s", showOut.String())
	}

	invalidKindCLI := newProjectTestCLI(dataDir, &bytes.Buffer{})
	err = invalidKindCLI.Run(ctx, []string{
		"run", "record-artifact",
		strconv.FormatInt(started.ID, 10),
		"--kind", "validation",
		"--input", stderrInput,
	})
	if err == nil {
		t.Fatal("expected invalid file artifact kind error")
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

func TestCLIRunHeartbeatRecoverAndActiveGuard(t *testing.T) {
	ctx := context.Background()
	dataDir := t.TempDir()
	projectDir := t.TempDir()

	projectCLI := newProjectTestCLI(dataDir, &bytes.Buffer{})
	if err := projectCLI.Run(ctx, []string{"project", "add", projectDir, "--name", "tok"}); err != nil {
		t.Fatalf("project add returned error: %v", err)
	}
	taskID := createTaskForTest(t, ctx, dataDir, "tok", "Heartbeat task")
	started := startRunForTest(t, ctx, dataDir, taskID)
	if started.LeaseOwner == "" || started.HeartbeatAt == "" || started.ExpiresAt == "" {
		t.Fatalf("run start should set lease fields: %+v", started)
	}

	duplicateCLI := newProjectTestCLI(dataDir, &bytes.Buffer{})
	err := duplicateCLI.Run(ctx, []string{"run", "start", "--task", strconv.FormatInt(taskID, 10)})
	if err == nil || !strings.Contains(err.Error(), "active run already exists for task") {
		t.Fatalf("expected active run guard error, got %v", err)
	}

	override := startRunForTestWithArgs(t, ctx, dataDir, []string{"run", "start", "--task", strconv.FormatInt(taskID, 10), "--allow-active", "--json"})
	if override.ID == started.ID {
		t.Fatalf("expected allow-active to create distinct run, got %+v", override)
	}
	finishOverrideCLI := newProjectTestCLI(dataDir, &bytes.Buffer{})
	if err := finishOverrideCLI.Run(ctx, []string{
		"run", "finish",
		strconv.FormatInt(override.ID, 10),
		"--status", "failed",
		"--summary", "Override finished.",
	}); err != nil {
		t.Fatalf("finish override returned error: %v", err)
	}

	var heartbeatOut bytes.Buffer
	heartbeatCLI := newProjectTestCLI(dataDir, &heartbeatOut)
	if err := heartbeatCLI.Run(ctx, []string{
		"run", "heartbeat",
		strconv.FormatInt(started.ID, 10),
		"--owner", "agent/test",
		"--ttl", "1ms",
		"--json",
	}); err != nil {
		t.Fatalf("run heartbeat --json returned error: %v", err)
	}
	var heartbeated runOutput
	if err := json.Unmarshal(heartbeatOut.Bytes(), &heartbeated); err != nil {
		t.Fatalf("parse heartbeat run JSON: %v\n%s", err, heartbeatOut.String())
	}
	if heartbeated.LeaseOwner != "agent/test" || heartbeated.HeartbeatAt == "" || heartbeated.ExpiresAt == "" {
		t.Fatalf("unexpected heartbeated run: %+v", heartbeated)
	}

	var recoverOut bytes.Buffer
	recoverCLI := newProjectTestCLI(dataDir, &recoverOut)
	if err := recoverCLI.Run(ctx, []string{
		"run", "recover",
		"--now", "9999-01-01T00:00:00.000Z",
		"--summary", "Recovered stale run.",
		"--json",
	}); err != nil {
		t.Fatalf("run recover --json returned error: %v", err)
	}
	var recovered []runOutput
	if err := json.Unmarshal(recoverOut.Bytes(), &recovered); err != nil {
		t.Fatalf("parse recovered runs JSON: %v\n%s", err, recoverOut.String())
	}
	if len(recovered) != 1 || recovered[0].ID != started.ID || recovered[0].Status != "cancelled" || recovered[0].ResultSummary != "Recovered stale run." {
		t.Fatalf("unexpected recovered runs: %+v", recovered)
	}

	heartbeatTerminalCLI := newProjectTestCLI(dataDir, &bytes.Buffer{})
	err = heartbeatTerminalCLI.Run(ctx, []string{
		"run", "heartbeat",
		strconv.FormatInt(started.ID, 10),
		"--owner", "agent/test",
	})
	if err == nil || !strings.Contains(err.Error(), "run cannot be heartbeated from current status") {
		t.Fatalf("expected terminal heartbeat error, got %v", err)
	}

	var emptyRecoverOut bytes.Buffer
	emptyRecoverCLI := newProjectTestCLI(dataDir, &emptyRecoverOut)
	if err := emptyRecoverCLI.Run(ctx, []string{"run", "recover", "--summary", "No stale runs."}); err != nil {
		t.Fatalf("empty run recover returned error: %v", err)
	}
	if !strings.Contains(emptyRecoverOut.String(), "no stale runs") {
		t.Fatalf("unexpected empty recover output:\n%s", emptyRecoverOut.String())
	}
}

func startRunForTest(t *testing.T, ctx context.Context, dataDir string, taskID int64) runOutput {
	t.Helper()

	return startRunForTestWithArgs(t, ctx, dataDir, []string{"run", "start", "--task", strconv.FormatInt(taskID, 10), "--json"})
}

func startRunForTestWithArgs(t *testing.T, ctx context.Context, dataDir string, args []string) runOutput {
	t.Helper()

	var out bytes.Buffer
	cli := newProjectTestCLI(dataDir, &out)
	if err := cli.Run(ctx, args); err != nil {
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

func TestCLIRunValidateCommandRecordsArtifacts(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skipf("sh is required for validation command test: %v", err)
	}
	ctx := context.Background()
	dataDir := t.TempDir()
	projectDir := t.TempDir()

	projectCLI := newProjectTestCLI(dataDir, &bytes.Buffer{})
	if err := projectCLI.Run(ctx, []string{"project", "add", projectDir, "--name", "tok"}); err != nil {
		t.Fatalf("project add returned error: %v", err)
	}
	taskID := createTaskForTest(t, ctx, dataDir, "tok", "Executable validation task")
	started := startRunForTest(t, ctx, dataDir, taskID)

	var validateOut bytes.Buffer
	validateCLI := newProjectTestCLI(dataDir, &validateOut)
	if err := validateCLI.Run(ctx, []string{
		"run", "validate",
		strconv.FormatInt(started.ID, 10),
		"--timeout", "2s",
		"--limit-bytes", "64",
		"--json",
		"--",
		"sh", "-c", `printf '\160\141\163\163\055\157\165\164'; printf '\160\141\163\163\055\145\162\162' >&2`,
	}); err != nil {
		t.Fatalf("run validate --json returned error: %v", err)
	}
	var validation runArtifactOutput
	if err := json.Unmarshal(validateOut.Bytes(), &validation); err != nil {
		t.Fatalf("parse run validate JSON: %v\n%s", err, validateOut.String())
	}
	if validation.Kind != "validation" || validation.RunID != started.ID {
		t.Fatalf("unexpected validation artifact: %+v", validation)
	}
	var validationMetadata struct {
		Command          string   `json:"command"`
		Args             []string `json:"args"`
		Status           string   `json:"status"`
		Summary          string   `json:"summary"`
		ExitCode         int      `json:"exit_code"`
		DurationMS       int64    `json:"duration_ms"`
		TimedOut         bool     `json:"timed_out"`
		TimeoutMS        int64    `json:"timeout_ms"`
		StdoutArtifactID int64    `json:"stdout_artifact_id"`
		StderrArtifactID int64    `json:"stderr_artifact_id"`
	}
	if err := json.Unmarshal([]byte(validation.Metadata), &validationMetadata); err != nil {
		t.Fatalf("parse validation metadata: %v\n%s", err, validation.Metadata)
	}
	if validationMetadata.Status != "passed" || validationMetadata.ExitCode != 0 || validationMetadata.TimedOut || validationMetadata.Summary != "Validation passed." {
		t.Fatalf("unexpected validation metadata: %+v", validationMetadata)
	}
	if len(validationMetadata.Args) != 3 || validationMetadata.Args[0] != "sh" {
		t.Fatalf("unexpected validation command metadata: %+v", validationMetadata)
	}
	if validationMetadata.TimeoutMS != 2000 || validationMetadata.StdoutArtifactID == 0 || validationMetadata.StderrArtifactID == 0 {
		t.Fatalf("validation metadata missing stream references: %+v", validationMetadata)
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
	if len(shown.Artifacts) != 3 {
		t.Fatalf("expected stdout, stderr and validation artifacts, got %+v", shown.Artifacts)
	}
	stdoutArtifact := shown.Artifacts[0]
	stderrArtifact := shown.Artifacts[1]
	if stdoutArtifact.ID != validationMetadata.StdoutArtifactID || stdoutArtifact.Kind != "stdout" {
		t.Fatalf("unexpected stdout artifact: %+v", stdoutArtifact)
	}
	if stderrArtifact.ID != validationMetadata.StderrArtifactID || stderrArtifact.Kind != "stderr" {
		t.Fatalf("unexpected stderr artifact: %+v", stderrArtifact)
	}
	stdoutContent, err := os.ReadFile(stdoutArtifact.Path)
	if err != nil {
		t.Fatalf("read stdout artifact: %v", err)
	}
	if string(stdoutContent) != "pass-out" || stdoutArtifact.ContentHash != sha256ContentHash("pass-out") || stdoutArtifact.SizeBytes != 8 || stdoutArtifact.Truncated {
		t.Fatalf("unexpected stdout artifact metadata/content: %+v content=%q", stdoutArtifact, string(stdoutContent))
	}
	stderrContent, err := os.ReadFile(stderrArtifact.Path)
	if err != nil {
		t.Fatalf("read stderr artifact: %v", err)
	}
	if string(stderrContent) != "pass-err" || stderrArtifact.ContentHash != sha256ContentHash("pass-err") || stderrArtifact.SizeBytes != 8 || stderrArtifact.Truncated {
		t.Fatalf("unexpected stderr artifact metadata/content: %+v content=%q", stderrArtifact, string(stderrContent))
	}
	if strings.Contains(showOut.String(), "pass-out") {
		t.Fatalf("run show JSON should not inline validation stdout:\n%s", showOut.String())
	}

	finishCLI := newProjectTestCLI(dataDir, &bytes.Buffer{})
	if err := finishCLI.Run(ctx, []string{
		"run", "finish",
		strconv.FormatInt(started.ID, 10),
		"--status", "succeeded",
		"--summary", "Validation passed.",
	}); err != nil {
		t.Fatalf("run finish after executable validation returned error: %v", err)
	}
}

func TestCLIRunValidateFailureAndTimeoutEvidence(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skipf("sh is required for validation command test: %v", err)
	}
	ctx := context.Background()
	dataDir := t.TempDir()
	projectDir := t.TempDir()

	projectCLI := newProjectTestCLI(dataDir, &bytes.Buffer{})
	if err := projectCLI.Run(ctx, []string{"project", "add", projectDir, "--name", "tok"}); err != nil {
		t.Fatalf("project add returned error: %v", err)
	}
	failedTaskID := createTaskForTest(t, ctx, dataDir, "tok", "Failed validation task")
	failedRun := startRunForTest(t, ctx, dataDir, failedTaskID)

	var failedOut bytes.Buffer
	failedCLI := newProjectTestCLI(dataDir, &failedOut)
	if err := failedCLI.Run(ctx, []string{
		"run", "validate",
		strconv.FormatInt(failedRun.ID, 10),
		"--json",
		"--",
		"sh", "-c", "printf bad-out; printf bad-err >&2; exit 7",
	}); err != nil {
		t.Fatalf("failed run validate should record evidence without command error: %v", err)
	}
	var failedValidation runArtifactOutput
	if err := json.Unmarshal(failedOut.Bytes(), &failedValidation); err != nil {
		t.Fatalf("parse failed validation JSON: %v\n%s", err, failedOut.String())
	}
	var failedMetadata struct {
		Status   string `json:"status"`
		ExitCode int    `json:"exit_code"`
		TimedOut bool   `json:"timed_out"`
		Summary  string `json:"summary"`
	}
	if err := json.Unmarshal([]byte(failedValidation.Metadata), &failedMetadata); err != nil {
		t.Fatalf("parse failed validation metadata: %v\n%s", err, failedValidation.Metadata)
	}
	if failedMetadata.Status != "failed" || failedMetadata.ExitCode != 7 || failedMetadata.TimedOut || failedMetadata.Summary != "Validation failed with exit code 7." {
		t.Fatalf("unexpected failed validation metadata: %+v", failedMetadata)
	}
	finishFailedCLI := newProjectTestCLI(dataDir, &bytes.Buffer{})
	err := finishFailedCLI.Run(ctx, []string{
		"run", "finish",
		strconv.FormatInt(failedRun.ID, 10),
		"--status", "succeeded",
		"--summary", "Should remain blocked by failed validation.",
	})
	if err == nil || !strings.Contains(err.Error(), "requires passed validation evidence") {
		t.Fatalf("expected failed validation to block success, got %v", err)
	}

	timeoutTaskID := createTaskForTest(t, ctx, dataDir, "tok", "Timeout validation task")
	timeoutRun := startRunForTest(t, ctx, dataDir, timeoutTaskID)
	var timeoutOut bytes.Buffer
	timeoutCLI := newProjectTestCLI(dataDir, &timeoutOut)
	if err := timeoutCLI.Run(ctx, []string{
		"run", "validate",
		strconv.FormatInt(timeoutRun.ID, 10),
		"--timeout", "50ms",
		"--json",
		"--",
		"sh", "-c", "printf started; sleep 1",
	}); err != nil {
		t.Fatalf("timeout run validate should record evidence without command error: %v", err)
	}
	var timeoutValidation runArtifactOutput
	if err := json.Unmarshal(timeoutOut.Bytes(), &timeoutValidation); err != nil {
		t.Fatalf("parse timeout validation JSON: %v\n%s", err, timeoutOut.String())
	}
	var timeoutMetadata struct {
		Status    string `json:"status"`
		TimedOut  bool   `json:"timed_out"`
		TimeoutMS int64  `json:"timeout_ms"`
		Summary   string `json:"summary"`
	}
	if err := json.Unmarshal([]byte(timeoutValidation.Metadata), &timeoutMetadata); err != nil {
		t.Fatalf("parse timeout validation metadata: %v\n%s", err, timeoutValidation.Metadata)
	}
	if timeoutMetadata.Status != "failed" || !timeoutMetadata.TimedOut || timeoutMetadata.TimeoutMS != 50 || timeoutMetadata.Summary != "Validation timed out after 50ms." {
		t.Fatalf("unexpected timeout validation metadata: %+v", timeoutMetadata)
	}
	var timeoutShowOut bytes.Buffer
	timeoutShowCLI := newProjectTestCLI(dataDir, &timeoutShowOut)
	if err := timeoutShowCLI.Run(ctx, []string{"run", "show", strconv.FormatInt(timeoutRun.ID, 10), "--json"}); err != nil {
		t.Fatalf("run show timeout returned error: %v", err)
	}
	var timeoutShown runOutput
	if err := json.Unmarshal(timeoutShowOut.Bytes(), &timeoutShown); err != nil {
		t.Fatalf("parse timeout run show JSON: %v\n%s", err, timeoutShowOut.String())
	}
	if len(timeoutShown.Artifacts) != 3 || timeoutShown.Artifacts[2].Kind != "validation" {
		t.Fatalf("timeout validation evidence not visible in run show: %+v", timeoutShown.Artifacts)
	}
}

func TestCLIRunValidateSafetyRedactionAndEnvFiltering(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skipf("sh is required for validation command test: %v", err)
	}
	t.Setenv("TOK_TEST_SECRET_TOKEN", "env-secret-value")
	t.Setenv("TOK_SECRET_PATTERNS", "pattern-secret-value")

	ctx := context.Background()
	dataDir := t.TempDir()
	projectDir := t.TempDir()

	projectCLI := newProjectTestCLI(dataDir, &bytes.Buffer{})
	if err := projectCLI.Run(ctx, []string{"project", "add", projectDir, "--name", "tok"}); err != nil {
		t.Fatalf("project add returned error: %v", err)
	}
	taskID := createTaskForTest(t, ctx, dataDir, "tok", "Safe validation task")
	started := startRunForTest(t, ctx, dataDir, taskID)

	var validateOut bytes.Buffer
	validateCLI := newProjectTestCLI(dataDir, &validateOut)
	if err := validateCLI.Run(ctx, []string{
		"run", "validate",
		strconv.FormatInt(started.ID, 10),
		"--json",
		"--",
		"sh", "-c", "printf 'token=%s run=%s path=%s pattern-secret-value' \"$TOK_TEST_SECRET_TOKEN\" \"$TOK_RUN_ID\" \"$PATH\"",
	}); err != nil {
		t.Fatalf("run validate safety returned error: %v", err)
	}
	if strings.Contains(validateOut.String(), "env-secret-value") || strings.Contains(validateOut.String(), "pattern-secret-value") {
		t.Fatalf("validation JSON leaked secret material:\n%s", validateOut.String())
	}
	var validation runArtifactOutput
	if err := json.Unmarshal(validateOut.Bytes(), &validation); err != nil {
		t.Fatalf("parse safety validation JSON: %v\n%s", err, validateOut.String())
	}
	if !strings.Contains(validation.Metadata, "[REDACTED]") {
		t.Fatalf("validation metadata did not show redaction marker:\n%s", validation.Metadata)
	}
	var metadata struct {
		Safety struct {
			EnvPolicy        string   `json:"env_policy"`
			EnvNames         []string `json:"env_names"`
			RedactionEnabled bool     `json:"redaction_enabled"`
			AllowDangerous   bool     `json:"allow_dangerous"`
		} `json:"safety"`
	}
	if err := json.Unmarshal([]byte(validation.Metadata), &metadata); err != nil {
		t.Fatalf("parse safety metadata: %v\n%s", err, validation.Metadata)
	}
	if metadata.Safety.EnvPolicy != "filtered" || !metadata.Safety.RedactionEnabled || metadata.Safety.AllowDangerous {
		t.Fatalf("unexpected safety metadata: %+v", metadata.Safety)
	}
	for _, name := range metadata.Safety.EnvNames {
		if name == "TOK_TEST_SECRET_TOKEN" {
			t.Fatalf("secret env name should not be inherited: %+v", metadata.Safety.EnvNames)
		}
	}
	for _, want := range []string{"PATH", "TOK_RUN_ID", "TOK_TASK_ID", "TOK_PROJECT_NAME"} {
		if !containsString(metadata.Safety.EnvNames, want) {
			t.Fatalf("expected env %s in filtered validation env: %+v", want, metadata.Safety.EnvNames)
		}
	}

	var showOut bytes.Buffer
	showCLI := newProjectTestCLI(dataDir, &showOut)
	if err := showCLI.Run(ctx, []string{"run", "show", strconv.FormatInt(started.ID, 10), "--json"}); err != nil {
		t.Fatalf("run show safety returned error: %v", err)
	}
	if strings.Contains(showOut.String(), "env-secret-value") || strings.Contains(showOut.String(), "pattern-secret-value") {
		t.Fatalf("run show JSON leaked secret material:\n%s", showOut.String())
	}
	var shown runOutput
	if err := json.Unmarshal(showOut.Bytes(), &shown); err != nil {
		t.Fatalf("parse safety run show JSON: %v\n%s", err, showOut.String())
	}
	stdoutContent, err := os.ReadFile(shown.Artifacts[0].Path)
	if err != nil {
		t.Fatalf("read safety stdout artifact: %v", err)
	}
	if strings.Contains(string(stdoutContent), "env-secret-value") {
		t.Fatalf("validation command inherited secret env unexpectedly: %q", string(stdoutContent))
	}
	if !strings.Contains(string(stdoutContent), "run="+strconv.FormatInt(started.ID, 10)) || !strings.Contains(string(stdoutContent), "path=") {
		t.Fatalf("validation command missing expected safe env/context: %q", string(stdoutContent))
	}
}

func TestCLIRunValidateDangerousCommandPolicy(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skipf("sh is required for validation command test: %v", err)
	}
	ctx := context.Background()
	dataDir := t.TempDir()
	projectDir := t.TempDir()

	projectCLI := newProjectTestCLI(dataDir, &bytes.Buffer{})
	if err := projectCLI.Run(ctx, []string{"project", "add", projectDir, "--name", "tok"}); err != nil {
		t.Fatalf("project add returned error: %v", err)
	}
	taskID := createTaskForTest(t, ctx, dataDir, "tok", "Dangerous validation task")
	started := startRunForTest(t, ctx, dataDir, taskID)

	rejectedCLI := newProjectTestCLI(dataDir, &bytes.Buffer{})
	err := rejectedCLI.Run(ctx, []string{
		"run", "validate",
		strconv.FormatInt(started.ID, 10),
		"--",
		"sh", "-c", "printf blocked; : rm -rf /",
	})
	if err == nil || !strings.Contains(err.Error(), "rejected dangerous command") {
		t.Fatalf("expected dangerous command rejection, got %v", err)
	}

	var showOut bytes.Buffer
	showCLI := newProjectTestCLI(dataDir, &showOut)
	if err := showCLI.Run(ctx, []string{"run", "show", strconv.FormatInt(started.ID, 10), "--json"}); err != nil {
		t.Fatalf("run show after rejected dangerous command returned error: %v", err)
	}
	var shown runOutput
	if err := json.Unmarshal(showOut.Bytes(), &shown); err != nil {
		t.Fatalf("parse run show after dangerous rejection: %v\n%s", err, showOut.String())
	}
	if len(shown.Artifacts) != 0 {
		t.Fatalf("rejected dangerous command should not create artifacts: %+v", shown.Artifacts)
	}

	var allowedOut bytes.Buffer
	allowedCLI := newProjectTestCLI(dataDir, &allowedOut)
	if err := allowedCLI.Run(ctx, []string{
		"run", "validate",
		strconv.FormatInt(started.ID, 10),
		"--allow-dangerous",
		"--json",
		"--",
		"sh", "-c", "printf allowed; : rm -rf /",
	}); err != nil {
		t.Fatalf("dangerous override validation returned error: %v", err)
	}
	var validation runArtifactOutput
	if err := json.Unmarshal(allowedOut.Bytes(), &validation); err != nil {
		t.Fatalf("parse dangerous override validation: %v\n%s", err, allowedOut.String())
	}
	var metadata struct {
		Status string `json:"status"`
		Safety struct {
			AllowDangerous    bool   `json:"allow_dangerous"`
			DangerousOverride string `json:"dangerous_override"`
		} `json:"safety"`
	}
	if err := json.Unmarshal([]byte(validation.Metadata), &metadata); err != nil {
		t.Fatalf("parse dangerous override metadata: %v\n%s", err, validation.Metadata)
	}
	if metadata.Status != "passed" || !metadata.Safety.AllowDangerous || metadata.Safety.DangerousOverride == "" {
		t.Fatalf("unexpected dangerous override metadata: %+v", metadata)
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

	missingValidationCLI := newProjectTestCLI(dataDir, &bytes.Buffer{})
	err = missingValidationCLI.Run(ctx, []string{
		"run", "finish",
		strconv.FormatInt(started.ID, 10),
		"--status", "succeeded",
		"--summary", "No validation yet.",
	})
	if err == nil || !strings.Contains(err.Error(), "requires passed validation evidence") {
		t.Fatalf("expected validation required error, got %v", err)
	}

	failedValidationCLI := newProjectTestCLI(dataDir, &bytes.Buffer{})
	if err := failedValidationCLI.Run(ctx, []string{
		"run", "record-validation",
		strconv.FormatInt(started.ID, 10),
		"--command", "go test ./...",
		"--status", "failed",
		"--summary", "Tests failed.",
	}); err != nil {
		t.Fatalf("run record-validation failed returned error: %v", err)
	}
	err = missingValidationCLI.Run(ctx, []string{
		"run", "finish",
		strconv.FormatInt(started.ID, 10),
		"--status", "succeeded",
		"--summary", "Failed validation should block success.",
	})
	if err == nil || !strings.Contains(err.Error(), "requires passed validation evidence") {
		t.Fatalf("expected failed validation to block success, got %v", err)
	}

	passedValidationCLI := newProjectTestCLI(dataDir, &bytes.Buffer{})
	if err := passedValidationCLI.Run(ctx, []string{
		"run", "record-validation",
		strconv.FormatInt(started.ID, 10),
		"--command", "go test ./...",
		"--status", "passed",
		"--summary", "Tests pass.",
	}); err != nil {
		t.Fatalf("run record-validation passed returned error: %v", err)
	}
	var finishOut bytes.Buffer
	finishCLI := newProjectTestCLI(dataDir, &finishOut)
	if err := finishCLI.Run(ctx, []string{
		"run", "finish",
		strconv.FormatInt(started.ID, 10),
		"--status", "succeeded",
		"--summary", "Validated.",
		"--json",
	}); err != nil {
		t.Fatalf("validated run finish returned error: %v", err)
	}
	var finished runOutput
	if err := json.Unmarshal(finishOut.Bytes(), &finished); err != nil {
		t.Fatalf("parse validated finish JSON: %v\n%s", err, finishOut.String())
	}
	if finished.Status != "succeeded" || len(finished.Artifacts) != 2 {
		t.Fatalf("unexpected validated finish output: %+v", finished)
	}

	// Expected-to-change in tasks 153/154: --allow-unvalidated should require
	// a non-empty override reason and leave explicit audit evidence.
	overrideTaskID := createTaskForTest(t, ctx, dataDir, "tok", "Override run validation")
	override := startRunForTest(t, ctx, dataDir, overrideTaskID)
	overrideCLI := newProjectTestCLI(dataDir, &bytes.Buffer{})
	if err := overrideCLI.Run(ctx, []string{
		"run", "finish",
		strconv.FormatInt(override.ID, 10),
		"--status", "succeeded",
		"--summary", "Explicit override.",
		"--allow-unvalidated",
	}); err != nil {
		t.Fatalf("run finish override returned error: %v", err)
	}
}
