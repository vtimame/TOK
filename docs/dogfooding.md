# Dogfooding Metrics

TOK should prove its execution workflow by using it for its own development.
The minimum dogfooding report is intentionally local and dependency-light: it
reads the existing SQLite model and measures one context build on demand.

## Report Command

```bash
scripts/dogfooding-metrics.sh --project tok --task <task-id>
```

Use `--db /path/to/tok.db` to inspect an isolated demo database, or
`TOK_BIN=/path/to/tok` to force a specific binary.

The report includes:

- completed tasks and total tasks;
- total runs and average runs per completed task;
- active runs and currently stale leases;
- recovered runs;
- validation records and failed validation records;
- completion overrides;
- handoff artifacts and whether a handoff case is recorded;
- context package build latency for the supplied task.

## Current Local Baseline

On 2026-07-28, the local TOK project report showed TOK is being used through
the execution workflow with runs and validation records. The local project did
not yet contain a persisted handoff artifact case, so the report explicitly
prints `handoff_cases: none recorded yet`.

The recovery demo records a handoff case in an isolated database:

```bash
TOK_BIN=./bin/tok scripts/demo-agent-recovery.sh
TOK_BIN=./bin/tok scripts/dogfooding-metrics.sh \
  --project recovery-demo \
  --task 1 \
  --db /path/from/demo/.tok-data/tok.db
```

Use this as the public reproducible recovery sample until the main TOK project
has a real multi-agent handoff worth publishing.
