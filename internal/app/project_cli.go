package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	projectpkg "s26.sh/tok/internal/project"
	"s26.sh/tok/internal/storage"
)

func (c *CLI) runProject(ctx context.Context, opts runtimeOptions) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if len(opts.args) < 2 {
		return &UsageError{
			Message: fmt.Sprintf("missing project command\n\nRun '%s help' for usage.", commandName),
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
		return c.runProjectAdd(ctx, store, opts.args[2:])
	case "list":
		return c.runProjectList(ctx, store, opts.args[2:])
	case "show":
		return c.runProjectShow(ctx, store, opts.args[2:])
	case "instruction":
		return c.runProjectInstruction(ctx, store, opts.args[2:])
	default:
		return &UsageError{
			Message: fmt.Sprintf("unknown project command %q\n\nRun '%s help' for usage.", opts.args[1], commandName),
			Code:    2,
		}
	}
}

type projectAddOptions struct {
	path        string
	name        string
	displayName string
	json        bool
}

type projectListOptions struct {
	json bool
}

type projectShowOptions struct {
	name string
	json bool
}

type projectInstructionOptions struct {
	projectName     string
	id              int64
	title           string
	body            string
	priority        string
	json            bool
	includeDisabled bool
}

func (c *CLI) runProjectAdd(ctx context.Context, store *storage.Store, args []string) error {
	addOpts, err := parseProjectAddOptions(args)
	if err != nil {
		return err
	}

	projectPath, err := validateProjectPath(addOpts.path)
	if err != nil {
		return err
	}
	displayName := addOpts.displayName
	if displayName == "" {
		displayName = addOpts.name
	}

	project, err := store.CreateProject(ctx, storage.CreateProjectInput{
		Name:        addOpts.name,
		DisplayName: displayName,
		Path:        projectPath,
	})
	if err != nil {
		return err
	}

	if addOpts.json {
		return printProjectJSON(c.out, project)
	}

	printProject(c.out, project)
	return nil
}

func (c *CLI) runProjectList(ctx context.Context, store *storage.Store, args []string) error {
	listOpts, err := parseProjectListOptions(args)
	if err != nil {
		return err
	}

	projects, err := store.ListProjects(ctx)
	if err != nil {
		return err
	}
	if listOpts.json {
		return printProjectsJSON(c.out, projects)
	}

	if len(projects) == 0 {
		fmt.Fprintln(c.out, "no projects")
		return nil
	}

	rows := [][]string{{"name", "display_name", "path"}}
	for _, project := range projects {
		rows = append(rows, []string{project.Name, project.DisplayName, project.Path})
	}
	return printTerminalTable(c.out, rows)
}

func (c *CLI) runProjectShow(ctx context.Context, store *storage.Store, args []string) error {
	showOpts, err := parseProjectShowOptions(args)
	if err != nil {
		return err
	}

	project, err := store.GetProject(ctx, showOpts.name)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("project not found: %s", showOpts.name)
		}
		return err
	}

	if showOpts.json {
		return printProjectJSON(c.out, project)
	}

	printProject(c.out, project)
	return nil
}

func (c *CLI) runProjectInstruction(ctx context.Context, store *storage.Store, args []string) error {
	if len(args) < 1 {
		return &UsageError{Message: "missing project instruction command", Code: 2}
	}

	command := args[0]
	instructionOpts, err := parseProjectInstructionOptions(args[1:], "project instruction "+command, command)
	if err != nil {
		return err
	}
	project, err := getProjectForTask(ctx, store, instructionOpts.projectName)
	if err != nil {
		return err
	}

	switch command {
	case "add":
		instruction, err := store.CreateProjectInstruction(ctx, storage.CreateProjectInstructionInput{
			ProjectID: project.ID,
			Title:     instructionOpts.title,
			Body:      instructionOpts.body,
			Priority:  instructionOpts.priority,
		})
		if err != nil {
			return err
		}
		if instructionOpts.json {
			return printProjectInstructionJSON(c.out, instruction)
		}
		printProjectInstruction(c.out, instruction)
	case "list":
		instructions, err := store.ListProjectInstructions(ctx, storage.ListProjectInstructionsOptions{
			ProjectID:       project.ID,
			IncludeDisabled: instructionOpts.includeDisabled,
		})
		if err != nil {
			return err
		}
		if instructionOpts.json {
			return printProjectInstructionsJSON(c.out, instructions)
		}
		printProjectInstructions(c.out, instructions)
	case "show":
		instruction, err := store.GetProjectInstruction(ctx, project.ID, instructionOpts.id)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return fmt.Errorf("project instruction not found: %d", instructionOpts.id)
			}
			return err
		}
		if instructionOpts.json {
			return printProjectInstructionJSON(c.out, instruction)
		}
		printProjectInstruction(c.out, instruction)
	case "enable", "disable":
		instruction, err := store.SetProjectInstructionEnabled(ctx, project.ID, instructionOpts.id, command == "enable")
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return fmt.Errorf("project instruction not found: %d", instructionOpts.id)
			}
			return err
		}
		if instructionOpts.json {
			return printProjectInstructionJSON(c.out, instruction)
		}
		printProjectInstruction(c.out, instruction)
	case "remove":
		if err := store.DeleteProjectInstruction(ctx, project.ID, instructionOpts.id); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return fmt.Errorf("project instruction not found: %d", instructionOpts.id)
			}
			return err
		}
		if instructionOpts.json {
			return printProjectInstructionRemovedJSON(c.out, instructionOpts.id)
		}
		fmt.Fprintf(c.out, "removed project instruction: %d\n", instructionOpts.id)
	default:
		return &UsageError{Message: fmt.Sprintf("unknown project instruction command %q", command), Code: 2}
	}

	return nil
}

