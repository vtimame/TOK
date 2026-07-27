package mcpserver

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	contextpkg "s26.sh/tok/internal/context"
	"s26.sh/tok/internal/retrieval"
	tokservice "s26.sh/tok/internal/service"
	"s26.sh/tok/internal/storage"
)

const defaultVersion = "dev"

type Config struct {
	Store   *storage.Store
	Actor   storage.ActorRef
	Version string
}

type service struct {
	store     *storage.Store
	tasks     *tokservice.TaskService
	runs      *tokservice.RunService
	actor     storage.ActorRef
	retrieval *retrieval.Service
}

func New(cfg Config) (*mcp.Server, error) {
	if cfg.Store == nil {
		return nil, errors.New("mcp store is required")
	}
	actor := sanitizeActor(cfg.Actor)
	if actor.ID <= 0 {
		return nil, errors.New("mcp actor is required")
	}
	version := strings.TrimSpace(cfg.Version)
	if version == "" {
		version = defaultVersion
	}

	svc := &service{
		store:     cfg.Store,
		tasks:     tokservice.NewTaskService(cfg.Store),
		runs:      tokservice.NewRunService(cfg.Store),
		actor:     actor,
		retrieval: retrieval.NewService(cfg.Store),
	}
	server := mcp.NewServer(&mcp.Implementation{Name: "tok", Version: version}, nil)
	svc.addTools(server)
	return server, nil
}

func (s *service) addTools(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "project_create",
		Description: "Register a local project path in TOK.",
	}, s.projectCreate)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "task_create",
		Description: "Create a task in a project.",
	}, s.taskCreate)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "project_list",
		Description: "List registered TOK projects.",
	}, s.projectList)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "project_show",
		Description: "Show a registered TOK project by name.",
	}, s.projectShow)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "task_list",
		Description: "List tasks for a project, optionally filtered by status.",
	}, s.taskList)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "task_show",
		Description: "Show a task and its event history.",
	}, s.taskShow)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "task_ready",
		Description: "List ready tasks for a project.",
	}, s.taskReady)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "task_claim",
		Description: "Claim a specific ready task or the next ready task for a project.",
	}, s.taskClaim)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "task_comment",
		Description: "Add a comment event to a task.",
	}, s.taskComment)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "task_progress",
		Description: "Add a progress event to a task.",
	}, s.taskProgress)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "task_block",
		Description: "Block an open task with a reason.",
	}, s.taskBlock)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "task_unblock",
		Description: "Unblock a blocked task.",
	}, s.taskUnblock)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "task_done",
		Description: "Mark an in-progress task done.",
	}, s.taskDone)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "task_status",
		Description: "Set an arbitrary task status with actor attribution.",
	}, s.taskStatus)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "task_source",
		Description: "Update a task external source reference.",
	}, s.taskSource)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "task_dependency_add",
		Description: "Create a task dependency edge.",
	}, s.taskDependencyAdd)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "task_dependency_remove",
		Description: "Remove a task dependency edge.",
	}, s.taskDependencyRemove)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "project_instruction_list",
		Description: "List project instructions.",
	}, s.projectInstructionList)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "project_instruction_create",
		Description: "Create a project instruction.",
	}, s.projectInstructionCreate)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "project_instruction_show",
		Description: "Show a project instruction.",
	}, s.projectInstructionShow)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "project_instruction_enable",
		Description: "Enable a project instruction.",
	}, s.projectInstructionEnable)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "project_instruction_disable",
		Description: "Disable a project instruction.",
	}, s.projectInstructionDisable)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "project_instruction_delete",
		Description: "Delete a project instruction.",
	}, s.projectInstructionDelete)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "index_update",
		Description: "Update the lexical index for a project.",
	}, s.indexUpdate)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "index_update_all",
		Description: "Update lexical indexes for all projects.",
	}, s.indexUpdateAll)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "index_status",
		Description: "Show index status for a project.",
	}, s.indexStatus)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "index_status_all",
		Description: "Show index status for all projects.",
	}, s.indexStatusAll)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "search",
		Description: "Search indexed project files.",
	}, s.search)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "context_build",
		Description: "Build a structured TOK context package for a task.",
	}, s.contextBuild)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "run_list",
		Description: "List task runs optionally filtered by project/task/status.",
	}, s.runList)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "run_show",
		Description: "Show a run with artifacts.",
	}, s.runShow)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "run_create",
		Description: "Create a new task run.",
	}, s.runCreate)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "run_finish",
		Description: "Finish a task run.",
	}, s.runFinish)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "run_artifact_list",
		Description: "List artifacts for a run.",
	}, s.runArtifactList)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "run_artifact_add",
		Description: "Add a run artifact.",
	}, s.runArtifactAdd)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "run_validation_record",
		Description: "Record validation evidence for a run.",
	}, s.runValidationRecord)
}

type emptyInput struct{}

type projectNameInput struct {
	Project string `json:"project" jsonschema:"project name"`
}

type projectShowInput struct {
	Name string `json:"name" jsonschema:"project name"`
}

type projectCreateInput struct {
	Name        string `json:"name" jsonschema:"project name"`
	DisplayName string `json:"display_name,omitempty" jsonschema:"project display name; defaults to name"`
	Path        string `json:"path" jsonschema:"local project path"`
}

type projectInstructionListInput struct {
	Project         string `json:"project" jsonschema:"project name"`
	IncludeDisabled bool   `json:"include_disabled,omitempty" jsonschema:"include disabled instructions"`
}

type projectInstructionCreateInput struct {
	Project  string `json:"project" jsonschema:"project name"`
	Title    string `json:"title" jsonschema:"instruction title"`
	Body     string `json:"body" jsonschema:"instruction body"`
	Priority string `json:"priority,omitempty" jsonschema:"critical, high, normal or low (default normal)"`
	Source   string `json:"source,omitempty" jsonschema:"instruction source"`
}

type projectInstructionIDInput struct {
	Project string `json:"project" jsonschema:"project name"`
	ID      int64  `json:"id" jsonschema:"instruction id"`
}

type taskIDInput struct {
	ID int64 `json:"id" jsonschema:"task id"`
}

type taskListInput struct {
	Project string `json:"project" jsonschema:"project name"`
	Status  string `json:"status,omitempty" jsonschema:"optional task status: open, in_progress, blocked, or done"`
}

type taskClaimInput struct {
	Project string `json:"project" jsonschema:"project name"`
	ID      int64  `json:"id,omitempty" jsonschema:"optional task id; if omitted, claims the next ready task"`
}

type taskNoteInput struct {
	ID   int64  `json:"id" jsonschema:"task id"`
	Body string `json:"body" jsonschema:"comment or progress body"`
}

type taskCreateInput struct {
	Project            string `json:"project" jsonschema:"project name"`
	Title              string `json:"title" jsonschema:"task title"`
	Description        string `json:"description,omitempty" jsonschema:"task description"`
	AcceptanceCriteria string `json:"acceptance_criteria,omitempty" jsonschema:"acceptance criteria"`
	Notes              string `json:"notes,omitempty" jsonschema:"notes"`
	Source             string `json:"source,omitempty" jsonschema:"task source: local, github, linear, or jira"`
	ExternalID         string `json:"external_id,omitempty" jsonschema:"external tracker issue id or key"`
	ExternalURL        string `json:"external_url,omitempty" jsonschema:"external tracker issue URL"`
	ExternalRevision   string `json:"external_revision,omitempty" jsonschema:"last known external tracker revision"`
}

type taskStatusInput struct {
	ID     int64  `json:"id" jsonschema:"task id"`
	Status string `json:"status" jsonschema:"open, in_progress, blocked, or done"`
}

