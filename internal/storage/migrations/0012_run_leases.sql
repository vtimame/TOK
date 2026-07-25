ALTER TABLE runs ADD COLUMN lease_owner TEXT NOT NULL DEFAULT '';
ALTER TABLE runs ADD COLUMN heartbeat_at TEXT NOT NULL DEFAULT '';
ALTER TABLE runs ADD COLUMN expires_at TEXT NOT NULL DEFAULT '';

CREATE INDEX idx_runs_task_active ON runs(task_id, status);
CREATE INDEX idx_runs_expires_at ON runs(expires_at);
