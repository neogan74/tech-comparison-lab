#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)
REPO_ROOT=$(cd "$SCRIPT_DIR/.." && pwd)
TMP_DIR=$(mktemp -d "${TMPDIR:-/tmp}/validate-k8s-openshift.XXXXXX")

cleanup() {
  rm -rf "$TMP_DIR"
}

trap cleanup EXIT

log() {
  echo "[validate-k8s-openshift] $*"
}

log "Checking experiment runner syntax"
bash -n "$REPO_ROOT/experiments/orchestration/k8s-vs-openshift/run.sh"

log "Checking experiment runner help output"
# No --help flag in run.sh, check it's executable instead
if [ ! -x "$REPO_ROOT/experiments/orchestration/k8s-vs-openshift/run.sh" ]; then
  echo "error: run.sh is not executable" >&2
  exit 1
fi

log "Running loadgen-k8s tests"
(
  cd "$REPO_ROOT/benchmarks/loadgen-k8s"
  GOCACHE="$TMP_DIR/go-build" go test ./...
)

log "Validation completed"
