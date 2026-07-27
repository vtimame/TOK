package mcpserver

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	contextpkg "s26.sh/tok/internal/context"
	"s26.sh/tok/internal/storage"
)

func TestServerToolsClaimTaskWithAgentAttribution(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)

	project, err := store.CreateProject(ctx, storage.CreateProjectInput{
		Name:        "tok",
		DisplayName: "TOK",
		Path:        t.TempDir(),
	})
	if err != nil {
		t.Fatalf("CreateProject returned error: %v", err)
	}
	createdAgent, err := store.CreateAgent(ctx, "Codex MCP")
	if err != nil {
		t.Fatalf("CreateAgent returned error: %v", err)
	}
	task, err := store.CreateTask(ctx, storage.CreateTaskInput{
		ProjectID: project.ID,
		Title:     "Wire MCP",
	})
	if err != nil {
		t.Fatalf("CreateTask returned error: %v", err)
	}
	instruction, err := store.CreateProjectInstruction(ctx, storage.CreateProjectInstructionInput{
		ProjectID: project.ID,
		Title:     "Use Context7",
		Body:      "Use Context7 for library documentation.",
		Priority:  "high",
	})
	if err != nil {
		t.Fatalf("CreateProjectInstruction returned error: %v", err)
	}

	server, err := New(Config{
		Store:   store,
		Actor:   storage.ActorRefFromActor(createdAgent.Agent),
		Version: "test",
	})
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	clientSession, serverSession := connectTestClient(t, ctx, server)
	defer clientSession.Close()
	defer serverSession.Close()

	projectResult, err := clientSession.CallTool(ctx, &mcp.CallToolParams{
		Name:      "project_list",
		Arguments: map[string]any{},
	})
	if err != nil {
		t.Fatalf("project_list returned error: %v", err)
	}
	if projectResult.IsError {
		t.Fatalf("project_list returned tool error: %+v", projectResult)
	}
	var projects projectListOutput
	decodeStructured(t, projectResult.StructuredContent, &projects)
	if len(projects.Projects) != 1 || projects.Projects[0].Name != "tok" {
		t.Fatalf("unexpected project_list output: %+v", projects)
	}

	claimResult, err := clientSession.CallTool(ctx, &mcp.CallToolParams{
		Name: "task_claim",
		Arguments: map[string]any{
			"project": "tok",
		},
	})
	if err != nil {
		t.Fatalf("task_claim returned error: %v", err)
	}
	if claimResult.IsError {
		t.Fatalf("task_claim returned tool error: %+v", claimResult)
	}
	var claimed taskOutput
	decodeStructured(t, claimResult.StructuredContent, &claimed)
	if claimed.Task.ID != task.ID || claimed.Task.Status != "in_progress" || claimed.Task.Title != "Wire MCP" {
		t.Fatalf("unexpected task_claim output: %+v", claimed)
	}

	showResult, err := clientSession.CallTool(ctx, &mcp.CallToolParams{
		Name: "task_show",
		Arguments: map[string]any{
			"id": task.ID,
		},
	})
	if err != nil {
		t.Fatalf("task_show returned error: %v", err)
	}
	if showResult.IsError {
		t.Fatalf("task_show returned tool error: %+v", showResult)
	}
	var shown taskShowOutput
	decodeStructured(t, showResult.StructuredContent, &shown)
	if len(shown.Events) != 2 || shown.Events[1].Type != "claimed" {
		t.Fatalf("unexpected task_show events: %+v", shown.Events)
	}
	if shown.Events[1].Actor == nil || shown.Events[1].Actor.ID != createdAgent.Agent.ID || shown.Events[1].Actor.Name != "Codex MCP" {
		t.Fatalf("claimed event missing agent attribution: %+v", shown.Events[1])
	}

	contextResult, err := clientSession.CallTool(ctx, &mcp.CallToolParams{
		Name: "context_build",
		Arguments: map[string]any{
			"project": "tok",
			"task_id": task.ID,
		},
	})
	if err != nil {
		t.Fatalf("context_build returned error: %v", err)
	}
	if contextResult.IsError {
		t.Fatalf("context_build returned tool error: %+v", contextResult)
	}
	var contextPackage contextBuildOutput
	decodeStructured(t, contextResult.StructuredContent, &contextPackage)
	if contextPackage.Project.Name != "tok" || contextPackage.Task.ID != task.ID {
		t.Fatalf("unexpected context_build metadata: %+v", contextPackage)
	}
	if len(contextPackage.ProjectInstructions) != 1 || contextPackage.ProjectInstructions[0].ID != instruction.ID || contextPackage.ProjectInstructions[0].Title != "Use Context7" {
		t.Fatalf("context_build missing project instruction: %+v", contextPackage.ProjectInstructions)
	}
}

