package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/pterm/pterm"

	"s26.sh/tok/internal/indexwatch"
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
	case "watch":
		return c.runIndexWatch(ctx, store, opts.args[2:])
	case "ignore":
		return c.runIndexIgnore(ctx, store, opts.args[2:])
	default:
		return &UsageError{
			Message: fmt.Sprintf("unknown index command %q\n\nRun '%s help' for usage.", opts.args[1], commandName),
			Code:    2,
		}
	}
}

type indexWatchOptions struct {
	projectName      string
	debounce         time.Duration
	registryInterval time.Duration
	noInitialIndex   bool
	quiet            bool
}

func (c *CLI) runIndexWatch(ctx context.Context, store *storage.Store, args []string) error {
	watchOpts, err := parseIndexWatchOptions(args)
	if err != nil {
		return err
	}

	events := make(chan indexwatch.Event, 256)
	watchCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	printDone := make(chan struct{})
	go func() {
		defer close(printDone)
		for {
			select {
			case <-watchCtx.Done():
				return
			case event := <-events:
				printIndexWatchEvent(c.out, c.err, event, watchOpts.quiet)
			}
		}
	}()

	if !watchOpts.quiet {
		scope := "all projects"
		if watchOpts.projectName != "" {
			scope = "project " + watchOpts.projectName
		}
		pterm.Info.WithWriter(c.out).Printfln(
			"watching %s (debounce=%s, registry_interval=%s)",
			scope,
			watchOpts.debounce,
			watchOpts.registryInterval,
		)
	}

	service, err := indexwatch.New(indexwatch.Config{
		Store:            store,
		Indexer:          retrieval.NewService(store),
		Debounce:         watchOpts.debounce,
		RegistryInterval: watchOpts.registryInterval,
		ProjectName:      watchOpts.projectName,
		NoInitialIndex:   watchOpts.noInitialIndex,
		Events:           events,
	})
	if err != nil {
		return err
	}

	err = service.Run(watchCtx)
	cancel()
	<-printDone
	if errors.Is(err, context.Canceled) {
		return nil
	}
	return err
}

func parseIndexWatchOptions(args []string) (indexWatchOptions, error) {
	opts := indexWatchOptions{
		debounce:         indexwatch.DefaultDebounce,
		registryInterval: indexwatch.DefaultRegistryInterval,
	}

	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--project":
			i++
			if i >= len(args) {
				return indexWatchOptions{}, &UsageError{Message: "--project requires a value", Code: 2}
			}
			opts.projectName = args[i]
		case strings.HasPrefix(arg, "--project="):
			opts.projectName = strings.TrimPrefix(arg, "--project=")
			if opts.projectName == "" {
				return indexWatchOptions{}, &UsageError{Message: "--project requires a value", Code: 2}
			}
		case arg == "--debounce":
			i++
			if i >= len(args) {
				return indexWatchOptions{}, &UsageError{Message: "--debounce requires a value", Code: 2}
			}
			debounce, err := parsePositiveDuration("--debounce", args[i])
			if err != nil {
				return indexWatchOptions{}, err
			}
			opts.debounce = debounce
		case strings.HasPrefix(arg, "--debounce="):
			debounce, err := parsePositiveDuration("--debounce", strings.TrimPrefix(arg, "--debounce="))
			if err != nil {
				return indexWatchOptions{}, err
			}
			opts.debounce = debounce
		case arg == "--registry-interval":
			i++
			if i >= len(args) {
				return indexWatchOptions{}, &UsageError{Message: "--registry-interval requires a value", Code: 2}
			}
			interval, err := parsePositiveDuration("--registry-interval", args[i])
			if err != nil {
				return indexWatchOptions{}, err
			}
			opts.registryInterval = interval
		case strings.HasPrefix(arg, "--registry-interval="):
			interval, err := parsePositiveDuration("--registry-interval", strings.TrimPrefix(arg, "--registry-interval="))
			if err != nil {
				return indexWatchOptions{}, err
			}
			opts.registryInterval = interval
		case arg == "--no-initial-index":
			opts.noInitialIndex = true
		case arg == "--quiet":
			opts.quiet = true
		default:
			return indexWatchOptions{}, &UsageError{Message: fmt.Sprintf("unknown index watch option %q", arg), Code: 2}
		}
	}

	opts.projectName = strings.TrimSpace(opts.projectName)
	return opts, nil
}

