package context

import (
	"bytes"
	stdctx "context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"s26.sh/tok/internal/retrieval"
	"s26.sh/tok/internal/storage"
)

const DefaultRetrievalLimit = 5

type Store interface {
	ListTaskEvents(ctx stdctx.Context, taskID int64) ([]storage.TaskEvent, error)
}

type Builder struct {
	store     Store
	retrieval *retrieval.Service
	git       GitInspector
}

type BuildInput struct {
	Project        storage.Project
	Task           storage.Task
	RetrievalLimit int
	Query          string
}

type Package struct {
	Project storage.Project
	Task    storage.Task
	Events  []storage.TaskEvent
	Results []retrieval.SearchResult
	Git     GitState
}

type GitInspector interface {
	Inspect(ctx stdctx.Context, path string) GitState
}

type GitState struct {
	Available bool
	Branch    string
	Head      string
	Status    []string
	Error     string
}

func NewBuilder(store Store, retrievalService *retrieval.Service) *Builder {
	return &Builder{
		store:     store,
		retrieval: retrievalService,
		git:       CommandGitInspector{Timeout: 2 * time.Second},
	}
}

func (b *Builder) Build(ctx stdctx.Context, input BuildInput) (Package, error) {
	if b == nil || b.store == nil || b.retrieval == nil {
		return Package{}, errors.New("context package builder dependencies are required")
	}
	if input.Project.ID <= 0 {
		return Package{}, errors.New("project id is required")
	}
	if input.Task.ID <= 0 {
		return Package{}, errors.New("task id is required")
	}
	if input.Task.ProjectID != input.Project.ID {
		return Package{}, errors.New("task does not belong to project")
	}
	if input.RetrievalLimit <= 0 {
		input.RetrievalLimit = DefaultRetrievalLimit
	}

	events, err := b.store.ListTaskEvents(ctx, input.Task.ID)
	if err != nil {
		return Package{}, err
	}

	query := strings.TrimSpace(input.Query)
	if query == "" {
		query = taskRetrievalQuery(input.Task)
	}

	results, err := b.retrieval.Search(ctx, input.Project, query, input.RetrievalLimit)
	if err != nil {
		return Package{}, err
	}

	return Package{
		Project: input.Project,
		Task:    input.Task,
		Events:  events,
		Results: results,
		Git:     b.git.Inspect(ctx, input.Project.Path),
	}, nil
}

