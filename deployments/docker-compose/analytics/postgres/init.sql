-- Analytics events table for PostgreSQL benchmark
CREATE TABLE IF NOT EXISTS events (
    id       TEXT         NOT NULL,
    user_id  INTEGER      NOT NULL,
    event    VARCHAR(32)  NOT NULL,
    ts       TIMESTAMPTZ  NOT NULL,
    country  CHAR(2)      NOT NULL,
    value    REAL         NOT NULL DEFAULT 0,
    session  TEXT         NOT NULL
);

-- Indexes chosen to give PostgreSQL the best possible chance in each query type
CREATE INDEX IF NOT EXISTS idx_events_user    ON events(user_id);
CREATE INDEX IF NOT EXISTS idx_events_ts      ON events(ts);
CREATE INDEX IF NOT EXISTS idx_events_event   ON events(event);
CREATE INDEX IF NOT EXISTS idx_events_ev_ts   ON events(event, ts);
