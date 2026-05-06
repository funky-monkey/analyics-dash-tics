-- 003_continuous_aggregates.sql

CREATE MATERIALIZED VIEW stats_hourly
WITH (timescaledb.continuous) AS
SELECT
    site_id,
    time_bucket('1 hour', timestamp) AS hour,
    COUNT(*) FILTER (WHERE type = 'pageview') AS pageviews,
    COUNT(DISTINCT session_id) FILTER (WHERE type = 'pageview') AS sessions,
    COUNT(DISTINCT visitor_id) FILTER (WHERE type = 'pageview') AS visitors,
    COUNT(*) FILTER (WHERE is_bounce = TRUE) AS bounces,
    SUM(duration_ms) AS total_duration_ms
FROM events
GROUP BY site_id, time_bucket('1 hour', timestamp)
WITH NO DATA;

SELECT add_continuous_aggregate_policy('stats_hourly',
    start_offset => INTERVAL '3 hours',
    end_offset   => INTERVAL '1 hour',
    schedule_interval => INTERVAL '1 hour');

CREATE MATERIALIZED VIEW page_stats_daily
WITH (timescaledb.continuous) AS
SELECT
    site_id,
    time_bucket('1 day', timestamp) AS day,
    url,
    COUNT(*) AS pageviews,
    COUNT(DISTINCT session_id) AS sessions,
    AVG(duration_ms) AS avg_duration_ms
FROM events
WHERE type = 'pageview'
GROUP BY site_id, time_bucket('1 day', timestamp), url
WITH NO DATA;

SELECT add_continuous_aggregate_policy('page_stats_daily',
    start_offset => INTERVAL '3 days',
    end_offset   => INTERVAL '1 day',
    schedule_interval => INTERVAL '1 day');

CREATE MATERIALIZED VIEW source_stats_daily
WITH (timescaledb.continuous) AS
SELECT
    site_id,
    time_bucket('1 day', timestamp) AS day,
    channel,
    referrer,
    utm_source,
    COUNT(DISTINCT session_id) AS sessions,
    COUNT(*) AS pageviews
FROM events
WHERE type = 'pageview'
GROUP BY site_id, time_bucket('1 day', timestamp), channel, referrer, utm_source
WITH NO DATA;

SELECT add_continuous_aggregate_policy('source_stats_daily',
    start_offset => INTERVAL '3 days',
    end_offset   => INTERVAL '1 day',
    schedule_interval => INTERVAL '1 day');
