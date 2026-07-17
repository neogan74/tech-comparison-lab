#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)
REPO_ROOT=$(cd "$SCRIPT_DIR/.." && pwd)
TMP_DIR=$(mktemp -d "${TMPDIR:-/tmp}/validate-kubernetes-vs-docker-swarm.XXXXXX")
trap 'rm -rf "$TMP_DIR"' EXIT

log() { echo "[validate-kubernetes-vs-docker-swarm] $*"; }

log "Checking runner syntax and help"
bash -n "$REPO_ROOT/experiments/orchestration/kubernetes-vs-docker-swarm/run.sh"
"$REPO_ROOT/experiments/orchestration/kubernetes-vs-docker-swarm/run.sh" --help >/dev/null

log "Running loadgen-scheduler tests"
(
  cd "$REPO_ROOT/benchmarks/loadgen-scheduler"
  GOCACHE="$TMP_DIR/go-build" go test ./...
)

log "Checking module files"
(cd "$REPO_ROOT/benchmarks/loadgen-scheduler" && go mod verify)

log "Validation completed"
