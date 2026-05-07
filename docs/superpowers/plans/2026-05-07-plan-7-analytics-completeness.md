# Analytics Completeness Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:subagent-driven-development` (recommended) or `superpowers:executing-plans` to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Complete the core analytics data set by adding entry/exit page tracking, new-vs-returning visitor detection, and a real-time active-visitors counter.

**Architecture:** Entry/exit pages are derived from raw `events` using window functions at query time (no new aggregate needed for MVP — query runs against the hypertable with proper indexes). New-vs-returning visitors are tracked by storing each `visitor_id`'s first-seen timestamp in a `visitor_first_seen` lookup table written at ingestion time. Real-time active visitors query events from the last 5 minutes directly from the hypertable. All three features are additive — no schema changes to existing tables.

**Tech Stack:** Go, pgx/v5, TimescaleDB hypertable queries, `html/template`, Tailwind CSS (inline styles only — no Tailwind rebuild required).

> **Working directory:** `/Users/sidneydekoning/stack/Github/analyics-dash-tics`
> **Module:** `github.com/funky-monkey/analyics-dash-tics`
> **No Co-Authored-By in commit messages.**

---

## File Map

```
internal/repository/migrations/010_visitor_first_seen.sql   — new lookup table
internal/repository/stats.go                                 — add GetEntryPages, GetExitPages, GetNewVsReturning, GetActiveVisitors
internal/handler/dashboard.go                                — wire new queries into Pages and Audience handlers; add ActiveVisitors handler
templates/pages/dashboard/pages.html                         — add entry/exit tabs
templates/pages/dashboard/audience.html                      — add new-vs-returning row
templates/layout/dashboard.html                              — add real-time visitor dot/count in header
cmd/server/main.go                                           — add GET /sites/{siteID}/active-visitors route
```

---

### Task 1: Migration — visitor_first_seen table

This table maps each `(site_id, visitor_id)` pair to the first timestamp we ever saw them. Written at collect time; never updated.

**Files:**
- Create: `internal/repository/migrations/010_visitor_first_seen.sql`

- [ ] **Step 1: Create the migration file**

```sql
-- 010_visitor_first_seen.sql
-- Tracks the first event timestamp per visitor per site.
-- Used to classify sessions as "new" (first visit today/in period) vs "returning".
CREATE TABLE IF NOT EXISTS visitor_first_seen (
    site_id    UUID        NOT NULL,
    visitor_id TEXT        NOT NULL,
    first_seen TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (site_id, visitor_id)
);

CREATE INDEX IF NOT EXISTS visitor_first_seen_site_first
    ON visitor_first_seen (site_id, first_seen DESC);
```

- [ ] **Step 2: Verify the migration runner picks it up**

The migration runner reads files in alphanumeric order from `internal/repository/migrations/`. File `010_visitor_first_seen.sql` sorts after `009_aggregate_indexes.sql`, so it will run next. No code change needed.

Run: `go build ./...`
Expected: no errors.

- [ ] **Step 3: Commit**

```bash
git add internal/repository/migrations/010_visitor_first_seen.sql
git commit -m "feat: add visitor_first_seen table for new-vs-returning detection"
```

---

### Task 2: Write visitor_first_seen at ingestion

The `CollectService.Process` method already handles event ingestion. After writing the event, also upsert into `visitor_first_seen`.

**Files:**
- Modify: `internal/repository/event.go` — add `UpsertVisitorFirstSeen` to `EventRepository`
- Modify: `internal/service/collect.go` — call `UpsertVisitorFirstSeen` after writing the event

- [ ] **Step 1: Add `UpsertVisitorFirstSeen` to `EventRepository` interface in `internal/repository/event.go`**

Add to the interface:
```go
UpsertVisitorFirstSeen(ctx context.Context, siteID, visitorID string) error
```

Add the implementation after `ListCustomEvents`:
```go
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
```

- [ ] **Step 2: Call `UpsertVisitorFirstSeen` in the collect service**