func parseProjectAddOptions(args []string) (projectAddOptions, error) {
	var opts projectAddOptions

	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--name":
			i++
			if i >= len(args) {
				return projectAddOptions{}, &UsageError{Message: "--name requires a value", Code: 2}
			}
			opts.name = args[i]
		case strings.HasPrefix(arg, "--name="):
			opts.name = strings.TrimPrefix(arg, "--name=")
			if opts.name == "" {
				return projectAddOptions{}, &UsageError{Message: "--name requires a value", Code: 2}
			}
		case arg == "--display-name":
			i++
			if i >= len(args) {
				return projectAddOptions{}, &UsageError{Message: "--display-name requires a value", Code: 2}
			}
			opts.displayName = args[i]
		case strings.HasPrefix(arg, "--display-name="):
			opts.displayName = strings.TrimPrefix(arg, "--display-name=")
			if opts.displayName == "" {
				return projectAddOptions{}, &UsageError{Message: "--display-name requires a value", Code: 2}
			}
		case arg == "--json":
			opts.json = true
		case strings.HasPrefix(arg, "-"):
			return projectAddOptions{}, &UsageError{Message: fmt.Sprintf("unknown project add option %q", arg), Code: 2}
		default:
			if opts.path != "" {
				return projectAddOptions{}, &UsageError{Message: "project add accepts exactly one path", Code: 2}
			}
			opts.path = arg
		}
	}

	if opts.path == "" {
		return projectAddOptions{}, &UsageError{Message: "project add requires a path", Code: 2}
	}
	if strings.TrimSpace(opts.name) == "" {
		return projectAddOptions{}, &UsageError{Message: "project add requires --name", Code: 2}
	}
	if strings.TrimSpace(opts.displayName) != opts.displayName {
		return projectAddOptions{}, &UsageError{Message: "project display name cannot have leading or trailing spaces", Code: 2}
	}
	opts.name = strings.TrimSpace(opts.name)

	return opts, nil
}

func parseProjectListOptions(args []string) (projectListOptions, error) {
	var opts projectListOptions
	for _, arg := range args {
		switch arg {
		case "--json":
			opts.json = true
		default:
			return projectListOptions{}, &UsageError{Message: fmt.Sprintf("unknown project list option %q", arg), Code: 2}
		}
	}
	return opts, nil
}

func parseProjectShowOptions(args []string) (projectShowOptions, error) {
	var opts projectShowOptions
	for _, arg := range args {
		switch {
		case arg == "--json":
			opts.json = true
		case strings.HasPrefix(arg, "-"):
			return projectShowOptions{}, &UsageError{Message: fmt.Sprintf("unknown project show option %q", arg), Code: 2}
		default:
			if opts.name != "" {
				return projectShowOptions{}, &UsageError{Message: "project show accepts exactly one project name", Code: 2}
			}
			opts.name = arg
		}
	}

	opts.name = strings.TrimSpace(opts.name)
	if opts.name == "" {
		return projectShowOptions{}, &UsageError{Message: "project show requires a project name", Code: 2}
	}
	return opts, nil
}

