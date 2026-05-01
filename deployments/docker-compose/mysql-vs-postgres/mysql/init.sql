-- Idempotent schema initialization for MySQL benchmark (MySQL 8.0+)
-- Run automatically on first container start via /docker-entrypoint-initdb.d/

-- Virtual generated columns allow indexing JSON paths without LONGTEXT key-length errors.
CREATE TABLE IF NOT EXISTS orders (
    id           VARCHAR(36)  NOT NULL,
    doc          JSON         NOT NULL,
    created_at   DATETIME(6)  NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    user_country VARCHAR(100) GENERATED ALWAYS AS (JSON_UNQUOTE(JSON_EXTRACT(doc, '$.user.country'))) VIRTUAL,
    doc_status   VARCHAR(50)  GENERATED ALWAYS AS (JSON_UNQUOTE(JSON_EXTRACT(doc, '$.status')))        VIRTUAL,
    PRIMARY KEY (id),
    INDEX idx_orders_country (user_country),
    INDEX idx_orders_status  (doc_status)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;