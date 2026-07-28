package mcpserver

import (
	"errors"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"s26.sh/tok/internal/retrieval"
	tokservice "s26.sh/tok/internal/service"
	"s26.sh/tok/internal/storage"
)

const defaultVersion = "dev"

type Profile string

const (
	ProfileAll        Profile = ""
	ProfileWorker     Profile = "worker"
	ProfileSupervisor Profile = "supervisor"
	ProfileAdmin      Profile = "admin"
)

type Config struct {
	Store   *storage.Store
	Actor   storage.ActorRef
	Version string
	Profile Profile
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
	profile, err := NormalizeProfile(cfg.Profile)
	if err != nil {
		return nil, err
	}
	server := mcp.NewServer(&mcp.Implementation{Name: "tok", Version: version}, nil)
	svc.addTools(server, profile)
	return server, nil
}

func NormalizeProfile(profile Profile) (Profile, error) {
	switch Profile(strings.TrimSpace(string(profile))) {
	case ProfileAll, "all", "default":
		return ProfileAll, nil
	case ProfileWorker:
		return ProfileWorker, nil
	case ProfileSupervisor:
		return ProfileSupervisor, nil
	case ProfileAdmin:
		return ProfileAdmin, nil
	default:
		return ProfileAll, fmt.Errorf("invalid MCP profile %q", profile)
	}
}

