package httpserver

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/user"
	"sort"
	"strconv"
	"strings"

	"github.com/go-fuego/fuego"
	"github.com/go-fuego/fuego/option"

	"s26.sh/tok/internal/retrieval"
	"s26.sh/tok/internal/storage"
)

const defaultAddr = "127.0.0.1:7654"

type Config struct {
	Addr    string
	Store   *storage.Store
	Version string
}

type Server struct {
	api    *api
	server *fuego.Server
}

type api struct {
	store     *storage.Store
	retrieval *retrieval.Service
	version   string
}

func New(cfg Config) (*Server, error) {
	if cfg.Store == nil {
		return nil, errors.New("http server store is required")
	}
	addr := strings.TrimSpace(cfg.Addr)
	if addr == "" {
		addr = defaultAddr
	}
	version := strings.TrimSpace(cfg.Version)
	if version == "" {
		version = "dev"
	}

	s := fuego.NewServer(
		fuego.WithAddr(addr),
		fuego.WithoutLogger(),
		fuego.WithEngineOptions(fuego.WithOpenAPIConfig(fuego.OpenAPIConfig{
			DisableLocalSave: true,
			PrettyFormatJSON: true,
		})),
	)
	a := &api{
		store:     cfg.Store,
		retrieval: retrieval.NewService(cfg.Store),
		version:   version,
	}
	registerRoutes(s, a)

	return &Server{api: a, server: s}, nil
}

func (s *Server) Run(ctx context.Context) error {
	if s == nil || s.server == nil {
		return errors.New("http server is nil")
	}
	return s.server.RunContext(ctx)
}

func (s *Server) Handler() http.Handler {
	if s == nil || s.server == nil {
		return http.NewServeMux()
	}
	s.server.OutputOpenAPISpec()
	s.server.RegisterOpenAPIRoutes(s.server)
	return s.server.Mux
}

func registerRoutes(s *fuego.Server, a *api) {
	fuego.Get(s, "/api/health", a.health, operation("getHealth", "System", "Show local TOK UI API health")...)

	fuego.Get(s, "/api/projects", a.listProjects, append(operation("listProjects", "Projects", "List registered projects"), option.Query("limit", "Maximum projects to return"), option.Query("offset", "Projects to skip before returning results"))...)
	fuego.Post(s, "/api/projects", a.createProject, append(operation("createProject", "Projects", "Register a project"), jsonBody()...)...)
	fuego.Get(s, "/api/projects/{project}", a.showProject, operation("showProject", "Projects", "Show a project")...)

	fuego.Get(s, "/api/projects/{project}/tasks", a.listTasks, append(operation("listProjectTasks", "Tasks", "List project tasks"), option.Query("status", "Optional task status filter"))...)
	fuego.Post(s, "/api/projects/{project}/tasks", a.createTask, append(operation("createTask", "Tasks", "Create a project task"), jsonBody()...)...)
	fuego.Get(s, "/api/projects/{project}/tasks/ready", a.readyTasks, operation("listReadyTasks", "Tasks", "List ready project tasks")...)
	fuego.Post(s, "/api/projects/{project}/tasks/claim", a.claimTask, append(operation("claimTask", "Tasks", "Claim the next ready task or a specific ready task"), jsonBody()...)...)
	fuego.Get(s, "/api/tasks/{id}", a.showTask, operation("showTask", "Tasks", "Show a task with event history")...)
	fuego.Post(s, "/api/tasks/{id}/comment", a.commentTask, append(operation("commentTask", "Tasks", "Add a task comment"), jsonBody()...)...)
	fuego.Post(s, "/api/tasks/{id}/progress", a.progressTask, append(operation("progressTask", "Tasks", "Add task progress"), jsonBody()...)...)
	fuego.Post(s, "/api/tasks/{id}/block", a.blockTask, append(operation("blockTask", "Tasks", "Block a task"), jsonBody()...)...)
	fuego.Post(s, "/api/tasks/{id}/unblock", a.unblockTask, append(operation("unblockTask", "Tasks", "Unblock a task"), jsonBody()...)...)
	fuego.Post(s, "/api/tasks/{id}/done", a.doneTask, append(operation("completeTask", "Tasks", "Complete a task"), jsonBody()...)...)

	fuego.Get(s, "/api/projects/{project}/index", a.indexStatus, operation("getProjectIndexStatus", "Index", "Show project index status")...)
	fuego.Post(s, "/api/projects/{project}/index/update", a.indexUpdate, operation("updateProjectIndex", "Index", "Update project index")...)
}

