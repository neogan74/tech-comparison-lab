# Phase 02 — loadgen-db (Go Benchmark Tool)

**Parent plan:** [plan.md](./plan.md)
**Dependencies:** Phase 01 (running stack for local testing)
**Status:** pending
**Priority:** high
**Date:** 2026-03-16

---

## Overview

A self-contained Go CLI (`loadgen-db`) that drives insert, query, aggregation,
and update workloads against both PostgreSQL and MongoDB. Emits per-operation
latency arrays, computes p50/p95/p99/min/max/mean/throughput, prints a stdout
table, and writes `results/summary.json`. Single binary, no external
dependencies beyond Go standard library + two driver packages.

---

## Key Insights

- **Driver choice matters for fairness.** `pgx/v5` with `pgxpool` bypasses
  `database/sql` overhead; `mongo-driver/v2` uses the official Go driver.
  Both drivers support batch/bulk operations natively — critical for insert
  throughput comparison.
- **Batch sizing:** 1000 docs/batch is the sweet spot for both engines.
  Postgres `COPY` protocol or multi-row `INSERT` avoids per-statement RTT.
  MongoDB `InsertMany` with `Ordered: false` maximizes throughput.
- **Worker concurrency model:** Use `sync.WaitGroup` + buffered channel of
  work items (doc batches). Each worker goroutine pulls a batch, executes,
  records duration. This avoids goroutine-per-request overhead at 10M scale.
- **Latency recording:** `[]time.Duration` slice pre-allocated to expected
  count. `sort.Slice` for percentile computation. No external histogram library
  needed (YAGNI).
- **Storage size queries:** PostgreSQL uses `pg_total_relation_size('orders')`;
  MongoDB uses `db.runCommand({collStats: "orders"}).storageSize`. Both return
  bytes — directly comparable.
- **Index creation timing:** Postgres indexes are pre-created via `init.sql`
  (so timing reflects insert-into-indexed-table cost, not index build). MongoDB
  indexes created by the tool before insert phase; index build time recorded
  separately.
- **Document generation:** `google/uuid` for UUID generation is a single
  dep addition worth the reliability. Alternatively `crypto/rand` + formatting
  — prefer the latter to keep dep count minimal.
- **Flag design:** `--db postgres|mongo`, `--op insert|query|agg|update|all`,
  `--count N`, `--batch 1000`, `--workers 8`, `--dsn <connection-string>`,
  `--out results/summary.json`. `--op all` runs the full sequence.

---

## Requirements

### Functional
- CLI flags: `--db`, `--op`, `--count`, `--batch`, `--workers`, `--dsn`, `--out`
- Operations: insert, query, agg, update (each as separate `--op` value; `all` runs sequence)
- Document schema matches spec exactly (9 fields, ~500 bytes)
- Insert: batch=1000, workers=8, total=N docs
- Query: `user.country='US'` LIMIT 100, N iterations, workers=8
- Agg: GROUP BY `user.id` SUM(`quantity`), N iterations (single-threaded is fine given 10 iterations)
- Update: set `status='delivered'` WHERE `status='shipped'` LIMIT 1000 per call, N iterations
- Metrics: p50, p95, p99, min, max, mean, throughput (ops/sec), total duration
- Output: table to stdout + JSON file at `--out` path
- Storage size queried after insert and included in JSON output
- Exit code 0 on success, non-zero on error

### Non-Functional
- Single `go build ./...` from `benchmarks/loadgen-db/` produces runnable binary
- No CGo dependencies
- Compilable on linux/amd64 and darwin/arm64
- `go vet ./...` and `go test ./...` pass

---

## Architecture

```
benchmarks/loadgen-db/
├── go.mod                         module: github.com/tech-comparison-lab/loadgen-db
├── go.sum
├── main.go                        flag parsing, dispatch, top-level error handling
└── internal/
    ├── postgres/
    │   └── bench.go               Postgres benchmark implementation
    ├── mongo/
    │   └── bench.go               MongoDB benchmark implementation
    └── report/
        └── report.go              Latency collection, percentile math, table+JSON output
```

### Package Responsibilities

**`main.go`**
```
- Parse flags via `flag` stdlib package
- Validate --db and --op values
- Instantiate correct bench impl (postgres.New | mongo.New)
- Call bench.Setup() → bench.Run(op) → report.Print() → report.WriteJSON()
- Return exit 1 on any error with descriptive message
```

