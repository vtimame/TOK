package app

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"s26.sh/tok/internal/storage"
)

type runFileArtifactWriteResult struct {
	ContentHash       string
	SizeBytes         int64
	OriginalSizeBytes int64
	Truncated         bool
}

type runStreamArtifactWriters struct {
	StdoutPath   string
	StderrPath   string
	StdoutWriter *boundedRunArtifactWriter
	StderrWriter *boundedRunArtifactWriter
}

type boundedRunArtifactWriter struct {
	file       *os.File
	hasher     hashWriter
	path       string
	limitBytes int64
	written    int64
	original   int64
}

type hashWriter interface {
	Write([]byte) (int, error)
	Sum([]byte) []byte
}

func writeRunFileArtifact(ctx context.Context, store *storage.Store, dataDir string, opts runRecordArtifactOptions, actor storage.ActorRef) (storage.RunArtifact, error) {
	run, err := store.GetRun(ctx, opts.runID)
	if err != nil {
		return storage.RunArtifact{}, err
	}

	artifacts, err := store.ListRunArtifacts(ctx, run.ID)
	if err != nil {
		return storage.RunArtifact{}, err
	}

	var input io.Reader
	sourcePath := opts.inputPath
	var inputFile *os.File
	if opts.inputPath == "-" {
		input = os.Stdin
	} else {
		absPath, err := filepath.Abs(opts.inputPath)
		if err != nil {
			return storage.RunArtifact{}, fmt.Errorf("resolve artifact input path %q: %w", opts.inputPath, err)
		}
		sourcePath = absPath
		inputFile, err = os.Open(absPath)
		if err != nil {
			return storage.RunArtifact{}, fmt.Errorf("open artifact input %q: %w", absPath, err)
		}
		defer inputFile.Close()
		input = inputFile
	}

	outputPath, _, err := nextRunArtifactPath(dataDir, run.ID, opts.kind, len(artifacts)+1)
	if err != nil {
		return storage.RunArtifact{}, err
	}

	result, err := writeBoundedRunArtifactFile(outputPath, input, opts.limitBytes)
	if err != nil {
		return storage.RunArtifact{}, err
	}

	metadata, err := fileRunArtifactMetadata(opts, sourcePath, result)
	if err != nil {
		return storage.RunArtifact{}, err
	}

	artifact, err := store.AddRunArtifact(ctx, storage.AddRunArtifactInput{
		RunID:       run.ID,
		Kind:        opts.kind,
		Path:        outputPath,
		ContentHash: result.ContentHash,
		SizeBytes:   result.SizeBytes,
		Truncated:   result.Truncated,
		Metadata:    metadata,
		Actor:       actor,
	})
	if err != nil {
		_ = os.Remove(outputPath)
		return storage.RunArtifact{}, err
	}
	return artifact, nil
}

func nextRunArtifactPath(dataDir string, runID int64, kind string, startOrdinal int) (string, int, error) {
	return nextRunArtifactPathWithExt(dataDir, runID, kind, ".txt", startOrdinal)
}

func nextRunArtifactPathWithExt(dataDir string, runID int64, kind, ext string, startOrdinal int) (string, int, error) {
	outputDir := filepath.Join(dataDir, "run-artifacts", fmt.Sprintf("run-%d", runID))
	for ordinal := startOrdinal; ; ordinal++ {
		outputPath := filepath.Join(outputDir, fmt.Sprintf("%04d-%s%s", ordinal, kind, ext))
		_, err := os.Stat(outputPath)
		if errors.Is(err, os.ErrNotExist) {
			return outputPath, ordinal + 1, nil
		}
		if err != nil {
			return "", 0, fmt.Errorf("stat artifact path %q: %w", outputPath, err)
		}
	}
}

func newRunStreamArtifactWriters(dataDir string, runID int64, startOrdinal int, limitBytes int64) (*runStreamArtifactWriters, error) {
	stdoutPath, nextOrdinal, err := nextRunArtifactPath(dataDir, runID, "stdout", startOrdinal)
	if err != nil {
		return nil, err
	}
	stderrPath, _, err := nextRunArtifactPath(dataDir, runID, "stderr", nextOrdinal)
	if err != nil {
		return nil, err
	}

	stdoutWriter, err := newBoundedRunArtifactWriter(stdoutPath, limitBytes)
	if err != nil {
		return nil, err
	}
	stderrWriter, err := newBoundedRunArtifactWriter(stderrPath, limitBytes)
	if err != nil {
		_, _ = stdoutWriter.Close()
		_ = os.Remove(stdoutPath)
		return nil, err
	}

	return &runStreamArtifactWriters{
		StdoutPath:   stdoutPath,
		StderrPath:   stderrPath,
		StdoutWriter: stdoutWriter,
		StderrWriter: stderrWriter,
	}, nil
}