func TestServerRejectsMissingActor(t *testing.T) {
	_, err := New(Config{Store: openTestStore(t)})
	if err == nil {
		t.Fatal("expected missing actor error")
	}
}

func TestServerProjectCreateSupportsAgentProjectRegistration(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	createdAgent, err := store.CreateAgent(ctx, "Codex MCP")
	if err != nil {
		t.Fatalf("CreateAgent returned error: %v", err)
	}

	server, err := New(Config{
		Store:   store,
		Actor:   storage.ActorRefFromActor(createdAgent.Agent),
		Version: "test",
	})
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	clientSession, serverSession := connectTestClient(t, ctx, server)
	defer clientSession.Close()
	defer serverSession.Close()

	projectPath := t.TempDir()
	createProjectResult, err := clientSession.CallTool(ctx, &mcp.CallToolParams{
		Name: "project_create",
		Arguments: map[string]any{
			"name":         "agent-workspace",
			"display_name": "Agent Workspace",
			"path":         projectPath,
		},
	})
	if err != nil {
		t.Fatalf("project_create returned error: %v", err)
	}
	if createProjectResult.IsError {
		t.Fatalf("project_create returned tool error: %+v", createProjectResult)
	}
	var createdProject projectOutput
	decodeStructured(t, createProjectResult.StructuredContent, &createdProject)
	if createdProject.Project.ID <= 0 || createdProject.Project.Name != "agent-workspace" || createdProject.Project.DisplayName != "Agent Workspace" {
		t.Fatalf("unexpected project_create output: %+v", createdProject)
	}
	if createdProject.Project.Path != projectPath {
		t.Fatalf("expected canonical project path %q, got %+v", projectPath, createdProject)
	}

	showProjectResult, err := clientSession.CallTool(ctx, &mcp.CallToolParams{
		Name: "project_show",
		Arguments: map[string]any{
			"name": "agent-workspace",
		},
	})
	if err != nil {
		t.Fatalf("project_show returned error: %v", err)
	}
	if showProjectResult.IsError {
		t.Fatalf("project_show returned tool error: %+v", showProjectResult)
	}
	var shownProject projectOutput
	decodeStructured(t, showProjectResult.StructuredContent, &shownProject)
	if shownProject.Project.ID != createdProject.Project.ID {
		t.Fatalf("unexpected project_show output: %+v", shownProject)
	}

	createdTaskResult, err := clientSession.CallTool(ctx, &mcp.CallToolParams{
		Name: "task_create",
		Arguments: map[string]any{
			"project":           "agent-workspace",
			"title":             "Use newly registered project",
			"source":            "github",
			"external_id":       "42",
			"external_url":      "https://github.com/vtimame/TOK/issues/42",
			"external_revision": "rev-1",
		},
	})
	if err != nil {
		t.Fatalf("task_create returned error: %v", err)
	}
	if createdTaskResult.IsError {
		t.Fatalf("task_create returned tool error: %+v", createdTaskResult)
	}
	var createdTask taskOutput
	decodeStructured(t, createdTaskResult.StructuredContent, &createdTask)
	if createdTask.Task.ProjectID != createdProject.Project.ID ||
		createdTask.Task.Source != "github" ||
		createdTask.Task.ExternalID != "42" ||
		createdTask.Task.ExternalURL != "https://github.com/vtimame/TOK/issues/42" ||
		createdTask.Task.ExternalRevision != "rev-1" {
		t.Fatalf("task_create did not use created project: %+v", createdTask)
	}

	sourceResult, err := clientSession.CallTool(ctx, &mcp.CallToolParams{
		Name: "task_source",
		Arguments: map[string]any{
			"id":           createdTask.Task.ID,
			"source":       "jira",
			"external_id":  "TOK-42",
			"external_url": "https://example.atlassian.net/browse/TOK-42",
		},
	})
	if err != nil {
		t.Fatalf("task_source returned error: %v", err)
	}
	if sourceResult.IsError {
		t.Fatalf("task_source returned tool error: %+v", sourceResult)
	}
	var sourcedTask taskOutput
	decodeStructured(t, sourceResult.StructuredContent, &sourcedTask)
	if sourcedTask.Task.Source != "jira" || sourcedTask.Task.ExternalID != "TOK-42" || sourcedTask.Task.ExternalURL != "https://example.atlassian.net/browse/TOK-42" || sourcedTask.Task.ExternalRevision != "" {
		t.Fatalf("unexpected task_source output: %+v", sourcedTask)
	}
}

