#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
: "${PORT:?Devcraft must assign a service port}"
mkdir -p data/devcraft

# Publish the process identity with each port to reject stale worktree state.
printf '%s\n' "${DEVCRAFT_PUBLIC_URL:-http://localhost:$PORT}" > "data/devcraft/${1:?service required}.url"
printf '%s %s %s\n' "$$" "$PORT" "$PWD" > "data/devcraft/$1.port"

wait_for_service() {
  local service="$1"
  for ((attempt=0; attempt<120; attempt++)); do
    if read -r peer_pid peer_port peer_path < "data/devcraft/$service.port" 2>/dev/null &&
      [[ "$peer_path" == "$PWD" ]] && kill -0 "$peer_pid" 2>/dev/null; then
      return 0
    fi
    sleep 1
  done
  echo "Start all Devcraft services; $service has not published its port." >&2
  exit 1
}

case "$1" in
  web)
    wait_for_service api
    export VITE_BACKEND_PORT="$peer_port"
    export VITE_FRONTEND_PORT="$PORT"
    cd web
    exec bun run --bun dev --host 127.0.0.1
    ;;
  docs)
    cd guide
    exec bun run --bun dev --host 127.0.0.1 --port "$PORT"
    ;;
  api)
    wait_for_service web
    export VITE_FRONTEND_PORT="$peer_port"
    export SERVER_BASE_URL="$(cat data/devcraft/web.url)"
    wait_for_service docs
    export DOCS_URL="$(cat data/devcraft/docs.url)"
    export EMAIL_ENABLED=false
    make migrate
    go run ./cmd/devpreview
    exec make run
    ;;
  *) echo 'Usage: bash scripts/dev-service.sh {api|web|docs}' >&2; exit 1 ;;
esac
