# Usage Guide

TOK is a local execution control plane for coding agents. Its core workflow is
the execution loop around a task: build deterministic context, claim work,
record one or more runs, keep lease and heartbeat state, preserve artifacts,
record validation evidence and complete the task with an auditable link to that
evidence.

TOK can sit alongside Jira, Linear or GitHub Issues. Those systems can remain
the backlog or planning source of truth; TOK tracks the local execution attempt
and evidence for work happening inside a repository. Local TOK tasks remain the
minimum supported mode and require no external service, network access or
tracker credentials.

## Web UI

TOK ships with a local HTTP API and web UI.

```bash
tok ui serve --addr 127.0.0.1:7654
```

Then open:

```text
http://127.0.0.1:7654
```

## MCP Server

TOK exposes project, task, run, index and search tools over MCP.

Create an agent identity:

```bash
tok agent add "Codex"
```

Start the MCP server:

```bash
TOK_AGENT_TOKEN="tok_agent_..." tok mcp serve
```

By default, TOK exposes the full MCP tool surface for backward compatibility.
Use a profile to limit the tools visible to an agent:

```bash
TOK_AGENT_TOKEN="tok_agent_..." tok mcp serve --profile worker
TOK_AGENT_TOKEN="tok_agent_..." tok mcp serve --profile supervisor
TOK_AGENT_TOKEN="tok_agent_..." tok mcp serve --profile admin
```

`worker` exposes only the core execution-loop tools:

```text
task_ready
task_claim
task_show
context_build
task_progress
task_block
run_create
run_validation_record
run_finish
task_done
```

`supervisor` includes the worker tools plus task/run inspection, task comments,
unblock/status operations, run listing, run artifact review and stale run
recovery. `admin` exposes the full surface, including project management,
agent identity management, instructions, indexing, search, dependencies, task
source references and manual artifact attachment.

`tok mcp serve` is a stdio server. When it is started without an MCP client
attached, it can exit cleanly as soon as stdin closes. In normal use, configure
your agent or editor to launch the command and keep the stdio connection open.

## Core Concepts

### Project

A registered repository or workspace. TOK stores its canonical path, display
name, task counts, project instructions and indexing metadata.

### Task

A unit of work with status, description, acceptance criteria, notes,
dependencies and events. In TOK, tasks are local execution records rather than
a replacement for a planning backlog. Tasks move through:

```text
open -> in_progress -> done
```

Tasks can also be blocked and unblocked. Blocked tasks are excluded from ready
work and cannot be claimed until unblocked.

Tasks can be local-only or linked to an external tracker item. The minimum
external reference contract is `source`, `external_id`, `external_url` and
`external_revision`; see [Task Sources and External Trackers](task-sources.md)
for the product decision, sync states and unsupported alpha scenarios.

### Context Package

A reproducible handoff bundle for humans or agents. It combines task state,
project state, dependencies, blockers, retrieval results and git repository
state into a compact package.

### Run

An attempt to perform work for a task. Runs can be started manually, wrapped
around a local command, or delegated to a local agent adapter. TOK records run
state, leases, artifacts and validation evidence.

### Validation

Evidence that a run was checked. TOK can execute a validation command and store
bounded output artifacts, or record manual validation evidence.

Completing a task records which succeeded run and validation artifact provided
the evidence. An explicit override can be used when validation is intentionally
skipped, but the override remains visible in the task history.

## Canonical Execution Workflow

The main TOK workflow is:

```text
claim
  -> context
  -> run
  -> progress / heartbeat
  -> artifacts
  -> validation
  -> finish
  -> task completion with evidence
```

| Step              | CLI                                                                                 | MCP tool                                    | Web API / UI                             | Audit trail                                                                                              |
| ----------------- | ----------------------------------------------------------------------------------- | ------------------------------------------- | ---------------------------------------- | -------------------------------------------------------------------------------------------------------- |
| Find ready work   | `tok task ready --project <name>`                                                   | `task_ready`                                | `GET /api/tasks/ready` / ready task list | no mutation                                                                                              |
| Claim task        | `tok task claim --project <name> <task-id>`                                         | `task_claim`                                | task claim action                        | task `claimed` event                                                                                     |
| Build context     | `tok context build --project <name> --task <task-id>`                               | `context_build`                             | task/context view                        | context package includes task state, project instructions, dependencies, retrieval results and git state |
| Start run         | `tok run start --task <task-id>`                                                    | `run_create`                                | run create action                        | run row with base branch/head, handoff contract, lease fields and actor                                  |
| Record progress   | `tok task progress <task-id> --body <text>`                                         | `task_progress`                             | progress/comment action                  | task `progress` event                                                                                    |
| Refresh lease     | `tok run heartbeat <run-id> --ttl 15m`                                              | supervisor recovery surface observes leases | run heartbeat action                     | run `lease_owner`, `heartbeat_at` and `expires_at` fields                                                |
| Attach artifacts  | `tok run record-artifact <run-id> --kind <kind> --input <path>`                     | admin `run_artifact_add`                    | artifact upload/action                   | run artifact row with kind, path/hash/size and actor                                                     |
| Record validation | `tok run validate <run-id> -- <command...>` or `tok run record-validation <run-id>` | `run_validation_record`                     | validation action                        | validation artifact with command, status and summary                                                     |
| Finish run        | `tok run finish <run-id> --status succeeded --summary <text>`                       | `run_finish`                                | run finish action                        | run terminal status and finished actor                                                                   |
| Complete task     | `tok task done <task-id> --note <text> --evidence-run <run-id>`                     | `task_done`                                 | task done action                         | task `completed` event with `evidence_run_id` and `evidence_artifact_id`                                 |

