package httpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	"s26.sh/tok/internal/storage"
)

func TestServerListsProjectsAndExposesOpenAPI(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)

	if _, err := store.CreateProject(ctx, storage.CreateProjectInput{
		Name:        "tok",
		DisplayName: "TOK",
		Path:        t.TempDir(),
	}); err != nil {
		t.Fatalf("CreateProject returned error: %v", err)
	}

	handler := newTestHandler(t, store)

	res := doJSON(t, handler, http.MethodGet, "/api/projects", nil)
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/projects status = %d", res.StatusCode)
	}
	var projects ProjectListResponse
	decodeJSON(t, res, &projects)
	if len(projects.Projects) != 1 || projects.Projects[0].Name != "tok" || projects.Projects[0].DisplayName != "TOK" {
		t.Fatalf("unexpected projects output: %+v", projects)
	}
	if projects.Total != 1 || projects.Limit != 1 || projects.NextCursor != "" {
		t.Fatalf("unexpected projects pagination: %+v", projects)
	}

	specRes := doJSON(t, handler, http.MethodGet, "/swagger/openapi.json", nil)
	defer specRes.Body.Close()
	if specRes.StatusCode != http.StatusOK {
		t.Fatalf("GET /swagger/openapi.json status = %d", specRes.StatusCode)
	}
	var spec struct {
		OpenAPI    string                             `json:"openapi"`
		Paths      map[string]map[string]openAPIRoute `json:"paths"`
		Components struct{ Schemas map[string]any }   `json:"components"`
	}
	decodeJSON(t, specRes, &spec)
	if spec.OpenAPI == "" {
		t.Fatalf("openapi spec version is empty")
	}
	if spec.Paths["/api/projects"] == nil || spec.Paths["/api/tasks"] == nil || spec.Paths["/api/tasks/{id}"] == nil {
		t.Fatalf("openapi spec missing expected paths: %+v", spec.Paths)
	}

	expectedOperations := map[string]string{
		"/api/health GET":                                        "getHealth",
		"/api/agents GET":                                        "listAgents",
		"/api/agents POST":                                       "createAgent",
		"/api/agents/{id} GET":                                   "showAgent",
		"/api/agents/{id} PATCH":                                 "updateAgent",
		"/api/agents/{id} DELETE":                                "deleteAgent",
		"/api/projects GET":                                      "listProjects",
		"/api/projects POST":                                     "createProject",
		"/api/projects/{project} GET":                            "showProject",
		"/api/projects/{project} PATCH":                          "updateProject",
		"/api/projects/{project} DELETE":                         "deleteProject",
		"/api/projects/{project}/instructions GET":               "listProjectInstructions",
		"/api/projects/{project}/instructions POST":              "createProjectInstruction",
		"/api/projects/{project}/instructions/{id} GET":          "showProjectInstruction",
		"/api/projects/{project}/instructions/{id}/enable POST":  "enableProjectInstruction",
		"/api/projects/{project}/instructions/{id}/disable POST": "disableProjectInstruction",
		"/api/projects/{project}/instructions/{id} DELETE":       "deleteProjectInstruction",
		"/api/projects/{project}/tasks GET":                      "listProjectTasks",
		"/api/projects/{project}/tasks POST":                     "createTask",
		"/api/projects/{project}/tasks/ready GET":                "listReadyTasks",
		"/api/projects/{project}/tasks/claim POST":               "claimTask",
		"/api/tasks GET":                                         "listTasks",
		"/api/tasks/{id} GET":                                    "showTask",
		"/api/tasks/{id}/comment POST":                           "commentTask",
		"/api/tasks/{id}/progress POST":                          "progressTask",
		"/api/tasks/{id}/block POST":                             "blockTask",
		"/api/tasks/{id}/unblock POST":                           "unblockTask",
		"/api/tasks/{id}/done POST":                              "completeTask",
		"/api/index GET":                                         "listIndexStatus",
		"/api/index/update POST":                                 "updateAllProjectIndexes",
		"/api/projects/{project}/index GET":                      "getProjectIndexStatus",
		"/api/projects/{project}/index/update POST":              "updateProjectIndex",
		"/api/projects/{project}/index/ignore GET":               "getProjectIndexIgnorePolicy",
		"/api/projects/{project}/index/ignore/refresh POST":      "refreshProjectIndexIgnorePolicy",
		"/api/projects/{project}/index/ignore/patterns POST":     "addProjectIndexIgnorePattern",
		"/api/projects/{project}/index/ignore/patterns DELETE":   "removeProjectIndexIgnorePattern",
	}
	for key, expected := range expectedOperations {
		path, method, _ := strings.Cut(key, " ")
		route, ok := spec.Paths[path][strings.ToLower(method)]
		if !ok {
			t.Fatalf("openapi spec missing operation %s", key)
		}
		if route.OperationID != expected {
			t.Fatalf("operation %s id = %q, want %q", key, route.OperationID, expected)
		}
	}

	for _, name := range []string{"AgentListResponse", "AgentResponse", "CreateAgentResponse", "AgentOutput", "AgentProjectOutput", "CreateAgentInput", "UpdateAgentInput", "ProjectListResponse", "ProjectResponse", "ProjectInstructionListResponse", "ProjectInstructionResponse", "ProjectInstructionOutput", "ProjectInstructionInput", "TaskListResponse", "TaskResponse", "TaskShowResponse", "TaskProject", "TaskEventResponse", "TaskDependencyOutput", "CreateTaskInput", "UpdateProjectInput", "ClaimTaskInput", "TaskDoneInput", "IndexResponse", "IndexListResponse", "IndexIgnorePolicyResponse", "IndexIgnorePatternInput"} {
		if _, ok := spec.Components.Schemas[name]; !ok {
			t.Fatalf("openapi spec missing schema %s", name)
		}
	}
	for schemaName, fields := range map[string][]string{
		"ProjectListResponse":            {"projects", "total", "limit", "next_cursor"},
		"ProjectInstructionListResponse": {"instructions"},
		"ProjectInstructionResponse":     {"instruction"},
		"ProjectInstructionOutput":       {"id", "project_id", "title", "body", "priority", "enabled", "source"},
		"ProjectInstructionInput":        {"title", "body", "priority"},
		"TaskListResponse":               {"tasks", "total", "limit", "next_cursor"},
		"ProjectOutput":                  {"tasks_count", "task_counts", "agents"},
		"TaskCounts":                     {"total", "open", "in_progress", "blocked", "done", "ready"},
		"TaskOutput":                     {"project", "agents"},
		"TaskProject":                    {"id", "name", "display_name"},
		"TaskShowResponse":               {"dependencies"},
		"AgentListResponse":              {"agents"},
		"AgentResponse":                  {"agent"},
		"CreateAgentResponse":            {"agent", "token"},
		"AgentOutput":                    {"projects", "tasks_count", "events_count", "last_activity_at"},
		"AgentProjectOutput":             {"display_name", "tasks_count", "events_count", "last_activity_at"},
		"IndexResponse":                  {"state", "path_exists", "indexed_chunks", "last_error"},
		"IndexListResponse":              {"indexes", "total"},
		"IndexIgnorePolicyResponse":      {"project_name", "ignore_patterns", "seeded_from_gitignore"},
		"IndexIgnorePatternInput":        {"pattern"},
	} {
		props := openAPISchemaProperties(t, spec.Components.Schemas[schemaName])
		for _, field := range fields {
			if _, ok := props[field]; !ok {
				t.Fatalf("openapi schema %s missing field %s: %+v", schemaName, field, props)
			}
		}
	}

	claim := spec.Paths["/api/projects/{project}/tasks/claim"]["post"]
	if claim.RequestBody == nil || !slices.Contains(claim.RequestBody.ContentTypes(), "application/json") {
		t.Fatalf("claim request body should be application/json, got %+v", claim.RequestBody)
	}
	createTask := spec.Paths["/api/projects/{project}/tasks"]["post"]
	if createTask.RequestBody == nil || !slices.Contains(createTask.RequestBody.ContentTypes(), "application/json") {
		t.Fatalf("create task request body should be application/json, got %+v", createTask.RequestBody)
	}
	updateProject := spec.Paths["/api/projects/{project}"]["patch"]
	if updateProject.RequestBody == nil || !slices.Contains(updateProject.RequestBody.ContentTypes(), "application/json") {
		t.Fatalf("update project request body should be application/json, got %+v", updateProject.RequestBody)
	}
	createInstruction := spec.Paths["/api/projects/{project}/instructions"]["post"]
	if createInstruction.RequestBody == nil || !slices.Contains(createInstruction.RequestBody.ContentTypes(), "application/json") {
		t.Fatalf("create instruction request body should be application/json, got %+v", createInstruction.RequestBody)
	}
	createAgent := spec.Paths["/api/agents"]["post"]
	if createAgent.RequestBody == nil || !slices.Contains(createAgent.RequestBody.ContentTypes(), "application/json") {
		t.Fatalf("create agent request body should be application/json, got %+v", createAgent.RequestBody)
	}
	updateAgent := spec.Paths["/api/agents/{id}"]["patch"]
	if updateAgent.RequestBody == nil || !slices.Contains(updateAgent.RequestBody.ContentTypes(), "application/json") {
		t.Fatalf("update agent request body should be application/json, got %+v", updateAgent.RequestBody)
	}
}

