-- Idempotent schema initialization for PostgreSQL benchmark
-- Run automatically on first container start via /docker-entrypoint-initdb.d/

CREATE TABLE IF NOT EXISTS orders (
    id         TEXT        PRIMARY KEY,
    doc        JSONB       NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- GIN index: accelerates @>, ?, jsonb_path_query operators
CREATE INDEX IF NOT EXISTS idx_orders_doc_gin
    ON orders USING GIN (doc);

-- Expression index on status: enables index scan for equality filter
CREATE INDEX IF NOT EXISTS idx_orders_doc_status
    ON orders ((doc->>'status'));

-- Expression index on user.country: used by query + update benchmarks
CREATE INDEX IF NOT EXISTS idx_orders_doc_country
    ON orders ((doc->'user'->>'country'));
