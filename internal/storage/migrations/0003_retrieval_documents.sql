CREATE TABLE retrieval_documents (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	project_id INTEGER NOT NULL,
	path TEXT NOT NULL,
	provenance TEXT NOT NULL,
	size_bytes INTEGER NOT NULL,
	indexed_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
	content TEXT NOT NULL,
	FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE,
	UNIQUE (project_id, path)
);

CREATE INDEX idx_retrieval_documents_project ON retrieval_documents(project_id, path);