Read `internal/service/collect.go` to find where `repos.Events.Write` or `repos.Events.WriteBatch` is called. After the event write, add a best-effort upsert (non-blocking fire-and-forget is fine — first-seen data is advisory, not critical):

Find `Process` or the equivalent method. It will look something like:

```go
if err := s.repos.Events.Write(ctx, event); err != nil {
    return fmt.Errorf("collect: write event: %w", err)
}
```

After that line, add:
```go
// Best-effort: ignore errors — first-seen data is advisory.
_ = s.repos.Events.UpsertVisitorFirstSeen(ctx, event.SiteID, event.VisitorID)
```

- [ ] **Step 3: Build**

```bash
go build ./...
```

- [ ] **Step 4: Commit**

```bash
git add internal/repository/event.go internal/service/collect.go
git commit -m "feat: upsert visitor_first_seen on every collected event"
```

---

### Task 3: Stats queries — entry pages, exit pages, new/returning, active visitors

**Files:**
- Modify: `internal/repository/stats.go` — add 4 new methods

- [ ] **Step 1: Add 4 methods to `StatsRepository` interface**

In `internal/repository/stats.go`, extend the interface:

```go
type StatsRepository interface {
    GetSummary(ctx context.Context, siteID string, from, to time.Time) (*model.StatsSummary, error)
    GetTimeSeries(ctx context.Context, siteID string, from, to time.Time) ([]*model.TimePoint, error)
    GetTopPages(ctx context.Context, siteID string, from, to time.Time, limit int) ([]*model.PageStat, error)
    GetTopSources(ctx context.Context, siteID string, from, to time.Time, limit int) ([]*model.SourceStat, error)
    GetAudienceByDimension(ctx context.Context, siteID, dimension string, from, to time.Time, limit int) ([]*model.AudienceStat, error)
    GetEntryPages(ctx context.Context, siteID string, from, to time.Time, limit int) ([]*model.PageStat, error)
    GetExitPages(ctx context.Context, siteID string, from, to time.Time, limit int) ([]*model.PageStat, error)
    GetNewVsReturning(ctx context.Context, siteID string, from, to time.Time) (newVisitors, returning int64, err error)
    GetActiveVisitors(ctx context.Context, siteID string, windowMinutes int) (int64, error)
}
```

- [ ] **Step 2: Implement `GetEntryPages`**

Entry page = the first URL a visitor hits in a session (minimum timestamp per session).

```go
func (r *pgStatsRepository) GetEntryPages(ctx context.Context, siteID string, from, to time.Time, limit int) ([]*model.PageStat, error) {
    rows, err := r.pool.Query(ctx, `
        SELECT url, COUNT(*) AS entries
        FROM (
            SELECT DISTINCT ON (session_id) url
            FROM events
            WHERE site_id = $1 AND type = 'pageview' AND timestamp BETWEEN $2 AND $3
            ORDER BY session_id, timestamp ASC
        ) first_pages
        GROUP BY url
        ORDER BY entries DESC
        LIMIT $4
    `, siteID, from, to, limit)
    if err != nil {
        return nil, fmt.Errorf("statsRepository.GetEntryPages: %w", err)
    }
    defer rows.Close()
    var pages []*model.PageStat
    for rows.Next() {
        p := &model.PageStat{}
        if err := rows.Scan(&p.URL, &p.Sessions); err != nil {
            return nil, fmt.Errorf("statsRepository.GetEntryPages: scan: %w", err)
        }
        pages = append(pages, p)
    }
    return pages, rows.Err()
}
```

- [ ] **Step 3: Implement `GetExitPages`**

Exit page = the last URL in a session (maximum timestamp per session).

