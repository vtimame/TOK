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
pnpm build
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

## Shell Completion

```bash
tok completion bash > ~/.local/share/bash-completion/completions/tok
tok completion zsh > ~/.zfunc/_tok
tok completion fish > ~/.config/fish/completions/tok.fish
```
