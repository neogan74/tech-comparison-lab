#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)
REPO_ROOT=$(cd "$SCRIPT_DIR/.." && pwd)
EXPERIMENT_DIR="$REPO_ROOT/experiments/mesh/istio-vs-linkerd"

log() { echo "[smoke-istio-vs-linkerd] $*"; }

SMOKE_REPLICAS="${SMOKE_REPLICAS:-1}"
SMOKE_ROUNDS="${SMOKE_ROUNDS:-1}"
SMOKE_COUNT="${SMOKE_COUNT:-200}"
SMOKE_WORKERS="${SMOKE_WORKERS:-5}"

log "Starting end-to-end smoke run"
(
  cd "$EXPERIMENT_DIR"
  SMOKE_REPLICAS="$SMOKE_REPLICAS" \
  SMOKE_ROUNDS="$SMOKE_ROUNDS" \
  SMOKE_COUNT="$SMOKE_COUNT" \
  SMOKE_WORKERS="$SMOKE_WORKERS" \
    ./run.sh --clean --smoke-only
)

for f in \
  "$EXPERIMENT_DIR/results/istio.json" \
  "$EXPERIMENT_DIR/results/linkerd.json" \
  "$EXPERIMENT_DIR/results/summary.json"; do
  [[ -s "$f" ]] || { echo "ERROR: expected result missing: $f" >&2; exit 1; }
done

log "Smoke run completed"
