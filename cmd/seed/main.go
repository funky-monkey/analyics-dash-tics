// cmd/seed/main.go — local development data seeder.
// Populates the database with realistic high-scale analytics data for one demo site.
// REFUSES to run when ENV=production.
//
// Usage:
//
//	go run ./cmd/seed
//
// Creates (idempotent):
//   - Admin user:  demo@dashtics.io  /  Demo1234567890!
//   - Site:        acme.io  (token tk_demo_acme)
//
// Then deletes existing events for that site and inserts ~100 000 events
// spread across the past 90 days.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math/rand"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/sidneydekoning/analytics/config"
	"github.com/sidneydekoning/analytics/internal/repository"
	"golang.org/x/crypto/bcrypt"
)

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})))

	cfg, err := config.Load()
	if err != nil {
		slog.Error("config", "error", err)
		os.Exit(1)
	}
	if cfg.Env == "production" {
		slog.Error("seed: refusing to run in production — set ENV=development")
		os.Exit(1)
	}

	ctx := context.Background()
	pool, err := repository.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		slog.Error("db", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	if err := repository.Migrate(ctx, pool); err != nil {
		slog.Error("migrate", "error", err)
		os.Exit(1)
	}

	siteID, domain, err := resolveSeedSite(ctx, pool)
	if err != nil {
		slog.Error("resolveSeedSite", "error", err)
		os.Exit(1)
	}

	if err := seedEvents(ctx, pool, siteID); err != nil {
		slog.Error("seedEvents", "error", err)
		os.Exit(1)
	}

	slog.Info("seed complete", "site", domain)
}

// resolveSeedSite picks the site to seed into:
//  1. If a site already exists, use the most recently created one.
//  2. If no sites exist, create the demo user + acme.io site.
//
// Returns (siteID, domain, error).
func resolveSeedSite(ctx context.Context, pool *pgxpool.Pool) (string, string, error) {
	var siteID, domain string
	err := pool.QueryRow(ctx,
		`SELECT id, domain FROM sites ORDER BY created_at DESC LIMIT 1`).
		Scan(&siteID, &domain)
	if err == nil {
		slog.Info("seeding existing site", "id", siteID, "domain", domain)
		return siteID, domain, nil
	}

	// No sites — create demo account
	slog.Info("no sites found, creating demo account")
	hash, err := bcrypt.GenerateFromPassword([]byte("Demo1234567890!"), bcrypt.DefaultCost)
	if err != nil {
		return "", "", fmt.Errorf("bcrypt: %w", err)
	}
	var userID string
	if err := pool.QueryRow(ctx,
		`INSERT INTO users (email, password_hash, role, name, is_active)
		 VALUES ('demo@dashtics.io', $1, 'admin', 'Demo User', true)
		 ON CONFLICT (email) DO UPDATE SET password_hash = EXCLUDED.password_hash
		 RETURNING id`, string(hash)).Scan(&userID); err != nil {
		return "", "", fmt.Errorf("upsert user: %w", err)
	}
	if err := pool.QueryRow(ctx,
		`INSERT INTO sites (owner_id, name, domain, token, timezone)
		 VALUES ($1, 'Acme SaaS', 'acme.io', 'tk_demo_acme', 'Europe/Amsterdam')
		 ON CONFLICT (token) DO UPDATE SET owner_id = EXCLUDED.owner_id
		 RETURNING id`, userID).Scan(&siteID); err != nil {
		return "", "", fmt.Errorf("upsert site: %w", err)
	}
	slog.Info("demo site created", "domain", "acme.io", "login", "demo@dashtics.io / Demo1234567890!")
	return siteID, "acme.io", nil
}

