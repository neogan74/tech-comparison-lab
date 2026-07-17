#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)
REPO_ROOT=$(cd "$SCRIPT_DIR/.." && pwd)
TMP_DIR=$(mktemp -d "${TMPDIR:-/tmp}/validate-kubernetes-vs-nomad.XXXXXX")
trap 'rm -rf "$TMP_DIR"' EXIT

log() { echo "[validate-kubernetes-vs-nomad] $*"; }

log "Checking runner syntax and help"
bash -n "$REPO_ROOT/experiments/orchestration/kubernetes-vs-nomad/run.sh"
"$REPO_ROOT/experiments/orchestration/kubernetes-vs-nomad/run.sh" --help >/dev/null

log "Running loadgen-scheduler tests"
(
  cd "$REPO_ROOT/benchmarks/loadgen-scheduler"
  GOCACHE="$TMP_DIR/go-build" go test ./...
)

log "Checking module files"
(cd "$REPO_ROOT/benchmarks/loadgen-scheduler" && go mod verify)

log "Validation completed"
