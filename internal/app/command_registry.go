package app

import (
	"fmt"
	"strings"
)

type commandSpec struct {
	Name        string
	Summary     string
	Usage       string
	Flags       []flagSpec
	Children    []*commandSpec
	ValuesByKey map[string][]string
	Hidden      bool
}

type flagSpec struct {
	Name         string
	ValueName    string
	Summary      string
	Completes    string
	ValueChoices []string
}

func tokCommandSpec() *commandSpec {
	return &commandSpec{
		Name:    commandName,
		Summary: "Task Operations Kernel",
		Usage:   "tok [--config <path>] [--log-level <level>] <command>",
		Flags: []flagSpec{
			{Name: "--config", ValueName: "<path>", Summary: "Use an explicit config file"},
			{Name: "--log-level", ValueName: "<level>", Summary: "Override configured log level", ValueChoices: []string{"debug", "info", "warn", "error"}},
		},
		Children: []*commandSpec{
			{Name: "version", Summary: "Print build version information", Usage: "tok version"},
			{Name: "init", Summary: "Initialize local runtime storage", Usage: "tok init"},
			{
				Name:    "config",
				Summary: "Inspect runtime configuration",
				Usage:   "tok config <command>",
				Children: []*commandSpec{
					{Name: "paths", Summary: "Print resolved runtime paths", Usage: "tok config paths"},
				},
			},
			{
				Name:    "project",
				Summary: "Register and inspect local projects",
				Usage:   "tok project <command>",
				Children: []*commandSpec{
					{Name: "add", Summary: "Register a local project", Usage: "tok project add <path> --name <name> [--display-name <name>] [--json]", Flags: []flagSpec{
						{Name: "--name", ValueName: "<name>", Summary: "Stable project name"},
						{Name: "--display-name", ValueName: "<name>", Summary: "Human-readable project name"},
						{Name: "--json", Summary: "Print JSON output"},
					}},
					{Name: "list", Summary: "List registered projects", Usage: "tok project list [--json]", Flags: []flagSpec{
						{Name: "--json", Summary: "Print JSON output"},
					}},
					{Name: "show", Summary: "Show a registered project", Usage: "tok project show <name> [--json]", Flags: []flagSpec{
						{Name: "--json", Summary: "Print JSON output"},
					}},
				},
			},
			{
				Name:    "task",
				Summary: "Create and update project tasks",
				Usage:   "tok task <command>",
				Children: []*commandSpec{
					{Name: "create", Summary: "Create a task", Usage: "tok task create --project <name> --title <title> [--description <text>] [--acceptance-criteria <text>] [--notes <text>]", Flags: []flagSpec{
						projectFlag(),
						{Name: "--title", ValueName: "<title>", Summary: "Task title"},
						{Name: "--description", ValueName: "<text>", Summary: "Task description"},
						{Name: "--acceptance-criteria", ValueName: "<text>", Summary: "Acceptance criteria"},
						{Name: "--notes", ValueName: "<text>", Summary: "Initial notes"},
					}},
					{Name: "list", Summary: "List project tasks", Usage: "tok task list --project <name> [--status <status>] [--json]", Flags: []flagSpec{
						projectFlag(),
						{Name: "--status", ValueName: "<status>", Summary: "Filter by task status", ValueChoices: taskStatusChoices()},
						{Name: "--json", Summary: "Print JSON output"},
					}},
					{Name: "show", Summary: "Show task details and events", Usage: "tok task show <task-id> [--json]", Flags: []flagSpec{{Name: "--json", Summary: "Print JSON output"}}},
					{Name: "status", Summary: "Set task status directly", Usage: "tok task status <task-id> <status>", ValuesByKey: map[string][]string{"status": taskStatusChoices()}},
					{Name: "done", Summary: "Complete an in-progress task", Usage: "tok task done <task-id> --note <text>", Flags: []flagSpec{{Name: "--note", ValueName: "<text>", Summary: "Completion note"}}},
					{Name: "comment", Summary: "Add a task comment", Usage: "tok task comment <task-id> --body <text>", Flags: []flagSpec{{Name: "--body", ValueName: "<text>", Summary: "Comment body"}}},
					{Name: "progress", Summary: "Record task progress", Usage: "tok task progress <task-id> --body <text>", Flags: []flagSpec{{Name: "--body", ValueName: "<text>", Summary: "Progress note"}}},
					{Name: "block", Summary: "Block a task", Usage: "tok task block <task-id> --reason <text>", Flags: []flagSpec{{Name: "--reason", ValueName: "<text>", Summary: "Blocker reason"}}},
					{Name: "unblock", Summary: "Unblock a task", Usage: "tok task unblock <task-id> --note <text>", Flags: []flagSpec{{Name: "--note", ValueName: "<text>", Summary: "Unblock note"}}},
					{
						Name:    "dependency",
						Summary: "Manage task dependencies",
						Usage:   "tok task dependency <command>",
						Children: []*commandSpec{
							{Name: "add", Summary: "Add a task dependency", Usage: "tok task dependency add [--type blocks] <blocker-task-id> <blocked-task-id>", Flags: []flagSpec{{Name: "--type", ValueName: "<type>", Summary: "Dependency type", ValueChoices: []string{"blocks"}}}},
							{Name: "remove", Summary: "Remove a task dependency", Usage: "tok task dependency remove [--type blocks] <blocker-task-id> <blocked-task-id>", Flags: []flagSpec{{Name: "--type", ValueName: "<type>", Summary: "Dependency type", ValueChoices: []string{"blocks"}}}},
						},
					},
					{Name: "ready", Summary: "List tasks ready to claim", Usage: "tok task ready --project <name> [--json]", Flags: []flagSpec{projectFlag(), {Name: "--json", Summary: "Print JSON output"}}},
					{Name: "claim", Summary: "Claim the next ready task or a specific task", Usage: "tok task claim --project <name> [task-id] [--json]", Flags: []flagSpec{projectFlag(), {Name: "--json", Summary: "Print JSON output"}}},
				},
			},
			{
				Name:    "user",
				Summary: "Inspect and set the local user profile",
				Usage:   "tok user <command>",
				Children: []*commandSpec{
					{Name: "show", Summary: "Show resolved local user", Usage: "tok user show"},
					{Name: "set-name", Summary: "Set local display name", Usage: "tok user set-name <display-name>"},
				},
			},
			{
				Name:    "agent",
				Summary: "Manage local agent identities and tokens",
				Usage:   "tok agent <command>",
				Children: []*commandSpec{
					{Name: "add", Summary: "Create an agent identity", Usage: "tok agent add <name>"},
					{Name: "list", Summary: "List agent identities", Usage: "tok agent list"},
					{Name: "revoke", Summary: "Revoke an agent token", Usage: "tok agent revoke <agent-id>"},
				},
			},
			{
				Name:    "mcp",
				Summary: "Serve TOK tools over MCP",
				Usage:   "tok mcp <command>",
				Children: []*commandSpec{
					{Name: "serve", Summary: "Run the MCP stdio server", Usage: "tok mcp serve [--token <token>]", Flags: []flagSpec{{Name: "--token", ValueName: "<token>", Summary: "Agent token; defaults to TOK_AGENT_TOKEN"}}},
				},
			},
			{
				Name:    "ui",
				Summary: "Serve the local UI API",
				Usage:   "tok ui <command>",
				Children: []*commandSpec{
					{Name: "serve", Summary: "Serve the local HTTP API", Usage: "tok ui serve [--addr <host:port>]", Flags: []flagSpec{{Name: "--addr", ValueName: "<host:port>", Summary: "Listen address"}}},
					{Name: "openapi", Summary: "Print or write OpenAPI schema", Usage: "tok ui openapi [--out <path>]", Flags: []flagSpec{{Name: "--out", ValueName: "<path>", Summary: "Write schema to path"}}},
				},
			},
			{
				Name:    "index",
				Summary: "Update local retrieval indexes",
				Usage:   "tok index <command>",
				Children: []*commandSpec{
					{Name: "update", Summary: "Update one or all project indexes", Usage: "tok index update (--project <name> | --all) [--json]", Flags: []flagSpec{projectFlag(), {Name: "--all", Summary: "Update all projects"}, {Name: "--json", Summary: "Print JSON output"}}},
					{Name: "status", Summary: "Show one or all project index states", Usage: "tok index status (--project <name> | --all) [--json]", Flags: []flagSpec{projectFlag(), {Name: "--all", Summary: "Show all projects"}, {Name: "--json", Summary: "Print JSON output"}}},
					{
						Name:    "ignore",
						Summary: "Inspect and edit project index ignore policy",
						Usage:   "tok index ignore <command>",
						Children: []*commandSpec{
							{Name: "list", Summary: "List project ignore patterns", Usage: "tok index ignore list --project <name> [--json]", Flags: []flagSpec{projectFlag(), {Name: "--json", Summary: "Print JSON output"}}},
							{Name: "refresh", Summary: "Refresh policy from project .gitignore", Usage: "tok index ignore refresh --project <name> [--json]", Flags: []flagSpec{projectFlag(), {Name: "--json", Summary: "Print JSON output"}}},
							{Name: "add", Summary: "Add one ignore pattern", Usage: "tok index ignore add --project <name> [--json] <pattern>", Flags: []flagSpec{projectFlag(), {Name: "--json", Summary: "Print JSON output"}}},
							{Name: "remove", Summary: "Remove one ignore pattern", Usage: "tok index ignore remove --project <name> [--json] <pattern>", Flags: []flagSpec{projectFlag(), {Name: "--json", Summary: "Print JSON output"}}},
						},
					},
				},
			},
			{Name: "search", Summary: "Search indexed project files", Usage: "tok search --project <name> [--limit <n>] [--json] <query>", Flags: []flagSpec{projectFlag(), {Name: "--limit", ValueName: "<n>", Summary: "Maximum result count"}, {Name: "--json", Summary: "Print JSON output"}}},
			{
				Name:    "context",
				Summary: "Build compact task context packages",
				Usage:   "tok context <command>",
				Children: []*commandSpec{
					{Name: "build", Summary: "Build a task context package", Usage: "tok context build --project <name> --task <task-id> [--limit <n>] [--query <text>] [--output <path>] [--json]", Flags: []flagSpec{
						projectFlag(),
						{Name: "--task", ValueName: "<task-id>", Summary: "Task id"},
						{Name: "--limit", ValueName: "<n>", Summary: "Maximum retrieval result count"},
						{Name: "--query", ValueName: "<text>", Summary: "Retrieval query override"},
						{Name: "--output", ValueName: "<path>", Summary: "Write package to path"},
						{Name: "--json", Summary: "Print JSON output"},
					}},
				},
			},
			{
				Name:    "run",
				Summary: "Record agent run attempts",
				Usage:   "tok run <command>",
				Children: []*commandSpec{
					{Name: "start", Summary: "Start a run for a task", Usage: "tok run start --task <task-id> [--limit <n>] [--handoff-output <path>] [--json]", Flags: []flagSpec{
						{Name: "--task", ValueName: "<task-id>", Summary: "Task id"},
						{Name: "--limit", ValueName: "<n>", Summary: "Maximum retrieval result count"},
						{Name: "--handoff-output", ValueName: "<path>", Summary: "Write handoff context package"},
						{Name: "--json", Summary: "Print JSON output"},
					}},
					{Name: "show", Summary: "Show a run", Usage: "tok run show <run-id> [--json]", Flags: []flagSpec{{Name: "--json", Summary: "Print JSON output"}}},
					{Name: "record-validation", Summary: "Attach validation evidence to a run", Usage: "tok run record-validation <run-id> --command <cmd> --status <passed|failed> --summary <text> [--json]", Flags: []flagSpec{
						{Name: "--command", ValueName: "<cmd>", Summary: "Validation command"},
						{Name: "--status", ValueName: "<passed|failed>", Summary: "Validation status", ValueChoices: []string{"passed", "failed"}},
						{Name: "--summary", ValueName: "<text>", Summary: "Validation summary"},
						{Name: "--json", Summary: "Print JSON output"},
					}},
					{Name: "finish", Summary: "Finish a run", Usage: "tok run finish <run-id> --status <status> --summary <text> [--json]", Flags: []flagSpec{
						{Name: "--status", ValueName: "<status>", Summary: "Final run status", ValueChoices: []string{"succeeded", "failed"}},
						{Name: "--summary", ValueName: "<text>", Summary: "Result summary"},
						{Name: "--json", Summary: "Print JSON output"},
					}},
				},
			},
			{Name: "completion", Summary: "Print shell completion script", Usage: "tok completion <bash|zsh|fish>"},
			{Name: "help", Summary: "Show help for a command", Usage: "tok help [command] [subcommand]"},
		},
	}
}