Completion without validation must use
`--allow-unvalidated --override-reason <text>`. TOK records that as a separate
`completion_override` event so later reviewers can distinguish a normal
validation-based completion from an intentional exception.

## Common Commands

### Projects

```bash
tok project add /path/to/repo --name my-project
tok project list --json
tok project show my-project --json
```

### Tasks

```bash
tok task create --project my-project --title "Fix pagination"
tok task create \
  --project my-project \
  --title "Fix GitHub issue" \
  --source github \
  --external-id 42 \
  --external-url https://github.com/example/repo/issues/42
tok task source <task-id> \
  --source linear \
  --external-id ENG-42 \
  --external-url https://linear.app/example/issue/ENG-42
tok task ready --project my-project --json
tok task claim --project my-project <task-id>
tok task progress <task-id> --body "Implemented the first slice."
tok task block <task-id> --reason "Waiting for API decision."
tok task unblock <task-id> --note "Decision recorded."
tok task done <task-id> --note "Implemented and validated." --evidence-run <run-id>
tok task show <task-id> --json
```

### Index And Search

```bash
tok index update --project my-project
tok index status --project my-project --json
tok index watch --project my-project
tok search --project my-project "pagination cursor" --json
```

TOK uses a local Bleve index. Indexing respects built-in skip rules and a
project ignore policy.

### Context

```bash
tok context build --project my-project --task <task-id> --limit 5
tok context build --project my-project --task <task-id> --output handoff.md
tok context build --project my-project --task <task-id> --json
```

### Runs

```bash
tok run start --task <task-id> --handoff-output handoff.md --json
tok run exec --task <task-id> --timeout 10m --json -- go test ./...
tok run validate <run-id> --timeout 2m --json -- go test ./...
tok run finish <run-id> --status succeeded --summary "Validation passed."
tok run finish <run-id> --status succeeded --summary "Manual override." --allow-unvalidated --override-reason "Why validation was skipped."
tok run list --project my-project --status in_progress --json
tok run show <run-id> --json
tok run heartbeat <run-id> --ttl 15m --json
tok run recover --summary "Recovered stale run." --json
```

`tok run exec` records validation evidence from the command result. Completing a
task requires a succeeded run with passed validation evidence, unless
`--allow-unvalidated --override-reason <text>` is used explicitly.

### Dogfooding Metrics

```bash
scripts/dogfooding-metrics.sh --project tok --task <task-id>
```

The report reads the local TOK SQLite database and prints task/run counts,
validation records, overrides, stale leases, recovery count, handoff artifacts
and context build latency for the supplied task. See
[Dogfooding Metrics](dogfooding.md).

## Agent Adapter Runs

`tok run agent` invokes a local adapter command and passes the task context via
file, stdin or environment variable. Adapter success is not validation evidence
by itself; record validation separately, or use an explicit audited override.

```bash
tok run agent \
  --task <task-id> \
  --context file \
  --timeout 30m \
  --json \
  -- ./my-agent-adapter
```

TOK provides the adapter with:

- `TOK_AGENT_ADAPTER_CONTRACT=tok.agent_adapter.v0`
- `TOK_RUN_ARTIFACT_DIR`
- `TOK_AGENT_RESULT_FILE`
- `TOK_AGENT_CONTEXT_MODE`
- `TOK_PROJECT_PATH`
- safe run, task and project environment variables

The adapter writes a structured result to `TOK_AGENT_RESULT_FILE`:

```json
{ "status": "succeeded", "summary": "Implemented and validated." }
```

Supported statuses:

```text
succeeded | failed | blocked | cancelled
```

TOK maps that result to the run outcome without parsing human-oriented stdout
or stderr.
