package storage

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"testing"
)

type scaleBenchmarkConfig struct {
	TaskCount       int
	EventCount      int
	RunCount        int
	ArtifactsPerRun int
	IndexedDocs     int
	HotTaskEvents   int
	HotTaskRuns     int
}

var scaleReadPathSink int

func BenchmarkScaleReadPaths(b *testing.B) {
	cfg := scaleBenchmarkConfig{
		TaskCount:       3000,
		EventCount:      8650,
		RunCount:        2200,
		ArtifactsPerRun: 3,
		IndexedDocs:     200,
		HotTaskEvents:   252,
		HotTaskRuns:     201,
	}
	ctx := context.Background()
	store, projectID, hotTaskID := openScaleBenchmarkStore(b, ctx, cfg)
	defer func() {
		if err := store.Close(); err != nil {
			b.Fatalf("Close returned error: %v", err)
		}
	}()

	b.ReportAllocs()
	b.Logf(
		"dataset: tasks=%d events=%d runs=%d artifacts=%d indexed_docs=%d hot_task_events=%d hot_task_runs=%d",
		cfg.TaskCount,
		cfg.EventCount,
		cfg.RunCount,
		cfg.RunCount*cfg.ArtifactsPerRun,
		cfg.IndexedDocs,
		cfg.HotTaskEvents,
		cfg.HotTaskRuns,
	)

	b.Run("TaskDetailsHotTask", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			task, err := store.GetTask(ctx, hotTaskID)
			if err != nil {
				b.Fatalf("GetTask returned error: %v", err)
			}
			project, err := store.GetProjectByID(ctx, task.ProjectID)
			if err != nil {
				b.Fatalf("GetProjectByID returned error: %v", err)
			}
			events, err := store.ListTaskEvents(ctx, task.ID)
			if err != nil {
				b.Fatalf("ListTaskEvents returned error: %v", err)
			}
			dependencies, err := store.ListTaskDependencies(ctx, task.ProjectID, task.ID)
			if err != nil {
				b.Fatalf("ListTaskDependencies returned error: %v", err)
			}
			runs, err := store.ListRuns(ctx, ListRunsOptions{TaskID: task.ID})
			if err != nil {
				b.Fatalf("ListRuns task returned error: %v", err)
			}
			artifactCount := 0
			for _, run := range runs {
				artifacts, err := store.ListRunArtifacts(ctx, run.ID)
				if err != nil {
					b.Fatalf("ListRunArtifacts returned error: %v", err)
				}
				artifactCount += len(artifacts)
			}
			scaleReadPathSink += len(project.Name) + len(events) + len(dependencies) + len(runs) + artifactCount
		}
	})

	b.Run("ProjectTaskPage", func(b *testing.B) {
		opts := ListTasksOptions{ProjectID: projectID, Limit: 51}
		for i := 0; i < b.N; i++ {
			total, err := store.CountTasksWithOptions(ctx, projectID, opts)
			if err != nil {
				b.Fatalf("CountTasksWithOptions returned error: %v", err)
			}
			tasks, err := store.ListAllTasksWithOptions(ctx, opts)
			if err != nil {
				b.Fatalf("ListAllTasksWithOptions returned error: %v", err)
			}
			eventCount := 0
			for _, task := range tasks {
				events, err := store.ListTaskEvents(ctx, task.ID)
				if err != nil {
					b.Fatalf("ListTaskEvents page task returned error: %v", err)
				}
				eventCount += len(events)
			}
			scaleReadPathSink += total + len(tasks) + eventCount
		}
	})

	b.Run("ProjectCounts", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			allTasks, err := store.ListTasks(ctx, projectID)
			if err != nil {
				b.Fatalf("ListTasks returned error: %v", err)
			}
			readyTasks, err := store.ListReadyTasks(ctx, projectID)
			if err != nil {
				b.Fatalf("ListReadyTasks returned error: %v", err)
			}
			scaleReadPathSink += len(allTasks) + len(readyTasks)
		}
	})

	b.Run("RunsAndArtifactsPage", func(b *testing.B) {
		opts := ListRunsOptions{ProjectID: projectID, Limit: 100}
		for i := 0; i < b.N; i++ {
			runs, err := store.ListRuns(ctx, opts)
			if err != nil {
				b.Fatalf("ListRuns project returned error: %v", err)
			}
			artifactCount := 0
			for _, run := range runs {
				artifacts, err := store.ListRunArtifacts(ctx, run.ID)
				if err != nil {
					b.Fatalf("ListRunArtifacts page run returned error: %v", err)
				}
				artifactCount += len(artifacts)
			}
			scaleReadPathSink += len(runs) + artifactCount
		}
	})
}

