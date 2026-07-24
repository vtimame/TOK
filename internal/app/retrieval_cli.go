package app

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sort"
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
	case "status":
		return c.runIndexStatus(ctx, store, opts.args[2:])
	default:
		return &UsageError{
			Message: fmt.Sprintf("unknown index command %q\n\nRun '%s help' for usage.", opts.args[1], commandName),
			Code:    2,
		}
	}
}

type indexProjectOptions struct {
	projectName string
	json        bool
}

func (c *CLI) runIndexUpdate(ctx context.Context, store *storage.Store, args []string) error {
	indexOpts, err := parseIndexProjectOptions(args, "index update")
	if err != nil {
		return err
	}

	project, err := getProjectForTask(ctx, store, indexOpts.projectName)
	if err != nil {
		return err
	}

	summary, err := retrieval.NewService(store).IndexProject(ctx, project)
	if err != nil {
		return err
	}

	if indexOpts.json {
		return printIndexSummaryJSON(c.out, summary)
	}
	printIndexSummary(c.out, summary)
	return nil
}

func (c *CLI) runIndexStatus(ctx context.Context, store *storage.Store, args []string) error {
	indexOpts, err := parseIndexProjectOptions(args, "index status")
	if err != nil {
		return err
	}

	project, err := getProjectForTask(ctx, store, indexOpts.projectName)
	if err != nil {
		return err
	}

	status, err := retrieval.NewService(store).IndexStatus(ctx, project)
	if err != nil {
		return err
	}

	if indexOpts.json {
		return printIndexStatusJSON(c.out, status)
	}
	printIndexStatus(c.out, status)
	return nil
}

func parseIndexProjectOptions(args []string, command string) (indexProjectOptions, error) {
	var opts indexProjectOptions

	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--project":
			i++
			if i >= len(args) {
				return indexProjectOptions{}, &UsageError{Message: "--project requires a value", Code: 2}
			}
			opts.projectName = args[i]
		case strings.HasPrefix(arg, "--project="):
			opts.projectName = strings.TrimPrefix(arg, "--project=")
			if opts.projectName == "" {
				return indexProjectOptions{}, &UsageError{Message: "--project requires a value", Code: 2}
			}
		case arg == "--json":
			opts.json = true
		default:
			return indexProjectOptions{}, &UsageError{Message: fmt.Sprintf("unknown %s option %q", command, arg), Code: 2}
		}
	}

	opts.projectName = strings.TrimSpace(opts.projectName)
	if opts.projectName == "" {
		return indexProjectOptions{}, &UsageError{Message: command + " requires --project", Code: 2}
	}

	return opts, nil
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

type indexSummaryOutput struct {
	ProjectName      string         `json:"project_name"`
	IndexedDocuments int            `json:"indexed_documents"`
	SkippedFiles     int            `json:"skipped_files"`
	SkippedReasons   map[string]int `json:"skipped_reasons"`
	UpdatedAt        string         `json:"updated_at"`
}

func printIndexSummary(out io.Writer, summary retrieval.IndexSummary) {
	fmt.Fprintf(out, "project: %s\n", summary.ProjectName)
	fmt.Fprintf(out, "indexed_documents: %d\n", summary.IndexedDocuments)
	fmt.Fprintf(out, "skipped_files: %d\n", summary.SkippedFiles)
	printSkippedReasons(out, summary.SkippedReasons)
	fmt.Fprintf(out, "updated_at: %s\n", summary.UpdatedAt)
}

func printIndexStatus(out io.Writer, status retrieval.IndexStatus) {
	fmt.Fprintf(out, "project: %s\n", status.ProjectName)
	fmt.Fprintf(out, "indexed_documents: %d\n", status.IndexedDocuments)
	fmt.Fprintf(out, "skipped_files: %d\n", status.SkippedFiles)
	printSkippedReasons(out, status.SkippedReasons)
	fmt.Fprintf(out, "updated_at: %s\n", status.UpdatedAt)
}

func printSkippedReasons(out io.Writer, reasons map[string]int) {
	if len(reasons) == 0 {
		fmt.Fprintln(out, "skipped_reasons: none")
		return
	}
	fmt.Fprintln(out, "skipped_reasons:")
	for _, reason := range sortedReasonKeys(reasons) {
		fmt.Fprintf(out, "- %s: %d\n", reason, reasons[reason])
	}
}

func printIndexSummaryJSON(out io.Writer, summary retrieval.IndexSummary) error {
	encoder := json.NewEncoder(out)
	encoder.SetIndent("", "  ")
	return encoder.Encode(indexSummaryOutput{
		ProjectName:      summary.ProjectName,
		IndexedDocuments: summary.IndexedDocuments,
		SkippedFiles:     summary.SkippedFiles,
		SkippedReasons:   summary.SkippedReasons,
		UpdatedAt:        summary.UpdatedAt,
	})
}

func printIndexStatusJSON(out io.Writer, status retrieval.IndexStatus) error {
	encoder := json.NewEncoder(out)
	encoder.SetIndent("", "  ")
	return encoder.Encode(indexSummaryOutput{
		ProjectName:      status.ProjectName,
		IndexedDocuments: status.IndexedDocuments,
		SkippedFiles:     status.SkippedFiles,
		SkippedReasons:   status.SkippedReasons,
		UpdatedAt:        status.UpdatedAt,
	})
}

func sortedReasonKeys(reasons map[string]int) []string {
	keys := make([]string, 0, len(reasons))
	for key := range reasons {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
