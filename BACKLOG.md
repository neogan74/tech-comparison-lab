# Tech Comparison Lab — Backlog

## Completed ✅

### Smoke-Tested Experiments (All CI Integrated)
- PostgreSQL vs MongoDB
- MongoDB vs Cassandra
- gRPC vs REST
- Redis vs Valkey
- ClickHouse vs PostgreSQL
- Kafka vs RabbitMQ
- Apache Kafka vs Apache Pulsar
- NATS vs Apache Kafka
- RabbitMQ vs NATS
- Kubernetes vs Nomad
- Kubernetes vs Docker Swarm
- Kubernetes vs OpenShift (kind)
- Redis vs Memcached
- Redis vs Dragonfly
- etcd vs Consul
- Argo CD vs Flux CD (kind)
- Prometheus vs VictoriaMetrics
- Zabbix vs Prometheus
- Istio vs Linkerd (kind)
- Kong vs Traefik
- Traefik vs NGINX
- Jaeger vs Zipkin
- Loki vs Elasticsearch

All experiments have CI validation, smoke tests, and follow the established pattern.

### Results Collection Framework ✅
- `Makefile` with experiment management commands
- `scripts/collect-results.sh` for repo-level aggregation
- Schema documentation: `results-summary-v1.md`, `results-index-v1.md`
- `docs/index.json` generation

---

## Priority 1: New Experiments

### 1. New Experiments (Priority from README)

The following experiments are planned but not yet implemented. Choose from this list when starting new work:

**Databases:**
- ~~PostgreSQL vs CockroachDB — distributed SQL comparison~~ ✅
- ~~MySQL vs PostgreSQL — OLTP workloads~~ ✅
- ~~MongoDB vs Cassandra — distributed NoSQL~~ ✅

**Cache / KV:**
- ~~Redis vs Memcached — cache performance~~ ✅
- ~~etcd vs Consul — KV store consensus~~ ✅
- ~~Redis vs Dragonfly — Redis alternative~~ ✅

**Messaging / Streaming:**
- ~~Apache Kafka vs Apache Pulsar — streaming comparison~~ ✅

**Kubernetes / Platform:**
- ~~Kubernetes vs Nomad — scheduler comparison~~ ✅
- ~~Kubernetes vs Docker Swarm — orchestration~~ ✅
- ~~Argo CD vs Flux CD — GitOps comparison~~ ✅
- ~~Istio vs Linkerd — service mesh~~ ✅
- ~~Traefik vs NGINX — reverse proxy~~ ✅

**API / Networking:**
- ~~GraphQL vs REST — API query models~~ ✅
- ~~Kong vs Traefik — API gateway~~ ✅

**Observability:**
- ~~Prometheus vs VictoriaMetrics — metrics storage~~ ✅
- ~~Loki vs Elasticsearch — log storage~~ ✅
- ~~Jaeger vs Zipkin — distributed tracing~~ ✅
- ~~Zabbix vs Prometheus — monitoring systems~~ ✅

**Definition of Done for New Experiments:**
- [ ] Create benchmark in `benchmarks/loadgen-*/`
- [ ] Create experiment in `experiments/category/name/` with `run.sh`
- [ ] Create validation script in `scripts/validate-*.sh`
- [ ] Create smoke test in `scripts/smoke-*.sh`
- [ ] Add to Makefile `EXPERIMENTS` list
- [ ] Add jobs to `.github/workflows/ci.yaml`
- [ ] Update `README.md` status table
- [ ] README with schema follows `results-summary/v1.md`

---

## Priority 2: Infrastructure & Tooling

### 2. Experiment Template & Helper Scripts

**Goal:** Simplify adding new experiments with templates and automation

**Tasks:**
- [ ] Create `experiments/template/` directory
  - Template `run.sh` with common patterns (build_binary, check_deps, wait_for_*)
  - Template `README.md` structure
  - Template validation and smoke scripts
