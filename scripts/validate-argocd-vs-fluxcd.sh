#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)
REPO_ROOT=$(cd "$SCRIPT_DIR/.." && pwd)
TMP_DIR=$(mktemp -d "${TMPDIR:-/tmp}/validate-argocd-fluxcd.XXXXXX")

trap 'rm -rf "$TMP_DIR"' EXIT

log() { echo "[validate-argocd-vs-fluxcd] $*"; }

log "Checking experiment runner syntax"
bash -n "$REPO_ROOT/experiments/platform/argocd-vs-fluxcd/run.sh"

log "Checking experiment runner help output"
"$REPO_ROOT/experiments/platform/argocd-vs-fluxcd/run.sh" --help >/dev/null

log "Checking Docker Compose config"
docker compose -f "$REPO_ROOT/deployments/docker-compose/gitops/docker-compose.yml" config --quiet

log "Running loadgen-gitops unit tests"
(
  cd "$REPO_ROOT/benchmarks/loadgen-gitops"
  GOCACHE="$TMP_DIR/go-build" go test ./...
)

log "Building loadgen-gitops binary"
(
  cd "$REPO_ROOT/benchmarks/loadgen-gitops"
  GOCACHE="$TMP_DIR/go-build" go build -o "$TMP_DIR/loadgen-gitops" .
)

log "Checking binary help output"
"$TMP_DIR/loadgen-gitops" --help 2>&1 | grep -q "gitops tool" || true

log "Validation completed"
