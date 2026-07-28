# Alpha Notes And Safety

TOK is alpha software. It is intended for trusted local developer machines and
local agent workflows.

The product direction is a local execution control plane for coding agents, not
a hosted team task tracker or a generic multi-user agent platform. TOK focuses
on making local agent work reproducible, recoverable and auditable through
tasks, deterministic context, runs, leases, artifacts and validation evidence.

## Versioning

Install the current public alpha from GitHub Releases.

TOK uses Semantic Versioning while it is in public alpha: `v0.MINOR.PATCH`.
The git release tag is the source of truth for the product version shown by
`tok version`.

While TOK remains on `v0.x.y`, CLI, HTTP and MCP contracts may change.

## Current Limitations

- The HTTP UI binds to `127.0.0.1` by default and is not hardened for
  untrusted networks.
- Storage is local SQLite; there is no team sync or multi-tenant deployment
  model in the first alpha.
- TOK can complement Jira, Linear or GitHub Issues, but it is not intended to
  replace those systems as a planning backlog.
- Local TOK tasks are the dependency-free default. External tracker integration
  is a product direction, but the current alpha does not promise automatic
  import, export, webhooks, OAuth or bidirectional sync.
- Release assets initially cover Linux x86_64 and macOS x86_64/arm64 only.
- Run artifacts, indexed source content and handoff packages may contain
  sensitive project data.

## State And Safety Model

TOK is local-first and intentionally explicit.

- Tasks are not closed automatically when a run finishes.
- A task cannot be completed while it has a non-terminal run.
- Completing a task requires a succeeded evidence run with passed validation,
  unless an explicit override reason is recorded.
- Completed tasks are terminal in the current alpha. Create a new task for
  corrections or follow-up work instead of reopening a `done` task.
- `tok run exec` records validation evidence from the command result.
- A run can finish as `succeeded` only after passed validation evidence exists,
  unless `--allow-unvalidated --override-reason <text>` is used explicitly.
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
