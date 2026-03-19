# Phase 01 — Infrastructure (Docker Compose Stack)

**Parent plan:** [plan.md](./plan.md)
**Dependencies:** none
**Status:** pending
**Priority:** high (blocks phases 02, 03)
**Date:** 2026-03-16

---

## Overview

Stand up a reproducible local stack containing PostgreSQL 16, MongoDB 7,
Prometheus, and Grafana via Docker Compose. Postgres schema and indexes are
initialized automatically via an `init.sql` mount. Health checks ensure
downstream scripts can reliably wait for readiness.

---

## Key Insights

- PostgreSQL `JSONB` stores binary-parsed JSON; GIN index accelerates `@>`,
  `?`, and expression operators. Expression indexes on extracted scalar fields
  (`doc->>'status'`) enable index-only scans for equality filters — critical
  for fair comparison vs MongoDB's field indexes.
- MongoDB 7 default WiredTiger storage engine compresses data on disk;
  PostgreSQL TOAST compresses JSONB above 2 kB. Both behaviors affect the
  "storage size" metric — must query engine-level sizes, not filesystem du.
- Prometheus scrape targets for both engines require exporters
  (`postgres_exporter`, `mongodb_exporter`). Including them in compose avoids
  needing a separate metrics phase.
- Grafana provisioning via `provisioning/` directory eliminates manual
  dashboard import — datasource and dashboard JSON are mounted at startup.
- Named Docker volumes (not bind mounts) for DB data dirs improve I/O
  performance on macOS due to avoiding osxfs overhead.

---

## Requirements

### Functional
- `docker compose up -d` starts all services; all pass health checks within 60s
- PostgreSQL `orders` table + all indexes created automatically on first start
- Both databases reachable on deterministic host ports (PG: 5432, Mongo: 27017)
- Prometheus scrapes both exporters; Grafana pre-provisioned with datasource
- `docker compose down -v` tears down fully including named volumes

### Non-Functional
- `init.sql` is idempotent (`CREATE TABLE IF NOT EXISTS`, `CREATE INDEX IF NOT EXISTS`)
- No hardcoded passwords in tracked files; use `.env` with documented defaults
- Compose file uses `depends_on: condition: service_healthy` to enforce order

---

## Architecture

```
docker-compose.yml
├── postgres:16
│   ├── port 5432:5432
│   ├── volume: pg_data (named)
│   ├── healthcheck: pg_isready
│   └── mount: ./postgres/init.sql → /docker-entrypoint-initdb.d/init.sql
│
├── mongo:7
│   ├── port 27017:27017
│   ├── volume: mongo_data (named)
│   └── healthcheck: mongosh --eval "db.adminCommand('ping')"
│
├── postgres-exporter
│   ├── image: prometheuscommunity/postgres-exporter
│   └── port 9187 (internal only)
│
├── mongodb-exporter
│   ├── image: percona/mongodb_exporter:2.x
│   └── port 9216 (internal only)
│
├── prometheus
│   ├── port 9090:9090
│   ├── mount: ./prometheus.yml
│   └── depends_on: postgres-exporter, mongodb-exporter
│
└── grafana
    ├── port 3000:3000
    ├── mount: ./grafana/provisioning/
    └── depends_on: prometheus
```

### PostgreSQL Schema (`init.sql`)

```sql
CREATE TABLE IF NOT EXISTS orders (
    id         UUID        PRIMARY KEY,
    doc        JSONB       NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_orders_doc_gin
    ON orders USING GIN (doc);

CREATE INDEX IF NOT EXISTS idx_orders_doc_status
    ON orders ((doc->>'status'));

CREATE INDEX IF NOT EXISTS idx_orders_doc_country
    ON orders ((doc->'user'->>'country'));
```

Rationale:
- GIN covers `@>` containment queries and `jsonb_path_query`
- Expression indexes on `status` and `country` enable seq-scan avoidance for
  the filter-by-country query and the update-by-status operation