func parsePositiveDuration(flagName, value string) (time.Duration, error) {
	duration, err := time.ParseDuration(value)
	if err != nil || duration <= 0 {
		return 0, &UsageError{Message: fmt.Sprintf("invalid %s duration: %s", flagName, value), Code: 2}
	}
	return duration, nil
}

func printIndexWatchEvent(out, errOut io.Writer, event indexwatch.Event, quiet bool) {
	if quiet && event.Err == nil && event.Type != indexwatch.EventPathMissing {
		return
	}

	switch event.Type {
	case indexwatch.EventProjectWatching:
		pterm.Info.WithWriter(out).Printfln("watching %s %s", event.ProjectName, event.Path)
	case indexwatch.EventProjectRemoved:
		pterm.Warning.WithWriter(out).Printfln("removed %s from watcher", event.ProjectName)
	case indexwatch.EventPathMissing:
		pterm.Warning.WithWriter(out).Printfln("path missing for %s: %s", event.ProjectName, event.Path)
	case indexwatch.EventPathRestored:
		pterm.Success.WithWriter(out).Printfln("path restored for %s: %s", event.ProjectName, event.Path)
	case indexwatch.EventIndexQueued:
		pterm.DefaultBasicText.WithWriter(out).Printfln("queued %s: %s", event.ProjectName, event.Message)
	case indexwatch.EventIndexStarted:
		pterm.Info.WithWriter(out).Printfln("indexing %s", event.ProjectName)
	case indexwatch.EventIndexCompleted:
		if event.Summary == nil {
			pterm.Success.WithWriter(out).Printfln("indexed %s", event.ProjectName)
			return
		}
		pterm.Success.WithWriter(out).Printfln(
			"indexed %s: state=%s documents=%d chunks=%d skipped=%d",
			event.ProjectName,
			event.Summary.State,
			event.Summary.IndexedDocuments,
			event.Summary.IndexedChunks,
			event.Summary.SkippedFiles,
		)
	case indexwatch.EventIndexFailed, indexwatch.EventWatchError:
		if event.Err != nil {
			pterm.Error.WithWriter(errOut).Printfln("%s %s: %v", event.Type, event.ProjectName, event.Err)
			return
		}
		pterm.Error.WithWriter(errOut).Printfln("%s %s", event.Type, event.ProjectName)
	}
}

type indexIgnoreOptions struct {
	projectName string
	pattern     string
	json        bool
}

func (c *CLI) runIndexIgnore(ctx context.Context, store *storage.Store, args []string) error {
	if len(args) < 1 {
		return &UsageError{Message: "missing index ignore command", Code: 2}
	}

	command := args[0]
	ignoreOpts, err := parseIndexIgnoreOptions(args[1:], "index ignore "+command, command == "add" || command == "remove")
	if err != nil {
		return err
	}
	project, err := getProjectForTask(ctx, store, ignoreOpts.projectName)
	if err != nil {
		return err
	}
	service := retrieval.NewService(store)

	var policy retrieval.IndexPolicy
	switch command {
	case "list":
		policy, err = service.IgnorePolicy(ctx, project)
	case "refresh":
		policy, err = service.RefreshIgnorePolicy(ctx, project)
	case "add":
		policy, err = service.AddIgnorePattern(ctx, project, ignoreOpts.pattern)
	case "remove":
		policy, err = service.RemoveIgnorePattern(ctx, project, ignoreOpts.pattern)
	default:
		return &UsageError{Message: fmt.Sprintf("unknown index ignore command %q", command), Code: 2}
	}
	if err != nil {
		return err
	}
	if ignoreOpts.json {
		return printIndexIgnorePolicyJSON(c.out, policy)
	}
	printIndexIgnorePolicy(c.out, policy)
	return nil
}

