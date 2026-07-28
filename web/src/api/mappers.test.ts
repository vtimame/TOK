import { describe, expect, test } from "vitest";

import {
  actorDisplayName,
  agentIconValue,
  completionCandidateForTask,
  completionEvidenceForTask,
  parseValidationMetadata,
  projectFromApi,
  runFromApi,
  safeExternalUrl,
  statusLabel,
  statusTone,
  taskEventFromApi,
  taskFromApi,
  type Run,
} from "@/api/mappers";

function completionEvent(evidenceArtifactId: number, withActor = false) {
  return taskEventFromApi({
    id: 12,
    task_id: 7,
    type: "completed",
    body: "Done with tests.",
    from_status: "in_progress",
    to_status: "done",
    evidence_run_id: 3,
    evidence_artifact_id: evidenceArtifactId,
    created_at: "2026-03-02T00:00:00Z",
    actor: withActor ? { id: 4, kind: "agent", name: "Codex MCP" } : undefined,
  } as Parameters<typeof taskEventFromApi>[0]);
}

function completedEventAt(
  id: number,
  createdAt: string,
  body = "Done with tests.",
  evidence: { runId: number; artifactId: number } = { runId: 3, artifactId: 9 },
) {
  return taskEventFromApi({
    id,
    task_id: 7,
    type: "completed",
    body,
    from_status: "in_progress",
    to_status: "done",
    evidence_run_id: evidence.runId,
    evidence_artifact_id: evidence.artifactId,
    created_at: createdAt,
  } as Parameters<typeof taskEventFromApi>[0]);
}

