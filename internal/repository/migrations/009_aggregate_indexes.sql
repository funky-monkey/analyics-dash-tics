-- 009_aggregate_indexes.sql
-- Explicit indexes on continuous aggregate materialized views.
-- TimescaleDB does not automatically create compound indexes on cagg views,
-- so range scans on (site_id, time_bucket) are sequential without these.

CREATE INDEX IF NOT EXISTS stats_hourly_site_hour
    ON stats_hourly (site_id, hour DESC);

CREATE INDEX IF NOT EXISTS page_stats_daily_site_day
    ON page_stats_daily (site_id, day DESC);

CREATE INDEX IF NOT EXISTS source_stats_daily_site_day
    ON source_stats_daily (site_id, day DESC);

-- Also index funnel_steps by funnel for the drop-off CTE.
CREATE INDEX IF NOT EXISTS funnel_steps_funnel_id
    ON funnel_steps (funnel_id, position);
