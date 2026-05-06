# Analytics SaaS — Plan 3: Dashboard

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:subagent-driven-development` (recommended) or `superpowers:executing-plans` to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build all analytics dashboard views — aggregate multi-site overview, single-site overview, pages, sources, audience, events, and funnels — served by Go handlers with HTMX-driven partial updates and uPlot/Chart.js charts.

**Architecture:** Each dashboard section is a Go handler that queries the repository layer (using continuous aggregates for speed), renders a full HTML page on first load, and returns HTMX fragments for filter/date-range changes. Chart data is serialised as JSON into the template and rendered client-side by uPlot (time-series) and Chart.js (pie/bar). All queries are scoped by `site_id` — never cross-tenant.

**Tech Stack:** Go `html/template`, HTMX (CDN), Alpine.js (CDN), uPlot (CDN), Chart.js (CDN), `pgx/v5` continuous aggregate queries, existing `repository.Repos`, `service.AuthService`, Ginkgo + Gomega (handler BDD tests)

> **No Co-Authored-By** in any commit message.
> **Working directory:** `/Users/sidneydekoning/stack/Github/analyics-dash-tics`
> **Module:** `github.com/sidneydekoning/analytics`

---

## File Map

```
internal/
  repository/
    stats.go                     — StatsRepository: query continuous aggregates
    stats_test.go                — integration tests (skip without TEST_DATABASE_URL)
  service/
    dashboard.go                 — DashboardService: aggregate stats, date range helpers
    dashboard_test.go            — unit tests
  handler/
    dashboard.go                 — all dashboard page handlers
    dashboard_test.go            — Ginkgo BDD tests

templates/
  layout/
    dashboard.html               — dashboard shell (sidebar nav + top bar)
  partials/
    stats-cards.html             — 4 KPI cards (pageviews, visitors, bounce, duration)
    chart-line.html              — time-series chart container
    table-pages.html             — top pages table
    table-sources.html           — traffic sources table
    table-audience.html          — audience breakdown table
    table-events.html            — events table
    funnel-view.html             — funnel steps + drop-off display
  pages/
    dashboard/
      aggregate.html             — multi-site aggregate overview
      overview.html              — single-site overview
      pages.html                 — top/entry/exit pages
      sources.html               — channels, referrers, UTM
      audience.html              — countries, devices, browsers
      events.html                — custom events + goals
      funnels.html               — funnel list
      funnel-detail.html         — single funnel with drop-off

static/
  ts/
    charts/
      timeseries.ts              — uPlot wrapper
      pie.ts                     — Chart.js donut/pie wrapper
      bar.ts                     — Chart.js bar wrapper
    pages/
      overview.ts                — init charts on overview page
```

---

### Task 1: Stats repository

**Files:**
- Create: `internal/repository/stats.go`
- Create: `internal/repository/stats_test.go`

- [ ] **Step 1: Write failing test — `internal/repository/stats_test.go`**

```go
package repository_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/sidneydekoning/analytics/internal/model"
	"github.com/sidneydekoning/analytics/internal/repository"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStatsRepository_GetSummary_Empty(t *testing.T) {
	repos := setupTestRepos(t)
	ctx := context.Background()

	owner := &model.User{Email: uniqueEmail(), PasswordHash: "x", Role: model.RoleUser, Name: "O", IsActive: true}
	require.NoError(t, repos.Users.Create(ctx, owner))
	site := &model.Site{
		OwnerID: owner.ID, Name: "S", Domain: "s.com",
		Token: fmt.Sprintf("tk_s%d", time.Now().UnixNano()), Timezone: "UTC",
	}
	require.NoError(t, repos.Sites.Create(ctx, site))

	from := time.Now().Add(-24 * time.Hour)
	to := time.Now()

	summary, err := repos.Stats.GetSummary(ctx, site.ID, from, to)
	require.NoError(t, err)
	assert.Equal(t, int64(0), summary.Pageviews)
	assert.Equal(t, int64(0), summary.Visitors)
}

func TestStatsRepository_GetTopPages_Empty(t *testing.T) {
	repos := setupTestRepos(t)
	ctx := context.Background()

	owner := &model.User{Email: uniqueEmail(), PasswordHash: "x", Role: model.RoleUser, Name: "O", IsActive: true}
	require.NoError(t, repos.Users.Create(ctx, owner))
	site := &model.Site{
		OwnerID: owner.ID, Name: "S", Domain: "s2.com",
		Token: fmt.Sprintf("tk_tp%d", time.Now().UnixNano()), Timezone: "UTC",
	}
	require.NoError(t, repos.Sites.Create(ctx, site))

	pages, err := repos.Stats.GetTopPages(ctx, site.ID, time.Now().Add(-time.Hour), time.Now(), 10)
	require.NoError(t, err)
	assert.Empty(t, pages)
}
```

- [ ] **Step 2: Run — verify it fails**

```bash
go test ./internal/repository/... -v -run TestStatsRepository
```

Expected: FAIL — `repos.Stats` field not yet defined.

- [ ] **Step 3: Add model types for stats — append to `internal/model/event.go`**

Add these types at the bottom of `internal/model/event.go`:

```go
// StatsSummary holds the aggregate KPI numbers for a dashboard period.
type StatsSummary struct {
	Pageviews   int64
	Visitors    int64
	Sessions    int64
	Bounces     int64
	BounceRate  float64 // percentage 0-100
	AvgDuration int64   // milliseconds
}

// PageStat holds per-URL traffic data.
type PageStat struct {
	URL         string
	Pageviews   int64
	Sessions    int64
	AvgDuration float64
}

// SourceStat holds per-channel/referrer traffic data.
type SourceStat struct {
	Channel  string
	Referrer string
	Sessions int64
	Pageviews int64
}

// AudienceStat holds a dimension breakdown row (country, device, browser, etc.).
type AudienceStat struct {
	Dimension string
	Sessions  int64
	Share     float64 // percentage 0-100
}

