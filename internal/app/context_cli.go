package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
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
	outputPath     string
	query          string
	json           bool
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
		Query:          buildOpts.query,
	})
	if err != nil {
		return err
	}

	if buildOpts.json {
		data, err := json.MarshalIndent(contextPackageOutputFromPackage(pkg), "", "  ")
		if err != nil {
			return fmt.Errorf("encode context package JSON: %w", err)
		}
		data = append(data, '\n')
		if buildOpts.outputPath != "" {
			if err := writeContextPackage(buildOpts.outputPath, string(data)); err != nil {
				return err
			}
			fmt.Fprintf(c.out, "wrote context package: %s\n", buildOpts.outputPath)
			return nil
		}
		_, err = c.out.Write(data)
		return err
	}

	text := pkg.RenderText()
	if buildOpts.outputPath != "" {
		if err := writeContextPackage(buildOpts.outputPath, text); err != nil {
			return err
		}
		fmt.Fprintf(c.out, "wrote context package: %s\n", buildOpts.outputPath)
		return nil
	}

	fmt.Fprint(c.out, text)
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
		case arg == "--output":
			i++
			if i >= len(args) {
				return contextBuildOptions{}, &UsageError{Message: "--output requires a path", Code: 2}
			}
			opts.outputPath = args[i]
		case strings.HasPrefix(arg, "--output="):
			opts.outputPath = strings.TrimPrefix(arg, "--output=")
			if opts.outputPath == "" {
				return contextBuildOptions{}, &UsageError{Message: "--output requires a path", Code: 2}
			}
		case arg == "--query":
			i++
			if i >= len(args) {
				return contextBuildOptions{}, &UsageError{Message: "--query requires a value", Code: 2}
			}
			opts.query = args[i]
		case strings.HasPrefix(arg, "--query="):
			opts.query = strings.TrimPrefix(arg, "--query=")
			if opts.query == "" {
				return contextBuildOptions{}, &UsageError{Message: "--query requires a value", Code: 2}
			}
		case arg == "--json":
			opts.json = true
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
	opts.outputPath = strings.TrimSpace(opts.outputPath)
	opts.query = strings.TrimSpace(opts.query)

	return opts, nil
}

func parseContextLimit(value string) (int, error) {
	limit, err := strconv.Atoi(value)
	if err != nil || limit <= 0 {
		return 0, &UsageError{Message: fmt.Sprintf("invalid context retrieval limit: %s", value), Code: 2}
	}
	return limit, nil
}

func writeContextPackage(path, text string) error {
	if !filepath.IsAbs(path) {
		absPath, err := filepath.Abs(path)
		if err != nil {
			return fmt.Errorf("resolve context output path %q: %w", path, err)
		}
		path = absPath
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create context output directory: %w", err)
	}
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		return fmt.Errorf("write context package %q: %w", path, err)
	}
	return nil
}

type contextPackageOutput struct {
	ContractVersion   string                  `json:"contract_version"`
	Project           projectOutput           `json:"project"`
	Task              readyTaskOutput         `json:"task"`
	Dependencies      []taskDependencyOutput  `json:"dependencies"`
	Blockers          []taskDependencyOutput  `json:"blockers"`
	Events            []taskEventOutput       `json:"events"`
	RetrievalResults  []retrievalResultOutput `json:"retrieval_results"`
	RepositoryState   repositoryStateOutput   `json:"repository_state"`
	SuggestedCommands []string                `json:"suggested_commands"`
}

type projectOutput struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	DisplayName string `json:"display_name"`
	Path        string `json:"path"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

type retrievalResultOutput struct {
	Path       string  `json:"path"`
	Score      float64 `json:"score"`
	Line       int     `json:"line"`
	Snippet    string  `json:"snippet"`
	Excerpt    string  `json:"excerpt"`
	Provenance string  `json:"provenance"`
}

type repositoryStateOutput struct {
	Available bool     `json:"available"`
	Branch    string   `json:"branch"`
	Head      string   `json:"head"`
	Status    []string `json:"status"`
	Error     string   `json:"error"`
}

type taskDependencyOutput struct {
	ID            int64  `json:"id"`
	EdgeType      string `json:"edge_type"`
	BlockerTaskID int64  `json:"blocker_task_id"`
	BlockedTaskID int64  `json:"blocked_task_id"`
	CreatedAt     string `json:"created_at"`
}

func contextPackageOutputFromPackage(pkg contextpkg.Package) contextPackageOutput {
	dependencies := make([]taskDependencyOutput, 0, len(pkg.Dependencies))
	for _, dependency := range pkg.Dependencies {
		dependencies = append(dependencies, taskDependencyOutputFromStorage(dependency))
	}

	blockers := make([]taskDependencyOutput, 0, len(pkg.Blockers))
	for _, blocker := range pkg.Blockers {
		blockers = append(blockers, taskDependencyOutputFromStorage(blocker))
	}

	events := make([]taskEventOutput, 0, len(pkg.Events))
	for _, event := range pkg.Events {
		events = append(events, taskEventOutput{
			ID:         event.ID,
			TaskID:     event.TaskID,
			Type:       event.Type,
			Body:       event.Body,
			FromStatus: event.FromStatus,
			ToStatus:   event.ToStatus,
			CreatedAt:  event.CreatedAt,
		})
	}

	results := make([]retrievalResultOutput, 0, len(pkg.Results))
	for _, result := range pkg.Results {
		results = append(results, retrievalResultOutput{
			Path:       result.Path,
			Score:      result.Score,
			Line:       result.Line,
			Snippet:    result.Snippet,
			Excerpt:    result.Excerpt,
			Provenance: result.Provenance,
		})
	}

	return contextPackageOutput{
		ContractVersion: pkg.ContractVersion,
		Project: projectOutput{
			ID:          pkg.Project.ID,
			Name:        pkg.Project.Name,
			DisplayName: pkg.Project.DisplayName,
			Path:        pkg.Project.Path,
			CreatedAt:   pkg.Project.CreatedAt,
			UpdatedAt:   pkg.Project.UpdatedAt,
		},
		Task:             taskOutputFromStorage(pkg.Task),
		Dependencies:     dependencies,
		Blockers:         blockers,
		Events:           events,
		RetrievalResults: results,
		RepositoryState: repositoryStateOutput{
			Available: pkg.Git.Available,
			Branch:    pkg.Git.Branch,
			Head:      pkg.Git.Head,
			Status:    pkg.Git.Status,
			Error:     pkg.Git.Error,
		},
		SuggestedCommands: pkg.SuggestedCommands,
	}
}

func taskDependencyOutputFromStorage(dependency storage.TaskDependency) taskDependencyOutput {
	return taskDependencyOutput{
		ID:            dependency.ID,
		EdgeType:      dependency.EdgeType,
		BlockerTaskID: dependency.BlockerTaskID,
		BlockedTaskID: dependency.BlockedTaskID,
		CreatedAt:     dependency.CreatedAt,
	}
}