func openScaleBenchmarkStore(b *testing.B, ctx context.Context, cfg scaleBenchmarkConfig) (*Store, int64, int64) {
	b.Helper()

	path := filepath.Join(b.TempDir(), DatabaseFileName)
	store, err := Open(ctx, path)
	if err != nil {
		b.Fatalf("Open returned error: %v", err)
	}
	if err := store.Init(ctx); err != nil {
		b.Fatalf("Init returned error: %v", err)
	}
	if err := insertScaleBenchmarkFixture(ctx, store.db, cfg); err != nil {
		b.Fatalf("insert scale fixture: %v", err)
	}
	return store, 1, 1
}

func insertScaleBenchmarkFixture(ctx context.Context, db *sql.DB, cfg scaleBenchmarkConfig) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin fixture transaction: %w", err)
	}
	defer rollback(tx)

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO projects (id, name, display_name, path)
		VALUES (1, 'scale-fixture', 'Scale Fixture', '/tmp/scale-fixture')
	`); err != nil {
		return fmt.Errorf("insert project: %w", err)
	}
	if err := insertScaleTasks(ctx, tx, cfg); err != nil {
		return err
	}
	if err := insertScaleTaskEvents(ctx, tx, cfg); err != nil {
		return err
	}
	if err := insertScaleDependencies(ctx, tx); err != nil {
		return err
	}
	if err := insertScaleRunsAndArtifacts(ctx, tx, cfg); err != nil {
		return err
	}
	if err := insertScaleIndexedDocuments(ctx, tx, cfg); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit fixture transaction: %w", err)
	}
	return nil
}

func insertScaleTasks(ctx context.Context, tx *sql.Tx, cfg scaleBenchmarkConfig) error {
	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO tasks (id, project_id, status, title, description, acceptance_criteria, notes)
		VALUES (?, 1, ?, ?, 'Scale fixture task.', '- read paths stay responsive', '')
	`)
	if err != nil {
		return fmt.Errorf("prepare tasks insert: %w", err)
	}
	defer stmt.Close()

	statuses := []string{"open", "in_progress", "blocked", "done"}
	for taskID := 1; taskID <= cfg.TaskCount; taskID++ {
		status := statuses[taskID%len(statuses)]
		if taskID == 1 {
			status = "done"
		}
		if _, err := stmt.ExecContext(ctx, taskID, status, fmt.Sprintf("Scale task %04d", taskID)); err != nil {
			return fmt.Errorf("insert task %d: %w", taskID, err)
		}
	}
	return nil
}

func insertScaleTaskEvents(ctx context.Context, tx *sql.Tx, cfg scaleBenchmarkConfig) error {
	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO task_events (task_id, type, body, from_status, to_status, actor_id, actor_kind, actor_name)
		VALUES (?, ?, ?, ?, ?, 0, '', '')
	`)
	if err != nil {
		return fmt.Errorf("prepare task events insert: %w", err)
	}
	defer stmt.Close()

	inserted := 0
	for i := 0; i < cfg.HotTaskEvents; i++ {
		eventType := "progress"
		fromStatus := ""
		toStatus := ""
		if i == 0 {
			eventType = "created"
			toStatus = "open"
		}
		if i == 1 {
			eventType = "status_changed"
			fromStatus = "open"
			toStatus = "in_progress"
		}
		if i == cfg.HotTaskEvents-1 {
			eventType = "completed"
			fromStatus = "in_progress"
			toStatus = "done"
		}
		if _, err := stmt.ExecContext(ctx, 1, eventType, fmt.Sprintf("hot task event %03d", i), fromStatus, toStatus); err != nil {
			return fmt.Errorf("insert hot task event %d: %w", i, err)
		}
		inserted++
	}

	for taskID := 2; taskID <= cfg.TaskCount; taskID++ {
		if _, err := stmt.ExecContext(ctx, taskID, "created", fmt.Sprintf("Scale task %04d", taskID), "", "open"); err != nil {
			return fmt.Errorf("insert created event for task %d: %w", taskID, err)
		}
		inserted++
		if _, err := stmt.ExecContext(ctx, taskID, "status_changed", "", "open", "in_progress"); err != nil {
			return fmt.Errorf("insert status event for task %d: %w", taskID, err)
		}
		inserted++
	}

	for extra := inserted; extra < cfg.EventCount; extra++ {
		taskID := 2 + ((extra - inserted) % max(cfg.TaskCount-1, 1))
		if _, err := stmt.ExecContext(ctx, taskID, "progress", fmt.Sprintf("scale progress event %04d", extra), "", ""); err != nil {
			return fmt.Errorf("insert extra event %d: %w", extra, err)
		}
	}
	return nil
}

func insertScaleDependencies(ctx context.Context, tx *sql.Tx) error {
	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO task_dependencies (edge_type, blocker_task_id, blocked_task_id)
		VALUES ('blocks', ?, ?)
	`)
	if err != nil {
		return fmt.Errorf("prepare dependencies insert: %w", err)
	}
	defer stmt.Close()

	for blockerID := 2; blockerID <= 11; blockerID++ {
		if _, err := stmt.ExecContext(ctx, blockerID, 1); err != nil {
			return fmt.Errorf("insert dependency %d -> 1: %w", blockerID, err)
		}
	}
	return nil
}

