# TOK

TOK, the Task Operations Kernel, is a local-first workflow system for
agentic software development.

It gives developers and coding agents a shared operating layer for:

- project-aware task tracking;
- reproducible task context packages;
- local code indexing and search;
- agent run history, leases and artifacts;
- validation evidence before work is marked complete;
- MCP tools for IDEs and AI coding agents.

TOK is not trying to replace Jira, Linear or GitHub Issues. It is designed to
sit closer to the codebase: a small local control plane for the work that
happens when humans and agents collaborate inside a repository.

## Status

TOK is alpha software. The core CLI, local SQLite storage, retrieval index,
HTTP API, web UI and MCP server are usable today, but public contracts can
change while the project remains on `v0.x.y`.

The first public alpha release is planned as `v0.1.0`. TOK uses Semantic
Versioning; the git release tag is the source of truth for the product version
shown by `tok version`.

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
tok task done <task-id> --note "Implemented and validated."
```

## Documentation

- [Install TOK](docs/install.md)
- [First Demo Flow](docs/demo.md)
- [Usage Guide](docs/usage.md)
- [Alpha Notes and Safety](docs/alpha.md)
- [Development](docs/development.md)
- [Security Policy](SECURITY.md)

## License

TOK is licensed under the Apache License, Version 2.0. See `LICENSE`.
