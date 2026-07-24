package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"s26.sh/tok/internal/storage"
)

type agentOptions struct {
	name string
	id   int64
	json bool
}

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
	addOpts, err := parseAgentAddOptions(args)
	if err != nil {
		return err
	}

	created, err := store.CreateAgent(ctx, addOpts.name)
	if err != nil {
		return err
	}

	if addOpts.json {
		return printCreatedAgentJSON(c.out, created)
	}
	printAgent(c.out, created.Agent)
	fmt.Fprintf(c.out, "token: %s\n", created.Token)
	return nil
}

func (c *CLI) runAgentList(ctx context.Context, store *storage.Store, args []string) error {
	jsonOutput, err := parseNoArgJSONOption(args, "agent list")
	if err != nil {
		return err
	}

	agents, err := store.ListAgents(ctx)
	if err != nil {
		return err
	}
	if jsonOutput {
		return printAgentsJSON(c.out, agents)
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
	revokeOpts, err := parseAgentRevokeOptions(args)
	if err != nil {
		return err
	}

	agent, err := store.RevokeAgent(ctx, revokeOpts.id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("agent not found: %d", revokeOpts.id)
		}
		return err
	}

	if revokeOpts.json {
		return printAgentJSON(c.out, agent)
	}
	printAgent(c.out, agent)
	return nil
}

func parseAgentAddOptions(args []string) (agentOptions, error) {
	var opts agentOptions
	for _, arg := range args {
		switch {
		case arg == "--json":
			opts.json = true
		case strings.HasPrefix(arg, "-"):
			return agentOptions{}, &UsageError{Message: fmt.Sprintf("unknown agent add option %q", arg), Code: 2}
		default:
			if opts.name != "" {
				return agentOptions{}, &UsageError{Message: "agent add accepts exactly one agent name", Code: 2}
			}
			opts.name = strings.TrimSpace(arg)
		}
	}
	if opts.name == "" {
		return agentOptions{}, &UsageError{Message: "agent add requires an agent name", Code: 2}
	}
	return opts, nil
}

func parseAgentRevokeOptions(args []string) (agentOptions, error) {
	var opts agentOptions
	for _, arg := range args {
		switch {
		case arg == "--json":
			opts.json = true
		case strings.HasPrefix(arg, "-"):
			return agentOptions{}, &UsageError{Message: fmt.Sprintf("unknown agent revoke option %q", arg), Code: 2}
		default:
			if opts.id != 0 {
				return agentOptions{}, &UsageError{Message: "agent revoke accepts exactly one agent id", Code: 2}
			}
			id, err := strconv.ParseInt(strings.TrimSpace(arg), 10, 64)
			if err != nil || id <= 0 {
				return agentOptions{}, &UsageError{Message: "agent revoke requires a positive integer agent id", Code: 2}
			}
			opts.id = id
		}
	}
	if opts.id == 0 {
		return agentOptions{}, &UsageError{Message: "agent revoke requires an agent id", Code: 2}
	}
	return opts, nil
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

type agentCLIOutput struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	Status    string `json:"status"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
	RevokedAt string `json:"revoked_at,omitempty"`
}

type createdAgentCLIOutput struct {
	Agent agentCLIOutput `json:"agent"`
	Token string         `json:"token"`
}

func printAgentJSON(out io.Writer, agent storage.Actor) error {
	encoder := json.NewEncoder(out)
	encoder.SetIndent("", "  ")
	return encoder.Encode(agentCLIOutputFromStorage(agent))
}

func printAgentsJSON(out io.Writer, agents []storage.Actor) error {
	output := make([]agentCLIOutput, 0, len(agents))
	for _, agent := range agents {
		output = append(output, agentCLIOutputFromStorage(agent))
	}
	encoder := json.NewEncoder(out)
	encoder.SetIndent("", "  ")
	return encoder.Encode(output)
}

func printCreatedAgentJSON(out io.Writer, created storage.AgentWithToken) error {
	encoder := json.NewEncoder(out)
	encoder.SetIndent("", "  ")
	return encoder.Encode(createdAgentCLIOutput{
		Agent: agentCLIOutputFromStorage(created.Agent),
		Token: created.Token,
	})
}

func agentCLIOutputFromStorage(agent storage.Actor) agentCLIOutput {
	status := "active"
	if agent.TokenRevokedAt != "" {
		status = "revoked"
	}
	return agentCLIOutput{
		ID:        agent.ID,
		Name:      agent.Name,
		Status:    status,
		CreatedAt: agent.CreatedAt,
		UpdatedAt: agent.UpdatedAt,
		RevokedAt: agent.TokenRevokedAt,
	}
}
