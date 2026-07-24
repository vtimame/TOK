CREATE TABLE actors (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	kind TEXT NOT NULL CHECK (kind IN ('human', 'agent', 'system')),
	name TEXT NOT NULL,
	token_hash TEXT NOT NULL DEFAULT '',
	token_revoked_at TEXT NOT NULL DEFAULT '',
	created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
	updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
	CHECK (name <> ''),
	CHECK ((kind = 'agent' AND token_hash <> '') OR (kind <> 'agent' AND token_hash = '')),
	CHECK (kind = 'agent' OR token_revoked_at = '')
);

CREATE UNIQUE INDEX idx_actors_local_human ON actors(kind) WHERE kind = 'human';
CREATE UNIQUE INDEX idx_actors_agent_name ON actors(name) WHERE kind = 'agent';
CREATE UNIQUE INDEX idx_actors_agent_token_hash ON actors(token_hash) WHERE kind = 'agent';
