package mcpserver

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

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
