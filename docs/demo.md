# First Demo Flow

This demo uses an isolated TOK data directory and a tiny local git repository.
It is safe to run without touching your normal TOK workspace.

For the public crash/recovery scenario, use:

```bash
scripts/demo-agent-recovery.sh
```

The script creates a temporary repository, claims a task, records deterministic
handoff files, simulates a stale agent lease, recovers the stale run, attaches a
patch artifact, records validation evidence and completes the task with
`--evidence-run`.

## Create A Demo Workspace

```bash
mkdir hello-tok
cd hello-tok
git init
cat > README.md <<'EOF'
# Hello TOK

This repository is used for a TOK demo.
EOF

export TOK_DATA_DIR="$PWD/.tok-demo"
tok init
tok user set-name "Demo User"
tok project add "$PWD" --name hello-tok
```

## Create Work And Build Context

```bash
tok task create \
  --project hello-tok \
  --title "Add a usage note" \
  --description "Add a short TOK usage note to README." \
  --acceptance-criteria "README contains the TOK usage note."

tok index update --project hello-tok
tok context build --project hello-tok --task <task-id> --output handoff.md
tok task claim --project hello-tok <task-id>
tok task progress <task-id> --body "Context package reviewed."
```

## Make And Validate The Change

```bash
cat >> README.md <<'EOF'

Usage note: TOK keeps task context close to code.
EOF

tok run start --task <task-id> --handoff-output run-handoff.md

tok run validate <run-id> --timeout 2m --json -- \
  grep -q "TOK keeps task context" README.md

tok run finish <run-id> \
  --status succeeded \
  --summary "README usage note validated."

tok task done <task-id> \
  --note "Demo completed and validated." \
  --evidence-run <run-id>
```

## Inspect In The UI

```bash
tok ui serve --addr 127.0.0.1:7654
```

Then open:

```text
http://127.0.0.1:7654
```

Inspect the project, task history, run and validation evidence.

## Connect An MCP Client

Create an agent token and configure your MCP client to launch the stdio server:

```bash
tok agent add "Demo Agent"
TOK_AGENT_TOKEN="tok_agent_..." tok mcp serve --profile worker
```

Replace `tok_agent_...` with the token printed by `tok agent add`.
