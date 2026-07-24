CREATE TABLE index_file_manifest (
	project_id INTEGER NOT NULL,
	path TEXT NOT NULL,
	size_bytes INTEGER NOT NULL DEFAULT 0,
	mod_time TEXT NOT NULL DEFAULT '',
	content_hash TEXT NOT NULL DEFAULT '',
	indexed_chunks INTEGER NOT NULL DEFAULT 0,
	skipped_reason TEXT NOT NULL DEFAULT '',
	updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
	FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE,
	UNIQUE (project_id, path)
);

CREATE INDEX idx_index_file_manifest_project ON index_file_manifest(project_id, path);
