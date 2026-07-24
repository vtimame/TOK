package app

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"s26.sh/tok/internal/storage"
)

func (c *CLI) runAgent(ctx context.Context, opts runtimeOptions) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if len(opts.args) < 2 {
		return &UsageError{
			Message: fmt.Sprintf("missing agent command\n\nRun '%s help' for usage.", commandName),
			Code:    2,
		}
	}

	_, _, store, err := c.runtimeStore(ctx, opts)
	if err != nil {
		return err
	}
	defer store.Close()

	switch opts.args[1] {
	case "add":
		return c.runAgentAdd(ctx, store, opts.args[2:])
	case "list":
		return c.runAgentList(ctx, store, opts.args[2:])
	case "revoke":
		return c.runAgentRevoke(ctx, store, opts.args[2:])
	default:
		return &UsageError{
			Message: fmt.Sprintf("unknown agent command %q\n\nRun '%s help' for usage.", opts.args[1], commandName),
			Code:    2,
		}
	}
}

func (c *CLI) runAgentAdd(ctx context.Context, store *storage.Store, args []string) error {
	if len(args) != 1 {
		return &UsageError{Message: "agent add requires an agent name", Code: 2}
	}

	created, err := store.CreateAgent(ctx, args[0])
	if err != nil {
		return err
	}

	printAgent(c.out, created.Agent)
	fmt.Fprintf(c.out, "token: %s\n", created.Token)
	return nil
}

func (c *CLI) runAgentList(ctx context.Context, store *storage.Store, args []string) error {
	if len(args) > 0 {
		return &UsageError{Message: "agent list does not accept arguments", Code: 2}
	}

	agents, err := store.ListAgents(ctx)
	if err != nil {
		return err
	}
	if len(agents) == 0 {
		fmt.Fprintln(c.out, "no agents")
		return nil
	}

	rows := [][]string{{"id", "status", "name", "created_at", "revoked_at"}}
	for _, agent := range agents {
		status := "active"
		if agent.TokenRevokedAt != "" {
			status = "revoked"
		}
		rows = append(rows, []string{strconv.FormatInt(agent.ID, 10), status, agent.Name, agent.CreatedAt, agent.TokenRevokedAt})
	}
	return printTerminalTable(c.out, rows)
}

func (c *CLI) runAgentRevoke(ctx context.Context, store *storage.Store, args []string) error {
	if len(args) != 1 {
		return &UsageError{Message: "agent revoke requires an agent id", Code: 2}
	}
	id, err := strconv.ParseInt(strings.TrimSpace(args[0]), 10, 64)
	if err != nil || id <= 0 {
		return &UsageError{Message: "agent revoke requires a positive integer agent id", Code: 2}
	}

	agent, err := store.RevokeAgent(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("agent not found: %d", id)
		}
		return err
	}

	printAgent(c.out, agent)
	return nil
}

func printAgent(out interface {
	Write([]byte) (int, error)
}, agent storage.Actor) {
	status := "active"
	if agent.TokenRevokedAt != "" {
		status = "revoked"
	}
	fmt.Fprintf(out, "id: %d\n", agent.ID)
	fmt.Fprintf(out, "name: %s\n", agent.Name)
	fmt.Fprintf(out, "status: %s\n", status)
	fmt.Fprintf(out, "created_at: %s\n", agent.CreatedAt)
	fmt.Fprintf(out, "updated_at: %s\n", agent.UpdatedAt)
	if agent.TokenRevokedAt != "" {
		fmt.Fprintf(out, "revoked_at: %s\n", agent.TokenRevokedAt)
	}
}
