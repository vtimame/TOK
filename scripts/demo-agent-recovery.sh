#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
if [[ -z "${TOK_BIN:-}" ]]; then
  if [[ -x "$ROOT_DIR/bin/tok" ]]; then
    TOK_BIN="$ROOT_DIR/bin/tok"
  else
    TOK_BIN="tok"
  fi
fi

WORK_DIR="${1:-$(mktemp -d "${TMPDIR:-/tmp}/tok-recovery-demo.XXXXXX")}"
mkdir -p "$WORK_DIR"
cd "$WORK_DIR"

export TOK_DATA_DIR="$WORK_DIR/.tok-data"

echo "demo workspace: $WORK_DIR"

git init -q
git config user.email demo@example.invalid
git config user.name "TOK Demo"

cat > README.md <<'EOF'
# TOK Recovery Demo

This repository is used to demonstrate agent recovery.
EOF
git add README.md
git commit -q -m "Initial demo repository"

"$TOK_BIN" init >/dev/null
"$TOK_BIN" user set-name "Demo Supervisor" >/dev/null
"$TOK_BIN" project add "$WORK_DIR" --name recovery-demo >/dev/null

task_json="$("$TOK_BIN" task create \
  --project recovery-demo \
  --title "Add recovered note" \
  --description "Simulate a failed agent handoff and finish the task from preserved TOK state." \
  --acceptance-criteria "README contains the recovered agent note." \
  --json)"
task_id="$(printf '%s\n' "$task_json" | sed -n 's/.*"id": \([0-9][0-9]*\).*/\1/p' | head -1)"

"$TOK_BIN" index update --project recovery-demo >/dev/null
"$TOK_BIN" context build --project recovery-demo --task "$task_id" --output initial-handoff.md >/dev/null
"$TOK_BIN" task claim --project recovery-demo "$task_id" >/dev/null

stale_run_json="$("$TOK_BIN" run start --task "$task_id" --handoff-output stale-run-handoff.md --json)"
stale_run_id="$(printf '%s\n' "$stale_run_json" | sed -n 's/.*"id": \([0-9][0-9]*\).*/\1/p' | head -1)"
"$TOK_BIN" run heartbeat "$stale_run_id" --owner "agent/first-worker" --ttl 1s >/dev/null
sleep 2
"$TOK_BIN" run recover --summary "First worker stopped heartbeating; recovered for supervisor handoff." --json >/dev/null

"$TOK_BIN" task progress "$task_id" --body "Second worker resumed from stale-run-handoff.md and recovered run state." >/dev/null
cat >> README.md <<'EOF'

Recovered note: TOK preserved the task, handoff, stale lease and validation path.
EOF

recovery_run_json="$("$TOK_BIN" run start --task "$task_id" --handoff-output recovery-handoff.md --json)"
recovery_run_id="$(printf '%s\n' "$recovery_run_json" | sed -n 's/.*"id": \([0-9][0-9]*\).*/\1/p' | head -1)"
"$TOK_BIN" run record-artifact "$recovery_run_id" --kind patch --input README.md >/dev/null
"$TOK_BIN" run validate "$recovery_run_id" --timeout 2m --json -- \
  grep -q "Recovered note: TOK preserved" README.md >/dev/null
"$TOK_BIN" run finish "$recovery_run_id" \
  --status succeeded \
  --summary "Recovered worker validated README note." >/dev/null
"$TOK_BIN" task done "$task_id" \
  --note "Recovered demo task completed with validation evidence." \
  --evidence-run "$recovery_run_id" >/dev/null

echo "task_id=$task_id"
echo "stale_run_id=$stale_run_id"
echo "recovery_run_id=$recovery_run_id"
echo "handoff=$WORK_DIR/recovery-handoff.md"
echo "inspect:"
echo "  TOK_DATA_DIR=$TOK_DATA_DIR $TOK_BIN task show $task_id"
echo "  TOK_DATA_DIR=$TOK_DATA_DIR $TOK_BIN run show $stale_run_id"
echo "  TOK_DATA_DIR=$TOK_DATA_DIR $TOK_BIN run show $recovery_run_id"
