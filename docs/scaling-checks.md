# Scaling Checks

This page records local scale checks for TOK task, run, artifact and event
read paths.

## Reproducible Read-Path Benchmark

Run the storage read-path benchmark manually when changing task, run, artifact,
event, project count or task list query behavior:

```bash
make scale-bench
```

The target runs:

```bash
go test ./internal/storage -run '^$' -bench BenchmarkScaleReadPaths -benchmem -benchtime=1s -count=3
```

Tune the run locally when needed:

```bash
make scale-bench BENCHTIME=3s BENCHCOUNT=5
```

The benchmark generates an isolated SQLite fixture in the test temp directory
and does not read the developer's TOK database. Dataset:

- 3,000 tasks;
- 8,650 task events;
- 2,200 runs;
- 6,600 run artifacts;
- 200 indexed retrieval documents;
- hot task `1` with 252 events, 201 runs and 603 run artifacts.

Protected read paths:

- hot task details: task, project, task events, dependencies, runs and artifacts
  for each run;
- project task page: total count, bounded task list and per-task events for the
  page;
- project counts: full project task scan and ready-task calculation;
- runs/artifacts page: bounded project run list and artifacts for each listed
  run.

This is intentionally a manual/local benchmark rather than a default CI gate:
runtime depends on machine, SQLite build and filesystem characteristics. The Go
benchmark output includes `goos`, `goarch`, package and `cpu` metadata; keep
that output with any reported numbers. For CI, `go test ./...` compiles the
benchmark but does not execute the heavy fixture.

## 2026-07-28 Local Smoke Fixture

The check used an isolated `TOK_DATA_DIR` and a temporary project named
`scale-fixture`. Fixture data was inserted directly into SQLite after current
migrations were applied.

Dataset:

- 3,000 tasks;
- 8,650 task events;
- 2,200 runs;
- 6,600 run artifacts;
- 200 indexed retrieval documents;
- hot task `1` with 252 events and 201 runs for task-details stress.

Measured with a current binary built into `/tmp`:

| Check                                                                     | Result |
| ------------------------------------------------------------------------- | -----: |
| `tok task list --project scale-fixture --json`                            |  23 ms |
| `tok task show 1 --json`                                                  |  10 ms |
| `tok run list --task 1 --json`                                            |  11 ms |
| `tok context build --project scale-fixture --task 1 --output ...`         |  13 ms |
| `scripts/dogfooding-metrics.sh --project scale-fixture --task 1 --db ...` |  35 ms |

HTTP API measurements from `tok ui serve --addr 127.0.0.1:7668`:

| Endpoint                                     |                 Result |
| -------------------------------------------- | ---------------------: |
| `/api/health`                                |      7.69 ms, 32 bytes |
| `/api/tasks/1`                               | 8.88 ms, 281,565 bytes |
| `/api/projects/scale-fixture/tasks?limit=50` |  3.94 ms, 24,643 bytes |
| `/api/tasks?projectId=1&limit=50`            |  2.49 ms, 24,643 bytes |

Observations:

- These are local dogfooding smoke numbers from one machine, not an independent
  benchmark or release performance claim.
- Task details stayed responsive with hundreds of events/runs on one task.
- Web list endpoints use bounded result sizes and remained small.
- CLI full project task JSON is intentionally larger: about 1.47 MB for 3,000
  tasks. This is acceptable for local diagnostics, but humans should prefer
  status filters or Web pagination when scanning large projects.
- Context package latency remained low with the indexed fixture.
- No separate performance task was opened from this run.