// TimePoint is a single data point for time-series charts.
type TimePoint struct {
	Time      time.Time
	Pageviews int64
	Visitors  int64
}
```

- [ ] **Step 4: Implement `internal/repository/stats.go`**

```go
package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/sidneydekoning/analytics/internal/model"
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
		&s.Pageviews, &s.Visitors, &s.Sessions, &s.Bounces, &s.AvgDuration,
	)
	if err != nil {
		return nil, fmt.Errorf("statsRepository.GetSummary: %w", err)
	}
	if s.Sessions > 0 {
		s.BounceRate = float64(s.Bounces) / float64(s.Sessions) * 100
		s.AvgDuration = s.AvgDuration / s.Sessions
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
	// dimension must be one of: country, device_type, browser, os — validated by caller
	allowedDimensions := map[string]bool{
		"country": true, "device_type": true, "browser": true, "os": true,
	}
	if !allowedDimensions[dimension] {
		return nil, fmt.Errorf("statsRepository.GetAudienceByDimension: invalid dimension %q", dimension)
	}

	// Raw events query — no continuous aggregate for audience breakdown in V1
	rows, err := r.pool.Query(ctx, fmt.Sprintf(`
		SELECT %s, COUNT(DISTINCT session_id) as sessions
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
```

**IMPORTANT:** `GetAudienceByDimension` uses `fmt.Sprintf` to interpolate the `dimension` column name into SQL. This is safe ONLY because `dimension` is validated against a strict allowlist before the query. Add this comment above the query to make the intention explicit.

- [ ] **Step 5: Add Stats to Repos — update `internal/repository/repos.go`**

```go
package repository

import "github.com/jackc/pgx/v5/pgxpool"

// Repos aggregates all repository interfaces for dependency injection.
type Repos struct {
	Users  UserRepository
	Sites  SiteRepository
	Events EventRepository
	Stats  StatsRepository
}

// New creates a Repos with all pg implementations wired up.
func New(pool *pgxpool.Pool) *Repos {
	return &Repos{
		Users:  &pgUserRepository{pool: pool},
		Sites:  &pgSiteRepository{pool: pool},
		Events: &pgEventRepository{pool: pool},
		Stats:  &pgStatsRepository{pool: pool},
	}
}
```

- [ ] **Step 6: Run tests**

```bash
go build ./...
go test ./internal/repository/... -v
```

Expected: clean build; tests SKIP without TEST_DATABASE_URL.

- [ ] **Step 7: Commit**

```bash
git add internal/model/event.go internal/repository/stats.go internal/repository/stats_test.go internal/repository/repos.go
git commit -m "feat: add stats repository with summary, timeseries, pages, sources, audience"
```

---

### Task 2: Dashboard service

**Files:**
- Create: `internal/service/dashboard.go`
- Create: `internal/service/dashboard_test.go`

- [ ] **Step 1: Write failing tests — `internal/service/dashboard_test.go`**

```go
package service_test

import (
	"testing"
	"time"

	"github.com/sidneydekoning/analytics/internal/service"
	"github.com/stretchr/testify/assert"
)

func TestDateRange_Last30Days(t *testing.T) {
	from, to := service.DateRange("30d")
	diff := to.Sub(from)
	assert.InDelta(t, 30*24*float64(time.Hour), float64(diff), float64(time.Hour))
	assert.True(t, to.After(from))
}

func TestDateRange_Last7Days(t *testing.T) {
	from, to := service.DateRange("7d")
	diff := to.Sub(from)
	assert.InDelta(t, 7*24*float64(time.Hour), float64(diff), float64(time.Hour))
}

func TestDateRange_Today(t *testing.T) {
	from, to := service.DateRange("today")
	assert.Equal(t, from.YearDay(), to.YearDay())
}

func TestDateRange_Unknown_Defaults30Days(t *testing.T) {
	from, to := service.DateRange("invalid")
	diff := to.Sub(from)
	assert.InDelta(t, 30*24*float64(time.Hour), float64(diff), float64(time.Hour))
}
```

- [ ] **Step 2: Run — verify it fails**

```bash
go test ./internal/service/... -v -run TestDateRange
```

Expected: FAIL.

- [ ] **Step 3: Implement `internal/service/dashboard.go`**

```go
package service

import "time"

// DateRange returns from/to time.Time for a named period string.
// Supported: "today", "7d", "30d", "90d". Unknown values default to "30d".
func DateRange(period string) (from, to time.Time) {
	now := time.Now().UTC()
	to = now
	switch period {
	case "today":
		from = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	case "7d":
		from = now.Add(-7 * 24 * time.Hour)
	case "90d":
		from = now.Add(-90 * 24 * time.Hour)
	default: // "30d" and anything unknown
		from = now.Add(-30 * 24 * time.Hour)
	}
	return from, to
}
```

- [ ] **Step 4: Run tests**

```bash
go test ./internal/service/... -v -run TestDateRange
```

Expected: all 4 PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/service/dashboard.go internal/service/dashboard_test.go
git commit -m "feat: add dashboard service with DateRange helper"
```

---

### Task 3: Dashboard layout template + navigation

**Files:**
- Create: `templates/layout/dashboard.html`
- Create: `templates/partials/stats-cards.html`
- Create: `templates/partials/chart-line.html`

- [ ] **Step 1: Create `templates/layout/dashboard.html`**

```html
<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>{{block "title" .}}Dashboard — Analytics{{end}}</title>
  <link rel="stylesheet" href="/static/css/output.css">
  <script src="https://unpkg.com/htmx.org@1.9.12" defer></script>
  <script src="https://unpkg.com/alpinejs@3.x.x/dist/cdn.min.js" defer></script>
  <script src="https://cdn.jsdelivr.net/npm/uplot@1.6.31/dist/uPlot.iife.min.js"></script>
  <link rel="stylesheet" href="https://cdn.jsdelivr.net/npm/uplot@1.6.31/dist/uPlot.min.css">
  <script src="https://cdn.jsdelivr.net/npm/chart.js@4.4.0/dist/chart.umd.min.js"></script>
</head>
<body class="bg-gray-50 text-gray-900 antialiased flex">

  {{/* Sidebar navigation */}}
  <aside class="w-14 bg-gray-900 min-h-screen flex flex-col items-center py-4 gap-3 shrink-0">
    {{/* Logo */}}
    <a href="/dashboard" class="w-8 h-8 bg-violet-600 rounded-lg flex items-center justify-center mb-2">
      <svg class="w-4 h-4 text-white" fill="currentColor" viewBox="0 0 24 24">
        <path d="M3 3h7v7H3V3zm0 11h7v7H3v-7zm11-11h7v7h-7V3zm0 11h7v7h-7v-7z"/>
      </svg>
    </a>
    {{/* Nav icons — HTMX nav swaps main content area */}}
    <a href="{{.SiteBaseURL}}/overview" title="Overview"
       class="w-9 h-9 rounded-lg flex items-center justify-center text-gray-400 hover:text-white hover:bg-gray-700 transition-colors {{if eq .ActiveNav "overview"}}bg-gray-700 text-white{{end}}">
      <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 19v-6a2 2 0 00-2-2H5a2 2 0 00-2 2v6a2 2 0 002 2h2a2 2 0 002-2zm0 0V9a2 2 0 012-2h2a2 2 0 012 2v10m-6 0a2 2 0 002 2h2a2 2 0 002-2m0 0V5a2 2 0 012-2h2a2 2 0 012 2v14a2 2 0 01-2 2h-2a2 2 0 01-2-2z"/></svg>
    </a>
    <a href="{{.SiteBaseURL}}/pages" title="Pages"
       class="w-9 h-9 rounded-lg flex items-center justify-center text-gray-400 hover:text-white hover:bg-gray-700 transition-colors {{if eq .ActiveNav "pages"}}bg-gray-700 text-white{{end}}">
      <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 12h6m-6 4h6m2 5H7a2 2 0 01-2-2V5a2 2 0 012-2h5.586a1 1 0 01.707.293l5.414 5.414a1 1 0 01.293.707V19a2 2 0 01-2 2z"/></svg>
    </a>
    <a href="{{.SiteBaseURL}}/sources" title="Sources"
       class="w-9 h-9 rounded-lg flex items-center justify-center text-gray-400 hover:text-white hover:bg-gray-700 transition-colors {{if eq .ActiveNav "sources"}}bg-gray-700 text-white{{end}}">
      <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 10V3L4 14h7v7l9-11h-7z"/></svg>
    </a>
    <a href="{{.SiteBaseURL}}/audience" title="Audience"
       class="w-9 h-9 rounded-lg flex items-center justify-center text-gray-400 hover:text-white hover:bg-gray-700 transition-colors {{if eq .ActiveNav "audience"}}bg-gray-700 text-white{{end}}">
      <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M17 20h5v-2a3 3 0 00-5.356-1.857M17 20H7m10 0v-2c0-.656-.126-1.283-.356-1.857M7 20H2v-2a3 3 0 015.356-1.857M7 20v-2c0-.656.126-1.283.356-1.857m0 0a5.002 5.002 0 019.288 0M15 7a3 3 0 11-6 0 3 3 0 016 0z"/></svg>
    </a>
    <a href="{{.SiteBaseURL}}/events" title="Events"
       class="w-9 h-9 rounded-lg flex items-center justify-center text-gray-400 hover:text-white hover:bg-gray-700 transition-colors {{if eq .ActiveNav "events"}}bg-gray-700 text-white{{end}}">
      <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 10V3L4 14h7v7l9-11h-7z"/></svg>
    </a>
    <a href="{{.SiteBaseURL}}/funnels" title="Funnels"
       class="w-9 h-9 rounded-lg flex items-center justify-center text-gray-400 hover:text-white hover:bg-gray-700 transition-colors {{if eq .ActiveNav "funnels"}}bg-gray-700 text-white{{end}}">
      <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M3 4a1 1 0 011-1h16a1 1 0 011 1v2.586a1 1 0 01-.293.707l-6.414 6.414a1 1 0 00-.293.707V17l-4 4v-6.586a1 1 0 00-.293-.707L3.293 7.293A1 1 0 013 6.586V4z"/></svg>
    </a>
    {{/* Spacer */}}
    <div class="flex-1"></div>
    {{/* Settings */}}
    <a href="{{.SiteBaseURL}}/settings" title="Settings"
       class="w-9 h-9 rounded-lg flex items-center justify-center text-gray-400 hover:text-white hover:bg-gray-700 transition-colors">
      <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M10.325 4.317c.426-1.756 2.924-1.756 3.35 0a1.724 1.724 0 002.573 1.066c1.543-.94 3.31.826 2.37 2.37a1.724 1.724 0 001.065 2.572c1.756.426 1.756 2.924 0 3.35a1.724 1.724 0 00-1.066 2.573c.94 1.543-.826 3.31-2.37 2.37a1.724 1.724 0 00-2.572 1.065c-.426 1.756-2.924 1.756-3.35 0a1.724 1.724 0 00-2.573-1.066c-1.543.94-3.31-.826-2.37-2.37a1.724 1.724 0 00-1.065-2.572c-1.756-.426-1.756-2.924 0-3.35a1.724 1.724 0 001.066-2.573c-.94-1.543.826-3.31 2.37-2.37.996.608 2.296.07 2.572-1.065z"/><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15 12a3 3 0 11-6 0 3 3 0 016 0z"/></svg>
    </a>
  </aside>

  {{/* Main content */}}
  <div class="flex-1 flex flex-col min-h-screen">
    {{/* Top bar */}}
    <header class="bg-white border-b border-gray-100 px-6 py-3 flex items-center justify-between">
      <div class="flex items-center gap-3">
        {{/* Site selector */}}
        <div x-data="{ open: false }" class="relative">
          <button @click="open = !open" class="flex items-center gap-2 text-sm font-medium text-gray-700 hover:text-gray-900">
            <span class="w-2 h-2 bg-green-400 rounded-full"></span>
            {{.SiteDomain}}
            <svg class="w-4 h-4 text-gray-400" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 9l-7 7-7-7"/></svg>
          </button>
        </div>
      </div>
      {{/* Date range picker */}}
      <div class="flex items-center gap-2">
        {{range $period := .AvailablePeriods}}
        <a href="?period={{$period.Value}}"
           class="px-3 py-1.5 text-xs font-medium rounded-lg transition-colors {{if eq $period.Value $.Period}}bg-violet-600 text-white{{else}}text-gray-500 hover:bg-gray-100{{end}}">
          {{$period.Label}}
        </a>
        {{end}}
      </div>
    </header>

    {{/* Page content */}}
    <main class="flex-1 p-6">
      {{block "content" .}}{{end}}
    </main>
  </div>

</body>
</html>
```

- [ ] **Step 2: Create `templates/partials/stats-cards.html`**

```html
{{define "stats-cards"}}
<div class="grid grid-cols-4 gap-4 mb-6">
  <div class="bg-white rounded-xl border border-gray-100 p-5">
    <p class="text-xs font-medium text-gray-400 uppercase tracking-wide mb-1">Visitors</p>
    <p class="text-2xl font-bold text-gray-900">{{.Summary.Visitors | formatNumber}}</p>
  </div>
  <div class="bg-white rounded-xl border border-gray-100 p-5">
    <p class="text-xs font-medium text-gray-400 uppercase tracking-wide mb-1">Pageviews</p>
    <p class="text-2xl font-bold text-gray-900">{{.Summary.Pageviews | formatNumber}}</p>
  </div>
  <div class="bg-white rounded-xl border border-gray-100 p-5">
    <p class="text-xs font-medium text-gray-400 uppercase tracking-wide mb-1">Bounce rate</p>
    <p class="text-2xl font-bold text-gray-900">{{printf "%.1f" .Summary.BounceRate}}%</p>
  </div>
  <div class="bg-white rounded-xl border border-gray-100 p-5">
    <p class="text-xs font-medium text-gray-400 uppercase tracking-wide mb-1">Avg. duration</p>
    <p class="text-2xl font-bold text-gray-900">{{.Summary.AvgDuration | formatDuration}}</p>
  </div>
</div>
{{end}}
```

- [ ] **Step 3: Create `templates/partials/chart-line.html`**

```html
{{define "chart-line"}}
<div class="bg-white rounded-xl border border-gray-100 p-5 mb-6">
  <div id="chart-timeseries" class="w-full" style="height:200px"></div>
</div>
<script>
(function() {
  var times = {{.ChartTimes}};
  var pageviews = {{.ChartPageviews}};
  var opts = {
    width: document.getElementById('chart-timeseries').offsetWidth || 800,
    height: 200,
    series: [
      {},
      {
        label: "Pageviews",
        stroke: "#7c6af7",
        fill: "rgba(124,106,247,0.08)",
        width: 2,
      }
    ],
    axes: [
      { stroke: "#ccc", ticks: { stroke: "#eee" } },
      { stroke: "#ccc", ticks: { stroke: "#eee" } },
    ],
    cursor: { show: true },
  };
  if (typeof uPlot !== 'undefined' && times.length > 0) {
    new uPlot(opts, [times, pageviews], document.getElementById('chart-timeseries'));
  }
})();
</script>
{{end}}
```

- [ ] **Step 4: Compile check**

```bash
go build ./...
```

- [ ] **Step 5: Commit**

```bash
git add templates/layout/dashboard.html templates/partials/
git commit -m "feat: add dashboard layout template and stats-cards/chart partials"
```

---

### Task 4: Dashboard handler + overview page

**Files:**
- Create: `internal/handler/dashboard.go`
- Create: `internal/handler/dashboard_test.go`
- Create: `templates/pages/dashboard/overview.html`
- Create: `templates/pages/dashboard/aggregate.html`

- [ ] **Step 1: Write failing Ginkgo tests — `internal/handler/dashboard_test.go`**

```go
package handler_test

import (
	"net/http"
	"net/http/httptest"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/sidneydekoning/analytics/internal/handler"
	"github.com/sidneydekoning/analytics/internal/middleware"
	"github.com/sidneydekoning/analytics/internal/service"
)

func contextWithUser(userID, role string) *http.Request {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	ctx := r.Context()
	ctx = middleware.WithUserID(ctx, userID)
	ctx = middleware.WithRole(ctx, role)
	return r.WithContext(ctx)
}

var _ = Describe("DashboardHandler", func() {
	var h *handler.DashboardHandler

	BeforeEach(func() {
		authSvc := service.NewAuth(
			[]byte("test-access-secret-32-bytes-xxxxx"),
			[]byte("test-refresh-secret-32-bytes-xxxx"),
		)
		// nil repos — tests cover redirect/auth behaviour, not DB queries
		h = handler.NewDashboardHandler(authSvc, nil)
	})

	Describe("GET /dashboard", func() {
		Context("when user has no sites", func() {
			It("redirects to /account/sites/new", func() {
				req := contextWithUser("user-123", "user")
				req.URL.Path = "/dashboard"
				rec := httptest.NewRecorder()

				h.Aggregate(rec, req)

				Expect(rec.Code).To(Equal(http.StatusSeeOther))
				Expect(rec.Header().Get("Location")).To(Equal("/account/sites/new"))
			})
		})
	})

	Describe("GET /sites/:id/overview", func() {
		Context("when site not found", func() {
			It("returns 404", func() {
				req := contextWithUser("user-123", "user")
				rec := httptest.NewRecorder()

				h.Overview(rec, req)

				Expect(rec.Code).To(Equal(http.StatusNotFound))
			})
		})
	})
})
```

**Note:** The test uses `middleware.WithUserID` and `middleware.WithRole` helpers. Add these to `internal/middleware/auth.go`:

```go
// WithUserID returns a context with the given user ID set.
func WithUserID(ctx context.Context, userID string) context.Context {
	return context.WithValue(ctx, ContextKeyUserID, userID)
}

// WithRole returns a context with the given role set.
func WithRole(ctx context.Context, role string) context.Context {
	return context.WithValue(ctx, ContextKeyRole, role)
}
```

- [ ] **Step 2: Run — verify it fails**

```bash
go test ./internal/handler/... -v -run TestHandler
```

Expected: compile error — `handler.DashboardHandler` not defined.

- [ ] **Step 3: Implement `internal/handler/dashboard.go`**

```go
package handler

import (
	"encoding/json"
	"html/template"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/sidneydekoning/analytics/internal/middleware"
	"github.com/sidneydekoning/analytics/internal/model"
	"github.com/sidneydekoning/analytics/internal/repository"
	"github.com/sidneydekoning/analytics/internal/service"
)

// DashboardHandler handles all analytics dashboard routes.
type DashboardHandler struct {
	auth  service.AuthService
	repos *repository.Repos
	tmpls map[string]*template.Template
}

// NewDashboardHandler constructs a DashboardHandler. repos may be nil in tests.
func NewDashboardHandler(auth service.AuthService, repos *repository.Repos) *DashboardHandler {
	return &DashboardHandler{auth: auth, repos: repos}
}

// SetTemplates wires the template map.
func (h *DashboardHandler) SetTemplates(tmpls map[string]*template.Template) {
	h.tmpls = tmpls
}

// periodsAvailable are the date range options shown in the top bar.
var periodsAvailable = []struct {
	Value string
	Label string
}{
	{"today", "Today"},
	{"7d", "7 days"},
	{"30d", "30 days"},
	{"90d", "90 days"},
}

// dashboardBaseData holds fields shared across all dashboard pages.
type dashboardBaseData struct {
	SiteID          string
	SiteDomain      string
	SiteBaseURL     string
	ActiveNav       string
	Period          string
	AvailablePeriods []struct{ Value, Label string }
	Summary         *model.StatsSummary
	ChartTimes      template.JS
	ChartPageviews  template.JS
}

// Aggregate renders GET /dashboard — multi-site aggregate view.
// Redirects to /account/sites/new if the user has no sites yet.
func (h *DashboardHandler) Aggregate(w http.ResponseWriter, r *http.Request) {
	userID := middleware.UserIDFromContext(r.Context())

	if h.repos == nil {
		http.Redirect(w, r, "/account/sites/new", http.StatusSeeOther)
		return
	}

	sites, err := h.repos.Sites.ListByOwner(r.Context(), userID)
	if err != nil || len(sites) == 0 {
		http.Redirect(w, r, "/account/sites/new", http.StatusSeeOther)
		return
	}

	// If only one site, redirect directly to its overview
	if len(sites) == 1 {
		http.Redirect(w, r, "/sites/"+sites[0].ID+"/overview", http.StatusSeeOther)
		return
	}

	h.renderTemplate(w, "aggregate.html", map[string]any{
		"Sites":            sites,
		"ActiveNav":        "overview",
		"AvailablePeriods": periodsAvailable,
		"Period":           "30d",
		"SiteBaseURL":      "/dashboard",
		"SiteDomain":       "All sites",
	})
}

// Overview renders GET /sites/:id/overview.
func (h *DashboardHandler) Overview(w http.ResponseWriter, r *http.Request) {
	siteID := chi.URLParam(r, "siteID")
	userID := middleware.UserIDFromContext(r.Context())

	if h.repos == nil {
		http.NotFound(w, r)
		return
	}

	site, err := h.repos.Sites.GetByID(r.Context(), siteID)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	// Verify the user has access to this site
	sites, err := h.repos.Sites.ListByOwner(r.Context(), userID)
	if err != nil || !userOwnsSite(sites, siteID) {
		http.NotFound(w, r)
		return
	}

	period := r.URL.Query().Get("period")
	if period == "" {
		period = "30d"
	}
	from, to := service.DateRange(period)

	summary, err := h.repos.Stats.GetSummary(r.Context(), siteID, from, to)
	if err != nil {
		slog.Error("dashboard.Overview: get summary", "error", err)
		summary = &model.StatsSummary{}
	}

	timeseries, err := h.repos.Stats.GetTimeSeries(r.Context(), siteID, from, to)
	if err != nil {
		slog.Error("dashboard.Overview: get timeseries", "error", err)
	}

	pages, err := h.repos.Stats.GetTopPages(r.Context(), siteID, from, to, 10)
	if err != nil {
		slog.Error("dashboard.Overview: get top pages", "error", err)
	}

	sources, err := h.repos.Stats.GetTopSources(r.Context(), siteID, from, to, 10)
	if err != nil {
		slog.Error("dashboard.Overview: get top sources", "error", err)
	}

	chartTimes, chartPageviews := marshalTimeSeries(timeseries)

	h.renderTemplate(w, "overview.html", map[string]any{
		"SiteID":           siteID,
		"SiteDomain":       site.Domain,
		"SiteBaseURL":      "/sites/" + siteID,
		"ActiveNav":        "overview",
		"Period":           period,
		"AvailablePeriods": periodsAvailable,
		"Summary":          summary,
		"ChartTimes":       template.JS(chartTimes),
		"ChartPageviews":   template.JS(chartPageviews),
		"TopPages":         pages,
		"TopSources":       sources,
	})
}

// Pages renders GET /sites/:id/pages.
func (h *DashboardHandler) Pages(w http.ResponseWriter, r *http.Request) {
	siteID := chi.URLParam(r, "siteID")
	if h.repos == nil {
		http.NotFound(w, r)
		return
	}
	site, err := h.repos.Sites.GetByID(r.Context(), siteID)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	period := r.URL.Query().Get("period")
	if period == "" {
		period = "30d"
	}
	from, to := service.DateRange(period)
	pages, err := h.repos.Stats.GetTopPages(r.Context(), siteID, from, to, 50)
	if err != nil {
		slog.Error("dashboard.Pages", "error", err)
	}
	h.renderTemplate(w, "pages.html", map[string]any{
		"SiteID": siteID, "SiteDomain": site.Domain,
		"SiteBaseURL": "/sites/" + siteID, "ActiveNav": "pages",
		"Period": period, "AvailablePeriods": periodsAvailable,
		"Pages": pages,
	})
}

// Sources renders GET /sites/:id/sources.
func (h *DashboardHandler) Sources(w http.ResponseWriter, r *http.Request) {
	siteID := chi.URLParam(r, "siteID")
	if h.repos == nil {
		http.NotFound(w, r)
		return
	}
	site, err := h.repos.Sites.GetByID(r.Context(), siteID)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	period := r.URL.Query().Get("period")
	if period == "" {
		period = "30d"
	}
	from, to := service.DateRange(period)
	sources, err := h.repos.Stats.GetTopSources(r.Context(), siteID, from, to, 50)
	if err != nil {
		slog.Error("dashboard.Sources", "error", err)
	}
	h.renderTemplate(w, "sources.html", map[string]any{
		"SiteID": siteID, "SiteDomain": site.Domain,
		"SiteBaseURL": "/sites/" + siteID, "ActiveNav": "sources",
		"Period": period, "AvailablePeriods": periodsAvailable,
		"Sources": sources,
	})
}

// Audience renders GET /sites/:id/audience.
func (h *DashboardHandler) Audience(w http.ResponseWriter, r *http.Request) {
	siteID := chi.URLParam(r, "siteID")
	if h.repos == nil {
		http.NotFound(w, r)
		return
	}
	site, err := h.repos.Sites.GetByID(r.Context(), siteID)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	period := r.URL.Query().Get("period")
	if period == "" {
		period = "30d"
	}
	from, to := service.DateRange(period)
	countries, _ := h.repos.Stats.GetAudienceByDimension(r.Context(), siteID, "country", from, to, 20)
	devices, _ := h.repos.Stats.GetAudienceByDimension(r.Context(), siteID, "device_type", from, to, 5)
	browsers, _ := h.repos.Stats.GetAudienceByDimension(r.Context(), siteID, "browser", from, to, 10)
	h.renderTemplate(w, "audience.html", map[string]any{
		"SiteID": siteID, "SiteDomain": site.Domain,
		"SiteBaseURL": "/sites/" + siteID, "ActiveNav": "audience",
		"Period": period, "AvailablePeriods": periodsAvailable,
		"Countries": countries, "Devices": devices, "Browsers": browsers,
	})
}

// Events renders GET /sites/:id/events.
func (h *DashboardHandler) Events(w http.ResponseWriter, r *http.Request) {
	siteID := chi.URLParam(r, "siteID")
	if h.repos == nil {
		http.NotFound(w, r)
		return
	}
	site, err := h.repos.Sites.GetByID(r.Context(), siteID)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	period := r.URL.Query().Get("period")
	if period == "" {
		period = "30d"
	}
	h.renderTemplate(w, "events.html", map[string]any{
		"SiteID": siteID, "SiteDomain": site.Domain,
		"SiteBaseURL": "/sites/" + siteID, "ActiveNav": "events",
		"Period": period, "AvailablePeriods": periodsAvailable,
	})
}

// Funnels renders GET /sites/:id/funnels.
func (h *DashboardHandler) Funnels(w http.ResponseWriter, r *http.Request) {
	siteID := chi.URLParam(r, "siteID")
	if h.repos == nil {
		http.NotFound(w, r)
		return
	}
	site, err := h.repos.Sites.GetByID(r.Context(), siteID)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	h.renderTemplate(w, "funnels.html", map[string]any{
		"SiteID": siteID, "SiteDomain": site.Domain,
		"SiteBaseURL": "/sites/" + siteID, "ActiveNav": "funnels",
		"AvailablePeriods": periodsAvailable, "Period": "30d",
	})
}

func (h *DashboardHandler) renderTemplate(w http.ResponseWriter, name string, data any) {
	if h.tmpls == nil {
		w.Header().Set("Content-Type", "text/html")
		return
	}
	t, ok := h.tmpls[name]
	if !ok {
		slog.Error("dashboard template not found", "name", name)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := t.ExecuteTemplate(w, "dashboard.html", data); err != nil {
		slog.Error("render dashboard template", "name", name, "error", err)
	}
}

func userOwnsSite(sites []*model.Site, siteID string) bool {
	for _, s := range sites {
		if s.ID == siteID {
			return true
		}
	}
	return false
}

func marshalTimeSeries(points []*model.TimePoint) (times, pageviews string) {
	if len(points) == 0 {
		return "[]", "[]"
	}
	ts := make([]int64, len(points))
	pvs := make([]int64, len(points))
	for i, p := range points {
		ts[i] = p.Time.Unix()
		pvs[i] = p.Pageviews
	}
	tb, _ := json.Marshal(ts)
	pb, _ := json.Marshal(pvs)
	return string(tb), string(pb)
}

// formatDuration is a template function converting ms to "Xm Ys" string.
func formatDuration(ms int64) string {
	if ms == 0 {
		return "0s"
	}
	total := ms / 1000
	m := total / 60
	s := total % 60
	if m == 0 {
		return fmt.Sprintf("%ds", s)
	}
	return fmt.Sprintf("%dm %ds", m, s)
}

// formatNumber formats large integers with k suffix (e.g. 12430 → "12.4k").
func formatNumber(n int64) string {
	if n >= 1_000_000 {
		return fmt.Sprintf("%.1fM", float64(n)/1_000_000)
	}
	if n >= 1000 {
		return fmt.Sprintf("%.1fk", float64(n)/1000)
	}
	return fmt.Sprintf("%d", n)
}
```

**Note:** Add `"fmt"` to the import block.

- [ ] **Step 4: Create `templates/pages/dashboard/overview.html`**

```html
{{template "dashboard.html" .}}

{{define "title"}}{{.SiteDomain}} — Overview{{end}}

{{define "content"}}
{{template "stats-cards" .}}
{{template "chart-line" .}}

<div class="grid grid-cols-2 gap-4">
  {{/* Top pages */}}
  <div class="bg-white rounded-xl border border-gray-100 p-5">
    <h3 class="text-sm font-semibold text-gray-700 mb-4">Top pages</h3>
    {{if .TopPages}}
    <table class="w-full text-sm">
      <thead><tr class="text-xs text-gray-400 border-b border-gray-50">
        <th class="text-left pb-2 font-medium">Page</th>
        <th class="text-right pb-2 font-medium">Views</th>
      </tr></thead>
      <tbody>
        {{range .TopPages}}
        <tr class="border-b border-gray-50 last:border-0">
          <td class="py-2 text-gray-700 truncate max-w-xs">{{.URL}}</td>
          <td class="py-2 text-right font-medium text-gray-900">{{.Pageviews}}</td>
        </tr>
        {{end}}
      </tbody>
    </table>
    {{else}}
    <p class="text-sm text-gray-400 text-center py-8">No data yet — add the tracking script to your site.</p>
    {{end}}
  </div>

  {{/* Top sources */}}
  <div class="bg-white rounded-xl border border-gray-100 p-5">
    <h3 class="text-sm font-semibold text-gray-700 mb-4">Traffic sources</h3>
    {{if .TopSources}}
    <table class="w-full text-sm">
      <thead><tr class="text-xs text-gray-400 border-b border-gray-50">
        <th class="text-left pb-2 font-medium">Source</th>
        <th class="text-right pb-2 font-medium">Sessions</th>
      </tr></thead>
      <tbody>
        {{range .TopSources}}
        <tr class="border-b border-gray-50 last:border-0">
          <td class="py-2">
            <span class="inline-block px-2 py-0.5 text-xs rounded-full bg-violet-50 text-violet-700 mr-2">{{.Channel}}</span>
            <span class="text-gray-600 text-xs truncate">{{.Referrer}}</span>
          </td>
          <td class="py-2 text-right font-medium text-gray-900">{{.Sessions}}</td>
        </tr>
        {{end}}
      </tbody>
    </table>
    {{else}}
    <p class="text-sm text-gray-400 text-center py-8">No sources tracked yet.</p>
    {{end}}
  </div>
</div>
{{end}}
```

- [ ] **Step 5: Create `templates/pages/dashboard/aggregate.html`**

```html
{{template "dashboard.html" .}}

{{define "title"}}All sites — Analytics{{end}}

{{define "content"}}
<h1 class="text-xl font-bold text-gray-900 mb-6">Your sites</h1>
{{if .Sites}}
<div class="grid grid-cols-3 gap-4">
  {{range .Sites}}
  <a href="/sites/{{.ID}}/overview"
     class="bg-white rounded-xl border border-gray-100 p-5 hover:shadow-sm transition-shadow group">
    <div class="flex items-center gap-3 mb-3">
      <div class="w-8 h-8 bg-violet-100 rounded-lg flex items-center justify-center">
        <span class="text-violet-600 text-xs font-bold">{{slice .Domain 0 1 | upper}}</span>
      </div>
      <span class="font-medium text-gray-900 group-hover:text-violet-600 transition-colors">{{.Domain}}</span>
    </div>
    <p class="text-xs text-gray-400">{{.Name}}</p>
  </a>
  {{end}}
</div>
{{else}}
<div class="text-center py-16">
  <p class="text-gray-400 mb-4">No sites yet.</p>
  <a href="/account/sites/new" class="btn-primary">Add your first site</a>
</div>
{{end}}
{{end}}
```

- [ ] **Step 6: Create remaining dashboard page templates**

Create `templates/pages/dashboard/pages.html`:
```html
{{template "dashboard.html" .}}
{{define "title"}}{{.SiteDomain}} — Pages{{end}}
{{define "content"}}
<h2 class="text-lg font-semibold text-gray-900 mb-4">Top pages</h2>
<div class="bg-white rounded-xl border border-gray-100 p-5">
  {{if .Pages}}
  <table class="w-full text-sm">
    <thead><tr class="text-xs text-gray-400 border-b border-gray-100">
      <th class="text-left pb-3 font-medium">URL</th>
      <th class="text-right pb-3 font-medium">Pageviews</th>
      <th class="text-right pb-3 font-medium">Sessions</th>
      <th class="text-right pb-3 font-medium">Avg. time</th>
    </tr></thead>
    <tbody>
      {{range .Pages}}
      <tr class="border-b border-gray-50 last:border-0">
        <td class="py-3 text-gray-700 font-mono text-xs">{{.URL}}</td>
        <td class="py-3 text-right font-medium">{{.Pageviews}}</td>
        <td class="py-3 text-right text-gray-500">{{.Sessions}}</td>
        <td class="py-3 text-right text-gray-500">{{printf "%.0f" .AvgDuration}}ms</td>
      </tr>
      {{end}}
    </tbody>
  </table>
  {{else}}
  <p class="text-sm text-gray-400 text-center py-12">No page data for this period.</p>
  {{end}}
</div>
{{end}}
```

Create `templates/pages/dashboard/sources.html`:
```html
{{template "dashboard.html" .}}
{{define "title"}}{{.SiteDomain}} — Sources{{end}}
{{define "content"}}
<h2 class="text-lg font-semibold text-gray-900 mb-4">Traffic sources</h2>
<div class="bg-white rounded-xl border border-gray-100 p-5">
  {{if .Sources}}
  <table class="w-full text-sm">
    <thead><tr class="text-xs text-gray-400 border-b border-gray-100">
      <th class="text-left pb-3 font-medium">Channel</th>
      <th class="text-left pb-3 font-medium">Referrer</th>
      <th class="text-right pb-3 font-medium">Sessions</th>
      <th class="text-right pb-3 font-medium">Pageviews</th>
    </tr></thead>
    <tbody>
      {{range .Sources}}
      <tr class="border-b border-gray-50 last:border-0">
        <td class="py-3"><span class="px-2 py-1 text-xs rounded-full bg-violet-50 text-violet-700">{{.Channel}}</span></td>
        <td class="py-3 text-gray-600 text-xs">{{.Referrer}}</td>
        <td class="py-3 text-right font-medium">{{.Sessions}}</td>
        <td class="py-3 text-right text-gray-500">{{.Pageviews}}</td>
      </tr>
      {{end}}
    </tbody>
  </table>
  {{else}}
  <p class="text-sm text-gray-400 text-center py-12">No source data for this period.</p>
  {{end}}
</div>
{{end}}
```

Create `templates/pages/dashboard/audience.html`:
```html
{{template "dashboard.html" .}}
{{define "title"}}{{.SiteDomain}} — Audience{{end}}
{{define "content"}}
<div class="grid grid-cols-3 gap-4">
  <div class="bg-white rounded-xl border border-gray-100 p-5">
    <h3 class="text-sm font-semibold text-gray-700 mb-4">Countries</h3>
    {{range .Countries}}
    <div class="flex items-center justify-between py-1.5 border-b border-gray-50 last:border-0">
      <span class="text-sm text-gray-700">{{.Dimension}}</span>
      <span class="text-sm font-medium">{{.Sessions}}</span>
    </div>
    {{else}}<p class="text-sm text-gray-400 text-center py-8">No data</p>{{end}}
  </div>
  <div class="bg-white rounded-xl border border-gray-100 p-5">
    <h3 class="text-sm font-semibold text-gray-700 mb-4">Devices</h3>
    {{range .Devices}}
    <div class="flex items-center justify-between py-1.5 border-b border-gray-50 last:border-0">
      <span class="text-sm text-gray-700 capitalize">{{.Dimension}}</span>
      <span class="text-sm font-medium">{{printf "%.1f" .Share}}%</span>
    </div>
    {{else}}<p class="text-sm text-gray-400 text-center py-8">No data</p>{{end}}
  </div>
  <div class="bg-white rounded-xl border border-gray-100 p-5">
    <h3 class="text-sm font-semibold text-gray-700 mb-4">Browsers</h3>
    {{range .Browsers}}
    <div class="flex items-center justify-between py-1.5 border-b border-gray-50 last:border-0">
      <span class="text-sm text-gray-700">{{.Dimension}}</span>
      <span class="text-sm font-medium">{{.Sessions}}</span>
    </div>
    {{else}}<p class="text-sm text-gray-400 text-center py-8">No data</p>{{end}}
  </div>
</div>
{{end}}
```

Create `templates/pages/dashboard/events.html`:
```html
{{template "dashboard.html" .}}
{{define "title"}}{{.SiteDomain}} — Events{{end}}
{{define "content"}}
<h2 class="text-lg font-semibold text-gray-900 mb-4">Events</h2>
<div class="bg-white rounded-xl border border-gray-100 p-12 text-center">
  <p class="text-gray-400 text-sm">Custom event tracking coming in a future update.</p>
  <p class="text-gray-300 text-xs mt-2">Track events with <code class="bg-gray-100 px-1 rounded">window.analytics.track('event-name')</code></p>
</div>
{{end}}
```

Create `templates/pages/dashboard/funnels.html`:
```html
{{template "dashboard.html" .}}
{{define "title"}}{{.SiteDomain}} — Funnels{{end}}
{{define "content"}}
<h2 class="text-lg font-semibold text-gray-900 mb-4">Funnels</h2>
<div class="bg-white rounded-xl border border-gray-100 p-12 text-center">
  <p class="text-gray-400 text-sm">Funnel builder coming soon.</p>
</div>
{{end}}
```

- [ ] **Step 7: Register template functions in main.go**

The dashboard templates use `formatNumber` and `formatDuration` functions. These need to be registered in `buildTemplateMap`. Update `cmd/server/main.go`:

Change `buildTemplateMap` to accept a `template.FuncMap`:
```go
func buildTemplateMap(basePath, pagesRoot string) (map[string]*template.Template, error) {
	funcs := template.FuncMap{
		"formatNumber":   handler.FormatNumber,
		"formatDuration": handler.FormatDuration,
		"upper": strings.ToUpper,
	}
	tmpls := make(map[string]*template.Template)
	err := filepath.WalkDir(pagesRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && strings.HasSuffix(path, ".html") && path != basePath {
			name := filepath.Base(path)
			t, err := template.New(filepath.Base(basePath)).Funcs(funcs).ParseFiles(basePath, path)
			if err != nil {
				return fmt.Errorf("parse %s: %w", path, err)
			}
			tmpls[name] = t
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("buildTemplateMap: %w", err)
	}
	if len(tmpls) == 0 {
		return nil, fmt.Errorf("buildTemplateMap: no page templates found under %s", pagesRoot)
	}
	return tmpls, nil
}
```

Export `FormatNumber` and `FormatDuration` from `internal/handler/dashboard.go` (capitalise them):

```go
// FormatNumber formats large integers with k/M suffix for display.
func FormatNumber(n int64) string { ... }

// FormatDuration converts milliseconds to "Xm Ys" display string.
func FormatDuration(ms int64) string { ... }
```

Also update `buildTemplateMap` in `cmd/server/main.go` to parse partials alongside the page file. The partials (`stats-cards.html`, `chart-line.html`) must be included for templates that use them. Parse them alongside:

```go
// For dashboard pages, also parse all partials
partialGlob, _ := filepath.Glob("templates/partials/*.html")
allFiles := append([]string{basePath}, partialGlob...)
allFiles = append(allFiles, path)
t, err := template.New(filepath.Base(basePath)).Funcs(funcs).ParseFiles(allFiles...)
```

Actually, to keep it simple: change the `buildTemplateMap` to always include partials for ALL templates:

```go
func buildTemplateMap(basePath, pagesRoot string) (map[string]*template.Template, error) {
	funcs := template.FuncMap{
		"formatNumber":   handler.FormatNumber,
		"formatDuration": handler.FormatDuration,
		"upper":          strings.ToUpper,
	}

	// Collect all partial template files
	partials, err := filepath.Glob("templates/partials/*.html")
	if err != nil {
		return nil, fmt.Errorf("buildTemplateMap: glob partials: %w", err)
	}

	tmpls := make(map[string]*template.Template)
	err = filepath.WalkDir(pagesRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		// Skip layout and partials directories — they are not standalone pages
		if strings.Contains(path, "/layout/") || strings.Contains(path, "/partials/") {
			return nil
		}
		if !d.IsDir() && strings.HasSuffix(path, ".html") {
			name := filepath.Base(path)
			files := []string{basePath}
			files = append(files, partials...)
			files = append(files, path)
			t, err := template.New(filepath.Base(basePath)).Funcs(funcs).ParseFiles(files...)
			if err != nil {
				return fmt.Errorf("parse %s: %w", path, err)
			}
			tmpls[name] = t
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("buildTemplateMap: %w", err)
	}
	if len(tmpls) == 0 {
		return nil, fmt.Errorf("buildTemplateMap: no page templates found under %s", pagesRoot)
	}
	return tmpls, nil
}
```

**CRITICAL:** The `dashboard.html` layout file is in `templates/layout/`. The `buildTemplateMap` function uses `basePath = "templates/layout/base.html"`. But dashboard pages need `templates/layout/dashboard.html` as their base. 

The cleanest solution: detect which base template to use based on directory:
- Pages in `templates/pages/dashboard/` → use `templates/layout/dashboard.html`
- All other pages → use `templates/layout/base.html`

Update the walk to handle this:

```go
err = filepath.WalkDir(pagesRoot, func(path string, d fs.DirEntry, err error) error {
    if err != nil || d.IsDir() {
        return err
    }
    if strings.Contains(path, "/layout/") || strings.Contains(path, "/partials/") {
        return nil
    }
    if !strings.HasSuffix(path, ".html") {
        return nil
    }
    name := filepath.Base(path)
    
    // Dashboard pages use the dashboard layout
    layoutPath := basePath
    if strings.Contains(path, "/dashboard/") {
        layoutPath = "templates/layout/dashboard.html"
    }
    
    files := []string{layoutPath}
    files = append(files, partials...)
    files = append(files, path)
    
    t, err := template.New(filepath.Base(layoutPath)).Funcs(funcs).ParseFiles(files...)
    if err != nil {
        return fmt.Errorf("parse %s: %w", path, err)
    }
    tmpls[name] = t
    return nil
})
```

And update `renderTemplate` in `AuthHandler` and `SitesHandler` to call `t.ExecuteTemplate(w, "base.html", data)` (unchanged).

For `DashboardHandler.renderTemplate`, it already calls `t.ExecuteTemplate(w, "dashboard.html", data)` — correct.

- [ ] **Step 8: Wire dashboard routes in `cmd/server/main.go`**

After wiring `sitesHandler`, add:

```go
dashHandler := handler.NewDashboardHandler(authSvc, repos)
dashHandler.SetTemplates(tmpls)
```

Replace the placeholder dashboard routes inside the `jwtAuth` group:

```go
r.With(jwtAuth).Group(func(r chi.Router) {
    r.Get("/dashboard", dashHandler.Aggregate)
    r.Get("/account/sites/new", sitesHandler.NewSitePage)
    r.Post("/account/sites/new", sitesHandler.CreateSite)
    r.Get("/sites/{siteID}/overview", dashHandler.Overview)
    r.Get("/sites/{siteID}/pages", dashHandler.Pages)
    r.Get("/sites/{siteID}/sources", dashHandler.Sources)
    r.Get("/sites/{siteID}/audience", dashHandler.Audience)
    r.Get("/sites/{siteID}/events", dashHandler.Events)
    r.Get("/sites/{siteID}/funnels", dashHandler.Funnels)
})
```

- [ ] **Step 9: Build and test**

```bash
go build -o bin/analytics ./cmd/server
go test -race ./...
```

Expected: clean build, all tests pass.

- [ ] **Step 10: Rebuild Tailwind CSS**

```bash
./bin/tailwindcss -i static/css/input.css -o static/css/output.css --minify
```

- [ ] **Step 11: Smoke test**

Restart server and visit `https://dash.local/dashboard`:

```bash
pkill -f bin/analytics 2>/dev/null; sleep 1
DATABASE_URL="postgres://sidneydekoning@localhost:5432/analytics?sslmode=disable" \
JWT_SECRET="55b8fa86529f04fbf54de43cfa221b57795b63166c6cab23881ee9693698ff91" \
JWT_REFRESH_SECRET="73c246e9baeb07f098c8b9c1a5d98e53fcd7d19defaa9af76f39cb0c1c90d03c" \
BASE_URL="https://dash.local" PORT="8090" ENV="development" \
./bin/analytics &
sleep 2
curl -sk https://dash.local/login | grep '<title>'
```

Expected: `<title>Sign in — Analytics</title>`

- [ ] **Step 12: Commit and push**

```bash
git add internal/handler/dashboard.go internal/handler/dashboard_test.go \
        internal/service/dashboard.go internal/service/dashboard_test.go \
        internal/middleware/auth.go \
        templates/layout/dashboard.html templates/partials/ \
        templates/pages/dashboard/ \
        static/css/output.css \
        cmd/server/main.go
git commit -m "feat: add dashboard views (overview, pages, sources, audience, events, funnels)"
git push origin main
```

---

## Self-Review

**Spec coverage check:**

| Requirement | Task |
|---|---|
| Multi-site aggregate dashboard | Task 4 (Aggregate handler) |
| Single-site overview (visitors, pageviews, bounce, duration, chart, top pages, sources) | Task 4 (Overview handler) |
| Top pages / entry / exit pages | Task 1 (GetTopPages) + Task 4 (Pages handler) |
| Traffic sources / channels / referrers | Task 1 (GetTopSources) + Task 4 (Sources handler) |
| Audience (countries, devices, browsers) | Task 1 (GetAudienceByDimension) + Task 4 (Audience handler) |
| Time-series chart (uPlot) | Task 3 (chart-line partial) + Task 4 (marshalTimeSeries) |
| Custom date ranges (today/7d/30d/90d) | Task 2 (DateRange service) |
| Events section | Task 4 (Events handler + template) |
| Funnels section | Task 4 (Funnels handler + template) |
| KPI cards (4 metrics) | Task 3 (stats-cards partial) |
| Dashboard layout + sidebar nav | Task 3 (dashboard.html) |
| Redirect to /account/sites/new when no sites | Task 4 (Aggregate handler) |
| Template function map (formatNumber, formatDuration) | Task 4 (Step 7) |
