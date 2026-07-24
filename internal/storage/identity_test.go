package storage

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"
)

func TestLocalHumanProfileCanBeSetAndUpdated(t *testing.T) {
	ctx := context.Background()
	store := openInitializedTestStore(t)

	if _, err := store.GetLocalHuman(ctx); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("expected no explicit local user, got %v", err)
	}

	human, err := store.SetLocalHuman(ctx, "Timur Valitiov")
	if err != nil {
		t.Fatalf("SetLocalHuman returned error: %v", err)
	}
	if human.Kind != "human" || human.Name != "Timur Valitiov" || human.TokenHash != "" {
		t.Fatalf("unexpected local human: %+v", human)
	}

	updated, err := store.SetLocalHuman(ctx, "TOK Operator")
	if err != nil {
		t.Fatalf("second SetLocalHuman returned error: %v", err)
	}
	if updated.ID != human.ID || updated.Name != "TOK Operator" {
		t.Fatalf("expected local human to be updated in place, got %+v then %+v", human, updated)
	}
}

func TestCreateAgentReturnsRawTokenOnceAndStoresHash(t *testing.T) {
	ctx := context.Background()
	store := openInitializedTestStore(t)

	created, err := store.CreateAgent(ctx, "Codex Designer")
	if err != nil {
		t.Fatalf("CreateAgent returned error: %v", err)
	}
	if created.Agent.Kind != "agent" || created.Agent.Name != "Codex Designer" {
		t.Fatalf("unexpected created agent: %+v", created.Agent)
	}
	if !strings.HasPrefix(created.Token, agentTokenPrefix) {
		t.Fatalf("expected token prefix %q, got %q", agentTokenPrefix, created.Token)
	}
	if created.Agent.TokenHash == "" || created.Agent.TokenHash == created.Token || !strings.HasPrefix(created.Agent.TokenHash, "sha256:") {
		t.Fatalf("expected stored token hash, got agent=%+v token=%q", created.Agent, created.Token)
	}

	var storedHash string
	if err := store.db.QueryRowContext(ctx, "SELECT token_hash FROM actors WHERE id = ?", created.Agent.ID).Scan(&storedHash); err != nil {
		t.Fatalf("read stored token hash: %v", err)
	}
	if storedHash == created.Token || !strings.HasPrefix(storedHash, "sha256:") {
		t.Fatalf("raw token leaked to storage: %q", storedHash)
	}

	resolved, err := store.ResolveAgentByToken(ctx, created.Token)
	if err != nil {
		t.Fatalf("ResolveAgentByToken returned error: %v", err)
	}
	if resolved.ID != created.Agent.ID || resolved.Name != created.Agent.Name {
		t.Fatalf("unexpected resolved agent: %+v", resolved)
	}
}

func TestAgentListAndRevoke(t *testing.T) {
	ctx := context.Background()
	store := openInitializedTestStore(t)

	first, err := store.CreateAgent(ctx, "Codex")
	if err != nil {
		t.Fatalf("CreateAgent first returned error: %v", err)
	}
	second, err := store.CreateAgent(ctx, "Claude")
	if err != nil {
		t.Fatalf("CreateAgent second returned error: %v", err)
	}

	agents, err := store.ListAgents(ctx)
	if err != nil {
		t.Fatalf("ListAgents returned error: %v", err)
	}
	if len(agents) != 2 || agents[0].ID != first.Agent.ID || agents[1].ID != second.Agent.ID {
		t.Fatalf("unexpected agents: %+v", agents)
	}

	revoked, err := store.RevokeAgent(ctx, first.Agent.ID)
	if err != nil {
		t.Fatalf("RevokeAgent returned error: %v", err)
	}
	if revoked.TokenRevokedAt == "" {
		t.Fatalf("expected revoked timestamp: %+v", revoked)
	}
	if _, err := store.ResolveAgentByToken(ctx, first.Token); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("expected revoked token to stop resolving, got %v", err)
	}
	if _, err := store.ResolveAgentByToken(ctx, second.Token); err != nil {
		t.Fatalf("expected active second token to resolve, got %v", err)
	}
}

func TestTaskEventsAndRunsStoreActorAttributionSnapshots(t *testing.T) {
	ctx := context.Background()
	store := openInitializedTestStore(t)

	human, err := store.SetLocalHuman(ctx, "Original Name")
	if err != nil {
		t.Fatalf("SetLocalHuman returned error: %v", err)
	}
	actor := ActorRefFromActor(human)

	project, err := store.CreateProject(ctx, CreateProjectInput{
		Name:        "tok",
		DisplayName: "TOK",
		Path:        "/tmp/tok",
	})
	if err != nil {
		t.Fatalf("CreateProject returned error: %v", err)
	}
	task, err := store.CreateTask(ctx, CreateTaskInput{
		ProjectID: project.ID,
		Title:     "Attributed task",
		Actor:     actor,
	})
	if err != nil {
		t.Fatalf("CreateTask returned error: %v", err)
	}
	if _, err := store.ClaimTaskByActor(ctx, project.ID, task.ID, actor); err != nil {
		t.Fatalf("ClaimTaskByActor returned error: %v", err)
	}
	progress, err := store.AddTaskProgressByActor(ctx, task.ID, "Working.", actor)
	if err != nil {
		t.Fatalf("AddTaskProgressByActor returned error: %v", err)
	}
	if progress.ActorID != human.ID || progress.ActorKind != "human" || progress.ActorName != "Original Name" {
		t.Fatalf("unexpected progress actor snapshot: %+v", progress)
	}

	if _, err := store.SetLocalHuman(ctx, "Renamed User"); err != nil {
		t.Fatalf("rename local human returned error: %v", err)
	}
	events, err := store.ListTaskEvents(ctx, task.ID)
	if err != nil {
		t.Fatalf("ListTaskEvents returned error: %v", err)
	}
	if len(events) != 3 {
		t.Fatalf("unexpected task events: %+v", events)
	}
	for _, event := range events {
		if event.ActorID != human.ID || event.ActorKind != "human" || event.ActorName != "Original Name" {
			t.Fatalf("event did not keep actor snapshot after rename: %+v", event)
		}
	}

	run, err := store.CreateRun(ctx, CreateRunInput{
		TaskID:                 task.ID,
		HandoffContractVersion: "tok.handoff.v0",
		Actor:                  actor,
	})
	if err != nil {
		t.Fatalf("CreateRun returned error: %v", err)
	}
	if run.ActorID != human.ID || run.ActorKind != "human" || run.ActorName != "Original Name" {
		t.Fatalf("unexpected run actor snapshot: %+v", run)
	}
	artifact, err := store.AddRunArtifact(ctx, AddRunArtifactInput{
		RunID:    run.ID,
		Kind:     "validation",
		Metadata: `{"status":"passed"}`,
		Actor:    actor,
	})
	if err != nil {
		t.Fatalf("AddRunArtifact returned error: %v", err)
	}
	if artifact.ActorID != human.ID || artifact.ActorName != "Original Name" {
		t.Fatalf("unexpected artifact actor snapshot: %+v", artifact)
	}
	finished, err := store.FinishRun(ctx, FinishRunInput{
		ID:            run.ID,
		Status:        "succeeded",
		ResultSummary: "Done.",
		Actor:         actor,
	})
	if err != nil {
		t.Fatalf("FinishRun returned error: %v", err)
	}
	if finished.FinishedActorID != human.ID || finished.FinishedActorKind != "human" || finished.FinishedActorName != "Original Name" {
		t.Fatalf("unexpected finished actor snapshot: %+v", finished)
	}
}
