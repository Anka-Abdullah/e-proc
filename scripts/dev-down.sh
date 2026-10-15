#!/usr/bin/env bash
# Stop everything scripts/dev-up.sh started. Infra containers are left running.
set -uo pipefail

cd "$(dirname "$0")/.."

PID_DIR=".dev-pids"
if [ ! -d "$PID_DIR" ]; then
  echo "nothing to stop ($PID_DIR not found)"
  exit 0
fi

stopped=0
for pidfile in "$PID_DIR"/*.pid; do
  [ -e "$pidfile" ] || continue
  name="$(basename "$pidfile" .pid)"
  pid="$(cat "$pidfile")"
  if kill -0 "$pid" 2>/dev/null; then
    kill "$pid" 2>/dev/null && echo "stopped $name (pid $pid)"
    stopped=$((stopped + 1))
  else
    echo "not running: $name (pid $pid)"
  fi
  rm -f "$pidfile"
done

echo "$stopped process(es) stopped"
echo "infra containers still up; stop them with:"
echo "  docker compose -f deploy/docker/docker-compose.yml down"
