package app

import (
	"encoding/json"
	"fmt"
	"io"

	"s26.sh/tok/internal/storage"
)

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
	StartedBy              *actorOutput        `json:"started_by,omitempty"`
	FinishedBy             *actorOutput        `json:"finished_by,omitempty"`
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
	Actor       *actorOutput `json:"actor,omitempty"`
	CreatedAt   string       `json:"created_at"`
}

func printRun(out io.Writer, run storage.Run) {
	fmt.Fprintf(out, "id: %d\n", run.ID)
	fmt.Fprintf(out, "task_id: %d\n", run.TaskID)
	fmt.Fprintf(out, "status: %s\n", run.Status)
	fmt.Fprintf(out, "handoff_contract_version: %s\n", run.HandoffContractVersion)
	fmt.Fprintf(out, "retrieval_limit: %d\n", run.RetrievalLimit)
	fmt.Fprintf(out, "started_at: %s\n", run.StartedAt)
	fmt.Fprintf(out, "finished_at: %s\n", run.FinishedAt)
	fmt.Fprintf(out, "base_branch: %s\n", run.BaseBranch)
	fmt.Fprintf(out, "base_head: %s\n", run.BaseHead)
	fmt.Fprintf(out, "result_summary: %s\n", run.ResultSummary)
	fmt.Fprintf(out, "lease_owner: %s\n", run.LeaseOwner)
	fmt.Fprintf(out, "heartbeat_at: %s\n", run.HeartbeatAt)
	fmt.Fprintf(out, "expires_at: %s\n", run.ExpiresAt)
	if run.ActorName != "" {
		fmt.Fprintf(out, "started_by: %s/%s\n", run.ActorKind, run.ActorName)
	}
	if run.FinishedActorName != "" {
		fmt.Fprintf(out, "finished_by: %s/%s\n", run.FinishedActorKind, run.FinishedActorName)
	}
}

func printRunArtifact(out io.Writer, artifact storage.RunArtifact) {
	fmt.Fprintf(out, "id: %d\n", artifact.ID)
	fmt.Fprintf(out, "run_id: %d\n", artifact.RunID)
	fmt.Fprintf(out, "kind: %s\n", artifact.Kind)
	fmt.Fprintf(out, "path: %s\n", artifact.Path)
	fmt.Fprintf(out, "content_hash: %s\n", artifact.ContentHash)
	fmt.Fprintf(out, "size_bytes: %d\n", artifact.SizeBytes)
	fmt.Fprintf(out, "truncated: %t\n", artifact.Truncated)
	fmt.Fprintf(out, "metadata: %s\n", artifact.Metadata)
	if artifact.ActorName != "" {
		fmt.Fprintf(out, "actor: %s/%s\n", artifact.ActorKind, artifact.ActorName)
	}
	fmt.Fprintf(out, "created_at: %s\n", artifact.CreatedAt)
}

func printRunJSON(out io.Writer, run storage.Run, artifacts []storage.RunArtifact) error {
	encoder := json.NewEncoder(out)
	encoder.SetIndent("", "  ")
	return encoder.Encode(runOutputFromStorage(run, artifacts))
}

func printRunsJSON(out io.Writer, runs []storage.Run) error {
	outputs := make([]runOutput, 0, len(runs))
	for _, run := range runs {
		outputs = append(outputs, runOutputFromStorage(run, nil))
	}
	encoder := json.NewEncoder(out)
	encoder.SetIndent("", "  ")
	return encoder.Encode(outputs)
}

func printRunArtifactJSON(out io.Writer, artifact storage.RunArtifact) error {
	encoder := json.NewEncoder(out)
	encoder.SetIndent("", "  ")
	return encoder.Encode(runArtifactOutputFromStorage(artifact))
}

func runOutputFromStorage(run storage.Run, artifacts []storage.RunArtifact) runOutput {
	artifactOutputs := make([]runArtifactOutput, 0, len(artifacts))
	for _, artifact := range artifacts {
		artifactOutputs = append(artifactOutputs, runArtifactOutputFromStorage(artifact))
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
		StartedBy:              actorOutputFromSnapshot(run.ActorID, run.ActorKind, run.ActorName),
		FinishedBy:             actorOutputFromSnapshot(run.FinishedActorID, run.FinishedActorKind, run.FinishedActorName),
		Artifacts:              artifactOutputs,
	}
}

func runArtifactOutputFromStorage(artifact storage.RunArtifact) runArtifactOutput {
	return runArtifactOutput{
		ID:          artifact.ID,
		RunID:       artifact.RunID,
		Kind:        artifact.Kind,
		Path:        artifact.Path,
		ContentHash: artifact.ContentHash,
		SizeBytes:   artifact.SizeBytes,
		Truncated:   artifact.Truncated,
		Metadata:    artifact.Metadata,
		Actor:       actorOutputFromSnapshot(artifact.ActorID, artifact.ActorKind, artifact.ActorName),
		CreatedAt:   artifact.CreatedAt,
	}
}
