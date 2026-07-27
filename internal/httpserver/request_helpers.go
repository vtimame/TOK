package httpserver

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"os/user"
	"strconv"
	"strings"

	"github.com/go-fuego/fuego"
	tokservice "s26.sh/tok/internal/service"
	"s26.sh/tok/internal/storage"
)

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

func cursorQuery(ctx fuego.ContextNoBody, name string) (int64, error) {
	value := strings.TrimSpace(ctx.QueryParam(name))
	if value == "" {
		return 0, nil
	}
	cursor, err := strconv.ParseInt(value, 10, 64)
	if err != nil || cursor <= 0 {
		return 0, badRequest(fmt.Sprintf("%s must be a positive integer", name))
	}
	return cursor, nil
}

func boolQuery(ctx fuego.ContextNoBody, name string, defaultValue bool) (bool, error) {
	value := strings.TrimSpace(strings.ToLower(ctx.QueryParam(name)))
	if value == "" {
		return defaultValue, nil
	}
	switch value {
	case "1", "true", "yes", "on":
		return true, nil
	case "0", "false", "no", "off":
		return false, nil
	default:
		return false, badRequest(fmt.Sprintf("%s must be a boolean", name))
	}
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

func (a *api) agentByID(ctx context.Context, rawID string) (storage.Actor, error) {
	id, err := agentIDFromPath(rawID)
	if err != nil {
		return storage.Actor{}, err
	}
	actor, err := a.store.GetActor(ctx, id)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && actor.Kind != "agent") {
		return storage.Actor{}, fuego.NotFoundError{Title: "Agent not found", Detail: strconv.FormatInt(id, 10)}
	}
	return actor, err
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

func (a *api) projectAndInstructionFromPath(ctx fuego.ContextNoBody) (storage.Project, int64, error) {
	project, err := a.projectByName(ctx.Context(), ctx.PathParam("project"))
	if err != nil {
		return storage.Project{}, 0, err
	}
	instructionID, err := instructionIDFromPath(ctx.PathParam("id"))
	if err != nil {
		return storage.Project{}, 0, err
	}
	return project, instructionID, nil
}

func agentIDFromPath(raw string) (int64, error) {
	raw = strings.TrimSpace(raw)
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 {
		return 0, badRequest(fmt.Sprintf("invalid agent id: %s", raw))
	}
	return id, nil
}

func instructionIDFromPath(raw string) (int64, error) {
	raw = strings.TrimSpace(raw)
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 {
		return 0, badRequest(fmt.Sprintf("invalid project instruction id: %s", raw))
	}
	return id, nil
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
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return fuego.NotFoundError{Title: "Project not found"}
	case strings.Contains(err.Error(), "UNIQUE"):
		return fuego.ConflictError{Title: "Project already exists", Detail: err.Error()}
	default:
		return err
	}
}

func mapProjectInstructionError(err error) error {
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return fuego.NotFoundError{Title: "Project instruction not found"}
	case strings.Contains(err.Error(), "project instruction"):
		return badRequest(err.Error())
	default:
		return err
	}
}

func mapAgentWriteError(err error) error {
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return fuego.NotFoundError{Title: "Agent not found"}
	case strings.Contains(err.Error(), "UNIQUE"):
		return fuego.ConflictError{Title: "Agent already exists", Detail: err.Error()}
	default:
		return err
	}
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
	case errors.Is(err, storage.ErrActiveRunExists):
		return fuego.ConflictError{Title: "Task has an active run"}
	case errors.Is(err, storage.ErrTaskCompletionNoteEmpty), errors.Is(err, storage.ErrTaskNoteEmpty):
		return badRequest("task note is required")
	case errors.Is(err, storage.ErrInvalidTaskSource), errors.Is(err, storage.ErrInvalidTaskExternalReference):
		return badRequest(err.Error())
	case errors.Is(err, tokservice.ErrTaskCompletionEvidenceRequired):
		return fuego.ConflictError{Title: "Task completion evidence is required"}
	case errors.Is(err, tokservice.ErrOverrideReasonRequired):
		return badRequest("override reason is required")
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
