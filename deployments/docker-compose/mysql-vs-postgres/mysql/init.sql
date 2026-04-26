-- Idempotent schema initialization for MySQL benchmark (MySQL 8.0+)
-- Run automatically on first container start via /docker-entrypoint-initdb.d/

CREATE TABLE IF NOT EXISTS orders (
    id         VARCHAR(36)  NOT NULL,
    doc        JSON         NOT NULL,
    created_at DATETIME(6)  NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    PRIMARY KEY (id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Functional index on user.country: used by query + update benchmarks
-- Requires MySQL 8.0.13+
CREATE INDEX IF NOT EXISTS idx_orders_country
    ON orders ((doc->>'$.user.country'));

-- Functional index on status
CREATE INDEX IF NOT EXISTS idx_orders_status
    ON orders ((doc->>'$.status'));