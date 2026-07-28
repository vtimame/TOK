package app

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"s26.sh/tok/internal/storage"
)

type runMetadataRedactor struct {
	values   []string
	patterns []string
}

type runCommandSafetyMetadata struct {
	EnvPolicy         string   `json:"env_policy"`
	EnvNames          []string `json:"env_names"`
	RedactionEnabled  bool     `json:"redaction_enabled"`
	AllowDangerous    bool     `json:"allow_dangerous"`
	DangerousOverride string   `json:"dangerous_override"`
}

type runCommandArtifactMetadata struct {
	Source            string                   `json:"source"`
	AdapterContract   string                   `json:"adapter_contract,omitempty"`
	Command           string                   `json:"command"`
	Args              []string                 `json:"args"`
	Status            string                   `json:"status"`
	RunStatus         string                   `json:"run_status"`
	Summary           string                   `json:"summary"`
	ExitCode          int                      `json:"exit_code"`
	DurationMS        int64                    `json:"duration_ms"`
	TimedOut          bool                     `json:"timed_out"`
	TimeoutMS         int64                    `json:"timeout_ms"`
	Signal            string                   `json:"signal,omitempty"`
	PID               int                      `json:"pid"`
	ProcessGroupID    int                      `json:"process_group_id"`
	SessionID         int                      `json:"session_id"`
	ProcessGroup      bool                     `json:"process_group"`
	ForwardedSignals  []string                 `json:"forwarded_signals"`
	ContextMode       string                   `json:"context_mode,omitempty"`
	ContextFile       string                   `json:"context_file,omitempty"`
	ResultFile        string                   `json:"result_file,omitempty"`
	ResultRead        *bool                    `json:"result_read,omitempty"`
	ResultError       string                   `json:"result_error,omitempty"`
	ArtifactDir       string                   `json:"artifact_dir,omitempty"`
	HandoffArtifactID int64                    `json:"handoff_artifact_id,omitempty"`
	StdoutArtifactID  int64                    `json:"stdout_artifact_id"`
	StderrArtifactID  int64                    `json:"stderr_artifact_id"`
	Safety            runCommandSafetyMetadata `json:"safety"`
}

func formatRunTimestamp(t time.Time) string {
	return t.UTC().Format("2006-01-02T15:04:05.000Z")
}

func runLeaseOwner(actor storage.ActorRef) string {
	if actor.Kind != "" && actor.Name != "" {
		return actor.Kind + "/" + actor.Name
	}
	if actor.Name != "" {
		return actor.Name
	}
	if actor.Kind != "" {
		return actor.Kind
	}
	return "local"
}

