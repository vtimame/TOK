CREATE TABLE task_dependencies (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	edge_type TEXT NOT NULL CHECK (edge_type IN ('blocks')),
	blocker_task_id INTEGER NOT NULL,
	blocked_task_id INTEGER NOT NULL,
	created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
	FOREIGN KEY (blocker_task_id) REFERENCES tasks(id) ON DELETE CASCADE,
	FOREIGN KEY (blocked_task_id) REFERENCES tasks(id) ON DELETE CASCADE,
	UNIQUE (edge_type, blocker_task_id, blocked_task_id),
	CHECK (blocker_task_id <> blocked_task_id)
);

CREATE INDEX idx_task_dependencies_blocked ON task_dependencies(blocked_task_id, edge_type);
CREATE INDEX idx_task_dependencies_blocker ON task_dependencies(blocker_task_id, edge_type);
