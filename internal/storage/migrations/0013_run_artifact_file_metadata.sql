CREATE TABLE run_artifacts_new (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	run_id INTEGER NOT NULL,
	kind TEXT NOT NULL CHECK (kind IN ('handoff', 'validation', 'stdout', 'stderr', 'log', 'patch', 'note')),
	path TEXT NOT NULL DEFAULT '',
	content_hash TEXT NOT NULL DEFAULT '',
	size_bytes INTEGER NOT NULL DEFAULT 0,
	truncated INTEGER NOT NULL DEFAULT 0 CHECK (truncated IN (0, 1)),
	metadata TEXT NOT NULL DEFAULT '{}',
	created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
	actor_id INTEGER NOT NULL DEFAULT 0,
	actor_kind TEXT NOT NULL DEFAULT '',
	actor_name TEXT NOT NULL DEFAULT '',
	FOREIGN KEY (run_id) REFERENCES runs(id) ON DELETE CASCADE
);

INSERT INTO run_artifacts_new (
	id,
	run_id,
	kind,
	path,
	content_hash,
	metadata,
	created_at,
	actor_id,
	actor_kind,
	actor_name
)
SELECT
	id,
	run_id,
	kind,
	path,
	content_hash,
	metadata,
	created_at,
	actor_id,
	actor_kind,
	actor_name
FROM run_artifacts;

DROP TABLE run_artifacts;
ALTER TABLE run_artifacts_new RENAME TO run_artifacts;

CREATE INDEX idx_run_artifacts_run_created ON run_artifacts(run_id, created_at);
CREATE INDEX idx_run_artifacts_kind ON run_artifacts(kind);
