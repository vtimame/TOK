import { describe, expect, test } from "vitest";

import {
  actorDisplayName,
  agentIconValue,
  projectFromApi,
  statusLabel,
  statusTone,
  taskFromApi,
} from "@/api/mappers";

describe("mappers", () => {
  test("projectFromApi maps fields and agent icons", () => {
    const project = {
      id: 11,
      name: "backend-service",
      display_name: "Backend Service",
      path: "/opt/backend",
      created_at: "2026-01-01T00:00:00Z",
      updated_at: "2026-02-01T00:00:00Z",
      tasks_count: 5,
      task_counts: { done: 2 },
      agents: [{ name: "OpenAI helper" }, { name: "Unknown" }],
    } as unknown as Parameters<typeof projectFromApi>[0];

    expect(projectFromApi(project)).toEqual({
      id: 11,
      name: "backend-service",
      displayName: "Backend Service",
      path: "/opt/backend",
      createdAt: "2026-01-01T00:00:00Z",
      updatedAt: "2026-02-01T00:00:00Z",
      tasksCount: 5,
      taskCounts: { done: 2 },
      agents: ["openai", "unknown"],
    });
  });

  test("taskFromApi maps payload with optional agents", () => {
    const task = {
      id: 7,
      project_id: 2,
      project: {
        id: 2,
        name: "frontend",
        display_name: "Frontend",
      },
      title: "Build UI",
      description: "Create login screen",
      acceptance_criteria: "Works",
      notes: "N/A",
      status: "done",
      created_at: "2026-03-01T00:00:00Z",
      updated_at: "2026-03-02T00:00:00Z",
    } as unknown as Parameters<typeof taskFromApi>[0];

    expect(taskFromApi(task)).toEqual({
      id: 7,
      projectId: 2,
      project: {
        id: 2,
        name: "frontend",
        displayName: "Frontend",
      },
      title: "Build UI",
      description: "Create login screen",
      acceptanceCriteria: "Works",
      notes: "N/A",
      status: "done",
      createdAt: "2026-03-01T00:00:00Z",
      updatedAt: "2026-03-02T00:00:00Z",
      agents: [],
    });
  });

  test("agentIconValue and actorDisplayName are stable", () => {
    expect(agentIconValue("  OpenAI Assistant ")).toBe("openai");
    expect(agentIconValue("random")).toBe("random");
    expect(
      actorDisplayName({
        id: 3,
        name: "  ",
        kind: "agent",
      } as Parameters<typeof actorDisplayName>[0]),
    ).toBe("Agent 3");
  });

  test("status helpers return expected values", () => {
    expect(statusLabel("in_progress")).toBe("in progress");
    expect(statusTone("done")).toBe("border-emerald-500/30 bg-emerald-500/10 text-emerald-700");
    expect(statusTone("blocked")).toBe("border-destructive/30 bg-destructive/10 text-destructive");
    expect(statusTone("other")).toBe("border-border bg-muted text-muted-foreground");
  });
});
