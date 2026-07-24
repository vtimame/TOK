package httpserver

import (
	"sort"
	"strconv"

	"s26.sh/tok/internal/retrieval"
	"s26.sh/tok/internal/storage"
)

type HealthOutput struct {
	Status  string `json:"status"`
	Version string `json:"version"`
}

type CreateProjectInput struct {
	Name        string `json:"name" validate:"required"`
	DisplayName string `json:"display_name,omitempty"`
	Path        string `json:"path" validate:"required"`
}

type CreateTaskInput struct {
	Title              string `json:"title" validate:"required"`
	Description        string `json:"description,omitempty"`
	AcceptanceCriteria string `json:"acceptance_criteria,omitempty"`
	Notes              string `json:"notes,omitempty"`
}

type ProjectListResponse struct {
	Projects []ProjectOutput `json:"projects"`
	Total    int             `json:"total"`
	Limit    int             `json:"limit"`
	Offset   int             `json:"offset"`
}

type ProjectResponse struct {
	Project ProjectOutput `json:"project"`
}

type ProjectOutput struct {
	ID          int64         `json:"id"`
	Name        string        `json:"name"`
	DisplayName string        `json:"display_name"`
	Path        string        `json:"path"`
	TasksCount  int           `json:"tasks_count"`
	TaskCounts  TaskCounts    `json:"task_counts"`
	Agents      []ActorOutput `json:"agents"`
	CreatedAt   string        `json:"created_at"`
	UpdatedAt   string        `json:"updated_at"`
}

type TaskCounts struct {
	Total      int `json:"total"`
	Open       int `json:"open"`
	InProgress int `json:"in_progress"`
	Blocked    int `json:"blocked"`
	Done       int `json:"done"`
	Ready      int `json:"ready"`
}

type TaskListResponse struct {
	Tasks []TaskOutput `json:"tasks"`
}

type TaskResponse struct {
	Task TaskOutput `json:"task"`
}

type TaskShowResponse struct {
	Task   TaskOutput        `json:"task"`
	Events []TaskEventOutput `json:"events"`
}

type ClaimTaskInput struct {
	ID int64 `json:"id,omitempty"`
}

type TaskNoteInput struct {
	Body string `json:"body" validate:"required"`
}

type TaskBlockInput struct {
	Reason string `json:"reason" validate:"required"`
}

type TaskUnblockInput struct {
	Note string `json:"note" validate:"required"`
}

type TaskDoneInput struct {
	Note string `json:"note" validate:"required"`
}

type TaskOutput struct {
	ID                 int64         `json:"id"`
	ProjectID          int64         `json:"project_id"`
	Status             string        `json:"status"`
	Title              string        `json:"title"`
	Description        string        `json:"description"`
	AcceptanceCriteria string        `json:"acceptance_criteria"`
	Notes              string        `json:"notes"`
	Agents             []ActorOutput `json:"agents"`
	CreatedAt          string        `json:"created_at"`
	UpdatedAt          string        `json:"updated_at"`
}

type TaskEventResponse struct {
	Event TaskEventOutput `json:"event"`
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

type ActorOutput struct {
	ID   int64  `json:"id"`
	Kind string `json:"kind"`
	Name string `json:"name"`
}

type IndexResponse struct {
	ProjectName      string         `json:"project_name"`
	IndexedDocuments int            `json:"indexed_documents"`
	SkippedFiles     int            `json:"skipped_files"`
	SkippedReasons   map[string]int `json:"skipped_reasons"`
	UpdatedAt        string         `json:"updated_at"`
}

func projectFromStorage(project storage.Project, taskCounts TaskCounts, agents []ActorOutput) ProjectOutput {
	agents = nonNilActors(agents)
	return ProjectOutput{
		ID:          project.ID,
		Name:        project.Name,
		DisplayName: project.DisplayName,
		Path:        project.Path,
		TasksCount:  taskCounts.Total,
		TaskCounts:  taskCounts,
		Agents:      agents,
		CreatedAt:   project.CreatedAt,
		UpdatedAt:   project.UpdatedAt,
	}
}

func tasksFromStorage(tasks []storage.Task, agents map[int64][]ActorOutput) TaskListResponse {
	out := TaskListResponse{Tasks: make([]TaskOutput, 0, len(tasks))}
	for _, task := range tasks {
		out.Tasks = append(out.Tasks, taskFromStorage(task, agents[task.ID]))
	}
	return out
}

func taskShowFromStorage(task storage.Task, events []storage.TaskEvent) TaskShowResponse {
	out := TaskShowResponse{
		Task:   taskFromStorage(task, agentsFromEvents(events)),
		Events: make([]TaskEventOutput, 0, len(events)),
	}
	for _, event := range events {
		out.Events = append(out.Events, taskEventFromStorage(event))
	}
	return out
}

func taskFromStorage(task storage.Task, agents []ActorOutput) TaskOutput {
	agents = nonNilActors(agents)
	return TaskOutput{
		ID:                 task.ID,
		ProjectID:          task.ProjectID,
		Status:             task.Status,
		Title:              task.Title,
		Description:        task.Description,
		AcceptanceCriteria: task.AcceptanceCriteria,
		Notes:              task.Notes,
		Agents:             agents,
		CreatedAt:          task.CreatedAt,
		UpdatedAt:          task.UpdatedAt,
	}
}

func nonNilActors(actors []ActorOutput) []ActorOutput {
	if actors == nil {
		return []ActorOutput{}
	}
	return actors
}

func taskCountsFromStorage(tasks []storage.Task, ready int) TaskCounts {
	counts := TaskCounts{Total: len(tasks), Ready: ready}
	for _, task := range tasks {
		switch task.Status {
		case "open":
			counts.Open++
		case "in_progress":
			counts.InProgress++
		case "blocked":
			counts.Blocked++
		case "done":
			counts.Done++
		}
	}
	return counts
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

func agentsFromEvents(events []storage.TaskEvent) []ActorOutput {
	seen := make(map[string]ActorOutput)
	for _, event := range events {
		actor := actorFromSnapshot(event.ActorID, event.ActorKind, event.ActorName)
		if actor == nil || actor.Kind != "agent" {
			continue
		}
		key := actor.Kind + ":" + strconv.FormatInt(actor.ID, 10)
		seen[key] = *actor
	}
	out := make([]ActorOutput, 0, len(seen))
	for _, actor := range seen {
		out = append(out, actor)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Name == out[j].Name {
			return out[i].ID < out[j].ID
		}
		return out[i].Name < out[j].Name
	})
	return out
}

func indexFromSummary(summary retrieval.IndexSummary) IndexResponse {
	return IndexResponse{
		ProjectName:      summary.ProjectName,
		IndexedDocuments: summary.IndexedDocuments,
		SkippedFiles:     summary.SkippedFiles,
		SkippedReasons:   summary.SkippedReasons,
		UpdatedAt:        summary.UpdatedAt,
	}
}

func indexFromStatus(status retrieval.IndexStatus) IndexResponse {
	return IndexResponse{
		ProjectName:      status.ProjectName,
		IndexedDocuments: status.IndexedDocuments,
		SkippedFiles:     status.SkippedFiles,
		SkippedReasons:   status.SkippedReasons,
		UpdatedAt:        status.UpdatedAt,
	}
}
