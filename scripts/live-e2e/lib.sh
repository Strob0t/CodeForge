# shellcheck shell=bash
# Shared helpers of the live end-to-end scripts (sourced, not executed).
#
# Loads the environment (LIVE_ENV_FILE, default: env.example.sh next to this
# file) and starts, waits for and stops the processes of the stack. Each
# process runs detached in its own session; its PID goes to
# $LIVE_DIR/run/<name>.pid and its output to $LIVE_DIR/logs/<name>.log.

LIVE_SCRIPTS_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
export LIVE_REPO_ROOT=${LIVE_REPO_ROOT:-$(cd "$LIVE_SCRIPTS_DIR/../.." && pwd)}

# shellcheck source=scripts/live-e2e/env.example.sh
. "${LIVE_ENV_FILE:-$LIVE_SCRIPTS_DIR/env.example.sh}"

mkdir -p "$LIVE_DIR/run" "$LIVE_DIR/logs" "$LIVE_DIR/bin" \
  "$CODEFORGE_WORKSPACE_ROOT" "$CODEFORGE_POLICY_DIR" \
  "$CODEFORGE_KNOWLEDGE_CONTENT_ROOT" "$CODEFORGE_WORKSPACE_ADOPT_ROOTS"

live_log() {
  printf '[live-e2e] %s\n' "$*" >&2
}

live_die() {
  live_log "error: $*"
  exit 1
}

live_pidfile() {
  printf '%s/run/%s.pid' "$LIVE_DIR" "$1"
}

# live_pid NAME prints the PID of a running process of the stack.
live_pid() {
  local pidfile pid
  pidfile=$(live_pidfile "$1")
  [ -f "$pidfile" ] || return 1
  pid=$(cat "$pidfile")
  if kill -0 "$pid" 2>/dev/null; then
    printf '%s\n' "$pid"
    return 0
  fi
  rm -f "$pidfile"
  return 1
}

# live_ensure_db creates the session's database (shared by the Core and
# LiteLLM) when psql is available.
live_ensure_db() {
  if ! command -v psql >/dev/null; then
    live_log "psql not found: create the database $LIVE_DB_NAME yourself if it does not exist"
    return 0
  fi
  if [ "$(psql "$LIVE_PG_URL/postgres" -tAc "SELECT 1 FROM pg_database WHERE datname = '$LIVE_DB_NAME'")" != 1 ]; then
    psql "$LIVE_PG_URL/postgres" -qc "CREATE DATABASE \"$LIVE_DB_NAME\""
    live_log "created database $LIVE_DB_NAME"
  fi
}

# live_start NAME DIR COMMAND... runs COMMAND in DIR, detached in a new
# session (so stop.sh can end its whole process group).
live_start() {
  local name=$1 dir=$2 log pid
  shift 2
  if pid=$(live_pid "$name"); then
    live_die "$name is already running (PID $pid); run stop.sh $name first"
  fi
  log="$LIVE_DIR/logs/$name.log"
  # Keep the previous log for the session notes.
  [ -s "$log" ] && mv "$log" "$log.$(date +%Y%m%d-%H%M%S)"
  (
    cd "$dir" || exit 1
    # The process outlives this script: it must not keep descriptors the
    # caller holds open, such as a lock file of a wrapper like flock(1).
    for fd in /proc/self/fd/*; do
      fd=${fd##*/}
      if [ "$fd" -gt 2 ]; then eval "exec $fd>&-"; fi
    done 2>/dev/null
    exec setsid "$@" </dev/null >"$log" 2>&1
  ) &
  printf '%s\n' "$!" >"$(live_pidfile "$name")"
  live_log "$name started (PID $!), log: $log"
}

# live_wait_http NAME URL SECONDS waits until URL answers with a 2xx status,
# and fails early when the process NAME has exited.
live_wait_http() {
  local name=$1 url=$2 deadline=$((SECONDS + $3))
  until curl -sf -o /dev/null "$url"; do
    if ! live_pid "$name" >/dev/null; then
      tail -n 30 "$LIVE_DIR/logs/$name.log" >&2
      live_die "$name exited before $url was ready"
    fi
    [ "$SECONDS" -lt "$deadline" ] || live_die "$name: $url not ready after $3 s"
    sleep 2
  done
  live_log "$name ready: $url"
}

# live_stop NAME ends the process group of NAME: TERM, then KILL after 30 s.
live_stop() {
  local name=$1 pid i
  if ! pid=$(live_pid "$name"); then
    live_log "$name is not running"
    return 0
  fi
  kill -TERM -- "-$pid" 2>/dev/null || kill -TERM "$pid" 2>/dev/null
  for i in $(seq 30); do
    kill -0 "$pid" 2>/dev/null || break
    [ "$i" -eq 30 ] && kill -KILL -- "-$pid" 2>/dev/null
    sleep 1
  done
  rm -f "$(live_pidfile "$name")"
  live_log "$name stopped"
}
