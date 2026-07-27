package httpserver

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/go-fuego/fuego"
	tokservice "s26.sh/tok/internal/service"
	"s26.sh/tok/internal/storage"
)

func (a *api) listTasks(ctx fuego.ContextNoBody) (TaskListResponse, error) {
	project, err := a.projectByName(ctx.Context(), ctx.PathParam("project"))
	if err != nil {
		return TaskListResponse{}, err
	}
	limit, err := positiveIntQuery(ctx, "limit", 25, 100)
	if err != nil {
		return TaskListResponse{}, err
	}
	cursor, err := cursorQuery(ctx, "cursor")
	if err != nil {
		return TaskListResponse{}, err
	}
	statuses, err := statusesFromQuery(ctx)
	if err != nil {
		return TaskListResponse{}, err
	}
	opts := storage.ListTasksOptions{Statuses: statuses, ProjectID: project.ID, Limit: limit, Cursor: cursor}
	total, err := a.store.CountTasksWithOptions(ctx.Context(), project.ID, opts)
	if err != nil {
		return TaskListResponse{}, err
	}
	listOpts := opts
	listOpts.Limit = pageFetchLimit(limit)
	tasks, err := a.store.ListAllTasksWithOptions(ctx.Context(), listOpts)
	if err != nil {
		return TaskListResponse{}, err
	}
	return a.taskListResponse(ctx.Context(), tasks, total, limit)
}

func (a *api) listAllTasks(ctx fuego.ContextNoBody) (TaskListResponse, error) {
	limit, err := positiveIntQuery(ctx, "limit", 25, 100)
	if err != nil {
		return TaskListResponse{}, err
	}
	cursor, err := cursorQuery(ctx, "cursor")
	if err != nil {
		return TaskListResponse{}, err
	}
	statuses, err := statusesFromQuery(ctx)
	if err != nil {
		return TaskListResponse{}, err
	}
	projectID, err := a.taskProjectIDFromQuery(ctx)
	if err != nil {
		return TaskListResponse{}, err
	}
	opts := storage.ListTasksOptions{Statuses: statuses, ProjectID: projectID, Limit: limit, Cursor: cursor}
	total, err := a.store.CountTasksWithOptions(ctx.Context(), projectID, opts)
	if err != nil {
		return TaskListResponse{}, err
	}
	listOpts := opts
	listOpts.Limit = pageFetchLimit(limit)
	tasks, err := a.store.ListAllTasksWithOptions(ctx.Context(), listOpts)
	if err != nil {
		return TaskListResponse{}, err
	}
	return a.taskListResponse(ctx.Context(), tasks, total, limit)
}

func (a *api) taskProjectIDFromQuery(ctx fuego.ContextNoBody) (int64, error) {
	rawProjectID := strings.TrimSpace(ctx.QueryParam("projectId"))
	if rawProjectID != "" {
		projectID, err := strconv.ParseInt(rawProjectID, 10, 64)
		if err != nil || projectID <= 0 {
			return 0, badRequest("projectId must be a positive integer")
		}
		return projectID, nil
	}
	projectName := strings.TrimSpace(ctx.QueryParam("project"))
	if projectName == "" {
		return 0, nil
	}
	project, err := a.projectByName(ctx.Context(), projectName)
	if err != nil {
		return 0, err
	}
	return project.ID, nil
}

func statusesFromQuery(ctx fuego.ContextNoBody) ([]string, error) {
	raw := strings.TrimSpace(ctx.QueryParam("status"))
	if raw == "" {
		return nil, nil
	}
	parts := strings.Split(raw, ",")
	statuses := make([]string, 0, len(parts))
	for _, part := range parts {
		status := strings.TrimSpace(part)
		if status == "" {
			continue
		}
		if !validTaskStatus(status) {
			return nil, badRequest(fmt.Sprintf("invalid task status %q", status))
		}
		statuses = append(statuses, status)
	}
	return statuses, nil
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
		Source:             strings.TrimSpace(body.Source),
		ExternalID:         strings.TrimSpace(body.ExternalID),
		ExternalURL:        strings.TrimSpace(body.ExternalURL),
		ExternalRevision:   strings.TrimSpace(body.ExternalRevision),
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
	return a.taskListResponse(ctx.Context(), tasks, len(tasks), 0)
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
	dependencies, err := a.store.ListTaskDependencies(ctx.Context(), task.ProjectID, task.ID)
	if err != nil {
		return TaskShowResponse{}, err
	}
	project, err := a.store.GetProjectByID(ctx.Context(), task.ProjectID)
	if err != nil {
		return TaskShowResponse{}, err
	}
	return taskShowFromStorage(task, project, events, dependencies), nil
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

func (a *api) updateTaskSource(ctx fuego.ContextWithBody[TaskSourceInput]) (TaskResponse, error) {
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
	task, err := a.store.UpdateTaskExternalReference(ctx.Context(), storage.UpdateTaskExternalReferenceInput{
		ID:               taskID,
		Source:           strings.TrimSpace(body.Source),
		ExternalID:       strings.TrimSpace(body.ExternalID),
		ExternalURL:      strings.TrimSpace(body.ExternalURL),
		ExternalRevision: strings.TrimSpace(body.ExternalRevision),
		Actor:            actor,
	})
	if err != nil {
		return TaskResponse{}, mapTaskError(err)
	}
	return a.taskResponse(ctx.Context(), task)
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
	task, err := a.tasks.CompleteTask(ctx.Context(), tokservice.CompleteTaskInput{
		ID:               taskID,
		Note:             body.Note,
		EvidenceRunID:    body.EvidenceRunID,
		AllowUnvalidated: body.AllowUnvalidated,
		OverrideReason:   body.OverrideReason,
		Actor:            actor,
	})
	if err != nil {
		return TaskResponse{}, mapTaskError(err)
	}
	return a.taskResponse(ctx.Context(), task)
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
