# Migration Checks

This page records ad hoc migration checks that exercise real release-to-current
SQLite upgrade paths.

## v0.2.1 To Current

Checked on 2026-07-28. `gh release list --limit 10` reported `v0.2.1` as the
latest public release, and local tags matched `v0.2.1`, `v0.2.0`, and `v0.1.0`.

The snapshot was created from a detached `v0.2.1` worktree with an isolated
`TOK_DATA_DIR`. Before running current code, the database had 13 applied
migrations, ending at `0013_run_artifact_file_metadata.sql`.

Fixture data:

- task `1`: `done` with a legacy `status_changed` event from `in_progress` to
  `done`;
- task `2`: `done` using the v0.2.1 audited completion override flow.

Current-code migration result:

- `schema_migrations` count is `15`, max version is `15`;
- `task_events` has `evidence_run_id` and `evidence_artifact_id` columns from
  migration `0014`;
- `tasks` has `source`, `external_id`, `external_url`, and `external_revision`
  columns from migration `0015`;
- existing tasks read as `source=local` with empty external reference fields;
- legacy task `1` still reads as `done` and preserves its
  `status_changed -> done` event through `tok task show --json`;
- v0.2.1 completion override task `2` still reads as `done` and preserves both
  `completed` and `completion_override` events.

Validation commands:

```bash
TOK_DATA_DIR=/tmp/.../data go run ./cmd/tok init
TOK_DATA_DIR=/tmp/.../data go run ./cmd/tok task show 1 --json
TOK_DATA_DIR=/tmp/.../data go run ./cmd/tok task show 2 --json
make quality && make vet && go test ./... && make staticcheck
```