**`internal/report/report.go`**
```go
type Result struct {
    DB         string        `json:"db"`
    Operation  string        `json:"op"`
    Count      int           `json:"count"`
    Workers    int           `json:"workers"`
    Latencies  []time.Duration  // not serialized — derived fields below
    P50        time.Duration `json:"p50_ms"`
    P95        time.Duration `json:"p95_ms"`
    P99        time.Duration `json:"p99_ms"`
    Min        time.Duration `json:"min_ms"`
    Max        time.Duration `json:"max_ms"`
    Mean       time.Duration `json:"mean_ms"`
    Throughput float64       `json:"throughput_ops_sec"`
    StorageBytes int64       `json:"storage_bytes,omitempty"`
    IndexBuildMs int64       `json:"index_build_ms,omitempty"`
    TotalMs    int64         `json:"total_ms"`
}

type Summary struct {
    RunID     string    `json:"run_id"`
    Timestamp string    `json:"timestamp"`
    Results   []Result  `json:"results"`
}
```

Percentile function: sort `[]time.Duration`, index at `int(math.Ceil(p/100*float64(n))) - 1`.

**`internal/postgres/bench.go`**
```
type Bench struct { pool *pgxpool.Pool }

func New(dsn string) (*Bench, error)         // opens pool, pings
func (b *Bench) Insert(count, batch, workers int) ([]time.Duration, error)
  // generates docs, sends batches via channel to workers
  // each worker: pgx CopyFrom or multi-row INSERT VALUES (...)
  // records time.Duration per batch → flatten to per-doc durations
func (b *Bench) Query(iterations, workers int) ([]time.Duration, error)
  // SELECT id, doc FROM orders WHERE doc @> '{"user":{"country":"US"}}' LIMIT 100
  // OR: WHERE doc->>'user'->>'country' = 'US' LIMIT 100 (expression index path)
  // records duration per iteration
func (b *Bench) Agg(iterations int) ([]time.Duration, error)
  // SELECT doc->>'user'->>'id', SUM((doc->>'quantity')::int)
  //   FROM orders GROUP BY 1
  // records duration per iteration
func (b *Bench) Update(iterations int) ([]time.Duration, error)
  // UPDATE orders SET doc = jsonb_set(doc, '{status}', '"delivered"')
  //   WHERE doc->>'status' = 'shipped' LIMIT 1000
  // records duration per iteration
func (b *Bench) StorageSize() (int64, error)
  // SELECT pg_total_relation_size('orders')
```

Note on JSONB update: PostgreSQL has no `UPDATE ... LIMIT`. Use CTE:
```sql
WITH target AS (
  SELECT id FROM orders WHERE doc->>'status' = 'shipped' LIMIT 1000
)
UPDATE orders SET doc = jsonb_set(doc, '{status}', '"delivered"')
WHERE id IN (SELECT id FROM target);
```

**`internal/mongo/bench.go`**
```
type Bench struct { client *mongo.Client; coll *mongo.Collection }

func New(dsn string) (*Bench, error)         // connects, pings
func (b *Bench) EnsureIndexes() (int64, error)
  // creates compound indexes, returns build duration in ms
func (b *Bench) Insert(count, batch, workers int) ([]time.Duration, error)
  // generates bson.D docs, InsertMany with Ordered:false
func (b *Bench) Query(iterations, workers int) ([]time.Duration, error)
  // Find({"user.country":"US"}).Limit(100).All(ctx, &results)
func (b *Bench) Agg(iterations int) ([]time.Duration, error)
  // Aggregate pipeline: $group by user.id, $sum quantity
func (b *Bench) Update(iterations int) ([]time.Duration, error)
  // UpdateMany({status:"shipped"}, {$set:{status:"delivered"}}, limit via $limit in pipeline...
  // or: find 1000 IDs then UpdateMany({_id:{$in:[...]}})
func (b *Bench) StorageSize() (int64, error)
  // db.RunCommand({collStats:"orders"}).storageSize
```

### Document Generation

Use a shared `genDoc()` function (in a `internal/docgen` package or inlined).
Countries: fixed pool of 10 ISO codes, weighted ~40% "US" to ensure query hits.
Statuses: ["pending","processing","shipped","delivered"] with ~50% "shipped".
Tags: random 0-3 tags from pool of 10.
UUIDs: `crypto/rand` + `fmt.Sprintf("%08x-%04x-...")` — avoids external dep.

---

## Related Code Files