func TestServerServesWebUIAssetsAndSPAFallback(t *testing.T) {
	store := openTestStore(t)

	server, err := New(Config{
		Addr:    "127.0.0.1:0",
		Store:   store,
		Version: "test",
		WebFS:   testWebFS(),
	})
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	handler := server.Handler()

	for _, requestPath := range []string{"/", "/projects/tok"} {
		res := doJSON(t, handler, http.MethodGet, requestPath, nil)
		defer res.Body.Close()
		if res.StatusCode != http.StatusOK {
			t.Fatalf("GET %s status = %d", requestPath, res.StatusCode)
		}
		body := readBody(t, res)
		if !strings.Contains(body, "TOK UI") {
			t.Fatalf("GET %s body = %q, want web UI index", requestPath, body)
		}
	}

	assetRes := doJSON(t, handler, http.MethodGet, "/assets/app.js", nil)
	defer assetRes.Body.Close()
	if assetRes.StatusCode != http.StatusOK {
		t.Fatalf("GET /assets/app.js status = %d", assetRes.StatusCode)
	}
	if body := readBody(t, assetRes); !strings.Contains(body, "console.log") {
		t.Fatalf("asset body = %q", body)
	}

	underscoreAssetRes := doJSON(t, handler, http.MethodGet, "/assets/_project_.js", nil)
	defer underscoreAssetRes.Body.Close()
	if underscoreAssetRes.StatusCode != http.StatusOK {
		t.Fatalf("GET /assets/_project_.js status = %d", underscoreAssetRes.StatusCode)
	}
	if contentType := underscoreAssetRes.Header.Get("Content-Type"); strings.Contains(contentType, "text/html") {
		t.Fatalf("underscore asset content type = %q, want script asset", contentType)
	}
	if body := readBody(t, underscoreAssetRes); !strings.Contains(body, "project chunk") {
		t.Fatalf("underscore asset body = %q", body)
	}

	apiRes := doJSON(t, handler, http.MethodGet, "/api/not-found", nil)
	defer apiRes.Body.Close()
	if apiRes.StatusCode != http.StatusNotFound {
		t.Fatalf("GET /api/not-found status = %d", apiRes.StatusCode)
	}
	if body := readBody(t, apiRes); strings.Contains(body, "TOK UI") {
		t.Fatalf("unknown API route served web UI: %q", body)
	}
}