func parseProjectInstructionOptions(args []string, command, action string) (projectInstructionOptions, error) {
	var opts projectInstructionOptions

	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--project":
			i++
			if i >= len(args) {
				return projectInstructionOptions{}, &UsageError{Message: "--project requires a value", Code: 2}
			}
			opts.projectName = args[i]
		case strings.HasPrefix(arg, "--project="):
			opts.projectName = strings.TrimPrefix(arg, "--project=")
			if opts.projectName == "" {
				return projectInstructionOptions{}, &UsageError{Message: "--project requires a value", Code: 2}
			}
		case arg == "--title":
			i++
			if i >= len(args) {
				return projectInstructionOptions{}, &UsageError{Message: "--title requires a value", Code: 2}
			}
			opts.title = args[i]
		case strings.HasPrefix(arg, "--title="):
			opts.title = strings.TrimPrefix(arg, "--title=")
			if opts.title == "" {
				return projectInstructionOptions{}, &UsageError{Message: "--title requires a value", Code: 2}
			}
		case arg == "--body":
			i++
			if i >= len(args) {
				return projectInstructionOptions{}, &UsageError{Message: "--body requires a value", Code: 2}
			}
			opts.body = args[i]
		case strings.HasPrefix(arg, "--body="):
			opts.body = strings.TrimPrefix(arg, "--body=")
			if opts.body == "" {
				return projectInstructionOptions{}, &UsageError{Message: "--body requires a value", Code: 2}
			}
		case arg == "--priority":
			i++
			if i >= len(args) {
				return projectInstructionOptions{}, &UsageError{Message: "--priority requires a value", Code: 2}
			}
			opts.priority = args[i]
		case strings.HasPrefix(arg, "--priority="):
			opts.priority = strings.TrimPrefix(arg, "--priority=")
			if opts.priority == "" {
				return projectInstructionOptions{}, &UsageError{Message: "--priority requires a value", Code: 2}
			}
		case arg == "--include-disabled":
			opts.includeDisabled = true
		case arg == "--json":
			opts.json = true
		case strings.HasPrefix(arg, "-"):
			return projectInstructionOptions{}, &UsageError{Message: fmt.Sprintf("unknown %s option %q", command, arg), Code: 2}
		default:
			if opts.id != 0 {
				return projectInstructionOptions{}, &UsageError{Message: command + " accepts exactly one instruction id", Code: 2}
			}
			id, err := parseInstructionID(arg)
			if err != nil {
				return projectInstructionOptions{}, err
			}
			opts.id = id
		}
	}

	opts.projectName = strings.TrimSpace(opts.projectName)
	opts.title = strings.TrimSpace(opts.title)
	opts.body = strings.TrimSpace(opts.body)
	opts.priority = strings.TrimSpace(opts.priority)
	if opts.projectName == "" {
		return projectInstructionOptions{}, &UsageError{Message: command + " requires --project", Code: 2}
	}

	switch action {
	case "add":
		if opts.title == "" {
			return projectInstructionOptions{}, &UsageError{Message: command + " requires --title", Code: 2}
		}
		if opts.body == "" {
			return projectInstructionOptions{}, &UsageError{Message: command + " requires --body", Code: 2}
		}
		if opts.id != 0 {
			return projectInstructionOptions{}, &UsageError{Message: command + " does not accept an instruction id", Code: 2}
		}
	case "list":
		if opts.id != 0 || opts.title != "" || opts.body != "" || opts.priority != "" {
			return projectInstructionOptions{}, &UsageError{Message: command + " accepts only --project, --include-disabled, and --json", Code: 2}
		}
	case "show", "enable", "disable", "remove":
		if opts.id == 0 {
			return projectInstructionOptions{}, &UsageError{Message: command + " requires an instruction id", Code: 2}
		}
		if opts.title != "" || opts.body != "" || opts.priority != "" || opts.includeDisabled {
			return projectInstructionOptions{}, &UsageError{Message: command + " accepts only --project, --json, and instruction id", Code: 2}
		}
	}

	return opts, nil
}

func parseInstructionID(value string) (int64, error) {
	id, err := parseTaskID(value)
	if err != nil {
		return 0, &UsageError{Message: fmt.Sprintf("invalid project instruction id: %s", value), Code: 2}
	}
	return id, nil
}

func validateProjectPath(path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", &UsageError{Message: "project path is required", Code: 2}
	}
	return projectpkg.ValidateLocalPath(path)
}

func printProject(out io.Writer, project storage.Project) {
	fmt.Fprintf(out, "id: %d\n", project.ID)
	fmt.Fprintf(out, "name: %s\n", project.Name)
	fmt.Fprintf(out, "display_name: %s\n", project.DisplayName)
	fmt.Fprintf(out, "path: %s\n", project.Path)
	fmt.Fprintf(out, "created_at: %s\n", project.CreatedAt)
	fmt.Fprintf(out, "updated_at: %s\n", project.UpdatedAt)
}

