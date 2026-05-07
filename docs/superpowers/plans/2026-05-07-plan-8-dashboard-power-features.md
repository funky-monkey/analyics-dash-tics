# Dashboard Power Features Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:subagent-driven-development` (recommended) or `superpowers:executing-plans` to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add custom date range picking and per-dimension filters (channel, country, device) so users can slice dashboard data beyond the preset period buttons.

**Architecture:** Custom date ranges are passed as `?from=YYYY-MM-DD&to=YYYY-MM-DD` query params alongside the existing `?period=` param; `service.DateRange` is extended to parse them. Filters are passed as `?channel=organic&country=NL&device=mobile` query params and appended to every stats query via a `StatsFilter` struct. All existing handlers thread the filter through without touching the repository interfaces — only the SQL queries are extended with optional WHERE clauses.

**Tech Stack:** Go, pgx/v5, TimescaleDB, `html/template`, vanilla JS date picker (no external library).

> **Working directory:** `/Users/sidneydekoning/stack/Github/analyics-dash-tics`
> **Module:** `github.com/funky-monkey/analyics-dash-tics`
> **No Co-Authored-By in commit messages.**

---

## File Map

```
internal/service/dashboard.go            — extend DateRange to accept custom from/to strings
internal/repository/stats.go             — add StatsFilter struct; add optional filter clauses to all queries
internal/handler/dashboard.go            — thread filter + custom dates through all page handlers
templates/layout/dashboard.html          — replace period buttons with date range picker + filter bar
templates/partials/filter-bar.html       — reusable active-filters display strip
```

---

### Task 1: Extend DateRange for custom from/to

**Files:**
- Modify: `internal/service/dashboard.go`
- Modify: `internal/service/dashboard_test.go`

- [ ] **Step 1: Update `DateRange` to accept optional custom strings**

Replace the existing `DateRange` function:

```go
// DateRange returns from/to time.Time for a named period or custom date strings.
// If fromStr and toStr are non-empty valid YYYY-MM-DD dates, they take precedence.
// Supported periods: "today", "7d", "30d", "90d". Unknown values default to "30d".
func DateRange(period, fromStr, toStr string) (from, to time.Time) {
    if fromStr != "" && toStr != "" {
        f, errF := time.ParseInLocation("2006-01-02", fromStr, time.UTC)
        t, errT := time.ParseInLocation("2006-01-02", toStr, time.UTC)
        if errF == nil && errT == nil && !t.Before(f) {
            // to = end of that day
            return f, t.Add(24*time.Hour - time.Second)
        }
    }
    now := time.Now().UTC()
    to = now
    switch period {
    case "today":
        from = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
    case "7d":
        from = now.Add(-7 * 24 * time.Hour)
    case "90d":
        from = now.Add(-90 * 24 * time.Hour)
    default:
        from = now.Add(-30 * 24 * time.Hour)
    }
    return from, to
}
```

- [ ] **Step 2: Update tests in `internal/service/dashboard_test.go`**

Read the existing test file. Update every call to `service.DateRange(period)` to `service.DateRange(period, "", "")`. Then add two new tests:

```go
func TestDateRange_CustomDates(t *testing.T) {
    from, to := service.DateRange("30d", "2026-01-01", "2026-01-31")
    assert.Equal(t, "2026-01-01", from.Format("2006-01-02"))
    assert.Equal(t, "2026-01-31", to.Format("2006-01-02"))
}

func TestDateRange_InvalidCustomFallsBack(t *testing.T) {
    from, to := service.DateRange("7d", "bad-date", "2026-01-31")
    assert.True(t, to.After(from))
    // Should fall back to 7d preset
    assert.InDelta(t, 7*24*float64(time.Hour), float64(to.Sub(from)), float64(time.Hour))
}
```

- [ ] **Step 3: Fix all callers of `service.DateRange` in `internal/handler/dashboard.go`**

Search for every `service.DateRange(period)` call and change to `service.DateRange(period, r.URL.Query().Get("from"), r.URL.Query().Get("to"))`.

