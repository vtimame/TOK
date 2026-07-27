package httpserver

import (
	"github.com/go-fuego/fuego"
	"github.com/go-fuego/fuego/option"
)

func registerRoutes(s *fuego.Server, a *api) {
	fuego.Get(s, "/api/health", a.health, operation("getHealth", "System", "Show local TOK UI API health")...)

	fuego.Get(s, "/api/agents", a.listAgents, operation("listAgents", "Agents", "List registered agents with project activity")...)
	fuego.Post(s, "/api/agents", a.createAgent, append(operation("createAgent", "Agents", "Register an agent"), jsonBody()...)...)
	fuego.Get(s, "/api/agents/{id}", a.showAgent, operation("showAgent", "Agents", "Show an agent")...)
	fuego.Patch(s, "/api/agents/{id}", a.updateAgent, append(operation("updateAgent", "Agents", "Update an agent"), jsonBody()...)...)
	fuego.Delete(s, "/api/agents/{id}", a.deleteAgent, operation("deleteAgent", "Agents", "Delete an agent")...)

	fuego.Get(s, "/api/projects", a.listProjects, append(operation("listProjects", "Projects", "List registered projects"), option.Query("limit", "Maximum projects to return"), option.Query("cursor", "Optional cursor for the next page"))...)
	fuego.Post(s, "/api/projects", a.createProject, append(operation("createProject", "Projects", "Register a project"), jsonBody()...)...)
	fuego.Get(s, "/api/projects/{project}", a.showProject, operation("showProject", "Projects", "Show a project")...)
	fuego.Patch(s, "/api/projects/{project}", a.updateProject, append(operation("updateProject", "Projects", "Update a project"), jsonBody()...)...)
	fuego.Delete(s, "/api/projects/{project}", a.deleteProject, operation("deleteProject", "Projects", "Delete a project")...)
	fuego.Get(s, "/api/projects/{project}/instructions", a.listProjectInstructions, append(operation("listProjectInstructions", "Projects", "List project instructions"), option.Query("includeDisabled", "Include disabled instructions"))...)
	fuego.Post(s, "/api/projects/{project}/instructions", a.createProjectInstruction, append(operation("createProjectInstruction", "Projects", "Create a project instruction"), jsonBody()...)...)
	fuego.Get(s, "/api/projects/{project}/instructions/{id}", a.showProjectInstruction, operation("showProjectInstruction", "Projects", "Show a project instruction")...)
	fuego.Post(s, "/api/projects/{project}/instructions/{id}/enable", a.enableProjectInstruction, operation("enableProjectInstruction", "Projects", "Enable a project instruction")...)
	fuego.Post(s, "/api/projects/{project}/instructions/{id}/disable", a.disableProjectInstruction, operation("disableProjectInstruction", "Projects", "Disable a project instruction")...)
	fuego.Delete(s, "/api/projects/{project}/instructions/{id}", a.deleteProjectInstruction, operation("deleteProjectInstruction", "Projects", "Delete a project instruction")...)

	fuego.Get(s, "/api/projects/{project}/tasks", a.listTasks, append(operation("listProjectTasks", "Tasks", "List project tasks"), option.Query("limit", "Maximum tasks to return"), option.Query("cursor", "Optional cursor for the next page"), option.Query("status", "Optional task status filter"))...)
	fuego.Post(s, "/api/projects/{project}/tasks", a.createTask, append(operation("createTask", "Tasks", "Create a project task"), jsonBody()...)...)
	fuego.Get(s, "/api/projects/{project}/tasks/ready", a.readyTasks, operation("listReadyTasks", "Tasks", "List ready project tasks")...)
	fuego.Post(s, "/api/projects/{project}/tasks/claim", a.claimTask, append(operation("claimTask", "Tasks", "Claim the next ready task or a specific ready task"), jsonBody()...)...)
	fuego.Get(s, "/api/tasks", a.listAllTasks, append(operation("listTasks", "Tasks", "List tasks"), option.Query("limit", "Maximum tasks to return"), option.Query("cursor", "Optional cursor for the next page"), option.Query("projectId", "Optional project id filter"), option.Query("project", "Optional project name filter"), option.Query("status", "Optional comma-separated task status filter"))...)
	fuego.Get(s, "/api/tasks/{id}", a.showTask, operation("showTask", "Tasks", "Show a task with event history")...)
	fuego.Post(s, "/api/tasks/{id}/comment", a.commentTask, append(operation("commentTask", "Tasks", "Add a task comment"), jsonBody()...)...)
	fuego.Post(s, "/api/tasks/{id}/progress", a.progressTask, append(operation("progressTask", "Tasks", "Add task progress"), jsonBody()...)...)
	fuego.Patch(s, "/api/tasks/{id}/source", a.updateTaskSource, append(operation("updateTaskSource", "Tasks", "Update a task external source reference"), jsonBody()...)...)
	fuego.Post(s, "/api/tasks/{id}/block", a.blockTask, append(operation("blockTask", "Tasks", "Block a task"), jsonBody()...)...)
	fuego.Post(s, "/api/tasks/{id}/unblock", a.unblockTask, append(operation("unblockTask", "Tasks", "Unblock a task"), jsonBody()...)...)
	fuego.Post(s, "/api/tasks/{id}/done", a.doneTask, append(operation("completeTask", "Tasks", "Complete a task"), jsonBody()...)...)

	fuego.Get(s, "/api/index", a.indexStatusAll, operation("listIndexStatus", "Index", "Show all project index statuses")...)
	fuego.Post(s, "/api/index/update", a.indexUpdateAll, operation("updateAllProjectIndexes", "Index", "Update all project indexes")...)
	fuego.Get(s, "/api/projects/{project}/index", a.indexStatus, operation("getProjectIndexStatus", "Index", "Show project index status")...)
	fuego.Post(s, "/api/projects/{project}/index/update", a.indexUpdate, operation("updateProjectIndex", "Index", "Update project index")...)
	fuego.Get(s, "/api/projects/{project}/index/ignore", a.indexIgnorePolicy, operation("getProjectIndexIgnorePolicy", "Index", "Show project index ignore policy")...)
	fuego.Post(s, "/api/projects/{project}/index/ignore/refresh", a.indexIgnoreRefresh, operation("refreshProjectIndexIgnorePolicy", "Index", "Refresh project index ignore policy from .gitignore")...)
	fuego.Post(s, "/api/projects/{project}/index/ignore/patterns", a.indexIgnoreAdd, append(operation("addProjectIndexIgnorePattern", "Index", "Add project index ignore pattern"), jsonBody()...)...)
	fuego.Delete(s, "/api/projects/{project}/index/ignore/patterns", a.indexIgnoreRemove, append(operation("removeProjectIndexIgnorePattern", "Index", "Remove project index ignore pattern"), jsonBody()...)...)
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
