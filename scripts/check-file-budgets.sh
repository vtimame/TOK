#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

DEFAULT_MAX_LINES=700
DEFAULT_MAX_BYTES=70000
REGRESSION_COUNT=0

is_ignored() {
  local file="$1"
  case "$file" in
    # Test boilerplate
    *_test.go|*_test.ts|*_test.tsx|*_test.vue|*_test.js|*_test.mjs|*_test.jsx)
      return 0
      ;;
    # Generated code
    *.gen.go|*.pb.go|*_generated.go|web/src/api/generated/*|web/typed-router.d.ts)
      return 0
      ;;
    # Dependency/build artifact directories
    web/dist/*|web/.output/*|web/coverage/*|internal/httpserver/webdist/*|**/dist/*|**/build/*|**/coverage/*|**/node_modules/*)
      return 0
      ;;
    *)
      return 1
      ;;
  esac
}

get_budgets() {
  local file="$1"
  local max_lines="$DEFAULT_MAX_LINES"
  local max_bytes="$DEFAULT_MAX_BYTES"
  local reason="default budget"

  case "$file" in
    internal/storage/tasks.go)
      max_lines=750
      reason="storage task aggregate"
    ;;
    # run CLI was previously split from a 3400-line monolith into run_cli + run_cli_parse.
    # Keep strict, explicit budgets now that the split is complete.
    internal/app/run_cli.go)
      max_lines=800
      reason="run CLI dispatcher and handlers (strict)"
    ;;
    internal/app/task_cli.go)
      max_lines=1400
      max_bytes=60000
      reason="CLI command surface (bounded)"
      ;;
    internal/app/run_cli_parse.go)
      max_lines=900
      reason="run CLI parser and option validation (split)"
      ;;
    internal/app/retrieval_cli.go)
      max_lines=1000
      max_bytes=45000
      reason="CLI command surface (bounded)"
      ;;
    # MCP server handlers and DTOs are split across focused files now.
    # Keep server.go bounded to transport wiring and high-level command flow.
    internal/mcpserver/server.go)
      max_lines=1250
      max_bytes=60000
      reason="MCP transport wiring after handler/type split"
      ;;
    internal/retrieval/retrieval.go)
      max_lines=1300
      max_bytes=70000
      reason="retrieval orchestration"
      ;;
    web/src/components/ui/*)
      max_lines=2000
      max_bytes=140000
      reason="scaffolded UI component boilerplate"
      ;;
  esac

  printf "%s %s %s\n" "$max_lines" "$max_bytes" "$reason"
}

printf "Checking tracked source file budgets...\n"

while IFS= read -r -d '' file; do
  if [[ "$file" != cmd/* && "$file" != internal/* && "$file" != web/src/* ]]; then
    continue
  fi

  [[ -f "$ROOT_DIR/$file" ]] || continue

  if is_ignored "$file"; then
    continue
  fi

  read -r max_lines max_bytes reason <<<"$(get_budgets "$file")"

  lines=$(wc -l < "$ROOT_DIR/$file")
  bytes=$(wc -c < "$ROOT_DIR/$file")

  if (( lines > max_lines )) || (( bytes > max_bytes )); then
    REGRESSION_COUNT=$((REGRESSION_COUNT + 1))
    printf "REGRESSION: %-70s lines=%s (>%s) bytes=%s (>%s) reason=%s\n" \
      "$file" "$lines" "$max_lines" "$bytes" "$max_bytes" "$reason"
  fi
done < <(git -C "$ROOT_DIR" ls-files -z)

if (( REGRESSION_COUNT > 0 )); then
  echo ""
  echo "$REGRESSION_COUNT oversized-file budget regression(s) detected."
  exit 1
fi

echo "All tracked files are within size budgets."
