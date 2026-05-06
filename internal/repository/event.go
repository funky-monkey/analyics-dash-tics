package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/sidneydekoning/analytics/internal/model"
)

// EventRepository defines write operations for analytics events.
type EventRepository interface {
	Write(ctx context.Context, e *model.Event) error
	WriteBatch(ctx context.Context, events []*model.Event) error
	CountBySite(ctx context.Context, siteID string, from, to time.Time) (int64, error)
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