```go
func (r *pgStatsRepository) GetExitPages(ctx context.Context, siteID string, from, to time.Time, limit int) ([]*model.PageStat, error) {
    rows, err := r.pool.Query(ctx, `
        SELECT url, COUNT(*) AS exits
        FROM (
            SELECT DISTINCT ON (session_id) url
            FROM events
            WHERE site_id = $1 AND type = 'pageview' AND timestamp BETWEEN $2 AND $3
            ORDER BY session_id, timestamp DESC
        ) last_pages
        GROUP BY url
        ORDER BY exits DESC
        LIMIT $4
    `, siteID, from, to, limit)
    if err != nil {
        return nil, fmt.Errorf("statsRepository.GetExitPages: %w", err)
    }
    defer rows.Close()
    var pages []*model.PageStat
    for rows.Next() {
        p := &model.PageStat{}
        if err := rows.Scan(&p.URL, &p.Sessions); err != nil {
            return nil, fmt.Errorf("statsRepository.GetExitPages: scan: %w", err)
        }
        pages = append(pages, p)
    }
    return pages, rows.Err()
}
```

- [ ] **Step 4: Implement `GetNewVsReturning`**

New visitor = their `first_seen` timestamp falls within the query period.
Returning visitor = their `first_seen` was before the period, but they have events in the period.

```go
func (r *pgStatsRepository) GetNewVsReturning(ctx context.Context, siteID string, from, to time.Time) (newVisitors, returning int64, err error) {
    err = r.pool.QueryRow(ctx, `
        WITH period_visitors AS (
            SELECT DISTINCT visitor_id
            FROM events
            WHERE site_id = $1 AND type = 'pageview' AND timestamp BETWEEN $2 AND $3
        )
        SELECT
            COUNT(*) FILTER (WHERE vfs.first_seen >= $2) AS new_visitors,
            COUNT(*) FILTER (WHERE vfs.first_seen < $2)  AS returning_visitors
        FROM period_visitors pv
        LEFT JOIN visitor_first_seen vfs ON vfs.site_id = $1 AND vfs.visitor_id = pv.visitor_id
    `, siteID, from, to).Scan(&newVisitors, &returning)
    if err != nil {
        return 0, 0, fmt.Errorf("statsRepository.GetNewVsReturning: %w", err)
    }
    return newVisitors, returning, nil
}
```

- [ ] **Step 5: Implement `GetActiveVisitors`**

Active = distinct visitor_ids with an event in the last `windowMinutes` minutes.

```go
func (r *pgStatsRepository) GetActiveVisitors(ctx context.Context, siteID string, windowMinutes int) (int64, error) {
    var count int64
    err := r.pool.QueryRow(ctx, `
        SELECT COUNT(DISTINCT visitor_id)
        FROM events
        WHERE site_id = $1
          AND timestamp >= NOW() - ($2 * INTERVAL '1 minute')
    `, siteID, windowMinutes).Scan(&count)
    if err != nil {
        return 0, fmt.Errorf("statsRepository.GetActiveVisitors: %w", err)
    }
    return count, nil
}
```

- [ ] **Step 6: Build**

```bash
go build ./...
```

Expected: success.

- [ ] **Step 7: Commit**

```bash
git add internal/repository/stats.go
git commit -m "feat: add GetEntryPages, GetExitPages, GetNewVsReturning, GetActiveVisitors to StatsRepository"
```

---

### Task 4: Pages dashboard — entry/exit tabs

The existing `/sites/:id/pages` shows only top pages. Add two more tabs: Entry pages and Exit pages.

**Files:**
- Modify: `internal/handler/dashboard.go` — update `Pages` handler to fetch entry/exit data
- Modify: `templates/pages/dashboard/pages.html` — add tabs

- [ ] **Step 1: Update `Pages` handler in `internal/handler/dashboard.go`**

Replace the existing `Pages` function:

