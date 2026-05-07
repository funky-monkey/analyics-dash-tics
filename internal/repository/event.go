package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/funky-monkey/analyics-dash-tics/internal/model"
)

// EventRepository defines write operations for analytics events.
type EventRepository interface {
	Write(ctx context.Context, e *model.Event) error
	WriteBatch(ctx context.Context, events []*model.Event) error
	CountBySite(ctx context.Context, siteID string, from, to time.Time) (int64, error)
	ListCustomEvents(ctx context.Context, siteID string, from, to time.Time, limit int) ([]*model.CustomEventStat, error)
	UpsertVisitorFirstSeen(ctx context.Context, siteID, visitorID string) error
}

type pgEventRepository struct {
	pool *pgxpool.Pool
}

func (r *pgEventRepository) Write(ctx context.Context, e *model.Event) error {
	props, err := json.Marshal(e.Props)
	if err != nil {
		return fmt.Errorf("eventRepository.Write: marshal props: %w", err)
	}
	_, err = r.pool.Exec(ctx, `
		INSERT INTO events
			(site_id, type, url, referrer, channel, utm_source, utm_medium, utm_campaign,
			 country, city, device_type, browser, os, language,
			 session_id, visitor_id, is_bounce, duration_ms, props, timestamp)
		VALUES
			($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20)
	`,
		e.SiteID, e.Type, e.URL, e.Referrer, e.Channel,
		e.UTMSource, e.UTMMedium, e.UTMCampaign,
		e.Country, e.City, e.DeviceType, e.Browser, e.OS, e.Language,
		e.SessionID, e.VisitorID, e.IsBounce, e.DurationMS,
		props, e.Timestamp,
	)
	if err != nil {
		return fmt.Errorf("eventRepository.Write: %w", err)
	}
	return nil
}

func (r *pgEventRepository) WriteBatch(ctx context.Context, events []*model.Event) error {
	for _, e := range events {
		if err := r.Write(ctx, e); err != nil {
			return fmt.Errorf("eventRepository.WriteBatch: %w", err)
		}
	}
	return nil
}

func (r *pgEventRepository) CountBySite(ctx context.Context, siteID string, from, to time.Time) (int64, error) {
	var count int64
	err := r.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM events WHERE site_id = $1 AND timestamp BETWEEN $2 AND $3`,
		siteID, from, to,
	).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("eventRepository.CountBySite: %w", err)
	}
	return count, nil
}

func (r *pgEventRepository) ListCustomEvents(ctx context.Context, siteID string, from, to time.Time, limit int) ([]*model.CustomEventStat, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT type, url, COUNT(*) AS cnt
		FROM events
		WHERE site_id=$1 AND type != 'pageview' AND timestamp BETWEEN $2 AND $3
		GROUP BY type, url
		ORDER BY cnt DESC
		LIMIT $4
	`, siteID, from, to, limit)
	if err != nil {
		return nil, fmt.Errorf("eventRepository.ListCustomEvents: %w", err)
	}
	defer rows.Close()
	var stats []*model.CustomEventStat
	for rows.Next() {
		s := &model.CustomEventStat{}
		if err := rows.Scan(&s.EventType, &s.URL, &s.Count); err != nil {
			return nil, fmt.Errorf("eventRepository.ListCustomEvents: scan: %w", err)
		}
		stats = append(stats, s)
	}
	return stats, rows.Err()
}

func (r *pgEventRepository) UpsertVisitorFirstSeen(ctx context.Context, siteID, visitorID string) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO visitor_first_seen (site_id, visitor_id)
		 VALUES ($1, $2)
		 ON CONFLICT (site_id, visitor_id) DO NOTHING`,
		siteID, visitorID)
	if err != nil {
		return fmt.Errorf("eventRepository.UpsertVisitorFirstSeen: %w", err)
	}
	return nil
}