func TestServerWorkflowToolsSupportTaskInstructionDependencyAndRunLifecycle(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)

	_, err := store.CreateProject(ctx, storage.CreateProjectInput{
		Name:        "tok",
		DisplayName: "TOK",
		Path:        t.TempDir(),
	})
	if err != nil {
		t.Fatalf("CreateProject returned error: %v", err)
	}
	createdAgent, err := store.CreateAgent(ctx, "Codex MCP")
	if err != nil {
		t.Fatalf("CreateAgent returned error: %v", err)
	}

	server, err := New(Config{
		Store:   store,
		Actor:   storage.ActorRefFromActor(createdAgent.Agent),
		Version: "test",
	})
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	clientSession, serverSession := connectTestClient(t, ctx, server)
	defer clientSession.Close()
	defer serverSession.Close()

	createdTaskResult, err := clientSession.CallTool(ctx, &mcp.CallToolParams{
		Name: "task_create",
		Arguments: map[string]any{
			"project":             "tok",
			"title":               "Run toolchain checks",
			"description":         "Ensure workflow MCP can drive run lifecycle",
			"acceptance_criteria": "All tools return expected JSON",
		},
	})
	if err != nil {
		t.Fatalf("task_create returned error: %v", err)
	}
	if createdTaskResult.IsError {
		t.Fatalf("task_create returned tool error: %+v", createdTaskResult)
	}
	var createdTask taskOutput
	decodeStructured(t, createdTaskResult.StructuredContent, &createdTask)
	if createdTask.Task.ID <= 0 || createdTask.Task.Status != "open" {
		t.Fatalf("unexpected task_create output: %+v", createdTask)
	}

	statusResult, err := clientSession.CallTool(ctx, &mcp.CallToolParams{
		Name: "task_status",
		Arguments: map[string]any{
			"id":     createdTask.Task.ID,
			"status": "in_progress",
		},
	})
	if err != nil {
		t.Fatalf("task_status returned error: %v", err)
	}
	if statusResult.IsError {
		t.Fatalf("task_status returned tool error: %+v", statusResult)
	}
	var updatedTask taskOutput
	decodeStructured(t, statusResult.StructuredContent, &updatedTask)
	if updatedTask.Task.Status != "in_progress" {
		t.Fatalf("unexpected task_status output: %+v", updatedTask)
	}
	if updatedTask.Task.ID != createdTask.Task.ID {
		t.Fatalf("unexpected task_status output id: %+v", updatedTask)
	}

	commentResult, err := clientSession.CallTool(ctx, &mcp.CallToolParams{
		Name: "task_comment",
		Arguments: map[string]any{
			"id":   createdTask.Task.ID,
			"body": "MCP comment event",
		},
	})
	if err != nil {
		t.Fatalf("task_comment returned error: %v", err)
	}
	if commentResult.IsError {
		t.Fatalf("task_comment returned tool error: %+v", commentResult)
	}
	var comment taskEventOutput
	decodeStructured(t, commentResult.StructuredContent, &comment)
	if comment.Event.Type != "commented" || comment.Event.Body != "MCP comment event" {
		t.Fatalf("unexpected task_comment output: %+v", comment)
	}

	progressResult, err := clientSession.CallTool(ctx, &mcp.CallToolParams{
		Name: "task_progress",
		Arguments: map[string]any{
			"id":   createdTask.Task.ID,
			"body": "MCP progress event",
		},
	})
	if err != nil {
		t.Fatalf("task_progress returned error: %v", err)
	}
	if progressResult.IsError {
		t.Fatalf("task_progress returned tool error: %+v", progressResult)
	}
	var progress taskEventOutput
	decodeStructured(t, progressResult.StructuredContent, &progress)
	if progress.Event.Type != "progress" || progress.Event.Body != "MCP progress event" {
		t.Fatalf("unexpected task_progress output: %+v", progress)
	}

	blockerTaskResult, err := clientSession.CallTool(ctx, &mcp.CallToolParams{
		Name: "task_create",
		Arguments: map[string]any{
			"project": "tok",
			"title":   "Block dependent task",
		},
	})
	if err != nil {
		t.Fatalf("task_create (blocker) returned error: %v", err)
	}
	if blockerTaskResult.IsError {
		t.Fatalf("task_create (blocker) returned tool error: %+v", blockerTaskResult)
	}
	var blockedTask taskOutput
	decodeStructured(t, blockerTaskResult.StructuredContent, &blockedTask)
	if blockedTask.Task.ID <= 0 {
		t.Fatalf("unexpected blocker task output: %+v", blockedTask)
	}

	instructionCreateResult, err := clientSession.CallTool(ctx, &mcp.CallToolParams{
		Name: "project_instruction_create",
		Arguments: map[string]any{
			"project":  "tok",
			"title":    "Use tests",
			"body":     "Use mcp in-memory tests for workflows.",
			"priority": "high",
		},
	})
	if err != nil {
		t.Fatalf("project_instruction_create returned error: %v", err)
	}
	if instructionCreateResult.IsError {
		t.Fatalf("project_instruction_create returned tool error: %+v", instructionCreateResult)
	}
	var instructionCreated projectInstructionShowOutput
	decodeStructured(t, instructionCreateResult.StructuredContent, &instructionCreated)
	if instructionCreated.Instruction.ID == 0 || instructionCreated.Instruction.Title != "Use tests" {
		t.Fatalf("unexpected project_instruction_create output: %+v", instructionCreated)
	}

	listResult, err := clientSession.CallTool(ctx, &mcp.CallToolParams{
		Name: "project_instruction_list",
		Arguments: map[string]any{
			"project": "tok",
		},
	})
	if err != nil {
		t.Fatalf("project_instruction_list returned error: %v", err)
	}
	if listResult.IsError {
		t.Fatalf("project_instruction_list returned tool error: %+v", listResult)
	}
	var instructionList projectInstructionListOutput
	decodeStructured(t, listResult.StructuredContent, &instructionList)
	if len(instructionList.Instructions) != 1 {
		t.Fatalf("expected one project instruction, got: %+v", instructionList)
	}

	disableResult, err := clientSession.CallTool(ctx, &mcp.CallToolParams{
		Name: "project_instruction_disable",
		Arguments: map[string]any{
			"project": "tok",
			"id":      instructionCreated.Instruction.ID,
		},
	})
	if err != nil {
		t.Fatalf("project_instruction_disable returned error: %v", err)
	}
	if disableResult.IsError {
		t.Fatalf("project_instruction_disable returned tool error: %+v", disableResult)
	}
	var disabledInstruction projectInstructionShowOutput
	decodeStructured(t, disableResult.StructuredContent, &disabledInstruction)
	if disabledInstruction.Instruction.Enabled {
		t.Fatalf("expected disabled instruction: %+v", disabledInstruction)
	}

	showResult, err := clientSession.CallTool(ctx, &mcp.CallToolParams{
		Name: "project_instruction_show",
		Arguments: map[string]any{
			"project": "tok",
			"id":      instructionCreated.Instruction.ID,
		},
	})
	if err != nil {
		t.Fatalf("project_instruction_show returned error: %v", err)
	}
	if showResult.IsError {
		t.Fatalf("project_instruction_show returned tool error: %+v", showResult)
	}
	var shownInstruction projectInstructionShowOutput
	decodeStructured(t, showResult.StructuredContent, &shownInstruction)
	if shownInstruction.Instruction.ID != instructionCreated.Instruction.ID {
		t.Fatalf("unexpected project_instruction_show output: %+v", shownInstruction)
	}

	enableResult, err := clientSession.CallTool(ctx, &mcp.CallToolParams{
		Name: "project_instruction_enable",
		Arguments: map[string]any{
			"project": "tok",
			"id":      instructionCreated.Instruction.ID,
		},
	})
	if err != nil {
		t.Fatalf("project_instruction_enable returned error: %v", err)
	}
	if enableResult.IsError {
		t.Fatalf("project_instruction_enable returned tool error: %+v", enableResult)
	}
	var enabledInstruction projectInstructionShowOutput
	decodeStructured(t, enableResult.StructuredContent, &enabledInstruction)
	if !enabledInstruction.Instruction.Enabled {
		t.Fatalf("expected enabled instruction: %+v", enabledInstruction)
	}

	dependencyAddResult, err := clientSession.CallTool(ctx, &mcp.CallToolParams{
		Name: "task_dependency_add",
		Arguments: map[string]any{
			"blocker_task_id": createdTask.Task.ID,
			"blocked_task_id": blockedTask.Task.ID,
		},
	})
	if err != nil {
		t.Fatalf("task_dependency_add returned error: %v", err)
	}
	if dependencyAddResult.IsError {
		t.Fatalf("task_dependency_add returned tool error: %+v", dependencyAddResult)
	}

	var dependency TaskDependencyOutput
	decodeStructured(t, dependencyAddResult.StructuredContent, &dependency)
	if dependency.BlockerTaskID != createdTask.Task.ID || dependency.BlockedTaskID != blockedTask.Task.ID {
		t.Fatalf("unexpected dependency add output: %+v", dependency)
	}

	dependencyRemoveResult, err := clientSession.CallTool(ctx, &mcp.CallToolParams{
		Name: "task_dependency_remove",
		Arguments: map[string]any{
			"blocker_task_id": createdTask.Task.ID,
			"blocked_task_id": blockedTask.Task.ID,
		},
	})
	if err != nil {
		t.Fatalf("task_dependency_remove returned error: %v", err)
	}
	if dependencyRemoveResult.IsError {
		t.Fatalf("task_dependency_remove returned tool error: %+v", dependencyRemoveResult)
	}
	var removed taskDependencyRemovedOutput
	decodeStructured(t, dependencyRemoveResult.StructuredContent, &removed)
	if !removed.Removed || removed.BlockerTaskID != createdTask.Task.ID || removed.BlockedTaskID != blockedTask.Task.ID {
		t.Fatalf("unexpected task_dependency_remove output: %+v", removed)
	}

	blockResult, err := clientSession.CallTool(ctx, &mcp.CallToolParams{
		Name: "task_block",
		Arguments: map[string]any{
			"id":     blockedTask.Task.ID,
			"reason": "Waiting for MCP audit",
		},
	})
	if err != nil {
		t.Fatalf("task_block returned error: %v", err)
	}
	if blockResult.IsError {
		t.Fatalf("task_block returned tool error: %+v", blockResult)
	}
	var blocked taskOutput
	decodeStructured(t, blockResult.StructuredContent, &blocked)
	if blocked.Task.Status != "blocked" {
		t.Fatalf("unexpected task_block output: %+v", blocked)
	}

	unblockResult, err := clientSession.CallTool(ctx, &mcp.CallToolParams{
		Name: "task_unblock",
		Arguments: map[string]any{
			"id":   blockedTask.Task.ID,
			"note": "MCP audit complete",
		},
	})
	if err != nil {
		t.Fatalf("task_unblock returned error: %v", err)
	}
	if unblockResult.IsError {
		t.Fatalf("task_unblock returned tool error: %+v", unblockResult)
	}
	var unblocked taskOutput
	decodeStructured(t, unblockResult.StructuredContent, &unblocked)
	if unblocked.Task.Status != "open" {
		t.Fatalf("unexpected task_unblock output: %+v", unblocked)
	}

	runCreateResult, err := clientSession.CallTool(ctx, &mcp.CallToolParams{
		Name: "run_create",
		Arguments: map[string]any{
			"task_id":         createdTask.Task.ID,
			"status":          "in_progress",
			"retrieval_limit": 7,
		},
	})
	if err != nil {
		t.Fatalf("run_create returned error: %v", err)
	}
	if runCreateResult.IsError {
		t.Fatalf("run_create returned tool error: %+v", runCreateResult)
	}
	var run runOutput
	decodeStructured(t, runCreateResult.StructuredContent, &run)
	if run.ID <= 0 || run.TaskID != createdTask.Task.ID {
		t.Fatalf("unexpected run_create output: %+v", run)
	}
	if run.HandoffContractVersion != contextpkg.HandoffContractV0 {
		t.Fatalf("expected default handoff contract, got: %+v", run)
	}

	artifactAddResult, err := clientSession.CallTool(ctx, &mcp.CallToolParams{
		Name: "run_validation_record",
		Arguments: map[string]any{
			"run_id":  run.ID,
			"command": "go test ./internal/mcpserver",
			"status":  "passed",
			"summary": "MCP workflow tests passed.",
		},
	})
	if err != nil {
		t.Fatalf("run_validation_record returned error: %v", err)
	}
	if artifactAddResult.IsError {
		t.Fatalf("run_validation_record returned tool error: %+v", artifactAddResult)
	}
	var artifact runArtifactOutput
	decodeStructured(t, artifactAddResult.StructuredContent, &artifact)
	if artifact.ID <= 0 || artifact.Kind != "validation" || artifact.RunID != run.ID {
		t.Fatalf("unexpected run_validation_record output: %+v", artifact)
	}
	var metadata struct {
		Command string `json:"command"`
		Status  string `json:"status"`
		Summary string `json:"summary"`
	}
	if err := json.Unmarshal([]byte(artifact.Metadata), &metadata); err != nil {
		t.Fatalf("decode validation metadata: %v", err)
	}
	if metadata.Command != "go test ./internal/mcpserver" || metadata.Status != "passed" || metadata.Summary == "" {
		t.Fatalf("unexpected validation metadata: %+v", metadata)
	}

	artifactListResult, err := clientSession.CallTool(ctx, &mcp.CallToolParams{
		Name: "run_artifact_list",
		Arguments: map[string]any{
			"run_id": run.ID,
		},
	})
	if err != nil {
		t.Fatalf("run_artifact_list returned error: %v", err)
	}
	if artifactListResult.IsError {
		t.Fatalf("run_artifact_list returned tool error: %+v", artifactListResult)
	}
	var artifactList runArtifactListOutput
	decodeStructured(t, artifactListResult.StructuredContent, &artifactList)
	if len(artifactList.Artifacts) != 1 {
		t.Fatalf("unexpected run_artifact_list output: %+v", artifactList)
	}

	runFinishResult, err := clientSession.CallTool(ctx, &mcp.CallToolParams{
		Name: "run_finish",
		Arguments: map[string]any{
			"id":      run.ID,
			"status":  "succeeded",
			"summary": "Workflow completed via MCP",
		},
	})
	if err != nil {
		t.Fatalf("run_finish returned error: %v", err)
	}
	if runFinishResult.IsError {
		t.Fatalf("run_finish returned tool error: %+v", runFinishResult)
	}
	var finishedRun runOutput
	decodeStructured(t, runFinishResult.StructuredContent, &finishedRun)
	if finishedRun.Status != "succeeded" || finishedRun.ResultSummary != "Workflow completed via MCP" {
		t.Fatalf("unexpected run_finish output: %+v", finishedRun)
	}
	if len(finishedRun.Artifacts) != 1 {
		t.Fatalf("expected validation artifact on finished run: %+v", finishedRun.Artifacts)
	}

	runShowResult, err := clientSession.CallTool(ctx, &mcp.CallToolParams{
		Name: "run_show",
		Arguments: map[string]any{
			"id": run.ID,
		},
	})
	if err != nil {
		t.Fatalf("run_show returned error: %v", err)
	}
	if runShowResult.IsError {
		t.Fatalf("run_show returned tool error: %+v", runShowResult)
	}
	var showedRun runOutput
	decodeStructured(t, runShowResult.StructuredContent, &showedRun)
	if showedRun.Status != "succeeded" || len(showedRun.Artifacts) != 1 {
		t.Fatalf("unexpected run_show output: %+v", showedRun)
	}

	runListResult, err := clientSession.CallTool(ctx, &mcp.CallToolParams{
		Name: "run_list",
		Arguments: map[string]any{
			"project": "tok",
			"status":  "succeeded",
		},
	})
	if err != nil {
		t.Fatalf("run_list returned error: %v", err)
	}
	if runListResult.IsError {
		t.Fatalf("run_list returned tool error: %+v", runListResult)
	}
	var runs runListOutput
	decodeStructured(t, runListResult.StructuredContent, &runs)
	if len(runs.Runs) != 1 || runs.Runs[0].ID != run.ID {
		t.Fatalf("unexpected run_list output: %+v", runs)
	}

	doneResult, err := clientSession.CallTool(ctx, &mcp.CallToolParams{
		Name: "task_done",
		Arguments: map[string]any{
			"id":              createdTask.Task.ID,
			"note":            "MCP workflow completed",
			"evidence_run_id": run.ID,
		},
	})
	if err != nil {
		t.Fatalf("task_done returned error: %v", err)
	}
	if doneResult.IsError {
		t.Fatalf("task_done returned tool error: %+v", doneResult)
	}
	var doneTask taskOutput
	decodeStructured(t, doneResult.StructuredContent, &doneTask)
	if doneTask.Task.Status != "done" {
		t.Fatalf("unexpected task_done output: %+v", doneTask)
	}

	showDoneResult, err := clientSession.CallTool(ctx, &mcp.CallToolParams{
		Name: "task_show",
		Arguments: map[string]any{
			"id": createdTask.Task.ID,
		},
	})
	if err != nil {
		t.Fatalf("task_show returned error: %v", err)
	}
	if showDoneResult.IsError {
		t.Fatalf("task_show returned tool error: %+v", showDoneResult)
	}
	var shownDoneTask taskShowOutput
	decodeStructured(t, showDoneResult.StructuredContent, &shownDoneTask)
	if len(shownDoneTask.Events) == 0 || shownDoneTask.Events[len(shownDoneTask.Events)-1].Type != "completed" {
		t.Fatalf("expected completion event in task history: %+v", shownDoneTask.Events)
	}
	lastDoneEvent := shownDoneTask.Events[len(shownDoneTask.Events)-1]
	if lastDoneEvent.EvidenceRunID != run.ID {
		t.Fatalf("expected completion evidence run id %d, got %d", run.ID, lastDoneEvent.EvidenceRunID)
	}
	if lastDoneEvent.EvidenceArtifactID != artifact.ID {
		t.Fatalf("expected completion evidence artifact id %d, got %d", artifact.ID, lastDoneEvent.EvidenceArtifactID)
	}

	deleteResult, err := clientSession.CallTool(ctx, &mcp.CallToolParams{
		Name: "project_instruction_delete",
		Arguments: map[string]any{
			"project": "tok",
			"id":      instructionCreated.Instruction.ID,
		},
	})
	if err != nil {
		t.Fatalf("project_instruction_delete returned error: %v", err)
	}
	if deleteResult.IsError {
		t.Fatalf("project_instruction_delete returned tool error: %+v", deleteResult)
	}
	var deletedInstruction projectInstructionShowOutput
	decodeStructured(t, deleteResult.StructuredContent, &deletedInstruction)
	if deletedInstruction.Instruction.ID != instructionCreated.Instruction.ID {
		t.Fatalf("unexpected project_instruction_delete output: %+v", deletedInstruction)
	}
}

