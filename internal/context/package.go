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

const (
	DefaultRetrievalLimit = 5
	HandoffContractV0     = "tok.handoff.v0"
)

type Store interface {
	ListTaskEvents(ctx stdctx.Context, taskID int64) ([]storage.TaskEvent, error)
	ListTaskDependencies(ctx stdctx.Context, projectID, taskID int64) ([]storage.TaskDependency, error)
	ListProjectInstructions(ctx stdctx.Context, opts storage.ListProjectInstructionsOptions) ([]storage.ProjectInstruction, error)
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
	ContractVersion     string
	Project             storage.Project
	Task                storage.Task
	RetrievalLimit      int
	Dependencies        []storage.TaskDependency
	Blockers            []storage.TaskDependency
	Events              []storage.TaskEvent
	ProjectInstructions []storage.ProjectInstruction
	Results             []retrieval.SearchResult
	Git                 GitState
	SuggestedCommands   []string
}

type GitInspector interface {
	Inspect(ctx stdctx.Context, path string) GitState
}

type GitState struct {
	Available   bool
	Branch      string
	Head        string
	Status      []string
	DiffSummary []string
	Error       string
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
	dependencies, err := b.store.ListTaskDependencies(ctx, input.Project.ID, input.Task.ID)
	if err != nil {
		return Package{}, err
	}
	instructions, err := b.store.ListProjectInstructions(ctx, storage.ListProjectInstructionsOptions{ProjectID: input.Project.ID})
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
		ContractVersion:     HandoffContractV0,
		Project:             input.Project,
		Task:                input.Task,
		RetrievalLimit:      input.RetrievalLimit,
		Dependencies:        dependencies,
		Blockers:            blockersForTask(input.Task.ID, dependencies),
		Events:              events,
		ProjectInstructions: instructions,
		Results:             results,
		Git:                 b.git.Inspect(ctx, input.Project.Path),
		SuggestedCommands:   suggestedCommands(input.Project, input.Task),
	}, nil
}