func runExecArtifactMetadata(opts runExecOptions, redactor runMetadataRedactor, safety runCommandSafetyMetadata, execution runCommandExecutionResult, stdoutArtifactID, stderrArtifactID int64) (string, error) {
	metadata := newRunCommandArtifactMetadata("run exec", opts.command, opts.timeout, redactor, safety, execution, stdoutArtifactID, stderrArtifactID)
	raw, err := json.Marshal(metadata)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

func runAgentArtifactMetadata(opts runAgentOptions, redactor runMetadataRedactor, safety runCommandSafetyMetadata, execution runCommandExecutionResult, handoffArtifactID, stdoutArtifactID, stderrArtifactID int64, artifactDir, contextFile, resultFile string, resultRead bool, resultErr error) (string, error) {
	resultError := ""
	if resultErr != nil {
		resultError = resultErr.Error()
	}
	metadata := newRunCommandArtifactMetadata("run agent", opts.command, opts.timeout, redactor, safety, execution, stdoutArtifactID, stderrArtifactID)
	metadata.AdapterContract = agentAdapterContractV0
	metadata.ContextMode = opts.contextMode
	metadata.ContextFile = contextFile
	metadata.ResultFile = resultFile
	metadata.ResultRead = &resultRead
	metadata.ResultError = redactor.redactString(resultError)
	metadata.ArtifactDir = artifactDir
	metadata.HandoffArtifactID = handoffArtifactID
	raw, err := json.Marshal(metadata)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

func newRunCommandArtifactMetadata(source string, command []string, timeout time.Duration, redactor runMetadataRedactor, safety runCommandSafetyMetadata, execution runCommandExecutionResult, stdoutArtifactID, stderrArtifactID int64) runCommandArtifactMetadata {
	redactedArgs := redactor.redactArgs(command)
	return runCommandArtifactMetadata{
		Source:           source,
		Command:          strings.Join(redactedArgs, " "),
		Args:             redactedArgs,
		Status:           execution.Status,
		RunStatus:        execution.RunStatus,
		Summary:          redactor.redactString(execution.Summary),
		ExitCode:         execution.ExitCode,
		DurationMS:       execution.Duration.Milliseconds(),
		TimedOut:         execution.TimedOut,
		TimeoutMS:        timeout.Milliseconds(),
		Signal:           execution.Signal,
		PID:              execution.PID,
		ProcessGroupID:   execution.ProcessGroupID,
		SessionID:        execution.SessionID,
		ProcessGroup:     execution.ProcessGroupID != 0,
		ForwardedSignals: []string{"SIGINT", "SIGTERM"},
		StdoutArtifactID: stdoutArtifactID,
		StderrArtifactID: stderrArtifactID,
		Safety:           safety,
	}
}

func validationArtifactMetadata(opts runRecordValidationOptions, redactor runMetadataRedactor) (string, error) {
	raw, err := json.Marshal(struct {
		Command          string `json:"command"`
		Status           string `json:"status"`
		Summary          string `json:"summary"`
		RedactionEnabled bool   `json:"redaction_enabled"`
	}{
		Command:          redactor.redactString(opts.command),
		Status:           opts.status,
		Summary:          redactor.redactString(opts.summary),
		RedactionEnabled: true,
	})
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

func executedValidationArtifactMetadata(opts runValidateOptions, redactor runMetadataRedactor, safety runCommandSafetyMetadata, status string, exitCode int, duration time.Duration, timedOut bool, stdoutArtifactID, stderrArtifactID int64) (string, error) {
	summary := validationSummary(status, exitCode, timedOut, opts.timeout)
	redactedArgs := redactor.redactArgs(opts.command)
	raw, err := json.Marshal(struct {
		Command          string                   `json:"command"`
		Args             []string                 `json:"args"`
		Status           string                   `json:"status"`
		Summary          string                   `json:"summary"`
		ExitCode         int                      `json:"exit_code"`
		DurationMS       int64                    `json:"duration_ms"`
		TimedOut         bool                     `json:"timed_out"`
		TimeoutMS        int64                    `json:"timeout_ms"`
		StdoutArtifactID int64                    `json:"stdout_artifact_id"`
		StderrArtifactID int64                    `json:"stderr_artifact_id"`
		Safety           runCommandSafetyMetadata `json:"safety"`
	}{
		Command:          strings.Join(redactedArgs, " "),
		Args:             redactedArgs,
		Status:           status,
		Summary:          redactor.redactString(summary),
		ExitCode:         exitCode,
		DurationMS:       duration.Milliseconds(),
		TimedOut:         timedOut,
		TimeoutMS:        opts.timeout.Milliseconds(),
		StdoutArtifactID: stdoutArtifactID,
		StderrArtifactID: stderrArtifactID,
		Safety:           safety,
	})
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

func validationSummary(status string, exitCode int, timedOut bool, timeout time.Duration) string {
	if timedOut {
		return fmt.Sprintf("Validation timed out after %s.", timeout)
	}
	if status == "passed" {
		return "Validation passed."
	}
	return fmt.Sprintf("Validation failed with exit code %d.", exitCode)
}
func newRunMetadataRedactor(env []string) runMetadataRedactor {
	seen := map[string]bool{}
	var values []string
	for _, item := range env {
		name, value, ok := strings.Cut(item, "=")
		if !ok || value == "" || len(value) < 4 || !secretLikeName(name) || seen[value] {
			continue
		}
		seen[value] = true
		values = append(values, value)
	}

	var patterns []string
	for _, pattern := range strings.Split(os.Getenv("TOK_SECRET_PATTERNS"), ",") {
		pattern = strings.TrimSpace(pattern)
		if pattern != "" && !seen[pattern] {
			seen[pattern] = true
			patterns = append(patterns, pattern)
		}
	}
	return runMetadataRedactor{values: values, patterns: patterns}
}

func (r runMetadataRedactor) redactArgs(args []string) []string {
	redacted := make([]string, 0, len(args))
	for _, arg := range args {
		redacted = append(redacted, r.redactArg(arg))
	}
	return redacted
}

func (r runMetadataRedactor) redactArg(arg string) string {
	name, value, ok := strings.Cut(arg, "=")
	if ok && value != "" && secretLikeName(name) {
		return name + "=[REDACTED]"
	}
	return r.redactString(arg)
}

func (r runMetadataRedactor) redactString(value string) string {
	redacted := value
	for _, secret := range r.values {
		redacted = strings.ReplaceAll(redacted, secret, "[REDACTED]")
	}
	for _, pattern := range r.patterns {
		redacted = strings.ReplaceAll(redacted, pattern, "[REDACTED]")
	}
	return redacted
}

func secretLikeName(name string) bool {
	upper := strings.ToUpper(strings.TrimSpace(name))
	for _, marker := range []string{"SECRET", "TOKEN", "PASSWORD", "PASS", "API_KEY", "PRIVATE", "CREDENTIAL", "AUTH"} {
		if strings.Contains(upper, marker) {
			return true
		}
	}
	return false
}
