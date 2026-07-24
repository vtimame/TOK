import type { ActorOutput } from "@/api/generated/models/ActorOutput.ts";
import type { AgentOutput as ApiAgentOutput } from "@/api/generated/models/AgentOutput.ts";
import type { AgentProjectOutput as ApiAgentProjectOutput } from "@/api/generated/models/AgentProjectOutput.ts";
import type { ProjectOutput } from "@/api/generated/models/ProjectOutput.ts";
import type { TaskDependencyOutput } from "@/api/generated/models/TaskDependencyOutput.ts";
import type { TaskEventOutput } from "@/api/generated/models/TaskEventOutput.ts";
import type { TaskOutput } from "@/api/generated/models/TaskOutput.ts";
import type { Project } from "@/components/pages/projects";

export type Task = {
  id: number;
  projectId: number;
  project: {
    id: number;
    name: string;
    displayName: string;
  };
  title: string;
  description: string;
  acceptanceCriteria: string;
  notes: string;
  status: string;
  createdAt: string;
  updatedAt: string;
  agents: string[];
};

export type TaskEvent = {
  id: number;
  taskId: number;
  type: string;
  body: string;
  fromStatus: string;
  toStatus: string;
  createdAt: string;
  actor?: {
    id: number;
    name: string;
    icon: string;
  };
};

export type TaskDependency = {
  id: number;
  edgeType: string;
  blockerTaskId: number;
  blockedTaskId: number;
  role: string;
  createdAt: string;
};

export type AgentProject = {
  id: number;
  name: string;
  displayName: string;
  tasksCount: number;
  eventsCount: number;
  lastActivityAt: string;
};

export type Agent = {
  id: number;
  kind: string;
  name: string;
  icon: string;
  projects: AgentProject[];
  tasksCount: number;
  eventsCount: number;
  lastActivityAt: string;
  createdAt: string;
  updatedAt: string;
};

export function projectFromApi(project: ProjectOutput): Project {
  return {
    id: project.id,
    name: project.name,
    displayName: project.display_name,
    path: project.path,
    createdAt: project.created_at,
    updatedAt: project.updated_at,
    tasksCount: project.tasks_count,
    taskCounts: project.task_counts,
    agents: (project.agents ?? []).map((actor) => agentIconValue(actor.name)),
  };
}

export function agentFromApi(agent: ApiAgentOutput): Agent {
  return {
    id: agent.id,
    kind: agent.kind,
    name: actorDisplayName(agent),
    icon: agentIconValue(agent.name),
    projects: (agent.projects ?? []).map(agentProjectFromApi),
    tasksCount: agent.tasks_count,
    eventsCount: agent.events_count,
    lastActivityAt: agent.last_activity_at,
    createdAt: agent.created_at,
    updatedAt: agent.updated_at,
  };
}

function agentProjectFromApi(project: ApiAgentProjectOutput): AgentProject {
  return {
    id: project.id,
    name: project.name,
    displayName: project.display_name,
    tasksCount: project.tasks_count,
    eventsCount: project.events_count,
    lastActivityAt: project.last_activity_at,
  };
}

export function taskFromApi(task: TaskOutput): Task {
  return {
    id: task.id,
    projectId: task.project_id,
    project: {
      id: task.project.id,
      name: task.project.name,
      displayName: task.project.display_name,
    },
    title: task.title,
    description: task.description,
    acceptanceCriteria: task.acceptance_criteria,
    notes: task.notes,
    status: task.status,
    createdAt: task.created_at,
    updatedAt: task.updated_at,
    agents: (task.agents ?? []).map((actor) => agentIconValue(actor.name)),
  };
}

export function taskEventFromApi(event: TaskEventOutput): TaskEvent {
  return {
    id: event.id,
    taskId: event.task_id,
    type: event.type,
    body: event.body,
    fromStatus: event.from_status,
    toStatus: event.to_status,
    createdAt: event.created_at,
    actor: event.actor
      ? {
          id: event.actor.id,
          name: event.actor.name,
          icon: agentIconValue(event.actor.name),
        }
      : undefined,
  };
}

export function taskDependencyFromApi(dependency: TaskDependencyOutput): TaskDependency {
  return {
    id: dependency.id,
    edgeType: dependency.edge_type,
    blockerTaskId: dependency.blocker_task_id,
    blockedTaskId: dependency.blocked_task_id,
    role: dependency.role,
    createdAt: dependency.created_at,
  };
}

export function agentIconValue(name: string): string {
  const normalized = name.trim().toLowerCase();
  for (const known of ["codex", "openai", "claude", "gemini", "cursor", "copilot", "grok"]) {
    if (normalized.includes(known)) {
      return known;
    }
  }
  return normalized || "agent";
}

export function actorDisplayName(actor: ActorOutput): string {
  return actor.name.trim() || `Agent ${actor.id}`;
}

export function statusLabel(status: string): string {
  return status.replace(/_/g, " ");
}

export function statusTone(status: string): string {
  switch (status) {
    case "done":
      return "border-emerald-500/30 bg-emerald-500/10 text-emerald-700";
    case "in_progress":
      return "border-sky-500/30 bg-sky-500/10 text-sky-700";
    case "blocked":
      return "border-destructive/30 bg-destructive/10 text-destructive";
    default:
      return "border-border bg-muted text-muted-foreground";
  }
}