// seedEvents deletes old seed data and inserts fresh events.
func seedEvents(ctx context.Context, pool *pgxpool.Pool, siteID string) error {
	tag, err := pool.Exec(ctx, `DELETE FROM events WHERE site_id=$1`, siteID)
	if err != nil {
		return fmt.Errorf("delete events: %w", err)
	}
	slog.Info("cleared old events", "deleted", tag.RowsAffected())

	now := time.Now().UTC().Truncate(time.Hour)
	daysBack := 90
	rng := rand.New(rand.NewSource(42)) // fixed seed for reproducible data

	var totalEvents int
	batchSize := 500

	type evRow struct {
		url, referrer, channel, utmSource, utmMedium, utmCampaign string
		country, city, device, browser, os, language               string
		sessionID, visitorID                                        string
		isBounce                                                    bool
		durationMS                                                  int
		props                                                       []byte
		ts                                                          time.Time
		eventType                                                   string
	}

	var buf []evRow

	flush := func() error {
		if len(buf) == 0 {
			return nil
		}
		tx, err := pool.Begin(ctx)
		if err != nil {
			return err
		}
		defer tx.Rollback(ctx) //nolint:errcheck

		for _, e := range buf {
			_, err := tx.Exec(ctx, `
				INSERT INTO events
					(site_id, type, url, referrer, channel, utm_source, utm_medium, utm_campaign,
					 country, city, device_type, browser, os, language,
					 session_id, visitor_id, is_bounce, duration_ms, props, timestamp)
				VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20)`,
				siteID, e.eventType, e.url, e.referrer, e.channel,
				e.utmSource, e.utmMedium, e.utmCampaign,
				e.country, e.city, e.device, e.browser, e.os, e.language,
				e.sessionID, e.visitorID, e.isBounce, e.durationMS, e.props, e.ts,
			)
			if err != nil {
				return fmt.Errorf("insert event: %w", err)
			}
		}
		if err := tx.Commit(ctx); err != nil {
			return err
		}
		totalEvents += len(buf)
		buf = buf[:0]
		return nil
	}

	for day := daysBack; day >= 0; day-- {
		dayStart := now.AddDate(0, 0, -day).Truncate(24 * time.Hour)
		weekday := dayStart.Weekday()

		// Realistic traffic: weekdays ~520 visitors, weekends ~310
		visitorsToday := 520
		if weekday == time.Saturday || weekday == time.Sunday {
			visitorsToday = 310
		}
		// Slight linear growth over 90 days (+30% from day 90 to day 0)
		growthFactor := 0.85 + 0.15*float64(daysBack-day)/float64(daysBack)
		visitorsToday = int(float64(visitorsToday) * growthFactor)

		for v := 0; v < visitorsToday; v++ {
			visitorID := fmt.Sprintf("v%x", rng.Int63())
			geo := pick(rng, geoPool)
			device := pickDevice(rng)
			browser := pickBrowser(rng, device)
			osName := pickOS(rng, device)
			lang := pick(rng, langPool)
			channel, referrer, utmSource, utmMedium, utmCampaign := pickChannel(rng)

			// Session start — hour weighted toward business hours
			hour := weightedHour(rng)
			minute := rng.Intn(60)
			second := rng.Intn(60)
			sessionStart := dayStart.Add(time.Duration(hour)*time.Hour +
				time.Duration(minute)*time.Minute +
				time.Duration(second)*time.Second)
			sessionID := fmt.Sprintf("s%x", rng.Int63())

			isBounce := rng.Float64() < 0.42
			numPages := 1
			if !isBounce {
				numPages = 1 + rng.Intn(6) // 2-6 pageviews if not a bounce
			}

			entryPage := pickPage(rng)
			t := sessionStart

			for pg := 0; pg < numPages; pg++ {
				pageURL := entryPage
				if pg > 0 {
					pageURL = pickPage(rng)
				}
				ref := ""
				if pg == 0 {
					ref = referrer
				} else {
					ref = "https://acme.io" + entryPage
				}

				duration := 0
				if !isBounce || pg < numPages-1 {
					duration = 15000 + rng.Intn(175000) // 15s–190s
				}

				props, _ := json.Marshal(map[string]string(nil))

				buf = append(buf, evRow{
					eventType:   "pageview",
					url:         "https://acme.io" + pageURL,
					referrer:    ref,
					channel:     channel,
					utmSource:   utmSource,
					utmMedium:   utmMedium,
					utmCampaign: utmCampaign,
					country:     geo[0],
					city:        geo[1],
					device:      device,
					browser:     browser,
					os:          osName,
					language:    lang,
					sessionID:   sessionID,
					visitorID:   visitorID,
					isBounce:    isBounce && pg == 0,
					durationMS:  duration,
					props:       props,
					ts:          t,
				})

				t = t.Add(time.Duration(30+rng.Intn(120)) * time.Second)

				// Custom events: ~8% of non-bounce sessions fire a custom event
				if !isBounce && rng.Float64() < 0.08 {
					evName, evProps := pickCustomEvent(rng, pageURL)
					eprops, _ := json.Marshal(evProps)
					buf = append(buf, evRow{
						eventType:  evName,
						url:        "https://acme.io" + pageURL,
						channel:    channel,
						country:    geo[0],
						city:       geo[1],
						device:     device,
						browser:    browser,
						os:         osName,
						language:   lang,
						sessionID:  sessionID,
						visitorID:  visitorID,
						isBounce:   false,
						durationMS: 0,
						props:      eprops,
						ts:         t,
					})
				}
			}

			if len(buf) >= batchSize {
				if err := flush(); err != nil {
					return err
				}
				if totalEvents%10000 == 0 {
					slog.Info("progress", "events", totalEvents)
				}
			}
		}
	}

	if err := flush(); err != nil {
		return err
	}
	slog.Info("events inserted", "total", totalEvents)

	// Continuous aggregates won't pick up bulk inserts until their next scheduled run.
	// Force a full refresh of all time so the dashboard shows data immediately.
	slog.Info("refreshing continuous aggregates (this may take a few seconds)...")

	for _, view := range []string{"stats_hourly", "page_stats_daily", "source_stats_daily"} {
		// Use NULL bounds to refresh all time — avoids pg type inference issues with $N params.
		q := fmt.Sprintf(`CALL refresh_continuous_aggregate('%s', NULL, NULL)`, view)
		if _, err := pool.Exec(ctx, q); err != nil {
			return fmt.Errorf("refresh %s: %w", view, err)
		}
		// Verify rows were written
		var rowCount int64
		if err := pool.QueryRow(ctx,
			`SELECT COUNT(*) FROM `+view+` WHERE site_id = $1`, siteID).Scan(&rowCount); err == nil {
			slog.Info("aggregate refreshed", "view", view, "rows", rowCount)
		}
	}

	// Final sanity check: raw events vs aggregated pageviews
	var rawEvents int64
	_ = pool.QueryRow(ctx, `SELECT COUNT(*) FROM events WHERE site_id=$1`, siteID).Scan(&rawEvents)
	var aggPageviews int64
	_ = pool.QueryRow(ctx, `SELECT COALESCE(SUM(pageviews),0) FROM stats_hourly WHERE site_id=$1`, siteID).Scan(&aggPageviews)
	slog.Info("verification", "raw_events", rawEvents, "aggregated_pageviews", aggPageviews)

	if aggPageviews == 0 {
		slog.Warn("aggregate has 0 pageviews — try running: psql $DATABASE_URL -c \"CALL refresh_continuous_aggregate('stats_hourly', NULL, NULL)\"")
	}
	return nil
}

