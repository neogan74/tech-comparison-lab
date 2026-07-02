# Phase 04 — Validate & Smoke Scripts

**Parent plan:** [plan.md](plan.md)  
**Dependencies:** Phase 01, 02, 03  
**Date:** 2026-07-01  
**Status:** 🔲 Not started

## Key Insights

- Mirrors `scripts/validate-redis-vs-valkey.sh` and `scripts/smoke-redis-vs-valkey.sh` exactly
- Validate: syntax check + compose config check + `go test ./...` (no Docker services needed)
- Smoke: starts services, runs `run.sh --smoke-only`, tears down via `trap cleanup EXIT`

## validate-etcd-vs-consul.sh

```bash
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
```

## smoke-etcd-vs-consul.sh

```bash
#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)
REPO_ROOT=$(cd "$SCRIPT_DIR/.." && pwd)
COMPOSE_FILE="$REPO_ROOT/deployments/docker-compose/kv/docker-compose.yml"
EXPERIMENT_DIR="$REPO_ROOT/experiments/kv/etcd-vs-consul"

log() { echo "[smoke-etcd-consul] $*"; }

cleanup() {
  log "Stopping benchmark stack"
  docker compose -f "$COMPOSE_FILE" down -v --remove-orphans >/dev/null 2>&1 || true
}
trap cleanup EXIT

SMOKE_COUNT="${SMOKE_COUNT:-100}"

log "Starting end-to-end smoke run"
(
  cd "$EXPERIMENT_DIR"
  SMOKE_COUNT="$SMOKE_COUNT" ./run.sh --clean --smoke-only
)

log "Smoke run completed"
```

## Implementation Steps

1. Write `scripts/validate-etcd-vs-consul.sh`
2. Write `scripts/smoke-etcd-vs-consul.sh`
3. `chmod +x` both scripts
4. Run validate locally (no Docker services needed): `bash scripts/validate-etcd-vs-consul.sh`
5. Run smoke locally: `bash scripts/smoke-etcd-vs-consul.sh`

## Todo

- [ ] Write validate script
- [ ] Write smoke script
- [ ] chmod +x both
- [ ] Test validate (fast, no services)
- [ ] Test smoke (full end-to-end, ~2 min)

## Success Criteria

- Validate completes < 60s without running any containers
- Smoke completes < 120s (CI budget for `--smoke-only`)
- Both exit 0
- Smoke cleanup removes all containers/volumes on EXIT (even on failure)