func TestServerPaginatesProjects(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)

	for _, name := range []string{"alpha", "bravo", "charlie", "delta"} {
		if _, err := store.CreateProject(ctx, storage.CreateProjectInput{
			Name:        name,
			DisplayName: strings.ToUpper(name),
			Path:        t.TempDir(),
		}); err != nil {
			t.Fatalf("CreateProject(%q) returned error: %v", name, err)
		}
	}

	handler := newTestHandler(t, store)
	res := doJSON(t, handler, http.MethodGet, "/api/projects?limit=2", nil)
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/projects?limit=2 status = %d", res.StatusCode)
	}
	var projects ProjectListResponse
	decodeJSON(t, res, &projects)

	if projects.Total != 4 || projects.Limit != 2 || projects.NextCursor != "bravo" {
		t.Fatalf("unexpected pagination metadata: %+v", projects)
	}
	if got := projectNames(projects.Projects); !slices.Equal(got, []string{"alpha", "bravo"}) {
		t.Fatalf("unexpected first page projects: %v", got)
	}

	pagedRes := doJSON(t, handler, http.MethodGet, "/api/projects?limit=2&cursor=bravo", nil)
	defer pagedRes.Body.Close()
	if pagedRes.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/projects?limit=2&cursor=bravo status = %d", pagedRes.StatusCode)
	}
	var pagedProjects ProjectListResponse
	decodeJSON(t, pagedRes, &pagedProjects)

	if pagedProjects.Total != 4 || pagedProjects.Limit != 2 || pagedProjects.NextCursor != "" {
		t.Fatalf("unexpected second page projects metadata: %+v", pagedProjects)
	}
	if got := projectNames(pagedProjects.Projects); !slices.Equal(got, []string{"charlie", "delta"}) {
		t.Fatalf("unexpected paged projects: %v", got)
	}
}

func TestServerCreatesUpdatesAndDeletesAgents(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	handler := newTestHandler(t, store)

	createRes := doJSON(t, handler, http.MethodPost, "/api/agents", CreateAgentInput{Name: "Codex UI"})
	defer createRes.Body.Close()
	if createRes.StatusCode != http.StatusOK {
		t.Fatalf("create agent status = %d", createRes.StatusCode)
	}
	var created CreateAgentResponse
	decodeJSON(t, createRes, &created)
	if created.Agent.Name != "Codex UI" || !strings.HasPrefix(created.Token, "tok_agent_") {
		t.Fatalf("unexpected created agent: %+v", created)
	}

	if _, err := store.CreateAgent(ctx, "Claude"); err != nil {
		t.Fatalf("CreateAgent duplicate target returned error: %v", err)
	}
	duplicateRes := doJSON(t, handler, http.MethodPatch, "/api/agents/"+jsonNumber(created.Agent.ID), UpdateAgentInput{Name: "Claude"})
	defer duplicateRes.Body.Close()
	if duplicateRes.StatusCode != http.StatusConflict {
		t.Fatalf("duplicate update status = %d", duplicateRes.StatusCode)
	}

	updateRes := doJSON(t, handler, http.MethodPatch, "/api/agents/"+jsonNumber(created.Agent.ID), UpdateAgentInput{Name: "Codex Backend"})
	defer updateRes.Body.Close()
	if updateRes.StatusCode != http.StatusOK {
		t.Fatalf("update agent status = %d", updateRes.StatusCode)
	}
	var updated AgentResponse
	decodeJSON(t, updateRes, &updated)
	if updated.Agent.ID != created.Agent.ID || updated.Agent.Name != "Codex Backend" {
		t.Fatalf("unexpected updated agent: %+v", updated)
	}

	showRes := doJSON(t, handler, http.MethodGet, "/api/agents/"+jsonNumber(created.Agent.ID), nil)
	defer showRes.Body.Close()
	if showRes.StatusCode != http.StatusOK {
		t.Fatalf("show agent status = %d", showRes.StatusCode)
	}
	var shown AgentResponse
	decodeJSON(t, showRes, &shown)
	if shown.Agent.Name != "Codex Backend" {
		t.Fatalf("unexpected shown agent: %+v", shown)
	}

	deleteRes := doJSON(t, handler, http.MethodDelete, "/api/agents/"+jsonNumber(created.Agent.ID), nil)
	defer deleteRes.Body.Close()
	if deleteRes.StatusCode != http.StatusOK {
		t.Fatalf("delete agent status = %d", deleteRes.StatusCode)
	}
	var deleted AgentResponse
	decodeJSON(t, deleteRes, &deleted)
	if deleted.Agent.ID != created.Agent.ID || deleted.Agent.Name != "Codex Backend" {
		t.Fatalf("unexpected deleted agent response: %+v", deleted)
	}

	missingRes := doJSON(t, handler, http.MethodGet, "/api/agents/"+jsonNumber(created.Agent.ID), nil)
	defer missingRes.Body.Close()
	if missingRes.StatusCode != http.StatusNotFound {
		t.Fatalf("show deleted agent status = %d", missingRes.StatusCode)
	}
}

