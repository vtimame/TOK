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

DB_PATH=""
PROJECT_NAME="tok"
TASK_ID=""

while [[ $# -gt 0 ]]; do
  case "$1" in
    --db)
      shift
      [[ $# -gt 0 ]] || { echo "--db requires a path" >&2; exit 2; }
      DB_PATH="$1"
      ;;
    --project)
      shift
      [[ $# -gt 0 ]] || { echo "--project requires a name" >&2; exit 2; }
      PROJECT_NAME="$1"
      ;;
    --task)
      shift
      [[ $# -gt 0 ]] || { echo "--task requires an id" >&2; exit 2; }
      TASK_ID="$1"
      ;;
    -h|--help)
      echo "Usage: scripts/dogfooding-metrics.sh [--project tok] [--task <task-id>] [--db /path/to/tok.db]"
      exit 0
      ;;
    *)
      echo "unknown option: $1" >&2
      exit 2
      ;;
  esac
  shift
done

if [[ "$PROJECT_NAME" == *"'"* ]]; then
  echo "project names containing single quotes are not supported by this report" >&2
  exit 2
fi

if [[ -z "$DB_PATH" ]]; then
  DB_PATH="$("$TOK_BIN" config paths --json | sed -n 's/.*"database_path": "\([^"]*\)".*/\1/p')"
fi

if [[ -z "$DB_PATH" || ! -f "$DB_PATH" ]]; then
  echo "TOK database not found: ${DB_PATH:-unknown}" >&2
  exit 1
fi

CONTEXT_DATA_DIR="$(dirname "$DB_PATH")"
project_count="$(sqlite3 -cmd ".timeout 10000" "$DB_PATH" "SELECT COUNT(*) FROM projects WHERE name = '$PROJECT_NAME';")"
if [[ "$project_count" == "0" ]]; then
  echo "project not found in TOK database: $PROJECT_NAME" >&2
  exit 1
fi

CONTEXT_BUILD_MS="not measured"
if [[ -n "$TASK_ID" ]]; then
  context_output="$(mktemp "${TMPDIR:-/tmp}/tok-context.XXXXXX.md")"
  started_ms="$(date +%s%3N)"
  TOK_DATA_DIR="$CONTEXT_DATA_DIR" "$TOK_BIN" context build --project "$PROJECT_NAME" --task "$TASK_ID" --output "$context_output" >/dev/null
  finished_ms="$(date +%s%3N)"
  CONTEXT_BUILD_MS="$((finished_ms - started_ms))"
  rm -f "$context_output"
fi

sqlite3 -cmd ".timeout 10000" -header -column "$DB_PATH" <<SQL
WITH selected_project AS (
  SELECT id, name, path FROM projects WHERE name = '$PROJECT_NAME'
),
project_tasks AS (
  SELECT t.* FROM tasks t JOIN selected_project p ON p.id = t.project_id
),
project_runs AS (
  SELECT r.* FROM runs r JOIN project_tasks t ON t.id = r.task_id
),
project_artifacts AS (
  SELECT a.* FROM run_artifacts a JOIN project_runs r ON r.id = a.run_id
),
project_events AS (
  SELECT e.* FROM task_events e JOIN project_tasks t ON t.id = e.task_id
)
SELECT 'project' AS metric, name AS value FROM selected_project
UNION ALL SELECT 'project_path', path FROM selected_project
UNION ALL SELECT 'tasks_total', CAST(COUNT(*) AS TEXT) FROM project_tasks
UNION ALL SELECT 'tasks_done', CAST(COALESCE(SUM(status = 'done'), 0) AS TEXT) FROM project_tasks
UNION ALL SELECT 'runs_total', CAST(COUNT(*) AS TEXT) FROM project_runs
UNION ALL SELECT 'runs_per_done_task', printf('%.2f', 1.0 * (SELECT COUNT(*) FROM project_runs) / NULLIF((SELECT COUNT(*) FROM project_tasks WHERE status = 'done'), 0))
UNION ALL SELECT 'active_runs', CAST(COALESCE(SUM(status = 'in_progress'), 0) AS TEXT) FROM project_runs
UNION ALL SELECT 'stale_leases_now', CAST(COALESCE(SUM(status = 'in_progress' AND expires_at <> '' AND expires_at < strftime('%Y-%m-%dT%H:%M:%fZ', 'now')), 0) AS TEXT) FROM project_runs
UNION ALL SELECT 'recovered_runs', CAST(COUNT(*) AS TEXT) FROM project_runs WHERE status = 'cancelled' AND result_summary LIKE '%Recover%'
UNION ALL SELECT 'validation_records', CAST(COUNT(*) AS TEXT) FROM project_artifacts WHERE kind = 'validation'
UNION ALL SELECT 'validation_blocks', CAST(COUNT(*) AS TEXT) FROM project_artifacts WHERE kind = 'validation' AND metadata LIKE '%"status":"failed"%'
UNION ALL SELECT 'completion_overrides', CAST(COUNT(*) AS TEXT) FROM project_events WHERE type = 'completion_override'
UNION ALL SELECT 'handoff_artifacts', CAST(COUNT(*) AS TEXT) FROM project_artifacts WHERE kind = 'handoff'
UNION ALL SELECT 'handoff_cases', CASE WHEN (SELECT COUNT(*) FROM project_artifacts WHERE kind = 'handoff') > 0 THEN 'present' ELSE 'none recorded yet' END;
SQL

printf "%-32s %s\n" "context_build_ms" "$CONTEXT_BUILD_MS"
