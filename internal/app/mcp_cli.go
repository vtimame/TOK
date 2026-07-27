package app

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"s26.sh/tok/internal/mcpserver"
	"s26.sh/tok/internal/storage"
)

const agentTokenEnv = "TOK_AGENT_TOKEN"

type mcpServeOptions struct {
	token   string
	profile mcpserver.Profile
}

func (c *CLI) runMCP(ctx context.Context, opts runtimeOptions) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if len(opts.args) < 2 {
		return &UsageError{
			Message: fmt.Sprintf("missing mcp command\n\nRun '%s help' for usage.", commandName),
			Code:    2,
		}
	}

	switch opts.args[1] {
	case "serve":
		return c.runMCPServe(ctx, opts)
	default:
		return &UsageError{
			Message: fmt.Sprintf("unknown mcp command %q\n\nRun '%s help' for usage.", opts.args[1], commandName),
			Code:    2,
		}
	}
}

func (c *CLI) runMCPServe(ctx context.Context, opts runtimeOptions) error {
	serveOpts, err := parseMCPServeOptions(opts.args[2:])
	if err != nil {
		return err
	}

	_, _, store, err := c.runtimeStore(ctx, opts)
	if err != nil {
		return err
	}
	defer store.Close()

	actor, err := resolveMCPActor(ctx, store, serveOpts.token)
	if err != nil {
		return err
	}

	server, err := mcpserver.New(mcpserver.Config{
		Store:   store,
		Actor:   actor,
		Version: c.version.Version,
		Profile: serveOpts.profile,
	})
	if err != nil {
		return err
	}

	return server.Run(ctx, &mcp.StdioTransport{})
}

func parseMCPServeOptions(args []string) (mcpServeOptions, error) {
	var opts mcpServeOptions
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--token":
			i++
			if i >= len(args) {
				return mcpServeOptions{}, &UsageError{Message: "--token requires a value", Code: 2}
			}
			opts.token = args[i]
		case strings.HasPrefix(arg, "--token="):
			opts.token = strings.TrimPrefix(arg, "--token=")
			if opts.token == "" {
				return mcpServeOptions{}, &UsageError{Message: "--token requires a value", Code: 2}
			}
		case arg == "--profile":
			i++
			if i >= len(args) {
				return mcpServeOptions{}, &UsageError{Message: "--profile requires a value", Code: 2}
			}
			profile, err := mcpserver.NormalizeProfile(mcpserver.Profile(args[i]))
			if err != nil {
				return mcpServeOptions{}, &UsageError{Message: err.Error(), Code: 2}
			}
			opts.profile = profile
		case strings.HasPrefix(arg, "--profile="):
			raw := strings.TrimPrefix(arg, "--profile=")
			if raw == "" {
				return mcpServeOptions{}, &UsageError{Message: "--profile requires a value", Code: 2}
			}
			profile, err := mcpserver.NormalizeProfile(mcpserver.Profile(raw))
			if err != nil {
				return mcpServeOptions{}, &UsageError{Message: err.Error(), Code: 2}
			}
			opts.profile = profile
		default:
			return mcpServeOptions{}, &UsageError{Message: fmt.Sprintf("unknown mcp serve option %q", arg), Code: 2}
		}
	}

	opts.token = strings.TrimSpace(opts.token)
	if opts.token == "" {
		opts.token = strings.TrimSpace(os.Getenv(agentTokenEnv))
	}
	return opts, nil
}

func resolveMCPActor(ctx context.Context, store *storage.Store, token string) (storage.ActorRef, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return storage.ActorRef{}, &UsageError{Message: "mcp serve requires an agent token via --token or TOK_AGENT_TOKEN", Code: 2}
	}

	agent, err := store.ResolveAgentByToken(ctx, token)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return storage.ActorRef{}, fmt.Errorf("invalid agent token")
		}
		return storage.ActorRef{}, err
	}
	return storage.ActorRefFromActor(agent), nil
}