func (w *runStreamArtifactWriters) closeQuietly() {
	if w == nil {
		return
	}
	if w.StdoutWriter != nil {
		_, _ = w.StdoutWriter.Close()
	}
	if w.StderrWriter != nil {
		_, _ = w.StderrWriter.Close()
	}
}

func (w *runStreamArtifactWriters) removeFiles() {
	if w == nil {
		return
	}
	_ = os.Remove(w.StdoutPath)
	_ = os.Remove(w.StderrPath)
}

func (w *runStreamArtifactWriters) close() (runFileArtifactWriteResult, runFileArtifactWriteResult, error) {
	stdoutResult, err := w.StdoutWriter.Close()
	if err != nil {
		_, _ = w.StderrWriter.Close()
		w.removeFiles()
		return runFileArtifactWriteResult{}, runFileArtifactWriteResult{}, err
	}
	stderrResult, err := w.StderrWriter.Close()
	if err != nil {
		w.removeFiles()
		return runFileArtifactWriteResult{}, runFileArtifactWriteResult{}, err
	}
	return stdoutResult, stderrResult, nil
}

func recordRunStreamArtifacts(ctx context.Context, store *storage.Store, runID int64, source string, limitBytes int64, streams *runStreamArtifactWriters, actor storage.ActorRef) (storage.RunArtifact, storage.RunArtifact, error) {
	stdoutResult, stderrResult, err := streams.close()
	if err != nil {
		return storage.RunArtifact{}, storage.RunArtifact{}, err
	}

	stdoutMetadata, err := streamArtifactMetadata(source, "stdout", limitBytes, stdoutResult)
	if err != nil {
		streams.removeFiles()
		return storage.RunArtifact{}, storage.RunArtifact{}, err
	}
	stdoutArtifact, err := store.AddRunArtifact(ctx, storage.AddRunArtifactInput{
		RunID:       runID,
		Kind:        "stdout",
		Path:        streams.StdoutPath,
		ContentHash: stdoutResult.ContentHash,
		SizeBytes:   stdoutResult.SizeBytes,
		Truncated:   stdoutResult.Truncated,
		Metadata:    stdoutMetadata,
		Actor:       actor,
	})
	if err != nil {
		streams.removeFiles()
		return storage.RunArtifact{}, storage.RunArtifact{}, err
	}

	stderrMetadata, err := streamArtifactMetadata(source, "stderr", limitBytes, stderrResult)
	if err != nil {
		_ = os.Remove(streams.StderrPath)
		return storage.RunArtifact{}, storage.RunArtifact{}, err
	}
	stderrArtifact, err := store.AddRunArtifact(ctx, storage.AddRunArtifactInput{
		RunID:       runID,
		Kind:        "stderr",
		Path:        streams.StderrPath,
		ContentHash: stderrResult.ContentHash,
		SizeBytes:   stderrResult.SizeBytes,
		Truncated:   stderrResult.Truncated,
		Metadata:    stderrMetadata,
		Actor:       actor,
	})
	if err != nil {
		_ = os.Remove(streams.StderrPath)
		return storage.RunArtifact{}, storage.RunArtifact{}, err
	}

	return stdoutArtifact, stderrArtifact, nil
}

func newBoundedRunArtifactWriter(path string, limitBytes int64) (*boundedRunArtifactWriter, error) {
	if limitBytes <= 0 {
		return nil, errors.New("run artifact limit bytes must be positive")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("create run artifact directory: %w", err)
	}
	file, err := os.Create(path)
	if err != nil {
		return nil, fmt.Errorf("create run artifact %q: %w", path, err)
	}
	return &boundedRunArtifactWriter{
		file:       file,
		hasher:     sha256.New(),
		path:       path,
		limitBytes: limitBytes,
	}, nil
}

func (w *boundedRunArtifactWriter) Write(p []byte) (int, error) {
	if w.file == nil {
		return 0, errors.New("run artifact writer is closed")
	}
	w.original += int64(len(p))
	if w.written >= w.limitBytes {
		return len(p), nil
	}

	remaining := w.limitBytes - w.written
	writeN := int64(len(p))
	if writeN > remaining {
		writeN = remaining
	}
	chunk := p[:writeN]
	if _, err := w.file.Write(chunk); err != nil {
		return 0, fmt.Errorf("write run artifact %q: %w", w.path, err)
	}
	if _, err := w.hasher.Write(chunk); err != nil {
		return 0, fmt.Errorf("hash run artifact %q: %w", w.path, err)
	}
	w.written += writeN
	return len(p), nil
}