func parseIndexIgnoreOptions(args []string, command string, requirePattern bool) (indexIgnoreOptions, error) {
	var opts indexIgnoreOptions
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--project":
			i++
			if i >= len(args) {
				return indexIgnoreOptions{}, &UsageError{Message: "--project requires a value", Code: 2}
			}
			opts.projectName = args[i]
		case strings.HasPrefix(arg, "--project="):
			opts.projectName = strings.TrimPrefix(arg, "--project=")
			if opts.projectName == "" {
				return indexIgnoreOptions{}, &UsageError{Message: "--project requires a value", Code: 2}
			}
		case arg == "--json":
			opts.json = true
		case strings.HasPrefix(arg, "-"):
			return indexIgnoreOptions{}, &UsageError{Message: fmt.Sprintf("unknown %s option %q", command, arg), Code: 2}
		default:
			if opts.pattern != "" {
				return indexIgnoreOptions{}, &UsageError{Message: command + " accepts exactly one pattern", Code: 2}
			}
			opts.pattern = arg
		}
	}
	opts.projectName = strings.TrimSpace(opts.projectName)
	opts.pattern = strings.TrimSpace(opts.pattern)
	if opts.projectName == "" {
		return indexIgnoreOptions{}, &UsageError{Message: command + " requires --project", Code: 2}
	}
	if requirePattern && opts.pattern == "" {
		return indexIgnoreOptions{}, &UsageError{Message: command + " requires a pattern", Code: 2}
	}
	if !requirePattern && opts.pattern != "" {
		return indexIgnoreOptions{}, &UsageError{Message: command + " does not accept a pattern", Code: 2}
	}
	return opts, nil
}

type indexProjectOptions struct {
	projectName string
	all         bool
	json        bool
}

func (c *CLI) runIndexUpdate(ctx context.Context, store *storage.Store, args []string) error {
	indexOpts, err := parseIndexProjectOptions(args, "index update")
	if err != nil {
		return err
	}

	if indexOpts.all {
		projects, err := store.ListProjects(ctx)
		if err != nil {
			return err
		}
		summaries := make([]retrieval.IndexSummary, 0, len(projects))
		service := retrieval.NewService(store)
		var progress *pterm.ProgressbarPrinter
		if !indexOpts.json && len(projects) > 1 {
			progress, _ = pterm.DefaultProgressbar.
				WithWriter(c.out).
				WithTotal(len(projects)).
				WithShowCount().
				WithShowPercentage().
				Start("index projects")
		}
		for _, project := range projects {
			if progress != nil {
				progress.UpdateTitle("index " + project.Name)
			}
			summary, err := service.IndexProject(ctx, project)
			if err != nil {
				summary = retrieval.IndexSummary{
					ProjectName:    project.Name,
					State:          retrieval.StateFailed,
					SkippedReasons: map[string]int{},
					LastError:      err.Error(),
				}
			}
			summaries = append(summaries, summary)
			if progress != nil {
				progress.Increment()
			}
		}
		if progress != nil {
			_, _ = progress.Stop()
			fmt.Fprintln(c.out)
		}
		if indexOpts.json {
			return printIndexSummariesJSON(c.out, summaries)
		}
		printIndexSummaries(c.out, summaries)
		return nil
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

	if indexOpts.all {
		projects, err := store.ListProjects(ctx)
		if err != nil {
			return err
		}
		statuses := make([]retrieval.IndexStatus, 0, len(projects))
		service := retrieval.NewService(store)
		for _, project := range projects {
			status, err := service.IndexStatus(ctx, project)
			if err != nil {
				status = retrieval.IndexStatus{
					ProjectName:    project.Name,
					State:          retrieval.StateFailed,
					SkippedReasons: map[string]int{},
					LastError:      err.Error(),
				}
			}
			statuses = append(statuses, status)
		}
		if indexOpts.json {
			return printIndexStatusesJSON(c.out, statuses)
		}
		printIndexStatuses(c.out, statuses)
		return nil
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
			if opts.all {
				return indexProjectOptions{}, &UsageError{Message: command + " cannot combine --project and --all", Code: 2}
			}
			i++
			if i >= len(args) {
				return indexProjectOptions{}, &UsageError{Message: "--project requires a value", Code: 2}
			}
			opts.projectName = args[i]
		case strings.HasPrefix(arg, "--project="):
			if opts.all {
				return indexProjectOptions{}, &UsageError{Message: command + " cannot combine --project and --all", Code: 2}
			}
			opts.projectName = strings.TrimPrefix(arg, "--project=")
			if opts.projectName == "" {
				return indexProjectOptions{}, &UsageError{Message: "--project requires a value", Code: 2}
			}
		case arg == "--all":
			if opts.projectName != "" {
				return indexProjectOptions{}, &UsageError{Message: command + " cannot combine --project and --all", Code: 2}
			}
			opts.all = true
		case arg == "--json":
			opts.json = true
		default:
			return indexProjectOptions{}, &UsageError{Message: fmt.Sprintf("unknown %s option %q", command, arg), Code: 2}
		}
	}

	opts.projectName = strings.TrimSpace(opts.projectName)
	if opts.projectName == "" && !opts.all {
		return indexProjectOptions{}, &UsageError{Message: command + " requires --project", Code: 2}
	}

	return opts, nil
}

