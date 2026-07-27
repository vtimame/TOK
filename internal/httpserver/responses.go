package httpserver

import (
	"context"
	"sort"
	"strconv"

	"s26.sh/tok/internal/storage"
)

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

func (a *api) agentOutput(ctx context.Context, actor storage.Actor) (AgentOutput, error) {
	activity := storage.AgentActivity{Actor: actor}
	agents, err := a.store.ListAgentActivity(ctx)
	if err != nil {
		return AgentOutput{}, err
	}
	for _, agent := range agents {
		if agent.Actor.ID == actor.ID {
			activity = agent
			break
		}
	}
	projects, err := a.store.ListAgentProjectActivity(ctx)
	if err != nil {
		return AgentOutput{}, err
	}
	return agentOutputFromStorage(activity, projects), nil
}

func (a *api) projectResponse(ctx context.Context, project storage.Project) (ProjectResponse, error) {
	projectOut, err := a.projectOutput(ctx, project)
	if err != nil {
		return ProjectResponse{}, err
	}
	return ProjectResponse{Project: projectOut}, nil
}

func (a *api) taskResponse(ctx context.Context, task storage.Task) (TaskResponse, error) {
	events, err := a.store.ListTaskEvents(ctx, task.ID)
	if err != nil {
		return TaskResponse{}, err
	}
	project, err := a.store.GetProjectByID(ctx, task.ProjectID)
	if err != nil {
		return TaskResponse{}, err
	}
	return TaskResponse{Task: taskFromStorage(task, project, agentsFromEvents(events))}, nil
}

func (a *api) taskListResponse(ctx context.Context, tasks []storage.Task, total, limit int) (TaskListResponse, error) {
	responseLimit := limit
	if responseLimit == 0 {
		responseLimit = len(tasks)
	}
	hasNext := limit > 0 && len(tasks) > limit
	if hasNext {
		tasks = tasks[:limit]
	}
	agentsByTask, err := a.agentsByTask(ctx, tasks)
	if err != nil {
		return TaskListResponse{}, err
	}
	projectsByID, err := a.projectsByTask(ctx, tasks)
	if err != nil {
		return TaskListResponse{}, err
	}
	out := tasksFromStorage(tasks, agentsByTask, projectsByID)
	out.Total = total
	out.Limit = responseLimit
	if hasNext && len(tasks) > 0 {
		out.NextCursor = strconv.FormatInt(tasks[len(tasks)-1].ID, 10)
	}
	return out, nil
}

func pageFetchLimit(limit int) int {
	if limit <= 0 {
		return limit
	}
	return limit + 1
}

func (a *api) projectListResponse(ctx context.Context, projects []storage.Project, total, limit int) (ProjectListResponse, error) {
	responseLimit := limit
	if responseLimit == 0 {
		responseLimit = len(projects)
	}
	hasNext := limit > 0 && len(projects) > limit
	if hasNext {
		projects = projects[:limit]
	}

	out := ProjectListResponse{Projects: make([]ProjectOutput, 0, len(projects)), Total: total, Limit: responseLimit}
	for _, project := range projects {
		projectOut, err := a.projectOutput(ctx, project)
		if err != nil {
			return ProjectListResponse{}, err
		}
		out.Projects = append(out.Projects, projectOut)
	}
	if hasNext && len(projects) > 0 {
		out.NextCursor = projects[len(projects)-1].Name
	}
	return out, nil
}

func (a *api) projectsByTask(ctx context.Context, tasks []storage.Task) (map[int64]storage.Project, error) {
	out := make(map[int64]storage.Project)
	for _, task := range tasks {
		if _, ok := out[task.ProjectID]; ok {
			continue
		}
		project, err := a.store.GetProjectByID(ctx, task.ProjectID)
		if err != nil {
			return nil, err
		}
		out[task.ProjectID] = project
	}
	return out, nil
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
