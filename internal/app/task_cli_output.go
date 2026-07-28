package app

import (
	"encoding/json"
	"fmt"
	"io"

	"s26.sh/tok/internal/storage"
)

func printTask(out io.Writer, task storage.Task) {
	fmt.Fprintf(out, "id: %d\n", task.ID)
	fmt.Fprintf(out, "project_id: %d\n", task.ProjectID)
	fmt.Fprintf(out, "status: %s\n", task.Status)
	fmt.Fprintf(out, "title: %s\n", task.Title)
	fmt.Fprintf(out, "description: %s\n", task.Description)
	fmt.Fprintf(out, "acceptance_criteria: %s\n", task.AcceptanceCriteria)
	fmt.Fprintf(out, "notes: %s\n", task.Notes)
	fmt.Fprintf(out, "source: %s\n", task.Source)
	if task.ExternalID != "" {
		fmt.Fprintf(out, "external_id: %s\n", task.ExternalID)
	}
	if task.ExternalURL != "" {
		fmt.Fprintf(out, "external_url: %s\n", task.ExternalURL)
	}
	if task.ExternalRevision != "" {
		fmt.Fprintf(out, "external_revision: %s\n", task.ExternalRevision)
	}
	fmt.Fprintf(out, "created_at: %s\n", task.CreatedAt)
	fmt.Fprintf(out, "updated_at: %s\n", task.UpdatedAt)
}

func printTaskEvents(out io.Writer, events []storage.TaskEvent) {
	if len(events) == 0 {
		fmt.Fprintln(out, "events: none")
		return
	}

	fmt.Fprintln(out, "events:")
	for _, event := range events {
		fmt.Fprintf(out, "- id: %d type: %s", event.ID, event.Type)
		if event.FromStatus != "" && event.ToStatus != "" {
			fmt.Fprintf(out, " from: %s to: %s", event.FromStatus, event.ToStatus)
		} else if event.ToStatus != "" {
			fmt.Fprintf(out, " status: %s", event.ToStatus)
		}
		if event.Body != "" {
			fmt.Fprintf(out, " body: %s", event.Body)
		}
		if event.ActorName != "" {
			fmt.Fprintf(out, " actor: %s/%s", event.ActorKind, event.ActorName)
		}
		if event.EvidenceRunID != 0 {
			fmt.Fprintf(out, " evidence_run_id: %d", event.EvidenceRunID)
		}
		if event.EvidenceArtifactID != 0 {
			fmt.Fprintf(out, " evidence_artifact_id: %d", event.EvidenceArtifactID)
		}
		fmt.Fprintf(out, " created_at: %s\n", event.CreatedAt)
	}
}

func printTaskEvent(out io.Writer, event storage.TaskEvent) {
	fmt.Fprintf(out, "id: %d\n", event.ID)
	fmt.Fprintf(out, "task_id: %d\n", event.TaskID)
	fmt.Fprintf(out, "type: %s\n", event.Type)
	fmt.Fprintf(out, "body: %s\n", event.Body)
	if event.ActorName != "" {
		fmt.Fprintf(out, "actor_kind: %s\n", event.ActorKind)
		fmt.Fprintf(out, "actor_name: %s\n", event.ActorName)
		fmt.Fprintf(out, "actor_id: %d\n", event.ActorID)
	}
	if event.EvidenceRunID != 0 {
		fmt.Fprintf(out, "evidence_run_id: %d\n", event.EvidenceRunID)
	}
	if event.EvidenceArtifactID != 0 {
		fmt.Fprintf(out, "evidence_artifact_id: %d\n", event.EvidenceArtifactID)
	}
	fmt.Fprintf(out, "created_at: %s\n", event.CreatedAt)
}

func printTaskDependency(out io.Writer, dependency storage.TaskDependency) {
	fmt.Fprintf(out, "id: %d\n", dependency.ID)
	fmt.Fprintf(out, "edge_type: %s\n", dependency.EdgeType)
	fmt.Fprintf(out, "blocker_task_id: %d\n", dependency.BlockerTaskID)
	fmt.Fprintf(out, "blocked_task_id: %d\n", dependency.BlockedTaskID)
	fmt.Fprintf(out, "created_at: %s\n", dependency.CreatedAt)
}

type readyTaskOutput struct {
	ID                 int64  `json:"id"`
	ProjectID          int64  `json:"project_id"`
	Status             string `json:"status"`
	Title              string `json:"title"`
	Description        string `json:"description"`
	AcceptanceCriteria string `json:"acceptance_criteria"`
	Notes              string `json:"notes"`
	Source             string `json:"source"`
	ExternalID         string `json:"external_id"`
	ExternalURL        string `json:"external_url"`
	ExternalRevision   string `json:"external_revision"`
	CreatedAt          string `json:"created_at"`
	UpdatedAt          string `json:"updated_at"`
}

