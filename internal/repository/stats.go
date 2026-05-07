package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/funky-monkey/analyics-dash-tics/internal/model"
)

// StatsRepository queries pre-aggregated analytics data from continuous aggregates.
// All queries are scoped by site_id — never cross-tenant.
type StatsRepository interface {
	GetSummary(ctx context.Context, siteID string, from, to time.Time) (*model.StatsSummary, error)
	GetTimeSeries(ctx context.Context, siteID string, from, to time.Time) ([]*model.TimePoint, error)
	GetTopPages(ctx context.Context, siteID string, from, to time.Time, limit int) ([]*model.PageStat, error)
	GetTopSources(ctx context.Context, siteID string, from, to time.Time, limit int) ([]*model.SourceStat, error)
	GetAudienceByDimension(ctx context.Context, siteID, dimension string, from, to time.Time, limit int) ([]*model.AudienceStat, error)
}

type pgStatsRepository struct {
	pool *pgxpool.Pool
}

func (r *pgStatsRepository) GetSummary(ctx context.Context, siteID string, from, to time.Time) (*model.StatsSummary, error) {
	var s model.StatsSummary
	var totalDuration int64
	err := r.pool.QueryRow(ctx, `
		SELECT
			COALESCE(SUM(pageviews), 0),
			COALESCE(SUM(visitors), 0),
			COALESCE(SUM(sessions), 0),
			COALESCE(SUM(bounces), 0),
			COALESCE(SUM(total_duration_ms), 0)
		FROM stats_hourly
		WHERE site_id = $1 AND hour BETWEEN $2 AND $3
	`, siteID, from, to).Scan(
		&s.Pageviews, &s.Visitors, &s.Sessions, &s.Bounces, &totalDuration,
	)
	if err != nil {
		return nil, fmt.Errorf("statsRepository.GetSummary: %w", err)
	}
	if s.Sessions > 0 {
		s.BounceRate = float64(s.Bounces) / float64(s.Sessions) * 100
		s.AvgDuration = totalDuration / s.Sessions
	}
	return &s, nil
}

func (r *pgStatsRepository) GetTimeSeries(ctx context.Context, siteID string, from, to time.Time) ([]*model.TimePoint, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT hour, COALESCE(SUM(pageviews),0), COALESCE(SUM(visitors),0)
		FROM stats_hourly
		WHERE site_id = $1 AND hour BETWEEN $2 AND $3
		GROUP BY hour ORDER BY hour ASC
	`, siteID, from, to)
	if err != nil {
		return nil, fmt.Errorf("statsRepository.GetTimeSeries: %w", err)
	}
	defer rows.Close()

	var points []*model.TimePoint
	for rows.Next() {
		p := &model.TimePoint{}
		if err := rows.Scan(&p.Time, &p.Pageviews, &p.Visitors); err != nil {
			return nil, fmt.Errorf("statsRepository.GetTimeSeries: scan: %w", err)
		}
		points = append(points, p)
	}
	return points, rows.Err()
}

func (r *pgStatsRepository) GetTopPages(ctx context.Context, siteID string, from, to time.Time, limit int) ([]*model.PageStat, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT url, COALESCE(SUM(pageviews),0), COALESCE(SUM(sessions),0), COALESCE(AVG(avg_duration_ms),0)
		FROM page_stats_daily
		WHERE site_id = $1 AND day BETWEEN $2 AND $3
		GROUP BY url ORDER BY SUM(pageviews) DESC LIMIT $4
	`, siteID, from, to, limit)
	if err != nil {
		return nil, fmt.Errorf("statsRepository.GetTopPages: %w", err)
	}
	defer rows.Close()

	var pages []*model.PageStat
	for rows.Next() {
		p := &model.PageStat{}
		if err := rows.Scan(&p.URL, &p.Pageviews, &p.Sessions, &p.AvgDuration); err != nil {
			return nil, fmt.Errorf("statsRepository.GetTopPages: scan: %w", err)
		}
		pages = append(pages, p)
	}
	return pages, rows.Err()
}

func (r *pgStatsRepository) GetTopSources(ctx context.Context, siteID string, from, to time.Time, limit int) ([]*model.SourceStat, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT channel, referrer, COALESCE(SUM(sessions),0), COALESCE(SUM(pageviews),0)
		FROM source_stats_daily
		WHERE site_id = $1 AND day BETWEEN $2 AND $3
		GROUP BY channel, referrer ORDER BY SUM(sessions) DESC LIMIT $4
	`, siteID, from, to, limit)
	if err != nil {
		return nil, fmt.Errorf("statsRepository.GetTopSources: %w", err)
	}
	defer rows.Close()

	var sources []*model.SourceStat
	for rows.Next() {
		s := &model.SourceStat{}
		if err := rows.Scan(&s.Channel, &s.Referrer, &s.Sessions, &s.Pageviews); err != nil {
			return nil, fmt.Errorf("statsRepository.GetTopSources: scan: %w", err)
		}
		sources = append(sources, s)
	}
	return sources, rows.Err()
}

func (r *pgStatsRepository) GetAudienceByDimension(ctx context.Context, siteID, dimension string, from, to time.Time, limit int) ([]*model.AudienceStat, error) {
	// Validate dimension against allowlist before use in query — prevents SQL injection
	// despite using fmt.Sprintf. Column names cannot be parameterised in PostgreSQL.
	allowedDimensions := map[string]bool{
		"country": true, "device_type": true, "browser": true, "os": true,
	}
	if !allowedDimensions[dimension] {
		return nil, fmt.Errorf("statsRepository.GetAudienceByDimension: invalid dimension %q", dimension)
	}

	rows, err := r.pool.Query(ctx, fmt.Sprintf(`
		SELECT %s, COUNT(DISTINCT session_id) AS sessions
		FROM events
		WHERE site_id = $1 AND timestamp BETWEEN $2 AND $3
		  AND type = 'pageview' AND %s != ''
		GROUP BY %s ORDER BY sessions DESC LIMIT $4
	`, dimension, dimension, dimension), siteID, from, to, limit)
	if err != nil {
		return nil, fmt.Errorf("statsRepository.GetAudienceByDimension: %w", err)
	}
	defer rows.Close()

	var stats []*model.AudienceStat
	var total int64
	for rows.Next() {
		s := &model.AudienceStat{}
		if err := rows.Scan(&s.Dimension, &s.Sessions); err != nil {
			return nil, fmt.Errorf("statsRepository.GetAudienceByDimension: scan: %w", err)
		}
		total += s.Sessions
		stats = append(stats, s)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for _, s := range stats {
		if total > 0 {
			s.Share = float64(s.Sessions) / float64(total) * 100
		}
	}
	return stats, nil
}
