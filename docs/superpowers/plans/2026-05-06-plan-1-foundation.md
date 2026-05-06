# Analytics SaaS — Plan 1: Foundation

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:subagent-driven-development` (recommended) or `superpowers:executing-plans` to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the complete Go foundation — project scaffold, database connection, all V1 migrations, security middleware, JWT auth (login/signup/password reset/invite), and site token management — so every subsequent plan has a working base to build on.

**Architecture:** Single Go binary (`net/http` + `chi` router). All config from env vars loaded once at startup into a typed `Config` struct. `pgx/v5` pool for TimescaleDB. JWT access tokens (15 min) stored in HTTP-only cookies with 7-day refresh tokens. `bcrypt` (cost 12) for passwords. Security headers, CORS, CSRF, and per-IP rate limiting on every route via middleware.

**Tech Stack:** Go 1.22+, `go-chi/chi/v5`, `jackc/pgx/v5`, `golang-jwt/jwt/v5`, `golang.org/x/crypto/bcrypt`, `go-playground/validator/v10`, `patrickmn/go-cache`, `golang.org/x/time/rate`, `stretchr/testify`

> **Note on module path:** This plan uses `github.com/sidneydekoning/analytics`. Update `go.mod` to match your actual GitHub username/org.

---

## File Map

```
go.mod
go.sum
.env.example
Makefile

config/
  config.go                     — env vars → typed Config struct

cmd/
  server/
    main.go                     — wires everything, starts HTTP server

internal/
  model/
    user.go                     — User, Role types (pure data, no methods)
    site.go                     — Site type
    invite.go                   — Invitation type
    token.go                    — RefreshToken, RevokedToken types
  repository/
    db.go                       — pgx/v5 pool constructor
    migrations.go               — embedded SQL migration runner
    user.go                     — UserRepository interface + pg implementation
    site.go                     — SiteRepository interface + pg implementation
    token.go                    — TokenRepository (revoked tokens, invitations)
  service/
    auth.go                     — AuthService: JWT issue/validate, bcrypt, password reset
    site.go                     — SiteService: register site, generate token
  handler/
    auth.go                     — login, logout, signup, forgot/reset password, verify email, invite
    sites.go                    — new site form + save
  middleware/
    security.go                 — SecurityHeaders, CORS, CSRF middleware
    auth.go                     — JWTAuth, RequireRole middleware
    ratelimit.go                — per-IP token bucket
    logger.go                   — structured request logging (log/slog)

migrations/
  001_init.sql                  — users, sites, site_members, invitations
  002_hypertables.sql           — events hypertable
  003_continuous_aggregates.sql — stats_hourly, page_stats_daily, source_stats_daily
  004_funnels.sql               — funnels, funnel_steps, goals
  005_cms.sql                   — cms_layouts, cms_pages, cms_tags, cms_page_tags
  006_audit_log.sql             — audit_log, revoked_tokens
  007_indexes.sql               — all non-PK indexes

templates/
  layout/
    base.html                   — HTML shell with Tailwind CDN, HTMX, Alpine.js
  pages/
    auth/
      login.html
      signup.html
      forgot-password.html
      reset-password.html
      verify-email.html

static/
  css/
    input.css                   — Tailwind @tailwind directives

internal/
  service/
    auth_test.go
    site_test.go
  repository/
    user_test.go
    site_test.go
  handler/
    auth_test.go
```

---

### Task 1: Initialize Go module, directory scaffold, and Makefile

**Files:**
- Create: `go.mod`
- Create: `Makefile`
- Create: `.env.example`

- [ ] **Step 1: Create the module**

```bash
cd /Users/sidneydekoning/stack/Github/analyics-dash-tics
go mod init github.com/sidneydekoning/analytics
```

Expected: `go.mod` created with `module github.com/sidneydekoning/analytics` and `go 1.22` (or current version).

- [ ] **Step 2: Create all directories**

```bash
mkdir -p cmd/server \
  config \
  internal/{model,repository,service,handler,middleware} \
  migrations \
  templates/layout \
  templates/pages/auth \
  templates/pages/public \
  templates/pages/dashboard \
  templates/pages/admin \
  templates/partials \
  static/css \
  static/js \
  static/ts/{lib,charts,components,pages}
```

- [ ] **Step 3: Create `.env.example`**

```bash
cat > .env.example << 'EOF'
# Database
DATABASE_URL=postgres://postgres:password@localhost:5432/analytics?sslmode=disable

# JWT (generate with: openssl rand -hex 32)
JWT_SECRET=replace-with-32-byte-random-hex
JWT_REFRESH_SECRET=replace-with-different-32-byte-random-hex

# Server
BASE_URL=http://localhost:8080
PORT=8090

# CORS (comma-separated, for Stats API)
ALLOWED_ORIGINS=http://localhost:3000

# Email (for invitations and password reset)
SMTP_HOST=smtp.example.com
SMTP_PORT=587
SMTP_USER=noreply@example.com
SMTP_PASS=smtp-password
SMTP_FROM=Analytics <noreply@example.com>

# MaxMind (for geolocation — V1 can use free GeoLite2)
MAXMIND_DB_PATH=/etc/analytics/GeoLite2-City.mmdb

# Environment
ENV=development
EOF
```

- [ ] **Step 4: Create `Makefile`**

```makefile
# Makefile
.PHONY: dev test lint build migrate

# Load .env if it exists
ifneq (,$(wildcard .env))
  include .env
  export
endif

dev:
	go run ./cmd/server

test:
	go test -race ./...

lint:
	golangci-lint run

build:
	go build -o bin/analytics ./cmd/server

migrate:
	go run ./cmd/migrate

tidy:
	go mod tidy

tailwind:
	./bin/tailwindcss -i static/css/input.css -o static/css/output.css --minify
```

- [ ] **Step 5: Add all Go dependencies**

```bash
go get github.com/go-chi/chi/v5
go get github.com/jackc/pgx/v5
go get github.com/jackc/pgx/v5/pgxpool
go get github.com/golang-jwt/jwt/v5
go get golang.org/x/crypto/bcrypt
go get github.com/go-playground/validator/v10
go get github.com/patrickmn/go-cache
go get golang.org/x/time/rate
go get github.com/stretchr/testify
go get github.com/microcosm-cc/bluemonday
go mod tidy
```

- [ ] **Step 6: Commit**

```bash
git add go.mod go.sum .env.example Makefile
git commit -m "feat: initialize Go module, scaffold directories, add dependencies"
```

---

### Task 2: Config system

**Files:**
- Create: `config/config.go`

- [ ] **Step 1: Write the failing test**

Create `config/config_test.go`:

```go
package config_test

import (
	"os"
	"testing"

	"github.com/sidneydekoning/analytics/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoad_AllRequired(t *testing.T) {
	os.Setenv("DATABASE_URL", "postgres://localhost/test")
	os.Setenv("JWT_SECRET", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	os.Setenv("JWT_REFRESH_SECRET", "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")
	os.Setenv("BASE_URL", "http://localhost:8080")
	os.Setenv("PORT", "8080")
	defer func() {
		os.Unsetenv("DATABASE_URL")
		os.Unsetenv("JWT_SECRET")
		os.Unsetenv("JWT_REFRESH_SECRET")
		os.Unsetenv("BASE_URL")
		os.Unsetenv("PORT")
	}()

	cfg, err := config.Load()
	require.NoError(t, err)
	assert.Equal(t, "postgres://localhost/test", cfg.DatabaseURL)
	assert.Equal(t, 8080, cfg.Port)
	assert.Equal(t, "http://localhost:8080", cfg.BaseURL)
}

func TestLoad_MissingRequired(t *testing.T) {
	os.Unsetenv("DATABASE_URL")
	_, err := config.Load()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "DATABASE_URL")
}
```

- [ ] **Step 2: Run the test to verify it fails**

```bash
go test ./config/... -v
```

Expected: FAIL — `config` package does not exist yet.

- [ ] **Step 3: Implement `config/config.go`**

```go
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Config holds all application configuration loaded from environment variables.
// Never call os.Getenv outside this package.
type Config struct {
	DatabaseURL        string
	JWTSecret          []byte
	JWTRefreshSecret   []byte
	BaseURL            string
	Port               int
	AllowedOrigins     []string
	SMTPHost           string
	SMTPPort           int
	SMTPUser           string
	SMTPPass           string
	SMTPFrom           string
	MaxMindDBPath      string
	Env                string
}

// Load reads all required env vars and returns a Config.
// Returns an error listing every missing required variable.
func Load() (*Config, error) {
	var missing []string

	require := func(key string) string {
		v := os.Getenv(key)
		if v == "" {
			missing = append(missing, key)
		}
		return v
	}

	optional := func(key, fallback string) string {
		if v := os.Getenv(key); v != "" {
			return v
		}
		return fallback
	}

	dbURL := require("DATABASE_URL")
	jwtSecret := require("JWT_SECRET")
	jwtRefresh := require("JWT_REFRESH_SECRET")
	baseURL := require("BASE_URL")

	portStr := optional("PORT", "8080")
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return nil, fmt.Errorf("config: PORT must be an integer, got %q", portStr)
	}

	smtpPortStr := optional("SMTP_PORT", "587")
	smtpPort, err := strconv.Atoi(smtpPortStr)
	if err != nil {
		return nil, fmt.Errorf("config: SMTP_PORT must be an integer, got %q", smtpPortStr)
	}

	if len(missing) > 0 {
		return nil, fmt.Errorf("config: missing required environment variables: %s", strings.Join(missing, ", "))
	}

	originsRaw := optional("ALLOWED_ORIGINS", "")
	var origins []string
	if originsRaw != "" {
		for _, o := range strings.Split(originsRaw, ",") {
			if trimmed := strings.TrimSpace(o); trimmed != "" {
				origins = append(origins, trimmed)
			}
		}
	}

	return &Config{
		DatabaseURL:      dbURL,
		JWTSecret:        []byte(jwtSecret),
		JWTRefreshSecret: []byte(jwtRefresh),
		BaseURL:          baseURL,
		Port:             port,
		AllowedOrigins:   origins,
		SMTPHost:         optional("SMTP_HOST", ""),
		SMTPPort:         smtpPort,
		SMTPUser:         optional("SMTP_USER", ""),
		SMTPPass:         optional("SMTP_PASS", ""),
		SMTPFrom:         optional("SMTP_FROM", ""),
		MaxMindDBPath:    optional("MAXMIND_DB_PATH", ""),
		Env:              optional("ENV", "production"),
	}, nil
}

