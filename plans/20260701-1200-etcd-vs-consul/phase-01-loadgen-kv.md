# Phase 01 — Go Benchmark Tool (`loadgen-kv`)

**Parent plan:** [plan.md](plan.md)  
**Dependencies:** none  
**Date:** 2026-07-01  
**Priority:** High (blocks all other phases)  
**Status:** 🔲 Not started

## Key Insights

- Mirror `benchmarks/loadgen-cache` structure exactly: `main.go` + `internal/client/` + `internal/report/`
- Report package can be copy-adapted from `loadgen-cache/internal/report/report.go` (same `Result` / `Summary` structs)
- etcd uses gRPC client (`go.etcd.io/etcd/client/v3`); Consul uses HTTP REST client (`github.com/hashicorp/consul/api`)
- "Leader election" for etcd: use `concurrency.NewElection` + measure time-to-campaign; for Consul: use Session + Lock acquire time
- Watch latency: write a key then measure time until watch fires

## Architecture

```
benchmarks/loadgen-kv/
├── go.mod                          module github.com/tech-comparison-lab/loadgen-kv
├── go.sum
├── main.go                         CLI entry point (flag parsing, dispatch)
├── internal/
│   ├── client/
│   │   ├── etcd.go                 etcd v3 client wrapper
│   │   └── consul.go               Consul HTTP API wrapper
│   └── report/
│       └── report.go               Result/Summary structs + WriteJSON + PrintTable
└── bin/                            built binary (gitignored)
```

## CLI Flags

```
--db        etcd | consul           (required)
--op        write|read|watch|election|mixed|all  (default: all)
--count     int    total KV pairs / iterations   (default: 10000)
--workers   int    concurrent goroutines         (default: 8)
--duration  int    seconds for sustained-rate test (default: 30)
--addr      string host:port  (etcd: 2379, consul: 8500)
--out       string JSON output file
--dry-run   bool   connectivity check only
```

## Operations

| Op | What it measures |
|----|-----------------|
| `write` | PUT throughput (ops/sec) + p50/p99 latency |
| `read` | GET throughput + p50/p99 latency |
| `mixed` | 80% read / 20% write ratio |
| `watch` | Time from PUT to watch-event delivery (µs) |
| `election` | Time for leader campaign to succeed (ms) |

## Result JSON (per-op, per-db)

Follows the same `report.Result` struct as loadgen-cache:
```json
{
  "db": "etcd",
  "op": "write",
  "count": 10000,
  "workers": 8,
  "p50_ms": 0.42,
  "p95_ms": 1.1,
  "p99_ms": 2.3,
  "throughput_ops_sec": 18500,
  "total_ms": 540
}
```

Election and watch results add custom fields via `extra` map (or dedicated ops):
```json
{ "db": "etcd", "op": "election", "p50_ms": 12.5, "p99_ms": 45.0, ... }
{ "db": "etcd", "op": "watch",    "p50_ms": 0.8,  "p99_ms": 3.2,  ... }
```

## Implementation Steps

1. `mkdir -p benchmarks/loadgen-kv/internal/{client,report}`
2. Copy & adapt `report.go` from loadgen-cache (same structs, no changes needed)
3. Write `internal/client/etcd.go`:
   - `Connect(addr)` → `*clientv3.Client`
   - `Put(ctx, key, val)` → `time.Duration`
   - `Get(ctx, key)` → `time.Duration`
   - `Watch(ctx, key)` → channel of `time.Duration` (put-to-event)
   - `Campaign(ctx)` → `time.Duration` (election time)
4. Write `internal/client/consul.go`:
   - `Connect(addr)` → `*consul.Client`
   - `Put`, `Get` using KV API
   - `Watch` using blocking query (index-based long-poll)
   - `Campaign` using Session + Lock acquire
5. Write `main.go`: parse flags, run ops, collect `[]time.Duration`, call `report.Compute`, write JSON
6. Write `go.mod` with deps:
   - `go.etcd.io/etcd/client/v3`
   - `github.com/hashicorp/consul/api`
7. `go mod tidy && go build -o bin/loadgen-kv .`
8. Write unit tests for report package (`report_test.go`)

## Todo

- [ ] Create directory structure
- [ ] Copy report package
- [ ] Implement etcd client
- [ ] Implement consul client
- [ ] Implement main.go dispatch
- [ ] Write go.mod + go mod tidy
- [ ] Build binary and verify `--dry-run` works against local etcd/consul
- [ ] Write report_test.go

## Success Criteria

- `go build` succeeds
- `go test ./...` passes
- `./bin/loadgen-kv --db etcd --dry-run` connects to a running etcd
- `./bin/loadgen-kv --db consul --dry-run` connects to a running consul
- `--op all` produces valid JSON with results for all 5 ops

## Risk Assessment

- etcd v3 client brings in large gRPC dependency tree → `go mod tidy` may pull in many indirect deps (acceptable)
- Consul watch via blocking query is HTTP long-poll, not true push — latency measurement will include HTTP overhead (~1ms) which is still meaningful
- Election tests require no other campaigners; test must clean up election keys between runs

## Security Considerations

- Single-node, no auth, localhost only — CI environment, no security concerns
- No credentials stored; etcd connects without TLS (dev mode), Consul in dev mode (`-dev` flag)
