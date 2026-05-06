# Analytics SaaS — Design Specification

**Date:** 2026-05-06  
**Status:** Approved — ready for implementation planning  
**Author:** Sidney de Koning (via Claude Code brainstorming session)

---

## 1. Product Overview

A privacy-first web analytics SaaS built for marketers. Targets solo marketers, freelancers, agencies, in-house marketing teams, and SaaS founders — all segments, with pricing tiers per segment.

**Core promise:** Beautiful, Framer-quality analytics that respect user privacy. No cookies. No consent banners. GDPR-compliant by default.

**Design inspiration:** Framer Analytics — clean, minimal, light mode, data-forward.

**Deployment:** Cloud-only SaaS (V1). Self-hosted on a single EU-region server.

---

## 2. Tech Stack

| Layer | Technology | Notes |
|---|---|---|
| Backend | Go | SOLID, DRY, Effective Go — see AGENTS.md |
| Templating | `html/template` (stdlib) | Server-rendered, XSS-safe auto-escaping |
| Interactivity | HTMX + Alpine.js | Loaded via CDN, no build step |
| Styling | Tailwind CSS (standalone CLI) | Single binary, no Node.js |
| Charts | uPlot + Chart.js | uPlot for time-series, Chart.js for pie/bar/donut |
| CMS Editor | Trix (CDN) | Rich text, ~50KB, no dependencies |
| Database | TimescaleDB | PostgreSQL extension — hypertables + continuous aggregates |
| Caching | `go-cache` (in-memory, TTL) | No Redis — single server acceptable |
| Rate limiting | `golang.org/x/time/rate` | Token bucket per IP, in-memory |
| Auth | JWT in HTTP-only cookies | 15min access token, 7-day refresh with rotation |
| HTML sanitisation | `bluemonday` | CMS content before storage and before render |
| Input validation | `go-playground/validator/v10` | Struct tags, whitelist approach |
| Geolocation | MaxMind GeoLite2 (local DB) | IPs never sent to third parties, discarded after lookup |
| CI/CD | GitHub Actions | lint → test → build → rsync → systemctl restart |
| Deployment | systemd on bare server | No Docker |

**No Node.js. No npm. No Docker. No Redis.**

---

## 3. Architecture

### Layer Stack

```
Browser
  └── html/template rendered pages
  └── HTMX for partial updates (hx-get, hx-post, hx-swap)
  └── Alpine.js for UI state (dropdowns, modals, tabs)
  └── uPlot + Chart.js for charts

Go HTTP Server (net/http + chi router)
  └── Middleware: SecurityHeaders, Auth, CSRF, RateLimit, Logger
  └── Handler layer — thin, delegates to service
  └── Service layer — all business logic
  └── Repository layer — all SQL (pgx/v5 parameterised queries)

TimescaleDB
  └── Hypertables: events (partitioned by timestamp, 1-day chunks)
  └── Continuous aggregates: stats_hourly, page_stats_daily, source_stats_daily
  └── Regular tables: users, sites, funnels, ab_tests, cms_pages, etc.
```

### Request Flows

**Tracking event (hot path):**
```
Visitor browser → POST /collect → rate limiter → validate site token
→ async goroutine writes to TimescaleDB → 202 response (before DB write)
```
Target: sub-5ms response time. DB write is fire-and-forget via goroutine.

**Dashboard page load:**
```
User browser → auth middleware (JWT cookie) → handler → service
→ repository queries continuous aggregates → html/template render → HTML response
```
Target: sub-50ms. Continuous aggregates prevent full-table scans.

**HTMX partial update (e.g. date range change):**
```
HTMX request with hx-headers (CSRF token) → same handler path
→ renders partial template fragment → HTMX swaps DOM element
```

### Project Layout

