CREATE TABLE project_instructions (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	project_id INTEGER NOT NULL,
	scope TEXT NOT NULL DEFAULT 'project' CHECK (scope IN ('project')),
	title TEXT NOT NULL,
	body TEXT NOT NULL,
	priority TEXT NOT NULL DEFAULT 'normal' CHECK (priority IN ('low', 'normal', 'high', 'critical')),
	enabled INTEGER NOT NULL DEFAULT 1 CHECK (enabled IN (0, 1)),
	source TEXT NOT NULL DEFAULT 'manual',
	created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
	updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
	FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE
);

CREATE INDEX idx_project_instructions_project_order ON project_instructions(
	project_id,
	enabled,
	priority,
	created_at,
	id
);