function validationRun(metadata: string, overrides: Partial<Run> = {}) {
  return {
    id: overrides.id ?? 3,
    taskId: 7,
    status: overrides.status ?? "succeeded",
    handoffContractVersion: "tok.handoff.v0",
    retrievalLimit: 8,
    startedAt: "",
    finishedAt: "",
    baseBranch: "",
    baseHead: "",
    resultSummary: "",
    leaseOwner: "",
    heartbeatAt: "",
    expiresAt: "",
    artifacts: overrides.artifacts ?? [
      {
        id: 9,
        runId: overrides.id ?? 3,
        kind: "validation",
        path: "",
        contentHash: "",
        sizeBytes: 0,
        truncated: false,
        metadata,
        createdAt: "",
      },
    ],
  };
}

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
      source: "github",
      external_id: "42",
      external_url: "https://github.com/vtimame/TOK/issues/42",
      external_revision: "rev-1",
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
      source: "github",
      externalId: "42",
      externalUrl: "https://github.com/vtimame/TOK/issues/42",
      externalRevision: "rev-1",
      status: "done",
      createdAt: "2026-03-01T00:00:00Z",
      updatedAt: "2026-03-02T00:00:00Z",
      agents: [],
    });
  });

  test("taskFromApi keeps only safe external URLs clickable", () => {
    const baseTask = {
      id: 7,
      project_id: 2,
      project: {
        id: 2,
        name: "frontend",
        display_name: "Frontend",
      },
      title: "Build UI",
      description: "",
      acceptance_criteria: "",
      notes: "",
      source: "github",
      external_id: "42",
      external_revision: "",
      status: "open",
      created_at: "2026-03-01T00:00:00Z",
      updated_at: "2026-03-02T00:00:00Z",
    } as unknown as Parameters<typeof taskFromApi>[0];

    expect(safeExternalUrl(" https://github.com/vtimame/TOK/issues/42 ")).toBe(
      "https://github.com/vtimame/TOK/issues/42",
    );
    expect(safeExternalUrl("http://example.com/42")).toBe("http://example.com/42");
    expect(safeExternalUrl("javascript:alert(1)")).toBe("");
    expect(safeExternalUrl("data:text/html,<p>x</p>")).toBe("");
    expect(safeExternalUrl("://not-a-url")).toBe("");
    expect(
      taskFromApi({
        ...baseTask,
        external_url: "javascript:alert(1)",
      }).externalUrl,
    ).toBe("");
  });

  test("taskEventFromApi maps completion evidence ids", () => {
    const event = taskEventFromApi({
      id: 12,
      task_id: 7,
      type: "completed",
      body: "Done.",
      from_status: "in_progress",
      to_status: "done",
      evidence_run_id: 3,
      evidence_artifact_id: 9,
      created_at: "2026-03-02T00:00:00Z",
      actor: { id: 4, kind: "agent", name: "Codex MCP" },
    } as Parameters<typeof taskEventFromApi>[0]);

    expect(event.evidenceRunId).toBe(3);
    expect(event.evidenceArtifactId).toBe(9);
    expect(event.actor?.name).toBe("Codex MCP");
  });

  test("runFromApi maps artifacts and actors", () => {
    const run = runFromApi({
      id: 3,
      task_id: 7,
      status: "succeeded",
      handoff_contract_version: "tok.handoff.v0",
      retrieval_limit: 8,
      started_at: "2026-03-01T00:00:00Z",
      finished_at: "2026-03-02T00:00:00Z",
      base_branch: "main",
      base_head: "abc123456",
      result_summary: "Validation passed.",
      lease_owner: "worker",
      heartbeat_at: "2026-03-01T00:01:00Z",
      expires_at: "2026-03-01T00:30:00Z",
      started_by: { id: 4, kind: "agent", name: "Codex MCP" },
      finished_by: { id: 5, kind: "agent", name: "OpenAI Reviewer" },
      artifacts: [
        {
          id: 9,
          run_id: 3,
          kind: "validation",
          path: "",
          content_hash: "",
          size_bytes: 0,
          truncated: false,
          metadata: `{"status":"passed","command":"go test ./..."}`,
          created_at: "2026-03-02T00:00:00Z",
        },
      ],
    } as Parameters<typeof runFromApi>[0]);

    expect(run.startedBy?.icon).toBe("codex");
    expect(run.finishedBy?.icon).toBe("openai");
    expect(run.artifacts[0]).toMatchObject({
      id: 9,
      runId: 3,
      kind: "validation",
    });
  });

  test("completionEvidenceForTask returns validated evidence", () => {
    const event = completionEvent(9, true);
    const run = validationRun(`{"status":"passed","command":"go test ./...","summary":"ok"}`);

    const evidence = completionEvidenceForTask({ status: "done" }, [event], [run]);

    expect(evidence.status).toBe("validated");
    if (evidence.status !== "validated") throw new Error("expected validated evidence");
    expect(evidence.run?.id).toBe(3);
    expect(evidence.artifact?.id).toBe(9);
    expect(evidence.validation).toEqual({
      status: "passed",
      command: "go test ./...",
      summary: "ok",
    });
  });

  test("completionEvidenceForTask does not substitute another validation artifact", () => {
    const event = completionEvent(10);
    const run = validationRun(`{"status":"passed","command":"go test ./..."}`);

    const evidence = completionEvidenceForTask({ status: "done" }, [event], [run]);

    expect(evidence.status).toBe("missing_evidence");
    if (evidence.status !== "missing_evidence") throw new Error("expected missing evidence");
    expect(evidence.run?.id).toBe(3);
    expect(evidence.missingArtifactId).toBe(10);
  });

  test("completionEvidenceForTask uses latest cycle validated completion after old override", () => {
    const legacyOverride = taskEventFromApi({
      id: 1,
      task_id: 7,
      type: "completion_override",
      body: "Legacy override reason",
      from_status: "in_progress",
      to_status: "done",
      created_at: "2026-03-02T09:00:00Z",
      actor: { id: 10, kind: "agent", name: "OpenAI Reviewer" },
    } as Parameters<typeof taskEventFromApi>[0]);
    const reopen = taskEventFromApi({
      id: 2,
      task_id: 7,
      type: "status_changed",
      body: "Reopened after review",
      from_status: "done",
      to_status: "in_progress",
      created_at: "2026-03-02T09:30:00Z",
      actor: { id: 11, kind: "agent", name: "Ops Bot" },
    } as Parameters<typeof taskEventFromApi>[0]);
    const completed = taskEventFromApi({
      id: 3,
      task_id: 7,
      type: "completed",
      body: "Done with latest evidence.",
      from_status: "in_progress",
      to_status: "done",
      evidence_run_id: 5,
      evidence_artifact_id: 11,
      created_at: "2026-03-02T10:00:00Z",
      actor: { id: 12, kind: "agent", name: "Codex MCP" },
    } as Parameters<typeof taskEventFromApi>[0]);

    const evidence = completionEvidenceForTask(
      { status: "done" },
      [legacyOverride, completed, reopen],
      [
        {
          ...validationRun(`{"status":"passed","command":"go test ./..."}`, { id: 5 }),
          artifacts: [
            {
              id: 11,
              runId: 5,
              kind: "validation",
              path: "",
              contentHash: "",
              sizeBytes: 0,
              truncated: false,
              metadata: `{"status":"passed","command":"go test ./...","summary":"ok"}`,
              createdAt: "2026-03-02T10:01:00Z",
            },
          ],
        } as Run,
      ],
    );

    expect(evidence.status).toBe("validated");
    if (evidence.status !== "validated") throw new Error("expected validated evidence");
    expect(evidence.artifact.id).toBe(11);
    expect(evidence.validation.summary).toBe("ok");
  });

  test("completionEvidenceForTask prefers current override over old validated completion", () => {
    const completed = completedEventAt(21, "2026-03-02T09:00:00Z", "Original completion.");
    const reopen = taskEventFromApi({
      id: 22,
      task_id: 7,
      type: "status_changed",
      body: "Reopened before manual override.",
      from_status: "done",
      to_status: "in_progress",
      created_at: "2026-03-02T09:20:00Z",
    } as Parameters<typeof taskEventFromApi>[0]);
    const overrideCompleted = completedEventAt(
      23,
      "2026-03-02T09:39:00Z",
      "Closed with manual override.",
      { runId: 0, artifactId: 0 },
    );
    const override = taskEventFromApi({
      id: 24,
      task_id: 7,
      type: "completion_override",
      body: "Manual override requested.",
      from_status: "in_progress",
      to_status: "done",
      created_at: "2026-03-02T09:40:00Z",
      actor: { id: 13, kind: "agent", name: "OpenAI Reviewer" },
    } as Parameters<typeof taskEventFromApi>[0]);

    const evidence = completionEvidenceForTask(
      { status: "done" },
      [completed, reopen, overrideCompleted, override],
      [
        {
          ...validationRun(`{"status":"passed","command":"go test ./..."}`, { id: 3 }),
          artifacts: [
            {
              id: 9,
              runId: 3,
              kind: "validation",
              path: "",
              contentHash: "",
              sizeBytes: 0,
              truncated: false,
              metadata: `{"status":"passed","command":"go test ./...","summary":"ok"}`,
              createdAt: "2026-03-02T09:01:00Z",
            },
          ],
        } as Run,
      ],
    );

    expect(evidence.status).toBe("override");
    if (evidence.status !== "override") throw new Error("expected override evidence");
    expect(evidence.note).toBe("Closed with manual override.");
    expect(evidence.overrideReason).toBe("Manual override requested.");
    expect(evidence.actor?.name).toBe("OpenAI Reviewer");
  });

  test("completionEvidenceForTask keeps only the latest cycle when ending in override", () => {
    const oldCompleted = completedEventAt(
      29,
      "2026-03-02T08:00:00Z",
      "First cycle completed.",
      { runId: 0, artifactId: 0 },
    );
    const oldOverride = taskEventFromApi({
      id: 30,
      task_id: 7,
      type: "completion_override",
      body: "Old override reason.",
      from_status: "in_progress",
      to_status: "done",
      created_at: "2026-03-02T08:30:00Z",
      actor: { id: 14, kind: "agent", name: "Reviewer A" },
    } as Parameters<typeof taskEventFromApi>[0]);
    const reopenFirst = taskEventFromApi({
      id: 31,
      task_id: 7,
      type: "status_changed",
      body: "Reopen after first cycle.",
      from_status: "done",
      to_status: "in_progress",
      created_at: "2026-03-02T08:40:00Z",
    } as Parameters<typeof taskEventFromApi>[0]);
    const secondCompleted = completedEventAt(
      34,
      "2026-03-02T09:10:00Z",
      "Second cycle completed.",
      { runId: 3, artifactId: 11 },
    );
    const reopenSecond = taskEventFromApi({
      id: 35,
      task_id: 7,
      type: "status_changed",
      body: "Reopen before final override.",
      from_status: "done",
      to_status: "in_progress",
      created_at: "2026-03-02T09:20:00Z",
    } as Parameters<typeof taskEventFromApi>[0]);
    const latestCompleted = completedEventAt(
      36,
      "2026-03-02T09:29:00Z",
      "Final cycle completed with override.",
      { runId: 0, artifactId: 0 },
    );
    const latestOverride = taskEventFromApi({
      id: 37,
      task_id: 7,
      type: "completion_override",
      body: "Final override for latest cycle.",
      from_status: "in_progress",
      to_status: "done",
      created_at: "2026-03-02T09:30:00Z",
      actor: { id: 15, kind: "agent", name: "Reviewer B" },
    } as Parameters<typeof taskEventFromApi>[0]);

    const evidence = completionEvidenceForTask(
      { status: "done" },
      [
        latestOverride,
        reopenFirst,
        oldOverride,
        latestCompleted,
        reopenSecond,
        oldCompleted,
        secondCompleted,
      ],
      [validationRun(`{"status":"passed","command":"go test ./..."}`, { id: 3 })],
    );

    expect(evidence.status).toBe("override");
    if (evidence.status !== "override") throw new Error("expected override evidence");
    expect(evidence.overrideReason).toBe("Final override for latest cycle.");
  });

  test("completionEvidenceForTask returns missing_evidence for latest validated completion when old evidence exists", () => {
    const olderCompleted = completedEventAt(39, "2026-03-02T08:00:00Z", "Older completed.");
    const oldOverride = taskEventFromApi({
      id: 40,
      task_id: 7,
      type: "completion_override",
      body: "Outdated override.",
      from_status: "in_progress",
      to_status: "done",
      created_at: "2026-03-02T08:30:00Z",
      actor: { id: 10, kind: "agent", name: "Reviewer A" },
    } as Parameters<typeof taskEventFromApi>[0]);
    const latestCompleted = taskEventFromApi({
      id: 41,
      task_id: 7,
      type: "completed",
      body: "Latest completion with missing evidence.",
      from_status: "in_progress",
      to_status: "done",
      evidence_run_id: 99,
      evidence_artifact_id: 77,
      created_at: "2026-03-02T09:00:00Z",
    } as Parameters<typeof taskEventFromApi>[0]);

    const evidence = completionEvidenceForTask(
      { status: "done" },
      [oldOverride, olderCompleted, latestCompleted],
      [validationRun(`{"status":"passed","command":"go test ./..."}`, { id: 3 })],
    );

    expect(evidence.status).toBe("missing_evidence");
    if (evidence.status !== "missing_evidence") throw new Error("expected missing evidence");
    expect(evidence.missingRunId).toBe(99);
    expect(evidence.missingArtifactId).toBe(77);
  });

  test("completionEvidenceForTask returns override and legacy states", () => {
    const completed = taskEventFromApi({
      id: 12,
      task_id: 7,
      type: "completed",
      body: "Closed manually.",
      from_status: "in_progress",
      to_status: "done",
      created_at: "2026-03-02T00:00:00Z",
      actor: { id: 4, kind: "agent", name: "Codex MCP" },
    } as Parameters<typeof taskEventFromApi>[0]);
    const override = taskEventFromApi({
      id: 13,
      task_id: 7,
      type: "completion_override",
      body: "Validation cannot run locally.",
      from_status: "in_progress",
      to_status: "done",
      created_at: "2026-03-02T00:01:00Z",
      actor: { id: 5, kind: "agent", name: "OpenAI Reviewer" },
    } as Parameters<typeof taskEventFromApi>[0]);

    const overrideEvidence = completionEvidenceForTask(
      { status: "done" },
      [completed, override],
      [],
    );
    expect(overrideEvidence.status).toBe("override");
    if (overrideEvidence.status !== "override") throw new Error("expected override evidence");
    expect(overrideEvidence.actor?.name).toBe("OpenAI Reviewer");
    expect(overrideEvidence.overrideReason).toBe("Validation cannot run locally.");

    const legacyEvidence = completionEvidenceForTask({ status: "done" }, [completed], []);
    expect(legacyEvidence.status).toBe("legacy_unknown");
  });

  test("completionCandidateForTask selects the backend completion evidence candidate", () => {
    const older = validationRun(`{"status":"passed","command":"old"}`, { id: 2 });
    const latestWithoutPassedArtifact = validationRun(`{"status":"failed","command":"new"}`, {
      id: 4,
    });
    const selected = validationRun(`{"status":"failed"}`, {
      id: 3,
      artifacts: [
        {
          id: 10,
          runId: 3,
          kind: "validation",
          path: "",
          contentHash: "",
          sizeBytes: 0,
          truncated: false,
          metadata: `{"status":"failed"}`,
          createdAt: "",
        },
        {
          id: 11,
          runId: 3,
          kind: "validation",
          path: "",
          contentHash: "",
          sizeBytes: 0,
          truncated: false,
          metadata: `{"status":"passed","command":"go test ./..."}`,
          createdAt: "",
        },
      ],
    });

    const candidate = completionCandidateForTask({ status: "in_progress" }, [
      older,
      latestWithoutPassedArtifact,
      selected,
    ]);

    expect(candidate.status).toBe("ready");
    if (candidate.status !== "ready") throw new Error("expected completion candidate");
    expect(candidate.run.id).toBe(3);
    expect(candidate.artifact.id).toBe(11);
    expect(candidate.validation.command).toBe("go test ./...");
  });

  test("completionCandidateForTask blocks active runs before selecting evidence", () => {
    const active = validationRun(`{"status":"passed"}`, { id: 4, status: "in_progress" });
    const finished = validationRun(`{"status":"passed"}`, { id: 3 });

    const candidate = completionCandidateForTask({ status: "in_progress" }, [finished, active]);

    expect(candidate.status).toBe("active_run");
    if (candidate.status !== "active_run") throw new Error("expected active run");
    expect(candidate.run.id).toBe(4);
  });

  test("completionCandidateForTask reports missing evidence", () => {
    const failedValidation = validationRun(`{"status":"failed"}`, { id: 3 });

    expect(completionCandidateForTask({ status: "open" }, [failedValidation])).toEqual({
      status: "not_in_progress",
    });
    expect(completionCandidateForTask({ status: "in_progress" }, [failedValidation])).toEqual({
      status: "missing_evidence",
    });
  });

  test("parseValidationMetadata tolerates empty or invalid metadata", () => {
    expect(parseValidationMetadata("")).toEqual({});
    expect(parseValidationMetadata("{")).toEqual({});
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
