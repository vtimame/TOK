package httpserver

import (
	"strings"

	"github.com/go-fuego/fuego"
	"s26.sh/tok/internal/storage"
)

func (a *api) listProjects(ctx fuego.ContextNoBody) (ProjectListResponse, error) {
	limit, err := positiveIntQuery(ctx, "limit", 0, 100)
	if err != nil {
		return ProjectListResponse{}, err
	}
	cursor := strings.TrimSpace(ctx.QueryParam("cursor"))
	total, err := a.store.CountProjects(ctx.Context())
	if err != nil {
		return ProjectListResponse{}, err
	}
	projects, err := a.store.ListProjectsWithOptions(ctx.Context(), storage.ListProjectsOptions{Limit: pageFetchLimit(limit), Cursor: cursor})
	if err != nil {
		return ProjectListResponse{}, err
	}
	return a.projectListResponse(ctx.Context(), projects, total, limit)
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

func (a *api) updateProject(ctx fuego.ContextWithBody[UpdateProjectInput]) (ProjectResponse, error) {
	current, err := a.projectByName(ctx.Context(), ctx.PathParam("project"))
	if err != nil {
		return ProjectResponse{}, err
	}
	body, err := ctx.Body()
	if err != nil {
		return ProjectResponse{}, err
	}
	name := strings.TrimSpace(body.Name)
	displayName := strings.TrimSpace(body.DisplayName)
	path := strings.TrimSpace(body.Path)
	if name == "" {
		name = current.Name
	}
	if displayName == "" {
		displayName = name
	}
	if path == "" {
		path = current.Path
	}

	project, err := a.store.UpdateProject(ctx.Context(), current.ID, storage.UpdateProjectInput{
		Name:        name,
		DisplayName: displayName,
		Path:        path,
	})
	if err != nil {
		return ProjectResponse{}, mapProjectWriteError(err)
	}
	return a.projectResponse(ctx.Context(), project)
}

func (a *api) deleteProject(ctx fuego.ContextNoBody) (ProjectResponse, error) {
	project, err := a.projectByName(ctx.Context(), ctx.PathParam("project"))
	if err != nil {
		return ProjectResponse{}, err
	}
	out, err := a.projectOutput(ctx.Context(), project)
	if err != nil {
		return ProjectResponse{}, err
	}
	if err := a.store.DeleteProject(ctx.Context(), project.ID); err != nil {
		return ProjectResponse{}, mapProjectWriteError(err)
	}
	return ProjectResponse{Project: out}, nil
}

func (a *api) listProjectInstructions(ctx fuego.ContextNoBody) (ProjectInstructionListResponse, error) {
	project, err := a.projectByName(ctx.Context(), ctx.PathParam("project"))
	if err != nil {
		return ProjectInstructionListResponse{}, err
	}
	includeDisabled, err := boolQuery(ctx, "includeDisabled", false)
	if err != nil {
		return ProjectInstructionListResponse{}, err
	}
	instructions, err := a.store.ListProjectInstructions(ctx.Context(), storage.ListProjectInstructionsOptions{
		ProjectID:       project.ID,
		IncludeDisabled: includeDisabled,
	})
	if err != nil {
		return ProjectInstructionListResponse{}, err
	}
	return projectInstructionsFromStorage(instructions), nil
}

func (a *api) createProjectInstruction(ctx fuego.ContextWithBody[ProjectInstructionInput]) (ProjectInstructionResponse, error) {
	project, err := a.projectByName(ctx.Context(), ctx.PathParam("project"))
	if err != nil {
		return ProjectInstructionResponse{}, err
	}
	body, err := ctx.Body()
	if err != nil {
		return ProjectInstructionResponse{}, err
	}
	instruction, err := a.store.CreateProjectInstruction(ctx.Context(), storage.CreateProjectInstructionInput{
		ProjectID: project.ID,
		Title:     body.Title,
		Body:      body.Body,
		Priority:  body.Priority,
	})
	if err != nil {
		return ProjectInstructionResponse{}, mapProjectInstructionError(err)
	}
	return ProjectInstructionResponse{Instruction: projectInstructionFromStorage(instruction)}, nil
}

func (a *api) showProjectInstruction(ctx fuego.ContextNoBody) (ProjectInstructionResponse, error) {
	project, instructionID, err := a.projectAndInstructionFromPath(ctx)
	if err != nil {
		return ProjectInstructionResponse{}, err
	}
	instruction, err := a.store.GetProjectInstruction(ctx.Context(), project.ID, instructionID)
	if err != nil {
		return ProjectInstructionResponse{}, mapProjectInstructionError(err)
	}
	return ProjectInstructionResponse{Instruction: projectInstructionFromStorage(instruction)}, nil
}

func (a *api) enableProjectInstruction(ctx fuego.ContextNoBody) (ProjectInstructionResponse, error) {
	return a.setProjectInstructionEnabled(ctx, true)
}

func (a *api) disableProjectInstruction(ctx fuego.ContextNoBody) (ProjectInstructionResponse, error) {
	return a.setProjectInstructionEnabled(ctx, false)
}

func (a *api) setProjectInstructionEnabled(ctx fuego.ContextNoBody, enabled bool) (ProjectInstructionResponse, error) {
	project, instructionID, err := a.projectAndInstructionFromPath(ctx)
	if err != nil {
		return ProjectInstructionResponse{}, err
	}
	instruction, err := a.store.SetProjectInstructionEnabled(ctx.Context(), project.ID, instructionID, enabled)
	if err != nil {
		return ProjectInstructionResponse{}, mapProjectInstructionError(err)
	}
	return ProjectInstructionResponse{Instruction: projectInstructionFromStorage(instruction)}, nil
}

func (a *api) deleteProjectInstruction(ctx fuego.ContextNoBody) (ProjectInstructionResponse, error) {
	project, instructionID, err := a.projectAndInstructionFromPath(ctx)
	if err != nil {
		return ProjectInstructionResponse{}, err
	}
	instruction, err := a.store.GetProjectInstruction(ctx.Context(), project.ID, instructionID)
	if err != nil {
		return ProjectInstructionResponse{}, mapProjectInstructionError(err)
	}
	if err := a.store.DeleteProjectInstruction(ctx.Context(), project.ID, instructionID); err != nil {
		return ProjectInstructionResponse{}, mapProjectInstructionError(err)
	}
	return ProjectInstructionResponse{Instruction: projectInstructionFromStorage(instruction)}, nil
}