type searchOptions struct {
	projectName string
	query       string
	limit       int
	json        bool
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
	if searchOpts.json {
		return printSearchResultsJSON(c.out, results)
	}
	if len(results) == 0 {
		fmt.Fprintln(c.out, "no results")
		return nil
	}

	rows := [][]string{{"path", "line", "score", "provenance", "snippet"}}
	for _, result := range results {
		rows = append(rows, []string{
			result.Path,
			strconv.Itoa(result.Line),
			fmt.Sprintf("%.6f", result.Score),
			result.Provenance,
			result.Snippet,
		})
	}
	return printTerminalTable(c.out, rows)
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
		case arg == "--json":
			opts.json = true
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
	State            string         `json:"state"`
	PathExists       bool           `json:"path_exists"`
	IndexedDocuments int            `json:"indexed_documents"`
	IndexedChunks    int            `json:"indexed_chunks"`
	SkippedFiles     int            `json:"skipped_files"`
	SkippedReasons   map[string]int `json:"skipped_reasons"`
	UpdatedAt        string         `json:"updated_at"`
	LastError        string         `json:"last_error,omitempty"`
}

type indexIgnorePolicyOutput struct {
	ProjectName      string   `json:"project_name"`
	IncludePatterns  []string `json:"include_patterns"`
	IgnorePatterns   []string `json:"ignore_patterns"`
	CreatedAt        string   `json:"created_at"`
	UpdatedAt        string   `json:"updated_at"`
	SeededFromIgnore bool     `json:"seeded_from_gitignore"`
}

type searchListOutput struct {
	Results []searchResultOutput `json:"results"`
}

type searchResultOutput struct {
	Path       string  `json:"path"`
	Score      float64 `json:"score"`
	Line       int     `json:"line"`
	LineStart  int     `json:"line_start"`
	LineEnd    int     `json:"line_end"`
	Snippet    string  `json:"snippet"`
	Excerpt    string  `json:"excerpt"`
	Provenance string  `json:"provenance"`
}

