# Development

Run the full test suite:

```bash
go test ./...
```

Check whitespace issues before committing:

```bash
git diff --check
```

Build the web UI:

```bash
cd web
pnpm install
pnpm typecheck
pnpm build
```

Run frontend linting and tests:

```bash
cd web
pnpm lint
pnpm test
```

Generate the OpenAPI-backed web client:

```bash
cd web
pnpm api:generate
```

Validate the committed OpenAPI schema:

```bash
cd web
pnpm api:validate
```

Check OpenAPI and generated client drift:

```bash
cd web
pnpm api:check
```

`api:check` validates the generated `openapi.json`, regenerates the Kubb client,
and fails if `web/openapi.json` or `web/src/api/generated` change during the
check. On a clean CI checkout, that catches any committed API/client drift.

Build the release-style binary with embedded web UI:

```bash
make build
```

Start the local development API and Vite app:

```bash
make dev-start
make dev-status
make dev-stop
```

The development instance intentionally uses separate ports so it does not
conflict with a production local TOK instance:

- API: `127.0.0.1:7655`
- Vite web app: `127.0.0.1:5174`

Run quality checks:

```bash
make quality
```

Quality checks include:

- Guarding oversized files by path-based budgets in `scripts/check-file-budgets.sh` (tracked files via `git ls-files`).
- Duplicate-code detection with `jscpd` using `.jscpd.json` and console/JSON reporters.
- Baseline comparison in `scripts/check-jscpd-baseline.mjs` against `.jscpd-baseline.json`.

The `jscpd` JSON report is written under `.quality/`, which is local transient
output. The committed `.jscpd-baseline.json` is the accepted duplication budget.
Lower the baseline when duplicate code is removed. Raise it only when the extra
duplication is intentional and reviewed.

Tracked file size budgets are:

- Default: 700 lines, 70 KB.
- Explicit exceptions:
  - `internal/app/run_cli.go`: 800 lines, 70 KB.
  - `internal/app/run_cli_parse.go`: 900 lines, 70 KB.
  - `internal/storage/tasks.go`: 750 lines, 70 KB.
  - `internal/app/task_cli.go`: 1,400 lines, 60 KB.
  - `internal/app/retrieval_cli.go`: 1,000 lines, 45 KB.
  - `internal/mcpserver/server.go`: 1,800 lines, 80 KB.
  - `internal/retrieval/retrieval.go`: 1,300 lines, 70 KB.
  - `web/src/components/ui/*`: 2,000 lines, 140 KB.

Code path policy:

- Business logic must stay in the `internal/service` layer.
- Transport layers must only parse inbound input, build DTOs, and format outbound output.
- Storage layer must only persist and retrieve state, with no orchestration or business decisions.

## Shell Completion

```bash
tok completion bash > ~/.local/share/bash-completion/completions/tok
tok completion zsh > ~/.zfunc/_tok
tok completion fish > ~/.config/fish/completions/tok.fish
```
