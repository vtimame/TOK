package httpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

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
	if projects.Total != 1 || projects.Limit != 1 || projects.Offset != 0 {
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
		"/api/health GET":                           "getHealth",
		"/api/projects GET":                         "listProjects",
		"/api/projects POST":                        "createProject",
		"/api/projects/{project} GET":               "showProject",
		"/api/projects/{project} PATCH":             "updateProject",
		"/api/projects/{project} DELETE":            "deleteProject",
		"/api/projects/{project}/tasks GET":         "listProjectTasks",
		"/api/projects/{project}/tasks POST":        "createTask",
		"/api/projects/{project}/tasks/ready GET":   "listReadyTasks",
		"/api/projects/{project}/tasks/claim POST":  "claimTask",
		"/api/tasks GET":                            "listTasks",
		"/api/tasks/{id} GET":                       "showTask",
		"/api/tasks/{id}/comment POST":              "commentTask",
		"/api/tasks/{id}/progress POST":             "progressTask",
		"/api/tasks/{id}/block POST":                "blockTask",
		"/api/tasks/{id}/unblock POST":              "unblockTask",
		"/api/tasks/{id}/done POST":                 "completeTask",
		"/api/projects/{project}/index GET":         "getProjectIndexStatus",
		"/api/projects/{project}/index/update POST": "updateProjectIndex",
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

	for _, name := range []string{"ProjectListResponse", "ProjectResponse", "TaskListResponse", "TaskResponse", "TaskShowResponse", "TaskEventResponse", "TaskDependencyOutput", "CreateTaskInput", "UpdateProjectInput", "ClaimTaskInput", "TaskDoneInput", "IndexResponse"} {
		if _, ok := spec.Components.Schemas[name]; !ok {
			t.Fatalf("openapi spec missing schema %s", name)
		}
	}
	for schemaName, fields := range map[string][]string{
		"ProjectListResponse": {"projects", "total", "limit", "offset"},
		"TaskListResponse":    {"tasks", "total", "limit", "offset"},
		"ProjectOutput":       {"tasks_count", "task_counts", "agents"},
		"TaskCounts":          {"total", "open", "in_progress", "blocked", "done", "ready"},
		"TaskOutput":          {"agents"},
		"TaskShowResponse":    {"dependencies"},
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
	res := doJSON(t, handler, http.MethodGet, "/api/projects?limit=2&offset=1", nil)
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/projects?limit=2&offset=1 status = %d", res.StatusCode)
	}
	var projects ProjectListResponse
	decodeJSON(t, res, &projects)

	if projects.Total != 4 || projects.Limit != 2 || projects.Offset != 1 {
		t.Fatalf("unexpected pagination metadata: %+v", projects)
	}
	if got := projectNames(projects.Projects); !slices.Equal(got, []string{"bravo", "charlie"}) {
		t.Fatalf("unexpected paged projects: %v", got)
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
	res := doJSON(t, handler, http.MethodGet, "/api/tasks?limit=2&offset=1", nil)
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/tasks?limit=2&offset=1 status = %d", res.StatusCode)
	}
	var tasks TaskListResponse
	decodeJSON(t, res, &tasks)

	if tasks.Total != 5 || tasks.Limit != 2 || tasks.Offset != 1 {
		t.Fatalf("unexpected pagination metadata: %+v", tasks)
	}
	if got := taskTitles(tasks.Tasks); !slices.Equal(got, []string{"fourth", "third"}) {
		t.Fatalf("unexpected paged tasks: %v", got)
	}

	filteredRes := doJSON(t, handler, http.MethodGet, "/api/tasks?projectId="+jsonNumber(project.ID)+"&status=open&limit=10&offset=0", nil)
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
	if _, err := store.CompleteTaskByActor(ctx, doneTask.ID, "Done.", storage.ActorRefFromActor(agent.Agent)); err != nil {
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
	if len(tasks.Tasks) != 5 {
		t.Fatalf("unexpected tasks response: %+v", tasks)
	}
	assertSingleAgent(t, tasks.Tasks[0].Agents, agent.Agent.ID, "Codex Backend")

	showRes := doJSON(t, handler, http.MethodGet, "/api/tasks/"+jsonNumber(task.ID), nil)
	defer showRes.Body.Close()
	if showRes.StatusCode != http.StatusOK {
		t.Fatalf("show status = %d", showRes.StatusCode)
	}
	var shown TaskShowResponse
	decodeJSON(t, showRes, &shown)
	assertSingleAgent(t, shown.Task.Agents, agent.Agent.ID, "Codex Backend")
	if len(shown.Dependencies) != 1 || shown.Dependencies[0].Role != "blocks" {
		t.Fatalf("unexpected task dependencies: %+v", shown.Dependencies)
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