func printSearchResultsJSON(out io.Writer, results []retrieval.SearchResult) error {
	items := make([]searchResultOutput, 0, len(results))
	for _, result := range results {
		items = append(items, searchResultOutput{
			Path:       result.Path,
			Score:      result.Score,
			Line:       result.Line,
			LineStart:  result.LineStart,
			LineEnd:    result.LineEnd,
			Snippet:    result.Snippet,
			Excerpt:    result.Excerpt,
			Provenance: result.Provenance,
		})
	}

	encoder := json.NewEncoder(out)
	encoder.SetIndent("", "  ")
	return encoder.Encode(searchListOutput{Results: items})
}

func printIndexIgnorePolicy(out io.Writer, policy retrieval.IndexPolicy) {
	fmt.Fprintf(out, "project: %s\n", policy.ProjectName)
	fmt.Fprintf(out, "seeded_from_gitignore: %t\n", policy.SeededFromIgnore)
	if len(policy.IgnorePatterns) == 0 {
		fmt.Fprintln(out, "ignore_patterns: none")
	} else {
		fmt.Fprintln(out, "ignore_patterns:")
		for _, pattern := range policy.IgnorePatterns {
			fmt.Fprintf(out, "- %s\n", pattern)
		}
	}
	if policy.UpdatedAt != "" {
		fmt.Fprintf(out, "updated_at: %s\n", policy.UpdatedAt)
	}
}

func printIndexIgnorePolicyJSON(out io.Writer, policy retrieval.IndexPolicy) error {
	encoder := json.NewEncoder(out)
	encoder.SetIndent("", "  ")
	return encoder.Encode(indexIgnorePolicyOutput{
		ProjectName:      policy.ProjectName,
		IncludePatterns:  policy.IncludePatterns,
		IgnorePatterns:   policy.IgnorePatterns,
		CreatedAt:        policy.CreatedAt,
		UpdatedAt:        policy.UpdatedAt,
		SeededFromIgnore: policy.SeededFromIgnore,
	})
}

func printIndexSummary(out io.Writer, summary retrieval.IndexSummary) {
	fmt.Fprintf(out, "project: %s\n", summary.ProjectName)
	fmt.Fprintf(out, "state: %s\n", summary.State)
	fmt.Fprintf(out, "path_exists: %t\n", summary.PathExists)
	fmt.Fprintf(out, "indexed_documents: %d\n", summary.IndexedDocuments)
	fmt.Fprintf(out, "indexed_chunks: %d\n", summary.IndexedChunks)
	fmt.Fprintf(out, "skipped_files: %d\n", summary.SkippedFiles)
	printSkippedReasons(out, summary.SkippedReasons)
	fmt.Fprintf(out, "updated_at: %s\n", summary.UpdatedAt)
	if summary.LastError != "" {
		fmt.Fprintf(out, "last_error: %s\n", summary.LastError)
	}
}

func printIndexStatus(out io.Writer, status retrieval.IndexStatus) {
	fmt.Fprintf(out, "project: %s\n", status.ProjectName)
	fmt.Fprintf(out, "state: %s\n", status.State)
	fmt.Fprintf(out, "path_exists: %t\n", status.PathExists)
	fmt.Fprintf(out, "indexed_documents: %d\n", status.IndexedDocuments)
	fmt.Fprintf(out, "indexed_chunks: %d\n", status.IndexedChunks)
	fmt.Fprintf(out, "skipped_files: %d\n", status.SkippedFiles)
	printSkippedReasons(out, status.SkippedReasons)
	fmt.Fprintf(out, "updated_at: %s\n", status.UpdatedAt)
	if status.LastError != "" {
		fmt.Fprintf(out, "last_error: %s\n", status.LastError)
	}
}