// ── weighted sampling helpers ─────────────────────────────────────────────────

type weighted[T any] struct {
	value  T
	weight int
}

func pick[T any](rng *rand.Rand, pool []weighted[T]) T {
	total := 0
	for _, w := range pool {
		total += w.weight
	}
	n := rng.Intn(total)
	for _, w := range pool {
		n -= w.weight
		if n < 0 {
			return w.value
		}
	}
	return pool[len(pool)-1].value
}

// weightedHour returns an hour 0–23 weighted toward business hours.
func weightedHour(rng *rand.Rand) int {
	// Build a simple hourly weight: low at night, peak 9-17
	weights := []int{1, 1, 1, 1, 1, 2, 3, 5, 8, 10, 10, 10, 9, 10, 10, 9, 8, 7, 6, 5, 4, 3, 2, 1}
	return pick(rng, func() []weighted[int] {
		out := make([]weighted[int], len(weights))
		for i, w := range weights {
			out[i] = weighted[int]{i, w}
		}
		return out
	}())
}

// ── data pools ────────────────────────────────────────────────────────────────

// geo returns [country, city]
var geoPool = []weighted[[2]string]{
	{[2]string{"NL", "Amsterdam"}, 18},
	{[2]string{"NL", "Rotterdam"}, 4},
	{[2]string{"DE", "Berlin"}, 8},
	{[2]string{"DE", "Munich"}, 5},
	{[2]string{"GB", "London"}, 10},
	{[2]string{"US", "New York"}, 7},
	{[2]string{"US", "San Francisco"}, 5},
	{[2]string{"US", "Austin"}, 3},
	{[2]string{"FR", "Paris"}, 7},
	{[2]string{"BE", "Brussels"}, 4},
	{[2]string{"ES", "Madrid"}, 4},
	{[2]string{"IT", "Milan"}, 3},
	{[2]string{"CA", "Toronto"}, 3},
	{[2]string{"AU", "Sydney"}, 3},
	{[2]string{"SE", "Stockholm"}, 3},
	{[2]string{"DK", "Copenhagen"}, 2},
	{[2]string{"PL", "Warsaw"}, 2},
	{[2]string{"CH", "Zurich"}, 2},
	{[2]string{"PT", "Lisbon"}, 2},
	{[2]string{"IE", "Dublin"}, 2},
}