- [ ] Create `scripts/create-experiment.sh` helper script
  - Interactive prompts for experiment details (name, category, technologies)
  - Creates directory structure
  - Generates boilerplate files (run.sh, README.md, validate/smoke scripts)
  - Updates Makefile and CI workflow
- [ ] Update `AGENTS.md` with experiment creation workflow
- [ ] Create `CONTRIBUTING.md` if needed

**Acceptance Criteria:**
- Template files follow patterns from existing experiments
- Helper script works for all experiment types (db, cache, messaging, etc.)
- Documentation clear on how to add new experiments
- New experiments created with helper pass validation immediately

**Definition of Done:**
- [ ] Template validated by creating test experiment
- [ ] Helper script tested and documented
- [ ] AGENTS.md updated with experiment creation section
- [ ] Existing experiments still pass after template changes

---

### 3. Results Visualization & Comparison

**Goal:** Create automated results comparison and visualization tools

**Tasks:**
- [ ] Create `scripts/compare-results.sh`
  - Compare multiple runs of same experiment
  - Generate trend charts (if data available)
  - Export to CSV/JSON for external analysis
  - Support baseline comparison vs current run
- [ ] Enhance Grafana dashboards
  - Review existing dashboards in `deployments/docker-compose/*/grafana/`
  - Create cross-experiment comparison dashboard
  - Add trend panels for historical data
  - Export dashboards to JSON for version control
- [ ] Document result interpretation patterns
  - Add `docs/result-interpretation.md` with guidance on reading metrics
  - Examples of what each metric means in context
  - Common patterns to look for

**Acceptance Criteria:**
- Comparison script works with all result JSON formats
- Generates human-readable comparison output (tables + basic charts)
- Trend analysis across multiple runs works
- Grafana dashboards import correctly and display meaningful data
- Documentation helps users understand results

**Definition of Done:**
- [ ] Script tested with all existing result files
- [ ] Grafana dashboards exported to version control
- [ ] `docs/result-interpretation.md` created and comprehensive
- [ ] Examples in `examples/` directory showing comparison usage

---

### 4. Infrastructure Improvements

**Goal:** Improve Docker Compose setup, resource management, and CI stability

**Tasks:**
- [ ] Add resource limits to Docker Compose services
  - CPU/memory limits for all database/cache/messaging services
  - Prevent CI resource exhaustion (especially Kafka, ClickHouse)
  - Document limits in service README files
- [ ] Create Docker Compose profiles for different environments
  - `ci` profile: minimal resources, fast startup
  - `local` profile: full resources, performance-optimized
  - Document profile usage in AGENTS.md
- [ ] Improve health checks and startup detection
  - Review all healthcheck commands for optimality
  - Reduce retry counts where services are consistently fast
  - Better error messages when services fail to start
  - Add health check status logging to experiment runners
- [ ] Add `.env.example` files for each compose stack
  - `deployments/docker-compose/analytics/.env.example`
  - `deployments/docker-compose/cache/.env.example`
  - `deployments/docker-compose/messaging/.env.example`
  - Document all overridable variables

**Acceptance Criteria:**
- Resource limits prevent CI OOM and improve reliability
- Health checks fail fast when services misconfigured
- Profiles work and are documented
- Environment variables override defaults correctly
- All experiments pass with both profiles

**Definition of Done:**
- [ ] All Docker Compose files updated with resource limits
- [ ] Profiles tested locally and in CI (update CI to use `ci` profile)
- [ ] Health checks optimized (reduce timeouts where possible)
- [ ] `.env.example` files created for all stacks
- [ ] Documentation updated (AGENTS.md, experiment READMEs)
- [ ] All existing experiments still pass

---

## Priority 3: Documentation & Quality

### 5. Documentation Improvements

**Goal:** Improve repository documentation for new contributors

**Tasks:**
- [ ] Create `CONTRIBUTING.md`
  - How to run experiments locally
  - How to add new experiments
  - Code style conventions
  - Testing guidelines