func (p Package) RenderText() string {
	var out strings.Builder

	out.WriteString("# TOK Context Package\n\n")

	out.WriteString("## Handoff Contract\n")
	fmt.Fprintf(&out, "contract_version: %s\n", p.ContractVersion)
	fmt.Fprintf(&out, "retrieval_limit: %d\n", p.RetrievalLimit)
	out.WriteString("\n")

	out.WriteString("## Task\n")
	fmt.Fprintf(&out, "id: %d\n", p.Task.ID)
	fmt.Fprintf(&out, "project_id: %d\n", p.Task.ProjectID)
	fmt.Fprintf(&out, "status: %s\n", p.Task.Status)
	fmt.Fprintf(&out, "title: %s\n", p.Task.Title)
	writeBlock(&out, "description", p.Task.Description)
	writeBlock(&out, "acceptance_criteria", p.Task.AcceptanceCriteria)
	writeBlock(&out, "notes", p.Task.Notes)
	fmt.Fprintf(&out, "source: %s\n", p.Task.Source)
	if p.Task.ExternalID != "" {
		fmt.Fprintf(&out, "external_id: %s\n", p.Task.ExternalID)
	}
	if p.Task.ExternalURL != "" {
		fmt.Fprintf(&out, "external_url: %s\n", p.Task.ExternalURL)
	}
	if p.Task.ExternalRevision != "" {
		fmt.Fprintf(&out, "external_revision: %s\n", p.Task.ExternalRevision)
	}
	fmt.Fprintf(&out, "created_at: %s\n", p.Task.CreatedAt)
	fmt.Fprintf(&out, "updated_at: %s\n\n", p.Task.UpdatedAt)

	out.WriteString("## Current State\n")
	fmt.Fprintf(&out, "task_status: %s\n", p.Task.Status)
	fmt.Fprintf(&out, "active_blockers: %d\n", len(p.Blockers))
	fmt.Fprintf(&out, "repository_available: %t\n", p.Git.Available)
	if p.Git.Available {
		fmt.Fprintf(&out, "branch: %s\n", p.Git.Branch)
		fmt.Fprintf(&out, "head: %s\n", p.Git.Head)
	}
	out.WriteString("\n")

	out.WriteString("## Project\n")
	fmt.Fprintf(&out, "id: %d\n", p.Project.ID)
	fmt.Fprintf(&out, "name: %s\n", p.Project.Name)
	fmt.Fprintf(&out, "display_name: %s\n", p.Project.DisplayName)
	fmt.Fprintf(&out, "path: %s\n", p.Project.Path)
	fmt.Fprintf(&out, "created_at: %s\n", p.Project.CreatedAt)
	fmt.Fprintf(&out, "updated_at: %s\n\n", p.Project.UpdatedAt)

	out.WriteString("## Project Instructions\n")
	if len(p.ProjectInstructions) == 0 {
		out.WriteString("none\n\n")
	} else {
		for _, instruction := range p.ProjectInstructions {
			fmt.Fprintf(&out, "- id: %d priority: %s title: %s\n", instruction.ID, instruction.Priority, instruction.Title)
			fmt.Fprintf(&out, "  scope: %s\n", instruction.Scope)
			fmt.Fprintf(&out, "  source: %s\n", instruction.Source)
			out.WriteString("  body:\n")
			for _, line := range strings.Split(instruction.Body, "\n") {
				fmt.Fprintf(&out, "    %s\n", line)
			}
		}
		out.WriteString("\n")
	}

	out.WriteString("## Task Dependencies\n")
	if len(p.Dependencies) == 0 {
		out.WriteString("none\n\n")
	} else {
		for _, dependency := range p.Dependencies {
			fmt.Fprintf(&out, "- id: %d edge_type: %s blocker_task_id: %d blocked_task_id: %d",
				dependency.ID,
				dependency.EdgeType,
				dependency.BlockerTaskID,
				dependency.BlockedTaskID,
			)
			if dependency.BlockedTaskID == p.Task.ID {
				out.WriteString(" role: blocker")
			}
			if dependency.BlockerTaskID == p.Task.ID {
				out.WriteString(" role: blocks")
			}
			fmt.Fprintf(&out, " created_at: %s\n", dependency.CreatedAt)
		}
		out.WriteString("\n")
	}

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

	out.WriteString("## Relevant Files\n")
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
		out.WriteString("diff_summary:\n")
		if len(p.Git.DiffSummary) == 0 {
			out.WriteString("- none\n")
		} else {
			for _, line := range p.Git.DiffSummary {
				fmt.Fprintf(&out, "- %s\n", line)
			}
		}
	} else if p.Git.Error != "" {
		fmt.Fprintf(&out, "error: %s\n", p.Git.Error)
	}
	out.WriteString("\n\n")

	out.WriteString("## Commands\n")
	if len(p.SuggestedCommands) == 0 {
		out.WriteString("- none\n")
	} else {
		for _, command := range p.SuggestedCommands {
			fmt.Fprintf(&out, "- %s\n", command)
		}
	}
	out.WriteString("\n")

	out.WriteString("## Open Questions\n")
	out.WriteString("none\n")

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
	diffSummary := gitDiffSummary(ctx, path)
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
		Available:   true,
		Branch:      branch,
		Head:        head,
		Status:      statusLines,
		DiffSummary: diffSummary,
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

func gitDiffSummary(ctx stdctx.Context, path string) []string {
	var lines []string

	if staged, ok := gitOutput(ctx, path, "diff", "--cached", "--stat", "--"); ok {
		lines = appendDiffSummary(lines, "staged", staged)
	}
	if unstaged, ok := gitOutput(ctx, path, "diff", "--stat", "--"); ok {
		lines = appendDiffSummary(lines, "unstaged", unstaged)
	}

	return lines
}

func appendDiffSummary(lines []string, label, output string) []string {
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			lines = append(lines, label+": "+line)
		}
	}
	return lines
}

func taskRetrievalQuery(task storage.Task) string {
	return strings.TrimSpace(strings.Join([]string{
		task.Title,
		task.Description,
		task.AcceptanceCriteria,
		task.Notes,
		task.Source,
		task.ExternalID,
		task.ExternalURL,
	}, " "))
}

func blockersForTask(taskID int64, dependencies []storage.TaskDependency) []storage.TaskDependency {
	blockers := make([]storage.TaskDependency, 0)
	for _, dependency := range dependencies {
		if dependency.EdgeType == "blocks" && dependency.BlockedTaskID == taskID {
			blockers = append(blockers, dependency)
		}
	}
	return blockers
}

func suggestedCommands(project storage.Project, task storage.Task) []string {
	taskID := fmt.Sprintf("%d", task.ID)
	return []string{
		"tok task show " + taskID + " --json",
		"tok context build --project " + project.Name + " --task " + taskID + " --output context.md",
		"tok task comment " + taskID + " --body \"Progress update.\"",
		"tok task done " + taskID + " --note \"Done, tests pass.\" --evidence-run <run-id>",
	}
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
