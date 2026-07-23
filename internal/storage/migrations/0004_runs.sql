CREATE TABLE runs (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	task_id INTEGER NOT NULL,
	status TEXT NOT NULL CHECK (status IN ('created', 'in_progress', 'succeeded', 'failed', 'blocked', 'cancelled')),
	handoff_contract_version TEXT NOT NULL,
	started_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
	finished_at TEXT NOT NULL DEFAULT '',
	base_branch TEXT NOT NULL DEFAULT '',
	base_head TEXT NOT NULL DEFAULT '',
	result_summary TEXT NOT NULL DEFAULT '',
	FOREIGN KEY (task_id) REFERENCES tasks(id) ON DELETE CASCADE
);

CREATE INDEX idx_runs_task_started ON runs(task_id, started_at);
CREATE INDEX idx_runs_status ON runs(status);