```
cmd/
  server/
    main.go               — wires all dependencies, starts server

internal/
  handler/
    dashboard.go          — dashboard page handlers
    collect.go            — /collect event ingestion
    auth.go               — login, logout, signup, password reset
    sites.go              — site management
    admin.go              — admin panel handlers
    cms.go                — CMS page handlers
    api.go                — Stats API handlers
  service/
    analytics.go          — stats aggregation, channel classification
    funnel.go             — funnel calculation
    abtest.go             — A/B test stats + significance
    channel.go            — AI traffic detection, channel grouping
    auth.go               — JWT issue/validate, password hashing
    cms.go                — CMS business logic, slug generation
  repository/
    event.go              — event writes + queries
    site.go               — site CRUD
    user.go               — user CRUD
    funnel.go             — funnel + steps CRUD
    abtest.go             — A/B test + variant CRUD
    cms.go                — CMS pages + layouts CRUD
    audit.go              — audit log writes
  model/
    event.go              — domain types (no methods, pure data)
    site.go
    user.go
    funnel.go
    abtest.go
    cms.go
  middleware/
    auth.go               — JWT validation, role enforcement
    security.go           — security headers, CORS, CSRF
    ratelimit.go          — per-IP token bucket
    logger.go             — structured request logging

config/
  config.go               — env var struct, loaded once at startup

migrations/
  001_init.sql
  002_hypertables.sql
  003_continuous_aggregates.sql
  004_funnels.sql
  005_abtests.sql
  006_cms.sql
  007_audit_log.sql

templates/
  layout/
    base.html             — shell, nav, head (Tailwind, HTMX, Alpine CDN links)
  partials/               — HTMX fragment templates
  pages/
    dashboard.html
    overview.html
    pages.html
    sources.html
    audience.html
    events.html
    funnels.html
    abtest.html
    keywords.html
    performance.html
    settings.html
    admin/
      index.html
      users.html
      sites.html
      cms.html
      cms-edit.html
      audit.html
    auth/
      login.html
      signup.html
      forgot-password.html
      reset-password.html
    public/
      home.html
      blog-list.html
      blog-post.html
      page.html

static/
  ts/                     — TypeScript source (compiled to static/js/)
    lib/
      fetch.ts            — typed fetch wrapper
      format.ts           — number/date formatters
      types.ts            — shared types
    charts/
      timeseries.ts       — uPlot wrapper
      pie.ts              — Chart.js donut/pie
      bar.ts              — Chart.js bar
    components/
      datepicker.ts       — Alpine.js date range picker
      tooltip.ts
    pages/
      overview.ts         — page init for dashboard overview
      funnels.ts
      abtest.ts
  css/
    input.css             — Tailwind source (@tailwind directives)
    output.css            — compiled (committed, generated in CI)
  js/                     — compiled TS output
  script.js               — tracking snippet (~1KB, vanilla JS)
```

---

## 4. Auth & Security

Full security rules are in `AGENTS.md`. Key decisions:

### Auth Hierarchy

| Role | Access |
|---|---|
| **Super Admin** | Full system: manage all users, CMS, config, view all sites, audit log |
| **User** | Their own sites only: dashboard, funnels, A/B tests, team invitations |
| **Site Member** | One specific site, role: `viewer` or `editor` |

### JWT Implementation
- Access token: 15-minute expiry, HS256, stored in HTTP-only Secure SameSite=Strict cookie
- Refresh token: 7-day expiry, rotated on every use, revocation stored in `revoked_tokens` table
- Claims: `sub` (user UUID), `role`, `jti`, `iat`, `exp`
- Admin role checked on every `/admin/*` request by dedicated middleware — not just at login

### Security Headers (every response)
```
Strict-Transport-Security: max-age=63072000; includeSubDomains; preload
Content-Security-Policy:   default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; connect-src 'self'; frame-ancestors 'none'
X-Content-Type-Options:    nosniff
X-Frame-Options:           DENY
Referrer-Policy:           strict-origin-when-cross-origin
Permissions-Policy:        camera=(), microphone=(), geolocation=()
```

### CORS
- `/collect`: any origin, POST only, no credentials
- `/api/v1/*`: `ALLOWED_ORIGINS` env var, never wildcard on authenticated endpoints
- All other routes: same-origin only

### Privacy
- IP addresses: used for geolocation (MaxMind GeoLite2 local DB) then discarded — never stored
- Visitor/session IDs: hashed fingerprints derived from User-Agent + IP salt — not reversible
- No cookies set on customer sites — tracking is cookieless

---

## 5. Data Model

### Core Tables

**`users`** — id (uuid PK), email (unique), password_hash, role (enum: admin|user), name, is_active, created_at, last_login_at

**`sites`** — id (uuid PK), owner_id (FK → users), name, domain (idx), token (unique, e.g. `tk_a8f3c2b1`), timezone, created_at

**`site_members`** — id, site_id (FK), user_id (FK), role (enum: owner|editor|viewer), invited_at, accepted_at

**`invitations`** — id, site_id (FK), email, token (unique), role, expires_at

**`revoked_tokens`** — jti (PK), user_id, expires_at — cleaned up by background job