type taskSourceInput struct {
	ID               int64  `json:"id" jsonschema:"task id"`
	Source           string `json:"source" jsonschema:"task source: local, github, linear, or jira"`
	ExternalID       string `json:"external_id,omitempty" jsonschema:"external tracker issue id or key"`
	ExternalURL      string `json:"external_url,omitempty" jsonschema:"external tracker issue URL"`
	ExternalRevision string `json:"external_revision,omitempty" jsonschema:"last known external tracker revision"`
}

type taskDependencyInput struct {
	EdgeType      string `json:"edge_type,omitempty" jsonschema:"dependency edge type (default blocks)"`
	BlockerTaskID int64  `json:"blocker_task_id" jsonschema:"blocking task id"`
	BlockedTaskID int64  `json:"blocked_task_id" jsonschema:"blocked task id"`
}

type taskBlockInput struct {
	ID     int64  `json:"id" jsonschema:"task id"`
	Reason string `json:"reason" jsonschema:"block reason"`
}

type taskUnblockInput struct {
	ID   int64  `json:"id" jsonschema:"task id"`
	Note string `json:"note" jsonschema:"unblock note"`
}

type taskDoneInput struct {
	ID               int64  `json:"id" jsonschema:"task id"`
	Note             string `json:"note" jsonschema:"completion note"`
	EvidenceRunID    int64  `json:"evidence_run_id,omitempty" jsonschema:"succeeded run with passed validation evidence"`
	AllowUnvalidated bool   `json:"allow_unvalidated,omitempty" jsonschema:"complete without validation evidence"`
	OverrideReason   string `json:"override_reason,omitempty" jsonschema:"required reason for allow_unvalidated"`
}

type searchInput struct {
	Project string `json:"project" jsonschema:"project name"`
	Query   string `json:"query" jsonschema:"search query"`
	Limit   int    `json:"limit,omitempty" jsonschema:"optional positive result limit"`
}

type contextBuildInput struct {
	Project string `json:"project" jsonschema:"project name"`
	TaskID  int64  `json:"task_id" jsonschema:"task id"`
	Limit   int    `json:"limit,omitempty" jsonschema:"optional positive retrieval result limit"`
	Query   string `json:"query,omitempty" jsonschema:"optional retrieval query override"`
}

type runListInput struct {
	Project string `json:"project,omitempty" jsonschema:"optional project name"`
	TaskID  int64  `json:"task_id,omitempty" jsonschema:"optional task id"`
	Status  string `json:"status,omitempty" jsonschema:"optional run status"`
}

type runShowInput struct {
	ID int64 `json:"id" jsonschema:"run id"`
}

type runCreateInput struct {
	TaskID                 int64  `json:"task_id" jsonschema:"task id"`
	Status                 string `json:"status,omitempty" jsonschema:"created or in_progress"`
	HandoffContractVersion string `json:"handoff_contract_version,omitempty" jsonschema:"optional handoff contract version"`
	RetrievalLimit         int    `json:"retrieval_limit,omitempty" jsonschema:"optional retrieval result limit"`
	BaseBranch             string `json:"base_branch,omitempty" jsonschema:"optional base git branch"`
	BaseHead               string `json:"base_head,omitempty" jsonschema:"optional base git head"`
	LeaseOwner             string `json:"lease_owner,omitempty" jsonschema:"optional lease owner"`
	HeartbeatAt            string `json:"heartbeat_at,omitempty" jsonschema:"optional heartbeat timestamp"`
	ExpiresAt              string `json:"expires_at,omitempty" jsonschema:"optional lease expiration timestamp"`
	AllowActive            bool   `json:"allow_active,omitempty" jsonschema:"allow replacing active run"`
}

type runFinishInput struct {
	ID               int64  `json:"id" jsonschema:"run id"`
	Status           string `json:"status" jsonschema:"terminal run status"`
	Summary          string `json:"summary" jsonschema:"result summary"`
	AllowUnvalidated bool   `json:"allow_unvalidated,omitempty" jsonschema:"skip validation requirement"`
	OverrideReason   string `json:"override_reason,omitempty" jsonschema:"required reason for allow_unvalidated"`
}

type runArtifactListInput struct {
	RunID int64 `json:"run_id" jsonschema:"run id"`
}

type runArtifactAddInput struct {
	RunID       int64  `json:"run_id" jsonschema:"run id"`
	Kind        string `json:"kind" jsonschema:"artifact kind"`
	Path        string `json:"path,omitempty" jsonschema:"artifact path"`
	ContentHash string `json:"content_hash,omitempty" jsonschema:"artifact content hash"`
	SizeBytes   int64  `json:"size_bytes,omitempty" jsonschema:"artifact size in bytes"`
	Truncated   bool   `json:"truncated,omitempty" jsonschema:"artifact content truncated"`
	Metadata    string `json:"metadata,omitempty" jsonschema:"artifact metadata json"`
}

type runValidationRecordInput struct {
	RunID   int64  `json:"run_id" jsonschema:"run id"`
	Command string `json:"command" jsonschema:"validation command"`
	Status  string `json:"status" jsonschema:"passed or failed"`
	Summary string `json:"summary" jsonschema:"validation summary"`
}

type projectListOutput struct {
	Projects []ProjectOutput `json:"projects"`
}

type projectOutput struct {
	Project ProjectOutput `json:"project"`
}

type projectInstructionListOutput struct {
	Instructions []ProjectInstructionOutput `json:"instructions"`
}

type projectInstructionShowOutput struct {
	Instruction ProjectInstructionOutput `json:"instruction"`
}

type taskListOutput struct {
	Tasks []TaskOutput `json:"tasks"`
}

type taskOutput struct {
	Task TaskOutput `json:"task"`
}

type taskShowOutput struct {
	Task   TaskOutput        `json:"task"`
	Events []TaskEventOutput `json:"events"`
}

type runListOutput struct {
	Runs []runOutput `json:"runs"`
}

type runOutput struct {
	ID                     int64               `json:"id"`
	TaskID                 int64               `json:"task_id"`
	Status                 string              `json:"status"`
	HandoffContractVersion string              `json:"handoff_contract_version"`
	RetrievalLimit         int                 `json:"retrieval_limit"`
	StartedAt              string              `json:"started_at"`
	FinishedAt             string              `json:"finished_at"`
	BaseBranch             string              `json:"base_branch"`
	BaseHead               string              `json:"base_head"`
	ResultSummary          string              `json:"result_summary"`
	LeaseOwner             string              `json:"lease_owner"`
	HeartbeatAt            string              `json:"heartbeat_at"`
	ExpiresAt              string              `json:"expires_at"`
	StartedBy              *ActorOutput        `json:"started_by,omitempty"`
	FinishedBy             *ActorOutput        `json:"finished_by,omitempty"`
	Artifacts              []runArtifactOutput `json:"artifacts"`
}

type runArtifactOutput struct {
	ID          int64        `json:"id"`
	RunID       int64        `json:"run_id"`
	Kind        string       `json:"kind"`
	Path        string       `json:"path"`
	ContentHash string       `json:"content_hash"`
	SizeBytes   int64        `json:"size_bytes"`
	Truncated   bool         `json:"truncated"`
	Metadata    string       `json:"metadata"`
	Actor       *ActorOutput `json:"actor,omitempty"`
	CreatedAt   string       `json:"created_at"`
}

type runArtifactListOutput struct {
	Artifacts []runArtifactOutput `json:"artifacts"`
}

type taskDependencyRemovedOutput struct {
	Removed       bool   `json:"removed"`
	EdgeType      string `json:"edge_type"`
	BlockerTaskID int64  `json:"blocker_task_id"`
	BlockedTaskID int64  `json:"blocked_task_id"`
}

