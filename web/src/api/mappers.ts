import type { ActorOutput } from "@/api/generated/models/ActorOutput.ts";
import type { AgentOutput as ApiAgentOutput } from "@/api/generated/models/AgentOutput.ts";
import type { AgentProjectOutput as ApiAgentProjectOutput } from "@/api/generated/models/AgentProjectOutput.ts";
import type { ProjectOutput } from "@/api/generated/models/ProjectOutput.ts";
import type { RunArtifactOutput } from "@/api/generated/models/RunArtifactOutput.ts";
import type { RunOutput } from "@/api/generated/models/RunOutput.ts";
import type { TaskDependencyOutput } from "@/api/generated/models/TaskDependencyOutput.ts";
import type { TaskEventOutput } from "@/api/generated/models/TaskEventOutput.ts";
import type { TaskOutput } from "@/api/generated/models/TaskOutput.ts";
import type { Project } from "@/components/pages/projects";

export type Actor = {
  id: number;
  name: string;
  icon: string;
};

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
  source: string;
  externalId: string;
  externalUrl: string;
  externalRevision: string;
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
  evidenceRunId: number;
  evidenceArtifactId: number;
  createdAt: string;
  actor?: Actor;
};

export type TaskDependency = {
  id: number;
  edgeType: string;
  blockerTaskId: number;
  blockedTaskId: number;
  role: string;
  createdAt: string;
};

export type RunArtifact = {
  id: number;
  runId: number;
  kind: string;
  path: string;
  contentHash: string;
  sizeBytes: number;
  truncated: boolean;
  metadata: string;
  actor?: Actor;
  createdAt: string;
};

export type Run = {
  id: number;
  taskId: number;
  status: string;
  handoffContractVersion: string;
  retrievalLimit: number;
  startedAt: string;
  finishedAt: string;
  baseBranch: string;
  baseHead: string;
  resultSummary: string;
  leaseOwner: string;
  heartbeatAt: string;
  expiresAt: string;
  startedBy?: Actor;
  finishedBy?: Actor;
  artifacts: RunArtifact[];
};

export type ValidationMetadata = {
  status?: string;
  command?: string;
  summary?: string;
};

export type CompletionEvidence =
  | { status: "not_done" }
  | {
      status: "validated";
      event: TaskEvent;
      run: Run;
      artifact: RunArtifact;
      validation: ValidationMetadata;
      actor?: Actor;
      note: string;
      completedAt: string;
    }
  | {
      status: "missing_evidence";
      event: TaskEvent;
      run?: Run;
      missingRunId: number;
      missingArtifactId: number;
      actor?: Actor;
      note: string;
      completedAt: string;
    }
  | {
      status: "override";
      event: TaskEvent;
      overrideEvent: TaskEvent;
      actor?: Actor;
      note: string;
      overrideReason: string;
      completedAt: string;
    }
  | {
      status: "legacy_unknown";
      event?: TaskEvent;
      actor?: Actor;
      note: string;
      completedAt: string;
    };

export type CompletionCandidate =
  | { status: "not_in_progress" }
  | { status: "active_run"; run: Run }
  | {
      status: "ready";
      run: Run;
      artifact: RunArtifact;
      validation: ValidationMetadata;
    }
  | { status: "missing_evidence" };

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
    source: task.source || "local",
    externalId: task.external_id || "",
    externalUrl: safeExternalUrl(task.external_url || ""),
    externalRevision: task.external_revision || "",
    status: task.status,
    createdAt: task.created_at,
    updatedAt: task.updated_at,
    agents: (task.agents ?? []).map((actor) => agentIconValue(actor.name)),
  };
}

export function safeExternalUrl(value: string): string {
  const trimmed = value.trim();
  if (!trimmed) return "";

  try {
    const url = new URL(trimmed);
    return url.protocol === "http:" || url.protocol === "https:" ? trimmed : "";
  } catch {
    return "";
  }
}

export function taskEventFromApi(event: TaskEventOutput): TaskEvent {
  return {
    id: event.id,
    taskId: event.task_id,
    type: event.type,
    body: event.body,
    fromStatus: event.from_status,
    toStatus: event.to_status,
    evidenceRunId: event.evidence_run_id || 0,
    evidenceArtifactId: event.evidence_artifact_id || 0,
    createdAt: event.created_at,
    actor: event.actor ? actorFromApi(event.actor) : undefined,
  };
}

export function runFromApi(run: RunOutput): Run {
  return {
    id: run.id,
    taskId: run.task_id,
    status: run.status,
    handoffContractVersion: run.handoff_contract_version,
    retrievalLimit: run.retrieval_limit,
    startedAt: run.started_at,
    finishedAt: run.finished_at,
    baseBranch: run.base_branch,
    baseHead: run.base_head,
    resultSummary: run.result_summary,
    leaseOwner: run.lease_owner,
    heartbeatAt: run.heartbeat_at,
    expiresAt: run.expires_at,
    startedBy: run.started_by ? actorFromApi(run.started_by) : undefined,
    finishedBy: run.finished_by ? actorFromApi(run.finished_by) : undefined,
    artifacts: (run.artifacts ?? []).map(runArtifactFromApi),
  };
}

