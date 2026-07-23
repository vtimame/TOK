CREATE TABLE run_artifacts (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	run_id INTEGER NOT NULL,
	kind TEXT NOT NULL CHECK (kind IN ('handoff', 'validation', 'log', 'patch', 'note')),
	path TEXT NOT NULL DEFAULT '',
	content_hash TEXT NOT NULL DEFAULT '',
	metadata TEXT NOT NULL DEFAULT '{}',
	created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
	FOREIGN KEY (run_id) REFERENCES runs(id) ON DELETE CASCADE
);

CREATE INDEX idx_run_artifacts_run_created ON run_artifacts(run_id, created_at);
CREATE INDEX idx_run_artifacts_kind ON run_artifacts(kind);