type taskEventOutput struct {
	ID                 int64        `json:"id"`
	TaskID             int64        `json:"task_id"`
	Type               string       `json:"type"`
	Body               string       `json:"body"`
	FromStatus         string       `json:"from_status"`
	ToStatus           string       `json:"to_status"`
	Actor              *actorOutput `json:"actor,omitempty"`
	EvidenceRunID      int64        `json:"evidence_run_id,omitempty"`
	EvidenceArtifactID int64        `json:"evidence_artifact_id,omitempty"`
	CreatedAt          string       `json:"created_at"`
}

type actorOutput struct {
	ID   int64  `json:"id"`
	Kind string `json:"kind"`
	Name string `json:"name"`
}

type taskShowOutput struct {
	Task   readyTaskOutput   `json:"task"`
	Events []taskEventOutput `json:"events"`
}

func printReadyTasksJSON(out io.Writer, tasks []storage.Task) error {
	return printTasksJSON(out, tasks)
}

func printTaskJSON(out io.Writer, task storage.Task) error {
	encoder := json.NewEncoder(out)
	encoder.SetIndent("", "  ")
	return encoder.Encode(taskOutputFromStorage(task))
}

func printTasksJSON(out io.Writer, tasks []storage.Task) error {
	readyTasks := make([]readyTaskOutput, 0, len(tasks))
	for _, task := range tasks {
		readyTasks = append(readyTasks, taskOutputFromStorage(task))
	}

	encoder := json.NewEncoder(out)
	encoder.SetIndent("", "  ")
	return encoder.Encode(readyTasks)
}

func printClaimedTaskJSON(out io.Writer, task storage.Task) error {
	return printTaskJSON(out, task)
}

func printTaskEventJSON(out io.Writer, event storage.TaskEvent) error {
	encoder := json.NewEncoder(out)
	encoder.SetIndent("", "  ")
	return encoder.Encode(taskEventOutputFromStorage(event))
}

func printTaskDependencyJSON(out io.Writer, dependency storage.TaskDependency) error {
	encoder := json.NewEncoder(out)
	encoder.SetIndent("", "  ")
	return encoder.Encode(taskDependencyOutputFromStorage(dependency))
}

func printTaskDependencyRemovedJSON(out io.Writer, dependency taskDependencyOptions) error {
	encoder := json.NewEncoder(out)
	encoder.SetIndent("", "  ")
	return encoder.Encode(struct {
		Removed       bool   `json:"removed"`
		EdgeType      string `json:"edge_type"`
		BlockerTaskID int64  `json:"blocker_task_id"`
		BlockedTaskID int64  `json:"blocked_task_id"`
	}{
		Removed:       true,
		EdgeType:      dependency.edgeType,
		BlockerTaskID: dependency.blockerTaskID,
		BlockedTaskID: dependency.blockedTaskID,
	})
}

func printTaskShowJSON(out io.Writer, task storage.Task, events []storage.TaskEvent) error {
	eventOutputs := make([]taskEventOutput, 0, len(events))
	for _, event := range events {
		eventOutputs = append(eventOutputs, taskEventOutputFromStorage(event))
	}

	encoder := json.NewEncoder(out)
	encoder.SetIndent("", "  ")
	return encoder.Encode(taskShowOutput{
		Task:   taskOutputFromStorage(task),
		Events: eventOutputs,
	})
}

func actorOutputFromSnapshot(id int64, kind, name string) *actorOutput {
	if id <= 0 || kind == "" || name == "" {
		return nil
	}
	return &actorOutput{
		ID:   id,
		Kind: kind,
		Name: name,
	}
}

func taskOutputFromStorage(task storage.Task) readyTaskOutput {
	return readyTaskOutput{
		ID:                 task.ID,
		ProjectID:          task.ProjectID,
		Status:             task.Status,
		Title:              task.Title,
		Description:        task.Description,
		AcceptanceCriteria: task.AcceptanceCriteria,
		Notes:              task.Notes,
		Source:             task.Source,
		ExternalID:         task.ExternalID,
		ExternalURL:        task.ExternalURL,
		ExternalRevision:   task.ExternalRevision,
		CreatedAt:          task.CreatedAt,
		UpdatedAt:          task.UpdatedAt,
	}
}

func taskEventOutputFromStorage(event storage.TaskEvent) taskEventOutput {
	return taskEventOutput{
		ID:                 event.ID,
		TaskID:             event.TaskID,
		Type:               event.Type,
		Body:               event.Body,
		FromStatus:         event.FromStatus,
		ToStatus:           event.ToStatus,
		Actor:              actorOutputFromSnapshot(event.ActorID, event.ActorKind, event.ActorName),
		EvidenceRunID:      event.EvidenceRunID,
		EvidenceArtifactID: event.EvidenceArtifactID,
		CreatedAt:          event.CreatedAt,
	}
}