type taskEventOutput struct {
	Event TaskEventOutput `json:"event"`
}

type indexOutput struct {
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

type indexListOutput struct {
	Indexes []indexOutput `json:"indexes"`
	Total   int           `json:"total"`
}

type searchOutput struct {
	Results []SearchResultOutput `json:"results"`
}

type contextBuildOutput struct {
	ContractVersion     string                     `json:"contract_version"`
	Project             ProjectOutput              `json:"project"`
	Task                TaskOutput                 `json:"task"`
	RetrievalLimit      int                        `json:"retrieval_limit"`
	ProjectInstructions []ProjectInstructionOutput `json:"project_instructions"`
	Dependencies        []TaskDependencyOutput     `json:"dependencies"`
	Blockers            []TaskDependencyOutput     `json:"blockers"`
	Events              []TaskEventOutput          `json:"events"`
	RetrievalResults    []SearchResultOutput       `json:"retrieval_results"`
	RepositoryState     RepositoryStateOutput      `json:"repository_state"`
	SuggestedCommands   []string                   `json:"suggested_commands"`
}

type ProjectOutput struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	DisplayName string `json:"display_name"`
	Path        string `json:"path"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

type ProjectInstructionOutput struct {
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

type TaskOutput struct {
	ID                 int64  `json:"id"`
	ProjectID          int64  `json:"project_id"`
	Status             string `json:"status"`
	Title              string `json:"title"`
	Description        string `json:"description"`
	AcceptanceCriteria string `json:"acceptance_criteria"`
	Notes              string `json:"notes"`
	Source             string `json:"source"`
	ExternalID         string `json:"external_id"`
	ExternalURL        string `json:"external_url"`
	ExternalRevision   string `json:"external_revision"`
	CreatedAt          string `json:"created_at"`
	UpdatedAt          string `json:"updated_at"`
}

type TaskDependencyOutput struct {
	ID            int64  `json:"id"`
	EdgeType      string `json:"edge_type"`
	BlockerTaskID int64  `json:"blocker_task_id"`
	BlockedTaskID int64  `json:"blocked_task_id"`
	CreatedAt     string `json:"created_at"`
}

type RepositoryStateOutput struct {
	Available   bool     `json:"available"`
	Branch      string   `json:"branch"`
	Head        string   `json:"head"`
	Status      []string `json:"status"`
	DiffSummary []string `json:"diff_summary"`
	Error       string   `json:"error"`
}

type ActorOutput struct {
	ID   int64  `json:"id"`
	Kind string `json:"kind"`
	Name string `json:"name"`
}

type TaskEventOutput struct {
	ID                 int64        `json:"id"`
	TaskID             int64        `json:"task_id"`
	Type               string       `json:"type"`
	Body               string       `json:"body"`
	FromStatus         string       `json:"from_status"`
	ToStatus           string       `json:"to_status"`
	EvidenceRunID      int64        `json:"evidence_run_id,omitempty"`
	EvidenceArtifactID int64        `json:"evidence_artifact_id,omitempty"`
	Actor              *ActorOutput `json:"actor,omitempty"`
	CreatedAt          string       `json:"created_at"`
}

type SearchResultOutput struct {
	Path       string  `json:"path"`
	Score      float64 `json:"score"`
	Line       int     `json:"line"`
	LineStart  int     `json:"line_start"`
	LineEnd    int     `json:"line_end"`
	Snippet    string  `json:"snippet"`
	Excerpt    string  `json:"excerpt"`
	Provenance string  `json:"provenance"`
}

func (s *service) projectList(ctx context.Context, _ *mcp.CallToolRequest, _ emptyInput) (*mcp.CallToolResult, projectListOutput, error) {
	projects, err := s.store.ListProjects(ctx)
	if err != nil {
		return nil, projectListOutput{}, err
	}
	out := projectListOutput{Projects: make([]ProjectOutput, 0, len(projects))}
	for _, project := range projects {
		out.Projects = append(out.Projects, projectFromStorage(project))
	}
	return nil, out, nil
}

func (s *service) projectCreate(ctx context.Context, _ *mcp.CallToolRequest, input projectCreateInput) (*mcp.CallToolResult, projectOutput, error) {
	input.Name = strings.TrimSpace(input.Name)
	if input.Name == "" {
		return nil, projectOutput{}, errors.New("project_create requires name")
	}
	if strings.TrimSpace(input.DisplayName) != input.DisplayName {
		return nil, projectOutput{}, errors.New("project display_name cannot have leading or trailing spaces")
	}
	displayName := input.DisplayName
	if displayName == "" {
		displayName = input.Name
	}
	projectPath, err := validateLocalProjectPath(input.Path)
	if err != nil {
		return nil, projectOutput{}, err
	}
	project, err := s.store.CreateProject(ctx, storage.CreateProjectInput{
		Name:        input.Name,
		DisplayName: displayName,
		Path:        projectPath,
	})
	if err != nil {
		return nil, projectOutput{}, fmt.Errorf("create project %q: %w", input.Name, err)
	}
	return nil, projectOutput{Project: projectFromStorage(project)}, nil
}

func (s *service) projectShow(ctx context.Context, _ *mcp.CallToolRequest, input projectShowInput) (*mcp.CallToolResult, projectOutput, error) {
	project, err := s.projectByName(ctx, input.Name)
	if err != nil {
		return nil, projectOutput{}, err
	}
	return nil, projectOutput{Project: projectFromStorage(project)}, nil
}

func (s *service) taskList(ctx context.Context, _ *mcp.CallToolRequest, input taskListInput) (*mcp.CallToolResult, taskListOutput, error) {
	project, err := s.projectByName(ctx, input.Project)
	if err != nil {
		return nil, taskListOutput{}, err
	}
	status := strings.TrimSpace(input.Status)
	if status != "" && !validTaskStatus(status) {
		return nil, taskListOutput{}, fmt.Errorf("invalid task status %q", status)
	}
	tasks, err := s.store.ListTasksWithOptions(ctx, project.ID, storage.ListTasksOptions{Status: status})
	if err != nil {
		return nil, taskListOutput{}, err
	}
	return nil, taskListFromStorage(tasks), nil
}

func (s *service) taskCreate(ctx context.Context, _ *mcp.CallToolRequest, input taskCreateInput) (*mcp.CallToolResult, taskOutput, error) {
	project, err := s.projectByName(ctx, input.Project)
	if err != nil {
		return nil, taskOutput{}, err
	}
	input.Project = strings.TrimSpace(input.Project)
	input.Title = strings.TrimSpace(input.Title)
	if input.Title == "" {
		return nil, taskOutput{}, errors.New("task title is required")
	}
	task, err := s.store.CreateTask(ctx, storage.CreateTaskInput{
		ProjectID:          project.ID,
		Title:              input.Title,
		Description:        strings.TrimSpace(input.Description),
		AcceptanceCriteria: strings.TrimSpace(input.AcceptanceCriteria),
		Notes:              strings.TrimSpace(input.Notes),
		Source:             strings.TrimSpace(input.Source),
		ExternalID:         strings.TrimSpace(input.ExternalID),
		ExternalURL:        strings.TrimSpace(input.ExternalURL),
		ExternalRevision:   strings.TrimSpace(input.ExternalRevision),
		Actor:              s.actor,
	})
	if err != nil {
		return nil, taskOutput{}, err
	}
	return nil, taskOutput{Task: taskFromStorage(task)}, nil
}

func (s *service) taskSource(ctx context.Context, _ *mcp.CallToolRequest, input taskSourceInput) (*mcp.CallToolResult, taskOutput, error) {
	if input.ID <= 0 {
		return nil, taskOutput{}, errors.New("task source requires id")
	}
	task, err := s.store.UpdateTaskExternalReference(ctx, storage.UpdateTaskExternalReferenceInput{
		ID:               input.ID,
		Source:           strings.TrimSpace(input.Source),
		ExternalID:       strings.TrimSpace(input.ExternalID),
		ExternalURL:      strings.TrimSpace(input.ExternalURL),
		ExternalRevision: strings.TrimSpace(input.ExternalRevision),
		Actor:            s.actor,
	})
	if err != nil {
		return nil, taskOutput{}, err
	}
	return nil, taskOutput{Task: taskFromStorage(task)}, nil
}

func (s *service) taskStatus(ctx context.Context, _ *mcp.CallToolRequest, input taskStatusInput) (*mcp.CallToolResult, taskOutput, error) {
	if input.ID <= 0 {
		return nil, taskOutput{}, errors.New("task status requires id")
	}
	input.Status = strings.TrimSpace(input.Status)
	if !validTaskStatus(input.Status) {
		return nil, taskOutput{}, fmt.Errorf("invalid task status %q", input.Status)
	}
	task, err := s.tasks.UpdateStatus(ctx, input.ID, input.Status, s.actor)
	if err != nil {
		return nil, taskOutput{}, friendlyTaskError(err)
	}
	return nil, taskOutput{Task: taskFromStorage(task)}, nil
}

func (s *service) taskDependencyAdd(ctx context.Context, _ *mcp.CallToolRequest, input taskDependencyInput) (*mcp.CallToolResult, TaskDependencyOutput, error) {
	edgeType := strings.TrimSpace(input.EdgeType)
	if edgeType == "" {
		edgeType = "blocks"
	}
	if input.BlockerTaskID <= 0 {
		return nil, TaskDependencyOutput{}, errors.New("blocker_task_id is required")
	}
	if input.BlockedTaskID <= 0 {
		return nil, TaskDependencyOutput{}, errors.New("blocked_task_id is required")
	}
	dependency, err := s.store.AddTaskDependency(ctx, edgeType, input.BlockerTaskID, input.BlockedTaskID)
	if err != nil {
		return nil, TaskDependencyOutput{}, friendlyTaskDependencyError(err)
	}
	return nil, taskDependencyFromStorage(dependency), nil
}

func (s *service) taskDependencyRemove(ctx context.Context, _ *mcp.CallToolRequest, input taskDependencyInput) (*mcp.CallToolResult, taskDependencyRemovedOutput, error) {
	edgeType := strings.TrimSpace(input.EdgeType)
	if edgeType == "" {
		edgeType = "blocks"
	}
	if input.BlockerTaskID <= 0 {
		return nil, taskDependencyRemovedOutput{}, errors.New("blocker_task_id is required")
	}
	if input.BlockedTaskID <= 0 {
		return nil, taskDependencyRemovedOutput{}, errors.New("blocked_task_id is required")
	}
	err := s.store.RemoveTaskDependency(ctx, edgeType, input.BlockerTaskID, input.BlockedTaskID)
	if err != nil {
		return nil, taskDependencyRemovedOutput{}, friendlyTaskDependencyError(err)
	}
	return nil, taskDependencyRemovedOutput{
		Removed:       true,
		EdgeType:      edgeType,
		BlockerTaskID: input.BlockerTaskID,
		BlockedTaskID: input.BlockedTaskID,
	}, nil
}

func (s *service) projectInstructionList(ctx context.Context, _ *mcp.CallToolRequest, input projectInstructionListInput) (*mcp.CallToolResult, projectInstructionListOutput, error) {
	project, err := s.projectByName(ctx, input.Project)
	if err != nil {
		return nil, projectInstructionListOutput{}, err
	}
	instructions, err := s.store.ListProjectInstructions(ctx, storage.ListProjectInstructionsOptions{
		ProjectID:       project.ID,
		IncludeDisabled: input.IncludeDisabled,
	})
	if err != nil {
		return nil, projectInstructionListOutput{}, err
	}
	return nil, projectInstructionListFromStorage(instructions), nil
}

func (s *service) projectInstructionCreate(ctx context.Context, _ *mcp.CallToolRequest, input projectInstructionCreateInput) (*mcp.CallToolResult, projectInstructionShowOutput, error) {
	project, err := s.projectByName(ctx, input.Project)
	if err != nil {
		return nil, projectInstructionShowOutput{}, err
	}
	input.Title = strings.TrimSpace(input.Title)
	if input.Title == "" {
		return nil, projectInstructionShowOutput{}, errors.New("project instruction title is required")
	}
	input.Body = strings.TrimSpace(input.Body)
	if input.Body == "" {
		return nil, projectInstructionShowOutput{}, errors.New("project instruction body is required")
	}
	instruction, err := s.store.CreateProjectInstruction(ctx, storage.CreateProjectInstructionInput{
		ProjectID: project.ID,
		Title:     input.Title,
		Body:      input.Body,
		Priority:  input.Priority,
		Source:    input.Source,
	})
	if err != nil {
		return nil, projectInstructionShowOutput{}, friendlyProjectInstructionError(err)
	}
	return nil, projectInstructionShowOutput{Instruction: projectInstructionFromStorage(instruction)}, nil
}

func (s *service) projectInstructionShow(ctx context.Context, _ *mcp.CallToolRequest, input projectInstructionIDInput) (*mcp.CallToolResult, projectInstructionShowOutput, error) {
	project, err := s.projectByName(ctx, input.Project)
	if err != nil {
		return nil, projectInstructionShowOutput{}, err
	}
	instruction, err := s.store.GetProjectInstruction(ctx, project.ID, input.ID)
	if err != nil {
		return nil, projectInstructionShowOutput{}, friendlyProjectInstructionError(err)
	}
	return nil, projectInstructionShowOutput{Instruction: projectInstructionFromStorage(instruction)}, nil
}

func (s *service) projectInstructionEnable(ctx context.Context, _ *mcp.CallToolRequest, input projectInstructionIDInput) (*mcp.CallToolResult, projectInstructionShowOutput, error) {
	project, err := s.projectByName(ctx, input.Project)
	if err != nil {
		return nil, projectInstructionShowOutput{}, err
	}
	instruction, err := s.store.SetProjectInstructionEnabled(ctx, project.ID, input.ID, true)
	if err != nil {
		return nil, projectInstructionShowOutput{}, friendlyProjectInstructionError(err)
	}
	return nil, projectInstructionShowOutput{Instruction: projectInstructionFromStorage(instruction)}, nil
}

func (s *service) projectInstructionDisable(ctx context.Context, _ *mcp.CallToolRequest, input projectInstructionIDInput) (*mcp.CallToolResult, projectInstructionShowOutput, error) {
	project, err := s.projectByName(ctx, input.Project)
	if err != nil {
		return nil, projectInstructionShowOutput{}, err
	}
	instruction, err := s.store.SetProjectInstructionEnabled(ctx, project.ID, input.ID, false)
	if err != nil {
		return nil, projectInstructionShowOutput{}, friendlyProjectInstructionError(err)
	}
	return nil, projectInstructionShowOutput{Instruction: projectInstructionFromStorage(instruction)}, nil
}

func (s *service) projectInstructionDelete(ctx context.Context, _ *mcp.CallToolRequest, input projectInstructionIDInput) (*mcp.CallToolResult, projectInstructionShowOutput, error) {
	project, err := s.projectByName(ctx, input.Project)
	if err != nil {
		return nil, projectInstructionShowOutput{}, err
	}
	instruction, err := s.store.GetProjectInstruction(ctx, project.ID, input.ID)
	if err != nil {
		return nil, projectInstructionShowOutput{}, friendlyProjectInstructionError(err)
	}
	if err := s.store.DeleteProjectInstruction(ctx, project.ID, input.ID); err != nil {
		return nil, projectInstructionShowOutput{}, friendlyProjectInstructionError(err)
	}
	return nil, projectInstructionShowOutput{Instruction: projectInstructionFromStorage(instruction)}, nil
}

func (s *service) taskShow(ctx context.Context, _ *mcp.CallToolRequest, input taskIDInput) (*mcp.CallToolResult, taskShowOutput, error) {
	task, err := s.taskByID(ctx, input.ID)
	if err != nil {
		return nil, taskShowOutput{}, err
	}
	events, err := s.store.ListTaskEvents(ctx, task.ID)
	if err != nil {
		return nil, taskShowOutput{}, err
	}
	out := taskShowOutput{
		Task:   taskFromStorage(task),
		Events: make([]TaskEventOutput, 0, len(events)),
	}
	for _, event := range events {
		out.Events = append(out.Events, taskEventFromStorage(event))
	}
	return nil, out, nil
}

func (s *service) taskReady(ctx context.Context, _ *mcp.CallToolRequest, input projectNameInput) (*mcp.CallToolResult, taskListOutput, error) {
	project, err := s.projectByName(ctx, input.Project)
	if err != nil {
		return nil, taskListOutput{}, err
	}
	tasks, err := s.store.ListReadyTasks(ctx, project.ID)
	if err != nil {
		return nil, taskListOutput{}, err
	}
	return nil, taskListFromStorage(tasks), nil
}

func (s *service) taskClaim(ctx context.Context, _ *mcp.CallToolRequest, input taskClaimInput) (*mcp.CallToolResult, taskOutput, error) {
	project, err := s.projectByName(ctx, input.Project)
	if err != nil {
		return nil, taskOutput{}, err
	}
	var task storage.Task
	if input.ID > 0 {
		task, err = s.store.ClaimTaskByActor(ctx, project.ID, input.ID, s.actor)
	} else {
		task, err = s.store.ClaimNextReadyTaskByActor(ctx, project.ID, s.actor)
	}
	if err != nil {
		return nil, taskOutput{}, friendlyTaskError(err)
	}
	return nil, taskOutput{Task: taskFromStorage(task)}, nil
}

func (s *service) taskComment(ctx context.Context, _ *mcp.CallToolRequest, input taskNoteInput) (*mcp.CallToolResult, taskEventOutput, error) {
	event, err := s.addTaskNote(ctx, input.ID, input.Body, "comment")
	if err != nil {
		return nil, taskEventOutput{}, err
	}
	return nil, taskEventOutput{Event: taskEventFromStorage(event)}, nil
}

func (s *service) taskProgress(ctx context.Context, _ *mcp.CallToolRequest, input taskNoteInput) (*mcp.CallToolResult, taskEventOutput, error) {
	event, err := s.addTaskNote(ctx, input.ID, input.Body, "progress")
	if err != nil {
		return nil, taskEventOutput{}, err
	}
	return nil, taskEventOutput{Event: taskEventFromStorage(event)}, nil
}

func (s *service) taskBlock(ctx context.Context, _ *mcp.CallToolRequest, input taskBlockInput) (*mcp.CallToolResult, taskOutput, error) {
	if strings.TrimSpace(input.Reason) == "" {
		return nil, taskOutput{}, errors.New("task_block requires reason")
	}
	task, err := s.store.BlockTaskByActor(ctx, input.ID, input.Reason, s.actor)
	if err != nil {
		return nil, taskOutput{}, friendlyTaskError(err)
	}
	return nil, taskOutput{Task: taskFromStorage(task)}, nil
}

func (s *service) taskUnblock(ctx context.Context, _ *mcp.CallToolRequest, input taskUnblockInput) (*mcp.CallToolResult, taskOutput, error) {
	if strings.TrimSpace(input.Note) == "" {
		return nil, taskOutput{}, errors.New("task_unblock requires note")
	}
	task, err := s.store.UnblockTaskByActor(ctx, input.ID, input.Note, s.actor)
	if err != nil {
		return nil, taskOutput{}, friendlyTaskError(err)
	}
	return nil, taskOutput{Task: taskFromStorage(task)}, nil
}

func (s *service) taskDone(ctx context.Context, _ *mcp.CallToolRequest, input taskDoneInput) (*mcp.CallToolResult, taskOutput, error) {
	if strings.TrimSpace(input.Note) == "" {
		return nil, taskOutput{}, errors.New("task_done requires note")
	}
	task, err := s.tasks.CompleteTask(ctx, tokservice.CompleteTaskInput{
		ID:               input.ID,
		Note:             input.Note,
		EvidenceRunID:    input.EvidenceRunID,
		AllowUnvalidated: input.AllowUnvalidated,
		OverrideReason:   input.OverrideReason,
		Actor:            s.actor,
	})
	if err != nil {
		return nil, taskOutput{}, friendlyTaskError(err)
	}
	return nil, taskOutput{Task: taskFromStorage(task)}, nil
}

func (s *service) indexUpdate(ctx context.Context, _ *mcp.CallToolRequest, input projectNameInput) (*mcp.CallToolResult, indexOutput, error) {
	project, err := s.projectByName(ctx, input.Project)
	if err != nil {
		return nil, indexOutput{}, err
	}
	summary, err := s.retrieval.IndexProject(ctx, project)
	if err != nil {
		return nil, indexOutput{}, err
	}
	return nil, indexFromSummary(summary), nil
}

func (s *service) indexStatus(ctx context.Context, _ *mcp.CallToolRequest, input projectNameInput) (*mcp.CallToolResult, indexOutput, error) {
	project, err := s.projectByName(ctx, input.Project)
	if err != nil {
		return nil, indexOutput{}, err
	}
	status, err := s.retrieval.IndexStatus(ctx, project)
	if err != nil {
		return nil, indexOutput{}, err
	}
	return nil, indexFromStatus(status), nil
}

func (s *service) indexUpdateAll(ctx context.Context, _ *mcp.CallToolRequest, _ emptyInput) (*mcp.CallToolResult, indexListOutput, error) {
	projects, err := s.store.ListProjects(ctx)
	if err != nil {
		return nil, indexListOutput{}, err
	}
	out := indexListOutput{Indexes: make([]indexOutput, 0, len(projects))}
	for _, project := range projects {
		summary, err := s.retrieval.IndexProject(ctx, project)
		if err != nil {
			out.Indexes = append(out.Indexes, indexOutput{
				ProjectName:    project.Name,
				State:          retrieval.StateFailed,
				SkippedReasons: map[string]int{},
				LastError:      err.Error(),
			})
			continue
		}
		out.Indexes = append(out.Indexes, indexFromSummary(summary))
	}
	out.Total = len(out.Indexes)
	return nil, out, nil
}

func (s *service) indexStatusAll(ctx context.Context, _ *mcp.CallToolRequest, _ emptyInput) (*mcp.CallToolResult, indexListOutput, error) {
	projects, err := s.store.ListProjects(ctx)
	if err != nil {
		return nil, indexListOutput{}, err
	}
	out := indexListOutput{Indexes: make([]indexOutput, 0, len(projects))}
	for _, project := range projects {
		status, err := s.retrieval.IndexStatus(ctx, project)
		if err != nil {
			out.Indexes = append(out.Indexes, indexOutput{
				ProjectName:    project.Name,
				State:          retrieval.StateFailed,
				SkippedReasons: map[string]int{},
				LastError:      err.Error(),
			})
			continue
		}
		out.Indexes = append(out.Indexes, indexFromStatus(status))
	}
	out.Total = len(out.Indexes)
	return nil, out, nil
}

func (s *service) search(ctx context.Context, _ *mcp.CallToolRequest, input searchInput) (*mcp.CallToolResult, searchOutput, error) {
	project, err := s.projectByName(ctx, input.Project)
	if err != nil {
		return nil, searchOutput{}, err
	}
	query := strings.TrimSpace(input.Query)
	if query == "" {
		return nil, searchOutput{}, errors.New("search requires query")
	}
	limit := input.Limit
	if limit == 0 {
		limit = retrieval.DefaultLimit
	}
	if limit < 0 {
		return nil, searchOutput{}, fmt.Errorf("invalid search limit: %d", input.Limit)
	}
	results, err := s.retrieval.Search(ctx, project, query, limit)
	if err != nil {
		return nil, searchOutput{}, err
	}
	out := searchOutput{Results: make([]SearchResultOutput, 0, len(results))}
	for _, result := range results {
		out.Results = append(out.Results, SearchResultOutput{
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
	return nil, out, nil
}

func (s *service) contextBuild(ctx context.Context, _ *mcp.CallToolRequest, input contextBuildInput) (*mcp.CallToolResult, contextBuildOutput, error) {
	project, err := s.projectByName(ctx, input.Project)
	if err != nil {
		return nil, contextBuildOutput{}, err
	}
	task, err := s.taskByID(ctx, input.TaskID)
	if err != nil {
		return nil, contextBuildOutput{}, err
	}
	limit := input.Limit
	if limit < 0 {
		return nil, contextBuildOutput{}, fmt.Errorf("invalid context retrieval limit: %d", input.Limit)
	}
	pkg, err := contextpkg.NewBuilder(s.store, s.retrieval).Build(ctx, contextpkg.BuildInput{
		Project:        project,
		Task:           task,
		RetrievalLimit: limit,
		Query:          input.Query,
	})
	if err != nil {
		return nil, contextBuildOutput{}, err
	}
	return nil, contextOutputFromPackage(pkg), nil
}

func (s *service) runList(ctx context.Context, _ *mcp.CallToolRequest, input runListInput) (*mcp.CallToolResult, runListOutput, error) {
	if input.Status != "" && !validRunStatus(input.Status) {
		return nil, runListOutput{}, fmt.Errorf("invalid run status %q", input.Status)
	}
	var projectID int64
	if strings.TrimSpace(input.Project) != "" {
		project, err := s.projectByName(ctx, input.Project)
		if err != nil {
			return nil, runListOutput{}, err
		}
		projectID = project.ID
	}
	runs, err := s.store.ListRuns(ctx, storage.ListRunsOptions{ProjectID: projectID, TaskID: input.TaskID, Status: input.Status})
	if err != nil {
		return nil, runListOutput{}, err
	}
	out := runListOutput{Runs: make([]runOutput, 0, len(runs))}
	for _, run := range runs {
		out.Runs = append(out.Runs, runOutputFromStorage(run, nil))
	}
	return nil, out, nil
}

func (s *service) runShow(ctx context.Context, _ *mcp.CallToolRequest, input runShowInput) (*mcp.CallToolResult, runOutput, error) {
	run, err := s.store.GetRun(ctx, input.ID)
	if err != nil {
		return nil, runOutput{}, friendlyRunError(err)
	}
	artifacts, err := s.store.ListRunArtifacts(ctx, input.ID)
	if err != nil {
		return nil, runOutput{}, err
	}
	return nil, runOutputFromStorage(run, artifacts), nil
}

func (s *service) runCreate(ctx context.Context, _ *mcp.CallToolRequest, input runCreateInput) (*mcp.CallToolResult, runOutput, error) {
	input.Status = strings.TrimSpace(input.Status)
	input.HandoffContractVersion = strings.TrimSpace(input.HandoffContractVersion)
	input.BaseBranch = strings.TrimSpace(input.BaseBranch)
	input.BaseHead = strings.TrimSpace(input.BaseHead)
	input.LeaseOwner = strings.TrimSpace(input.LeaseOwner)
	input.HeartbeatAt = strings.TrimSpace(input.HeartbeatAt)
	input.ExpiresAt = strings.TrimSpace(input.ExpiresAt)
	if input.TaskID <= 0 {
		return nil, runOutput{}, errors.New("run_create requires task_id")
	}
	if input.Status != "" && !validRunStatus(input.Status) {
		return nil, runOutput{}, fmt.Errorf("invalid run status %q", input.Status)
	}
	if input.HandoffContractVersion == "" {
		input.HandoffContractVersion = contextpkg.HandoffContractV0
	}
	run, err := s.store.CreateRun(ctx, storage.CreateRunInput{
		TaskID:                 input.TaskID,
		Status:                 input.Status,
		HandoffContractVersion: input.HandoffContractVersion,
		RetrievalLimit:         input.RetrievalLimit,
		BaseBranch:             input.BaseBranch,
		BaseHead:               input.BaseHead,
		LeaseOwner:             input.LeaseOwner,
		HeartbeatAt:            input.HeartbeatAt,
		ExpiresAt:              input.ExpiresAt,
		AllowActive:            input.AllowActive,
		Actor:                  s.actor,
	})
	if err != nil {
		return nil, runOutput{}, friendlyRunError(err)
	}
	return nil, runOutputFromStorage(run, nil), nil
}

func (s *service) runFinish(ctx context.Context, _ *mcp.CallToolRequest, input runFinishInput) (*mcp.CallToolResult, runOutput, error) {
	input.Status = strings.TrimSpace(input.Status)
	input.Summary = strings.TrimSpace(input.Summary)
	if input.ID <= 0 {
		return nil, runOutput{}, errors.New("run_finish requires id")
	}
	if input.Status == "" {
		return nil, runOutput{}, errors.New("run_finish requires status")
	}
	if !runStatusTerminal(input.Status) {
		return nil, runOutput{}, fmt.Errorf("invalid terminal run status %q", input.Status)
	}
	if input.Summary == "" {
		return nil, runOutput{}, errors.New("run_finish requires summary")
	}
	run, err := s.runs.FinishRun(ctx, tokservice.FinishRunInput{
		ID:               input.ID,
		Status:           input.Status,
		ResultSummary:    input.Summary,
		AllowUnvalidated: input.AllowUnvalidated,
		OverrideReason:   input.OverrideReason,
		Actor:            s.actor,
	})
	if err != nil {
		return nil, runOutput{}, friendlyRunError(err)
	}
	artifacts, err := s.store.ListRunArtifacts(ctx, run.ID)
	if err != nil {
		return nil, runOutput{}, err
	}
	return nil, runOutputFromStorage(run, artifacts), nil
}

func (s *service) runArtifactList(ctx context.Context, _ *mcp.CallToolRequest, input runArtifactListInput) (*mcp.CallToolResult, runArtifactListOutput, error) {
	if input.RunID <= 0 {
		return nil, runArtifactListOutput{}, errors.New("run_artifact_list requires run_id")
	}
	artifacts, err := s.store.ListRunArtifacts(ctx, input.RunID)
	if err != nil {
		return nil, runArtifactListOutput{}, err
	}
	out := runArtifactListOutput{Artifacts: make([]runArtifactOutput, 0, len(artifacts))}
	for _, artifact := range artifacts {
		out.Artifacts = append(out.Artifacts, runArtifactFromStorage(artifact))
	}
	return nil, out, nil
}

func (s *service) runArtifactAdd(ctx context.Context, _ *mcp.CallToolRequest, input runArtifactAddInput) (*mcp.CallToolResult, runArtifactOutput, error) {
	if input.RunID <= 0 {
		return nil, runArtifactOutput{}, errors.New("run_artifact_add requires run_id")
	}
	input.Kind = strings.TrimSpace(input.Kind)
	if input.Kind == "" {
		return nil, runArtifactOutput{}, errors.New("run_artifact_add requires kind")
	}
	input.Path = strings.TrimSpace(input.Path)
	input.ContentHash = strings.TrimSpace(input.ContentHash)
	input.Metadata = strings.TrimSpace(input.Metadata)
	addInput := storage.AddRunArtifactInput{
		RunID:       input.RunID,
		Kind:        input.Kind,
		Path:        input.Path,
		ContentHash: input.ContentHash,
		SizeBytes:   input.SizeBytes,
		Truncated:   input.Truncated,
		Metadata:    input.Metadata,
		Actor:       s.actor,
	}
	var (
		artifact storage.RunArtifact
		err      error
	)
	if input.Kind == "validation" {
		artifact, err = s.runs.RecordValidationArtifact(ctx, addInput)
	} else {
		artifact, err = s.store.AddRunArtifact(ctx, addInput)
	}
	if err != nil {
		return nil, runArtifactOutput{}, friendlyRunError(err)
	}
	return nil, runArtifactFromStorage(artifact), nil
}

func (s *service) runValidationRecord(ctx context.Context, _ *mcp.CallToolRequest, input runValidationRecordInput) (*mcp.CallToolResult, runArtifactOutput, error) {
	if input.RunID <= 0 {
		return nil, runArtifactOutput{}, errors.New("run_validation_record requires run_id")
	}
	input.Command = strings.TrimSpace(input.Command)
	if input.Command == "" {
		return nil, runArtifactOutput{}, errors.New("run_validation_record requires command")
	}
	input.Status = strings.TrimSpace(input.Status)
	if input.Status != "passed" && input.Status != "failed" {
		return nil, runArtifactOutput{}, fmt.Errorf("invalid validation status %q", input.Status)
	}
	input.Summary = strings.TrimSpace(input.Summary)
	if input.Summary == "" {
		return nil, runArtifactOutput{}, errors.New("run_validation_record requires summary")
	}
	metadata, err := validationMetadata(input)
	if err != nil {
		return nil, runArtifactOutput{}, err
	}
	artifact, err := s.runs.RecordValidationArtifact(ctx, storage.AddRunArtifactInput{
		RunID:    input.RunID,
		Metadata: metadata,
		Actor:    s.actor,
	})
	if err != nil {
		return nil, runArtifactOutput{}, friendlyRunError(err)
	}
	return nil, runArtifactFromStorage(artifact), nil
}

func (s *service) projectByName(ctx context.Context, name string) (storage.Project, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return storage.Project{}, errors.New("project name is required")
	}
	project, err := s.store.GetProject(ctx, name)
	if errors.Is(err, sql.ErrNoRows) {
		return storage.Project{}, fmt.Errorf("project not found: %s", name)
	}
	return project, err
}

func (s *service) taskByID(ctx context.Context, id int64) (storage.Task, error) {
	if id <= 0 {
		return storage.Task{}, errors.New("task id is required")
	}
	task, err := s.store.GetTask(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return storage.Task{}, fmt.Errorf("task not found: %d", id)
	}
	return task, err
}

func (s *service) addTaskNote(ctx context.Context, id int64, body, kind string) (storage.TaskEvent, error) {
	if id <= 0 {
		return storage.TaskEvent{}, errors.New("task id is required")
	}
	body = strings.TrimSpace(body)
	if body == "" {
		return storage.TaskEvent{}, fmt.Errorf("task_%s requires body", kind)
	}
	var (
		event storage.TaskEvent
		err   error
	)
	switch kind {
	case "comment":
		event, err = s.store.AddTaskCommentByActor(ctx, id, body, s.actor)
	case "progress":
		event, err = s.store.AddTaskProgressByActor(ctx, id, body, s.actor)
	default:
		err = fmt.Errorf("unknown task note kind %q", kind)
	}
	if errors.Is(err, sql.ErrNoRows) {
		return storage.TaskEvent{}, fmt.Errorf("task not found: %d", id)
	}
	return event, err
}

func validateLocalProjectPath(path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", errors.New("project path is required")
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

func taskListFromStorage(tasks []storage.Task) taskListOutput {
	out := taskListOutput{Tasks: make([]TaskOutput, 0, len(tasks))}
	for _, task := range tasks {
		out.Tasks = append(out.Tasks, taskFromStorage(task))
	}
	return out
}

func projectFromStorage(project storage.Project) ProjectOutput {
	return ProjectOutput{
		ID:          project.ID,
		Name:        project.Name,
		DisplayName: project.DisplayName,
		Path:        project.Path,
		CreatedAt:   project.CreatedAt,
		UpdatedAt:   project.UpdatedAt,
	}
}

func taskFromStorage(task storage.Task) TaskOutput {
	return TaskOutput{
		ID:                 task.ID,
		ProjectID:          task.ProjectID,
		Status:             task.Status,
		Title:              task.Title,
		Description:        task.Description,
		AcceptanceCriteria: task.AcceptanceCriteria,
		Notes:              task.Notes,
		Source:             task.Source,
		ExternalID:         task.ExternalID,
		ExternalURL:        task.ExternalURL,
		ExternalRevision:   task.ExternalRevision,
		CreatedAt:          task.CreatedAt,
		UpdatedAt:          task.UpdatedAt,
	}
}

func taskEventFromStorage(event storage.TaskEvent) TaskEventOutput {
	return TaskEventOutput{
		ID:                 event.ID,
		TaskID:             event.TaskID,
		Type:               event.Type,
		Body:               event.Body,
		FromStatus:         event.FromStatus,
		ToStatus:           event.ToStatus,
		EvidenceRunID:      event.EvidenceRunID,
		EvidenceArtifactID: event.EvidenceArtifactID,
		Actor:              actorFromSnapshot(event.ActorID, event.ActorKind, event.ActorName),
		CreatedAt:          event.CreatedAt,
	}
}

func projectInstructionFromStorage(instruction storage.ProjectInstruction) ProjectInstructionOutput {
	return ProjectInstructionOutput{
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

func taskDependencyFromStorage(dependency storage.TaskDependency) TaskDependencyOutput {
	return TaskDependencyOutput{
		ID:            dependency.ID,
		EdgeType:      dependency.EdgeType,
		BlockerTaskID: dependency.BlockerTaskID,
		BlockedTaskID: dependency.BlockedTaskID,
		CreatedAt:     dependency.CreatedAt,
	}
}

func runOutputFromStorage(run storage.Run, artifacts []storage.RunArtifact) runOutput {
	artifactOutputs := make([]runArtifactOutput, 0, len(artifacts))
	for _, artifact := range artifacts {
		artifactOutputs = append(artifactOutputs, runArtifactFromStorage(artifact))
	}
	return runOutput{
		ID:                     run.ID,
		TaskID:                 run.TaskID,
		Status:                 run.Status,
		HandoffContractVersion: run.HandoffContractVersion,
		RetrievalLimit:         run.RetrievalLimit,
		StartedAt:              run.StartedAt,
		FinishedAt:             run.FinishedAt,
		BaseBranch:             run.BaseBranch,
		BaseHead:               run.BaseHead,
		ResultSummary:          run.ResultSummary,
		LeaseOwner:             run.LeaseOwner,
		HeartbeatAt:            run.HeartbeatAt,
		ExpiresAt:              run.ExpiresAt,
		StartedBy:              actorFromSnapshot(run.ActorID, run.ActorKind, run.ActorName),
		FinishedBy:             actorFromSnapshot(run.FinishedActorID, run.FinishedActorKind, run.FinishedActorName),
		Artifacts:              artifactOutputs,
	}
}

func runArtifactFromStorage(artifact storage.RunArtifact) runArtifactOutput {
	return runArtifactOutput{
		ID:          artifact.ID,
		RunID:       artifact.RunID,
		Kind:        artifact.Kind,
		Path:        artifact.Path,
		ContentHash: artifact.ContentHash,
		SizeBytes:   artifact.SizeBytes,
		Truncated:   artifact.Truncated,
		Metadata:    artifact.Metadata,
		Actor:       actorFromSnapshot(artifact.ActorID, artifact.ActorKind, artifact.ActorName),
		CreatedAt:   artifact.CreatedAt,
	}
}

func projectInstructionListFromStorage(instructions []storage.ProjectInstruction) projectInstructionListOutput {
	out := projectInstructionListOutput{Instructions: make([]ProjectInstructionOutput, 0, len(instructions))}
	for _, instruction := range instructions {
		out.Instructions = append(out.Instructions, projectInstructionFromStorage(instruction))
	}
	return out
}

func validationMetadata(input runValidationRecordInput) (string, error) {
	raw, err := json.Marshal(struct {
		Command          string `json:"command"`
		Status           string `json:"status"`
		Summary          string `json:"summary"`
		RedactionEnabled bool   `json:"redaction_enabled"`
	}{
		Command:          input.Command,
		Status:           input.Status,
		Summary:          input.Summary,
		RedactionEnabled: false,
	})
	if err != nil {
		return "", fmt.Errorf("encode validation metadata: %w", err)
	}
	return string(raw), nil
}

func searchResultFromRetrieval(result retrieval.SearchResult) SearchResultOutput {
	return SearchResultOutput{
		Path:       result.Path,
		Score:      result.Score,
		Line:       result.Line,
		LineStart:  result.LineStart,
		LineEnd:    result.LineEnd,
		Snippet:    result.Snippet,
		Excerpt:    result.Excerpt,
		Provenance: result.Provenance,
	}
}

func contextOutputFromPackage(pkg contextpkg.Package) contextBuildOutput {
	instructions := make([]ProjectInstructionOutput, 0, len(pkg.ProjectInstructions))
	for _, instruction := range pkg.ProjectInstructions {
		instructions = append(instructions, projectInstructionFromStorage(instruction))
	}
	dependencies := make([]TaskDependencyOutput, 0, len(pkg.Dependencies))
	for _, dependency := range pkg.Dependencies {
		dependencies = append(dependencies, taskDependencyFromStorage(dependency))
	}
	blockers := make([]TaskDependencyOutput, 0, len(pkg.Blockers))
	for _, blocker := range pkg.Blockers {
		blockers = append(blockers, taskDependencyFromStorage(blocker))
	}
	events := make([]TaskEventOutput, 0, len(pkg.Events))
	for _, event := range pkg.Events {
		events = append(events, taskEventFromStorage(event))
	}
	results := make([]SearchResultOutput, 0, len(pkg.Results))
	for _, result := range pkg.Results {
		results = append(results, searchResultFromRetrieval(result))
	}

	return contextBuildOutput{
		ContractVersion:     pkg.ContractVersion,
		Project:             projectFromStorage(pkg.Project),
		Task:                taskFromStorage(pkg.Task),
		RetrievalLimit:      pkg.RetrievalLimit,
		ProjectInstructions: instructions,
		Dependencies:        dependencies,
		Blockers:            blockers,
		Events:              events,
		RetrievalResults:    results,
		RepositoryState: RepositoryStateOutput{
			Available:   pkg.Git.Available,
			Branch:      pkg.Git.Branch,
			Head:        pkg.Git.Head,
			Status:      pkg.Git.Status,
			DiffSummary: pkg.Git.DiffSummary,
			Error:       pkg.Git.Error,
		},
		SuggestedCommands: pkg.SuggestedCommands,
	}
}

func actorFromSnapshot(id int64, kind, name string) *ActorOutput {
	if id <= 0 || kind == "" || name == "" {
		return nil
	}
	return &ActorOutput{ID: id, Kind: kind, Name: name}
}

func indexFromSummary(summary retrieval.IndexSummary) indexOutput {
	return indexOutput{
		ProjectName:      summary.ProjectName,
		State:            summary.State,
		PathExists:       summary.PathExists,
		IndexedDocuments: summary.IndexedDocuments,
		IndexedChunks:    summary.IndexedChunks,
		SkippedFiles:     summary.SkippedFiles,
		SkippedReasons:   summary.SkippedReasons,
		UpdatedAt:        summary.UpdatedAt,
		LastError:        summary.LastError,
	}
}

func indexFromStatus(status retrieval.IndexStatus) indexOutput {
	return indexOutput{
		ProjectName:      status.ProjectName,
		State:            status.State,
		PathExists:       status.PathExists,
		IndexedDocuments: status.IndexedDocuments,
		IndexedChunks:    status.IndexedChunks,
		SkippedFiles:     status.SkippedFiles,
		SkippedReasons:   status.SkippedReasons,
		UpdatedAt:        status.UpdatedAt,
		LastError:        status.LastError,
	}
}

func friendlyTaskError(err error) error {
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return errors.New("task not found")
	case errors.Is(err, storage.ErrNoReadyTask):
		return errors.New("no ready tasks")
	case errors.Is(err, storage.ErrTaskNotReady):
		return errors.New("task is not ready")
	case errors.Is(err, storage.ErrInvalidTaskTransition):
		return errors.New("invalid task status transition")
	case errors.Is(err, storage.ErrActiveRunExists):
		return errors.New("task cannot be completed while an active run exists")
	case errors.Is(err, tokservice.ErrTaskCompletionEvidenceRequired):
		return errors.New("task completion evidence run with passed validation is required")
	case errors.Is(err, tokservice.ErrOverrideReasonRequired):
		return errors.New("override reason is required")
	default:
		return err
	}
}

func friendlyRunError(err error) error {
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return errors.New("run not found")
	case errors.Is(err, tokservice.ErrRunValidationRequired):
		return errors.New("passed validation evidence is required")
	case errors.Is(err, tokservice.ErrOverrideReasonRequired):
		return errors.New("override reason is required")
	case errors.Is(err, storage.ErrRunResultSummaryEmpty):
		return errors.New("run result summary is required")
	case errors.Is(err, storage.ErrInvalidRunTransition):
		return errors.New("invalid run status transition")
	default:
		return err
	}
}

func friendlyProjectInstructionError(err error) error {
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return errors.New("project instruction not found")
	case strings.Contains(err.Error(), "project instruction"):
		return err
	default:
		return err
	}
}

func friendlyTaskDependencyError(err error) error {
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return errors.New("task dependency not found")
	case strings.Contains(err.Error(), "task dependency"):
		return err
	default:
		return err
	}
}

func sanitizeActor(actor storage.ActorRef) storage.ActorRef {
	actor.Kind = strings.TrimSpace(actor.Kind)
	actor.Name = strings.TrimSpace(actor.Name)
	if actor.ID <= 0 || actor.Kind == "" || actor.Name == "" {
		return storage.ActorRef{}
	}
	return actor
}

func validTaskStatus(status string) bool {
	switch status {
	case "open", "in_progress", "blocked", "done":
		return true
	default:
		return false
	}
}

func validRunStatus(status string) bool {
	switch status {
	case "created", "in_progress", "succeeded", "failed", "blocked", "cancelled":
		return true
	default:
		return false
	}
}

func runStatusTerminal(status string) bool {
	switch status {
	case "succeeded", "failed", "blocked", "cancelled":
		return true
	default:
		return false
	}
}