func TestServerTaskDoneCurrentBehaviorAllowsMissingEvidenceExpectedToChange(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)

	project, err := store.CreateProject(ctx, storage.CreateProjectInput{
		Name:        "tok",
		DisplayName: "TOK",
		Path:        t.TempDir(),
	})
	if err != nil {
		t.Fatalf("CreateProject returned error: %v", err)
	}
	agent, err := store.CreateAgent(ctx, "Codex MCP")
	if err != nil {
		t.Fatalf("CreateAgent returned error: %v", err)
	}
	task, err := store.CreateTask(ctx, storage.CreateTaskInput{
		ProjectID: project.ID,
		Title:     "MCP missing evidence completion",
	})
	if err != nil {
		t.Fatalf("CreateTask returned error: %v", err)
	}
	if _, err := store.ClaimTaskByActor(ctx, project.ID, task.ID, storage.ActorRefFromActor(agent.Agent)); err != nil {
		t.Fatalf("ClaimTaskByActor returned error: %v", err)
	}

	server, err := New(Config{
		Store:   store,
		Actor:   storage.ActorRefFromActor(agent.Agent),
		Version: "test",
	})
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	clientSession, serverSession := connectTestClient(t, ctx, server)
	defer clientSession.Close()
	defer serverSession.Close()

	doneResult, err := clientSession.CallTool(ctx, &mcp.CallToolParams{
		Name: "task_done",
		Arguments: map[string]any{
			"id":   task.ID,
			"note": "Done without evidence.",
		},
	})
	if err != nil {
		t.Fatalf("task_done returned error: %v", err)
	}
	if !doneResult.IsError {
		t.Fatalf("expected task_done tool error without evidence, got %+v", doneResult)
	}
}

func connectTestClient(t *testing.T, ctx context.Context, server *mcp.Server) (*mcp.ClientSession, *mcp.ServerSession) {
	t.Helper()

	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "tok-test", Version: "test"}, nil)
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		_ = serverSession.Close()
		t.Fatalf("client connect: %v", err)
	}
	return clientSession, serverSession
}

func openTestStore(t *testing.T) *storage.Store {
	t.Helper()

	store, err := storage.Open(context.Background(), ":memory:")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Fatalf("close store: %v", err)
		}
	})
	if err := store.Init(context.Background()); err != nil {
		t.Fatalf("init store: %v", err)
	}
	return store
}

func decodeStructured(t *testing.T, structured any, dst any) {
	t.Helper()

	raw, err := json.Marshal(structured)
	if err != nil {
		t.Fatalf("marshal structured content: %v", err)
	}
	if err := json.Unmarshal(raw, dst); err != nil {
		t.Fatalf("decode structured content: %v\n%s", err, raw)
	}
}
