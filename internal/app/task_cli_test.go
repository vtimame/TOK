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
		"source: local",
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
		"id | status | title",
		strconv.FormatInt(taskID, 10),
		"open",
		"Implement task store",
	} {
		if !strings.Contains(listOut.String(), want) {
			t.Fatalf("task list output missing %q:\n%s", want, listOut.String())
		}
	}

	var listJSONOut bytes.Buffer
	listJSONCLI := newProjectTestCLI(dataDir, &listJSONOut)
	if err := listJSONCLI.Run(ctx, []string{"task", "list", "--project", "tok", "--json"}); err != nil {
		t.Fatalf("task list --json returned error: %v", err)
	}
	var listedTasks []readyTaskOutput
	if err := json.Unmarshal(listJSONOut.Bytes(), &listedTasks); err != nil {
		t.Fatalf("parse task list JSON: %v\n%s", err, listJSONOut.String())
	}
	if len(listedTasks) != 1 ||
		listedTasks[0].ID != taskID ||
		listedTasks[0].ProjectID == 0 ||
		listedTasks[0].Status != "open" ||
		listedTasks[0].Title != "Implement task store" ||
		listedTasks[0].Description != "Create issue-like task CLI." ||
		listedTasks[0].AcceptanceCriteria != "- create\n- list\n- show\n- status" ||
		listedTasks[0].Notes != "Keep it local." ||
		listedTasks[0].Source != "local" ||
		listedTasks[0].CreatedAt == "" ||
		listedTasks[0].UpdatedAt == "" {
		t.Fatalf("unexpected task list JSON: %+v", listedTasks)
	}

	var statusOut bytes.Buffer
	statusCLI := newProjectTestCLI(dataDir, &statusOut)
	if err := statusCLI.Run(ctx, []string{"task", "status", strconv.FormatInt(taskID, 10), "blocked"}); err != nil {
		t.Fatalf("task status returned error: %v", err)
	}
	if !strings.Contains(statusOut.String(), "status: blocked") {
		t.Fatalf("unexpected task status output:\n%s", statusOut.String())
	}

	var blockedListJSONOut bytes.Buffer
	blockedListJSONCLI := newProjectTestCLI(dataDir, &blockedListJSONOut)
	if err := blockedListJSONCLI.Run(ctx, []string{"task", "list", "--project", "tok", "--status", "blocked", "--json"}); err != nil {
		t.Fatalf("task list --status blocked --json returned error: %v", err)
	}
	var blockedTasks []readyTaskOutput
	if err := json.Unmarshal(blockedListJSONOut.Bytes(), &blockedTasks); err != nil {
		t.Fatalf("parse blocked task list JSON: %v\n%s", err, blockedListJSONOut.String())
	}
	if len(blockedTasks) != 1 || blockedTasks[0].ID != taskID || blockedTasks[0].Status != "blocked" {
		t.Fatalf("unexpected blocked task list JSON: %+v", blockedTasks)
	}

	var openListJSONOut bytes.Buffer
	openListJSONCLI := newProjectTestCLI(dataDir, &openListJSONOut)
	if err := openListJSONCLI.Run(ctx, []string{"task", "list", "--project", "tok", "--status=open", "--json"}); err != nil {
		t.Fatalf("task list --status open --json returned error: %v", err)
	}
	var openTasks []readyTaskOutput
	if err := json.Unmarshal(openListJSONOut.Bytes(), &openTasks); err != nil {
		t.Fatalf("parse open task list JSON: %v\n%s", err, openListJSONOut.String())
	}
	if len(openTasks) != 0 {
		t.Fatalf("expected no open tasks after status change, got %+v", openTasks)
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

	var showJSONOut bytes.Buffer
	showJSONCLI := newProjectTestCLI(dataDir, &showJSONOut)
	if err := showJSONCLI.Run(ctx, []string{"task", "show", strconv.FormatInt(taskID, 10), "--json"}); err != nil {
		t.Fatalf("task show --json returned error: %v", err)
	}
	var showJSON taskShowOutput
	if err := json.Unmarshal(showJSONOut.Bytes(), &showJSON); err != nil {
		t.Fatalf("parse task show JSON: %v\n%s", err, showJSONOut.String())
	}
	if showJSON.Task.ID != taskID || showJSON.Task.Status != "blocked" || showJSON.Task.Title != "Implement task store" {
		t.Fatalf("unexpected task show JSON task: %+v", showJSON.Task)
	}
	if len(showJSON.Events) != 3 || showJSON.Events[0].Type != "created" || showJSON.Events[1].Type != "status_changed" || showJSON.Events[2].Type != "commented" {
		t.Fatalf("unexpected task show JSON events: %+v", showJSON.Events)
	}
}