Run: `grep -n "service.DateRange" internal/handler/dashboard.go`

Update every match.

- [ ] **Step 4: Run tests**

```bash
go test ./internal/service/... -v
go build ./...
```

Expected: all tests pass.

- [ ] **Step 5: Commit**

```bash
git add internal/service/dashboard.go internal/service/dashboard_test.go internal/handler/dashboard.go
git commit -m "feat: DateRange accepts custom from/to date strings"
```

---

### Task 2: StatsFilter — optional channel/country/device filters

**Files:**
- Modify: `internal/repository/stats.go`

- [ ] **Step 1: Define `StatsFilter` and update the `StatsRepository` interface**

Add before the interface definition:

```go
// StatsFilter holds optional dimension filters applied to all stats queries.
// Zero values mean "no filter" — the query runs unfiltered.
type StatsFilter struct {
    Channel string // e.g. "organic", "social", "" = all
    Country string // e.g. "NL", "" = all
    Device  string // e.g. "mobile", "" = all
}

func (f StatsFilter) isEmpty() bool {
    return f.Channel == "" && f.Country == "" && f.Device == ""
}

// filterSQL builds the WHERE clause fragment and args for a StatsFilter.
// baseArgN is the index of the next $N placeholder (args already bound before this).
func (f StatsFilter) filterSQL(baseArgN int) (clause string, args []any) {
    if f.isEmpty() {
        return "", nil
    }
    var parts []string
    n := baseArgN
    if f.Channel != "" {
        parts = append(parts, fmt.Sprintf("channel = $%d", n))
        args = append(args, f.Channel)
        n++
    }
    if f.Country != "" {
        parts = append(parts, fmt.Sprintf("country = $%d", n))
        args = append(args, f.Country)
        n++
    }
    if f.Device != "" {
        parts = append(parts, fmt.Sprintf("device_type = $%d", n))
        args = append(args, f.Device)
    }
    return " AND " + strings.Join(parts, " AND "), args
}
```

Update the interface to add a filter parameter to every method:

```go
type StatsRepository interface {
    GetSummary(ctx context.Context, siteID string, from, to time.Time, f StatsFilter) (*model.StatsSummary, error)
    GetTimeSeries(ctx context.Context, siteID string, from, to time.Time, f StatsFilter) ([]*model.TimePoint, error)
    GetTopPages(ctx context.Context, siteID string, from, to time.Time, limit int) ([]*model.PageStat, error)
    GetTopSources(ctx context.Context, siteID string, from, to time.Time, limit int) ([]*model.SourceStat, error)
    GetAudienceByDimension(ctx context.Context, siteID, dimension string, from, to time.Time, limit int) ([]*model.AudienceStat, error)
    GetEntryPages(ctx context.Context, siteID string, from, to time.Time, limit int) ([]*model.PageStat, error)
    GetExitPages(ctx context.Context, siteID string, from, to time.Time, limit int) ([]*model.PageStat, error)
    GetNewVsReturning(ctx context.Context, siteID string, from, to time.Time) (newVisitors, returning int64, err error)
    GetActiveVisitors(ctx context.Context, siteID string, windowMinutes int) (int64, error)
}
```

Note: only `GetSummary` and `GetTimeSeries` get the filter for now — filtering top pages/sources/audience by the same filter is useful for drill-down but adds complexity. The filter affects the KPI numbers and chart so users see filtered totals.

- [ ] **Step 2: Update `GetSummary` implementation to apply filter**