```go
// Pages renders GET /sites/:siteID/pages.
func (h *DashboardHandler) Pages(w http.ResponseWriter, r *http.Request) {
    if h.repos == nil {
        http.NotFound(w, r)
        return
    }
    site, err := resolveSite(r.Context(), h.repos, chi.URLParam(r, "siteID"))
    if err != nil {
        http.NotFound(w, r)
        return
    }
    period := periodParam(r)
    from, to := service.DateRange(period)
    slug := domainSlug(site.Domain)
    tab := r.URL.Query().Get("tab")
    if tab == "" {
        tab = "top"
    }

    var pages, entryPages, exitPages []*model.PageStat
    var wg sync.WaitGroup
    wg.Add(3)
    go func() { defer wg.Done(); pages, _ = h.repos.Stats.GetTopPages(r.Context(), site.ID, from, to, 50) }()
    go func() { defer wg.Done(); entryPages, _ = h.repos.Stats.GetEntryPages(r.Context(), site.ID, from, to, 50) }()
    go func() { defer wg.Done(); exitPages, _ = h.repos.Stats.GetExitPages(r.Context(), site.ID, from, to, 50) }()
    wg.Wait()

    h.renderDash(w, r, "pages.html", map[string]any{
        "SiteID": slug, "SiteDomain": site.Domain,
        "SiteBaseURL": "/sites/" + slug, "ActiveNav": "pages",
        "Period": period, "AvailablePeriods": periodsAvailable,
        "Tab": tab, "Pages": pages, "EntryPages": entryPages, "ExitPages": exitPages,
    })
}
```

- [ ] **Step 2: Replace `templates/pages/dashboard/pages.html`**

```html
{{template "dashboard.html" .}}
{{define "title"}}{{.SiteDomain}} — Pages{{end}}
{{define "content"}}
<h2 class="text-lg font-semibold text-gray-900 mb-5">Pages</h2>
<div class="flex gap-6 items-start">
  <div class="flex-1 min-w-0">

    {{/* Tabs */}}
    <div class="flex gap-1 mb-4 bg-gray-100 rounded-lg p-1 w-fit">
      <a href="?period={{.Period}}&tab=top"
         class="px-4 py-1.5 text-xs font-medium rounded-md transition-colors {{if eq .Tab "top"}}bg-white text-gray-900 shadow-sm{{else}}text-gray-500 hover:text-gray-700{{end}}">
        Top pages
      </a>
      <a href="?period={{.Period}}&tab=entry"
         class="px-4 py-1.5 text-xs font-medium rounded-md transition-colors {{if eq .Tab "entry"}}bg-white text-gray-900 shadow-sm{{else}}text-gray-500 hover:text-gray-700{{end}}">
        Entry pages
      </a>
      <a href="?period={{.Period}}&tab=exit"
         class="px-4 py-1.5 text-xs font-medium rounded-md transition-colors {{if eq .Tab "exit"}}bg-white text-gray-900 shadow-sm{{else}}text-gray-500 hover:text-gray-700{{end}}">
        Exit pages
      </a>
    </div>

    <div class="bg-white rounded-xl border border-gray-100 overflow-hidden">
      {{if eq .Tab "entry"}}
      <table class="w-full text-sm">
        <thead class="bg-gray-50"><tr class="text-xs text-gray-400 border-b border-gray-100">
          <th class="text-left px-5 py-3 font-medium">Entry page</th>
          <th class="text-right px-5 py-3 font-medium">Entrances</th>
        </tr></thead>
        <tbody>
          {{range .EntryPages}}
          <tr class="border-b border-gray-50 last:border-0 hover:bg-gray-50">
            <td class="px-5 py-3 font-mono text-xs text-gray-600 truncate max-w-sm">{{.URL}}</td>
            <td class="px-5 py-3 text-right font-medium text-gray-900">{{formatNumber .Sessions}}</td>
          </tr>
          {{else}}
          <tr><td colspan="2" class="px-5 py-10 text-center text-gray-400 text-sm">No data for this period.</td></tr>
          {{end}}
        </tbody>
      </table>

      {{else if eq .Tab "exit"}}
      <table class="w-full text-sm">
        <thead class="bg-gray-50"><tr class="text-xs text-gray-400 border-b border-gray-100">
          <th class="text-left px-5 py-3 font-medium">Exit page</th>
          <th class="text-right px-5 py-3 font-medium">Exits</th>
        </tr></thead>
        <tbody>
          {{range .ExitPages}}
          <tr class="border-b border-gray-50 last:border-0 hover:bg-gray-50">
            <td class="px-5 py-3 font-mono text-xs text-gray-600 truncate max-w-sm">{{.URL}}</td>
            <td class="px-5 py-3 text-right font-medium text-gray-900">{{formatNumber .Sessions}}</td>
          </tr>
          {{else}}
          <tr><td colspan="2" class="px-5 py-10 text-center text-gray-400 text-sm">No data for this period.</td></tr>
          {{end}}
        </tbody>
      </table>

      {{else}}
      <table class="w-full text-sm">
        <thead class="bg-gray-50"><tr class="text-xs text-gray-400 border-b border-gray-100">
          <th class="text-left px-5 py-3 font-medium">Page</th>
          <th class="text-right px-5 py-3 font-medium">Pageviews</th>
          <th class="text-right px-5 py-3 font-medium">Sessions</th>
          <th class="text-right px-5 py-3 font-medium">Avg. time</th>
        </tr></thead>
        <tbody>
          {{range .Pages}}
          <tr class="border-b border-gray-50 last:border-0 hover:bg-gray-50">
            <td class="px-5 py-3 font-mono text-xs text-gray-600 truncate max-w-sm">{{.URL}}</td>
            <td class="px-5 py-3 text-right font-medium text-gray-900">{{formatNumber .Pageviews}}</td>
            <td class="px-5 py-3 text-right text-gray-500">{{formatNumber .Sessions}}</td>
            <td class="px-5 py-3 text-right text-gray-500">{{formatDuration .AvgDuration}}</td>
          </tr>
          {{else}}
          <tr><td colspan="4" class="px-5 py-10 text-center text-gray-400 text-sm">No page data for this period.</td></tr>
          {{end}}
        </tbody>
      </table>
      {{end}}
    </div>
  </div>
  {{template "help-card" dict "Title" "Pages report" "Icon" "page" "Body" "Three views: Top pages by pageviews, Entry pages (first page of each session), and Exit pages (last page of each session)." "Items" (list (helpItem "Top pages" "All pages ranked by total pageviews in the period.") (helpItem "Entry pages" "The first page a visitor lands on — shows which pages bring people into your site.") (helpItem "Exit pages" "The last page before a visitor leaves — high exit rates can indicate friction or dead ends.") (helpItem "Avg. time" "Average time on page across all sessions that included this page."))}}
</div>
{{end}}
```