func (p Package) RenderText() string {
	var out strings.Builder

	out.WriteString("# TOK Context Package\n\n")

	out.WriteString("## Project\n")
	fmt.Fprintf(&out, "id: %d\n", p.Project.ID)
	fmt.Fprintf(&out, "name: %s\n", p.Project.Name)
	fmt.Fprintf(&out, "display_name: %s\n", p.Project.DisplayName)
	fmt.Fprintf(&out, "path: %s\n", p.Project.Path)
	fmt.Fprintf(&out, "created_at: %s\n", p.Project.CreatedAt)
	fmt.Fprintf(&out, "updated_at: %s\n\n", p.Project.UpdatedAt)

	out.WriteString("## Task\n")
	fmt.Fprintf(&out, "id: %d\n", p.Task.ID)
	fmt.Fprintf(&out, "project_id: %d\n", p.Task.ProjectID)
	fmt.Fprintf(&out, "status: %s\n", p.Task.Status)
	fmt.Fprintf(&out, "title: %s\n", p.Task.Title)
	writeBlock(&out, "description", p.Task.Description)
	writeBlock(&out, "acceptance_criteria", p.Task.AcceptanceCriteria)
	writeBlock(&out, "notes", p.Task.Notes)
	fmt.Fprintf(&out, "created_at: %s\n", p.Task.CreatedAt)
	fmt.Fprintf(&out, "updated_at: %s\n\n", p.Task.UpdatedAt)

	out.WriteString("## Task Events\n")
	if len(p.Events) == 0 {
		out.WriteString("none\n\n")
	} else {
		for _, event := range p.Events {
			fmt.Fprintf(&out, "- id: %d type: %s", event.ID, event.Type)
			if event.FromStatus != "" {
				fmt.Fprintf(&out, " from: %s", event.FromStatus)
			}
			if event.ToStatus != "" {
				fmt.Fprintf(&out, " to: %s", event.ToStatus)
			}
			if event.Body != "" {
				fmt.Fprintf(&out, " body: %s", compactLine(event.Body))
			}
			fmt.Fprintf(&out, " created_at: %s\n", event.CreatedAt)
		}
		out.WriteString("\n")
	}

	out.WriteString("## Retrieval Results\n")
	if len(p.Results) == 0 {
		out.WriteString("none\n\n")
	} else {
		for idx, result := range p.Results {
			fmt.Fprintf(&out, "%d. path: %s\n", idx+1, result.Path)
			fmt.Fprintf(&out, "   line: %d\n", result.Line)
			fmt.Fprintf(&out, "   score: %.6f\n", result.Score)
			fmt.Fprintf(&out, "   provenance: %s\n", result.Provenance)
			fmt.Fprintf(&out, "   snippet: %s\n", result.Snippet)
			if result.Excerpt != "" {
				out.WriteString("   excerpt:\n")
				for _, line := range strings.Split(result.Excerpt, "\n") {
					fmt.Fprintf(&out, "     %s\n", line)
				}
			}
		}
		out.WriteString("\n")
	}

	out.WriteString("## Repository State\n")
	fmt.Fprintf(&out, "available: %t\n", p.Git.Available)
	if p.Git.Available {
		fmt.Fprintf(&out, "branch: %s\n", p.Git.Branch)
		fmt.Fprintf(&out, "head: %s\n", p.Git.Head)
		out.WriteString("status:\n")
		if len(p.Git.Status) == 0 {
			out.WriteString("- clean\n")
		} else {
			for _, line := range p.Git.Status {
				fmt.Fprintf(&out, "- %s\n", line)
			}
		}
	} else if p.Git.Error != "" {
		fmt.Fprintf(&out, "error: %s\n", p.Git.Error)
	}

	return out.String()
}

type CommandGitInspector struct {
	Timeout time.Duration
}

func (g CommandGitInspector) Inspect(ctx stdctx.Context, path string) GitState {
	if strings.TrimSpace(path) == "" {
		return GitState{Available: false, Error: "project path is empty"}
	}

	timeout := g.Timeout
	if timeout <= 0 {
		timeout = 2 * time.Second
	}
	ctx, cancel := stdctx.WithTimeout(ctx, timeout)
	defer cancel()

	if _, ok := gitOutput(ctx, path, "rev-parse", "--is-inside-work-tree"); !ok {
		return GitState{Available: false, Error: "not a git worktree"}
	}

	branch, _ := gitOutput(ctx, path, "branch", "--show-current")
	head, headOK := gitOutput(ctx, path, "rev-parse", "--short", "HEAD")
	status, _ := gitOutput(ctx, path, "status", "--short")
	if !headOK {
		return GitState{Available: false, Error: "git head is unavailable"}
	}

	var statusLines []string
	for _, line := range strings.Split(status, "\n") {
		line = strings.TrimRight(line, "\r")
		if line != "" {
			statusLines = append(statusLines, line)
		}
	}

	return GitState{
		Available: true,
		Branch:    branch,
		Head:      head,
		Status:    statusLines,
	}
}

func gitOutput(ctx stdctx.Context, path string, args ...string) (string, bool) {
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", path}, args...)...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return "", false
	}
	return strings.TrimSpace(string(out)), true
}

func taskRetrievalQuery(task storage.Task) string {
	return strings.TrimSpace(strings.Join([]string{
		task.Title,
		task.Description,
		task.AcceptanceCriteria,
		task.Notes,
	}, " "))
}

func writeBlock(out *strings.Builder, label, value string) {
	fmt.Fprintf(out, "%s:\n", label)
	if value == "" {
		out.WriteString("  \n")
		return
	}
	for _, line := range strings.Split(value, "\n") {
		fmt.Fprintf(out, "  %s\n", line)
	}
}

func compactLine(value string) string {
	return strings.Join(strings.Fields(value), " ")
}
