# Phase 05 — CI + Makefile + README

**Parent plan:** [plan.md](plan.md)  
**Dependencies:** Phase 04  
**Date:** 2026-07-01  
**Status:** 🔲 Not started

## Key Insights

- Makefile: add `etcd-vs-consul` to the `EXPERIMENTS :=` list (alphabetical or at end)
- CI: two new jobs — `validate-etcd-vs-consul` and `smoke-etcd-vs-consul` (needs validate)
- CI `go-version-file` points to `benchmarks/loadgen-kv/go.mod` (new module)
- README: add row to existing status table

## Makefile Change

```makefile
EXPERIMENTS := \
    ...existing entries... \
    etcd-vs-consul          # ← add here
```

## CI Jobs (append to .github/workflows/ci.yaml)

```yaml
  validate-etcd-vs-consul:
    name: Validate etcd vs Consul experiment
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version-file: benchmarks/loadgen-kv/go.mod
      - run: docker compose version
      - run: bash ./scripts/validate-etcd-vs-consul.sh

  smoke-etcd-vs-consul:
    name: Smoke etcd vs Consul experiment
    runs-on: ubuntu-latest
    needs: validate-etcd-vs-consul
    timeout-minutes: 15
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version-file: benchmarks/loadgen-kv/go.mod
      - run: docker compose version
      - name: Run smoke benchmark
        env:
          SMOKE_COUNT: 100
        run: bash ./scripts/smoke-etcd-vs-consul.sh
```

## README.md Change

Find the status table and add:

```markdown
| etcd vs Consul | KV / Consensus | KV throughput, watch latency, leader election | ✅ |
```

## Implementation Steps

1. Edit `Makefile`: add `etcd-vs-consul` to EXPERIMENTS list
2. Edit `.github/workflows/ci.yaml`: append two job blocks after last existing experiment
3. Edit `README.md`: add row to status table
4. Run `make list` to verify new entry appears
5. Run `make validate-etcd-vs-consul` locally to verify Makefile wiring

## Todo

- [ ] Update Makefile EXPERIMENTS list
- [ ] Add CI jobs to ci.yaml
- [ ] Update README status table
- [ ] Verify `make list` shows etcd-vs-consul
- [ ] Verify `make validate-etcd-vs-consul` runs the script

## Success Criteria

- `make list` includes `etcd-vs-consul`
- `make validate-etcd-vs-consul` runs `scripts/validate-etcd-vs-consul.sh`
- `make smoke-etcd-vs-consul` runs `scripts/smoke-etcd-vs-consul.sh`
- CI yaml is valid YAML (`python3 -c "import yaml; yaml.safe_load(open('.github/workflows/ci.yaml'))"`)
- README shows experiment in table with ✅ status
