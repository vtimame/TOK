package mcpserver

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"s26.sh/tok/internal/retrieval"
	"s26.sh/tok/internal/storage"
)

const defaultVersion = "dev"

type Config struct {
	Store   *storage.Store
	Actor   storage.ActorRef
	Version string
}

type service struct {
	store     *storage.Store
	actor     storage.ActorRef
	retrieval *retrieval.Service
}

func New(cfg Config) (*mcp.Server, error) {
	if cfg.Store == nil {
		return nil, errors.New("mcp store is required")
	}
	actor := sanitizeActor(cfg.Actor)
	if actor.ID <= 0 {
		return nil, errors.New("mcp actor is required")
	}
	version := strings.TrimSpace(cfg.Version)
	if version == "" {
		version = defaultVersion
	}

	svc := &service{
		store:     cfg.Store,
		actor:     actor,
		retrieval: retrieval.NewService(cfg.Store),
	}
	server := mcp.NewServer(&mcp.Implementation{Name: "tok", Version: version}, nil)
	svc.addTools(server)
	return server, nil
}

func (s *service) addTools(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "project_list",
		Description: "List registered TOK projects.",
	}, s.projectList)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "project_show",
		Description: "Show a registered TOK project by name.",
	}, s.projectShow)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "task_list",
		Description: "List tasks for a project, optionally filtered by status.",
	}, s.taskList)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "task_show",
		Description: "Show a task and its event history.",
	}, s.taskShow)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "task_ready",
		Description: "List ready tasks for a project.",
	}, s.taskReady)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "task_claim",
		Description: "Claim a specific ready task or the next ready task for a project.",
	}, s.taskClaim)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "task_comment",
		Description: "Add a comment event to a task.",
	}, s.taskComment)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "task_progress",
		Description: "Add a progress event to a task.",
	}, s.taskProgress)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "task_block",
		Description: "Block an open task with a reason.",
	}, s.taskBlock)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "task_unblock",
		Description: "Unblock a blocked task.",
	}, s.taskUnblock)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "task_done",
		Description: "Mark an in-progress task done.",
	}, s.taskDone)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "index_update",
		Description: "Update the lexical index for a project.",
	}, s.indexUpdate)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "index_status",
		Description: "Show index status for a project.",
	}, s.indexStatus)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "search",
		Description: "Search indexed project files.",
	}, s.search)
}

type emptyInput struct{}

type projectNameInput struct {
	Project string `json:"project" jsonschema:"project name"`
}

type projectShowInput struct {
	Name string `json:"name" jsonschema:"project name"`
}

type taskIDInput struct {
	ID int64 `json:"id" jsonschema:"task id"`
}

type taskListInput struct {
	Project string `json:"project" jsonschema:"project name"`
	Status  string `json:"status,omitempty" jsonschema:"optional task status: open, in_progress, blocked, or done"`
}

type taskClaimInput struct {
	Project string `json:"project" jsonschema:"project name"`
	ID      int64  `json:"id,omitempty" jsonschema:"optional task id; if omitted, claims the next ready task"`
}

type taskNoteInput struct {
	ID   int64  `json:"id" jsonschema:"task id"`
	Body string `json:"body" jsonschema:"comment or progress body"`
}

type taskBlockInput struct {
	ID     int64  `json:"id" jsonschema:"task id"`
	Reason string `json:"reason" jsonschema:"block reason"`
}

type taskUnblockInput struct {
	ID   int64  `json:"id" jsonschema:"task id"`
	Note string `json:"note" jsonschema:"unblock note"`
}

type taskDoneInput struct {
	ID   int64  `json:"id" jsonschema:"task id"`
	Note string `json:"note" jsonschema:"completion note"`
}

type searchInput struct {
	Project string `json:"project" jsonschema:"project name"`
	Query   string `json:"query" jsonschema:"search query"`
	Limit   int    `json:"limit,omitempty" jsonschema:"optional positive result limit"`
}