function runArtifactFromApi(artifact: RunArtifactOutput): RunArtifact {
  return {
    id: artifact.id,
    runId: artifact.run_id,
    kind: artifact.kind,
    path: artifact.path,
    contentHash: artifact.content_hash,
    sizeBytes: artifact.size_bytes,
    truncated: artifact.truncated,
    metadata: artifact.metadata,
    actor: artifact.actor ? actorFromApi(artifact.actor) : undefined,
    createdAt: artifact.created_at,
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

export function completionEvidenceForTask(
  task: Pick<Task, "status">,
  events: TaskEvent[],
  runs: Run[],
): CompletionEvidence {
  if (task.status !== "done") {
    return { status: "not_done" };
  }

  const orderedEvents = [...events].sort(eventByCreatedAtThenId);
  const reversedEvents = [...orderedEvents].reverse();
  const completedEvent = reversedEvents.find((event) => event.type === "completed");

  const overrideEvent = reversedEvents.find(
    (event) => event.type === "completion_override" && eventCreatedAfter(event, completedEvent),
  );

  if (completedEvent && overrideEvent) {
    return {
      status: "override",
      event: completedEvent,
      overrideEvent,
      actor: overrideEvent.actor ?? completedEvent.actor,
      note: completedEvent.body,
      overrideReason: overrideEvent.body,
      completedAt: completedEvent.createdAt,
    };
  }

  if (completedEvent?.evidenceRunId) {
    const run = runs.find((item) => item.id === completedEvent.evidenceRunId);
    const artifact = run?.artifacts.find((item) => item.id === completedEvent.evidenceArtifactId);
    if (!run || !artifact) {
      return {
        status: "missing_evidence",
        event: completedEvent,
        run,
        missingRunId: completedEvent.evidenceRunId,
        missingArtifactId: completedEvent.evidenceArtifactId,
        actor: completedEvent.actor,
        note: completedEvent.body,
        completedAt: completedEvent.createdAt,
      };
    }

    return {
      status: "validated",
      event: completedEvent,
      run,
      artifact,
      validation: parseValidationMetadata(artifact?.metadata),
      actor: completedEvent.actor,
      note: completedEvent.body,
      completedAt: completedEvent.createdAt,
    };
  }

  return {
    status: "legacy_unknown",
    event: completedEvent,
    actor: completedEvent?.actor,
    note: completedEvent?.body || "",
    completedAt: completedEvent?.createdAt || "",
  };
}

function eventCreatedAtMs(event: TaskEvent): number {
  const timestamp = Date.parse(event.createdAt);
  return Number.isFinite(timestamp) ? timestamp : 0;
}

function eventByCreatedAtThenId(left: TaskEvent, right: TaskEvent): number {
  const leftAt = eventCreatedAtMs(left);
  const rightAt = eventCreatedAtMs(right);
  if (leftAt !== rightAt) return leftAt - rightAt;
  return left.id - right.id;
}

function eventCreatedAfter(left: TaskEvent, right?: TaskEvent): boolean {
  if (!right) return true;
  const leftAt = eventCreatedAtMs(left);
  const rightAt = eventCreatedAtMs(right);
  if (leftAt > rightAt) return true;
  if (leftAt < rightAt) return false;
  return left.id > right.id;
}

export function completionCandidateForTask(
  task: Pick<Task, "status">,
  runs: Run[],
): CompletionCandidate {
  if (task.status !== "in_progress") {
    return { status: "not_in_progress" };
  }

  const activeRun = latestRun(
    runs,
    (run) => run.status === "created" || run.status === "in_progress",
  );
  if (activeRun) {
    return { status: "active_run", run: activeRun };
  }

  const succeededRuns = runs
    .filter((run) => run.status === "succeeded")
    .sort((left, right) => right.id - left.id);
  for (const run of succeededRuns) {
    const artifact = latestPassedValidationArtifact(run);
    if (!artifact) continue;
    return {
      status: "ready",
      run,
      artifact,
      validation: parseValidationMetadata(artifact.metadata),
    };
  }

  return { status: "missing_evidence" };
}

export function parseValidationMetadata(metadata?: string): ValidationMetadata {
  if (!metadata) return {};
  try {
    const parsed = JSON.parse(metadata) as Record<string, unknown>;
    return {
      status: typeof parsed.status === "string" ? parsed.status : undefined,
      command: typeof parsed.command === "string" ? parsed.command : undefined,
      summary: typeof parsed.summary === "string" ? parsed.summary : undefined,
    };
  } catch {
    return {};
  }
}

function latestRun(runs: Run[], predicate: (run: Run) => boolean): Run | undefined {
  return runs.filter(predicate).sort((left, right) => right.id - left.id)[0];
}

function latestPassedValidationArtifact(run: Run): RunArtifact | undefined {
  return [...run.artifacts]
    .filter(
      (artifact) =>
        artifact.kind === "validation" &&
        parseValidationMetadata(artifact.metadata).status === "passed",
    )
    .sort((left, right) => right.id - left.id)[0];
}

function actorFromApi(actor: ActorOutput): Actor {
  return {
    id: actor.id,
    name: actorDisplayName(actor),
    icon: agentIconValue(actor.name),
  };
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

export function taskSourceLabel(source: string): string {
  switch (source) {
    case "github":
      return "GitHub";
    case "linear":
      return "Linear";
    case "jira":
      return "Jira";
    default:
      return "Local task";
  }
}
