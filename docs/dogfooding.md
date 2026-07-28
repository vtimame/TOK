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
the execution workflow with runs and validation records. During task 193, TOK
also recorded a real persisted handoff/recovery case in the main `tok` project:

- run `63` was started for task `193` with `--handoff-output`, producing
  handoff artifact `222`;
- the run lease was assigned to `agent/first-worker` with a one-second TTL;
- `tok run recover` cancelled the stale run after the lease expired;
- run `62` remained the active continuation run for the same task;
- the post-recovery report printed `handoff_cases: present`,
  `handoff_artifacts: 1`, `recovered_runs: 1`, and `stale_leases_now: 0`.

The local report at that point was:

```text
tasks_total           122
tasks_done            119
runs_total            56
runs_per_done_task    0.47
active_runs           1
stale_leases_now      0
recovered_runs        1
validation_records    209
validation_blocks     1
completion_overrides  0
handoff_artifacts     1
handoff_cases         present
context_build_ms      33
```

The recovery sequence was:

```bash
mkdir -p .quality/dogfooding
tok run start \
  --task 193 \
  --handoff-output .quality/dogfooding/task-193-stale-handoff.md \
  --allow-active \
  --json
tok run heartbeat 63 --owner agent/first-worker --ttl 1s --json
sleep 2
tok run recover \
  --summary "Dogfooding recovery: agent/first-worker lease expired during task 193 handoff; Codex MCP continued from persisted TOK state." \
  --json
scripts/dogfooding-metrics.sh --project tok --task 193
```

The recovery demo records a handoff case in an isolated database:

```bash
TOK_BIN=./bin/tok scripts/demo-agent-recovery.sh
TOK_BIN=./bin/tok scripts/dogfooding-metrics.sh \
  --project recovery-demo \
  --task 1 \
  --db /path/from/demo/.tok-data/tok.db
```

Use this as the public reproducible recovery sample when a clean disposable
database is better than showing the local TOK project history.
