-- 010_visitor_first_seen.sql
-- Tracks the first event timestamp per visitor per site.
-- Used to classify sessions as "new" (first visit in period) vs "returning".
CREATE TABLE IF NOT EXISTS visitor_first_seen (
    site_id    UUID        NOT NULL,
    visitor_id TEXT        NOT NULL,
    first_seen TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (site_id, visitor_id)
);

CREATE INDEX IF NOT EXISTS visitor_first_seen_site_first
    ON visitor_first_seen (site_id, first_seen DESC);