var langPool = []weighted[string]{
	{"nl", 20}, {"en", 40}, {"de", 12}, {"fr", 9},
	{"es", 6}, {"it", 4}, {"sv", 3}, {"pt", 3}, {"pl", 2}, {"da", 1},
}

var pagePool = []weighted[string]{
	{"/", 38},
	{"/pricing", 14},
	{"/features", 10},
	{"/blog/privacy-first-analytics", 7},
	{"/blog/cookieless-tracking", 6},
	{"/blog/ga4-alternatives", 5},
	{"/docs/getting-started", 5},
	{"/docs/api-reference", 4},
	{"/about", 4},
	{"/changelog", 3},
	{"/signup", 2},
	{"/login", 2},
}

func pickPage(rng *rand.Rand) string { return pick(rng, pagePool) }

func pickDevice(rng *rand.Rand) string {
	return pick(rng, []weighted[string]{
		{"desktop", 58}, {"mobile", 36}, {"tablet", 6},
	})
}

func pickBrowser(rng *rand.Rand, device string) string {
	if device == "mobile" {
		return pick(rng, []weighted[string]{
			{"Chrome", 48}, {"Safari", 38}, {"Firefox", 8}, {"Edge", 4}, {"Samsung Internet", 2},
		})
	}
	return pick(rng, []weighted[string]{
		{"Chrome", 55}, {"Safari", 20}, {"Firefox", 14}, {"Edge", 9}, {"Opera", 2},
	})
}

func pickOS(rng *rand.Rand, device string) string {
	switch device {
	case "mobile":
		return pick(rng, []weighted[string]{{"iOS", 52}, {"Android", 48}})
	case "tablet":
		return pick(rng, []weighted[string]{{"iOS", 60}, {"Android", 40}})
	default:
		return pick(rng, []weighted[string]{
			{"Windows", 42}, {"macOS", 34}, {"Linux", 14}, {"ChromeOS", 10},
		})
	}
}

// pickChannel returns channel, referrer, utmSource, utmMedium, utmCampaign.
func pickChannel(rng *rand.Rand) (string, string, string, string, string) {
	ch := pick(rng, []weighted[string]{
		{"organic", 35}, {"direct", 24}, {"referral", 14},
		{"social", 12}, {"email", 8}, {"ai", 4}, {"paid", 3},
	})
	switch ch {
	case "organic":
		ref := pick(rng, []weighted[string]{
			{"https://www.google.com", 65},
			{"https://www.bing.com", 18},
			{"https://duckduckgo.com", 12},
			{"https://www.yahoo.com", 5},
		})
		return ch, ref, "", "", ""
	case "referral":
		ref := pick(rng, []weighted[string]{
			{"https://news.ycombinator.com", 30},
			{"https://dev.to", 20},
			{"https://hashnode.com", 15},
			{"https://medium.com", 15},
			{"https://github.com", 12},
			{"https://producthunt.com", 8},
		})
		return ch, ref, "", "", ""
	case "social":
		ref := pick(rng, []weighted[string]{
			{"https://www.linkedin.com", 40},
			{"https://twitter.com", 25},
			{"https://www.reddit.com", 20},
			{"https://www.facebook.com", 10},
			{"https://www.tiktok.com", 5},
		})
		return ch, ref, "", "", ""
	case "email":
		campaigns := []weighted[string]{
			{"monthly-newsletter", 50}, {"product-update-q2", 30}, {"trial-followup", 20},
		}
		c := pick(rng, campaigns)
		return ch, "", "newsletter", "email", c
	case "paid":
		return ch, "https://www.google.com", "google", "cpc", "brand-search"
	case "ai":
		ref := pick(rng, []weighted[string]{
			{"https://chatgpt.com", 45},
			{"https://claude.ai", 30},
			{"https://perplexity.ai", 15},
			{"https://gemini.google.com", 10},
		})
		return ch, ref, "", "", ""
	default: // direct
		return ch, "", "", "", ""
	}
}

func pickCustomEvent(rng *rand.Rand, page string) (string, map[string]string) {
	if page == "/pricing" {
		return pick(rng, []weighted[string]{
			{"cta_click", 60}, {"plan_selected", 25}, {"faq_opened", 15},
		}), map[string]string{"page": page}
	}
	if page == "/signup" {
		return "form_submit", map[string]string{"form": "signup"}
	}
	return pick(rng, []weighted[string]{
		{"cta_click", 40}, {"outbound_click", 30}, {"file_download", 20}, {"video_play", 10},
	}), map[string]string{"page": page}
}