type projectListOutput struct {
	Projects []ProjectOutput `json:"projects"`
}

type projectOutput struct {
	Project ProjectOutput `json:"project"`
}

type taskListOutput struct {
	Tasks []TaskOutput `json:"tasks"`
}

type taskOutput struct {
	Task TaskOutput `json:"task"`
}

type taskShowOutput struct {
	Task   TaskOutput        `json:"task"`
	Events []TaskEventOutput `json:"events"`
}

type taskEventOutput struct {
	Event TaskEventOutput `json:"event"`
}

type indexOutput struct {
	ProjectName      string         `json:"project_name"`
	IndexedDocuments int            `json:"indexed_documents"`
	SkippedFiles     int            `json:"skipped_files"`
	SkippedReasons   map[string]int `json:"skipped_reasons"`
	UpdatedAt        string         `json:"updated_at"`
}

type searchOutput struct {
	Results []SearchResultOutput `json:"results"`
}

type ProjectOutput struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	DisplayName string `json:"display_name"`
	Path        string `json:"path"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

type TaskOutput struct {
	ID                 int64  `json:"id"`
	ProjectID          int64  `json:"project_id"`
	Status             string `json:"status"`
	Title              string `json:"title"`
	Description        string `json:"description"`
	AcceptanceCriteria string `json:"acceptance_criteria"`
	Notes              string `json:"notes"`
	CreatedAt          string `json:"created_at"`
	UpdatedAt          string `json:"updated_at"`
}

type ActorOutput struct {
	ID   int64  `json:"id"`
	Kind string `json:"kind"`
	Name string `json:"name"`
}

type TaskEventOutput struct {
	ID         int64        `json:"id"`
	TaskID     int64        `json:"task_id"`
	Type       string       `json:"type"`
	Body       string       `json:"body"`
	FromStatus string       `json:"from_status"`
	ToStatus   string       `json:"to_status"`
	Actor      *ActorOutput `json:"actor,omitempty"`
	CreatedAt  string       `json:"created_at"`
}

type SearchResultOutput struct {
	Path       string  `json:"path"`
	Score      float64 `json:"score"`
	Line       int     `json:"line"`
	Snippet    string  `json:"snippet"`
	Excerpt    string  `json:"excerpt"`
	Provenance string  `json:"provenance"`
}

func (s *service) projectList(ctx context.Context, _ *mcp.CallToolRequest, _ emptyInput) (*mcp.CallToolResult, projectListOutput, error) {
	projects, err := s.store.ListProjects(ctx)
	if err != nil {
		return nil, projectListOutput{}, err
	}
	out := projectListOutput{Projects: make([]ProjectOutput, 0, len(projects))}
	for _, project := range projects {
		out.Projects = append(out.Projects, projectFromStorage(project))
	}
	return nil, out, nil
}

func (s *service) projectShow(ctx context.Context, _ *mcp.CallToolRequest, input projectShowInput) (*mcp.CallToolResult, projectOutput, error) {
	project, err := s.projectByName(ctx, input.Name)
	if err != nil {
		return nil, projectOutput{}, err
	}
	return nil, projectOutput{Project: projectFromStorage(project)}, nil
}

func (s *service) taskList(ctx context.Context, _ *mcp.CallToolRequest, input taskListInput) (*mcp.CallToolResult, taskListOutput, error) {
	project, err := s.projectByName(ctx, input.Project)
	if err != nil {
		return nil, taskListOutput{}, err
	}
	status := strings.TrimSpace(input.Status)
	if status != "" && !validTaskStatus(status) {
		return nil, taskListOutput{}, fmt.Errorf("invalid task status %q", status)
	}
	tasks, err := s.store.ListTasksWithOptions(ctx, project.ID, storage.ListTasksOptions{Status: status})
	if err != nil {
		return nil, taskListOutput{}, err
	}
	return nil, taskListFromStorage(tasks), nil
}

func (s *service) taskShow(ctx context.Context, _ *mcp.CallToolRequest, input taskIDInput) (*mcp.CallToolResult, taskShowOutput, error) {
	task, err := s.taskByID(ctx, input.ID)
	if err != nil {
		return nil, taskShowOutput{}, err
	}
	events, err := s.store.ListTaskEvents(ctx, task.ID)
	if err != nil {
		return nil, taskShowOutput{}, err
	}
	out := taskShowOutput{
		Task:   taskFromStorage(task),
		Events: make([]TaskEventOutput, 0, len(events)),
	}
	for _, event := range events {
		out.Events = append(out.Events, taskEventFromStorage(event))
	}
	return nil, out, nil
}

func (s *service) taskReady(ctx context.Context, _ *mcp.CallToolRequest, input projectNameInput) (*mcp.CallToolResult, taskListOutput, error) {
	project, err := s.projectByName(ctx, input.Project)
	if err != nil {
		return nil, taskListOutput{}, err
	}
	tasks, err := s.store.ListReadyTasks(ctx, project.ID)
	if err != nil {
		return nil, taskListOutput{}, err
	}
	return nil, taskListFromStorage(tasks), nil
}

func (s *service) taskClaim(ctx context.Context, _ *mcp.CallToolRequest, input taskClaimInput) (*mcp.CallToolResult, taskOutput, error) {
	project, err := s.projectByName(ctx, input.Project)
	if err != nil {
		return nil, taskOutput{}, err
	}
	var task storage.Task
	if input.ID > 0 {
		task, err = s.store.ClaimTaskByActor(ctx, project.ID, input.ID, s.actor)
	} else {
		task, err = s.store.ClaimNextReadyTaskByActor(ctx, project.ID, s.actor)
	}
	if err != nil {
		return nil, taskOutput{}, friendlyTaskError(err)
	}
	return nil, taskOutput{Task: taskFromStorage(task)}, nil
}

func (s *service) taskComment(ctx context.Context, _ *mcp.CallToolRequest, input taskNoteInput) (*mcp.CallToolResult, taskEventOutput, error) {
	event, err := s.addTaskNote(ctx, input.ID, input.Body, "comment")
	if err != nil {
		return nil, taskEventOutput{}, err
	}
	return nil, taskEventOutput{Event: taskEventFromStorage(event)}, nil
}

func (s *service) taskProgress(ctx context.Context, _ *mcp.CallToolRequest, input taskNoteInput) (*mcp.CallToolResult, taskEventOutput, error) {
	event, err := s.addTaskNote(ctx, input.ID, input.Body, "progress")
	if err != nil {
		return nil, taskEventOutput{}, err
	}
	return nil, taskEventOutput{Event: taskEventFromStorage(event)}, nil
}

func (s *service) taskBlock(ctx context.Context, _ *mcp.CallToolRequest, input taskBlockInput) (*mcp.CallToolResult, taskOutput, error) {
	if strings.TrimSpace(input.Reason) == "" {
		return nil, taskOutput{}, errors.New("task_block requires reason")
	}
	task, err := s.store.BlockTaskByActor(ctx, input.ID, input.Reason, s.actor)
	if err != nil {
		return nil, taskOutput{}, friendlyTaskError(err)
	}
	return nil, taskOutput{Task: taskFromStorage(task)}, nil
}

func (s *service) taskUnblock(ctx context.Context, _ *mcp.CallToolRequest, input taskUnblockInput) (*mcp.CallToolResult, taskOutput, error) {
	if strings.TrimSpace(input.Note) == "" {
		return nil, taskOutput{}, errors.New("task_unblock requires note")
	}
	task, err := s.store.UnblockTaskByActor(ctx, input.ID, input.Note, s.actor)
	if err != nil {
		return nil, taskOutput{}, friendlyTaskError(err)
	}
	return nil, taskOutput{Task: taskFromStorage(task)}, nil
}

func (s *service) taskDone(ctx context.Context, _ *mcp.CallToolRequest, input taskDoneInput) (*mcp.CallToolResult, taskOutput, error) {
	if strings.TrimSpace(input.Note) == "" {
		return nil, taskOutput{}, errors.New("task_done requires note")
	}
	task, err := s.store.CompleteTaskByActor(ctx, input.ID, input.Note, s.actor)
	if err != nil {
		return nil, taskOutput{}, friendlyTaskError(err)
	}
	return nil, taskOutput{Task: taskFromStorage(task)}, nil
}

func (s *service) indexUpdate(ctx context.Context, _ *mcp.CallToolRequest, input projectNameInput) (*mcp.CallToolResult, indexOutput, error) {
	project, err := s.projectByName(ctx, input.Project)
	if err != nil {
		return nil, indexOutput{}, err
	}
	summary, err := s.retrieval.IndexProject(ctx, project)
	if err != nil {
		return nil, indexOutput{}, err
	}
	return nil, indexFromSummary(summary), nil
}

func (s *service) indexStatus(ctx context.Context, _ *mcp.CallToolRequest, input projectNameInput) (*mcp.CallToolResult, indexOutput, error) {
	project, err := s.projectByName(ctx, input.Project)
	if err != nil {
		return nil, indexOutput{}, err
	}
	status, err := s.retrieval.IndexStatus(ctx, project)
	if err != nil {
		return nil, indexOutput{}, err
	}
	return nil, indexFromStatus(status), nil
}

func (s *service) search(ctx context.Context, _ *mcp.CallToolRequest, input searchInput) (*mcp.CallToolResult, searchOutput, error) {
	project, err := s.projectByName(ctx, input.Project)
	if err != nil {
		return nil, searchOutput{}, err
	}
	query := strings.TrimSpace(input.Query)
	if query == "" {
		return nil, searchOutput{}, errors.New("search requires query")
	}
	limit := input.Limit
	if limit == 0 {
		limit = retrieval.DefaultLimit
	}
	if limit < 0 {
		return nil, searchOutput{}, fmt.Errorf("invalid search limit: %d", input.Limit)
	}
	results, err := s.retrieval.Search(ctx, project, query, limit)
	if err != nil {
		return nil, searchOutput{}, err
	}
	out := searchOutput{Results: make([]SearchResultOutput, 0, len(results))}
	for _, result := range results {
		out.Results = append(out.Results, SearchResultOutput{
			Path:       result.Path,
			Score:      result.Score,
			Line:       result.Line,
			Snippet:    result.Snippet,
			Excerpt:    result.Excerpt,
			Provenance: result.Provenance,
		})
	}
	return nil, out, nil
}

func (s *service) projectByName(ctx context.Context, name string) (storage.Project, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return storage.Project{}, errors.New("project name is required")
	}
	project, err := s.store.GetProject(ctx, name)
	if errors.Is(err, sql.ErrNoRows) {
		return storage.Project{}, fmt.Errorf("project not found: %s", name)
	}
	return project, err
}

func (s *service) taskByID(ctx context.Context, id int64) (storage.Task, error) {
	if id <= 0 {
		return storage.Task{}, errors.New("task id is required")
	}
	task, err := s.store.GetTask(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return storage.Task{}, fmt.Errorf("task not found: %d", id)
	}
	return task, err
}

func (s *service) addTaskNote(ctx context.Context, id int64, body, kind string) (storage.TaskEvent, error) {
	if id <= 0 {
		return storage.TaskEvent{}, errors.New("task id is required")
	}
	body = strings.TrimSpace(body)
	if body == "" {
		return storage.TaskEvent{}, fmt.Errorf("task_%s requires body", kind)
	}
	var (
		event storage.TaskEvent
		err   error
	)
	switch kind {
	case "comment":
		event, err = s.store.AddTaskCommentByActor(ctx, id, body, s.actor)
	case "progress":
		event, err = s.store.AddTaskProgressByActor(ctx, id, body, s.actor)
	default:
		err = fmt.Errorf("unknown task note kind %q", kind)
	}
	if errors.Is(err, sql.ErrNoRows) {
		return storage.TaskEvent{}, fmt.Errorf("task not found: %d", id)
	}
	return event, err
}

func taskListFromStorage(tasks []storage.Task) taskListOutput {
	out := taskListOutput{Tasks: make([]TaskOutput, 0, len(tasks))}
	for _, task := range tasks {
		out.Tasks = append(out.Tasks, taskFromStorage(task))
	}
	return out
}

func projectFromStorage(project storage.Project) ProjectOutput {
	return ProjectOutput{
		ID:          project.ID,
		Name:        project.Name,
		DisplayName: project.DisplayName,
		Path:        project.Path,
		CreatedAt:   project.CreatedAt,
		UpdatedAt:   project.UpdatedAt,
	}
}

func taskFromStorage(task storage.Task) TaskOutput {
	return TaskOutput{
		ID:                 task.ID,
		ProjectID:          task.ProjectID,
		Status:             task.Status,
		Title:              task.Title,
		Description:        task.Description,
		AcceptanceCriteria: task.AcceptanceCriteria,
		Notes:              task.Notes,
		CreatedAt:          task.CreatedAt,
		UpdatedAt:          task.UpdatedAt,
	}
}

func taskEventFromStorage(event storage.TaskEvent) TaskEventOutput {
	return TaskEventOutput{
		ID:         event.ID,
		TaskID:     event.TaskID,
		Type:       event.Type,
		Body:       event.Body,
		FromStatus: event.FromStatus,
		ToStatus:   event.ToStatus,
		Actor:      actorFromSnapshot(event.ActorID, event.ActorKind, event.ActorName),
		CreatedAt:  event.CreatedAt,
	}
}

func actorFromSnapshot(id int64, kind, name string) *ActorOutput {
	if id <= 0 || kind == "" || name == "" {
		return nil
	}
	return &ActorOutput{ID: id, Kind: kind, Name: name}
}

func indexFromSummary(summary retrieval.IndexSummary) indexOutput {
	return indexOutput{
		ProjectName:      summary.ProjectName,
		IndexedDocuments: summary.IndexedDocuments,
		SkippedFiles:     summary.SkippedFiles,
		SkippedReasons:   summary.SkippedReasons,
		UpdatedAt:        summary.UpdatedAt,
	}
}

func indexFromStatus(status retrieval.IndexStatus) indexOutput {
	return indexOutput{
		ProjectName:      status.ProjectName,
		IndexedDocuments: status.IndexedDocuments,
		SkippedFiles:     status.SkippedFiles,
		SkippedReasons:   status.SkippedReasons,
		UpdatedAt:        status.UpdatedAt,
	}
}

func friendlyTaskError(err error) error {
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return errors.New("task not found")
	case errors.Is(err, storage.ErrNoReadyTask):
		return errors.New("no ready tasks")
	case errors.Is(err, storage.ErrTaskNotReady):
		return errors.New("task is not ready")
	case errors.Is(err, storage.ErrInvalidTaskTransition):
		return errors.New("invalid task status transition")
	default:
		return err
	}
}

func sanitizeActor(actor storage.ActorRef) storage.ActorRef {
	actor.Kind = strings.TrimSpace(actor.Kind)
	actor.Name = strings.TrimSpace(actor.Name)
	if actor.ID <= 0 || actor.Kind == "" || actor.Name == "" {
		return storage.ActorRef{}
	}
	return actor
}

func validTaskStatus(status string) bool {
	switch status {
	case "open", "in_progress", "blocked", "done":
		return true
	default:
		return false
	}
}