func TestServerPaginatesTasks(t *testing.T) {
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
	for _, title := range []string{"first", "second", "third", "fourth"} {
		if _, err := store.CreateTask(ctx, storage.CreateTaskInput{
			ProjectID: project.ID,
			Title:     title,
		}); err != nil {
			t.Fatalf("CreateTask(%q) returned error: %v", title, err)
		}
	}
	otherProject, err := store.CreateProject(ctx, storage.CreateProjectInput{
		Name:        "other",
		DisplayName: "Other",
		Path:        t.TempDir(),
	})
	if err != nil {
		t.Fatalf("CreateProject other returned error: %v", err)
	}
	if _, err := store.CreateTask(ctx, storage.CreateTaskInput{
		ProjectID: otherProject.ID,
		Title:     "other task",
	}); err != nil {
		t.Fatalf("CreateTask other returned error: %v", err)
	}

	handler := newTestHandler(t, store)
	res := doJSON(t, handler, http.MethodGet, "/api/tasks?limit=2", nil)
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/tasks?limit=2 status = %d", res.StatusCode)
	}
	var tasks TaskListResponse
	decodeJSON(t, res, &tasks)

	if tasks.Total != 5 || tasks.Limit != 2 || tasks.NextCursor != "4" {
		t.Fatalf("unexpected pagination metadata: %+v", tasks)
	}
	if got := taskTitles(tasks.Tasks); !slices.Equal(got, []string{"other task", "fourth"}) {
		t.Fatalf("unexpected paged tasks: %v", got)
	}

	pagedRes := doJSON(t, handler, http.MethodGet, "/api/tasks?limit=2&cursor=4", nil)
	defer pagedRes.Body.Close()
	if pagedRes.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/tasks?limit=2&cursor=4 status = %d", pagedRes.StatusCode)
	}
	var pagedTasks TaskListResponse
	decodeJSON(t, pagedRes, &pagedTasks)
	if pagedTasks.Total != 5 || pagedTasks.Limit != 2 || pagedTasks.NextCursor != "2" {
		t.Fatalf("unexpected second page tasks metadata: %+v", pagedTasks)
	}
	if got := taskTitles(pagedTasks.Tasks); !slices.Equal(got, []string{"third", "second"}) {
		t.Fatalf("unexpected second page tasks: %v", got)
	}

	if tasks.Tasks[0].Project.ID != otherProject.ID || tasks.Tasks[0].Project.Name != "other" || tasks.Tasks[0].Project.DisplayName != "Other" {
		t.Fatalf("unexpected task project summary: %+v", tasks.Tasks[0].Project)
	}

	filteredRes := doJSON(t, handler, http.MethodGet, "/api/tasks?projectId="+jsonNumber(project.ID)+"&status=open&limit=10", nil)
	defer filteredRes.Body.Close()
	if filteredRes.StatusCode != http.StatusOK {
		t.Fatalf("GET filtered tasks status = %d", filteredRes.StatusCode)
	}
	var filtered TaskListResponse
	decodeJSON(t, filteredRes, &filtered)
	if filtered.Total != 4 || len(filtered.Tasks) != 4 {
		t.Fatalf("unexpected filtered tasks: %+v", filtered)
	}
}

func TestServerUpdatesAndDeletesProjects(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)

	if _, err := store.CreateProject(ctx, storage.CreateProjectInput{
		Name:        "tok",
		DisplayName: "TOK",
		Path:        t.TempDir(),
	}); err != nil {
		t.Fatalf("CreateProject returned error: %v", err)
	}
	handler := newTestHandler(t, store)

	updateRes := doJSON(t, handler, http.MethodPatch, "/api/projects/tok", map[string]string{
		"name":         "tok-renamed",
		"display_name": "TOK Renamed",
		"path":         t.TempDir(),
	})
	defer updateRes.Body.Close()
	if updateRes.StatusCode != http.StatusOK {
		t.Fatalf("PATCH /api/projects/tok status = %d", updateRes.StatusCode)
	}
	var updated ProjectResponse
	decodeJSON(t, updateRes, &updated)
	if updated.Project.Name != "tok-renamed" || updated.Project.DisplayName != "TOK Renamed" {
		t.Fatalf("unexpected updated project: %+v", updated)
	}

	oldShowRes := doJSON(t, handler, http.MethodGet, "/api/projects/tok", nil)
	defer oldShowRes.Body.Close()
	if oldShowRes.StatusCode != http.StatusNotFound {
		t.Fatalf("GET old project status = %d", oldShowRes.StatusCode)
	}

	deleteRes := doJSON(t, handler, http.MethodDelete, "/api/projects/tok-renamed", nil)
	defer deleteRes.Body.Close()
	if deleteRes.StatusCode != http.StatusOK {
		t.Fatalf("DELETE /api/projects/tok-renamed status = %d", deleteRes.StatusCode)
	}
	var deleted ProjectResponse
	decodeJSON(t, deleteRes, &deleted)
	if deleted.Project.Name != "tok-renamed" {
		t.Fatalf("unexpected deleted project response: %+v", deleted)
	}

	showDeletedRes := doJSON(t, handler, http.MethodGet, "/api/projects/tok-renamed", nil)
	defer showDeletedRes.Body.Close()
	if showDeletedRes.StatusCode != http.StatusNotFound {
		t.Fatalf("GET deleted project status = %d", showDeletedRes.StatusCode)
	}
}

