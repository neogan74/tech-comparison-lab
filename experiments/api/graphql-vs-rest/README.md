# GraphQL vs REST

Measures latency and throughput of **GraphQL** (POST /graphql) versus **REST** (GET/POST HTTP) for the same three operations: echo, get-user, and create-order.

Both protocols are served by the same `bench-api` Go process on port 8080, ensuring identical business logic and I/O paths. The only variable is the API query model.

## Operations

| Operation     | REST                          | GraphQL                                             |
|---------------|-------------------------------|-----------------------------------------------------|
| `echo`        | `GET /echo?msg=hello`         | `query { echo(msg: "hello") { msg } }`              |
| `get-user`    | `GET /users/{id}`             | `query { getUser(id: $id) { id name email ... } }`  |
| `create-order`| `POST /orders` (JSON body)    | `mutation { createOrder(...) { id status total } }` |

## Fairness Notes

- Both protocols share the same in-process handler functions (`handleEcho`, `handleGetUser`, `handleCreateOrder`).
- REST uses dedicated URL-routed endpoints; GraphQL uses a single `POST /graphql` with query parsing.
- GraphQL overhead includes query parsing, schema validation, and field resolution — this is intentional and representative.
- Both clients use the same tuned HTTP transport (connection pooling, keep-alives).
- GraphQL uses variables (not inline literals) to prevent query string caching effects.

## Prerequisites

- Go 1.23+
- Docker + Docker Compose v2
- `jq`, `curl`

## Running

```bash
# Smoke test only (fast, ~1000 requests)
./run.sh --smoke-only

# Full benchmark (100k requests, 50 workers)
./run.sh

# Custom scale
COUNT=500000 WORKERS=100 ./run.sh

# Skip rebuild (use existing binaries)
SKIP_BUILD=1 ./run.sh

# Keep Prometheus/Grafana running after experiment
./run.sh --keep-stack
```

## Results

Results are written to `results/`:

| File               | Contents                                      |
|--------------------|-----------------------------------------------|
| `rest.json`        | REST latency/throughput per operation         |
| `graphql.json`     | GraphQL latency/throughput per operation      |
| `summary.json`     | Combined summary (schema `results-summary/v1`)|
| `smoke-rest.json`  | Smoke run REST results                        |
| `smoke-graphql.json` | Smoke run GraphQL results                   |

## Observability

After a full run (with `--keep-stack` or while the stack is still up):

- Prometheus: http://localhost:9096
- Grafana: http://localhost:3004

## Expected Results

GraphQL adds per-request overhead for query parsing and schema validation:

- `echo`: GraphQL ~20–50% slower (parsing overhead dominates on trivial workloads)
- `get-user`: GraphQL overhead partially amortized by field selection expressiveness
- `create-order`: Mutation overhead similar to query overhead; REST slightly faster

REST wins on raw RPS for simple operations. GraphQL's value is in flexible field selection and single-endpoint access patterns, not raw throughput.