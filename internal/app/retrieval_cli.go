package app

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"s26.sh/tok/internal/retrieval"
	"s26.sh/tok/internal/storage"
)

func (c *CLI) runIndex(ctx context.Context, opts runtimeOptions) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if len(opts.args) < 2 {
		return &UsageError{
			Message: fmt.Sprintf("missing index command\n\nRun '%s help' for usage.", commandName),
			Code:    2,
		}
	}

	_, _, store, err := c.runtimeStore(ctx, opts)
	if err != nil {
		return err
	}
	defer store.Close()

	switch opts.args[1] {
	case "update":
		return c.runIndexUpdate(ctx, store, opts.args[2:])
	default:
		return &UsageError{
			Message: fmt.Sprintf("unknown index command %q\n\nRun '%s help' for usage.", opts.args[1], commandName),
			Code:    2,
		}
	}
}

func (c *CLI) runIndexUpdate(ctx context.Context, store *storage.Store, args []string) error {
	projectName, err := parseRequiredProjectOption(args, "index update")
	if err != nil {
		return err
	}

	project, err := getProjectForTask(ctx, store, projectName)
	if err != nil {
		return err
	}

	summary, err := retrieval.NewService(store).IndexProject(ctx, project)
	if err != nil {
		return err
	}

	fmt.Fprintf(c.out, "project: %s\n", summary.ProjectName)
	fmt.Fprintf(c.out, "indexed_documents: %d\n", summary.IndexedDocuments)
	fmt.Fprintf(c.out, "skipped_files: %d\n", summary.SkippedFiles)
	return nil
}

type searchOptions struct {
	projectName string
	query       string
	limit       int
}

func (c *CLI) runSearch(ctx context.Context, opts runtimeOptions) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	searchOpts, err := parseSearchOptions(opts.args[1:])
	if err != nil {
		return err
	}

	_, _, store, err := c.runtimeStore(ctx, opts)
	if err != nil {
		return err
	}
	defer store.Close()

	project, err := getProjectForTask(ctx, store, searchOpts.projectName)
	if err != nil {
		return err
	}

	results, err := retrieval.NewService(store).Search(ctx, project, searchOpts.query, searchOpts.limit)
	if err != nil {
		return err
	}
	if len(results) == 0 {
		fmt.Fprintln(c.out, "no results")
		return nil
	}

	fmt.Fprintln(c.out, "path\tline\tscore\tprovenance\tsnippet")
	for _, result := range results {
		fmt.Fprintf(c.out, "%s\t%d\t%.6f\t%s\t%s\n", result.Path, result.Line, result.Score, result.Provenance, result.Snippet)
	}
	return nil
}

func parseSearchOptions(args []string) (searchOptions, error) {
	var opts searchOptions
	var queryParts []string

	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--project":
			i++
			if i >= len(args) {
				return searchOptions{}, &UsageError{Message: "--project requires a value", Code: 2}
			}
			opts.projectName = args[i]
		case strings.HasPrefix(arg, "--project="):
			opts.projectName = strings.TrimPrefix(arg, "--project=")
			if opts.projectName == "" {
				return searchOptions{}, &UsageError{Message: "--project requires a value", Code: 2}
			}
		case arg == "--limit":
			i++
			if i >= len(args) {
				return searchOptions{}, &UsageError{Message: "--limit requires a value", Code: 2}
			}
			limit, err := parseSearchLimit(args[i])
			if err != nil {
				return searchOptions{}, err
			}
			opts.limit = limit
		case strings.HasPrefix(arg, "--limit="):
			limit, err := parseSearchLimit(strings.TrimPrefix(arg, "--limit="))
			if err != nil {
				return searchOptions{}, err
			}
			opts.limit = limit
		case strings.HasPrefix(arg, "-"):
			return searchOptions{}, &UsageError{Message: fmt.Sprintf("unknown search option %q", arg), Code: 2}
		default:
			queryParts = append(queryParts, arg)
		}
	}

	opts.projectName = strings.TrimSpace(opts.projectName)
	opts.query = strings.TrimSpace(strings.Join(queryParts, " "))
	if opts.projectName == "" {
		return searchOptions{}, &UsageError{Message: "search requires --project", Code: 2}
	}
	if opts.query == "" {
		return searchOptions{}, &UsageError{Message: "search requires a query", Code: 2}
	}
	if opts.limit == 0 {
		opts.limit = retrieval.DefaultLimit
	}

	return opts, nil
}

func parseSearchLimit(value string) (int, error) {
	limit, err := strconv.Atoi(value)
	if err != nil || limit <= 0 {
		return 0, &UsageError{Message: fmt.Sprintf("invalid search limit: %s", value), Code: 2}
	}
	return limit, nil
}
