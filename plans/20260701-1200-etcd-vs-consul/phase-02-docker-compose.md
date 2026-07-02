# Phase 02 — Docker Compose Stack (`kv`)

**Parent plan:** [plan.md](plan.md)  
**Dependencies:** none (parallel with Phase 01)  
**Date:** 2026-07-01  
**Status:** 🔲 Not started

## Key Insights

- Single-node etcd and single-node Consul (CI can't afford 3-node clusters)
- etcd official image: `quay.io/coreos/etcd:v3.5` or `bitnami/etcd:3.5`; use `bitnami/etcd:3.5` (simpler env-var config)
- Consul image: `hashicorp/consul:1.18` with `-dev` flag (in-memory, no persistence)
- Ports: etcd 2379 (client), Consul 8500 (HTTP)
- Add Prometheus exporters: `etcd` exposes `/metrics` natively; consul uses `hashicorp/consul-exporter`
- Grafana optional (local only), provisioning dir mirrors cache stack pattern

## Architecture

```
deployments/docker-compose/kv/
├── docker-compose.yml
├── prometheus.yml
└── grafana/
    └── provisioning/
        ├── dashboards/
        │   └── dashboard.json
        └── datasources/
            └── prometheus.yml
```

## docker-compose.yml Services

| Service | Image | Port (host) | Purpose |
|---------|-------|-------------|---------|
| `etcd` | `bitnami/etcd:3.5` | 2379 | etcd client endpoint |
| `consul` | `hashicorp/consul:1.18` | 8500 | Consul HTTP API |
| `consul-exporter` | `hashicorp/consul-exporter:latest` | 9107 | Consul → Prometheus |
| `prometheus` | `prom/prometheus:latest` | 9095 | metrics scrape |
| `grafana` | `grafana/grafana:latest` | 3005 | dashboards |

**Port choices** avoid collisions with existing stacks (cache=9091/3001, analytics=9090/3000, etc.).

## etcd config (env vars for bitnami image)

```yaml
environment:
  ALLOW_NONE_AUTHENTICATION: "yes"
  ETCD_ADVERTISE_CLIENT_URLS: http://etcd:2379
  ETCD_LISTEN_CLIENT_URLS: http://0.0.0.0:2379
```

## Consul config

```yaml
command: agent -dev -bind=0.0.0.0 -client=0.0.0.0 -ui
```

## Health Checks

```yaml
# etcd
healthcheck:
  test: ["CMD", "etcdctl", "--endpoints=http://localhost:2379", "endpoint", "health"]
  interval: 3s
  timeout: 5s
  retries: 15

# consul
healthcheck:
  test: ["CMD", "curl", "-f", "http://localhost:8500/v1/status/leader"]
  interval: 3s
  timeout: 5s
  retries: 15
```

## Implementation Steps

1. Create `deployments/docker-compose/kv/` directory
2. Write `docker-compose.yml` with all 5 services
3. Write `prometheus.yml` scraping etcd `:2379/metrics` and consul-exporter `:9107/metrics`
4. Copy grafana provisioning structure from `deployments/docker-compose/cache/grafana/` and adapt datasource URLs
5. Test locally: `docker compose -f deployments/docker-compose/kv/docker-compose.yml up -d`
6. Verify etcd health: `etcdctl --endpoints=http://localhost:2379 endpoint health`
7. Verify consul health: `curl http://localhost:8500/v1/status/leader`

## Todo

- [ ] Create directory structure
- [ ] Write docker-compose.yml
- [ ] Write prometheus.yml
- [ ] Add grafana provisioning
- [ ] Test locally (`docker compose up`)
- [ ] Verify health checks pass

## Success Criteria

- `docker compose config --quiet` succeeds (no YAML errors)
- Both services healthy after `docker compose up -d`
- etcd responds to `etcdctl endpoint health`
- Consul responds to `curl /v1/status/leader` with a non-empty string
- Prometheus scrapes both targets (Status → Targets page shows UP)
