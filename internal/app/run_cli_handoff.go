package app

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	contextpkg "s26.sh/tok/internal/context"
	"s26.sh/tok/internal/retrieval"
	"s26.sh/tok/internal/storage"
)

func writeRunHandoffArtifact(ctx context.Context, store *storage.Store, project storage.Project, task storage.Task, run storage.Run, opts runStartOptions, actor storage.ActorRef) (storage.RunArtifact, error) {
	outputPath := opts.handoffOutput
	if !filepath.IsAbs(outputPath) {
		absPath, err := filepath.Abs(outputPath)
		if err != nil {
			return storage.RunArtifact{}, fmt.Errorf("resolve handoff output path %q: %w", outputPath, err)
		}
		outputPath = absPath
	}

	builder := contextpkg.NewBuilder(store, retrieval.NewService(store))
	pkg, err := builder.Build(ctx, contextpkg.BuildInput{
		Project:        project,
		Task:           task,
		RetrievalLimit: opts.retrievalLimit,
	})
	if err != nil {
		return storage.RunArtifact{}, err
	}

	text := pkg.RenderText()
	if err := writeContextPackage(outputPath, text); err != nil {
		return storage.RunArtifact{}, err
	}

	return store.AddRunArtifact(ctx, storage.AddRunArtifactInput{
		RunID:       run.ID,
		Kind:        "handoff",
		Path:        outputPath,
		ContentHash: sha256ContentHash(text),
		Metadata:    `{"format":"text"}`,
		Actor:       actor,
	})
}

func writeRunManagedHandoffArtifact(ctx context.Context, store *storage.Store, dataDir string, project storage.Project, task storage.Task, run storage.Run, retrievalLimit int, actor storage.ActorRef) (storage.RunArtifact, error) {
	artifact, _, err := writeRunManagedHandoffTextArtifact(ctx, store, dataDir, project, task, run, retrievalLimit, actor, "run exec")
	return artifact, err
}

func writeRunManagedHandoffTextArtifact(ctx context.Context, store *storage.Store, dataDir string, project storage.Project, task storage.Task, run storage.Run, retrievalLimit int, actor storage.ActorRef, source string) (storage.RunArtifact, string, error) {
	artifacts, err := store.ListRunArtifacts(ctx, run.ID)
	if err != nil {
		return storage.RunArtifact{}, "", err
	}
	outputPath, _, err := nextRunArtifactPathWithExt(dataDir, run.ID, "handoff", ".md", len(artifacts)+1)
	if err != nil {
		return storage.RunArtifact{}, "", err
	}

	builder := contextpkg.NewBuilder(store, retrieval.NewService(store))
	pkg, err := builder.Build(ctx, contextpkg.BuildInput{
		Project:        project,
		Task:           task,
		RetrievalLimit: retrievalLimit,
	})
	if err != nil {
		return storage.RunArtifact{}, "", err
	}

	text := pkg.RenderText()
	if err := writeContextPackage(outputPath, text); err != nil {
		return storage.RunArtifact{}, "", err
	}
	metadata, err := json.Marshal(struct {
		Format string `json:"format"`
		Source string `json:"source"`
	}{
		Format: "text",
		Source: source,
	})
	if err != nil {
		return storage.RunArtifact{}, "", err
	}

	artifact, err := store.AddRunArtifact(ctx, storage.AddRunArtifactInput{
		RunID:       run.ID,
		Kind:        "handoff",
		Path:        outputPath,
		ContentHash: sha256ContentHash(text),
		SizeBytes:   int64(len([]byte(text))),
		Metadata:    string(metadata),
		Actor:       actor,
	})
	if err != nil {
		_ = os.Remove(outputPath)
		return storage.RunArtifact{}, "", err
	}
	return artifact, text, nil
}

func sha256ContentHash(value string) string {
	sum := sha256.Sum256([]byte(value))
	return fmt.Sprintf("sha256:%x", sum)
}