- [ ] Expand `AGENTS.md` with common gotchas
  - Docker Compose v2 vs v1 differences
  - Common CI failure patterns and solutions
  - Platform-specific issues (Mac vs Linux)
- [ ] Create `EXPERIMENTS.md` with detailed descriptions
  - Move content from `experiments.md`
  - Add links to each experiment's README
  - Include status matrix
- [ ] Add troubleshooting guide
  - Common Docker issues
  - Database connection problems
  - CI failure debugging

**Acceptance Criteria:**
- New contributors can add experiments without asking questions
- Existing contributors can troubleshoot common issues independently
- All important information is in docs, not just in heads

**Definition of Done:**
- [ ] `CONTRIBUTING.md` created and comprehensive
- [ ] `AGENTS.md` expanded with troubleshooting section
- [ ] `EXPERIMENTS.md` created/updated with all experiments
- [ ] `docs/troubleshooting.md` created with common issues
- [ ] All docs reviewed by at least one other person

---

### 6. Test Coverage Improvements

**Goal:** Improve test coverage for benchmark tools

**Tasks:**
- [ ] Add integration tests for benchmark tools
  - Test against real Docker services where feasible
  - Mock external services where real ones are too heavy
  - Test error paths (connection failures, invalid data)
- [ ] Add unit tests for experiment runners
  - Test shell functions (build_binary, wait_for_*, etc.)
  - Mock Docker Compose commands for testing
  - Test result generation and validation
- [ ] Measure and report test coverage
  - Add `go test -cover` to CI
  - Set minimum coverage targets
  - Track coverage over time

**Acceptance Criteria:**
- All critical code paths have tests
- Integration tests cover real usage patterns
- Test coverage meets minimum threshold (e.g., 70%)
- Coverage reports available in CI

**Definition of Done:**
- [ ] Integration tests added for all benchmark tools
- [ ] Unit tests added for experiment runner utility functions
- [ ] Coverage reporting configured in CI
- [ ] Minimum coverage thresholds enforced
- [ ] All tests pass consistently in CI

---

## Future Ideas (Unprioritized)

These are potential enhancements to consider, but not yet prioritized:

### Experiment Enhancements
- Multi-region benchmarks for distributed systems (Cassandra, CockroachDB)
- Failure injection and chaos testing
- Long-running durability tests (24h+ stability)
- Real-world workload traces vs synthetic data
- Cost analysis (cloud pricing per workload)

### Tooling Enhancements
- Web UI for experiment configuration and results viewing
- Automated result archiving (S3, GCS) for historical data
- Alerting when performance degrades significantly
- Slack/Email integration for CI failures
- Performance regression detection in CI

### Observability
- Structured logging with correlation IDs
- Distributed tracing for multi-service experiments
- Resource usage monitoring during benchmarks
- Network I/O profiling
- Storage I/O profiling (especially for databases)

### Infrastructure
- Kubernetes-based benchmark runners for scale tests
- Spot instances for cost-effective cloud benchmarks
- Multi-cloud comparison (AWS vs GCP vs Azure)
- ARM vs x86 performance comparisons

---

## How to Prioritize

When choosing work from this backlog:

1. **Start with Priority 1** if you want to add new experiments
2. **Move to Priority 2** if infrastructure/tooling is blocking experiments
3. **Pick from Priority 3** for general quality improvements
4. **Browse Future Ideas** when looking for innovation opportunities

When adding items to backlog, include:
- Clear goal statement
- Task breakdown
- Acceptance criteria
- Definition of Done

---

## Current Status Summary

**Completed:**
- ✅ 6 experiments fully smoke-tested with CI
- ✅ Results collection framework implemented
- ✅ Makefile with experiment management
- ✅ Schema documentation (v1)

**In Progress:**
- None (all planned CI integration complete)

**Next Steps:**
- Choose new experiment from Priority 1 list
- Improve tooling from Priority 2 if needed
- Enhance documentation from Priority 3 to help new contributors