func operation(id, tag, summary string) []fuego.RouteOption {
	return []fuego.RouteOption{
		option.OperationID(id),
		option.Tags(tag),
		option.Summary(summary),
		option.OverrideDescription(summary + "."),
	}
}

func jsonBody() []fuego.RouteOption {
	return []fuego.RouteOption{option.RequestContentType("application/json")}
}

func (a *api) health(_ fuego.ContextNoBody) (HealthOutput, error) {
	return HealthOutput{Status: "ok", Version: a.version}, nil
}

func (a *api) listProjects(ctx fuego.ContextNoBody) (ProjectListResponse, error) {
	limit, err := positiveIntQuery(ctx, "limit", 25, 100)
	if err != nil {
		return ProjectListResponse{}, err
	}
	offset, err := nonNegativeIntQuery(ctx, "offset", 0)
	if err != nil {
		return ProjectListResponse{}, err
	}
	total, err := a.store.CountProjects(ctx.Context())
	if err != nil {
		return ProjectListResponse{}, err
	}
	projects, err := a.store.ListProjectsWithOptions(ctx.Context(), storage.ListProjectsOptions{Limit: limit, Offset: offset})
	if err != nil {
		return ProjectListResponse{}, err
	}
	out := ProjectListResponse{Projects: make([]ProjectOutput, 0, len(projects)), Total: total, Limit: limit, Offset: offset}
	for _, project := range projects {
		projectOut, err := a.projectOutput(ctx.Context(), project)
		if err != nil {
			return ProjectListResponse{}, err
		}
		out.Projects = append(out.Projects, projectOut)
	}
	return out, nil
}