- [ ] **Step 3: Build**

```bash
go build ./...
```

- [ ] **Step 4: Commit**

```bash
git add internal/handler/dashboard.go templates/pages/dashboard/pages.html
git commit -m "feat: pages report with entry/exit tabs"
```

---

### Task 5: Audience dashboard — new vs returning

**Files:**
- Modify: `internal/handler/dashboard.go` — update `Audience` handler
- Modify: `templates/pages/dashboard/audience.html` — add new/returning card

- [ ] **Step 1: Update `Audience` handler to fetch new/returning data**

In the existing `Audience` handler, add to the parallel queries block:

```go
var newVisitors, returningVisitors int64
```

Add a goroutine:
```go
run(func() {
    newVisitors, returningVisitors, _ = h.repos.Stats.GetNewVsReturning(r.Context(), site.ID, from, to)
})
```

Wait, the `Audience` handler doesn't use the `run`/`wg` pattern yet — it makes sequential calls. Add `sync.WaitGroup` parallelism following the same pattern as the Overview handler (see `internal/handler/dashboard.go` lines ~98-175 for the pattern). The simplest approach for Audience: add a goroutine for `GetNewVsReturning` alongside the existing three audience calls.

Full updated `Audience` function:

```go
// Audience renders GET /sites/:siteID/audience.
func (h *DashboardHandler) Audience(w http.ResponseWriter, r *http.Request) {
    if h.repos == nil {
        http.NotFound(w, r)
        return
    }
    site, err := resolveSite(r.Context(), h.repos, chi.URLParam(r, "siteID"))
    if err != nil {
        http.NotFound(w, r)
        return
    }
    period := periodParam(r)
    from, to := service.DateRange(period)
    slug := domainSlug(site.Domain)

    var (
        countries         []*model.AudienceStat
        devices           []*model.AudienceStat
        browsers          []*model.AudienceStat
        newVisitors       int64
        returningVisitors int64
        wg                sync.WaitGroup
    )
    wg.Add(4)
    go func() { defer wg.Done(); countries, _ = h.repos.Stats.GetAudienceByDimension(r.Context(), site.ID, "country", from, to, 20) }()
    go func() { defer wg.Done(); devices, _ = h.repos.Stats.GetAudienceByDimension(r.Context(), site.ID, "device_type", from, to, 5) }()
    go func() { defer wg.Done(); browsers, _ = h.repos.Stats.GetAudienceByDimension(r.Context(), site.ID, "browser", from, to, 10) }()
    go func() {
        defer wg.Done()
        newVisitors, returningVisitors, _ = h.repos.Stats.GetNewVsReturning(r.Context(), site.ID, from, to)
    }()
    wg.Wait()

    h.renderDash(w, r, "audience.html", map[string]any{
        "SiteID": slug, "SiteDomain": site.Domain,
        "SiteBaseURL": "/sites/" + slug, "ActiveNav": "audience",
        "Period": period, "AvailablePeriods": periodsAvailable,
        "Countries":  countries, "Devices": devices, "Browsers": browsers,
        "NewVisitors": newVisitors, "ReturningVisitors": returningVisitors,
    })
}
```