func (s *service) addTools(server *mcp.Server, profile Profile) {
	addTool(server, profile, []Profile{ProfileAdmin}, &mcp.Tool{
		Name:        "project_create",
		Description: "Register a local project path in TOK.",
	}, s.projectCreate)
	addTool(server, profile, []Profile{ProfileAdmin}, &mcp.Tool{
		Name:        "agent_create",
		Description: "Create a local agent identity and token.",
	}, s.agentCreate)
	addTool(server, profile, []Profile{ProfileAdmin}, &mcp.Tool{
		Name:        "agent_list",
		Description: "List local agent identities.",
	}, s.agentList)
	addTool(server, profile, []Profile{ProfileAdmin}, &mcp.Tool{
		Name:        "agent_revoke",
		Description: "Revoke a local agent token.",
	}, s.agentRevoke)
	addTool(server, profile, []Profile{ProfileAdmin}, &mcp.Tool{
		Name:        "task_create",
		Description: "Create a task in a project.",
	}, s.taskCreate)
	addTool(server, profile, []Profile{ProfileAdmin}, &mcp.Tool{
		Name:        "project_list",
		Description: "List registered TOK projects.",
	}, s.projectList)
	addTool(server, profile, []Profile{ProfileAdmin}, &mcp.Tool{
		Name:        "project_show",
		Description: "Show a registered TOK project by name.",
	}, s.projectShow)
	addTool(server, profile, []Profile{ProfileSupervisor, ProfileAdmin}, &mcp.Tool{
		Name:        "task_list",
		Description: "List tasks for a project, optionally filtered by status.",
	}, s.taskList)
	addTool(server, profile, []Profile{ProfileWorker, ProfileSupervisor, ProfileAdmin}, &mcp.Tool{
		Name:        "task_show",
		Description: "Show a task and its event history.",
	}, s.taskShow)
	addTool(server, profile, []Profile{ProfileWorker, ProfileSupervisor, ProfileAdmin}, &mcp.Tool{
		Name:        "task_ready",
		Description: "List ready tasks for a project.",
	}, s.taskReady)
	addTool(server, profile, []Profile{ProfileWorker, ProfileSupervisor, ProfileAdmin}, &mcp.Tool{
		Name:        "task_claim",
		Description: "Claim a specific ready task or the next ready task for a project.",
	}, s.taskClaim)
	addTool(server, profile, []Profile{ProfileSupervisor, ProfileAdmin}, &mcp.Tool{
		Name:        "task_comment",
		Description: "Add a comment event to a task.",
	}, s.taskComment)
	addTool(server, profile, []Profile{ProfileWorker, ProfileSupervisor, ProfileAdmin}, &mcp.Tool{
		Name:        "task_progress",
		Description: "Add a progress event to a task.",
	}, s.taskProgress)
	addTool(server, profile, []Profile{ProfileWorker, ProfileSupervisor, ProfileAdmin}, &mcp.Tool{
		Name:        "task_block",
		Description: "Block an open task with a reason.",
	}, s.taskBlock)
	addTool(server, profile, []Profile{ProfileSupervisor, ProfileAdmin}, &mcp.Tool{
		Name:        "task_unblock",
		Description: "Unblock a blocked task.",
	}, s.taskUnblock)
	addTool(server, profile, []Profile{ProfileWorker, ProfileSupervisor, ProfileAdmin}, &mcp.Tool{
		Name:        "task_done",
		Description: "Complete an in-progress task with validation evidence or an audited override.",
	}, s.taskDone)
	addTool(server, profile, []Profile{ProfileSupervisor, ProfileAdmin}, &mcp.Tool{
		Name:        "task_status",
		Description: "Set a task status to open, in_progress or blocked. Use task_done to complete work with validation evidence or an audited override.",
	}, s.taskStatus)
	addTool(server, profile, []Profile{ProfileAdmin}, &mcp.Tool{
		Name:        "task_source",
		Description: "Update a task external source reference.",
	}, s.taskSource)
	addTool(server, profile, []Profile{ProfileAdmin}, &mcp.Tool{
		Name:        "task_dependency_add",
		Description: "Create a task dependency edge.",
	}, s.taskDependencyAdd)
	addTool(server, profile, []Profile{ProfileAdmin}, &mcp.Tool{
		Name:        "task_dependency_remove",
		Description: "Remove a task dependency edge.",
	}, s.taskDependencyRemove)
	addTool(server, profile, []Profile{ProfileAdmin}, &mcp.Tool{
		Name:        "project_instruction_list",
		Description: "List project instructions.",
	}, s.projectInstructionList)
	addTool(server, profile, []Profile{ProfileAdmin}, &mcp.Tool{
		Name:        "project_instruction_create",
		Description: "Create a project instruction.",
	}, s.projectInstructionCreate)
	addTool(server, profile, []Profile{ProfileAdmin}, &mcp.Tool{
		Name:        "project_instruction_show",
		Description: "Show a project instruction.",
	}, s.projectInstructionShow)
	addTool(server, profile, []Profile{ProfileAdmin}, &mcp.Tool{
		Name:        "project_instruction_enable",
		Description: "Enable a project instruction.",
	}, s.projectInstructionEnable)
	addTool(server, profile, []Profile{ProfileAdmin}, &mcp.Tool{
		Name:        "project_instruction_disable",
		Description: "Disable a project instruction.",
	}, s.projectInstructionDisable)
	addTool(server, profile, []Profile{ProfileAdmin}, &mcp.Tool{
		Name:        "project_instruction_delete",
		Description: "Delete a project instruction.",
	}, s.projectInstructionDelete)
	addTool(server, profile, []Profile{ProfileAdmin}, &mcp.Tool{
		Name:        "index_update",
		Description: "Update the lexical index for a project.",
	}, s.indexUpdate)
	addTool(server, profile, []Profile{ProfileAdmin}, &mcp.Tool{
		Name:        "index_update_all",
		Description: "Update lexical indexes for all projects.",
	}, s.indexUpdateAll)
	addTool(server, profile, []Profile{ProfileAdmin}, &mcp.Tool{
		Name:        "index_status",
		Description: "Show index status for a project.",
	}, s.indexStatus)
	addTool(server, profile, []Profile{ProfileAdmin}, &mcp.Tool{
		Name:        "index_status_all",
		Description: "Show index status for all projects.",
	}, s.indexStatusAll)
	addTool(server, profile, []Profile{ProfileAdmin}, &mcp.Tool{
		Name:        "search",
		Description: "Search indexed project files.",
	}, s.search)
	addTool(server, profile, []Profile{ProfileWorker, ProfileSupervisor, ProfileAdmin}, &mcp.Tool{
		Name:        "context_build",
		Description: "Build a structured TOK context package for a task.",
	}, s.contextBuild)
	addTool(server, profile, []Profile{ProfileWorker, ProfileSupervisor, ProfileAdmin}, &mcp.Tool{
		Name:        "run_list",
		Description: "List task runs optionally filtered by project/task/status.",
	}, s.runList)
	addTool(server, profile, []Profile{ProfileWorker, ProfileSupervisor, ProfileAdmin}, &mcp.Tool{
		Name:        "run_show",
		Description: "Show a run with artifacts.",
	}, s.runShow)
	addTool(server, profile, []Profile{ProfileWorker, ProfileSupervisor, ProfileAdmin}, &mcp.Tool{
		Name:        "run_create",
		Description: "Create a new task run.",
	}, s.runCreate)
	addTool(server, profile, []Profile{ProfileWorker, ProfileSupervisor, ProfileAdmin}, &mcp.Tool{
		Name:        "run_finish",
		Description: "Finish a task run.",
	}, s.runFinish)
	addTool(server, profile, []Profile{ProfileSupervisor, ProfileAdmin}, &mcp.Tool{
		Name:        "run_recover",
		Description: "Recover stale active runs.",
	}, s.runRecover)
	addTool(server, profile, []Profile{ProfileWorker, ProfileSupervisor, ProfileAdmin}, &mcp.Tool{
		Name:        "run_artifact_list",
		Description: "List artifacts for a run.",
	}, s.runArtifactList)
	addTool(server, profile, []Profile{ProfileAdmin}, &mcp.Tool{
		Name:        "run_artifact_add",
		Description: "Add a run artifact.",
	}, s.runArtifactAdd)
	addTool(server, profile, []Profile{ProfileWorker, ProfileSupervisor, ProfileAdmin}, &mcp.Tool{
		Name:        "run_validation_record",
		Description: "Record validation evidence for a run.",
	}, s.runValidationRecord)
}

func addTool[In, Out any](server *mcp.Server, profile Profile, profiles []Profile, tool *mcp.Tool, handler mcp.ToolHandlerFor[In, Out]) {
	if toolAllowed(profile, profiles) {
		mcp.AddTool(server, tool, handler)
	}
}

func toolAllowed(profile Profile, profiles []Profile) bool {
	if profile == ProfileAll {
		return true
	}
	for _, allowed := range profiles {
		if allowed == profile {
			return true
		}
	}
	return false
}