func TestServerManagesProjectInstructions(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)

	if _, err := store.CreateProject(ctx, storage.CreateProjectInput{
		Name:        "tok",
		DisplayName: "TOK",
		Path:        t.TempDir(),
	}); err != nil {
		t.Fatalf("CreateProject returned error: %v", err)
	}
	handler := newTestHandler(t, store)

	createRes := doJSON(t, handler, http.MethodPost, "/api/projects/tok/instructions", ProjectInstructionInput{
		Title:    "Use Context7",
		Body:     "Use Context7 for library documentation.",
		Priority: "high",
	})
	defer createRes.Body.Close()
	if createRes.StatusCode != http.StatusOK {
		t.Fatalf("create instruction status = %d", createRes.StatusCode)
	}
	var created ProjectInstructionResponse
	decodeJSON(t, createRes, &created)
	if created.Instruction.ID == 0 || created.Instruction.Title != "Use Context7" || created.Instruction.Priority != "high" || !created.Instruction.Enabled {
		t.Fatalf("unexpected created instruction: %+v", created)
	}

	listRes := doJSON(t, handler, http.MethodGet, "/api/projects/tok/instructions", nil)
	defer listRes.Body.Close()
	if listRes.StatusCode != http.StatusOK {
		t.Fatalf("list instructions status = %d", listRes.StatusCode)
	}
	var listed ProjectInstructionListResponse
	decodeJSON(t, listRes, &listed)
	if len(listed.Instructions) != 1 || listed.Instructions[0].ID != created.Instruction.ID {
		t.Fatalf("unexpected listed instructions: %+v", listed)
	}

	disableRes := doJSON(t, handler, http.MethodPost, "/api/projects/tok/instructions/"+jsonNumber(created.Instruction.ID)+"/disable", nil)
	defer disableRes.Body.Close()
	if disableRes.StatusCode != http.StatusOK {
		t.Fatalf("disable instruction status = %d", disableRes.StatusCode)
	}
	var disabled ProjectInstructionResponse
	decodeJSON(t, disableRes, &disabled)
	if disabled.Instruction.Enabled {
		t.Fatalf("expected disabled instruction: %+v", disabled)
	}

	enabledListRes := doJSON(t, handler, http.MethodGet, "/api/projects/tok/instructions", nil)
	defer enabledListRes.Body.Close()
	if enabledListRes.StatusCode != http.StatusOK {
		t.Fatalf("enabled list instructions status = %d", enabledListRes.StatusCode)
	}
	var enabledList ProjectInstructionListResponse
	decodeJSON(t, enabledListRes, &enabledList)
	if len(enabledList.Instructions) != 0 {
		t.Fatalf("disabled instruction appeared in enabled list: %+v", enabledList)
	}

	includeDisabledRes := doJSON(t, handler, http.MethodGet, "/api/projects/tok/instructions?includeDisabled=true", nil)
	defer includeDisabledRes.Body.Close()
	if includeDisabledRes.StatusCode != http.StatusOK {
		t.Fatalf("include disabled list status = %d", includeDisabledRes.StatusCode)
	}
	var includeDisabled ProjectInstructionListResponse
	decodeJSON(t, includeDisabledRes, &includeDisabled)
	if len(includeDisabled.Instructions) != 1 || includeDisabled.Instructions[0].Enabled {
		t.Fatalf("expected disabled instruction in includeDisabled list: %+v", includeDisabled)
	}

	deleteRes := doJSON(t, handler, http.MethodDelete, "/api/projects/tok/instructions/"+jsonNumber(created.Instruction.ID), nil)
	defer deleteRes.Body.Close()
	if deleteRes.StatusCode != http.StatusOK {
		t.Fatalf("delete instruction status = %d", deleteRes.StatusCode)
	}
	var deleted ProjectInstructionResponse
	decodeJSON(t, deleteRes, &deleted)
	if deleted.Instruction.ID != created.Instruction.ID {
		t.Fatalf("unexpected deleted instruction: %+v", deleted)
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
	task, err := store.CreateTask(ctx, storage.CreateTaskInput{
		ProjectID: project.ID,
		Title:     "HTTP missing evidence completion",
	})
	if err != nil {
		t.Fatalf("CreateTask returned error: %v", err)
	}
	if _, err := store.ClaimTask(ctx, project.ID, task.ID); err != nil {
		t.Fatalf("ClaimTask returned error: %v", err)
	}

	handler := newTestHandler(t, store)
	res := doJSON(t, handler, http.MethodPost, "/api/tasks/"+jsonNumber(task.ID)+"/done", TaskDoneInput{
		Note: "Done without evidence.",
	})
	defer res.Body.Close()
	if res.StatusCode != http.StatusConflict {
		t.Fatalf("POST /api/tasks/{id}/done status = %d body=%s", res.StatusCode, readBody(t, res))
	}
}