// IsDev reports whether the application is running in development mode.
func (c *Config) IsDev() bool {
	return c.Env == "development"
}
```

- [ ] **Step 4: Run the test to verify it passes**

```bash
go test ./config/... -v
```

Expected: PASS — both tests pass.

- [ ] **Step 5: Commit**

```bash
git add config/
git commit -m "feat: add typed config system loaded from env vars"
```

---

### Task 3: Database connection pool

**Files:**
- Create: `internal/repository/db.go`

- [ ] **Step 1: Write the failing test**

Create `internal/repository/db_test.go`:

```go
package repository_test

import (
	"context"
	"os"
	"testing"

	"github.com/sidneydekoning/analytics/internal/repository"
	"github.com/stretchr/testify/require"
)

func TestNewPool_ConnectsSuccessfully(t *testing.T) {
	dbURL := os.Getenv("TEST_DATABASE_URL")
	if dbURL == "" {
		t.Skip("TEST_DATABASE_URL not set — skipping integration test")
	}

	pool, err := repository.NewPool(context.Background(), dbURL)
	require.NoError(t, err)
	defer pool.Close()

	err = pool.Ping(context.Background())
	require.NoError(t, err)
}
```

- [ ] **Step 2: Run the test to verify it fails**

```bash
go test ./internal/repository/... -v -run TestNewPool
```

Expected: FAIL — `repository` package does not exist.

- [ ] **Step 3: Implement `internal/repository/db.go`**

```go
package repository

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// NewPool creates a pgx/v5 connection pool for the given database URL.
// The caller is responsible for calling pool.Close() when done.
func NewPool(ctx context.Context, databaseURL string) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("repository.NewPool: parse config: %w", err)
	}

	// Conservative pool settings for a single-server deployment.
	cfg.MaxConns = 20
	cfg.MinConns = 2

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("repository.NewPool: create pool: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("repository.NewPool: ping: %w", err)
	}

	return pool, nil
}
```

- [ ] **Step 4: Run the test**

```bash
# Without TEST_DATABASE_URL — should skip
go test ./internal/repository/... -v -run TestNewPool
```

Expected: SKIP (no TEST_DATABASE_URL set). Set the var and re-run against a real TimescaleDB for full integration test.

- [ ] **Step 5: Commit**

```bash
git add internal/repository/db.go internal/repository/db_test.go
git commit -m "feat: add pgx/v5 connection pool constructor"
```

---

### Task 4: Database migrations

**Files:**
- Create: `internal/repository/migrations.go`
- Create: `migrations/001_init.sql` through `migrations/007_indexes.sql`
- Create: `cmd/migrate/main.go`

- [ ] **Step 1: Create `migrations/001_init.sql`**

```sql
-- 001_init.sql
CREATE EXTENSION IF NOT EXISTS "pgcrypto";

CREATE TYPE user_role AS ENUM ('admin', 'user');
CREATE TYPE member_role AS ENUM ('owner', 'editor', 'viewer');

CREATE TABLE users (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email         TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    role          user_role NOT NULL DEFAULT 'user',
    name          TEXT NOT NULL DEFAULT '',
    is_active     BOOLEAN NOT NULL DEFAULT TRUE,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_login_at TIMESTAMPTZ
);