func printIndexSummaries(out io.Writer, summaries []retrieval.IndexSummary) {
	if len(summaries) == 0 {
		fmt.Fprintln(out, "no projects")
		return
	}
	rows := [][]string{{"project", "state", "path_exists", "documents", "chunks", "skipped", "updated_at", "last_error"}}
	for _, summary := range summaries {
		rows = append(rows, []string{
			summary.ProjectName,
			summary.State,
			strconv.FormatBool(summary.PathExists),
			strconv.Itoa(summary.IndexedDocuments),
			strconv.Itoa(summary.IndexedChunks),
			strconv.Itoa(summary.SkippedFiles),
			summary.UpdatedAt,
			summary.LastError,
		})
	}
	_ = printTerminalTable(out, rows)
}

func printIndexStatuses(out io.Writer, statuses []retrieval.IndexStatus) {
	if len(statuses) == 0 {
		fmt.Fprintln(out, "no projects")
		return
	}
	rows := [][]string{{"project", "state", "path_exists", "documents", "chunks", "skipped", "updated_at", "last_error"}}
	for _, status := range statuses {
		rows = append(rows, []string{
			status.ProjectName,
			status.State,
			strconv.FormatBool(status.PathExists),
			strconv.Itoa(status.IndexedDocuments),
			strconv.Itoa(status.IndexedChunks),
			strconv.Itoa(status.SkippedFiles),
			status.UpdatedAt,
			status.LastError,
		})
	}
	_ = printTerminalTable(out, rows)
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
		State:            summary.State,
		PathExists:       summary.PathExists,
		IndexedDocuments: summary.IndexedDocuments,
		IndexedChunks:    summary.IndexedChunks,
		SkippedFiles:     summary.SkippedFiles,
		SkippedReasons:   summary.SkippedReasons,
		UpdatedAt:        summary.UpdatedAt,
		LastError:        summary.LastError,
	})
}

func printIndexSummariesJSON(out io.Writer, summaries []retrieval.IndexSummary) error {
	encoder := json.NewEncoder(out)
	encoder.SetIndent("", "  ")
	items := make([]indexSummaryOutput, 0, len(summaries))
	for _, summary := range summaries {
		items = append(items, indexSummaryOutput{
			ProjectName:      summary.ProjectName,
			State:            summary.State,
			PathExists:       summary.PathExists,
			IndexedDocuments: summary.IndexedDocuments,
			IndexedChunks:    summary.IndexedChunks,
			SkippedFiles:     summary.SkippedFiles,
			SkippedReasons:   summary.SkippedReasons,
			UpdatedAt:        summary.UpdatedAt,
			LastError:        summary.LastError,
		})
	}
	return encoder.Encode(items)
}

func printIndexStatusJSON(out io.Writer, status retrieval.IndexStatus) error {
	encoder := json.NewEncoder(out)
	encoder.SetIndent("", "  ")
	return encoder.Encode(indexSummaryOutput{
		ProjectName:      status.ProjectName,
		State:            status.State,
		PathExists:       status.PathExists,
		IndexedDocuments: status.IndexedDocuments,
		IndexedChunks:    status.IndexedChunks,
		SkippedFiles:     status.SkippedFiles,
		SkippedReasons:   status.SkippedReasons,
		UpdatedAt:        status.UpdatedAt,
		LastError:        status.LastError,
	})
}

func printIndexStatusesJSON(out io.Writer, statuses []retrieval.IndexStatus) error {
	encoder := json.NewEncoder(out)
	encoder.SetIndent("", "  ")
	items := make([]indexSummaryOutput, 0, len(statuses))
	for _, status := range statuses {
		items = append(items, indexSummaryOutput{
			ProjectName:      status.ProjectName,
			State:            status.State,
			PathExists:       status.PathExists,
			IndexedDocuments: status.IndexedDocuments,
			IndexedChunks:    status.IndexedChunks,
			SkippedFiles:     status.SkippedFiles,
			SkippedReasons:   status.SkippedReasons,
			UpdatedAt:        status.UpdatedAt,
			LastError:        status.LastError,
		})
	}
	return encoder.Encode(items)
}

func sortedReasonKeys(reasons map[string]int) []string {
	keys := make([]string, 0, len(reasons))
	for key := range reasons {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