func printProjectJSON(out io.Writer, project storage.Project) error {
	encoder := json.NewEncoder(out)
	encoder.SetIndent("", "  ")
	return encoder.Encode(projectOutputFromStorage(project))
}

func printProjectsJSON(out io.Writer, projects []storage.Project) error {
	projectOutputs := make([]projectOutput, 0, len(projects))
	for _, project := range projects {
		projectOutputs = append(projectOutputs, projectOutputFromStorage(project))
	}

	encoder := json.NewEncoder(out)
	encoder.SetIndent("", "  ")
	return encoder.Encode(projectOutputs)
}

func printProjectInstruction(out io.Writer, instruction storage.ProjectInstruction) {
	fmt.Fprintf(out, "id: %d\n", instruction.ID)
	fmt.Fprintf(out, "project_id: %d\n", instruction.ProjectID)
	fmt.Fprintf(out, "scope: %s\n", instruction.Scope)
	fmt.Fprintf(out, "title: %s\n", instruction.Title)
	fmt.Fprintf(out, "body:\n")
	for _, line := range strings.Split(instruction.Body, "\n") {
		fmt.Fprintf(out, "  %s\n", line)
	}
	fmt.Fprintf(out, "priority: %s\n", instruction.Priority)
	fmt.Fprintf(out, "enabled: %t\n", instruction.Enabled)
	fmt.Fprintf(out, "source: %s\n", instruction.Source)
	fmt.Fprintf(out, "created_at: %s\n", instruction.CreatedAt)
	fmt.Fprintf(out, "updated_at: %s\n", instruction.UpdatedAt)
}

func printProjectInstructions(out io.Writer, instructions []storage.ProjectInstruction) {
	if len(instructions) == 0 {
		fmt.Fprintln(out, "no project instructions")
		return
	}
	rows := [][]string{{"id", "priority", "enabled", "title", "source", "updated_at"}}
	for _, instruction := range instructions {
		rows = append(rows, []string{
			fmt.Sprintf("%d", instruction.ID),
			instruction.Priority,
			fmt.Sprintf("%t", instruction.Enabled),
			instruction.Title,
			instruction.Source,
			instruction.UpdatedAt,
		})
	}
	_ = printTerminalTable(out, rows)
}

func printProjectInstructionJSON(out io.Writer, instruction storage.ProjectInstruction) error {
	encoder := json.NewEncoder(out)
	encoder.SetIndent("", "  ")
	return encoder.Encode(projectInstructionOutputFromStorage(instruction))
}

func printProjectInstructionsJSON(out io.Writer, instructions []storage.ProjectInstruction) error {
	items := make([]projectInstructionOutput, 0, len(instructions))
	for _, instruction := range instructions {
		items = append(items, projectInstructionOutputFromStorage(instruction))
	}
	encoder := json.NewEncoder(out)
	encoder.SetIndent("", "  ")
	return encoder.Encode(items)
}

func printProjectInstructionRemovedJSON(out io.Writer, id int64) error {
	encoder := json.NewEncoder(out)
	encoder.SetIndent("", "  ")
	return encoder.Encode(map[string]any{
		"removed": true,
		"id":      id,
	})
}

func projectOutputFromStorage(project storage.Project) projectOutput {
	return projectOutput{
		ID:          project.ID,
		Name:        project.Name,
		DisplayName: project.DisplayName,
		Path:        project.Path,
		CreatedAt:   project.CreatedAt,
		UpdatedAt:   project.UpdatedAt,
	}
}

type projectInstructionOutput struct {
	ID        int64  `json:"id"`
	ProjectID int64  `json:"project_id"`
	Scope     string `json:"scope"`
	Title     string `json:"title"`
	Body      string `json:"body"`
	Priority  string `json:"priority"`
	Enabled   bool   `json:"enabled"`
	Source    string `json:"source"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

func projectInstructionOutputFromStorage(instruction storage.ProjectInstruction) projectInstructionOutput {
	return projectInstructionOutput{
		ID:        instruction.ID,
		ProjectID: instruction.ProjectID,
		Scope:     instruction.Scope,
		Title:     instruction.Title,
		Body:      instruction.Body,
		Priority:  instruction.Priority,
		Enabled:   instruction.Enabled,
		Source:    instruction.Source,
		CreatedAt: instruction.CreatedAt,
		UpdatedAt: instruction.UpdatedAt,
	}
}