### Analytics (TimescaleDB)

**`events` (HYPERTABLE, partitioned by timestamp, 1-day chunks)**

| Column | Type | Notes |
|---|---|---|
| id | bigserial | PK |
| site_id | uuid | IDX — always scoped |
| type | text | `pageview` or `custom` |
| url | text | |
| referrer | text | |
| channel | text | Pre-computed at ingestion — see classification rules below |
| utm_source / medium / campaign | text | |
| country | char(2) | From MaxMind, IP discarded |
| city | text | |
| device_type | text | |
| browser / os / language | text | |
| session_id / visitor_id | text | Hashed, no PII |
| is_bounce | bool | |
| duration_ms | int | |
| props | jsonb | Custom event properties |
| timestamp | timestamptz | PARTITION KEY |

**Continuous Aggregates:**
- `stats_hourly` — site_id, hour, pageviews, sessions, visitors, bounces, total_duration_ms
- `page_stats_daily` — site_id, date, url, pageviews, sessions, entries, exits, avg_duration_ms
- `source_stats_daily` — site_id, date, channel, referrer, utm_source, sessions, pageviews
- `geo_stats_daily` — site_id, date, country, city, sessions
- `device_stats_daily` — site_id, date, device_type, browser, os, sessions

Dashboard queries hit continuous aggregates only — never the raw events table.

**Channel classification rules (applied at ingestion, result stored in `channel` column):**

| Channel | Rule |
|---|---|
| `ai` | Referrer hostname matches: `chatgpt.com`, `chat.openai.com`, `claude.ai`, `perplexity.ai`, `gemini.google.com`, `copilot.microsoft.com`, `you.com`, `phind.com` |
| `organic` | Referrer matches known search engines (Google, Bing, DuckDuckGo, Yahoo, Baidu, Yandex) AND no UTM params |
| `paid` | `utm_medium` is `cpc`, `ppc`, `paid`, `paidsearch`, or `paidsocial` |
| `email` | `utm_medium` is `email` OR referrer matches known email client domains |
| `social` | Referrer matches known social domains (facebook.com, instagram.com, twitter.com, x.com, linkedin.com, tiktok.com, pinterest.com, reddit.com) AND no paid UTM |
| `dark_social` | Referrer is empty AND URL was not the entry page AND no UTM params (direct-to-deep-link pattern) |
| `direct` | Referrer is empty AND page is likely entry (e.g. homepage, /blog root) |
| `referral` | Any other referrer not matching above categories |

### Conversions & Testing

**`goals`** — id, site_id, name, type (pageview|event|outbound), value (URL or event name)

**`funnels`** — id, site_id, name, created_at  
**`funnel_steps`** — id, funnel_id, position, match_type (url|event|goal), value

**`ab_tests`** *(V2)* — id, site_id, name, status (draft|running|completed), goal_id (FK), started_at, ended_at  
**`ab_variants`** *(V2)* — id, ab_test_id, name, traffic_pct, is_control, visitors, conversions

### CMS

**`cms_layouts`** — id, name, template_file (e.g. `blog-post.html`), description

**`cms_pages`** — id, layout_id (FK), author_id (FK → users), title, slug (unique), type (blog|page), content_html (bluemonday-sanitised Trix output), excerpt, cover_image_url, meta_title, meta_description, status (draft|published), published_at, created_at, updated_at

**`cms_tags`** — id, name, slug (unique)  
**`cms_page_tags`** — page_id (FK), tag_id (FK)

### Audit

**`audit_log`** — id, actor_id (FK → users), action (text), resource_type, resource_id, ip_hash, created_at. Written on every admin action.

---

## 6. Route Structure

### Public
| Route | Description |
|---|---|
| `GET /` | Landing page |
| `GET /blog` | Blog listing |
| `GET /blog/:slug` | Blog post (CMS) |
| `GET /p/:slug` | Generic CMS page |
| `GET /script.js` | Tracking snippet |

### Auth
| Route | Description |
|---|---|
| `GET/POST /login` | Login form + authenticate |
| `GET/POST /signup` | Registration |
| `POST /logout` | Clear cookies, revoke refresh token |
| `GET /verify-email/:token` | Email verification |
| `GET/POST /forgot-password` | Request reset |
| `GET/POST /reset-password/:token` | New password (1hr token expiry) |
| `GET /invite/:token` | Accept site invitation |
| `POST /auth/refresh` | Rotate refresh token |