func positiveIntQuery(ctx fuego.ContextNoBody, name string, defaultValue, maxValue int) (int, error) {
	value := strings.TrimSpace(ctx.QueryParam(name))
	if value == "" {
		return defaultValue, nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed <= 0 {
		return 0, badRequest(fmt.Sprintf("%s must be a positive integer", name))
	}
	if maxValue > 0 && parsed > maxValue {
		return maxValue, nil
	}
	return parsed, nil
}

func nonNegativeIntQuery(ctx fuego.ContextNoBody, name string, defaultValue int) (int, error) {
	value := strings.TrimSpace(ctx.QueryParam(name))
	if value == "" {
		return defaultValue, nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed < 0 {
		return 0, badRequest(fmt.Sprintf("%s must be a non-negative integer", name))
	}
	return parsed, nil
}

func (a *api) createProject(ctx fuego.ContextWithBody[CreateProjectInput]) (ProjectResponse, error) {
	body, err := ctx.Body()
	if err != nil {
		return ProjectResponse{}, err
	}
	name := strings.TrimSpace(body.Name)
	displayName := strings.TrimSpace(body.DisplayName)
	path := strings.TrimSpace(body.Path)
	if name == "" {
		return ProjectResponse{}, badRequest("project name is required")
	}
	if path == "" {
		return ProjectResponse{}, badRequest("project path is required")
	}
	if displayName == "" {
		displayName = name
	}

	project, err := a.store.CreateProject(ctx.Context(), storage.CreateProjectInput{
		Name:        name,
		DisplayName: displayName,
		Path:        path,
	})
	if err != nil {
		return ProjectResponse{}, mapProjectWriteError(err)
	}
	return ProjectResponse{Project: projectFromStorage(project, TaskCounts{}, nil)}, nil
}

func (a *api) showProject(ctx fuego.ContextNoBody) (ProjectResponse, error) {
	project, err := a.projectByName(ctx.Context(), ctx.PathParam("project"))
	if err != nil {
		return ProjectResponse{}, err
	}
	projectOut, err := a.projectOutput(ctx.Context(), project)
	if err != nil {
		return ProjectResponse{}, err
	}
	return ProjectResponse{Project: projectOut}, nil
}

func (a *api) listTasks(ctx fuego.ContextNoBody) (TaskListResponse, error) {
	project, err := a.projectByName(ctx.Context(), ctx.PathParam("project"))
	if err != nil {
		return TaskListResponse{}, err
	}
	status := strings.TrimSpace(ctx.QueryParam("status"))
	if status != "" && !validTaskStatus(status) {
		return TaskListResponse{}, badRequest(fmt.Sprintf("invalid task status %q", status))
	}
	tasks, err := a.store.ListTasksWithOptions(ctx.Context(), project.ID, storage.ListTasksOptions{Status: status})
	if err != nil {
		return TaskListResponse{}, err
	}
	return a.taskListResponse(ctx.Context(), tasks)
}

func (a *api) createTask(ctx fuego.ContextWithBody[CreateTaskInput]) (TaskResponse, error) {
	project, err := a.projectByName(ctx.Context(), ctx.PathParam("project"))
	if err != nil {
		return TaskResponse{}, err
	}
	body, err := ctx.Body()
	if err != nil {
		return TaskResponse{}, err
	}
	title := strings.TrimSpace(body.Title)
	if title == "" {
		return TaskResponse{}, badRequest("task title is required")
	}
	actor, err := currentLocalHumanActor(ctx.Context(), a.store)
	if err != nil {
		return TaskResponse{}, err
	}
	task, err := a.store.CreateTask(ctx.Context(), storage.CreateTaskInput{
		ProjectID:          project.ID,
		Title:              title,
		Description:        strings.TrimSpace(body.Description),
		AcceptanceCriteria: strings.TrimSpace(body.AcceptanceCriteria),
		Notes:              strings.TrimSpace(body.Notes),
		Actor:              actor,
	})
	if err != nil {
		return TaskResponse{}, mapTaskError(err)
	}
	return a.taskResponse(ctx.Context(), task)
}

func (a *api) readyTasks(ctx fuego.ContextNoBody) (TaskListResponse, error) {
	project, err := a.projectByName(ctx.Context(), ctx.PathParam("project"))
	if err != nil {
		return TaskListResponse{}, err
	}
	tasks, err := a.store.ListReadyTasks(ctx.Context(), project.ID)
	if err != nil {
		return TaskListResponse{}, err
	}
	return a.taskListResponse(ctx.Context(), tasks)
}

func (a *api) showTask(ctx fuego.ContextNoBody) (TaskShowResponse, error) {
	taskID, err := taskIDFromPath(ctx)
	if err != nil {
		return TaskShowResponse{}, err
	}
	task, err := a.taskByID(ctx.Context(), taskID)
	if err != nil {
		return TaskShowResponse{}, err
	}
	events, err := a.store.ListTaskEvents(ctx.Context(), task.ID)
	if err != nil {
		return TaskShowResponse{}, err
	}
	return taskShowFromStorage(task, events), nil
}

func (a *api) claimTask(ctx fuego.ContextWithBody[ClaimTaskInput]) (TaskResponse, error) {
	project, err := a.projectByName(ctx.Context(), ctx.PathParam("project"))
	if err != nil {
		return TaskResponse{}, err
	}
	body, err := ctx.Body()
	if err != nil {
		return TaskResponse{}, err
	}
	actor, err := currentLocalHumanActor(ctx.Context(), a.store)
	if err != nil {
		return TaskResponse{}, err
	}

	var task storage.Task
	if body.ID > 0 {
		task, err = a.store.ClaimTaskByActor(ctx.Context(), project.ID, body.ID, actor)
	} else {
		task, err = a.store.ClaimNextReadyTaskByActor(ctx.Context(), project.ID, actor)
	}
	if err != nil {
		return TaskResponse{}, mapTaskError(err)
	}
	return a.taskResponse(ctx.Context(), task)
}

func (a *api) commentTask(ctx fuego.ContextWithBody[TaskNoteInput]) (TaskEventResponse, error) {
	return a.addTaskNote(ctx, "comment")
}

func (a *api) progressTask(ctx fuego.ContextWithBody[TaskNoteInput]) (TaskEventResponse, error) {
	return a.addTaskNote(ctx, "progress")
}

func (a *api) blockTask(ctx fuego.ContextWithBody[TaskBlockInput]) (TaskResponse, error) {
	taskID, err := taskIDFromPath(ctx)
	if err != nil {
		return TaskResponse{}, err
	}
	body, err := ctx.Body()
	if err != nil {
		return TaskResponse{}, err
	}
	actor, err := currentLocalHumanActor(ctx.Context(), a.store)
	if err != nil {
		return TaskResponse{}, err
	}
	task, err := a.store.BlockTaskByActor(ctx.Context(), taskID, body.Reason, actor)
	if err != nil {
		return TaskResponse{}, mapTaskError(err)
	}
	return a.taskResponse(ctx.Context(), task)
}

func (a *api) unblockTask(ctx fuego.ContextWithBody[TaskUnblockInput]) (TaskResponse, error) {
	taskID, err := taskIDFromPath(ctx)
	if err != nil {
		return TaskResponse{}, err
	}
	body, err := ctx.Body()
	if err != nil {
		return TaskResponse{}, err
	}
	actor, err := currentLocalHumanActor(ctx.Context(), a.store)
	if err != nil {
		return TaskResponse{}, err
	}
	task, err := a.store.UnblockTaskByActor(ctx.Context(), taskID, body.Note, actor)
	if err != nil {
		return TaskResponse{}, mapTaskError(err)
	}
	return a.taskResponse(ctx.Context(), task)
}

func (a *api) doneTask(ctx fuego.ContextWithBody[TaskDoneInput]) (TaskResponse, error) {
	taskID, err := taskIDFromPath(ctx)
	if err != nil {
		return TaskResponse{}, err
	}
	body, err := ctx.Body()
	if err != nil {
		return TaskResponse{}, err
	}
	actor, err := currentLocalHumanActor(ctx.Context(), a.store)
	if err != nil {
		return TaskResponse{}, err
	}
	task, err := a.store.CompleteTaskByActor(ctx.Context(), taskID, body.Note, actor)
	if err != nil {
		return TaskResponse{}, mapTaskError(err)
	}
	return a.taskResponse(ctx.Context(), task)
}

func (a *api) indexStatus(ctx fuego.ContextNoBody) (IndexResponse, error) {
	project, err := a.projectByName(ctx.Context(), ctx.PathParam("project"))
	if err != nil {
		return IndexResponse{}, err
	}
	status, err := a.retrieval.IndexStatus(ctx.Context(), project)
	if err != nil {
		return IndexResponse{}, err
	}
	return indexFromStatus(status), nil
}

func (a *api) indexUpdate(ctx fuego.ContextNoBody) (IndexResponse, error) {
	project, err := a.projectByName(ctx.Context(), ctx.PathParam("project"))
	if err != nil {
		return IndexResponse{}, err
	}
	summary, err := a.retrieval.IndexProject(ctx.Context(), project)
	if err != nil {
		return IndexResponse{}, err
	}
	return indexFromSummary(summary), nil
}

func (a *api) addTaskNote(ctx fuego.ContextWithBody[TaskNoteInput], kind string) (TaskEventResponse, error) {
	taskID, err := taskIDFromPath(ctx)
	if err != nil {
		return TaskEventResponse{}, err
	}
	body, err := ctx.Body()
	if err != nil {
		return TaskEventResponse{}, err
	}
	actor, err := currentLocalHumanActor(ctx.Context(), a.store)
	if err != nil {
		return TaskEventResponse{}, err
	}

	var event storage.TaskEvent
	switch kind {
	case "comment":
		event, err = a.store.AddTaskCommentByActor(ctx.Context(), taskID, body.Body, actor)
	case "progress":
		event, err = a.store.AddTaskProgressByActor(ctx.Context(), taskID, body.Body, actor)
	default:
		err = fmt.Errorf("unknown task note kind %q", kind)
	}
	if err != nil {
		return TaskEventResponse{}, mapTaskError(err)
	}
	return TaskEventResponse{Event: taskEventFromStorage(event)}, nil
}

func (a *api) projectOutput(ctx context.Context, project storage.Project) (ProjectOutput, error) {
	tasks, err := a.store.ListTasks(ctx, project.ID)
	if err != nil {
		return ProjectOutput{}, err
	}
	readyTasks, err := a.store.ListReadyTasks(ctx, project.ID)
	if err != nil {
		return ProjectOutput{}, err
	}
	agentsByTask, err := a.agentsByTask(ctx, tasks)
	if err != nil {
		return ProjectOutput{}, err
	}

	seen := make(map[int64]ActorOutput)
	for _, agents := range agentsByTask {
		for _, agent := range agents {
			seen[agent.ID] = agent
		}
	}
	agents := make([]ActorOutput, 0, len(seen))
	for _, agent := range seen {
		agents = append(agents, agent)
	}
	sortActors(agents)

	return projectFromStorage(project, taskCountsFromStorage(tasks, len(readyTasks)), agents), nil
}

func (a *api) taskResponse(ctx context.Context, task storage.Task) (TaskResponse, error) {
	events, err := a.store.ListTaskEvents(ctx, task.ID)
	if err != nil {
		return TaskResponse{}, err
	}
	return TaskResponse{Task: taskFromStorage(task, agentsFromEvents(events))}, nil
}

func (a *api) taskListResponse(ctx context.Context, tasks []storage.Task) (TaskListResponse, error) {
	agentsByTask, err := a.agentsByTask(ctx, tasks)
	if err != nil {
		return TaskListResponse{}, err
	}
	return tasksFromStorage(tasks, agentsByTask), nil
}

func (a *api) agentsByTask(ctx context.Context, tasks []storage.Task) (map[int64][]ActorOutput, error) {
	out := make(map[int64][]ActorOutput, len(tasks))
	for _, task := range tasks {
		events, err := a.store.ListTaskEvents(ctx, task.ID)
		if err != nil {
			return nil, err
		}
		out[task.ID] = agentsFromEvents(events)
	}
	return out, nil
}

func sortActors(actors []ActorOutput) {
	sort.Slice(actors, func(i, j int) bool {
		if actors[i].Name == actors[j].Name {
			return actors[i].ID < actors[j].ID
		}
		return actors[i].Name < actors[j].Name
	})
}

func (a *api) projectByName(ctx context.Context, name string) (storage.Project, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return storage.Project{}, badRequest("project name is required")
	}
	project, err := a.store.GetProject(ctx, name)
	if errors.Is(err, sql.ErrNoRows) {
		return storage.Project{}, fuego.NotFoundError{Title: "Project not found", Detail: name}
	}
	return project, err
}

func (a *api) taskByID(ctx context.Context, id int64) (storage.Task, error) {
	if id <= 0 {
		return storage.Task{}, badRequest("task id is required")
	}
	task, err := a.store.GetTask(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return storage.Task{}, fuego.NotFoundError{Title: "Task not found", Detail: strconv.FormatInt(id, 10)}
	}
	return task, err
}

func taskIDFromPath(ctx interface{ PathParam(string) string }) (int64, error) {
	raw := strings.TrimSpace(ctx.PathParam("id"))
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 {
		return 0, badRequest(fmt.Sprintf("invalid task id: %s", raw))
	}
	return id, nil
}

func currentLocalHumanActor(ctx context.Context, store *storage.Store) (storage.ActorRef, error) {
	resolved, err := resolveLocalUserDisplayName(ctx, store)
	if err != nil {
		return storage.ActorRef{}, err
	}
	actor, err := store.SetLocalHuman(ctx, resolved)
	if err != nil {
		return storage.ActorRef{}, err
	}
	return storage.ActorRefFromActor(actor), nil
}

func resolveLocalUserDisplayName(ctx context.Context, store *storage.Store) (string, error) {
	actor, err := store.GetLocalHuman(ctx)
	if err == nil && strings.TrimSpace(actor.Name) != "" {
		return actor.Name, nil
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return "", err
	}
	if current, err := user.Current(); err == nil && current != nil {
		if name := strings.TrimSpace(current.Name); name != "" {
			return name, nil
		}
		if username := strings.TrimSpace(current.Username); username != "" {
			return username, nil
		}
	}
	for _, key := range []string{"USER", "USERNAME", "LOGNAME"} {
		if value := strings.TrimSpace(os.Getenv(key)); value != "" {
			return value, nil
		}
	}
	return "local-user", nil
}

func mapProjectWriteError(err error) error {
	if strings.Contains(err.Error(), "UNIQUE") {
		return fuego.ConflictError{Title: "Project already exists", Detail: err.Error()}
	}
	return err
}

func mapTaskError(err error) error {
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return fuego.NotFoundError{Title: "Task not found"}
	case errors.Is(err, storage.ErrNoReadyTask):
		return fuego.NotFoundError{Title: "No ready tasks"}
	case errors.Is(err, storage.ErrTaskNotReady):
		return fuego.ConflictError{Title: "Task is not ready"}
	case errors.Is(err, storage.ErrInvalidTaskTransition):
		return fuego.ConflictError{Title: "Invalid task status transition"}
	case errors.Is(err, storage.ErrTaskCompletionNoteEmpty), errors.Is(err, storage.ErrTaskNoteEmpty):
		return badRequest("task note is required")
	default:
		return err
	}
}

func badRequest(detail string) error {
	return fuego.BadRequestError{Title: "Bad request", Detail: detail}
}

func validTaskStatus(status string) bool {
	switch status {
	case "open", "in_progress", "blocked", "done":
		return true
	default:
		return false
	}
}
