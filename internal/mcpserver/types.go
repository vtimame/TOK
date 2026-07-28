package mcpserver

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

type agentCreateInput struct {
	Name string `json:"name" jsonschema:"agent display name"`
}

type agentIDInput struct {
	ID int64 `json:"id" jsonschema:"agent id"`
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
	Status  string `json:"status,omitempty" jsonschema:"optional task status filter: open, in_progress, blocked, or done"`
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
	Status string `json:"status" jsonschema:"open, in_progress, or blocked; use task_done for completion"`
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

type runRecoverInput struct {
	Summary string `json:"summary" jsonschema:"recovery summary"`
	Now     string `json:"now,omitempty" jsonschema:"optional recovery timestamp; defaults to current UTC time"`
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

type agentListOutput struct {
	Agents []AgentOutput `json:"agents"`
}

type agentOutput struct {
	Agent AgentOutput `json:"agent"`
}

type agentCreateOutput struct {
	Agent AgentOutput `json:"agent"`
	Token string      `json:"token"`
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
	Artifacts              []runArtifactOutput `json:"artifacts"`
	StartedBy              *ActorOutput        `json:"started_by,omitempty"`
	FinishedBy             *ActorOutput        `json:"finished_by,omitempty"`
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

type AgentOutput struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	Status    string `json:"status"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
	RevokedAt string `json:"revoked_at,omitempty"`
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
