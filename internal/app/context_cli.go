package app

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"

	contextpkg "s26.sh/tok/internal/context"
	"s26.sh/tok/internal/retrieval"
	"s26.sh/tok/internal/storage"
)

func (c *CLI) runContext(ctx context.Context, opts runtimeOptions) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if len(opts.args) < 2 {
		return &UsageError{
			Message: fmt.Sprintf("missing context command\n\nRun '%s help' for usage.", commandName),
			Code:    2,
		}
	}

	_, _, store, err := c.runtimeStore(ctx, opts)
	if err != nil {
		return err
	}
	defer store.Close()

	switch opts.args[1] {
	case "build":
		return c.runContextBuild(ctx, store, opts.args[2:])
	default:
		return &UsageError{
			Message: fmt.Sprintf("unknown context command %q\n\nRun '%s help' for usage.", opts.args[1], commandName),
			Code:    2,
		}
	}
}

type contextBuildOptions struct {
	projectName    string
	taskID         int64
	retrievalLimit int
}

func (c *CLI) runContextBuild(ctx context.Context, store *storage.Store, args []string) error {
	buildOpts, err := parseContextBuildOptions(args)
	if err != nil {
		return err
	}

	project, err := getProjectForTask(ctx, store, buildOpts.projectName)
	if err != nil {
		return err
	}

	task, err := store.GetTask(ctx, buildOpts.taskID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("task not found: %d", buildOpts.taskID)
		}
		return err
	}

	builder := contextpkg.NewBuilder(store, retrieval.NewService(store))
	pkg, err := builder.Build(ctx, contextpkg.BuildInput{
		Project:        project,
		Task:           task,
		RetrievalLimit: buildOpts.retrievalLimit,
	})
	if err != nil {
		return err
	}

	fmt.Fprint(c.out, pkg.RenderText())
	return nil
}

func parseContextBuildOptions(args []string) (contextBuildOptions, error) {
	var opts contextBuildOptions

	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--project":
			i++
			if i >= len(args) {
				return contextBuildOptions{}, &UsageError{Message: "--project requires a value", Code: 2}
			}
			opts.projectName = args[i]
		case strings.HasPrefix(arg, "--project="):
			opts.projectName = strings.TrimPrefix(arg, "--project=")
			if opts.projectName == "" {
				return contextBuildOptions{}, &UsageError{Message: "--project requires a value", Code: 2}
			}
		case arg == "--task":
			i++
			if i >= len(args) {
				return contextBuildOptions{}, &UsageError{Message: "--task requires a value", Code: 2}
			}
			taskID, err := parseTaskID(args[i])
			if err != nil {
				return contextBuildOptions{}, err
			}
			opts.taskID = taskID
		case strings.HasPrefix(arg, "--task="):
			taskID, err := parseTaskID(strings.TrimPrefix(arg, "--task="))
			if err != nil {
				return contextBuildOptions{}, err
			}
			opts.taskID = taskID
		case arg == "--limit":
			i++
			if i >= len(args) {
				return contextBuildOptions{}, &UsageError{Message: "--limit requires a value", Code: 2}
			}
			limit, err := parseContextLimit(args[i])
			if err != nil {
				return contextBuildOptions{}, err
			}
			opts.retrievalLimit = limit
		case strings.HasPrefix(arg, "--limit="):
			limit, err := parseContextLimit(strings.TrimPrefix(arg, "--limit="))
			if err != nil {
				return contextBuildOptions{}, err
			}
			opts.retrievalLimit = limit
		default:
			return contextBuildOptions{}, &UsageError{Message: fmt.Sprintf("unknown context build option %q", arg), Code: 2}
		}
	}

	opts.projectName = strings.TrimSpace(opts.projectName)
	if opts.projectName == "" {
		return contextBuildOptions{}, &UsageError{Message: "context build requires --project", Code: 2}
	}
	if opts.taskID == 0 {
		return contextBuildOptions{}, &UsageError{Message: "context build requires --task", Code: 2}
	}
	if opts.retrievalLimit == 0 {
		opts.retrievalLimit = contextpkg.DefaultRetrievalLimit
	}

	return opts, nil
}

func parseContextLimit(value string) (int, error) {
	limit, err := strconv.Atoi(value)
	if err != nil || limit <= 0 {
		return 0, &UsageError{Message: fmt.Sprintf("invalid context retrieval limit: %s", value), Code: 2}
	}
	return limit, nil
}
