# TOK

TOK - Task Operations Kernel is a local agent loops system by
KONSTRUKTORSKI BIRO Sitematika-26.

Public repository target:
`https://github.com/SISTEMIKA-26/Task-Operations-Kernel`.

Public Go module path:
`s26.sh/tok`.

## Current Status

This repository contains the TOK product source code.

## Manual Core Workflow

TOK's current core workflow is intentionally CLI-first. It does not start a
runner, daemon, MCP server or UI process. A human operator or external agent can
drive the loop explicitly:

```bash
tok init
tok project add /path/to/repo --name tok

tok task create \
  --project tok \
  --title "Implement workflow slice" \
  --description "Make the core loop reliable." \
  --acceptance-criteria "- ready\n- claim\n- context\n- done"

tok task ready --project tok --json
tok task claim --project tok --json
tok run start --task <task-id> --limit 5 --handoff-output handoff.md --json

tok index update --project tok
tok context build \
  --project tok \
  --task <task-id> \
  --output context.md

tok context build --project tok --task <task-id> --json
tok task progress <task-id> --body "Implemented the first slice."
tok run show <run-id> --json
tok run record-validation <run-id> --command "go test ./..." --status passed --summary "Tests pass."
tok run finish <run-id> --status succeeded --summary "Run completed."
tok task block <task-id> --reason "Waiting for a decision."
tok task unblock <task-id> --note "Decision recorded."
tok task done <task-id> --note "Implemented and tests pass."
tok task show <task-id> --json
```

The core invariants are:

- `ready` returns open tasks without active blockers.
- `claim` atomically moves ready work from `open` to `in_progress`.
- `context build` produces a reproducible handoff package from task state,
  project metadata, lexical retrieval results and git repository state.
- Handoff packages expose `tok.handoff.v0`, dependencies/blockers, suggested
  commands and V1 text sections for agent use.
- `run start` records an agent run attempt with the handoff contract version,
  retrieval limit and git start snapshot. With `--handoff-output`, it also
  writes the handoff package and records it as a run artifact. `run finish`
  records the attempt outcome without closing the task automatically.
- `run record-validation` stores a manual validation result as a run artifact;
  it records the command text and outcome, but does not execute anything.
- `progress`, `block` and `unblock` record typed task events; blocked tasks are
  excluded from `ready` and cannot be claimed.
- `done` records a completion event and moves an `in_progress` task to `done`.
- JSON output is available for machine-driven workflow steps.