func TestCLITaskExternalReferenceCreateShowAndUpdate(t *testing.T) {
	ctx := context.Background()
	dataDir := t.TempDir()
	projectDir := t.TempDir()

	projectCLI := newProjectTestCLI(dataDir, &bytes.Buffer{})
	if err := projectCLI.Run(ctx, []string{"project", "add", projectDir, "--name", "tok"}); err != nil {
		t.Fatalf("project add returned error: %v", err)
	}

	var createOut bytes.Buffer
	createCLI := newProjectTestCLI(dataDir, &createOut)
	if err := createCLI.Run(ctx, []string{
		"task", "create",
		"--project", "tok",
		"--title", "Linked issue",
		"--source", "github",
		"--external-id", "42",
		"--external-url", "https://github.com/vtimame/TOK/issues/42",
		"--external-revision", "rev-1",
		"--json",
	}); err != nil {
		t.Fatalf("task create external reference returned error: %v", err)
	}
	var created readyTaskOutput
	if err := json.Unmarshal(createOut.Bytes(), &created); err != nil {
		t.Fatalf("parse task create JSON: %v\n%s", err, createOut.String())
	}
	if created.Source != "github" || created.ExternalID != "42" || created.ExternalURL != "https://github.com/vtimame/TOK/issues/42" || created.ExternalRevision != "rev-1" {
		t.Fatalf("unexpected created external reference: %+v", created)
	}

	var showOut bytes.Buffer
	showCLI := newProjectTestCLI(dataDir, &showOut)
	if err := showCLI.Run(ctx, []string{"task", "show", strconv.FormatInt(created.ID, 10)}); err != nil {
		t.Fatalf("task show external reference returned error: %v", err)
	}
	for _, want := range []string{
		"source: github",
		"external_id: 42",
		"external_url: https://github.com/vtimame/TOK/issues/42",
		"external_revision: rev-1",
	} {
		if !strings.Contains(showOut.String(), want) {
			t.Fatalf("task show output missing %q:\n%s", want, showOut.String())
		}
	}

	var sourceOut bytes.Buffer
	sourceCLI := newProjectTestCLI(dataDir, &sourceOut)
	if err := sourceCLI.Run(ctx, []string{
		"task", "source", strconv.FormatInt(created.ID, 10),
		"--source", "jira",
		"--external-id", "TOK-42",
		"--external-url", "https://example.atlassian.net/browse/TOK-42",
		"--json",
	}); err != nil {
		t.Fatalf("task source returned error: %v", err)
	}
	var updated readyTaskOutput
	if err := json.Unmarshal(sourceOut.Bytes(), &updated); err != nil {
		t.Fatalf("parse task source JSON: %v\n%s", err, sourceOut.String())
	}
	if updated.Source != "jira" || updated.ExternalID != "TOK-42" || updated.ExternalURL != "https://example.atlassian.net/browse/TOK-42" || updated.ExternalRevision != "" {
		t.Fatalf("unexpected updated external reference: %+v", updated)
	}

	var localOut bytes.Buffer
	localCLI := newProjectTestCLI(dataDir, &localOut)
	if err := localCLI.Run(ctx, []string{"task", "source", strconv.FormatInt(created.ID, 10), "--source", "local", "--json"}); err != nil {
		t.Fatalf("task source local returned error: %v", err)
	}
	var local readyTaskOutput
	if err := json.Unmarshal(localOut.Bytes(), &local); err != nil {
		t.Fatalf("parse task source local JSON: %v\n%s", err, localOut.String())
	}
	if local.Source != "local" || local.ExternalID != "" || local.ExternalURL != "" || local.ExternalRevision != "" {
		t.Fatalf("unexpected local source reset: %+v", local)
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
	for _, want := range []string{strconv.FormatInt(blockerID, 10), "open", "Blocker"} {
		if !strings.Contains(readyOut.String(), want) {
			t.Fatalf("expected blocker to be ready, missing %q:\n%s", want, readyOut.String())
		}
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
	if err := statusCLI.Run(ctx, []string{"task", "status", strconv.FormatInt(blockerID, 10), "in_progress"}); err != nil {
		t.Fatalf("task status blocker in_progress returned error: %v", err)
	}
	doneCLI := newProjectTestCLI(dataDir, &bytes.Buffer{})
	if err := doneCLI.Run(ctx, []string{
		"task", "done",
		strconv.FormatInt(blockerID, 10),
		"--note", "Dependency blocker resolved.",
		"--allow-unvalidated",
		"--override-reason", "Dependency ready fixture override.",
	}); err != nil {
		t.Fatalf("task done blocker returned error: %v", err)
	}

	readyOut.Reset()
	readyCLI = newProjectTestCLI(dataDir, &readyOut)
	if err := readyCLI.Run(ctx, []string{"task", "ready", "--project", "tok"}); err != nil {
		t.Fatalf("task ready after done returned error: %v", err)
	}
	for _, want := range []string{strconv.FormatInt(blockedID, 10), "open", "Blocked"} {
		if !strings.Contains(readyOut.String(), want) {
			t.Fatalf("expected blocked task to become ready, missing %q:\n%s", want, readyOut.String())
		}
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

	var depJSONOut bytes.Buffer
	depJSONCLI := newProjectTestCLI(dataDir, &depJSONOut)
	if err := depJSONCLI.Run(ctx, []string{"task", "dependency", "add", strconv.FormatInt(blockerID, 10), strconv.FormatInt(blockedID, 10), "--json"}); err != nil {
		t.Fatalf("task dependency add --json returned error: %v", err)
	}
	var dependencyJSON taskDependencyOutput
	if err := json.Unmarshal(depJSONOut.Bytes(), &dependencyJSON); err != nil {
		t.Fatalf("parse dependency add JSON: %v\n%s", err, depJSONOut.String())
	}
	if dependencyJSON.EdgeType != "blocks" || dependencyJSON.BlockerTaskID != blockerID || dependencyJSON.BlockedTaskID != blockedID {
		t.Fatalf("unexpected dependency add JSON: %+v", dependencyJSON)
	}

	var removeJSONOut bytes.Buffer
	removeJSONCLI := newProjectTestCLI(dataDir, &removeJSONOut)
	if err := removeJSONCLI.Run(ctx, []string{"task", "dependency", "remove", strconv.FormatInt(blockerID, 10), strconv.FormatInt(blockedID, 10), "--json"}); err != nil {
		t.Fatalf("task dependency remove --json returned error: %v", err)
	}
	var removedJSON struct {
		Removed       bool   `json:"removed"`
		EdgeType      string `json:"edge_type"`
		BlockerTaskID int64  `json:"blocker_task_id"`
		BlockedTaskID int64  `json:"blocked_task_id"`
	}
	if err := json.Unmarshal(removeJSONOut.Bytes(), &removedJSON); err != nil {
		t.Fatalf("parse dependency remove JSON: %v\n%s", err, removeJSONOut.String())
	}
	if !removedJSON.Removed || removedJSON.EdgeType != "blocks" || removedJSON.BlockerTaskID != blockerID || removedJSON.BlockedTaskID != blockedID {
		t.Fatalf("unexpected dependency remove JSON: %+v", removedJSON)
	}
}

func TestCLITaskClaimFlow(t *testing.T) {
	ctx := context.Background()
	dataDir := t.TempDir()
	projectDir := t.TempDir()

	projectCLI := newProjectTestCLI(dataDir, &bytes.Buffer{})
	if err := projectCLI.Run(ctx, []string{"project", "add", projectDir, "--name", "tok"}); err != nil {
		t.Fatalf("project add returned error: %v", err)
	}

	firstID := createTaskForTest(t, ctx, dataDir, "tok", "First")
	secondID := createTaskForTest(t, ctx, dataDir, "tok", "Second")

	var claimJSONOut bytes.Buffer
	claimJSONCLI := newProjectTestCLI(dataDir, &claimJSONOut)
	if err := claimJSONCLI.Run(ctx, []string{"task", "claim", "--project", "tok", "--json"}); err != nil {
		t.Fatalf("task claim --json returned error: %v", err)
	}
	var claimed readyTaskOutput
	if err := json.Unmarshal(claimJSONOut.Bytes(), &claimed); err != nil {
		t.Fatalf("parse claimed JSON: %v\n%s", err, claimJSONOut.String())
	}
	if claimed.ID != firstID || claimed.Status != "in_progress" || claimed.Title != "First" {
		t.Fatalf("unexpected claimed JSON: %+v", claimed)
	}

	var showOut bytes.Buffer
	showCLI := newProjectTestCLI(dataDir, &showOut)
	if err := showCLI.Run(ctx, []string{"task", "show", strconv.FormatInt(firstID, 10)}); err != nil {
		t.Fatalf("task show returned error: %v", err)
	}
	if !strings.Contains(showOut.String(), "type: claimed from: open to: in_progress") {
		t.Fatalf("expected claimed event:\n%s", showOut.String())
	}

	claimAgainCLI := newProjectTestCLI(dataDir, &bytes.Buffer{})
	err := claimAgainCLI.Run(ctx, []string{"task", "claim", "--project", "tok", strconv.FormatInt(firstID, 10)})
	if err == nil || !strings.Contains(err.Error(), "task is not ready to claim") {
		t.Fatalf("expected already claimed task error, got %v", err)
	}

	var claimSpecificOut bytes.Buffer
	claimSpecificCLI := newProjectTestCLI(dataDir, &claimSpecificOut)
	if err := claimSpecificCLI.Run(ctx, []string{"task", "claim", "--project", "tok", strconv.FormatInt(secondID, 10)}); err != nil {
		t.Fatalf("task claim specific returned error: %v", err)
	}
	if !strings.Contains(claimSpecificOut.String(), "status: in_progress") || !strings.Contains(claimSpecificOut.String(), "title: Second") {
		t.Fatalf("unexpected specific claim output:\n%s", claimSpecificOut.String())
	}

	emptyCLI := newProjectTestCLI(dataDir, &bytes.Buffer{})
	err = emptyCLI.Run(ctx, []string{"task", "claim", "--project", "tok"})
	if err == nil || !strings.Contains(err.Error(), "no ready tasks for project: tok") {
		t.Fatalf("expected empty ready queue error, got %v", err)
	}
}

func TestCLITaskMutationJSONOutputs(t *testing.T) {
	ctx := context.Background()
	dataDir := t.TempDir()
	projectDir := t.TempDir()

	projectCLI := newProjectTestCLI(dataDir, &bytes.Buffer{})
	if err := projectCLI.Run(ctx, []string{"project", "add", projectDir, "--name", "tok"}); err != nil {
		t.Fatalf("project add returned error: %v", err)
	}

	var createOut bytes.Buffer
	createCLI := newProjectTestCLI(dataDir, &createOut)
	if err := createCLI.Run(ctx, []string{"task", "create", "--project", "tok", "--title", "JSON task", "--json"}); err != nil {
		t.Fatalf("task create --json returned error: %v", err)
	}
	var created readyTaskOutput
	if err := json.Unmarshal(createOut.Bytes(), &created); err != nil {
		t.Fatalf("parse task create JSON: %v\n%s", err, createOut.String())
	}
	if created.ID == 0 || created.Status != "open" || created.Title != "JSON task" {
		t.Fatalf("unexpected task create JSON: %+v", created)
	}

	var statusOut bytes.Buffer
	statusCLI := newProjectTestCLI(dataDir, &statusOut)
	if err := statusCLI.Run(ctx, []string{"task", "status", strconv.FormatInt(created.ID, 10), "in_progress", "--json"}); err != nil {
		t.Fatalf("task status --json returned error: %v", err)
	}
	var statusTask readyTaskOutput
	if err := json.Unmarshal(statusOut.Bytes(), &statusTask); err != nil {
		t.Fatalf("parse task status JSON: %v\n%s", err, statusOut.String())
	}
	if statusTask.ID != created.ID || statusTask.Status != "in_progress" {
		t.Fatalf("unexpected task status JSON: %+v", statusTask)
	}

	statusDoneCLI := newProjectTestCLI(dataDir, &bytes.Buffer{})
	err := statusDoneCLI.Run(ctx, []string{"task", "status", strconv.FormatInt(created.ID, 10), "done"})
	if err == nil || !strings.Contains(err.Error(), "use task done to complete a task with evidence") {
		t.Fatalf("expected task status done rejection, got %v", err)
	}

	var commentOut bytes.Buffer
	commentCLI := newProjectTestCLI(dataDir, &commentOut)
	if err := commentCLI.Run(ctx, []string{"task", "comment", strconv.FormatInt(created.ID, 10), "--body", "JSON comment.", "--json"}); err != nil {
		t.Fatalf("task comment --json returned error: %v", err)
	}
	var comment taskEventOutput
	if err := json.Unmarshal(commentOut.Bytes(), &comment); err != nil {
		t.Fatalf("parse task comment JSON: %v\n%s", err, commentOut.String())
	}
	if comment.TaskID != created.ID || comment.Type != "commented" || comment.Body != "JSON comment." {
		t.Fatalf("unexpected task comment JSON: %+v", comment)
	}

	var progressOut bytes.Buffer
	progressCLI := newProjectTestCLI(dataDir, &progressOut)
	if err := progressCLI.Run(ctx, []string{"task", "progress", strconv.FormatInt(created.ID, 10), "--body", "JSON progress.", "--json"}); err != nil {
		t.Fatalf("task progress --json returned error: %v", err)
	}
	var progress taskEventOutput
	if err := json.Unmarshal(progressOut.Bytes(), &progress); err != nil {
		t.Fatalf("parse task progress JSON: %v\n%s", err, progressOut.String())
	}
	if progress.TaskID != created.ID || progress.Type != "progress" || progress.Body != "JSON progress." {
		t.Fatalf("unexpected task progress JSON: %+v", progress)
	}

	var doneOut bytes.Buffer
	doneCLI := newProjectTestCLI(dataDir, &doneOut)
	if err := doneCLI.Run(ctx, []string{
		"task", "done",
		strconv.FormatInt(created.ID, 10),
		"--note", "JSON done.",
		"--allow-unvalidated",
		"--override-reason", "Task mutation JSON test override.",
		"--json",
	}); err != nil {
		t.Fatalf("task done --json returned error: %v", err)
	}
	var doneTask readyTaskOutput
	if err := json.Unmarshal(doneOut.Bytes(), &doneTask); err != nil {
		t.Fatalf("parse task done JSON: %v\n%s", err, doneOut.String())
	}
	if doneTask.ID != created.ID || doneTask.Status != "done" {
		t.Fatalf("unexpected task done JSON: %+v", doneTask)
	}

	reopenOpenOut := newProjectTestCLI(dataDir, &bytes.Buffer{})
	err = reopenOpenOut.Run(ctx, []string{"task", "status", strconv.FormatInt(created.ID, 10), "open"})
	if err == nil || !strings.Contains(err.Error(), "invalid task status transition") {
		t.Fatalf("expected done->open status rejection, got %v", err)
	}
	reopenProgressOut := newProjectTestCLI(dataDir, &bytes.Buffer{})
	err = reopenProgressOut.Run(ctx, []string{"task", "status", strconv.FormatInt(created.ID, 10), "in_progress"})
	if err == nil || !strings.Contains(err.Error(), "invalid task status transition") {
		t.Fatalf("expected done->in_progress status rejection, got %v", err)
	}

	blockedID := createTaskForTest(t, ctx, dataDir, "tok", "Blockable")
	var blockOut bytes.Buffer
	blockCLI := newProjectTestCLI(dataDir, &blockOut)
	if err := blockCLI.Run(ctx, []string{"task", "block", strconv.FormatInt(blockedID, 10), "--reason", "JSON block.", "--json"}); err != nil {
		t.Fatalf("task block --json returned error: %v", err)
	}
	var blocked readyTaskOutput
	if err := json.Unmarshal(blockOut.Bytes(), &blocked); err != nil {
		t.Fatalf("parse task block JSON: %v\n%s", err, blockOut.String())
	}
	if blocked.ID != blockedID || blocked.Status != "blocked" {
		t.Fatalf("unexpected task block JSON: %+v", blocked)
	}

	var unblockOut bytes.Buffer
	unblockCLI := newProjectTestCLI(dataDir, &unblockOut)
	if err := unblockCLI.Run(ctx, []string{"task", "unblock", strconv.FormatInt(blockedID, 10), "--note", "JSON unblock.", "--json"}); err != nil {
		t.Fatalf("task unblock --json returned error: %v", err)
	}
	var unblocked readyTaskOutput
	if err := json.Unmarshal(unblockOut.Bytes(), &unblocked); err != nil {
		t.Fatalf("parse task unblock JSON: %v\n%s", err, unblockOut.String())
	}
	if unblocked.ID != blockedID || unblocked.Status != "open" {
		t.Fatalf("unexpected task unblock JSON: %+v", unblocked)
	}
}

func TestCLITaskClaimRejectsBlockedTask(t *testing.T) {
	ctx := context.Background()
	dataDir := t.TempDir()
	projectDir := t.TempDir()

	projectCLI := newProjectTestCLI(dataDir, &bytes.Buffer{})
	if err := projectCLI.Run(ctx, []string{"project", "add", projectDir, "--name", "tok"}); err != nil {
		t.Fatalf("project add returned error: %v", err)
	}

	blockerID := createTaskForTest(t, ctx, dataDir, "tok", "Blocker")
	blockedID := createTaskForTest(t, ctx, dataDir, "tok", "Blocked")

	depCLI := newProjectTestCLI(dataDir, &bytes.Buffer{})
	if err := depCLI.Run(ctx, []string{"task", "dependency", "add", strconv.FormatInt(blockerID, 10), strconv.FormatInt(blockedID, 10)}); err != nil {
		t.Fatalf("task dependency add returned error: %v", err)
	}

	claimCLI := newProjectTestCLI(dataDir, &bytes.Buffer{})
	err := claimCLI.Run(ctx, []string{"task", "claim", "--project", "tok", strconv.FormatInt(blockedID, 10)})
	if err == nil || !strings.Contains(err.Error(), "task is not ready to claim") {
		t.Fatalf("expected blocked claim error, got %v", err)
	}
}

func TestCLITaskDoneFlow(t *testing.T) {
	ctx := context.Background()
	dataDir := t.TempDir()
	projectDir := t.TempDir()

	projectCLI := newProjectTestCLI(dataDir, &bytes.Buffer{})
	if err := projectCLI.Run(ctx, []string{"project", "add", projectDir, "--name", "tok"}); err != nil {
		t.Fatalf("project add returned error: %v", err)
	}

	taskID := createTaskForTest(t, ctx, dataDir, "tok", "Finish workflow")

	doneEarlyCLI := newProjectTestCLI(dataDir, &bytes.Buffer{})
	err := doneEarlyCLI.Run(ctx, []string{"task", "done", strconv.FormatInt(taskID, 10), "--note", "too early"})
	if err == nil || !strings.Contains(err.Error(), "task must be in_progress to complete") {
		t.Fatalf("expected invalid transition error, got %v", err)
	}

	claimCLI := newProjectTestCLI(dataDir, &bytes.Buffer{})
	if err := claimCLI.Run(ctx, []string{"task", "claim", "--project", "tok", strconv.FormatInt(taskID, 10)}); err != nil {
		t.Fatalf("task claim returned error: %v", err)
	}

	var doneOut bytes.Buffer
	doneCLI := newProjectTestCLI(dataDir, &doneOut)
	if err := doneCLI.Run(ctx, []string{
		"task", "done",
		strconv.FormatInt(taskID, 10),
		"--note", "Implemented and tests pass.",
		"--allow-unvalidated",
		"--override-reason", "Task done flow test override.",
	}); err != nil {
		t.Fatalf("task done returned error: %v", err)
	}
	if !strings.Contains(doneOut.String(), "status: done") {
		t.Fatalf("unexpected task done output:\n%s", doneOut.String())
	}

	var showOut bytes.Buffer
	showCLI := newProjectTestCLI(dataDir, &showOut)
	if err := showCLI.Run(ctx, []string{"task", "show", strconv.FormatInt(taskID, 10)}); err != nil {
		t.Fatalf("task show returned error: %v", err)
	}
	for _, want := range []string{
		"type: claimed from: open to: in_progress",
		"type: completed from: in_progress to: done",
		"type: completion_override from: in_progress to: done",
		"body: Implemented and tests pass.",
		"body: Task done flow test override.",
	} {
		if !strings.Contains(showOut.String(), want) {
			t.Fatalf("task show output missing %q:\n%s", want, showOut.String())
		}
	}
}

func TestCLITaskDoneRejectsMissingEvidence(t *testing.T) {
	ctx := context.Background()
	dataDir := t.TempDir()
	projectDir := t.TempDir()

	projectCLI := newProjectTestCLI(dataDir, &bytes.Buffer{})
	if err := projectCLI.Run(ctx, []string{"project", "add", projectDir, "--name", "tok"}); err != nil {
		t.Fatalf("project add returned error: %v", err)
	}
	taskID := createTaskForTest(t, ctx, dataDir, "tok", "Missing evidence task")

	claimCLI := newProjectTestCLI(dataDir, &bytes.Buffer{})
	if err := claimCLI.Run(ctx, []string{"task", "claim", "--project", "tok", strconv.FormatInt(taskID, 10)}); err != nil {
		t.Fatalf("task claim returned error: %v", err)
	}

	var doneOut bytes.Buffer
	doneCLI := newProjectTestCLI(dataDir, &doneOut)
	err := doneCLI.Run(ctx, []string{"task", "done", strconv.FormatInt(taskID, 10), "--note", "Done without evidence."})
	if err == nil || !strings.Contains(err.Error(), "requires a succeeded evidence run") {
		t.Fatalf("expected missing evidence error, got %v", err)
	}
}

func TestCLITaskDoneRejectsActiveRun(t *testing.T) {
	ctx := context.Background()
	dataDir := t.TempDir()
	projectDir := t.TempDir()

	projectCLI := newProjectTestCLI(dataDir, &bytes.Buffer{})
	if err := projectCLI.Run(ctx, []string{"project", "add", projectDir, "--name", "tok"}); err != nil {
		t.Fatalf("project add returned error: %v", err)
	}
	taskID := createTaskForTest(t, ctx, dataDir, "tok", "Active run completion guard")

	claimCLI := newProjectTestCLI(dataDir, &bytes.Buffer{})
	if err := claimCLI.Run(ctx, []string{"task", "claim", "--project", "tok", strconv.FormatInt(taskID, 10)}); err != nil {
		t.Fatalf("task claim returned error: %v", err)
	}
	startRunForTest(t, ctx, dataDir, taskID)

	doneCLI := newProjectTestCLI(dataDir, &bytes.Buffer{})
	err := doneCLI.Run(ctx, []string{"task", "done", strconv.FormatInt(taskID, 10), "--note", "Done too early."})
	if err == nil || !strings.Contains(err.Error(), "active run exists") {
		t.Fatalf("expected active run completion error, got %v", err)
	}
}

func TestCLITaskDoneRejectsMissingNote(t *testing.T) {
	cli := newProjectTestCLI(t.TempDir(), &bytes.Buffer{})

	err := cli.Run(context.Background(), []string{"task", "done", "1"})
	if err == nil {
		t.Fatal("expected error")
	}

	var usageErr *UsageError
	if !errors.As(err, &usageErr) {
		t.Fatalf("expected UsageError, got %T: %v", err, err)
	}
	if usageErr.Code != 2 || !strings.Contains(usageErr.Message, "requires --note") {
		t.Fatalf("unexpected usage error: %+v", usageErr)
	}
}

func TestCLITaskProgressBlockUnblock(t *testing.T) {
	ctx := context.Background()
	dataDir := t.TempDir()
	projectDir := t.TempDir()

	projectCLI := newProjectTestCLI(dataDir, &bytes.Buffer{})
	if err := projectCLI.Run(ctx, []string{"project", "add", projectDir, "--name", "tok"}); err != nil {
		t.Fatalf("project add returned error: %v", err)
	}

	taskID := createTaskForTest(t, ctx, dataDir, "tok", "Investigate handoff")

	var progressOut bytes.Buffer
	progressCLI := newProjectTestCLI(dataDir, &progressOut)
	if err := progressCLI.Run(ctx, []string{"task", "progress", strconv.FormatInt(taskID, 10), "--body", "Checked current handoff shape."}); err != nil {
		t.Fatalf("task progress returned error: %v", err)
	}
	if !strings.Contains(progressOut.String(), "type: progress") || !strings.Contains(progressOut.String(), "Checked current handoff shape.") {
		t.Fatalf("unexpected task progress output:\n%s", progressOut.String())
	}

	var blockOut bytes.Buffer
	blockCLI := newProjectTestCLI(dataDir, &blockOut)
	if err := blockCLI.Run(ctx, []string{"task", "block", strconv.FormatInt(taskID, 10), "--reason", "Waiting for contract decision."}); err != nil {
		t.Fatalf("task block returned error: %v", err)
	}
	if !strings.Contains(blockOut.String(), "status: blocked") {
		t.Fatalf("unexpected task block output:\n%s", blockOut.String())
	}

	var readyOut bytes.Buffer
	readyCLI := newProjectTestCLI(dataDir, &readyOut)
	if err := readyCLI.Run(ctx, []string{"task", "ready", "--project", "tok", "--json"}); err != nil {
		t.Fatalf("task ready returned error: %v", err)
	}
	var readyTasks []readyTaskOutput
	if err := json.Unmarshal(readyOut.Bytes(), &readyTasks); err != nil {
		t.Fatalf("parse ready JSON: %v\n%s", err, readyOut.String())
	}
	if len(readyTasks) != 0 {
		t.Fatalf("blocked task should not be ready, got %+v", readyTasks)
	}

	claimCLI := newProjectTestCLI(dataDir, &bytes.Buffer{})
	err := claimCLI.Run(ctx, []string{"task", "claim", "--project", "tok", strconv.FormatInt(taskID, 10)})
	if err == nil || !strings.Contains(err.Error(), "task is not ready to claim") {
		t.Fatalf("expected blocked claim error, got %v", err)
	}

	var unblockOut bytes.Buffer
	unblockCLI := newProjectTestCLI(dataDir, &unblockOut)
	if err := unblockCLI.Run(ctx, []string{"task", "unblock", strconv.FormatInt(taskID, 10), "--note", "Decision made."}); err != nil {
		t.Fatalf("task unblock returned error: %v", err)
	}
	if !strings.Contains(unblockOut.String(), "status: open") {
		t.Fatalf("unexpected task unblock output:\n%s", unblockOut.String())
	}

	readyOut.Reset()
	readyCLI = newProjectTestCLI(dataDir, &readyOut)
	if err := readyCLI.Run(ctx, []string{"task", "ready", "--project", "tok", "--json"}); err != nil {
		t.Fatalf("task ready after unblock returned error: %v", err)
	}
	if err := json.Unmarshal(readyOut.Bytes(), &readyTasks); err != nil {
		t.Fatalf("parse ready JSON after unblock: %v\n%s", err, readyOut.String())
	}
	if len(readyTasks) != 1 || readyTasks[0].ID != taskID {
		t.Fatalf("unblocked task should be ready, got %+v", readyTasks)
	}

	var showOut bytes.Buffer
	showCLI := newProjectTestCLI(dataDir, &showOut)
	if err := showCLI.Run(ctx, []string{"task", "show", strconv.FormatInt(taskID, 10), "--json"}); err != nil {
		t.Fatalf("task show --json returned error: %v", err)
	}
	var shown taskShowOutput
	if err := json.Unmarshal(showOut.Bytes(), &shown); err != nil {
		t.Fatalf("parse task show JSON: %v\n%s", err, showOut.String())
	}
	if shown.Task.Status != "open" {
		t.Fatalf("expected unblocked task status open, got %+v", shown.Task)
	}
	wantEvents := []string{"created", "progress", "blocked", "unblocked"}
	if len(shown.Events) != len(wantEvents) {
		t.Fatalf("unexpected task events: %+v", shown.Events)
	}
	for idx, want := range wantEvents {
		if shown.Events[idx].Type != want {
			t.Fatalf("unexpected event %d: got %+v want %s", idx, shown.Events[idx], want)
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

func createTaskForTest(t *testing.T, ctx context.Context, dataDir, projectName, title string) int64 {
	t.Helper()

	var out bytes.Buffer
	cli := newProjectTestCLI(dataDir, &out)
	if err := cli.Run(ctx, []string{"task", "create", "--project", projectName, "--title", title}); err != nil {
		t.Fatalf("task create %q returned error: %v", title, err)
	}
	return mustExtractNumericField(t, out.String(), "id")
}

func createClaimedTaskForTest(t *testing.T, ctx context.Context, dataDir, projectName, title string) int64 {
	t.Helper()

	taskID := createTaskForTest(t, ctx, dataDir, projectName, title)
	cli := newProjectTestCLI(dataDir, &bytes.Buffer{})
	if err := cli.Run(ctx, []string{"task", "claim", "--project", projectName, strconv.FormatInt(taskID, 10)}); err != nil {
		t.Fatalf("task claim %q returned error: %v", title, err)
	}
	return taskID
}
