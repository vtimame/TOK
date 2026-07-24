#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
STATE_DIR="$ROOT_DIR/.dev"
WEB_DIR="$ROOT_DIR/web"

API_PORT="7654"
APP_PORT="5173"
API_ADDR="127.0.0.1:$API_PORT"
APP_ADDR="127.0.0.1:$APP_PORT"

mkdir -p "$STATE_DIR"

pid_file() {
  printf '%s/%s.pid\n' "$STATE_DIR" "$1"
}

log_file() {
  printf '%s/%s.log\n' "$STATE_DIR" "$1"
}

is_alive() {
  local pid="${1:-}"
  [[ -n "$pid" ]] && kill -0 "$pid" 2>/dev/null
}

managed_pid() {
  local name="$1"
  local file
  file="$(pid_file "$name")"
  if [[ -f "$file" ]]; then
    tr -d '[:space:]' < "$file"
  fi
}

is_managed_running() {
  local name="$1"
  local pid
  pid="$(managed_pid "$name")"
  is_alive "$pid"
}

port_in_use() {
  local port="$1"
  (echo >"/dev/tcp/127.0.0.1/$port") >/dev/null 2>&1
}

ensure_port_free_or_managed() {
  local name="$1"
  local port="$2"

  if ! port_in_use "$port"; then
    return
  fi

  if is_managed_running "$name"; then
    echo "$name is already running on port $port (managed pid $(managed_pid "$name"))."
    exit 0
  fi

  echo "Port $port is already occupied by an unmanaged process. Stop it manually." >&2
  echo "Refusing to run fuser/kill on an unmanaged process." >&2
  exit 1
}

start_service() {
  local name="$1"
  local port="$2"
  local cwd="$3"
  local command="$4"
  local pid

  pid="$(managed_pid "$name")"
  if is_alive "$pid"; then
    echo "$name is already running (managed pid $pid)."
    return
  fi

  ensure_port_free_or_managed "$name" "$port"

  echo "Starting $name on port $port..."
  (
    cd "$cwd"
    setsid bash -lc "$command" >"$(log_file "$name")" 2>&1 &
    echo "$!" >"$(pid_file "$name")"
  )

  pid="$(managed_pid "$name")"
  for _ in {1..300}; do
    if port_in_use "$port"; then
      echo "$name started (managed pid $pid, port $port, log $(log_file "$name"))."
      return
    fi

    if ! is_alive "$pid"; then
      echo "$name failed to start. See $(log_file "$name")." >&2
      exit 1
    fi

    sleep 0.2
  done

  echo "$name did not become ready on port $port within 60s. See $(log_file "$name")." >&2
  exit 1
}

stop_service() {
  local name="$1"
  local pid
  pid="$(managed_pid "$name")"

  if ! is_alive "$pid"; then
    rm -f "$(pid_file "$name")"
    echo "$name is not running under devctl."
    return
  fi

  echo "Stopping $name (managed pid $pid)..."
  kill -TERM "-$pid" 2>/dev/null || kill -TERM "$pid" 2>/dev/null || true

  for _ in {1..30}; do
    if ! is_alive "$pid"; then
      rm -f "$(pid_file "$name")"
      echo "$name stopped."
      return
    fi
    sleep 0.2
  done

  echo "$name did not stop after SIGTERM; sending SIGKILL to managed process group." >&2
  kill -KILL "-$pid" 2>/dev/null || kill -KILL "$pid" 2>/dev/null || true
  rm -f "$(pid_file "$name")"
}

status_service() {
  local name="$1"
  local port="$2"
  local pid
  pid="$(managed_pid "$name")"

  if is_alive "$pid"; then
    echo "$name: running (managed pid $pid, port $port, log $(log_file "$name"))"
  elif port_in_use "$port"; then
    echo "$name: port $port is occupied by an unmanaged process"
  else
    echo "$name: stopped (port $port is free)"
  fi
}

api_start() {
  start_service "api" "$API_PORT" "$ROOT_DIR" "go run ./cmd/tok ui serve --addr $API_ADDR"
}

app_start() {
  start_service "app" "$APP_PORT" "$WEB_DIR" "pnpm dev --host 127.0.0.1 --port $APP_PORT"
}

show_logs() {
  local name="${1:-all}"
  case "$name" in
    api|app)
      tail -n 120 -f "$(log_file "$name")"
      ;;
    all)
      echo "API log: $(log_file api)"
      [[ -f "$(log_file api)" ]] && tail -n 80 "$(log_file api)" || true
      echo
      echo "App log: $(log_file app)"
      [[ -f "$(log_file app)" ]] && tail -n 80 "$(log_file app)" || true
      ;;
    *)
      echo "Usage: $0 logs [api|app]" >&2
      exit 2
      ;;
  esac
}

case "${1:-status}" in
  start)
    api_start
    app_start
    ;;
  stop)
    stop_service app
    stop_service api
    ;;
  restart)
    "$0" stop
    "$0" start
    ;;
  status)
    status_service api "$API_PORT"
    status_service app "$APP_PORT"
    ;;
  logs)
    show_logs "${2:-all}"
    ;;
  api-start)
    api_start
    ;;
  api-stop)
    stop_service api
    ;;
  app-start)
    app_start
    ;;
  app-stop)
    stop_service app
    ;;
  *)
    cat >&2 <<EOF
Usage: $0 <command>

Commands:
  start        Start API and frontend.
  stop         Stop managed API and frontend processes.
  restart      Restart managed API and frontend processes.
  status       Show managed process and port status.
  logs [name]  Show logs for all, api, or app.
  api-start    Start managed API on $API_ADDR.
  api-stop     Stop managed API.
  app-start    Start managed frontend on $APP_ADDR.
  app-stop     Stop managed frontend.
EOF
    exit 2
    ;;
esac