func TestServerTaskActionsWriteHumanActorHistory(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)

	if _, err := store.CreateProject(ctx, storage.CreateProjectInput{
		Name:        "tok",
		DisplayName: "TOK",
		Path:        t.TempDir(),
	}); err != nil {
		t.Fatalf("CreateProject returned error: %v", err)
	}
	if _, err := store.SetLocalHuman(ctx, "TOK Operator"); err != nil {
		t.Fatalf("SetLocalHuman returned error: %v", err)
	}

	handler := newTestHandler(t, store)

	createRes := doJSON(t, handler, http.MethodPost, "/api/projects/tok/tasks", map[string]string{
		"title":               "Build local UI API",
		"description":         "Expose task actions over HTTP.",
		"acceptance_criteria": "OpenAPI includes task creation.",
		"notes":               "Keep UI untouched.",
	})
	defer createRes.Body.Close()
	if createRes.StatusCode != http.StatusOK {
		t.Fatalf("create task status = %d", createRes.StatusCode)
	}
	var created TaskResponse
	decodeJSON(t, createRes, &created)
	if created.Task.ID <= 0 || created.Task.Status != "open" || created.Task.Title != "Build local UI API" {
		t.Fatalf("unexpected created task: %+v", created)
	}

	claimRes := doJSON(t, handler, http.MethodPost, "/api/projects/tok/tasks/claim", map[string]any{"id": created.Task.ID})
	defer claimRes.Body.Close()
	if claimRes.StatusCode != http.StatusOK {
		t.Fatalf("claim status = %d", claimRes.StatusCode)
	}
	var claimed TaskResponse
	decodeJSON(t, claimRes, &claimed)
	if claimed.Task.Status != "in_progress" {
		t.Fatalf("expected in_progress after claim, got %+v", claimed)
	}

	commentRes := doJSON(t, handler, http.MethodPost, "/api/tasks/"+jsonNumber(created.Task.ID)+"/comment", map[string]string{"body": "HTTP API works."})
	defer commentRes.Body.Close()
	if commentRes.StatusCode != http.StatusOK {
		t.Fatalf("comment status = %d", commentRes.StatusCode)
	}

	showRes := doJSON(t, handler, http.MethodGet, "/api/tasks/"+jsonNumber(created.Task.ID), nil)
	defer showRes.Body.Close()
	if showRes.StatusCode != http.StatusOK {
		t.Fatalf("show status = %d", showRes.StatusCode)
	}
	var shown TaskShowResponse
	decodeJSON(t, showRes, &shown)
	if len(shown.Events) != 3 {
		t.Fatalf("expected created, claimed and commented events, got %+v", shown.Events)
	}
	claimedEvent := shown.Events[1]
	commentEvent := shown.Events[2]
	if claimedEvent.Actor == nil || claimedEvent.Actor.Kind != "human" || claimedEvent.Actor.Name != "TOK Operator" {
		t.Fatalf("claim event missing human actor: %+v", claimedEvent)
	}
	if commentEvent.Actor == nil || commentEvent.Actor.Name != "TOK Operator" || commentEvent.Body != "HTTP API works." {
		t.Fatalf("comment event missing actor/body: %+v", commentEvent)
	}
}