- [ ] **Step 2: Add new/returning card to `templates/pages/dashboard/audience.html`**

In the existing 3-column grid in `audience.html`, add a fourth card after Browsers:

```html
<div class="bg-white rounded-xl border border-gray-100 p-5">
  <h3 class="text-xs font-semibold text-gray-500 uppercase tracking-wide mb-4">New vs returning</h3>
  {{$total := add .NewVisitors .ReturningVisitors}}
  {{if gt $total 0}}
  <div class="space-y-3">
    <div>
      <div class="flex items-center justify-between mb-1">
        <span class="text-xs text-gray-700">New visitors</span>
        <span class="text-xs font-semibold text-gray-800">{{formatNumber .NewVisitors}}</span>
      </div>
      <div class="w-full bg-gray-100 rounded-full overflow-hidden" style="height:6px">
        <div class="h-full rounded-full bg-violet-500" style="width:{{pct .NewVisitors $total}}%"></div>
      </div>
    </div>
    <div>
      <div class="flex items-center justify-between mb-1">
        <span class="text-xs text-gray-700">Returning visitors</span>
        <span class="text-xs font-semibold text-gray-800">{{formatNumber .ReturningVisitors}}</span>
      </div>
      <div class="w-full bg-gray-100 rounded-full overflow-hidden" style="height:6px">
        <div class="h-full rounded-full bg-violet-300" style="width:{{pct .ReturningVisitors $total}}%"></div>
      </div>
    </div>
  </div>
  {{else}}
  <p class="text-xs text-gray-400 py-3 text-center">No data yet.</p>
  {{end}}
</div>
```

This template uses two new FuncMap helpers: `add` and `pct`. Add them to `buildTemplateMap` in `cmd/server/main.go`:

```go
"add": func(a, b int64) int64 { return a + b },
"pct": func(part, total int64) int64 {
    if total == 0 { return 0 }
    return part * 100 / total
},
```

- [ ] **Step 3: Build**

```bash
go build ./...
```

- [ ] **Step 4: Commit**

```bash
git add internal/handler/dashboard.go templates/pages/dashboard/audience.html cmd/server/main.go
git commit -m "feat: audience page adds new vs returning visitors"
```

---

### Task 6: Real-time active visitors in header

Show a live "N active now" count in the dashboard header, updating every 30 seconds via a small fetch poll.

