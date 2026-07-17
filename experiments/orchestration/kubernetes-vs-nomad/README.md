# Kubernetes vs Nomad

This experiment compares the scheduler control loops of **Kubernetes** and **Nomad** using the same resource request, replica counts, and running/ready condition.

## Workloads

| Operation | Measurement boundary |
|---|---|
| Deploy | Submit a one-instance workload → instance is running and ready |
| Scale up | Change from 1 to N instances → all N are running and ready |
| Recovery | Force-stop one instance → a replacement instance is running and ready |
| Scale down | Change from N to 1 instance → exactly one desired instance remains running |

Each operation is repeated independently. The report includes p50, p95, p99, mean, minimum, maximum, and error count.

## Environment

- Kubernetes: a two-node kind cluster (one control plane and one worker).
- Nomad: a single-node native `nomad agent -dev` client/server.
- Workload: a preloaded `pause:3.10` container on Kubernetes and a minimal `raw_exec` sleep task on Nomad.
- Benchmark: `benchmarks/loadgen-scheduler`, using Kubernetes APIs and the Nomad HTTP API directly.

This is a local scheduler responsiveness comparison, not a production architecture comparison. A kind control plane and a Nomad dev agent differ in isolation, topology, and task driver startup; the results do not measure HA behavior, network storage, service discovery, or multi-node bin-packing.

## Requirements

- Docker
- kind
- kubectl
- Nomad
- Go 1.23 or a prebuilt `benchmarks/loadgen-scheduler/bin/loadgen-scheduler`
- curl and jq

## Run

```bash
# Quick validation: one round, scale to three replicas
./run.sh --smoke-only

# Full run: five rounds, scale to 20 replicas
./run.sh

# Recreate kind and keep both environments after the run
./run.sh --clean --keep-environment

# Tune the workload
ROUNDS=10 REPLICAS=50 OP_TIMEOUT=5m ./run.sh

# Reuse an existing Nomad agent
NOMAD_ADDRESS=http://127.0.0.1:4646 ./run.sh

# Linux hosts commonly require root for Nomad cgroup management
NOMAD_USE_SUDO=1 ./run.sh --smoke-only
```

Results are written to:

- `results/kubernetes.json`
- `results/nomad.json`
- `results/summary.json` (`results-summary/v1`)
- `results/nomad.log` when the runner starts the dev agent

## Interpreting results

Lower latency is better, but compare trends across multiple runs rather than a single number. The first run can still include runtime initialization even though the container image is preloaded. Recovery measures the time until replacement capacity is available; it does not test application-level traffic or state recovery.
