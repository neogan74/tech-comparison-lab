# Phase 03 — Experiment Runner (`run.sh`)

**Parent plan:** [plan.md](plan.md)  
**Dependencies:** Phase 01 (binary), Phase 02 (compose stack)  
**Date:** 2026-07-01  
**Status:** 🔲 Not started

## Key Insights

- Mirrors `experiments/cache/redis-vs-valkey/run.sh` exactly; only service names, ports, and flag names change
- Compose dir: `deployments/docker-compose/kv/`
- Binary: `benchmarks/loadgen-kv/bin/loadgen-kv`
- Results: `experiments/kv/etcd-vs-consul/results/{etcd,consul,summary}.json`

## Directory Structure

```
experiments/kv/etcd-vs-consul/
├── run.sh
├── README.md
└── results/
    └── .gitkeep
```

## run.sh Structure (mirrors redis-vs-valkey)

```bash
# Config (env-overridable)
COUNT="${COUNT:-10000}"            # KV pairs per write run
DURATION="${DURATION:-30}"         # seconds for sustained test
WORKERS="${WORKERS:-8}"
SMOKE_COUNT="${SMOKE_COUNT:-500}"

# Functions:
check_deps()       # docker, jq
build_binary()     # go build or use existing
start_stack()      # docker compose up -d
wait_for_etcd()    # curl http://localhost:2379/health until OK (max 30s)
wait_for_consul()  # curl http://localhost:8500/v1/status/leader until non-empty (max 30s)
smoke_test()       # --op all --count 100 fast pass
run_full()         # --op all --count $COUNT --duration $DURATION --workers $WORKERS
merge_results()    # jq -s to summary.json
verify_results()   # check all 3 files non-empty
print_table()      # jq + printf table
```

## merge_results() jq template

```bash
jq -s '{
  schema_version: "results-summary/v1",
  experiment: {
    id: "etcd-vs-consul",
    name: "etcd vs Consul",
    category: "kv",
    path: "experiments/kv/etcd-vs-consul"
  },
  run_id: .[0].run_id,
  timestamp: .[0].timestamp,
  mode: "full",
  config: {
    count: '"$COUNT"',
    duration: '"$DURATION"',
    workers: '"$WORKERS"'
  },
  sources: [
    {name: "etcd",   file: "results/etcd.json"},
    {name: "consul", file: "results/consul.json"}
  ],
  results: [.[].results[]]
}' "$RESULTS_DIR/etcd.json" "$RESULTS_DIR/consul.json" > "$RESULTS_DIR/summary.json"
```

## print_table() columns

```
DB       Operation    ops/s    p50(ms)   p99(ms)
------   ---------   ------   -------   -------
etcd     write        18500     0.42      2.30
consul   write        12000     0.71      3.10
etcd     read         24000     0.31      1.20
...
```

## Implementation Steps

1. `mkdir -p experiments/kv/etcd-vs-consul/results`
2. `touch experiments/kv/etcd-vs-consul/results/.gitkeep`
3. Write `run.sh` following the exact pattern from redis-vs-valkey
4. `chmod +x experiments/kv/etcd-vs-consul/run.sh`
5. Write `README.md` with experiment description, how to run, sample results table
6. Test locally: `./run.sh --smoke-only`

## README.md Sections

- Experiment overview (what etcd and consul are)
- Test scenarios table
- How to run (`./run.sh`, `./run.sh --smoke-only`, env vars)
- Sample results table (placeholder, fill after first run)
- Metrics explained

## Todo

- [ ] Create directory structure + .gitkeep
- [ ] Write run.sh (all functions)
- [ ] chmod +x
- [ ] Write README.md
- [ ] Test with --smoke-only locally

## Success Criteria

- `bash -n run.sh` passes (syntax check)
- `./run.sh --help` prints usage
- `./run.sh --smoke-only` completes in < 90s
- `./run.sh` (full) completes in < 6 min
- `results/summary.json` is valid JSON with `schema_version: "results-summary/v1"`
- At least 5 result entries (write/read/watch/election/mixed × 2 dbs = 10)