func (w *boundedRunArtifactWriter) Close() (runFileArtifactWriteResult, error) {
	if w.file != nil {
		if err := w.file.Close(); err != nil {
			w.file = nil
			return runFileArtifactWriteResult{}, fmt.Errorf("close run artifact %q: %w", w.path, err)
		}
		w.file = nil
	}
	return runFileArtifactWriteResult{
		ContentHash:       fmt.Sprintf("sha256:%x", w.hasher.Sum(nil)),
		SizeBytes:         w.written,
		OriginalSizeBytes: w.original,
		Truncated:         w.original > w.written,
	}, nil
}

func writeBoundedRunArtifactFile(path string, input io.Reader, limitBytes int64) (runFileArtifactWriteResult, error) {
	if limitBytes <= 0 {
		return runFileArtifactWriteResult{}, errors.New("run artifact limit bytes must be positive")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return runFileArtifactWriteResult{}, fmt.Errorf("create run artifact directory: %w", err)
	}

	output, err := os.Create(path)
	if err != nil {
		return runFileArtifactWriteResult{}, fmt.Errorf("create run artifact %q: %w", path, err)
	}

	hasher := sha256.New()
	var written int64
	var original int64
	var writeErr error
	buffer := make([]byte, 32*1024)
	for {
		n, readErr := input.Read(buffer)
		if n > 0 {
			original += int64(n)
			if written < limitBytes {
				remaining := limitBytes - written
				writeN := int64(n)
				if writeN > remaining {
					writeN = remaining
				}
				chunk := buffer[:writeN]
				if _, err := output.Write(chunk); err != nil {
					writeErr = fmt.Errorf("write run artifact %q: %w", path, err)
					break
				}
				if _, err := hasher.Write(chunk); err != nil {
					writeErr = fmt.Errorf("hash run artifact %q: %w", path, err)
					break
				}
				written += writeN
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			writeErr = fmt.Errorf("read run artifact input: %w", readErr)
			break
		}
	}

	if closeErr := output.Close(); writeErr == nil && closeErr != nil {
		writeErr = fmt.Errorf("close run artifact %q: %w", path, closeErr)
	}
	if writeErr != nil {
		_ = os.Remove(path)
		return runFileArtifactWriteResult{}, writeErr
	}

	return runFileArtifactWriteResult{
		ContentHash:       fmt.Sprintf("sha256:%x", hasher.Sum(nil)),
		SizeBytes:         written,
		OriginalSizeBytes: original,
		Truncated:         original > written,
	}, nil
}

func fileRunArtifactMetadata(opts runRecordArtifactOptions, sourcePath string, result runFileArtifactWriteResult) (string, error) {
	raw, err := json.Marshal(struct {
		Format            string `json:"format"`
		SourcePath        string `json:"source_path,omitempty"`
		SizeBytes         int64  `json:"size_bytes"`
		OriginalSizeBytes int64  `json:"original_size_bytes"`
		LimitBytes        int64  `json:"limit_bytes"`
		Truncated         bool   `json:"truncated"`
	}{
		Format:            "text",
		SourcePath:        sourcePath,
		SizeBytes:         result.SizeBytes,
		OriginalSizeBytes: result.OriginalSizeBytes,
		LimitBytes:        opts.limitBytes,
		Truncated:         result.Truncated,
	})
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

func streamArtifactMetadata(source, stream string, limitBytes int64, result runFileArtifactWriteResult) (string, error) {
	raw, err := json.Marshal(struct {
		Format            string `json:"format"`
		Source            string `json:"source"`
		Stream            string `json:"stream"`
		SizeBytes         int64  `json:"size_bytes"`
		OriginalSizeBytes int64  `json:"original_size_bytes"`
		LimitBytes        int64  `json:"limit_bytes"`
		Truncated         bool   `json:"truncated"`
	}{
		Format:            "text",
		Source:            source,
		Stream:            stream,
		SizeBytes:         result.SizeBytes,
		OriginalSizeBytes: result.OriginalSizeBytes,
		LimitBytes:        limitBytes,
		Truncated:         result.Truncated,
	})
	if err != nil {
		return "", err
	}
	return string(raw), nil
}