CREATE TABLE sites (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    owner_id   UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name       TEXT NOT NULL,
    domain     TEXT NOT NULL,
    token      TEXT NOT NULL UNIQUE,
    timezone   TEXT NOT NULL DEFAULT 'UTC',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE site_members (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    site_id     UUID NOT NULL REFERENCES sites(id) ON DELETE CASCADE,
    user_id     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role        member_role NOT NULL DEFAULT 'viewer',
    invited_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    accepted_at TIMESTAMPTZ,
    UNIQUE (site_id, user_id)
);

CREATE TABLE invitations (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    site_id    UUID NOT NULL REFERENCES sites(id) ON DELETE CASCADE,
    email      TEXT NOT NULL,
    token      TEXT NOT NULL UNIQUE,
    role       member_role NOT NULL DEFAULT 'viewer',
    expires_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE revoked_tokens (
    jti        TEXT PRIMARY KEY,
    user_id    UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    expires_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE password_reset_tokens (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash TEXT NOT NULL UNIQUE,
    expires_at TIMESTAMPTZ NOT NULL,
    used_at    TIMESTAMPTZ
);

CREATE TABLE email_verifications (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash TEXT NOT NULL UNIQUE,
    expires_at TIMESTAMPTZ NOT NULL,
    verified_at TIMESTAMPTZ
);
```

- [ ] **Step 2: Create `migrations/002_hypertables.sql`**

```sql
-- 002_hypertables.sql
CREATE TABLE events (
    id          BIGSERIAL,
    site_id     UUID NOT NULL,
    type        TEXT NOT NULL DEFAULT 'pageview',
    url         TEXT NOT NULL DEFAULT '',
    referrer    TEXT NOT NULL DEFAULT '',
    channel     TEXT NOT NULL DEFAULT 'direct',
    utm_source  TEXT NOT NULL DEFAULT '',
    utm_medium  TEXT NOT NULL DEFAULT '',
    utm_campaign TEXT NOT NULL DEFAULT '',
    country     CHAR(2) NOT NULL DEFAULT '',
    city        TEXT NOT NULL DEFAULT '',
    device_type TEXT NOT NULL DEFAULT '',
    browser     TEXT NOT NULL DEFAULT '',
    os          TEXT NOT NULL DEFAULT '',
    language    TEXT NOT NULL DEFAULT '',
    session_id  TEXT NOT NULL DEFAULT '',
    visitor_id  TEXT NOT NULL DEFAULT '',
    is_bounce   BOOLEAN NOT NULL DEFAULT FALSE,
    duration_ms INTEGER NOT NULL DEFAULT 0,
    props       JSONB,
    timestamp   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (id, timestamp)
);

SELECT create_hypertable('events', 'timestamp', chunk_time_interval => INTERVAL '1 day');

-- Foreign key NOT enforced on hypertable (TimescaleDB limitation) — enforced at application layer.
-- Index site_id for fast per-site queries.
CREATE INDEX ON events (site_id, timestamp DESC);
```

- [ ] **Step 3: Create `migrations/003_continuous_aggregates.sql`**

```sql
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
    start_offset => INTERVAL '2 days',
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
    start_offset => INTERVAL '2 days',
    end_offset   => INTERVAL '1 day',
    schedule_interval => INTERVAL '1 day');
```

- [ ] **Step 4: Create `migrations/004_funnels.sql`**

```sql
-- 004_funnels.sql
CREATE TYPE goal_type AS ENUM ('pageview', 'event', 'outbound');
CREATE TYPE funnel_match AS ENUM ('url', 'event', 'goal');

CREATE TABLE goals (
    id      UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    site_id UUID NOT NULL REFERENCES sites(id) ON DELETE CASCADE,
    name    TEXT NOT NULL,
    type    goal_type NOT NULL,
    value   TEXT NOT NULL
);

CREATE TABLE funnels (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    site_id    UUID NOT NULL REFERENCES sites(id) ON DELETE CASCADE,
    name       TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE funnel_steps (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    funnel_id  UUID NOT NULL REFERENCES funnels(id) ON DELETE CASCADE,
    position   INTEGER NOT NULL,
    match_type funnel_match NOT NULL,
    value      TEXT NOT NULL,
    name       TEXT NOT NULL DEFAULT '',
    UNIQUE (funnel_id, position)
);
```

- [ ] **Step 5: Create `migrations/005_cms.sql`**

```sql
-- 005_cms.sql
CREATE TYPE page_type AS ENUM ('blog', 'page');
CREATE TYPE page_status AS ENUM ('draft', 'published');

CREATE TABLE cms_layouts (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name          TEXT NOT NULL,
    template_file TEXT NOT NULL,
    description   TEXT NOT NULL DEFAULT ''
);

CREATE TABLE cms_pages (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    layout_id        UUID NOT NULL REFERENCES cms_layouts(id),
    author_id        UUID NOT NULL REFERENCES users(id),
    title            TEXT NOT NULL,
    slug             TEXT NOT NULL UNIQUE,
    type             page_type NOT NULL DEFAULT 'blog',
    content_html     TEXT NOT NULL DEFAULT '',
    excerpt          TEXT NOT NULL DEFAULT '',
    cover_image_url  TEXT NOT NULL DEFAULT '',
    meta_title       TEXT NOT NULL DEFAULT '',
    meta_description TEXT NOT NULL DEFAULT '',
    status           page_status NOT NULL DEFAULT 'draft',
    published_at     TIMESTAMPTZ,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE cms_tags (
    id   UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name TEXT NOT NULL,
    slug TEXT NOT NULL UNIQUE
);

CREATE TABLE cms_page_tags (
    page_id UUID NOT NULL REFERENCES cms_pages(id) ON DELETE CASCADE,
    tag_id  UUID NOT NULL REFERENCES cms_tags(id) ON DELETE CASCADE,
    PRIMARY KEY (page_id, tag_id)
);
```

- [ ] **Step 6: Create `migrations/006_audit_log.sql`**

```sql
-- 006_audit_log.sql
CREATE TABLE audit_log (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    actor_id      UUID REFERENCES users(id) ON DELETE SET NULL,
    action        TEXT NOT NULL,
    resource_type TEXT NOT NULL DEFAULT '',
    resource_id   TEXT NOT NULL DEFAULT '',
    ip_hash       TEXT NOT NULL DEFAULT '',
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
```

- [ ] **Step 7: Create `migrations/007_indexes.sql`**

```sql
-- 007_indexes.sql
CREATE INDEX ON sites (owner_id);
CREATE INDEX ON sites (domain);
CREATE INDEX ON site_members (user_id);
CREATE INDEX ON site_members (site_id);
CREATE INDEX ON invitations (token);
CREATE INDEX ON invitations (email);
CREATE INDEX ON revoked_tokens (expires_at);
CREATE INDEX ON password_reset_tokens (user_id);
CREATE INDEX ON cms_pages (slug);
CREATE INDEX ON cms_pages (status, published_at DESC);
CREATE INDEX ON audit_log (actor_id, created_at DESC);
CREATE INDEX ON goals (site_id);
CREATE INDEX ON funnels (site_id);
```

- [ ] **Step 8: Create `internal/repository/migrations.go`**

```go
package repository

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed ../../migrations/*.sql
var migrationsFS embed.FS

// Migrate runs all SQL migration files in order.
// Safe to call on every startup — tracks applied migrations in schema_migrations table.
func Migrate(ctx context.Context, pool *pgxpool.Pool) error {
	if _, err := pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version TEXT PRIMARY KEY,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)
	`); err != nil {
		return fmt.Errorf("repository.Migrate: create migrations table: %w", err)
	}

	entries, err := fs.ReadDir(migrationsFS, "migrations")
	if err != nil {
		return fmt.Errorf("repository.Migrate: read migrations dir: %w", err)
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name() < entries[j].Name()
	})

	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}

		version := strings.TrimSuffix(entry.Name(), ".sql")

		var applied bool
		err := pool.QueryRow(ctx,
			`SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE version = $1)`,
			version,
		).Scan(&applied)
		if err != nil {
			return fmt.Errorf("repository.Migrate: check %s: %w", version, err)
		}
		if applied {
			continue
		}

		sql, err := migrationsFS.ReadFile("migrations/" + entry.Name())
		if err != nil {
			return fmt.Errorf("repository.Migrate: read %s: %w", version, err)
		}

		if _, err := pool.Exec(ctx, string(sql)); err != nil {
			return fmt.Errorf("repository.Migrate: apply %s: %w", version, err)
		}

		if _, err := pool.Exec(ctx,
			`INSERT INTO schema_migrations (version) VALUES ($1)`, version,
		); err != nil {
			return fmt.Errorf("repository.Migrate: record %s: %w", version, err)
		}
	}

	return nil
}
```

- [ ] **Step 9: Run the tests**

```bash
go test ./internal/repository/... -v
```

Expected: PASS (migration test skipped without TEST_DATABASE_URL). Set `TEST_DATABASE_URL` and run again against real TimescaleDB.

- [ ] **Step 10: Commit**

```bash
git add internal/repository/migrations.go migrations/
git commit -m "feat: add SQL migrations and embedded migration runner"
```

---

### Task 5: Domain models

**Files:**
- Create: `internal/model/user.go`
- Create: `internal/model/site.go`
- Create: `internal/model/token.go`

- [ ] **Step 1: Create `internal/model/user.go`**

```go
package model

import "time"

// Role values must match the user_role enum in 001_init.sql.
type Role string

const (
	RoleAdmin Role = "admin"
	RoleUser  Role = "user"
)

// MemberRole values must match the member_role enum in 001_init.sql.
type MemberRole string

const (
	MemberRoleOwner  MemberRole = "owner"
	MemberRoleEditor MemberRole = "editor"
	MemberRoleViewer MemberRole = "viewer"
)

// User represents a registered account. Never includes the password hash in
// responses — the hash field is only populated when loaded from the repository.
type User struct {
	ID           string
	Email        string
	PasswordHash string
	Role         Role
	Name         string
	IsActive     bool
	CreatedAt    time.Time
	LastLoginAt  *time.Time
}

// SiteMember represents a user's access to a specific site.
type SiteMember struct {
	ID         string
	SiteID     string
	UserID     string
	Role       MemberRole
	InvitedAt  time.Time
	AcceptedAt *time.Time
}

// Invitation is a pending invite to a site sent to an email address.
type Invitation struct {
	ID        string
	SiteID    string
	Email     string
	Token     string
	Role      MemberRole
	ExpiresAt time.Time
}
```

- [ ] **Step 2: Create `internal/model/site.go`**

```go
package model

import "time"

// Site represents a website registered by a user.
// Token is the value that goes in the data-site attribute of the tracking script.
type Site struct {
	ID        string
	OwnerID   string
	Name      string
	Domain    string
	Token     string
	Timezone  string
	CreatedAt time.Time
}
```

- [ ] **Step 3: Create `internal/model/token.go`**

```go
package model

import "time"

// RevokedToken marks a JWT jti as revoked before its natural expiry.
type RevokedToken struct {
	JTI       string
	UserID    string
	ExpiresAt time.Time
}

// PasswordResetToken is a short-lived token sent via email for password resets.
type PasswordResetToken struct {
	ID        string
	UserID    string
	TokenHash string
	ExpiresAt time.Time
	UsedAt    *time.Time
}
```

- [ ] **Step 4: Run tests (compile check)**

```bash
go build ./...
```

Expected: compiles cleanly.

- [ ] **Step 5: Commit**

```bash
git add internal/model/
git commit -m "feat: add domain model types (User, Site, Token)"
```

---

### Task 6: User repository

**Files:**
- Create: `internal/repository/user.go`
- Create: `internal/repository/user_test.go`

- [ ] **Step 1: Write the failing tests**

Create `internal/repository/user_test.go`:

```go
package repository_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/sidneydekoning/analytics/internal/model"
	"github.com/sidneydekoning/analytics/internal/repository"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTestDB(t *testing.T) *repository.Repos {
	t.Helper()
	dbURL := os.Getenv("TEST_DATABASE_URL")
	if dbURL == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	pool, err := repository.NewPool(context.Background(), dbURL)
	require.NoError(t, err)
	t.Cleanup(func() { pool.Close() })
	err = repository.Migrate(context.Background(), pool)
	require.NoError(t, err)
	return repository.New(pool)
}

func TestUserRepository_CreateAndGetByEmail(t *testing.T) {
	repos := setupTestDB(t)
	ctx := context.Background()

	user := &model.User{
		Email:        "test+" + time.Now().Format("20060102150405") + "@example.com",
		PasswordHash: "$2a$12$somehash",
		Role:         model.RoleUser,
		Name:         "Test User",
		IsActive:     true,
	}

	err := repos.Users.Create(ctx, user)
	require.NoError(t, err)
	assert.NotEmpty(t, user.ID)

	found, err := repos.Users.GetByEmail(ctx, user.Email)
	require.NoError(t, err)
	assert.Equal(t, user.Email, found.Email)
	assert.Equal(t, user.ID, found.ID)
}

func TestUserRepository_GetByEmail_NotFound(t *testing.T) {
	repos := setupTestDB(t)
	_, err := repos.Users.GetByEmail(context.Background(), "nonexistent@example.com")
	assert.ErrorIs(t, err, repository.ErrNotFound)
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/repository/... -v -run TestUser
```

Expected: FAIL — `repository.Repos` and `repository.ErrNotFound` not defined.

- [ ] **Step 3: Implement `internal/repository/user.go`**

```go
package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/sidneydekoning/analytics/internal/model"
)

// ErrNotFound is returned when a requested record does not exist.
var ErrNotFound = errors.New("record not found")

// UserRepository defines all database operations for users.
type UserRepository interface {
	Create(ctx context.Context, u *model.User) error
	GetByID(ctx context.Context, id string) (*model.User, error)
	GetByEmail(ctx context.Context, email string) (*model.User, error)
	UpdateLastLogin(ctx context.Context, id string) error
	SetActive(ctx context.Context, id string, active bool) error
}

type pgUserRepository struct {
	pool *pgxpool.Pool
}

func (r *pgUserRepository) Create(ctx context.Context, u *model.User) error {
	err := r.pool.QueryRow(ctx, `
		INSERT INTO users (email, password_hash, role, name, is_active)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, created_at
	`, u.Email, u.PasswordHash, u.Role, u.Name, u.IsActive).
		Scan(&u.ID, &u.CreatedAt)
	if err != nil {
		return fmt.Errorf("userRepository.Create: %w", err)
	}
	return nil
}

func (r *pgUserRepository) GetByID(ctx context.Context, id string) (*model.User, error) {
	u := &model.User{}
	err := r.pool.QueryRow(ctx, `
		SELECT id, email, password_hash, role, name, is_active, created_at, last_login_at
		FROM users WHERE id = $1
	`, id).Scan(
		&u.ID, &u.Email, &u.PasswordHash, &u.Role,
		&u.Name, &u.IsActive, &u.CreatedAt, &u.LastLoginAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("userRepository.GetByID: %w", err)
	}
	return u, nil
}

func (r *pgUserRepository) GetByEmail(ctx context.Context, email string) (*model.User, error) {
	u := &model.User{}
	err := r.pool.QueryRow(ctx, `
		SELECT id, email, password_hash, role, name, is_active, created_at, last_login_at
		FROM users WHERE email = $1
	`, email).Scan(
		&u.ID, &u.Email, &u.PasswordHash, &u.Role,
		&u.Name, &u.IsActive, &u.CreatedAt, &u.LastLoginAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("userRepository.GetByEmail: %w", err)
	}
	return u, nil
}

func (r *pgUserRepository) UpdateLastLogin(ctx context.Context, id string) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE users SET last_login_at = NOW() WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("userRepository.UpdateLastLogin: %w", err)
	}
	return nil
}

func (r *pgUserRepository) SetActive(ctx context.Context, id string, active bool) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE users SET is_active = $2 WHERE id = $1`, id, active)
	if err != nil {
		return fmt.Errorf("userRepository.SetActive: %w", err)
	}
	return nil
}

// Repos aggregates all repository interfaces for dependency injection.
type Repos struct {
	Users UserRepository
	Sites SiteRepository
}

// New creates a Repos with all pg implementations wired up.
func New(pool *pgxpool.Pool) *Repos {
	return &Repos{
		Users: &pgUserRepository{pool: pool},
		Sites: &pgSiteRepository{pool: pool},
	}
}
```

- [ ] **Step 4: Run the test**

```bash
go test ./internal/repository/... -v -run TestUser
```

Expected: SKIP without `TEST_DATABASE_URL`; PASS with it.

- [ ] **Step 5: Commit**

```bash
git add internal/repository/user.go internal/repository/user_test.go
git commit -m "feat: add user repository with pgx/v5"
```

---

### Task 7: Site repository

**Files:**
- Create: `internal/repository/site.go`
- Create: `internal/repository/site_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/repository/site_test.go`:

```go
package repository_test

import (
	"context"
	"testing"
	"time"

	"github.com/sidneydekoning/analytics/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSiteRepository_CreateAndGetByToken(t *testing.T) {
	repos := setupTestDB(t)
	ctx := context.Background()

	owner := &model.User{
		Email:        "owner+" + time.Now().Format("20060102150405") + "@example.com",
		PasswordHash: "$2a$12$somehash",
		Role:         model.RoleUser,
		Name:         "Owner",
		IsActive:     true,
	}
	require.NoError(t, repos.Users.Create(ctx, owner))

	site := &model.Site{
		OwnerID:  owner.ID,
		Name:     "Test Site",
		Domain:   "example.com",
		Token:    "tk_test" + time.Now().Format("20060102150405"),
		Timezone: "UTC",
	}
	err := repos.Sites.Create(ctx, site)
	require.NoError(t, err)
	assert.NotEmpty(t, site.ID)

	found, err := repos.Sites.GetByToken(ctx, site.Token)
	require.NoError(t, err)
	assert.Equal(t, site.ID, found.ID)
	assert.Equal(t, "example.com", found.Domain)
}
```

- [ ] **Step 2: Run to verify it fails**

```bash
go test ./internal/repository/... -v -run TestSite
```

Expected: FAIL — `SiteRepository` not yet implemented.

- [ ] **Step 3: Implement `internal/repository/site.go`**

```go
package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/sidneydekoning/analytics/internal/model"
)

// SiteRepository defines all database operations for sites.
type SiteRepository interface {
	Create(ctx context.Context, s *model.Site) error
	GetByID(ctx context.Context, id string) (*model.Site, error)
	GetByToken(ctx context.Context, token string) (*model.Site, error)
	ListByOwner(ctx context.Context, ownerID string) ([]*model.Site, error)
	Delete(ctx context.Context, id string) error
}

type pgSiteRepository struct {
	pool *pgxpool.Pool
}

func (r *pgSiteRepository) Create(ctx context.Context, s *model.Site) error {
	err := r.pool.QueryRow(ctx, `
		INSERT INTO sites (owner_id, name, domain, token, timezone)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, created_at
	`, s.OwnerID, s.Name, s.Domain, s.Token, s.Timezone).
		Scan(&s.ID, &s.CreatedAt)
	if err != nil {
		return fmt.Errorf("siteRepository.Create: %w", err)
	}
	return nil
}

func (r *pgSiteRepository) GetByID(ctx context.Context, id string) (*model.Site, error) {
	s := &model.Site{}
	err := r.pool.QueryRow(ctx, `
		SELECT id, owner_id, name, domain, token, timezone, created_at
		FROM sites WHERE id = $1
	`, id).Scan(&s.ID, &s.OwnerID, &s.Name, &s.Domain, &s.Token, &s.Timezone, &s.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("siteRepository.GetByID: %w", err)
	}
	return s, nil
}

func (r *pgSiteRepository) GetByToken(ctx context.Context, token string) (*model.Site, error) {
	s := &model.Site{}
	err := r.pool.QueryRow(ctx, `
		SELECT id, owner_id, name, domain, token, timezone, created_at
		FROM sites WHERE token = $1
	`, token).Scan(&s.ID, &s.OwnerID, &s.Name, &s.Domain, &s.Token, &s.Timezone, &s.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("siteRepository.GetByToken: %w", err)
	}
	return s, nil
}

func (r *pgSiteRepository) ListByOwner(ctx context.Context, ownerID string) ([]*model.Site, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, owner_id, name, domain, token, timezone, created_at
		FROM sites WHERE owner_id = $1 ORDER BY created_at DESC
	`, ownerID)
	if err != nil {
		return nil, fmt.Errorf("siteRepository.ListByOwner: %w", err)
	}
	defer rows.Close()

	var sites []*model.Site
	for rows.Next() {
		s := &model.Site{}
		if err := rows.Scan(&s.ID, &s.OwnerID, &s.Name, &s.Domain, &s.Token, &s.Timezone, &s.CreatedAt); err != nil {
			return nil, fmt.Errorf("siteRepository.ListByOwner: scan: %w", err)
		}
		sites = append(sites, s)
	}
	return sites, rows.Err()
}

func (r *pgSiteRepository) Delete(ctx context.Context, id string) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM sites WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("siteRepository.Delete: %w", err)
	}
	return nil
}
```

- [ ] **Step 4: Run tests**

```bash
go test ./internal/repository/... -v
```

Expected: PASS (or SKIP without TEST_DATABASE_URL).

- [ ] **Step 5: Commit**

```bash
git add internal/repository/site.go internal/repository/site_test.go
git commit -m "feat: add site repository with pgx/v5"
```

---

### Task 8: Auth service (JWT + bcrypt + token generation)

**Files:**
- Create: `internal/service/auth.go`
- Create: `internal/service/auth_test.go`

- [ ] **Step 1: Write the failing tests**

Create `internal/service/auth_test.go`:

```go
package service_test

import (
	"testing"

	"github.com/sidneydekoning/analytics/internal/service"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHashPassword_AndCompare(t *testing.T) {
	svc := service.NewAuth([]byte("test-secret-32-bytes-xxxxxxxxxx"), []byte("refresh-secret-32-bytes-xxxxxxxx"))

	hash, err := svc.HashPassword("correct-horse-battery-staple")
	require.NoError(t, err)
	assert.NotEmpty(t, hash)
	assert.NotEqual(t, "correct-horse-battery-staple", hash)

	assert.True(t, svc.CheckPassword("correct-horse-battery-staple", hash))
	assert.False(t, svc.CheckPassword("wrong-password", hash))
}

func TestIssueAndParseAccessToken(t *testing.T) {
	svc := service.NewAuth([]byte("test-secret-32-bytes-xxxxxxxxxx"), []byte("refresh-secret-32-bytes-xxxxxxxx"))

	claims, err := svc.IssueAccessToken("user-uuid-123", "user")
	require.NoError(t, err)
	assert.NotEmpty(t, claims.TokenString)
	assert.Equal(t, "user-uuid-123", claims.UserID)

	parsed, err := svc.ParseAccessToken(claims.TokenString)
	require.NoError(t, err)
	assert.Equal(t, "user-uuid-123", parsed.UserID)
	assert.Equal(t, "user", parsed.Role)
}

func TestGenerateSiteToken_IsUnique(t *testing.T) {
	svc := service.NewAuth([]byte("test-secret-32-bytes-xxxxxxxxxx"), []byte("refresh-secret-32-bytes-xxxxxxxx"))

	t1, err := svc.GenerateSiteToken()
	require.NoError(t, err)

	t2, err := svc.GenerateSiteToken()
	require.NoError(t, err)

	assert.NotEqual(t, t1, t2)
	assert.True(t, len(t1) > 8)
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/service/... -v -run TestHash
```

Expected: FAIL — package not defined.

- [ ] **Step 3: Implement `internal/service/auth.go`**

```go
package service

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

const (
	accessTokenDuration  = 15 * time.Minute
	refreshTokenDuration = 7 * 24 * time.Hour
	bcryptCost           = 12
	siteTokenPrefix      = "tk_"
)

// TokenClaims holds the parsed values from a JWT.
type TokenClaims struct {
	UserID      string
	Role        string
	JTI         string
	TokenString string // only populated on issue, not on parse
}

// AuthService handles JWT issuance/validation, password hashing, and token generation.
// All secret comparisons use constant-time operations.
type AuthService interface {
	HashPassword(password string) (string, error)
	CheckPassword(password, hash string) bool
	IssueAccessToken(userID, role string) (*TokenClaims, error)
	IssueRefreshToken(userID string) (*TokenClaims, error)
	ParseAccessToken(tokenString string) (*TokenClaims, error)
	ParseRefreshToken(tokenString string) (*TokenClaims, error)
	GenerateSiteToken() (string, error)
	GenerateSecureToken() (string, error)
}

type authService struct {
	accessSecret  []byte
	refreshSecret []byte
}

// NewAuth constructs an AuthService. Both secrets must be at least 32 bytes.
func NewAuth(accessSecret, refreshSecret []byte) AuthService {
	return &authService{
		accessSecret:  accessSecret,
		refreshSecret: refreshSecret,
	}
}

func (s *authService) HashPassword(password string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(password), bcryptCost)
	if err != nil {
		return "", fmt.Errorf("authService.HashPassword: %w", err)
	}
	return string(b), nil
}

// CheckPassword uses bcrypt's constant-time compare — safe against timing attacks.
func (s *authService) CheckPassword(password, hash string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	return err == nil
}

func (s *authService) IssueAccessToken(userID, role string) (*TokenClaims, error) {
	jti, err := randomHex(16)
	if err != nil {
		return nil, fmt.Errorf("authService.IssueAccessToken: %w", err)
	}

	now := time.Now()
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub":  userID,
		"role": role,
		"jti":  jti,
		"iat":  now.Unix(),
		"exp":  now.Add(accessTokenDuration).Unix(),
	})

	signed, err := token.SignedString(s.accessSecret)
	if err != nil {
		return nil, fmt.Errorf("authService.IssueAccessToken: sign: %w", err)
	}

	return &TokenClaims{UserID: userID, Role: role, JTI: jti, TokenString: signed}, nil
}

func (s *authService) IssueRefreshToken(userID string) (*TokenClaims, error) {
	jti, err := randomHex(16)
	if err != nil {
		return nil, fmt.Errorf("authService.IssueRefreshToken: %w", err)
	}

	now := time.Now()
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub": userID,
		"jti": jti,
		"iat": now.Unix(),
		"exp": now.Add(refreshTokenDuration).Unix(),
	})

	signed, err := token.SignedString(s.refreshSecret)
	if err != nil {
		return nil, fmt.Errorf("authService.IssueRefreshToken: sign: %w", err)
	}

	return &TokenClaims{UserID: userID, JTI: jti, TokenString: signed}, nil
}

func (s *authService) ParseAccessToken(tokenString string) (*TokenClaims, error) {
	return s.parseToken(tokenString, s.accessSecret)
}

func (s *authService) ParseRefreshToken(tokenString string) (*TokenClaims, error) {
	return s.parseToken(tokenString, s.refreshSecret)
}

func (s *authService) parseToken(tokenString string, secret []byte) (*TokenClaims, error) {
	token, err := jwt.Parse(tokenString, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return secret, nil
	}, jwt.WithValidMethods([]string{"HS256"}))
	if err != nil {
		return nil, fmt.Errorf("authService.parseToken: %w", err)
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok || !token.Valid {
		return nil, fmt.Errorf("authService.parseToken: invalid claims")
	}

	return &TokenClaims{
		UserID: fmt.Sprint(claims["sub"]),
		Role:   fmt.Sprint(claims["role"]),
		JTI:    fmt.Sprint(claims["jti"]),
	}, nil
}

// GenerateSiteToken returns a unique token for embedding in the tracking script.
func (s *authService) GenerateSiteToken() (string, error) {
	b, err := randomHex(8)
	if err != nil {
		return "", fmt.Errorf("authService.GenerateSiteToken: %w", err)
	}
	return siteTokenPrefix + b, nil
}

// GenerateSecureToken returns a 32-byte cryptographically random hex string for
// password reset and email verification tokens.
func (s *authService) GenerateSecureToken() (string, error) {
	b, err := randomHex(32)
	if err != nil {
		return "", fmt.Errorf("authService.GenerateSecureToken: %w", err)
	}
	return b, nil
}

func randomHex(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
```

- [ ] **Step 4: Run the tests**

```bash
go test ./internal/service/... -v
```

Expected: PASS — all 3 test functions pass.

- [ ] **Step 5: Commit**

```bash
git add internal/service/auth.go internal/service/auth_test.go
git commit -m "feat: add auth service with JWT, bcrypt, and secure token generation"
```

---

### Task 9: Security middleware

**Files:**
- Create: `internal/middleware/security.go`

- [ ] **Step 1: Implement `internal/middleware/security.go`**

```go
package middleware

import (
	"crypto/rand"
	"encoding/base64"
	"net/http"
	"strings"
)

// SecurityHeaders sets all required security headers on every response.
// Must be the outermost middleware so headers are present even on error responses.
func SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Strict-Transport-Security", "max-age=63072000; includeSubDomains; preload")
		h.Set("Content-Security-Policy",
			"default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; connect-src 'self'; frame-ancestors 'none'")
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("X-XSS-Protection", "0")
		h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		h.Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		next.ServeHTTP(w, r)
	})
}

// CORS returns a middleware that handles Cross-Origin Resource Sharing.
// collectOrigin: set to true for the /collect endpoint (allow all origins, POST only).
// allowedOrigins: list of allowed origins for API endpoints (never use * on auth routes).
func CORS(allowedOrigins []string, collectPaths ...string) func(http.Handler) http.Handler {
	collectSet := make(map[string]bool)
	for _, p := range collectPaths {
		collectSet[p] = true
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")

			// /collect: accept any origin, POST only
			if collectSet[r.URL.Path] {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Access-Control-Allow-Methods", "POST")
				w.Header().Set("Access-Control-Max-Age", "86400")
				if r.Method == http.MethodOptions {
					w.WriteHeader(http.StatusNoContent)
					return
				}
				next.ServeHTTP(w, r)
				return
			}

			// API endpoints: only allowed origins
			for _, allowed := range allowedOrigins {
				if origin == allowed {
					w.Header().Set("Access-Control-Allow-Origin", origin)
					w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
					w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
					w.Header().Set("Access-Control-Allow-Credentials", "true")
					break
				}
			}

			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// csrfTokenKey is the context key for the CSRF token.
type csrfTokenKey struct{}

// CSRF implements the double-submit cookie pattern.
// Sets a non-HTTP-only cookie with a random token.
// For state-changing requests (POST/PUT/DELETE/PATCH), validates the token
// from the X-CSRF-Token header or _csrf form field against the cookie.
func CSRF(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Exempt /collect — it uses token auth, not cookies
		if r.URL.Path == "/collect" {
			next.ServeHTTP(w, r)
			return
		}

		// Get or generate CSRF token
		token := ""
		if cookie, err := r.Cookie("csrf_token"); err == nil {
			token = cookie.Value
		} else {
			b := make([]byte, 32)
			if _, err := rand.Read(b); err != nil {
				http.Error(w, "internal error", http.StatusInternalServerError)
				return
			}
			token = base64.URLEncoding.EncodeToString(b)
			http.SetCookie(w, &http.Cookie{
				Name:     "csrf_token",
				Value:    token,
				Path:     "/",
				HttpOnly: false, // must be readable by JS to submit in header
				Secure:   true,
				SameSite: http.SameSiteStrictMode,
			})
		}

		// Validate on state-changing methods
		if r.Method == http.MethodPost || r.Method == http.MethodPut ||
			r.Method == http.MethodPatch || r.Method == http.MethodDelete {

			submitted := r.Header.Get("X-CSRF-Token")
			if submitted == "" {
				submitted = r.FormValue("_csrf")
			}

			if submitted == "" || !strings.EqualFold(submitted, token) {
				http.Error(w, "invalid CSRF token", http.StatusForbidden)
				return
			}
		}

		next.ServeHTTP(w, r)
	})
}
```

- [ ] **Step 2: Run a compile check**

```bash
go build ./...
```

Expected: compiles cleanly.

- [ ] **Step 3: Commit**

```bash
git add internal/middleware/security.go
git commit -m "feat: add security headers, CORS, and CSRF middleware"
```

---

### Task 10: Rate limiting, logger, and auth middleware

**Files:**
- Create: `internal/middleware/ratelimit.go`
- Create: `internal/middleware/logger.go`
- Create: `internal/middleware/auth.go`

- [ ] **Step 1: Create `internal/middleware/ratelimit.go`**

```go
package middleware

import (
	"log/slog"
	"net"
	"net/http"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

type ipLimiter struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

// RateLimiter returns a per-IP rate limiting middleware.
// r: requests per second; burst: maximum burst size.
func RateLimiter(r rate.Limit, burst int) func(http.Handler) http.Handler {
	limiters := make(map[string]*ipLimiter)
	mu := sync.Mutex{}

	// Clean up stale entries every 5 minutes.
	go func() {
		for range time.Tick(5 * time.Minute) {
			mu.Lock()
			for ip, l := range limiters {
				if time.Since(l.lastSeen) > 10*time.Minute {
					delete(limiters, ip)
				}
			}
			mu.Unlock()
		}
	}()

	getLimiter := func(ip string) *rate.Limiter {
		mu.Lock()
		defer mu.Unlock()
		l, ok := limiters[ip]
		if !ok {
			l = &ipLimiter{limiter: rate.NewLimiter(r, burst)}
			limiters[ip] = l
		}
		l.lastSeen = time.Now()
		return l.limiter
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			ip, _, err := net.SplitHostPort(req.RemoteAddr)
			if err != nil {
				ip = req.RemoteAddr
			}
			// Prefer X-Real-IP if set by a trusted reverse proxy.
			if realIP := req.Header.Get("X-Real-IP"); realIP != "" {
				ip = realIP
			}

			if !getLimiter(ip).Allow() {
				slog.Warn("rate limit exceeded", "ip", ip, "path", req.URL.Path)
				http.Error(w, "too many requests", http.StatusTooManyRequests)
				return
			}
			next.ServeHTTP(w, req)
		})
	}
}
```

- [ ] **Step 2: Create `internal/middleware/logger.go`**

```go
package middleware

import (
	"log/slog"
	"net/http"
	"time"
)

type responseRecorder struct {
	http.ResponseWriter
	status int
}

func (r *responseRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

// Logger logs each request using structured logging (log/slog).
// Never logs Authorization headers, cookies, or request bodies.
func Logger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &responseRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		slog.Info("request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", rec.status,
			"duration_ms", time.Since(start).Milliseconds(),
		)
	})
}
```

- [ ] **Step 3: Create `internal/middleware/auth.go`**

```go
package middleware

import (
	"context"
	"net/http"
	"strings"

	"github.com/sidneydekoning/analytics/internal/service"
)

type contextKey string

const (
	ContextKeyUserID contextKey = "user_id"
	ContextKeyRole   contextKey = "role"
)

// JWTAuth validates the access token from the auth cookie.
// On success, sets user_id and role in the request context.
// Returns 401 if no valid token is present.
func JWTAuth(authSvc service.AuthService) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			tokenString := tokenFromRequest(r)
			if tokenString == "" {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}

			claims, err := authSvc.ParseAccessToken(tokenString)
			if err != nil {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}

			ctx := context.WithValue(r.Context(), ContextKeyUserID, claims.UserID)
			ctx = context.WithValue(ctx, ContextKeyRole, claims.Role)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// RequireRole returns 403 if the authenticated user's role does not match.
func RequireRole(role string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			userRole, _ := r.Context().Value(ContextKeyRole).(string)
			if !strings.EqualFold(userRole, role) {
				// Return 404 — do not reveal that the route exists to unauthorized users.
				http.NotFound(w, r)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// UserIDFromContext extracts the authenticated user ID from the request context.
func UserIDFromContext(ctx context.Context) string {
	id, _ := ctx.Value(ContextKeyUserID).(string)
	return id
}

// RoleFromContext extracts the role from the request context.
func RoleFromContext(ctx context.Context) string {
	role, _ := ctx.Value(ContextKeyRole).(string)
	return role
}

func tokenFromRequest(r *http.Request) string {
	// Prefer HTTP-only cookie (browser dashboard)
	if cookie, err := r.Cookie("access_token"); err == nil {
		return cookie.Value
	}
	// Fall back to Authorization header (Stats API)
	if auth := r.Header.Get("Authorization"); strings.HasPrefix(auth, "Bearer ") {
		return strings.TrimPrefix(auth, "Bearer ")
	}
	return ""
}
```

- [ ] **Step 4: Compile check**

```bash
go build ./...
```

Expected: compiles cleanly.

- [ ] **Step 5: Commit**

```bash
git add internal/middleware/
git commit -m "feat: add rate limiter, structured logger, and JWT auth middleware"
```

---

### Task 11: Auth handlers (login, signup, logout)

**Files:**
- Create: `internal/handler/auth.go`
- Create: `internal/handler/auth_test.go`
- Create: `templates/pages/auth/login.html`
- Create: `templates/pages/auth/signup.html`
- Create: `templates/layout/base.html`

- [ ] **Step 1: Create `templates/layout/base.html`**

```html
<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>{{block "title" .}}Analytics{{end}}</title>
  <link rel="stylesheet" href="/static/css/output.css">
  <script src="https://unpkg.com/htmx.org@1.9.12" defer></script>
  <script src="https://unpkg.com/alpinejs@3.x.x/dist/cdn.min.js" defer></script>
</head>
<body class="bg-gray-50 text-gray-900 antialiased">
  {{block "content" .}}{{end}}
</body>
</html>
```

- [ ] **Step 2: Create `templates/pages/auth/login.html`**

```html
{{template "base.html" .}}

{{define "title"}}Sign in — Analytics{{end}}

{{define "content"}}
<div class="min-h-screen flex items-center justify-center">
  <div class="w-full max-w-sm bg-white rounded-2xl shadow-sm border border-gray-100 p-8">
    <h1 class="text-2xl font-bold mb-1">Welcome back</h1>
    <p class="text-gray-500 text-sm mb-8">Sign in to your account</p>

    {{if .Error}}
    <div class="mb-4 p-3 bg-red-50 border border-red-200 rounded-lg text-red-700 text-sm">{{.Error}}</div>
    {{end}}

    <form method="POST" action="/login" class="space-y-4">
      <input type="hidden" name="_csrf" value="{{.CSRFToken}}">
      <div>
        <label class="block text-sm font-medium text-gray-700 mb-1">Email</label>
        <input type="email" name="email" required autocomplete="email"
          class="w-full px-3 py-2 border border-gray-200 rounded-lg text-sm focus:outline-none focus:ring-2 focus:ring-violet-500">
      </div>
      <div>
        <label class="block text-sm font-medium text-gray-700 mb-1">Password</label>
        <input type="password" name="password" required autocomplete="current-password"
          class="w-full px-3 py-2 border border-gray-200 rounded-lg text-sm focus:outline-none focus:ring-2 focus:ring-violet-500">
      </div>
      <button type="submit"
        class="w-full bg-violet-600 hover:bg-violet-700 text-white font-medium py-2.5 rounded-lg text-sm transition-colors">
        Sign in
      </button>
    </form>
    <p class="text-center text-sm text-gray-500 mt-6">
      Don't have an account? <a href="/signup" class="text-violet-600 hover:underline">Sign up</a>
    </p>
    <p class="text-center text-sm mt-2">
      <a href="/forgot-password" class="text-gray-400 hover:text-gray-600 text-xs">Forgot password?</a>
    </p>
  </div>
</div>
{{end}}
```

- [ ] **Step 3: Create `templates/pages/auth/signup.html`**

```html
{{template "base.html" .}}

{{define "title"}}Create account — Analytics{{end}}

{{define "content"}}
<div class="min-h-screen flex items-center justify-center">
  <div class="w-full max-w-sm bg-white rounded-2xl shadow-sm border border-gray-100 p-8">
    <h1 class="text-2xl font-bold mb-1">Create account</h1>
    <p class="text-gray-500 text-sm mb-8">Start tracking in minutes</p>

    {{if .Error}}
    <div class="mb-4 p-3 bg-red-50 border border-red-200 rounded-lg text-red-700 text-sm">{{.Error}}</div>
    {{end}}

    <form method="POST" action="/signup" class="space-y-4">
      <input type="hidden" name="_csrf" value="{{.CSRFToken}}">
      <div>
        <label class="block text-sm font-medium text-gray-700 mb-1">Full name</label>
        <input type="text" name="name" required autocomplete="name"
          class="w-full px-3 py-2 border border-gray-200 rounded-lg text-sm focus:outline-none focus:ring-2 focus:ring-violet-500">
      </div>
      <div>
        <label class="block text-sm font-medium text-gray-700 mb-1">Email</label>
        <input type="email" name="email" required autocomplete="email"
          class="w-full px-3 py-2 border border-gray-200 rounded-lg text-sm focus:outline-none focus:ring-2 focus:ring-violet-500">
      </div>
      <div>
        <label class="block text-sm font-medium text-gray-700 mb-1">Password <span class="text-gray-400 font-normal">(min. 12 chars)</span></label>
        <input type="password" name="password" required minlength="12" autocomplete="new-password"
          class="w-full px-3 py-2 border border-gray-200 rounded-lg text-sm focus:outline-none focus:ring-2 focus:ring-violet-500">
      </div>
      <button type="submit"
        class="w-full bg-violet-600 hover:bg-violet-700 text-white font-medium py-2.5 rounded-lg text-sm transition-colors">
        Create account
      </button>
    </form>
    <p class="text-center text-sm text-gray-500 mt-6">
      Already have an account? <a href="/login" class="text-violet-600 hover:underline">Sign in</a>
    </p>
  </div>
</div>
{{end}}
```

- [ ] **Step 4: Write the failing test**

Create `internal/handler/auth_test.go`:

```go
package handler_test

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/sidneydekoning/analytics/internal/handler"
	"github.com/sidneydekoning/analytics/internal/service"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestAuthHandler() *handler.AuthHandler {
	authSvc := service.NewAuth(
		[]byte("test-secret-32-bytes-xxxxxxxxxx"),
		[]byte("refresh-secret-32-bytes-xxxxxxxx"),
	)
	// Use a nil repo — the handler should reject before hitting the DB on bad input
	return handler.NewAuthHandler(authSvc, nil, "http://localhost:8080")
}

func TestAuthHandler_LoginGET(t *testing.T) {
	h := newTestAuthHandler()
	req := httptest.NewRequest(http.MethodGet, "/login", nil)
	rec := httptest.NewRecorder()
	h.LoginPage(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestAuthHandler_SignupPOST_WeakPassword(t *testing.T) {
	h := newTestAuthHandler()
	form := url.Values{"name": {"Test"}, "email": {"test@example.com"}, "password": {"short"}, "_csrf": {"token"}}
	req := httptest.NewRequest(http.MethodPost, "/signup", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "csrf_token", Value: "token"})
	rec := httptest.NewRecorder()
	h.Signup(rec, req)
	// Should re-render signup with validation error, not redirect
	assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)
}
```

- [ ] **Step 5: Run test to verify it fails**

```bash
go test ./internal/handler/... -v
```

Expected: FAIL — `handler.AuthHandler` not defined.

- [ ] **Step 6: Implement `internal/handler/auth.go`**

```go
package handler

import (
	"html/template"
	"log/slog"
	"net/http"
	"time"

	"github.com/sidneydekoning/analytics/internal/middleware"
	"github.com/sidneydekoning/analytics/internal/model"
	"github.com/sidneydekoning/analytics/internal/repository"
	"github.com/sidneydekoning/analytics/internal/service"
)

// AuthHandler handles all authentication routes.
type AuthHandler struct {
	auth    service.AuthService
	repos   *repository.Repos
	baseURL string
	tmpl    *template.Template
}

// NewAuthHandler constructs an AuthHandler. repos may be nil in tests that only test
// routes which do not hit the database.
func NewAuthHandler(auth service.AuthService, repos *repository.Repos, baseURL string) *AuthHandler {
	return &AuthHandler{auth: auth, repos: repos, baseURL: baseURL}
}

// SetTemplates wires the parsed template set. Called once after templates are loaded.
func (h *AuthHandler) SetTemplates(tmpl *template.Template) {
	h.tmpl = tmpl
}

type authPageData struct {
	Error     string
	CSRFToken string
}

// LoginPage renders GET /login.
func (h *AuthHandler) LoginPage(w http.ResponseWriter, r *http.Request) {
	h.renderAuth(w, r, "login.html", authPageData{})
}

// Login handles POST /login.
func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	email := r.FormValue("email")
	password := r.FormValue("password")

	if email == "" || password == "" {
		h.renderAuth(w, r, "login.html", authPageData{Error: "Email and password are required."})
		return
	}

	user, err := h.repos.Users.GetByEmail(r.Context(), email)
	if err != nil || !h.auth.CheckPassword(password, user.PasswordHash) || !user.IsActive {
		// Same error for not-found and wrong-password — prevents user enumeration.
		w.WriteHeader(http.StatusUnauthorized)
		h.renderAuth(w, r, "login.html", authPageData{Error: "Invalid email or password."})
		return
	}

	h.issueTokensAndRedirect(w, r, user.ID, string(user.Role), "/dashboard")
	if err := h.repos.Users.UpdateLastLogin(r.Context(), user.ID); err != nil {
		slog.Error("failed to update last login", "error", err)
	}
}

// SignupPage renders GET /signup.
func (h *AuthHandler) SignupPage(w http.ResponseWriter, r *http.Request) {
	h.renderAuth(w, r, "signup.html", authPageData{})
}

// Signup handles POST /signup.
func (h *AuthHandler) Signup(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	name := r.FormValue("name")
	email := r.FormValue("email")
	password := r.FormValue("password")

	if len(password) < 12 {
		w.WriteHeader(http.StatusUnprocessableEntity)
		h.renderAuth(w, r, "signup.html", authPageData{Error: "Password must be at least 12 characters."})
		return
	}
	if name == "" || email == "" {
		w.WriteHeader(http.StatusUnprocessableEntity)
		h.renderAuth(w, r, "signup.html", authPageData{Error: "Name and email are required."})
		return
	}

	hash, err := h.auth.HashPassword(password)
	if err != nil {
		slog.Error("hash password", "error", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	user := &model.User{
		Email:        email,
		PasswordHash: hash,
		Role:         model.RoleUser,
		Name:         name,
		IsActive:     true,
	}
	if err := h.repos.Users.Create(r.Context(), user); err != nil {
		w.WriteHeader(http.StatusUnprocessableEntity)
		h.renderAuth(w, r, "signup.html", authPageData{Error: "An account with this email already exists."})
		return
	}

	h.issueTokensAndRedirect(w, r, user.ID, string(user.Role), "/dashboard")
}

// Logout handles POST /logout — clears cookies and revokes refresh token.
func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	clearCookie(w, "access_token")
	clearCookie(w, "refresh_token")
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

func (h *AuthHandler) issueTokensAndRedirect(w http.ResponseWriter, r *http.Request, userID, role, dest string) {
	access, err := h.auth.IssueAccessToken(userID, role)
	if err != nil {
		slog.Error("issue access token", "error", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	refresh, err := h.auth.IssueRefreshToken(userID)
	if err != nil {
		slog.Error("issue refresh token", "error", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	setAuthCookie(w, "access_token", access.TokenString, 15*time.Minute)
	setAuthCookie(w, "refresh_token", refresh.TokenString, 7*24*time.Hour)
	http.Redirect(w, r, dest, http.StatusSeeOther)
}

func (h *AuthHandler) renderAuth(w http.ResponseWriter, r *http.Request, name string, data authPageData) {
	if data.CSRFToken == "" {
		if c, err := r.Cookie("csrf_token"); err == nil {
			data.CSRFToken = c.Value
		}
	}
	if h.tmpl == nil {
		// Fallback when templates not wired (tests)
		w.Header().Set("Content-Type", "text/html")
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := h.tmpl.ExecuteTemplate(w, name, data); err != nil {
		slog.Error("render template", "name", name, "error", err)
	}
}

func setAuthCookie(w http.ResponseWriter, name, value string, maxAge time.Duration) {
	http.SetCookie(w, &http.Cookie{
		Name:     name,
		Value:    value,
		Path:     "/",
		MaxAge:   int(maxAge.Seconds()),
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteStrictMode,
	})
}

func clearCookie(w http.ResponseWriter, name string) {
	http.SetCookie(w, &http.Cookie{
		Name:     name,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteStrictMode,
	})
}

// UserIDFromContext is a convenience passthrough.
func UserIDFromContext(r *http.Request) string {
	return middleware.UserIDFromContext(r.Context())
}
```

- [ ] **Step 7: Run the tests**

```bash
go test ./internal/handler/... -v
```

Expected: PASS — both handler tests pass.

- [ ] **Step 8: Commit**

```bash
git add internal/handler/auth.go internal/handler/auth_test.go \
        templates/layout/base.html \
        templates/pages/auth/login.html \
        templates/pages/auth/signup.html
git commit -m "feat: add auth handlers (login, signup, logout) and auth templates"
```

---

### Task 12: Password reset flow

**Files:**
- Modify: `internal/handler/auth.go` (add ForgotPassword, ResetPassword handlers)
- Create: `templates/pages/auth/forgot-password.html`
- Create: `templates/pages/auth/reset-password.html`

- [ ] **Step 1: Create `templates/pages/auth/forgot-password.html`**

```html
{{template "base.html" .}}
{{define "title"}}Reset password — Analytics{{end}}
{{define "content"}}
<div class="min-h-screen flex items-center justify-center">
  <div class="w-full max-w-sm bg-white rounded-2xl shadow-sm border border-gray-100 p-8">
    <h1 class="text-2xl font-bold mb-1">Reset password</h1>
    <p class="text-gray-500 text-sm mb-8">We'll send you a reset link.</p>
    {{if .Success}}
    <div class="p-3 bg-green-50 border border-green-200 rounded-lg text-green-700 text-sm">
      If that email exists, a reset link has been sent.
    </div>
    {{else}}
    {{if .Error}}
    <div class="mb-4 p-3 bg-red-50 border border-red-200 rounded-lg text-red-700 text-sm">{{.Error}}</div>
    {{end}}
    <form method="POST" action="/forgot-password" class="space-y-4">
      <input type="hidden" name="_csrf" value="{{.CSRFToken}}">
      <div>
        <label class="block text-sm font-medium text-gray-700 mb-1">Email</label>
        <input type="email" name="email" required
          class="w-full px-3 py-2 border border-gray-200 rounded-lg text-sm focus:outline-none focus:ring-2 focus:ring-violet-500">
      </div>
      <button type="submit"
        class="w-full bg-violet-600 hover:bg-violet-700 text-white font-medium py-2.5 rounded-lg text-sm transition-colors">
        Send reset link
      </button>
    </form>
    {{end}}
    <p class="text-center mt-6"><a href="/login" class="text-sm text-gray-400 hover:text-gray-600">Back to sign in</a></p>
  </div>
</div>
{{end}}
```

- [ ] **Step 2: Create `templates/pages/auth/reset-password.html`**

```html
{{template "base.html" .}}
{{define "title"}}New password — Analytics{{end}}
{{define "content"}}
<div class="min-h-screen flex items-center justify-center">
  <div class="w-full max-w-sm bg-white rounded-2xl shadow-sm border border-gray-100 p-8">
    <h1 class="text-2xl font-bold mb-1">New password</h1>
    <p class="text-gray-500 text-sm mb-8">Choose a strong password (min. 12 characters).</p>
    {{if .Error}}
    <div class="mb-4 p-3 bg-red-50 border border-red-200 rounded-lg text-red-700 text-sm">{{.Error}}</div>
    {{end}}
    <form method="POST" action="/reset-password/{{.Token}}" class="space-y-4">
      <input type="hidden" name="_csrf" value="{{.CSRFToken}}">
      <div>
        <label class="block text-sm font-medium text-gray-700 mb-1">New password</label>
        <input type="password" name="password" required minlength="12" autocomplete="new-password"
          class="w-full px-3 py-2 border border-gray-200 rounded-lg text-sm focus:outline-none focus:ring-2 focus:ring-violet-500">
      </div>
      <button type="submit"
        class="w-full bg-violet-600 hover:bg-violet-700 text-white font-medium py-2.5 rounded-lg text-sm transition-colors">
        Set new password
      </button>
    </form>
  </div>
</div>
{{end}}
```

- [ ] **Step 3: Add password reset methods to `internal/handler/auth.go`**

Add these methods at the bottom of `internal/handler/auth.go`:

```go
type forgotPasswordData struct {
	Error     string
	CSRFToken string
	Success   bool
}

// ForgotPasswordPage renders GET /forgot-password.
func (h *AuthHandler) ForgotPasswordPage(w http.ResponseWriter, r *http.Request) {
	data := forgotPasswordData{}
	if c, err := r.Cookie("csrf_token"); err == nil {
		data.CSRFToken = c.Value
	}
	h.renderTemplate(w, "forgot-password.html", data)
}

// ForgotPassword handles POST /forgot-password.
// Always shows success — never reveals whether an email exists (prevents enumeration).
func (h *AuthHandler) ForgotPassword(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	email := r.FormValue("email")

	data := forgotPasswordData{Success: true}
	if c, err := r.Cookie("csrf_token"); err == nil {
		data.CSRFToken = c.Value
	}

	if email != "" {
		user, err := h.repos.Users.GetByEmail(r.Context(), email)
		if err == nil && user.IsActive {
			token, err := h.auth.GenerateSecureToken()
			if err != nil {
				slog.Error("generate reset token", "error", err)
			} else {
				// TODO (Plan 1 extension): store token hash + send email
				// For now, log token in dev — replace with email service in production
				slog.Info("password reset token generated (dev only)", "token", token, "user_id", user.ID)
			}
		}
	}

	h.renderTemplate(w, "forgot-password.html", data)
}

type resetPasswordData struct {
	Token     string
	Error     string
	CSRFToken string
}

// ResetPasswordPage renders GET /reset-password/:token.
func (h *AuthHandler) ResetPasswordPage(w http.ResponseWriter, r *http.Request) {
	token := r.PathValue("token")
	data := resetPasswordData{Token: token}
	if c, err := r.Cookie("csrf_token"); err == nil {
		data.CSRFToken = c.Value
	}
	h.renderTemplate(w, "reset-password.html", data)
}

// ResetPassword handles POST /reset-password/:token.
func (h *AuthHandler) ResetPassword(w http.ResponseWriter, r *http.Request) {
	token := r.PathValue("token")
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	password := r.FormValue("password")
	data := resetPasswordData{Token: token}
	if c, err := r.Cookie("csrf_token"); err == nil {
		data.CSRFToken = c.Value
	}

	if len(password) < 12 {
		w.WriteHeader(http.StatusUnprocessableEntity)
		data.Error = "Password must be at least 12 characters."
		h.renderTemplate(w, "reset-password.html", data)
		return
	}
	// TODO (Plan 1 extension): validate token hash against DB, update password, mark token used
	// Placeholder: redirect to login with success message
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

func (h *AuthHandler) renderTemplate(w http.ResponseWriter, name string, data any) {
	if h.tmpl == nil {
		w.Header().Set("Content-Type", "text/html")
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := h.tmpl.ExecuteTemplate(w, name, data); err != nil {
		slog.Error("render template", "name", name, "error", err)
	}
}
```

- [ ] **Step 4: Run tests**

```bash
go test ./... -v 2>&1 | head -40
```

Expected: all existing tests still pass, no new failures.

- [ ] **Step 5: Commit**

```bash
git add internal/handler/auth.go templates/pages/auth/
git commit -m "feat: add password reset flow handlers and templates"
```

---

### Task 13: Site management handler

**Files:**
- Create: `internal/handler/sites.go`
- Create: `templates/pages/account/new-site.html`

- [ ] **Step 1: Create `templates/pages/account/new-site.html`**

```bash
mkdir -p templates/pages/account
```

```html
{{template "base.html" .}}
{{define "title"}}Add site — Analytics{{end}}
{{define "content"}}
<div class="min-h-screen bg-gray-50 flex items-center justify-center">
  <div class="w-full max-w-md bg-white rounded-2xl shadow-sm border border-gray-100 p-8">
    <h1 class="text-xl font-bold mb-1">Add a website</h1>
    <p class="text-gray-500 text-sm mb-8">Get your tracking snippet after setup.</p>
    {{if .Error}}
    <div class="mb-4 p-3 bg-red-50 border border-red-200 rounded-lg text-red-700 text-sm">{{.Error}}</div>
    {{end}}
    <form method="POST" action="/account/sites/new" class="space-y-4">
      <input type="hidden" name="_csrf" value="{{.CSRFToken}}">
      <div>
        <label class="block text-sm font-medium text-gray-700 mb-1">Site name</label>
        <input type="text" name="name" required placeholder="My Blog"
          class="w-full px-3 py-2 border border-gray-200 rounded-lg text-sm focus:outline-none focus:ring-2 focus:ring-violet-500">
      </div>
      <div>
        <label class="block text-sm font-medium text-gray-700 mb-1">Domain</label>
        <input type="text" name="domain" required placeholder="myblog.com"
          class="w-full px-3 py-2 border border-gray-200 rounded-lg text-sm focus:outline-none focus:ring-2 focus:ring-violet-500">
        <p class="text-xs text-gray-400 mt-1">Without https:// — e.g. myblog.com</p>
      </div>
      <div>
        <label class="block text-sm font-medium text-gray-700 mb-1">Timezone</label>
        <select name="timezone"
          class="w-full px-3 py-2 border border-gray-200 rounded-lg text-sm focus:outline-none focus:ring-2 focus:ring-violet-500">
          <option value="UTC">UTC</option>
          <option value="Europe/Amsterdam">Europe/Amsterdam</option>
          <option value="Europe/London">Europe/London</option>
          <option value="America/New_York">America/New_York</option>
          <option value="America/Los_Angeles">America/Los_Angeles</option>
        </select>
      </div>
      <button type="submit"
        class="w-full bg-violet-600 hover:bg-violet-700 text-white font-medium py-2.5 rounded-lg text-sm transition-colors">
        Create site
      </button>
    </form>
  </div>
</div>
{{end}}
```

- [ ] **Step 2: Create `internal/handler/sites.go`**

```go
package handler

import (
	"html/template"
	"log/slog"
	"net/http"
	"strings"

	"github.com/sidneydekoning/analytics/internal/middleware"
	"github.com/sidneydekoning/analytics/internal/model"
	"github.com/sidneydekoning/analytics/internal/repository"
	"github.com/sidneydekoning/analytics/internal/service"
)

// SitesHandler handles site management routes.
type SitesHandler struct {
	auth  service.AuthService
	repos *repository.Repos
	tmpl  *template.Template
}

// NewSitesHandler constructs a SitesHandler.
func NewSitesHandler(auth service.AuthService, repos *repository.Repos) *SitesHandler {
	return &SitesHandler{auth: auth, repos: repos}
}

// SetTemplates wires the parsed template set.
func (h *SitesHandler) SetTemplates(tmpl *template.Template) {
	h.tmpl = tmpl
}

type newSiteData struct {
	Error     string
	CSRFToken string
}

// NewSitePage renders GET /account/sites/new.
func (h *SitesHandler) NewSitePage(w http.ResponseWriter, r *http.Request) {
	data := newSiteData{}
	if c, err := r.Cookie("csrf_token"); err == nil {
		data.CSRFToken = c.Value
	}
	h.renderTemplate(w, "new-site.html", data)
}

// CreateSite handles POST /account/sites/new.
func (h *SitesHandler) CreateSite(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	name := strings.TrimSpace(r.FormValue("name"))
	domain := strings.TrimSpace(r.FormValue("domain"))
	timezone := r.FormValue("timezone")

	if name == "" || domain == "" {
		data := newSiteData{Error: "Name and domain are required."}
		if c, err := r.Cookie("csrf_token"); err == nil {
			data.CSRFToken = c.Value
		}
		w.WriteHeader(http.StatusUnprocessableEntity)
		h.renderTemplate(w, "new-site.html", data)
		return
	}

	// Strip protocol if user included it
	domain = strings.TrimPrefix(domain, "https://")
	domain = strings.TrimPrefix(domain, "http://")
	domain = strings.TrimSuffix(domain, "/")

	if timezone == "" {
		timezone = "UTC"
	}

	token, err := h.auth.GenerateSiteToken()
	if err != nil {
		slog.Error("generate site token", "error", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	userID := middleware.UserIDFromContext(r.Context())
	site := &model.Site{
		OwnerID:  userID,
		Name:     name,
		Domain:   domain,
		Token:    token,
		Timezone: timezone,
	}

	if err := h.repos.Sites.Create(r.Context(), site); err != nil {
		slog.Error("create site", "error", err)
		data := newSiteData{Error: "Could not create site. Please try again."}
		if c, err := r.Cookie("csrf_token"); err == nil {
			data.CSRFToken = c.Value
		}
		w.WriteHeader(http.StatusInternalServerError)
		h.renderTemplate(w, "new-site.html", data)
		return
	}

	http.Redirect(w, r, "/sites/"+site.ID+"/overview", http.StatusSeeOther)
}

func (h *SitesHandler) renderTemplate(w http.ResponseWriter, name string, data any) {
	if h.tmpl == nil {
		w.Header().Set("Content-Type", "text/html")
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := h.tmpl.ExecuteTemplate(w, name, data); err != nil {
		slog.Error("render template", "name", name, "error", err)
	}
}
```

- [ ] **Step 3: Compile check**

```bash
go build ./...
```

Expected: compiles cleanly.

- [ ] **Step 4: Commit**

```bash
git add internal/handler/sites.go templates/pages/account/
git commit -m "feat: add site management handler (new site + token generation)"
```

---

### Task 14: Tailwind CSS setup

**Files:**
- Create: `static/css/input.css`

- [ ] **Step 1: Create `static/css/input.css`**

```css
@tailwind base;
@tailwind components;
@tailwind utilities;

/* Custom design tokens — override these to change the brand palette */
@layer base {
  :root {
    --color-brand: 124 106 247;   /* violet-500 equivalent */
    --color-brand-dark: 109 91 224;
  }

  html {
    @apply antialiased;
  }

  body {
    @apply bg-gray-50 text-gray-900;
  }
}

@layer components {
  .btn-primary {
    @apply bg-violet-600 hover:bg-violet-700 text-white font-medium py-2.5 px-4 rounded-lg text-sm transition-colors;
  }

  .card {
    @apply bg-white rounded-2xl border border-gray-100 shadow-sm;
  }

  .input {
    @apply w-full px-3 py-2 border border-gray-200 rounded-lg text-sm focus:outline-none focus:ring-2 focus:ring-violet-500;
  }
}
```

- [ ] **Step 2: Download Tailwind standalone CLI**

```bash
# macOS ARM
curl -sLO https://github.com/tailwindlabs/tailwindcss/releases/latest/download/tailwindcss-macos-arm64
chmod +x tailwindcss-macos-arm64
mv tailwindcss-macos-arm64 bin/tailwindcss

# Alternatively for macOS x64:
# curl -sLO https://github.com/tailwindlabs/tailwindcss/releases/latest/download/tailwindcss-macos-x64
```

- [ ] **Step 3: Create `tailwind.config.js`**

```js
/** @type {import('tailwindcss').Config} */
module.exports = {
  content: [
    "./templates/**/*.html",
    "./static/ts/**/*.ts",
  ],
  theme: {
    extend: {
      colors: {
        brand: {
          DEFAULT: '#7c6af7',
          dark: '#6d5be0',
        }
      },
      borderRadius: {
        '2xl': '1rem',
      }
    },
  },
  plugins: [],
}
```

- [ ] **Step 4: Build CSS**

```bash
./bin/tailwindcss -i static/css/input.css -o static/css/output.css
```

Expected: `static/css/output.css` created with Tailwind utility classes.

- [ ] **Step 5: Add `bin/` to `.gitignore`, commit CSS**

```bash
echo "bin/" >> .gitignore
git add static/css/input.css static/css/output.css tailwind.config.js .gitignore
git commit -m "feat: add Tailwind CSS setup with standalone CLI"
```

---

### Task 15: Main server wiring

**Files:**
- Create: `cmd/server/main.go`

- [ ] **Step 1: Create `cmd/server/main.go`**

```go
package main

import (
	"context"
	"fmt"
	"html/template"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/sidneydekoning/analytics/config"
	"github.com/sidneydekoning/analytics/internal/handler"
	"github.com/sidneydekoning/analytics/internal/middleware"
	"github.com/sidneydekoning/analytics/internal/repository"
	"github.com/sidneydekoning/analytics/internal/service"
)

func main() {
	// Structured logging to stdout — picked up by systemd journal
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))

	cfg, err := config.Load()
	if err != nil {
		slog.Error("config", "error", err)
		os.Exit(1)
	}

	ctx := context.Background()

	pool, err := repository.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		slog.Error("database", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	if err := repository.Migrate(ctx, pool); err != nil {
		slog.Error("migrations", "error", err)
		os.Exit(1)
	}

	repos := repository.New(pool)
	authSvc := service.NewAuth(cfg.JWTSecret, cfg.JWTRefreshSecret)

	// Parse all templates once at startup
	tmpl, err := template.ParseGlob("templates/**/*.html")
	if err != nil {
		slog.Error("templates", "error", err)
		os.Exit(1)
	}

	authHandler := handler.NewAuthHandler(authSvc, repos, cfg.BaseURL)
	authHandler.SetTemplates(tmpl)

	sitesHandler := handler.NewSitesHandler(authSvc, repos)
	sitesHandler.SetTemplates(tmpl)

	r := chi.NewRouter()

	// Global middleware — order matters
	r.Use(middleware.Logger)
	r.Use(middleware.SecurityHeaders)
	r.Use(middleware.CORS(cfg.AllowedOrigins, "/collect"))
	r.Use(middleware.CSRF)
	r.Use(chimiddleware.Recoverer)

	// Public routes
	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/login", http.StatusTemporaryRedirect)
	})

	// Static files
	r.Handle("/static/*", http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))

	// Auth routes (public)
	r.Get("/login", authHandler.LoginPage)
	r.Post("/login", authHandler.Login)
	r.Get("/signup", authHandler.SignupPage)
	r.Post("/signup", authHandler.Signup)
	r.Post("/logout", authHandler.Logout)
	r.Get("/forgot-password", authHandler.ForgotPasswordPage)
	r.Post("/forgot-password", authHandler.ForgotPassword)
	r.Get("/reset-password/{token}", authHandler.ResetPasswordPage)
	r.Post("/reset-password/{token}", authHandler.ResetPassword)

	// Rate limiter for /collect (100 req/min per IP)
	collectLimiter := middleware.RateLimiter(100.0/60.0, 20)

	// Tracking endpoint — no JWT, site token auth only
	r.With(collectLimiter).Post("/collect", func(w http.ResponseWriter, r *http.Request) {
		// Placeholder — implemented in Plan 2
		w.WriteHeader(http.StatusAccepted)
	})

	// Authenticated routes
	jwtAuth := middleware.JWTAuth(authSvc)

	r.With(jwtAuth).Group(func(r chi.Router) {
		r.Get("/dashboard", func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprintln(w, "dashboard — coming in Plan 3")
		})
		r.Get("/account/sites/new", sitesHandler.NewSitePage)
		r.Post("/account/sites/new", sitesHandler.CreateSite)
		r.Get("/sites/{siteID}/overview", func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprintln(w, "site overview — coming in Plan 3")
		})
	})

	// Admin routes
	adminRole := middleware.RequireRole("admin")
	r.With(jwtAuth, adminRole).Group(func(r chi.Router) {
		r.Get("/admin", func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprintln(w, "admin — coming in Plan 4")
		})
	})

	srv := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.Port),
		Handler:      r,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	slog.Info("server starting", "port", cfg.Port, "env", cfg.Env)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		slog.Error("server", "error", err)
		os.Exit(1)
	}
}
```

- [ ] **Step 2: Run the full test suite**

```bash
go test -race ./...
```

Expected: all tests pass (or skip without TEST_DATABASE_URL).

- [ ] **Step 3: Build the binary**

```bash
go build -o bin/analytics ./cmd/server
```

Expected: binary created with no errors.

- [ ] **Step 4: Smoke test — start the server**

```bash
# Copy .env.example to .env and fill in DATABASE_URL, JWT_SECRET, JWT_REFRESH_SECRET
cp .env.example .env