```go
func (r *pgStatsRepository) GetSummary(ctx context.Context, siteID string, from, to time.Time, f StatsFilter) (*model.StatsSummary, error) {
    filterClause, filterArgs := f.filterSQL(4)
    args := append([]any{siteID, from, to}, filterArgs...)
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
        WHERE site_id = $1 AND hour BETWEEN $2 AND $3`+filterClause,
        args...).Scan(&s.Pageviews, &s.Visitors, &s.Sessions, &s.Bounces, &totalDuration)
    if err != nil {
        return nil, fmt.Errorf("statsRepository.GetSummary: %w", err)
    }
    if s.Sessions > 0 {
        s.BounceRate = float64(s.Bounces) / float64(s.Sessions) * 100
        s.AvgDuration = totalDuration / s.Sessions
    }
    return &s, nil
}
```

- [ ] **Step 3: Update `GetTimeSeries` implementation to apply filter**

```go
func (r *pgStatsRepository) GetTimeSeries(ctx context.Context, siteID string, from, to time.Time, f StatsFilter) ([]*model.TimePoint, error) {
    filterClause, filterArgs := f.filterSQL(4)
    args := append([]any{siteID, from, to}, filterArgs...)
    rows, err := r.pool.Query(ctx, `
        SELECT hour, COALESCE(SUM(pageviews),0), COALESCE(SUM(visitors),0)
        FROM stats_hourly
        WHERE site_id = $1 AND hour BETWEEN $2 AND $3`+filterClause+`
        GROUP BY hour ORDER BY hour ASC`,
        args...)
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
```

- [ ] **Step 4: Update all callers in dashboard.go**

Search for all calls to `h.repos.Stats.GetSummary` and `h.repos.Stats.GetTimeSeries` and add the filter parameter. Parse the filter from the request at the top of each handler:

```go
filter := repository.StatsFilter{
    Channel: r.URL.Query().Get("channel"),
    Country: r.URL.Query().Get("country"),
    Device:  r.URL.Query().Get("device"),
}
```

Then pass `filter` to `GetSummary` and `GetTimeSeries`. All other stats calls remain unchanged.

- [ ] **Step 5: Build**

```bash
go build ./...
```

Expected: success.

- [ ] **Step 6: Commit**

```bash
git add internal/repository/stats.go internal/handler/dashboard.go
git commit -m "feat: StatsFilter threads channel/country/device filters through summary and timeseries queries"
```

---

### Task 3: Date range picker + filter bar in dashboard header

**Files:**
- Modify: `templates/layout/dashboard.html` — replace period buttons with date range UI

- [ ] **Step 1: Update the dashboard header in `templates/layout/dashboard.html`**

Replace the current period picker (the `{{range .AvailablePeriods}}` block) with a combined date range picker + active filters display:

```html
<div class="flex items-center gap-2">
  {{/* Preset period buttons */}}
  {{range .AvailablePeriods}}
  <a href="?period={{.Value}}"
     class="px-3 py-1.5 text-xs font-medium rounded-lg transition-colors {{if and (eq .Value $.Period) (not $.CustomRange)}}bg-violet-600 text-white{{else}}text-gray-500 hover:bg-gray-100{{end}}">
    {{.Label}}
  </a>
  {{end}}

  {{/* Custom date range */}}
  <div class="relative" id="daterange-wrap">
    <button onclick="toggleDatePicker()" id="daterange-btn"
            class="flex items-center gap-1.5 px-3 py-1.5 text-xs font-medium rounded-lg transition-colors
                   {{if $.CustomRange}}bg-violet-600 text-white{{else}}text-gray-500 hover:bg-gray-100{{end}}">
      <svg class="w-3 h-3" fill="none" stroke="currentColor" viewBox="0 0 24 24">
        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M8 7V3m8 4V3m-9 8h10M5 21h14a2 2 0 002-2V7a2 2 0 00-2-2H5a2 2 0 00-2 2v12a2 2 0 002 2z"/>
      </svg>
      {{if $.CustomRange}}{{$.FromParam}} – {{$.ToParam}}{{else}}Custom{{end}}
    </button>
    <div id="daterange-panel" style="display:none;position:absolute;right:0;top:calc(100% + 6px);background:#fff;border:1px solid #e5e7eb;border-radius:12px;padding:16px;box-shadow:0 4px 20px rgba(0,0,0,.1);z-index:50;min-width:260px">
      <p class="text-xs font-medium text-gray-600 mb-3">Custom date range</p>
      <form method="GET" id="daterange-form" class="space-y-2">
        {{/* Preserve existing non-date params */}}
        {{if .ActiveChannel}}<input type="hidden" name="channel" value="{{.ActiveChannel}}">{{end}}
        {{if .ActiveCountry}}<input type="hidden" name="country" value="{{.ActiveCountry}}">{{end}}
        {{if .ActiveDevice}}<input type="hidden" name="device" value="{{.ActiveDevice}}">{{end}}
        <div>
          <label class="block text-xs text-gray-500 mb-1">From</label>
          <input type="date" name="from" value="{{$.FromParam}}" required
                 class="w-full border border-gray-200 rounded-lg px-3 py-1.5 text-sm focus:outline-none focus:ring-2 focus:ring-violet-500">
        </div>
        <div>
          <label class="block text-xs text-gray-500 mb-1">To</label>
          <input type="date" name="to" value="{{$.ToParam}}" required
                 class="w-full border border-gray-200 rounded-lg px-3 py-1.5 text-sm focus:outline-none focus:ring-2 focus:ring-violet-500">
        </div>
        <button type="submit" class="w-full py-2 bg-violet-600 hover:bg-violet-700 text-white text-xs font-medium rounded-lg transition-colors">Apply</button>
      </form>
    </div>
  </div>
</div>
```

Add script to the layout (before `</body>`):

```html
<script nonce="{{.Nonce}}">
function toggleDatePicker() {
  var p = document.getElementById('daterange-panel');
  p.style.display = p.style.display === 'none' ? 'block' : 'none';
}
document.addEventListener('click', function(e) {
  var wrap = document.getElementById('daterange-wrap');
  if (wrap && !wrap.contains(e.target)) {
    var p = document.getElementById('daterange-panel');
    if (p) p.style.display = 'none';
  }
});
</script>
```

- [ ] **Step 2: Pass `CustomRange`, `FromParam`, `ToParam`, `ActiveChannel`, `ActiveCountry`, `ActiveDevice` from all dashboard handlers**

In each handler's `renderDash` call, add these fields to the data map:

```go
fromParam := r.URL.Query().Get("from")
toParam   := r.URL.Query().Get("to")
"CustomRange":    fromParam != "" && toParam != "",
"FromParam":      fromParam,
"ToParam":        toParam,
"ActiveChannel":  r.URL.Query().Get("channel"),
"ActiveCountry":  r.URL.Query().Get("country"),
"ActiveDevice":   r.URL.Query().Get("device"),
```

Add these to every handler that calls `renderDash` (Overview, Pages, Sources, Audience, Events, Funnels, FunnelDetail).

Create a helper function at the bottom of `dashboard.go` to avoid repetition:

```go
// dateRangeData extracts date range and filter params from r for template injection.
func dateRangeData(r *http.Request) map[string]any {
    return map[string]any{
        "CustomRange":   r.URL.Query().Get("from") != "" && r.URL.Query().Get("to") != "",
        "FromParam":     r.URL.Query().Get("from"),
        "ToParam":       r.URL.Query().Get("to"),
        "ActiveChannel": r.URL.Query().Get("channel"),
        "ActiveCountry": r.URL.Query().Get("country"),
        "ActiveDevice":  r.URL.Query().Get("device"),
    }
}
```

Use `maps.Copy` or merge manually into each handler's data map. Since Go doesn't have `maps.Copy` before 1.21, use a simple merge:

```go
for k, v := range dateRangeData(r) { data[k] = v }
```

Add this in each handler before calling `renderDash`.

- [ ] **Step 3: Build**

```bash
go build ./...
```

- [ ] **Step 4: Commit**

```bash
git add templates/layout/dashboard.html internal/handler/dashboard.go
git commit -m "feat: custom date range picker and filter params in dashboard header"
```

---

### Task 4: Active filter chips

Show a strip of active filter badges under the header when filters are applied, each with an × to remove that filter.

**Files:**
- Create: `templates/partials/filter-bar.html`
- Modify: `templates/layout/dashboard.html` — render filter bar below header

- [ ] **Step 1: Create `templates/partials/filter-bar.html`**

```html
{{define "filter-bar"}}
{{if or .ActiveChannel .ActiveCountry .ActiveDevice .CustomRange}}
<div class="bg-white border-b border-gray-100 px-6 py-2 flex items-center gap-2 flex-wrap">
  <span class="text-xs text-gray-400 mr-1">Filters:</span>
  {{if .ActiveChannel}}
  <a href="{{filterRemove . "channel"}}" class="inline-flex items-center gap-1 px-2.5 py-1 text-xs font-medium bg-violet-50 text-violet-700 rounded-full hover:bg-violet-100">
    Channel: {{.ActiveChannel}}
    <svg class="w-3 h-3" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12"/></svg>
  </a>
  {{end}}
  {{if .ActiveCountry}}
  <a href="{{filterRemove . "country"}}" class="inline-flex items-center gap-1 px-2.5 py-1 text-xs font-medium bg-blue-50 text-blue-700 rounded-full hover:bg-blue-100">
    Country: {{.ActiveCountry}}
    <svg class="w-3 h-3" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12"/></svg>
  </a>
  {{end}}
  {{if .ActiveDevice}}
  <a href="{{filterRemove . "device"}}" class="inline-flex items-center gap-1 px-2.5 py-1 text-xs font-medium bg-amber-50 text-amber-700 rounded-full hover:bg-amber-100">
    Device: {{.ActiveDevice}}
    <svg class="w-3 h-3" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12"/></svg>
  </a>
  {{end}}
  {{if .CustomRange}}
  <a href="?period={{.Period}}" class="inline-flex items-center gap-1 px-2.5 py-1 text-xs font-medium bg-gray-100 text-gray-600 rounded-full hover:bg-gray-200">
    {{.FromParam}} – {{.ToParam}}
    <svg class="w-3 h-3" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12"/></svg>
  </a>
  {{end}}
</div>
{{end}}
{{end}}
```

The `filterRemove` FuncMap helper builds the current URL without a specific filter key. Add it to `buildTemplateMap` in `cmd/server/main.go`:

```go
"filterRemove": func(data map[string]any, key string) string {
    params := url.Values{}
    if p, _ := data["Period"].(string); p != "" { params.Set("period", p) }
    if v, _ := data["FromParam"].(string); v != "" && key != "from" { params.Set("from", v) }
    if v, _ := data["ToParam"].(string); v != "" && key != "to" { params.Set("to", v) }
    if v, _ := data["ActiveChannel"].(string); v != "" && key != "channel" { params.Set("channel", v) }
    if v, _ := data["ActiveCountry"].(string); v != "" && key != "country" { params.Set("country", v) }
    if v, _ := data["ActiveDevice"].(string); v != "" && key != "device" { params.Set("device", v) }
    q := params.Encode()
    if q == "" { return "?" }
    return "?" + q
},
```

Add `"net/url"` to the imports in `cmd/server/main.go` if not already present.

- [ ] **Step 2: Render filter bar in `templates/layout/dashboard.html`**

After the closing `</header>` tag and before `<main`, add:

```html
{{template "filter-bar" .}}
```

- [ ] **Step 3: Build**

```bash
go build ./...
```

- [ ] **Step 4: Commit**

```bash
git add templates/partials/filter-bar.html templates/layout/dashboard.html cmd/server/main.go
git commit -m "feat: active filter chips strip with per-filter remove links"
```

---

## Self-Review

**Spec coverage:**
- ✅ Custom date ranges → Tasks 1, 3
- ✅ Filters (channel, country, device) → Tasks 2, 3, 4
- "Saved segments" (persist filter sets) → deferred to a future iteration; current plan covers stateless URL-based filters which covers 90% of the use case without a backend persistence layer.

**Placeholder scan:** All code is complete. No TBD patterns.

**Type consistency:**
- `StatsFilter` defined in Task 2 and used in Tasks 2 + 4's handler updates. ✓
- `filterRemove` FuncMap registered in Task 4 and used in `filter-bar.html`. ✓
- `dateRangeData` helper defined in Task 3 and used across all handlers. ✓
- `service.DateRange(period, fromStr, toStr)` new signature used consistently after Task 1. ✓