func TestServerAggregatesAgentsFromTaskHistory(t *testing.T) {
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
	task, err := store.CreateTask(ctx, storage.CreateTaskInput{
		ProjectID: project.ID,
		Title:     "Render agents in UI",
	})
	if err != nil {
		t.Fatalf("CreateTask returned error: %v", err)
	}
	agent, err := store.CreateAgent(ctx, "Codex Backend")
	if err != nil {
		t.Fatalf("CreateAgent returned error: %v", err)
	}
	idleAgent, err := store.CreateAgent(ctx, "Claude Frontend")
	if err != nil {
		t.Fatalf("CreateAgent idle returned error: %v", err)
	}
	blockedByDependency, err := store.CreateTask(ctx, storage.CreateTaskInput{
		ProjectID: project.ID,
		Title:     "Wait for renderer",
	})
	if err != nil {
		t.Fatalf("CreateTask blocked dependency returned error: %v", err)
	}
	if _, err := store.AddTaskDependency(ctx, "blocks", task.ID, blockedByDependency.ID); err != nil {
		t.Fatalf("AddTaskDependency returned error: %v", err)
	}
	inProgressTask, err := store.CreateTask(ctx, storage.CreateTaskInput{
		ProjectID: project.ID,
		Title:     "Wire HTTP client",
	})
	if err != nil {
		t.Fatalf("CreateTask in progress returned error: %v", err)
	}
	if _, err := store.ClaimTaskByActor(ctx, project.ID, inProgressTask.ID, storage.ActorRefFromActor(agent.Agent)); err != nil {
		t.Fatalf("ClaimTaskByActor returned error: %v", err)
	}
	blockedStatusTask, err := store.CreateTask(ctx, storage.CreateTaskInput{
		ProjectID: project.ID,
		Title:     "Blocked implementation",
	})
	if err != nil {
		t.Fatalf("CreateTask blocked status returned error: %v", err)
	}
	if _, err := store.BlockTaskByActor(ctx, blockedStatusTask.ID, "Waiting for design decision.", storage.ActorRefFromActor(agent.Agent)); err != nil {
		t.Fatalf("BlockTaskByActor returned error: %v", err)
	}
	doneTask, err := store.CreateTask(ctx, storage.CreateTaskInput{
		ProjectID: project.ID,
		Title:     "Completed implementation",
	})
	if err != nil {
		t.Fatalf("CreateTask done returned error: %v", err)
	}
	if _, err := store.ClaimTaskByActor(ctx, project.ID, doneTask.ID, storage.ActorRefFromActor(agent.Agent)); err != nil {
		t.Fatalf("ClaimTaskByActor done returned error: %v", err)
	}
	if _, err := store.CompleteTaskWithOptions(ctx, storage.CompleteTaskInput{
		ID:               doneTask.ID,
		Note:             "Done.",
		AllowUnvalidated: true,
		OverrideReason:   "HTTP aggregate fixture override.",
		Actor:            storage.ActorRefFromActor(agent.Agent),
	}); err != nil {
		t.Fatalf("CompleteTaskByActor returned error: %v", err)
	}
	if _, err := store.AddTaskCommentByActor(ctx, task.ID, "HTTP aggregate is ready.", storage.ActorRefFromActor(agent.Agent)); err != nil {
		t.Fatalf("AddTaskCommentByActor returned error: %v", err)
	}

	handler := newTestHandler(t, store)

	projectsRes := doJSON(t, handler, http.MethodGet, "/api/projects", nil)
	defer projectsRes.Body.Close()
	if projectsRes.StatusCode != http.StatusOK {
		t.Fatalf("projects status = %d", projectsRes.StatusCode)
	}
	var projects ProjectListResponse
	decodeJSON(t, projectsRes, &projects)
	if len(projects.Projects) != 1 || projects.Projects[0].TasksCount != 5 {
		t.Fatalf("unexpected project aggregate: %+v", projects)
	}
	assertTaskCounts(t, projects.Projects[0].TaskCounts, TaskCounts{
		Total:      5,
		Open:       2,
		InProgress: 1,
		Blocked:    1,
		Done:       1,
		Ready:      1,
	})
	assertSingleAgent(t, projects.Projects[0].Agents, agent.Agent.ID, "Codex Backend")

	projectRes := doJSON(t, handler, http.MethodGet, "/api/projects/tok", nil)
	defer projectRes.Body.Close()
	if projectRes.StatusCode != http.StatusOK {
		t.Fatalf("project status = %d", projectRes.StatusCode)
	}
	var projectOut ProjectResponse
	decodeJSON(t, projectRes, &projectOut)
	assertTaskCounts(t, projectOut.Project.TaskCounts, TaskCounts{
		Total:      5,
		Open:       2,
		InProgress: 1,
		Blocked:    1,
		Done:       1,
		Ready:      1,
	})
	assertSingleAgent(t, projectOut.Project.Agents, agent.Agent.ID, "Codex Backend")

	tasksRes := doJSON(t, handler, http.MethodGet, "/api/projects/tok/tasks", nil)
	defer tasksRes.Body.Close()
	if tasksRes.StatusCode != http.StatusOK {
		t.Fatalf("tasks status = %d", tasksRes.StatusCode)
	}
	var tasks TaskListResponse
	decodeJSON(t, tasksRes, &tasks)
	if len(tasks.Tasks) != 5 || tasks.Total != 5 || tasks.Limit != 25 || tasks.NextCursor != "" {
		t.Fatalf("unexpected tasks response: %+v", tasks)
	}
	assertSingleAgent(t, tasks.Tasks[0].Agents, agent.Agent.ID, "Codex Backend")
	if tasks.Tasks[0].Project.ID != project.ID || tasks.Tasks[0].Project.DisplayName != "TOK" {
		t.Fatalf("unexpected project summary in task list: %+v", tasks.Tasks[0].Project)
	}

	pagedTasksRes := doJSON(t, handler, http.MethodGet, "/api/projects/tok/tasks?limit=2", nil)
	defer pagedTasksRes.Body.Close()
	if pagedTasksRes.StatusCode != http.StatusOK {
		t.Fatalf("paged tasks status = %d", pagedTasksRes.StatusCode)
	}
	var pagedTasks TaskListResponse
	decodeJSON(t, pagedTasksRes, &pagedTasks)
	if pagedTasks.Total != 5 || pagedTasks.Limit != 2 || pagedTasks.NextCursor != "4" {
		t.Fatalf("unexpected paged tasks metadata: %+v", pagedTasks)
	}
	if got := taskTitles(pagedTasks.Tasks); !slices.Equal(got, []string{"Completed implementation", "Blocked implementation"}) {
		t.Fatalf("unexpected paged task titles: %v", got)
	}

	secondPageTasksRes := doJSON(t, handler, http.MethodGet, "/api/projects/tok/tasks?limit=2&cursor=4", nil)
	defer secondPageTasksRes.Body.Close()
	if secondPageTasksRes.StatusCode != http.StatusOK {
		t.Fatalf("second page project tasks status = %d", secondPageTasksRes.StatusCode)
	}
	var secondPageTasks TaskListResponse
	decodeJSON(t, secondPageTasksRes, &secondPageTasks)
	if secondPageTasks.Total != 5 || secondPageTasks.Limit != 2 || secondPageTasks.NextCursor != "2" {
		t.Fatalf("unexpected second page project tasks metadata: %+v", secondPageTasks)
	}
	if got := taskTitles(secondPageTasks.Tasks); !slices.Equal(got, []string{"Wire HTTP client", "Wait for renderer"}) {
		t.Fatalf("unexpected second page task titles: %v", got)
	}

	showRes := doJSON(t, handler, http.MethodGet, "/api/tasks/"+jsonNumber(task.ID), nil)
	defer showRes.Body.Close()
	if showRes.StatusCode != http.StatusOK {
		t.Fatalf("show status = %d", showRes.StatusCode)
	}
	var shown TaskShowResponse
	decodeJSON(t, showRes, &shown)
	assertSingleAgent(t, shown.Task.Agents, agent.Agent.ID, "Codex Backend")
	if shown.Task.Project.ID != project.ID || shown.Task.Project.Name != "tok" || shown.Task.Project.DisplayName != "TOK" {
		t.Fatalf("unexpected shown task project summary: %+v", shown.Task.Project)
	}
	if len(shown.Dependencies) != 1 || shown.Dependencies[0].Role != "blocks" {
		t.Fatalf("unexpected task dependencies: %+v", shown.Dependencies)
	}

	agentsRes := doJSON(t, handler, http.MethodGet, "/api/agents", nil)
	defer agentsRes.Body.Close()
	if agentsRes.StatusCode != http.StatusOK {
		t.Fatalf("agents status = %d", agentsRes.StatusCode)
	}
	var agents AgentListResponse
	decodeJSON(t, agentsRes, &agents)
	if len(agents.Agents) != 2 {
		t.Fatalf("expected two registered agents, got %+v", agents)
	}
	active := findAgentOutput(t, agents.Agents, agent.Agent.ID)
	if active.Name != "Codex Backend" || active.TasksCount != 4 || active.EventsCount != 6 || active.LastActivityAt == "" {
		t.Fatalf("unexpected active agent aggregate: %+v", active)
	}
	if len(active.Projects) != 1 || active.Projects[0].ID != project.ID || active.Projects[0].TasksCount != 4 || active.Projects[0].EventsCount != 6 {
		t.Fatalf("unexpected active agent projects: %+v", active.Projects)
	}
	idle := findAgentOutput(t, agents.Agents, idleAgent.Agent.ID)
	if idle.Name != "Claude Frontend" || idle.TasksCount != 0 || idle.EventsCount != 0 || idle.LastActivityAt != "" || len(idle.Projects) != 0 {
		t.Fatalf("unexpected idle agent aggregate: %+v", idle)
	}
}

