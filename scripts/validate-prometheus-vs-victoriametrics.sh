#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)
REPO_ROOT=$(cd "$SCRIPT_DIR/.." && pwd)
TMP_DIR=$(mktemp -d "${TMPDIR:-/tmp}/validate-prom-vm.XXXXXX")

cleanup() {
  rm -rf "$TMP_DIR"
}

trap cleanup EXIT

log() {
  echo "[validate-prom-vm] $*"
}

log "Checking experiment runner syntax"
bash -n "$REPO_ROOT/experiments/observability/prometheus-vs-victoriametrics/run.sh"

log "Checking experiment runner help output"
"$REPO_ROOT/experiments/observability/prometheus-vs-victoriametrics/run.sh" --help >/dev/null

log "Checking Docker Compose config"
docker compose -f "$REPO_ROOT/deployments/docker-compose/observability/docker-compose.yml" config --quiet

log "Running loadgen-observability tests"
(
  cd "$REPO_ROOT/benchmarks/loadgen-observability"
  GOCACHE="$TMP_DIR/go-build" go vet ./...
  GOCACHE="$TMP_DIR/go-build" go test ./...
)

log "Validation completed"