**Files:**
- Modify: `cmd/server/main.go` — add GET `/sites/{siteID}/active-visitors` JSON endpoint
- Modify: `internal/handler/dashboard.go` — add `ActiveVisitors` handler
- Modify: `templates/layout/dashboard.html` — add live count to header

- [ ] **Step 1: Add `ActiveVisitors` handler to `internal/handler/dashboard.go`**

```go
// ActiveVisitors handles GET /sites/:siteID/active-visitors — returns JSON count
// of distinct visitors who sent an event in the last 5 minutes.
func (h *DashboardHandler) ActiveVisitors(w http.ResponseWriter, r *http.Request) {
    if h.repos == nil {
        w.Header().Set("Content-Type", "application/json")
        w.Write([]byte(`{"active":0}`)) //nolint:errcheck
        return
    }
    site, err := resolveSite(r.Context(), h.repos, chi.URLParam(r, "siteID"))
    if err != nil {
        w.Header().Set("Content-Type", "application/json")
        w.Write([]byte(`{"active":0}`)) //nolint:errcheck
        return
    }
    count, _ := h.repos.Stats.GetActiveVisitors(r.Context(), site.ID, 5)
    w.Header().Set("Content-Type", "application/json")
    fmt.Fprintf(w, `{"active":%d}`, count) //nolint:errcheck
}
```

- [ ] **Step 2: Register the route in `cmd/server/main.go`**

In the authenticated routes group, after the existing site routes, add:

```go
r.Get("/sites/{siteID}/active-visitors", dashHandler.ActiveVisitors)
```

- [ ] **Step 3: Update dashboard header in `templates/layout/dashboard.html`**

In the header, replace:

```html
<span class="w-2 h-2 bg-green-400 rounded-full inline-block"></span>
{{.SiteDomain}}
```

With:

```html
<span id="active-dot" class="w-2 h-2 bg-green-400 rounded-full inline-block"></span>
{{.SiteDomain}}
<span class="text-xs text-gray-400 font-normal" id="active-count" style="display:none"></span>
```

Add at the bottom of the body (before `</body>`), after the existing scripts:

```html
<script>
(function() {
  var siteBase = '{{.SiteBaseURL}}';
  if (!siteBase || siteBase === '/dashboard') return;
  function poll() {
    fetch(siteBase + '/active-visitors')
      .then(function(r) { return r.json(); })
      .then(function(d) {
        var el = document.getElementById('active-count');
        if (!el) return;
        if (d.active > 0) {
          el.textContent = d.active + ' active now';
          el.style.display = '';
        } else {
          el.style.display = 'none';
        }
      })
      .catch(function() {});
  }
  poll();
  setInterval(poll, 30000);
})();
</script>
```

Note: this script is inline in the layout and uses `{{.SiteBaseURL}}` from template data. Since the layout itself needs the nonce, add `nonce="{{.Nonce}}"` to the script tag.

- [ ] **Step 4: Build**

```bash
go build ./...
```

- [ ] **Step 5: Commit**

```bash
git add internal/handler/dashboard.go cmd/server/main.go templates/layout/dashboard.html
git commit -m "feat: real-time active visitors counter in dashboard header"
```

---

## Self-Review

**Spec coverage:**
- ✅ Entry pages → Task 4
- ✅ Exit pages → Task 4
- ✅ New vs returning visitors → Tasks 1, 2, 5
- ✅ Real-time view → Task 6

**Placeholder scan:** All steps contain actual code. No TBD or "implement later".

**Type consistency:**
- `model.PageStat.Sessions` is used for entry/exit counts in Task 3/4 — `Sessions` field exists on `PageStat` (it's the `int64` field already in the struct). The `GetEntryPages`/`GetExitPages` queries scan into `p.URL` and `p.Sessions`. ✓
- `GetNewVsReturning` returns `(newVisitors, returning int64, err error)` — used exactly that way in Task 5. ✓
- `add` and `pct` FuncMap functions added in Task 5 and used in the template. ✓
- `ActiveVisitors` handler uses `resolveSite` + `domainSlug` — consistent with all other dashboard handlers. ✓
