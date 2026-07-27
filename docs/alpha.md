# Alpha Notes And Safety

TOK is alpha software. It is intended for trusted local developer machines and
local agent workflows.

## Versioning

The current public alpha release is `v0.1.0`.

TOK uses Semantic Versioning while it is in public alpha: `v0.MINOR.PATCH`.
The git release tag is the source of truth for the product version shown by
`tok version`.

While TOK remains on `v0.x.y`, CLI, HTTP and MCP contracts may change.

## Current Limitations

- The HTTP UI binds to `127.0.0.1` by default and is not hardened for
  untrusted networks.
- Storage is local SQLite; there is no team sync or multi-tenant deployment
  model in the first alpha.
- Release assets initially cover Linux x86_64 and macOS x86_64/arm64 only.
- Run artifacts, indexed source content and handoff packages may contain
  sensitive project data.

## State And Safety Model

TOK is local-first and intentionally explicit.

- Tasks are not closed automatically when a run finishes.
- A task cannot be completed while it has a non-terminal run.
- A run can finish as `succeeded` only after passed validation evidence exists,
  unless `--allow-unvalidated` is used explicitly.
- Active runs carry a local lease and heartbeat.
- Stale active runs can be recovered.
- Local commands run in the project workspace, but TOK is not a sandbox.
- Validation commands inherit a filtered environment by default.
- Secret-looking environment variables are not passed to validation commands.
- Validation metadata is redacted before it is written to JSON.
- Dangerous command patterns are rejected unless `--allow-dangerous` is passed.

This model is designed to reduce accidental damage and improve auditability. It
does not replace OS-level sandboxing, container isolation or code review.

## Data Handling

TOK stores local state in SQLite and stores run artifacts under the TOK data
directory. Operators should treat that directory as sensitive.

Validation command metadata is redacted before being written to JSON, and
secret-looking environment variables are not inherited by validation commands
by default. stdout/stderr artifact files are preserved as execution evidence
and should be reviewed before sharing.

Do not expose `tok ui serve` to untrusted networks without adding an
authentication and deployment model appropriate for your environment.