- `benchmarks/loadgen-db/main.go`
- `benchmarks/loadgen-db/go.mod`
- `benchmarks/loadgen-db/internal/postgres/bench.go`
- `benchmarks/loadgen-db/internal/mongo/bench.go`
- `benchmarks/loadgen-db/internal/report/report.go`

---

## Implementation Steps

1. `mkdir -p benchmarks/loadgen-db/internal/{postgres,mongo,report}`
2. Write `go.mod`
   - Module: `github.com/tech-comparison-lab/loadgen-db`
   - Go 1.23
   - Require: `github.com/jackc/pgx/v5`, `go.mongodb.org/mongo-driver/v2`
3. Write `internal/report/report.go`
   - `Result` and `Summary` structs
   - `Compute(label string, durations []time.Duration, count int) Result`
   - `PrintTable(results []Result)` — tab-aligned stdout table
   - `WriteJSON(summary Summary, path string) error`
4. Write `internal/postgres/bench.go`
   - `New(dsn string)` with pgxpool
   - `Insert`, `Query`, `Agg`, `Update`, `StorageSize`
   - CTE pattern for UPDATE ... LIMIT simulation
5. Write `internal/mongo/bench.go`
   - `New(dsn string)` with mongo.Connect
   - `EnsureIndexes`, `Insert`, `Query`, `Agg`, `Update`, `StorageSize`
   - `InsertMany` with `Ordered: false` for insert
6. Write `main.go`
   - `flag.Parse()` + validation
   - `switch db { case "postgres": ... case "mongo": ... }`
   - `switch op { case "insert": ..., case "all": run all in sequence }`
   - Collect `[]Result`, call `report.PrintTable` + `report.WriteJSON`
7. `go build ./...` — fix any compilation errors
8. `go vet ./...` — fix any issues
9. Write minimal unit test for `report.Compute` (percentile math correctness)

---

## Todo

- [ ] Write `go.mod` with correct module path and dependencies
- [ ] Write `internal/report/report.go` (structs + percentile + output)
- [ ] Write `internal/postgres/bench.go` (all 5 methods)
- [ ] Write `internal/mongo/bench.go` (all 6 methods)
- [ ] Write `main.go` (flag parsing + dispatch)
- [ ] Write `internal/report/report_test.go` (percentile unit test)
- [ ] Verify `go build ./...` succeeds
- [ ] Verify `go vet ./...` passes
- [ ] Smoke test against phase 01 stack: `--op insert --count 1000 --db postgres`

---

## Success Criteria

- `go build -o /tmp/loadgen-db ./...` succeeds with no errors
- `go test ./...` passes (report percentile tests)
- `--op insert --count 1000 --db postgres --dsn "..."` inserts 1000 docs and prints table
- `--op all --count 10000 --db mongo --dsn "..."` runs full sequence, writes `summary.json`
- JSON output validates against expected schema (all required fields present)
- p99 > p95 > p50 > min for all results (sanity check)

---

## Risk Assessment

| Risk | Likelihood | Impact | Mitigation |
|------|-----------|--------|------------|
| pgx CopyFrom vs INSERT VALUES throughput difference | low | medium | Use `CopyFrom` for Postgres — it's the fastest bulk insert path |
| MongoDB `InsertMany` memory for 1000-doc batches | low | low | 1000 * ~500B = 500 kB per batch — well within limits |
| UPDATE LIMIT workaround correctness for Postgres | medium | high | Use CTE pattern; add unit test with small dataset to verify row count |
| Latency slice memory for 10M insert ops | medium | medium | Record per-batch (not per-doc) durations for insert; document this in output |
| mongo-driver/v2 API differences from v1 | medium | medium | Use v2 from the start; check `InsertMany`, `Find`, `Aggregate` signatures carefully |
| Apple Silicon cross-compile for linux/amd64 | low | low | `GOOS=linux GOARCH=amd64 go build` — pure Go, no CGo |

---

## Security Considerations

- DSN passed via CLI flag (visible in `ps` output). For CI use, accept via
  `LOADGEN_DSN` env var as fallback — document but do not hardcode.
- No user-supplied strings are interpolated into SQL queries; all values use
  parameterized queries (`$1`, `$2` for pgx; BSON for mongo).
- Generated document data is synthetic; no PII involved.

---

## Next Steps

After phase 02 is complete:
- Phase 03 `run.sh` calls the binary directly (or via `go run`) for smoke test
  then full run
- Future: expose `--format prometheus` to emit metrics for scraping during run
- Future: add `--seed` flag for reproducible random doc generation
