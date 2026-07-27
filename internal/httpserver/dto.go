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

type UpdateProjectInput struct {
	Name        string `json:"name,omitempty"`
	DisplayName string `json:"display_name,omitempty"`
	Path        string `json:"path,omitempty"`
}

type CreateAgentInput struct {
	Name string `json:"name" validate:"required"`
}

type UpdateAgentInput struct {
	Name string `json:"name,omitempty"`
}

type CreateTaskInput struct {
	Title              string `json:"title" validate:"required"`
	Description        string `json:"description,omitempty"`
	AcceptanceCriteria string `json:"acceptance_criteria,omitempty"`
	Notes              string `json:"notes,omitempty"`
}

type ProjectListResponse struct {
	Projects   []ProjectOutput `json:"projects"`
	Total      int             `json:"total"`
	Limit      int             `json:"limit"`
	NextCursor string          `json:"next_cursor,omitempty"`
}

type ProjectResponse struct {
	Project ProjectOutput `json:"project"`
}

type ProjectInstructionListResponse struct {
	Instructions []ProjectInstructionOutput `json:"instructions"`
}

type ProjectInstructionResponse struct {
	Instruction ProjectInstructionOutput `json:"instruction"`
}

type ProjectInstructionInput struct {
	Title    string `json:"title,omitempty"`
	Body     string `json:"body,omitempty"`
	Priority string `json:"priority,omitempty"`
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

type ProjectInstructionOutput struct {
	ID        int64  `json:"id"`
	ProjectID int64  `json:"project_id"`
	Scope     string `json:"scope"`
	Title     string `json:"title"`
	Body      string `json:"body"`
	Priority  string `json:"priority"`
	Enabled   bool   `json:"enabled"`
	Source    string `json:"source"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
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
	Tasks      []TaskOutput `json:"tasks"`
	Total      int          `json:"total"`
	Limit      int          `json:"limit"`
	NextCursor string       `json:"next_cursor,omitempty"`
}

type TaskResponse struct {
	Task TaskOutput `json:"task"`
}

type TaskShowResponse struct {
	Task         TaskOutput             `json:"task"`
	Events       []TaskEventOutput      `json:"events"`
	Dependencies []TaskDependencyOutput `json:"dependencies"`
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
	Note             string `json:"note" validate:"required"`
	EvidenceRunID    int64  `json:"evidence_run_id,omitempty"`
	AllowUnvalidated bool   `json:"allow_unvalidated,omitempty"`
	OverrideReason   string `json:"override_reason,omitempty"`
}

type TaskOutput struct {
	ID                 int64         `json:"id"`
	ProjectID          int64         `json:"project_id"`
	Project            TaskProject   `json:"project"`
	Status             string        `json:"status"`
	Title              string        `json:"title"`
	Description        string        `json:"description"`
	AcceptanceCriteria string        `json:"acceptance_criteria"`
	Notes              string        `json:"notes"`
	Agents             []ActorOutput `json:"agents"`
	CreatedAt          string        `json:"created_at"`
	UpdatedAt          string        `json:"updated_at"`
}

type TaskProject struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	DisplayName string `json:"display_name"`
}

type TaskEventResponse struct {
	Event TaskEventOutput `json:"event"`
}

type TaskEventOutput struct {
	ID                 int64        `json:"id"`
	TaskID             int64        `json:"task_id"`
	Type               string       `json:"type"`
	Body               string       `json:"body"`
	FromStatus         string       `json:"from_status"`
	ToStatus           string       `json:"to_status"`
	EvidenceRunID      int64        `json:"evidence_run_id,omitempty"`
	EvidenceArtifactID int64        `json:"evidence_artifact_id,omitempty"`
	Actor              *ActorOutput `json:"actor,omitempty"`
	CreatedAt          string       `json:"created_at"`
}

type TaskDependencyOutput struct {
	ID            int64  `json:"id"`
	EdgeType      string `json:"edge_type"`
	BlockerTaskID int64  `json:"blocker_task_id"`
	BlockedTaskID int64  `json:"blocked_task_id"`
	Role          string `json:"role"`
	CreatedAt     string `json:"created_at"`
}

type ActorOutput struct {
	ID   int64  `json:"id"`
	Kind string `json:"kind"`
	Name string `json:"name"`
}

type AgentListResponse struct {
	Agents []AgentOutput `json:"agents"`
}

type AgentResponse struct {
	Agent AgentOutput `json:"agent"`
}

type CreateAgentResponse struct {
	Agent AgentOutput `json:"agent"`
	Token string      `json:"token"`
}

type AgentOutput struct {
	ID             int64                `json:"id"`
	Kind           string               `json:"kind"`
	Name           string               `json:"name"`
	Projects       []AgentProjectOutput `json:"projects"`
	TasksCount     int                  `json:"tasks_count"`
	EventsCount    int                  `json:"events_count"`
	LastActivityAt string               `json:"last_activity_at"`
	CreatedAt      string               `json:"created_at"`
	UpdatedAt      string               `json:"updated_at"`
}

type AgentProjectOutput struct {
	ID             int64  `json:"id"`
	Name           string `json:"name"`
	DisplayName    string `json:"display_name"`
	TasksCount     int    `json:"tasks_count"`
	EventsCount    int    `json:"events_count"`
	LastActivityAt string `json:"last_activity_at"`
}

type IndexResponse struct {
	ProjectName      string         `json:"project_name"`
	State            string         `json:"state"`
	PathExists       bool           `json:"path_exists"`
	IndexedDocuments int            `json:"indexed_documents"`
	IndexedChunks    int            `json:"indexed_chunks"`
	SkippedFiles     int            `json:"skipped_files"`
	SkippedReasons   map[string]int `json:"skipped_reasons"`
	UpdatedAt        string         `json:"updated_at"`
	LastError        string         `json:"last_error,omitempty"`
}

type IndexListResponse struct {
	Indexes []IndexResponse `json:"indexes"`
	Total   int             `json:"total"`
}

type IndexIgnorePolicyResponse struct {
	ProjectName      string   `json:"project_name"`
	IncludePatterns  []string `json:"include_patterns"`
	IgnorePatterns   []string `json:"ignore_patterns"`
	CreatedAt        string   `json:"created_at"`
	UpdatedAt        string   `json:"updated_at"`
	SeededFromIgnore bool     `json:"seeded_from_gitignore"`
}

type IndexIgnorePatternInput struct {
	Pattern string `json:"pattern"`
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

func projectInstructionsFromStorage(instructions []storage.ProjectInstruction) ProjectInstructionListResponse {
	out := ProjectInstructionListResponse{Instructions: make([]ProjectInstructionOutput, 0, len(instructions))}
	for _, instruction := range instructions {
		out.Instructions = append(out.Instructions, projectInstructionFromStorage(instruction))
	}
	return out
}

func projectInstructionFromStorage(instruction storage.ProjectInstruction) ProjectInstructionOutput {
	return ProjectInstructionOutput{
		ID:        instruction.ID,
		ProjectID: instruction.ProjectID,
		Scope:     instruction.Scope,
		Title:     instruction.Title,
		Body:      instruction.Body,
		Priority:  instruction.Priority,
		Enabled:   instruction.Enabled,
		Source:    instruction.Source,
		CreatedAt: instruction.CreatedAt,
		UpdatedAt: instruction.UpdatedAt,
	}
}

func tasksFromStorage(tasks []storage.Task, agents map[int64][]ActorOutput, projects map[int64]storage.Project) TaskListResponse {
	out := TaskListResponse{Tasks: make([]TaskOutput, 0, len(tasks))}
	for _, task := range tasks {
		out.Tasks = append(out.Tasks, taskFromStorage(task, projects[task.ProjectID], agents[task.ID]))
	}
	return out
}

func taskShowFromStorage(task storage.Task, project storage.Project, events []storage.TaskEvent, dependencies []storage.TaskDependency) TaskShowResponse {
	out := TaskShowResponse{
		Task:         taskFromStorage(task, project, agentsFromEvents(events)),
		Events:       make([]TaskEventOutput, 0, len(events)),
		Dependencies: make([]TaskDependencyOutput, 0, len(dependencies)),
	}
	for _, event := range events {
		out.Events = append(out.Events, taskEventFromStorage(event))
	}
	for _, dependency := range dependencies {
		out.Dependencies = append(out.Dependencies, taskDependencyFromStorage(task.ID, dependency))
	}
	return out
}

func taskFromStorage(task storage.Task, project storage.Project, agents []ActorOutput) TaskOutput {
	agents = nonNilActors(agents)
	return TaskOutput{
		ID:                 task.ID,
		ProjectID:          task.ProjectID,
		Project:            taskProjectFromStorage(project),
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

func taskProjectFromStorage(project storage.Project) TaskProject {
	displayName := project.DisplayName
	if displayName == "" {
		displayName = project.Name
	}
	return TaskProject{
		ID:          project.ID,
		Name:        project.Name,
		DisplayName: displayName,
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
		ID:                 event.ID,
		TaskID:             event.TaskID,
		Type:               event.Type,
		Body:               event.Body,
		FromStatus:         event.FromStatus,
		ToStatus:           event.ToStatus,
		EvidenceRunID:      event.EvidenceRunID,
		EvidenceArtifactID: event.EvidenceArtifactID,
		Actor:              actorFromSnapshot(event.ActorID, event.ActorKind, event.ActorName),
		CreatedAt:          event.CreatedAt,
	}
}

func taskDependencyFromStorage(taskID int64, dependency storage.TaskDependency) TaskDependencyOutput {
	role := ""
	if dependency.BlockedTaskID == taskID {
		role = "blocked_by"
	}
	if dependency.BlockerTaskID == taskID {
		role = "blocks"
	}
	return TaskDependencyOutput{
		ID:            dependency.ID,
		EdgeType:      dependency.EdgeType,
		BlockerTaskID: dependency.BlockerTaskID,
		BlockedTaskID: dependency.BlockedTaskID,
		Role:          role,
		CreatedAt:     dependency.CreatedAt,
	}
}

func agentListFromStorage(agents []storage.AgentActivity, projects []storage.AgentProjectActivity) AgentListResponse {
	projectsByAgent := make(map[int64][]AgentProjectOutput)
	for _, project := range projects {
		projectsByAgent[project.ActorID] = append(projectsByAgent[project.ActorID], AgentProjectOutput{
			ID:             project.ProjectID,
			Name:           project.ProjectName,
			DisplayName:    project.ProjectDisplay,
			TasksCount:     project.TasksCount,
			EventsCount:    project.EventsCount,
			LastActivityAt: project.LastActivityAt,
		})
	}

	out := AgentListResponse{Agents: make([]AgentOutput, 0, len(agents))}
	for _, agent := range agents {
		agentProjects := projectsByAgent[agent.Actor.ID]
		if agentProjects == nil {
			agentProjects = []AgentProjectOutput{}
		}
		out.Agents = append(out.Agents, AgentOutput{
			ID:             agent.Actor.ID,
			Kind:           agent.Actor.Kind,
			Name:           agent.Actor.Name,
			Projects:       agentProjects,
			TasksCount:     agent.TasksCount,
			EventsCount:    agent.EventsCount,
			LastActivityAt: agent.LastActivityAt,
			CreatedAt:      agent.Actor.CreatedAt,
			UpdatedAt:      agent.Actor.UpdatedAt,
		})
	}
	return out
}

func agentOutputFromStorage(agent storage.AgentActivity, projects []storage.AgentProjectActivity) AgentOutput {
	agentProjects := make([]AgentProjectOutput, 0)
	for _, project := range projects {
		if project.ActorID != agent.Actor.ID {
			continue
		}
		agentProjects = append(agentProjects, AgentProjectOutput{
			ID:             project.ProjectID,
			Name:           project.ProjectName,
			DisplayName:    project.ProjectDisplay,
			TasksCount:     project.TasksCount,
			EventsCount:    project.EventsCount,
			LastActivityAt: project.LastActivityAt,
		})
	}

	return AgentOutput{
		ID:             agent.Actor.ID,
		Kind:           agent.Actor.Kind,
		Name:           agent.Actor.Name,
		Projects:       agentProjects,
		TasksCount:     agent.TasksCount,
		EventsCount:    agent.EventsCount,
		LastActivityAt: agent.LastActivityAt,
		CreatedAt:      agent.Actor.CreatedAt,
		UpdatedAt:      agent.Actor.UpdatedAt,
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
		State:            summary.State,
		PathExists:       summary.PathExists,
		IndexedDocuments: summary.IndexedDocuments,
		IndexedChunks:    summary.IndexedChunks,
		SkippedFiles:     summary.SkippedFiles,
		SkippedReasons:   summary.SkippedReasons,
		UpdatedAt:        summary.UpdatedAt,
		LastError:        summary.LastError,
	}
}

func indexFromStatus(status retrieval.IndexStatus) IndexResponse {
	return IndexResponse{
		ProjectName:      status.ProjectName,
		State:            status.State,
		PathExists:       status.PathExists,
		IndexedDocuments: status.IndexedDocuments,
		IndexedChunks:    status.IndexedChunks,
		SkippedFiles:     status.SkippedFiles,
		SkippedReasons:   status.SkippedReasons,
		UpdatedAt:        status.UpdatedAt,
		LastError:        status.LastError,
	}
}

func indexListFromSummaries(summaries []retrieval.IndexSummary) IndexListResponse {
	out := make([]IndexResponse, 0, len(summaries))
	for _, summary := range summaries {
		out = append(out, indexFromSummary(summary))
	}
	return IndexListResponse{Indexes: out, Total: len(out)}
}

func indexListFromStatuses(statuses []retrieval.IndexStatus) IndexListResponse {
	out := make([]IndexResponse, 0, len(statuses))
	for _, status := range statuses {
		out = append(out, indexFromStatus(status))
	}
	return IndexListResponse{Indexes: out, Total: len(out)}
}

func indexIgnorePolicyFromRetrieval(policy retrieval.IndexPolicy) IndexIgnorePolicyResponse {
	return IndexIgnorePolicyResponse{
		ProjectName:      policy.ProjectName,
		IncludePatterns:  nonNilStrings(policy.IncludePatterns),
		IgnorePatterns:   nonNilStrings(policy.IgnorePatterns),
		CreatedAt:        policy.CreatedAt,
		UpdatedAt:        policy.UpdatedAt,
		SeededFromIgnore: policy.SeededFromIgnore,
	}
}

func nonNilStrings(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}
