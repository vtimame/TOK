CREATE TABLE index_policies (
	project_id INTEGER PRIMARY KEY,
	include_patterns TEXT NOT NULL DEFAULT '[]',
	ignore_patterns TEXT NOT NULL DEFAULT '[]',
	created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
	updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
	FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE
);
