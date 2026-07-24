package app

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"s26.sh/tok/internal/config"
)

func TestCLIUserSetNameAndShow(t *testing.T) {
	ctx := context.Background()
	dataDir := t.TempDir()

	var setOut bytes.Buffer
	setCLI := newIdentityTestCLI(dataDir, &setOut)
	if err := setCLI.Run(ctx, []string{"user", "set-name", "Timur Valitiov"}); err != nil {
		t.Fatalf("user set-name returned error: %v", err)
	}
	for _, want := range []string{"kind: human", "display_name: Timur Valitiov", "created_at:", "updated_at:"} {
		if !strings.Contains(setOut.String(), want) {
			t.Fatalf("user set-name output missing %q:\n%s", want, setOut.String())
		}
	}

	var showOut bytes.Buffer
	showCLI := newIdentityTestCLI(dataDir, &showOut)
	if err := showCLI.Run(ctx, []string{"user", "show"}); err != nil {
		t.Fatalf("user show returned error: %v", err)
	}
	for _, want := range []string{"display_name: Timur Valitiov", "source: explicit"} {
		if !strings.Contains(showOut.String(), want) {
			t.Fatalf("user show output missing %q:\n%s", want, showOut.String())
		}
	}
}

func TestCLIAgentAddListAndRevoke(t *testing.T) {
	ctx := context.Background()
	dataDir := t.TempDir()

	var addOut bytes.Buffer
	addCLI := newIdentityTestCLI(dataDir, &addOut)
	if err := addCLI.Run(ctx, []string{"agent", "add", "Codex Designer"}); err != nil {
		t.Fatalf("agent add returned error: %v", err)
	}
	addOutput := addOut.String()
	for _, want := range []string{"name: Codex Designer", "status: active", "token: tok_agent_"} {
		if !strings.Contains(addOutput, want) {
			t.Fatalf("agent add output missing %q:\n%s", want, addOutput)
		}
	}
	if strings.Contains(addOutput, "sha256:") {
		t.Fatalf("agent add should not print token hash:\n%s", addOutput)
	}

	var listOut bytes.Buffer
	listCLI := newIdentityTestCLI(dataDir, &listOut)
	if err := listCLI.Run(ctx, []string{"agent", "list"}); err != nil {
		t.Fatalf("agent list returned error: %v", err)
	}
	listOutput := listOut.String()
	for _, want := range []string{"id\tstatus\tname\tcreated_at\trevoked_at", "active\tCodex Designer"} {
		if !strings.Contains(listOutput, want) {
			t.Fatalf("agent list output missing %q:\n%s", want, listOutput)
		}
	}
	if strings.Contains(listOutput, "tok_agent_") || strings.Contains(listOutput, "sha256:") {
		t.Fatalf("agent list should not print token material:\n%s", listOutput)
	}

	var revokeOut bytes.Buffer
	revokeCLI := newIdentityTestCLI(dataDir, &revokeOut)
	if err := revokeCLI.Run(ctx, []string{"agent", "revoke", "1"}); err != nil {
		t.Fatalf("agent revoke returned error: %v", err)
	}
	for _, want := range []string{"name: Codex Designer", "status: revoked", "revoked_at:"} {
		if !strings.Contains(revokeOut.String(), want) {
			t.Fatalf("agent revoke output missing %q:\n%s", want, revokeOut.String())
		}
	}
}