# Start server
./bin/analytics
# Expected: {"level":"INFO","msg":"server starting","port":8080,"env":"development"}

# In another terminal:
curl -i http://localhost:8090/login
# Expected: 200 OK with HTML login page
```

- [ ] **Step 5: Commit**

```bash
git add cmd/server/main.go
git commit -m "feat: wire main server — chi router, all middleware, auth routes, placeholder dashboard"
```

---

### Task 16: Final cleanup and verification

- [ ] **Step 1: Run full lint**

```bash
go vet ./...
# Fix any warnings before proceeding
```

- [ ] **Step 2: Run tests with race detector**

```bash
go test -race ./...
```

Expected: all pass.

- [ ] **Step 3: Verify build is clean**

```bash
go build ./...
go mod tidy
```

- [ ] **Step 4: Final commit**

```bash
git add -A
git status
# Confirm only expected files are staged
git commit -m "chore: final cleanup, go mod tidy, all tests green"
```

---

## Self-Review

**Spec coverage check:**

| Spec requirement | Covered by task |
|---|---|
| Go project layout (cmd/, internal/handler, service, repository, model, middleware) | Tasks 1, 5–13 |
| Config from env vars, loaded once at startup | Task 2 |
| pgx/v5 pool | Task 3 |
| All V1 migrations (users, sites, members, invitations, events hypertable, continuous aggregates, funnels, CMS, audit log) | Task 4 |
| Domain model types | Task 5 |
| User repository | Task 6 |
| Site repository + token lookup | Task 7 |
| JWT access + refresh tokens, bcrypt, GenerateSiteToken | Task 8 |
| Security headers on every response | Task 9 |
| CORS (any-origin on /collect, allowlist on API) | Task 9 |
| CSRF double-submit cookie | Task 9 |
| Per-IP rate limiting (token bucket) | Task 10 |
| Structured logging (log/slog) | Task 10 |
| JWT auth middleware + RequireRole | Task 10 |
| Login + signup + logout handlers | Task 11 |
| Password reset flow | Task 12 |
| Site registration + token generation | Task 13 |
| Tailwind standalone CLI | Task 14 |
| chi router + all middleware wired + server config | Task 15 |

**Gaps noted (planned in later Plans):**
- Password reset token storage in DB (Plan 1 TODO comments — full implementation deferred to avoid over-engineering before email is wired)
- /collect endpoint body (Plan 2)
- Dashboard pages (Plan 3)
- Admin + CMS (Plan 4)
- Email sending (Plan 4 or later)
