-- Idempotent schema initialization for CockroachDB benchmark
-- Executed via: cockroach sql --insecure --database bench --file=/init.sql

CREATE TABLE IF NOT EXISTS orders (
    id         TEXT        PRIMARY KEY,
    doc        JSONB       NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Inverted (GIN) index: accelerates JSON containment operators
CREATE INVERTED INDEX IF NOT EXISTS idx_orders_doc_gin ON orders (doc);

-- Expression index on status
CREATE INDEX IF NOT EXISTS idx_orders_doc_status
    ON orders ((doc->>'status'));

-- Expression index on user.country: used by query + update benchmarks
CREATE INDEX IF NOT EXISTS idx_orders_doc_country
    ON orders ((doc->'user'->>'country'));