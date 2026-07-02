#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)
REPO_ROOT=$(cd "$SCRIPT_DIR/.." && pwd)
TMP_DIR=$(mktemp -d "${TMPDIR:-/tmp}/validate-etcd-consul.XXXXXX")

trap 'rm -rf "$TMP_DIR"' EXIT

log() { echo "[validate-etcd-consul] $*"; }

log "Checking experiment runner syntax"
bash -n "$REPO_ROOT/experiments/kv/etcd-vs-consul/run.sh"

log "Checking experiment runner help output"
"$REPO_ROOT/experiments/kv/etcd-vs-consul/run.sh" --help >/dev/null

log "Checking Docker Compose config"
docker compose -f "$REPO_ROOT/deployments/docker-compose/kv/docker-compose.yml" config --quiet

log "Running loadgen-kv tests"
(
  cd "$REPO_ROOT/benchmarks/loadgen-kv"
  GOCACHE="$TMP_DIR/go-build" go test ./...
)

log "Validation completed"