### Tracking
| Route | Description |
|---|---|
| `POST /collect` | Event ingestion — token auth, rate-limited, async write, returns 202 |

### Dashboard (JWT required)
| Route | Description |
|---|---|
| `GET /dashboard` | Aggregate view across all user sites |
| `GET /sites/:id/overview` | Single site: visitors, pageviews, bounce rate, chart, top pages, sources |
| `GET /sites/:id/pages` | Top pages · entry · exit · time on page |
| `GET /sites/:id/sources` | Channels · referrers · UTM · AI traffic · dark social |
| `GET /sites/:id/audience` | Countries · devices · browsers · OS · language · new vs returning |
| `GET /sites/:id/events` | Custom events · goals · outbound · forms · downloads |
| `GET /sites/:id/funnels` | Funnel list |
| `GET /sites/:id/funnels/new` | Funnel builder |
| `GET /sites/:id/funnels/:fid` | Funnel detail — steps + drop-off |
| `GET /sites/:id/abtests` | A/B test list **(V2)** |
| `GET /sites/:id/abtests/new` | Create A/B test **(V2)** |
| `GET /sites/:id/abtests/:tid` | Test results — significance **(V2)** |
| `GET /sites/:id/keywords` | SEO: Ahrefs + Search Console **(V2)** |
| `GET /sites/:id/performance` | PageSpeed + Core Web Vitals **(V2)** |
| `GET /sites/:id/settings` | Site config, token, team members |
| `GET /account` | User profile + password change |
| `GET/POST /account/sites/new` | Register new site |

### Admin (/admin — role=admin only)
| Route | Description |
|---|---|
| `GET /admin` | System overview: user count, event volume, health |
| `GET /admin/users` | User list |
| `GET/POST /admin/users/new` | Create user (only path to create users) |
| `GET/PUT /admin/users/:id` | Edit user |
| `DELETE /admin/users/:id` | Deactivate (soft delete) |
| `GET /admin/sites` | All registered sites |
| `GET /admin/cms` | CMS page list |
| `GET/POST /admin/cms/new` | Create page — Trix editor + layout picker |
| `GET/PUT /admin/cms/:id/edit` | Edit page |
| `POST /admin/cms/:id/publish` | Publish / unpublish |
| `GET /admin/config` | System configuration |
| `GET /admin/audit-log` | All admin actions |

### Stats API (/api/v1 — Bearer token)
| Route | Description |
|---|---|
| `GET /api/v1/sites` | List user's sites |
| `GET /api/v1/sites/:id/stats` | Summary stats (date range param) |
| `GET /api/v1/sites/:id/pages` | Top pages |
| `GET /api/v1/sites/:id/sources` | Traffic sources + channels |
| `GET /api/v1/sites/:id/events` | Custom events |
| `GET /api/v1/sites/:id/funnels` | Funnel list + results |

---

## 7. Tracking Script

~1KB vanilla JS snippet. Users embed once, no framework required:

```html
<script src="https://yourdomain.com/script.js" data-site="tk_a8f3c2b1" async></script>
```

**Behaviour:**
- Fires a pageview event on load
- Detects SPA navigation (history.pushState / popstate) and fires additional pageviews
- Sends events via `navigator.sendBeacon` (non-blocking) with POST to `/collect`
- Respects `Do Not Track` header — sends no data if set
- Detects and classifies AI tool referrers (ChatGPT, Claude, Perplexity, Gemini, Copilot) at ingestion
- Cookie-free — no document.cookie reads or writes

**Payload (JSON):**
```json
{
  "site": "tk_a8f3c2b1",
  "type": "pageview",
  "url": "https://customer.com/pricing",
  "referrer": "https://news.ycombinator.com",
  "width": 1440
}
```

---

## 8. Feature Roadmap

### V1 — Foundation (months 1–3)
Goal: Ship something better than GA on day one.

- Cookie-free tracking script, GDPR/CCPA compliant, EU hosting
- Bot & spam filtering, IP geolocation (MaxMind local), IP discarded
- Core metrics: pageviews, unique visitors, sessions, bounce rate, session duration, real-time view
- Traffic sources: referrers, UTM, auto channel grouping, AI tool traffic detection, dark social
- Content: top pages, entry pages, exit pages, time on page
- Audience: countries, cities, devices, browsers, OS, language, new vs returning visitors
- Events: custom events, goals, outbound links, file downloads, form completion tracking
- Funnels with drop-off analysis
- Multi-site aggregate dashboard
- Custom date ranges, saved segments, filters
- Email reports (scheduled)
- Multi-user team management, role-based access (owner / editor / viewer)
- Site token management
- Stats API
- Admin panel: user management, system overview, audit log
- CMS: blog + pages with Trix editor, layout templates, tags, SEO meta fields