func projectFlag() flagSpec {
	return flagSpec{Name: "--project", ValueName: "<name>", Summary: "Project name", Completes: "project"}
}

func taskStatusChoices() []string {
	return []string{"open", "in_progress", "blocked", "done"}
}

func findCommandSpec(path []string) (*commandSpec, bool) {
	spec := tokCommandSpec()
	if len(path) == 0 {
		return spec, true
	}
	for _, name := range path {
		child := spec.child(name)
		if child == nil {
			return nil, false
		}
		spec = child
	}
	return spec, true
}

func (s *commandSpec) child(name string) *commandSpec {
	for _, child := range s.Children {
		if child.Name == name {
			return child
		}
	}
	return nil
}

func (s *commandSpec) visibleChildren() []*commandSpec {
	children := make([]*commandSpec, 0, len(s.Children))
	for _, child := range s.Children {
		if !child.Hidden {
			children = append(children, child)
		}
	}
	return children
}

func formatCommandHelp(spec *commandSpec) string {
	var b strings.Builder
	if spec.Name == commandName {
		fmt.Fprintf(&b, "TOK - %s\n\n", spec.Summary)
	} else {
		fmt.Fprintf(&b, "%s - %s\n\n", spec.Name, spec.Summary)
	}
	fmt.Fprintf(&b, "Usage:\n  %s\n", spec.Usage)

	if children := spec.visibleChildren(); len(children) > 0 {
		fmt.Fprint(&b, "\nCommands:\n")
		for _, child := range children {
			fmt.Fprintf(&b, "  %-18s %s\n", child.Name, child.Summary)
		}
	}

	if len(spec.Flags) > 0 {
		fmt.Fprint(&b, "\nFlags:\n")
		for _, flag := range spec.Flags {
			name := flag.Name
			if flag.ValueName != "" {
				name += " " + flag.ValueName
			}
			fmt.Fprintf(&b, "  %-24s %s\n", name, flag.Summary)
		}
	}

	return strings.TrimRight(b.String(), "\n") + "\n"
}

func helpRequestPath(args []string) ([]string, bool, error) {
	if len(args) == 0 {
		return nil, true, nil
	}
	switch args[0] {
	case "help":
		return args[1:], true, nil
	case "-h", "--help":
		if len(args) > 1 {
			return nil, true, &UsageError{Message: fmt.Sprintf("%s %s does not accept arguments", commandName, args[0]), Code: 2}
		}
		return nil, true, nil
	}
	for i, arg := range args {
		if arg == "-h" || arg == "--help" {
			return args[:i], true, nil
		}
	}
	return nil, false, nil
}