func TestCLIWritesActorAttributionToTaskAndRunJSON(t *testing.T) {
	ctx := context.Background()
	dataDir := t.TempDir()
	projectDir := t.TempDir()

	userCLI := newIdentityTestCLI(dataDir, &bytes.Buffer{})
	if err := userCLI.Run(ctx, []string{"user", "set-name", "TOK Operator"}); err != nil {
		t.Fatalf("user set-name returned error: %v", err)
	}

	projectCLI := newIdentityTestCLI(dataDir, &bytes.Buffer{})
	if err := projectCLI.Run(ctx, []string{"project", "add", projectDir, "--name", "tok"}); err != nil {
		t.Fatalf("project add returned error: %v", err)
	}
	taskID := createTaskForTest(t, ctx, dataDir, "tok", "Attributed CLI task")

	commentCLI := newIdentityTestCLI(dataDir, &bytes.Buffer{})
	if err := commentCLI.Run(ctx, []string{"task", "comment", strconv.FormatInt(taskID, 10), "--body", "Operator note."}); err != nil {
		t.Fatalf("task comment returned error: %v", err)
	}

	var taskOut bytes.Buffer
	taskCLI := newIdentityTestCLI(dataDir, &taskOut)
	if err := taskCLI.Run(ctx, []string{"task", "show", strconv.FormatInt(taskID, 10), "--json"}); err != nil {
		t.Fatalf("task show --json returned error: %v", err)
	}
	var shown taskShowOutput
	if err := json.Unmarshal(taskOut.Bytes(), &shown); err != nil {
		t.Fatalf("parse task show JSON: %v\n%s", err, taskOut.String())
	}
	if len(shown.Events) != 2 {
		t.Fatalf("unexpected task events: %+v", shown.Events)
	}
	for _, event := range shown.Events {
		if event.Actor == nil || event.Actor.Kind != "human" || event.Actor.Name != "TOK Operator" || event.Actor.ID == 0 {
			t.Fatalf("task event missing actor attribution: %+v", event)
		}
	}

	handoffPath := filepath.Join(t.TempDir(), "handoff.md")
	var runStartOut bytes.Buffer
	runStartCLI := newIdentityTestCLI(dataDir, &runStartOut)
	if err := runStartCLI.Run(ctx, []string{
		"run", "start",
		"--task", strconv.FormatInt(taskID, 10),
		"--handoff-output", handoffPath,
		"--json",
	}); err != nil {
		t.Fatalf("run start --json returned error: %v", err)
	}
	var started runOutput
	if err := json.Unmarshal(runStartOut.Bytes(), &started); err != nil {
		t.Fatalf("parse run start JSON: %v\n%s", err, runStartOut.String())
	}
	if started.StartedBy == nil || started.StartedBy.Kind != "human" || started.StartedBy.Name != "TOK Operator" {
		t.Fatalf("run start missing started_by actor: %+v", started)
	}
	if len(started.Artifacts) != 1 || started.Artifacts[0].Actor == nil || started.Artifacts[0].Actor.Name != "TOK Operator" {
		t.Fatalf("handoff artifact missing actor: %+v", started.Artifacts)
	}

	var validationOut bytes.Buffer
	validationCLI := newIdentityTestCLI(dataDir, &validationOut)
	if err := validationCLI.Run(ctx, []string{
		"run", "record-validation",
		strconv.FormatInt(started.ID, 10),
		"--command", "go test ./...",
		"--status", "passed",
		"--summary", "All tests pass.",
		"--json",
	}); err != nil {
		t.Fatalf("run record-validation --json returned error: %v", err)
	}
	var validation runArtifactOutput
	if err := json.Unmarshal(validationOut.Bytes(), &validation); err != nil {
		t.Fatalf("parse validation artifact JSON: %v\n%s", err, validationOut.String())
	}
	if validation.Actor == nil || validation.Actor.Name != "TOK Operator" {
		t.Fatalf("validation artifact missing actor: %+v", validation)
	}

	var finishOut bytes.Buffer
	finishCLI := newIdentityTestCLI(dataDir, &finishOut)
	if err := finishCLI.Run(ctx, []string{
		"run", "finish",
		strconv.FormatInt(started.ID, 10),
		"--status", "succeeded",
		"--summary", "Done.",
		"--json",
	}); err != nil {
		t.Fatalf("run finish --json returned error: %v", err)
	}
	var finished runOutput
	if err := json.Unmarshal(finishOut.Bytes(), &finished); err != nil {
		t.Fatalf("parse run finish JSON: %v\n%s", err, finishOut.String())
	}
	if finished.FinishedBy == nil || finished.FinishedBy.Kind != "human" || finished.FinishedBy.Name != "TOK Operator" {
		t.Fatalf("run finish missing finished_by actor: %+v", finished)
	}
}

func newIdentityTestCLI(dataDir string, out *bytes.Buffer) *CLI {
	cli := NewCLI(out, &bytes.Buffer{}, VersionInfo{})
	cli.loadCfg = func(string) (config.Config, error) {
		return config.Config{
			DataDir: dataDir,
			Log: config.LogConfig{
				Level:  "info",
				Format: "json",
			},
		}, nil
	}
	return cli
}