- `created_at` column separate from JSONB for efficient range queries without
  JSON extraction overhead

### MongoDB Index Strategy

Indexes created programmatically by `loadgen-db` at startup (not in compose),
but documented here for completeness:

```
db.orders.createIndex({ "user.country": 1 })
db.orders.createIndex({ "user.id": 1 })
db.orders.createIndex({ "status": 1, "created_at": -1 })
```

---

## Related Code Files

- `deployments/docker-compose/docker-compose.yml` — primary artifact
- `deployments/docker-compose/postgres/init.sql` — schema + indexes
- `deployments/docker-compose/prometheus.yml` — scrape config
- `deployments/docker-compose/grafana/provisioning/datasources/prometheus.yml`
- `deployments/docker-compose/.env.example` — documented env vars

---

## Implementation Steps

1. Create `deployments/docker-compose/` directory tree
2. Write `docker-compose.yml`
   - Define services: postgres, mongo, postgres-exporter, mongodb-exporter,
     prometheus, grafana
   - Configure health checks for postgres and mongo (required by phase 03
     wait logic)
   - Use named volumes `pg_data`, `mongo_data`
   - Source `.env` for passwords (`POSTGRES_PASSWORD`, `MONGO_INITDB_ROOT_PASSWORD`)
3. Write `postgres/init.sql`
   - `CREATE TABLE IF NOT EXISTS orders (...)` with UUID PK, JSONB, TIMESTAMPTZ
   - GIN index on `doc`
   - Expression indexes on `status` and `country`
4. Write `prometheus.yml`
   - Scrape interval: 15s
   - Targets: postgres-exporter:9187, mongodb-exporter:9216
5. Write `grafana/provisioning/datasources/prometheus.yml`
   - Datasource type prometheus, url http://prometheus:9090, access proxy
6. Write `.env.example` with safe default values and comments
7. Smoke-test compose: `docker compose config --quiet` (validates YAML)
8. Verify health check commands work against real containers (manual step)

---

## Todo

- [ ] Write `docker-compose.yml` with all 6 services
- [ ] Write `postgres/init.sql` (idempotent schema + indexes)
- [ ] Write `prometheus.yml` scrape config
- [ ] Write Grafana datasource provisioning file
- [ ] Write `.env.example`
- [ ] Verify `docker compose config` passes without errors
- [ ] Document port map and credentials in phase README

---

## Success Criteria

- `docker compose up -d && docker compose ps` shows all services healthy
- `psql -h localhost -U bench -d bench -c '\d orders'` shows table + indexes
- `mongosh --eval "db.orders.stats()"` returns without error
- Prometheus at `http://localhost:9090/targets` shows both exporters UP
- Grafana at `http://localhost:3000` loads with Prometheus datasource pre-configured

---

## Risk Assessment

| Risk | Likelihood | Impact | Mitigation |
|------|-----------|--------|------------|
| Port conflicts on developer machine | medium | low | Document override via `.env`; use non-standard ports as defaults if needed |
| MongoDB exporter image compatibility with Mongo 7 | low | medium | Pin image version; test against mongo:7.0 specifically |
| macOS volume performance for 10M inserts | medium | medium | Use named volumes (not bind mounts); document `--platform linux/amd64` if on Apple Silicon |
| init.sql not executed on existing volume | low | high | Document `docker compose down -v` requirement before fresh benchmark run |

---

## Security Considerations

- Benchmark stack is local-only; no exposed ports to public network required
- Passwords in `.env` (gitignored); `.env.example` committed with placeholders
- No TLS configured — acceptable for local benchmark, not for production use
- MongoDB exporter connects with read-only user; document creation in init script

---

## Next Steps

After phase 01 is complete:
- Phase 02 can be developed and tested against the running stack
- Phase 03 `run.sh` uses `docker compose up -d` as its first step
- Future: add Grafana dashboard JSON for benchmark metrics visualization
