# Kubernetes vs Docker Swarm

This experiment compares scheduler control-loop latency in **Kubernetes** and **Docker Swarm** using the same container image, resource reservations, replica counts, and running/ready boundary.

## Workloads

| Operation | Measurement boundary |
|---|---|
| Deploy | Submit a one-instance workload -> one instance is running and ready |
| Scale up | Change from 1 to N instances -> all N instances are running and ready |
| Recovery | Kill one running instance -> a replacement instance is running and ready |
| Scale down | Change from N to 1 instance -> exactly one desired instance remains running |

Each operation is repeated independently. Reports include p50, p95, p99, mean, minimum, maximum, and error count.

## Environment

- Kubernetes: a two-node kind cluster (one control plane and one worker).
- Docker Swarm: the current Docker Engine configured as a single-node manager when Swarm is not already active.
- Workload: `registry.k8s.io/pause:3.10`, pre-pulled and loaded into kind.
- Resources: 10 millicpu and 8 MiB reserved per instance on both platforms.
- Benchmark: `benchmarks/loadgen-scheduler`, using Kubernetes and Docker Engine APIs directly.

This is a local scheduler-responsiveness comparison, not a production architecture comparison. A two-node kind cluster and a single-node Swarm manager have different control-plane topology and container-network paths. Results do not measure HA, multi-node placement, overlay-network throughput, persistent storage, rolling updates, or application-level recovery.

Kubernetes readiness is based on Ready pods and observed Deployment state. Swarm has no pod readiness equivalent in this workload, so its boundary is a task in `running` state. Interpret small differences with that semantic limitation in mind.

## Requirements

- Docker Engine with Swarm support
- kind
- kubectl
- Go 1.23 or a prebuilt `benchmarks/loadgen-scheduler/bin/loadgen-scheduler`
- jq

## Run

```bash
# Quick validation: one round, scale to three replicas
./run.sh --smoke-only

# Full run: five rounds, scale to 20 replicas
./run.sh

# Recreate kind and preserve both environments after the run
./run.sh --clean --keep-environment

# Tune the workload
ROUNDS=10 REPLICAS=50 OP_TIMEOUT=5m ./run.sh

# Explicit Docker socket or remote Engine API endpoint
DOCKER_HOST_URI=unix:///var/run/docker.sock ./run.sh --smoke-only
```

Results are written to:

- `results/kubernetes.json`
- `results/swarm.json`
- `results/summary.json` (`results-summary/v1`)

## Lifecycle and safety

If Docker Swarm is inactive, the runner initializes a single-node Swarm and leaves it after the benchmark. An already-active Swarm is reused and never left. The benchmark owns only the service named `scheduler-bench`; do not use that name for unrelated workloads while the experiment runs.

## Interpreting results

Lower latency is better, but compare trends across multiple runs. Recovery measures replacement capacity after a forced container kill; it does not test application state, traffic failover, or data recovery. The first run can still include runtime initialization even though the image is preloaded.
