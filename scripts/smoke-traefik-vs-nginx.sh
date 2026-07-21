#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)
REPO_ROOT=$(cd "$SCRIPT_DIR/.." && pwd)
EXPERIMENT_DIR="$REPO_ROOT/experiments/api/traefik-vs-nginx"

log() {
  echo "[smoke-traefik-nginx] $*"
}

SMOKE_COUNT="${SMOKE_COUNT:-1000}"

log "Starting end-to-end smoke run"
(
  cd "$EXPERIMENT_DIR"
  SMOKE_COUNT="$SMOKE_COUNT" ./run.sh --smoke-only
)

for path in \
  "$EXPERIMENT_DIR/results/smoke-nginx.json" \
  "$EXPERIMENT_DIR/results/smoke-traefik.json"; do
  [[ -s "$path" ]] || { echo "ERROR: expected smoke result missing: $path" >&2; exit 1; }
done

log "Smoke run completed"
