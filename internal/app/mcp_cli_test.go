package app

import (
	"context"
	"errors"
	"strings"
	"testing"

	"s26.sh/tok/internal/mcpserver"
	"s26.sh/tok/internal/storage"
)

func TestMCPServeOptionsResolveTokenFromFlagOrEnv(t *testing.T) {
	t.Setenv(agentTokenEnv, " env-token ")

	opts, err := parseMCPServeOptions(nil)
	if err != nil {
		t.Fatalf("parse empty mcp serve options returned error: %v", err)
	}
	if opts.token != "env-token" {
		t.Fatalf("expected env token, got %+v", opts)
	}

	opts, err = parseMCPServeOptions([]string{"--token", " flag-token "})
	if err != nil {
		t.Fatalf("parse flag mcp serve options returned error: %v", err)
	}
	if opts.token != "flag-token" || opts.profile != "" {
		t.Fatalf("expected flag token to win, got %+v", opts)
	}

	opts, err = parseMCPServeOptions([]string{"--profile", "worker"})
	if err != nil {
		t.Fatalf("parse worker profile returned error: %v", err)
	}
	if opts.token != "env-token" || opts.profile != mcpserver.ProfileWorker {
		t.Fatalf("unexpected worker profile options: %+v", opts)
	}

	opts, err = parseMCPServeOptions([]string{"--profile=supervisor"})
	if err != nil {
		t.Fatalf("parse supervisor profile returned error: %v", err)
	}
	if opts.profile != mcpserver.ProfileSupervisor {
		t.Fatalf("unexpected supervisor profile options: %+v", opts)
	}

	if _, err := parseMCPServeOptions([]string{"--profile", "wide-open"}); err == nil {
		t.Fatal("expected invalid profile error")
	}
}

func TestResolveMCPActorRequiresValidAgentToken(t *testing.T) {
	ctx := context.Background()
	store := openAppTestStore(t)

	_, err := resolveMCPActor(ctx, store, "")
	var usageErr *UsageError
	if !errors.As(err, &usageErr) || !strings.Contains(usageErr.Message, "requires an agent token") {
		t.Fatalf("expected missing token usage error, got %T: %v", err, err)
	}

	err = nil
	_, err = resolveMCPActor(ctx, store, "invalid")
	if err == nil || !strings.Contains(err.Error(), "invalid agent token") {
		t.Fatalf("expected invalid token error, got %v", err)
	}

	created, err := store.CreateAgent(ctx, "Codex MCP")
	if err != nil {
		t.Fatalf("CreateAgent returned error: %v", err)
	}
	actor, err := resolveMCPActor(ctx, store, created.Token)
	if err != nil {
		t.Fatalf("resolve valid token returned error: %v", err)
	}
	if actor.ID != created.Agent.ID || actor.Kind != "agent" || actor.Name != "Codex MCP" {
		t.Fatalf("unexpected resolved actor: %+v", actor)
	}
}

func openAppTestStore(t *testing.T) *storage.Store {
	t.Helper()

	store, err := storage.Open(context.Background(), ":memory:")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Fatalf("close store: %v", err)
		}
	})
	if err := store.Init(context.Background()); err != nil {
		t.Fatalf("init store: %v", err)
	}
	return store
}
