package app

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

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

	printProject(c.out, project)
	return nil
}

func (c *CLI) runProjectList(ctx context.Context, store *storage.Store, args []string) error {
	if len(args) > 0 {
		return &UsageError{Message: "project list does not accept arguments", Code: 2}
	}

	projects, err := store.ListProjects(ctx)
	if err != nil {
		return err
	}
	if len(projects) == 0 {
		fmt.Fprintln(c.out, "no projects")
		return nil
	}

	fmt.Fprintln(c.out, "name\tdisplay_name\tpath")
	for _, project := range projects {
		fmt.Fprintf(c.out, "%s\t%s\t%s\n", project.Name, project.DisplayName, project.Path)
	}
	return nil
}

func (c *CLI) runProjectShow(ctx context.Context, store *storage.Store, args []string) error {
	if len(args) != 1 {
		return &UsageError{Message: "project show requires a project name", Code: 2}
	}

	project, err := store.GetProject(ctx, args[0])
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("project not found: %s", args[0])
		}
		return err
	}

	printProject(c.out, project)
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

func validateProjectPath(path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", &UsageError{Message: "project path is required", Code: 2}
	}

	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve project path %q: %w", path, err)
	}

	info, err := os.Stat(absPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("project path does not exist: %s", absPath)
		}
		return "", fmt.Errorf("inspect project path %q: %w", absPath, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("project path must be a directory: %s", absPath)
	}

	return absPath, nil
}

func printProject(out io.Writer, project storage.Project) {
	fmt.Fprintf(out, "id: %d\n", project.ID)
	fmt.Fprintf(out, "name: %s\n", project.Name)
	fmt.Fprintf(out, "display_name: %s\n", project.DisplayName)
	fmt.Fprintf(out, "path: %s\n", project.Path)
	fmt.Fprintf(out, "created_at: %s\n", project.CreatedAt)
	fmt.Fprintf(out, "updated_at: %s\n", project.UpdatedAt)
}