func newTestHandler(t *testing.T, store *storage.Store) http.Handler {
	t.Helper()
	server, err := New(Config{
		Addr:    "127.0.0.1:0",
		Store:   store,
		Version: "test",
	})
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	return server.Handler()
}

func testWebFS() fstest.MapFS {
	return fstest.MapFS{
		"index.html":          {Data: []byte("<!doctype html><title>TOK UI</title><div id=\"app\"></div>")},
		"assets/app.js":       {Data: []byte("console.log('tok')")},
		"assets/_project_.js": {Data: []byte("console.log('project chunk')")},
	}
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

func doJSON(t *testing.T, handler http.Handler, method, path string, body any) *http.Response {
	t.Helper()
	var payload *bytes.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal request: %v", err)
		}
		payload = bytes.NewReader(data)
	} else {
		payload = bytes.NewReader(nil)
	}

	req := httptest.NewRequest(method, path, payload)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	return res.Result()
}

func decodeJSON(t *testing.T, res *http.Response, dst any) {
	t.Helper()
	if err := json.NewDecoder(res.Body).Decode(dst); err != nil {
		t.Fatalf("decode response: %v", err)
	}
}

func readBody(t *testing.T, res *http.Response) string {
	t.Helper()
	data, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatalf("read response body: %v", err)
	}
	return string(data)
}

func jsonNumber(id int64) string {
	return strings.Trim(string(mustJSON(id)), `"`)
}

func mustJSON(value any) []byte {
	data, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return data
}

func assertSingleAgent(t *testing.T, agents []ActorOutput, id int64, name string) {
	t.Helper()
	if len(agents) != 1 {
		t.Fatalf("expected one agent, got %+v", agents)
	}
	if agents[0].ID != id || agents[0].Kind != "agent" || agents[0].Name != name {
		t.Fatalf("unexpected agent: %+v", agents[0])
	}
}

func findAgentOutput(t *testing.T, agents []AgentOutput, id int64) AgentOutput {
	t.Helper()
	for _, agent := range agents {
		if agent.ID == id {
			return agent
		}
	}
	t.Fatalf("agent %d not found in %+v", id, agents)
	return AgentOutput{}
}

func projectNames(projects []ProjectOutput) []string {
	names := make([]string, 0, len(projects))
	for _, project := range projects {
		names = append(names, project.Name)
	}
	return names
}

func taskTitles(tasks []TaskOutput) []string {
	titles := make([]string, 0, len(tasks))
	for _, task := range tasks {
		titles = append(titles, task.Title)
	}
	return titles
}

func assertTaskCounts(t *testing.T, got, want TaskCounts) {
	t.Helper()
	if got != want {
		t.Fatalf("unexpected task counts: got %+v, want %+v", got, want)
	}
}

func openAPISchemaProperties(t *testing.T, schema any) map[string]any {
	t.Helper()
	raw, ok := schema.(map[string]any)
	if !ok {
		t.Fatalf("openapi schema has unexpected type %T", schema)
	}
	props, ok := raw["properties"].(map[string]any)
	if !ok {
		t.Fatalf("openapi schema missing properties: %+v", raw)
	}
	return props
}

type openAPIRoute struct {
	OperationID string              `json:"operationId"`
	RequestBody *openAPIRequestBody `json:"requestBody"`
}

type openAPIRequestBody struct {
	Content map[string]any `json:"content"`
}

func (b openAPIRequestBody) ContentTypes() []string {
	types := make([]string, 0, len(b.Content))
	for contentType := range b.Content {
		types = append(types, contentType)
	}
	return types
}
