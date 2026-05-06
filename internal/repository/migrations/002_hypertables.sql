-- 002_hypertables.sql
CREATE TABLE events (
    id           BIGSERIAL,
    site_id      UUID NOT NULL,
    type         TEXT NOT NULL DEFAULT 'pageview',
    url          TEXT NOT NULL DEFAULT '',
    referrer     TEXT NOT NULL DEFAULT '',
    channel      TEXT NOT NULL DEFAULT 'direct',
    utm_source   TEXT NOT NULL DEFAULT '',
    utm_medium   TEXT NOT NULL DEFAULT '',
    utm_campaign TEXT NOT NULL DEFAULT '',
    country      CHAR(2) NOT NULL DEFAULT '',
    city         TEXT NOT NULL DEFAULT '',
    device_type  TEXT NOT NULL DEFAULT '',
    browser      TEXT NOT NULL DEFAULT '',
    os           TEXT NOT NULL DEFAULT '',
    language     TEXT NOT NULL DEFAULT '',
    session_id   TEXT NOT NULL DEFAULT '',
    visitor_id   TEXT NOT NULL DEFAULT '',
    is_bounce    BOOLEAN NOT NULL DEFAULT FALSE,
    duration_ms  INTEGER NOT NULL DEFAULT 0,
    props        JSONB,
    timestamp    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (id, timestamp)
);

SELECT create_hypertable('events', 'timestamp', chunk_time_interval => INTERVAL '1 day');

CREATE INDEX ON events (site_id, timestamp DESC);