func insertScaleRunsAndArtifacts(ctx context.Context, tx *sql.Tx, cfg scaleBenchmarkConfig) error {
	runStmt, err := tx.PrepareContext(ctx, `
		INSERT INTO runs (id, task_id, status, handoff_contract_version, retrieval_limit, base_branch, base_head, result_summary)
		VALUES (?, ?, 'succeeded', 'tok.handoff.v0', 5, 'main', 'abcdef0', ?)
	`)
	if err != nil {
		return fmt.Errorf("prepare runs insert: %w", err)
	}
	defer runStmt.Close()

	artifactStmt, err := tx.PrepareContext(ctx, `
		INSERT INTO run_artifacts (run_id, kind, path, content_hash, size_bytes, truncated, metadata)
		VALUES (?, ?, ?, ?, ?, 0, '{}')
	`)
	if err != nil {
		return fmt.Errorf("prepare run artifacts insert: %w", err)
	}
	defer artifactStmt.Close()

	kinds := []string{"validation", "stdout", "stderr"}
	for runID := 1; runID <= cfg.RunCount; runID++ {
		taskID := 1
		if runID > cfg.HotTaskRuns {
			taskID = 2 + ((runID - cfg.HotTaskRuns - 1) % max(cfg.TaskCount-1, 1))
		}
		if _, err := runStmt.ExecContext(ctx, runID, taskID, fmt.Sprintf("scale run %04d passed", runID)); err != nil {
			return fmt.Errorf("insert run %d: %w", runID, err)
		}
		for artifactIndex := 0; artifactIndex < cfg.ArtifactsPerRun; artifactIndex++ {
			kind := kinds[artifactIndex%len(kinds)]
			path := fmt.Sprintf("runs/%04d/%s.txt", runID, kind)
			hash := fmt.Sprintf("sha256:%064d", runID*10+artifactIndex)
			if _, err := artifactStmt.ExecContext(ctx, runID, kind, path, hash, 128+artifactIndex); err != nil {
				return fmt.Errorf("insert artifact %d for run %d: %w", artifactIndex, runID, err)
			}
		}
	}
	return nil
}

func insertScaleIndexedDocuments(ctx context.Context, tx *sql.Tx, cfg scaleBenchmarkConfig) error {
	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO retrieval_documents (project_id, path, provenance, size_bytes, content)
		VALUES (1, ?, 'fixture', ?, ?)
	`)
	if err != nil {
		return fmt.Errorf("prepare indexed documents insert: %w", err)
	}
	defer stmt.Close()

	for docID := 1; docID <= cfg.IndexedDocs; docID++ {
		content := fmt.Sprintf("scale indexed document %04d\nread path benchmark fixture\n", docID)
		if _, err := stmt.ExecContext(ctx, fmt.Sprintf("docs/scale-%04d.md", docID), len(content), content); err != nil {
			return fmt.Errorf("insert indexed document %d: %w", docID, err)
		}
	}
	return nil
}
