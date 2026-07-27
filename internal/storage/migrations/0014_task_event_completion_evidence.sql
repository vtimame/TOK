ALTER TABLE task_events
ADD COLUMN evidence_run_id INTEGER NOT NULL DEFAULT 0;

ALTER TABLE task_events
ADD COLUMN evidence_artifact_id INTEGER NOT NULL DEFAULT 0;