### V2 — Power Tools (months 4–7)
Goal: Become indispensable for serious marketers.

- A/B testing: variant creation, traffic splitting, statistical significance
- Ahrefs API integration: keyword data in dashboard
- Google Search Console integration: search queries + keyword ranking
- Google PageSpeed API: scores + Core Web Vitals history per page
- Ecommerce & revenue tracking
- Slack reports
- Google Analytics data import
- Raw data export (CSV / JSON)
- Webhooks
- Uptime monitoring
- Audit log enhancements
- IP range exclusion
- GDPR data deletion request handling

### V3 — Full Stack (months 8–14)
Goal: Single source of truth for the entire marketing operation.

- Social channel integrations: Meta Ads, Instagram, LinkedIn, X/Twitter, TikTok
- Unified web + social aggregate dashboard
- Cross-channel attribution
- Custom dashboard builder
- AI intelligence layer: anomaly detection, NL queries, automated insight summaries, trend forecasting
- SSO (SAML / OIDC) for Enterprise
- White-label for agencies
- Data warehouse export (BigQuery)
- SLA & dedicated support tier

---

## 9. Deployment Pipeline

### GitHub Actions CI/CD

```
on: push to main

jobs:
  ci:
    - go vet ./...
    - golangci-lint run        (includes gosec, staticcheck, errcheck)
    - govulncheck ./...
    - go test -race ./...
    - tailwindcss build        (standalone CLI binary, generates output.css)
    - go build -o analytics ./cmd/server

  deploy:
    - rsync binary to server via SSH
    - rsync static/ and templates/ to server
    - systemctl restart analytics
```

### Server Setup
- Go binary runs as a `systemd` service: `analytics.service`
- TimescaleDB installed as a PostgreSQL extension on the server
- Reverse proxy (Caddy or nginx) handles TLS termination, HTTP→HTTPS redirect, HSTS
- MaxMind GeoLite2 database updated monthly via cron

### Environment Variables (never committed)
```
DATABASE_URL          — TimescaleDB connection string
JWT_SECRET            — 32+ byte random key (crypto/rand)
JWT_REFRESH_SECRET    — separate 32+ byte key
ALLOWED_ORIGINS       — comma-separated for CORS on API
SMTP_HOST / USER / PASS — for email reports and invitations
AHREFS_API_KEY        — V2
GOOGLE_CLIENT_ID/SECRET — Search Console OAuth (V2)
MAXMIND_LICENSE_KEY   — GeoLite2 download
BASE_URL              — https://yourdomain.com
```

---

## 10. Key Design Decisions & Rationale

| Decision | Rationale |
|---|---|
| Go backend | Speed (high-throughput /collect), single binary deploy, no runtime, excellent pgx/v5 driver |
| html/template over React/Next.js | No separate frontend build, no Node.js, XSS-safe by default, Go-native |
| HTMX + Alpine.js | Dynamic dashboard without SPA complexity. HTMX drives server-rendered partials. |
| Tailwind standalone CLI | No Node.js needed, single binary, full design control for Framer-quality UI |
| TimescaleDB over ClickHouse | User preference. Continuous aggregates compensate for slower aggregation. pgx/v5 for both. |
| No Redis | Single server — in-memory cache + rate limiter acceptable. Add Redis in V2 if multi-server. |
| No Docker | Simplicity. Go binary + systemd is sufficient. Add Docker in V3 if needed. |
| Async /collect write | Visitor never waits for DB write. 202 response before write completes. |
| JWT in HTTP-only cookies | Immune to XSS token theft. SameSite=Strict prevents CSRF on cookie reads. |
| Pre-computed channel at ingestion | Dashboard queries never classify — just filter/group by stored channel string. |
| MaxMind GeoLite2 local | IPs never leave the server. Privacy by architecture, not policy. |
| bluemonday on CMS HTML | Two passes (store + render) — defence in depth against stored XSS. |

---

## 11. Out of Scope (V1)

- Mobile app
- Dark mode
- Docker / container orchestration
- Redis
- White-label
- Social channel integrations (planned V3)
- Multivariate A/B testing (planned V2+)
- Session replay / heatmaps (not in roadmap)
- Self-hosted option for customers
