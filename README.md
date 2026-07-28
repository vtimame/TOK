# TOK

TOK, the Task Operations Kernel, is a local execution control plane for coding
agents.

Coding agents are useful, but their work is easy to lose track of. A chat can
say a task is done while the repository has no durable record of:

- what context the agent saw;
- which attempt performed the work;
- whether the attempt is still alive;
- which artifacts were produced;
- which validation actually passed;
- why a task was completed or overridden.

TOK keeps that execution trail local and explicit. It connects tasks,
deterministic context, claimed work, runs, leases, artifacts, validation
evidence and completion history so agent work is reproducible, recoverable and
auditable.

For example, if an agent crashes after a long run, TOK can still preserve:

- the claimed task and project instructions;
- the context package and repository base revision;
- the run lease and last heartbeat;
- produced artifacts and command output;
- validation state;
- enough handoff information for another agent or human to continue.

TOK is not trying to replace Jira, Linear or GitHub Issues. It is designed to
sit closer to the codebase as the execution layer for work that happens after a
task is selected: claim, context, run, heartbeat, artifacts, validation and
completion evidence. Local TOK tasks remain the dependency-free default, while
external tracker references are a planned integration path.

It also is not another `AGENTS.md` or shell-script convention. `AGENTS.md`
explains how to work; TOK records what actually happened. Shell scripts can run
checks; TOK attaches those checks to runs and task completion. GitHub Issues can
track the backlog; TOK tracks local execution evidence.

TOK is not a sandbox, cloud sync service, multi-tenant platform or team
workflow product. It is intended for trusted local developer machines where
humans and coding agents work on the same repositories.

## Status

TOK is alpha software. The core CLI, local SQLite storage, retrieval index,
HTTP API, web UI and MCP server are usable today, but public contracts can
change while the project remains on `v0.x.y`.

The current public alpha release is `v0.2.1`. TOK uses Semantic Versioning;
the git release tag is the source of truth for the product version shown by
`tok version`.

Public repository target:

```text
https://github.com/vtimame/TOK
```

Public Go module path:

```text
s26.sh/tok
```

## Quickstart

Install TOK from the latest GitHub Release, then initialize local storage:

```bash
tok version
tok init
tok user set-name "Your Name"
```

Register a local git repository and create your first task:

```bash
tok project add /path/to/repo --name my-project

tok task create \
  --project my-project \
  --title "Implement the first workflow slice" \
  --description "Make the smallest useful change." \
  --acceptance-criteria "The project builds and the relevant tests pass."
```

Build context and record work:

```bash
tok index update --project my-project
tok context build --project my-project --task <task-id> --output handoff.md
tok task claim --project my-project <task-id>
tok run exec --task <task-id> --timeout 10m --json -- go test ./...
tok task done <task-id> --note "Implemented and validated." --evidence-run <run-id>
```

`tok run exec` records validation evidence from the command result. For agent
adapter runs, finish with passed validation evidence or use an explicit audited
override.

## Documentation

- [Install TOK](docs/install.md)
- [First Demo Flow](docs/demo.md)
- [Dogfooding Metrics](docs/dogfooding.md)
- [Usage Guide](docs/usage.md)
- [Task Sources and External Trackers](docs/task-sources.md)
- [Alpha Notes and Safety](docs/alpha.md)
- [Development](docs/development.md)
- [Migration Checks](docs/migration-checks.md)
- [Scaling Checks](docs/scaling-checks.md)
- [Release Distribution Trust](docs/release-distribution.md)
- [Release checklist and draft notes](docs/release-checklist.md)
- [Security Policy](SECURITY.md)

## License

TOK is licensed under the Apache License, Version 2.0. See `LICENSE`.
