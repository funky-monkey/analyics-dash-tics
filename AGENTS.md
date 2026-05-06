# Agent Guidelines — Analytics SaaS

This file governs how Claude Code should approach every task in this project.
All agents working in this repo must follow these rules without exception.

---

## Project Overview

A privacy-first web analytics SaaS for marketers. Self-hosted on a single server.
Design inspiration: Framer Analytics (clean, minimal, beautiful).
Target: solo marketers, agencies, in-house teams, SaaS founders.

---

## Tech Stack

| Layer        | Technology                                                      |
|--------------|-----------------------------------------------------------------|
| Backend      | Go                                                              |
| Templating   | `html/template` (Go stdlib, server-rendered)                    |
| Interactivity| HTMX + Alpine.js (CDN)                                          |
| Styling      | Tailwind CSS — standalone CLI binary (no Node.js)               |
| Charts       | uPlot (time-series) + Chart.js (bar/pie/donut)                  |
| Database     | TimescaleDB (PostgreSQL extension)                              |
| Cache        | In-memory (`go-cache` with TTL) — no Redis                      |
| Rate limiting| `golang.org/x/time/rate` token bucket per IP — no Redis         |
| Deployment   | GitHub Actions → SSH/rsync → systemd on bare server             |
| Hosting      | Self-hosted, single server, EU region                           |

> No Node.js. No npm. No Docker. No Redis.
> Tailwind CLI is a single binary run in CI — no package.json.
> All pages are server-rendered by Go. Dynamic behaviour via HTMX + Alpine.js.
> Light mode only — no dark mode.

---

## Effective Go — Coding Style (Non-Negotiable)

All Go code must follow [Effective Go](https://go.dev/doc/effective_go) without exception.

### Naming
- Package names: lowercase, single word, no underscores (`handler`, `service`, `repository`).
- Exported types: `PascalCase`, descriptive (`SiteService`, `EventRepository`).
- Interfaces named by method + `-er` suffix where applicable (`Reader`, `Storer`, `Notifier`).
- No `Get` prefix on getters — `site.Owner()` not `site.GetOwner()`.
- Avoid stuttering — `site.Site{}` is wrong, `site.Record{}` is right.
- Variables: short in small scopes (`i`, `err`, `s`), descriptive in large scopes.
- Constants: `PascalCase` for exported, `camelCase` for unexported. `iota` for enumerations.

### Formatting
- `gofmt` (or `goimports`) runs on every file before commit. CI rejects unformatted code.
- Tabs for indentation. No manual line length limits.
- Opening brace on the same line — never on a new line.

### Error Handling
- Handle every error. Never assign to `_` on error returns in production paths.
- Wrap with context: `fmt.Errorf("service.CreateSite: %w", err)`.
- Guard clauses first — error path at the top, happy path runs down the page. No `else` after `return`.
- Never expose internal error details to HTTP responses — log the full error, return a generic message.
- Use `panic` only for programmer errors (impossible states). Never for expected runtime errors.
- `recover` only in top-level HTTP middleware to prevent a single goroutine from crashing the server.

### Functions & Methods
- Multiple return values for result + error — never use out-parameters.
- Named return values only when they genuinely clarify the signature (rare).
- `defer` for cleanup — always defer `rows.Close()`, file closes, unlock calls.
- Keep functions focused: if a function needs a long comment to explain what it does, split it.

### Interfaces
- Define interfaces at the consumer, not the implementor.
- Keep interfaces small — one or two methods. Compose larger interfaces from smaller ones.
- Check interface satisfaction at compile time: `var _ SiteStorer = (*siteRepository)(nil)`.

### Goroutines & Concurrency
- "Do not communicate by sharing memory; share memory by communicating."
- Every goroutine must have a clear owner responsible for its lifetime.
- Use `errgroup` for goroutines that can fail. Use `sync.WaitGroup` for fire-and-forget fan-out.
- Prefer buffered channels when the producer must not block.
- Run `go test -race ./...` in CI — zero race conditions tolerated.

### Testing Strategy — TDD + BDD

**TDD** (Test-Driven Development) is used at the unit and service layer:
- Write the failing test first using Go's `testing` package + `testify/assert` + `testify/require`.
- Table-driven tests for functions with multiple input scenarios.
- Run the test, verify it fails for the right reason, implement minimally to pass.
- Integration tests for repositories use a real TimescaleDB instance (never mock the DB).
- `go test -race ./...` in CI — zero race conditions tolerated.

**BDD** (Behaviour-Driven Development) is used at the handler and acceptance layer:
- Framework: **Ginkgo v2** + **Gomega** — the Go community standard for BDD.
- BDD specs live alongside the code they test: `internal/handler/auth_test.go` uses Ginkgo.
- Use `Describe` / `Context` / `It` blocks that read like acceptance criteria.
- `BeforeEach` for shared setup, `AfterEach` for teardown.
- `Expect(...)` with Gomega matchers (`To(Equal(...))`, `To(BeTrue())`, `To(HaveOccurred())`).
- Run with: `ginkgo ./...` or `go test ./...` (Ginkgo integrates with standard test runner).

**Layer → test style mapping:**
| Layer | Style | Rationale |
|---|---|---|
| `service/` | TDD (testify) | Pure functions, fast, table-driven |
| `repository/` | TDD integration (testify) | Real DB, verifies SQL correctness |
| `handler/` | BDD (Ginkgo) | Reads like "given/when/then" user behaviour |
| `middleware/` | TDD (testify) | Unit test each middleware in isolation |

**BDD example:**
```go
Describe("POST /login", func() {
    Context("with valid credentials", func() {
        It("sets an access_token cookie and redirects to /dashboard", func() {
            // ...
            Expect(resp.StatusCode).To(Equal(http.StatusSeeOther))
            Expect(resp.Header.Get("Location")).To(Equal("/dashboard"))
        })
    })
    Context("with wrong password", func() {
        It("returns 401 and renders the login page with an error", func() {
            // ...
            Expect(resp.StatusCode).To(Equal(http.StatusUnauthorized))
        })
    })
})
```

**Install:**
```bash
go get github.com/onsi/ginkgo/v2
go get github.com/onsi/gomega
go install github.com/onsi/ginkgo/v2/ginkgo
```

Bootstrap a BDD test file with: `ginkgo bootstrap` (in a package dir).

### Comments & Documentation
- Every exported type, function, and method has a doc comment.
- Doc comment starts with the name: `// SiteService handles business logic for sites.`
- No comments explaining *what* the code does — only *why*, when non-obvious.

---

## Go Backend — Non-Negotiable Rules

### SOLID Principles

- **Single Responsibility:** Every package, struct, and function has one reason to change.
  A handler handles HTTP. A service contains business logic. A repository touches the database. Never mix.
- **Open/Closed:** Extend behaviour through interfaces, not by modifying existing types.
- **Liskov Substitution:** Interfaces must be satisfied completely. No partial implementations.
- **Interface Segregation:** Keep interfaces small. Prefer many small interfaces over one large one.
  A `Reader` and a `Writer` are better than a `ReadWriter` unless both are always needed together.
- **Dependency Inversion:** High-level packages depend on interfaces, not concrete implementations.
  Wire dependencies at the top (main.go / cmd layer), not inside business logic.

### DRY — Don't Repeat Yourself

- Extract repeated logic into shared packages immediately — never copy-paste business logic.
- Shared utilities live in `internal/` — never in `pkg/` unless explicitly intended for external use.
- Database query patterns (pagination, soft-delete, tenant scoping) are written once and reused.

### Go Idioms & Best Practices

- **Project layout:** Follow the standard Go project layout:
  ```
  cmd/           — entry points (main packages)
  internal/      — private application code
    handler/     — HTTP handlers (thin, delegate to service)
    service/     — business logic
    repository/  — database access (one file per domain entity)
    model/       — domain types (no methods, pure data)
    middleware/  — HTTP middleware
  pkg/           — reusable code safe for external use (rare)
  ```
- **Error handling:** Always handle errors explicitly. Never `_` an error in production paths.
  Wrap errors with context: `fmt.Errorf("service.CreateSite: %w", err)`.
- **No global state:** No package-level variables except constants. Pass dependencies via constructors.
- **Context propagation:** Every function that does I/O accepts `context.Context` as its first argument.
- **Interfaces at the consumer:** Define interfaces where they are used, not where they are implemented.
- **Naming:** Short, clear names. `repo` not `repository`, `svc` not `service` in variable names.
  Exported types are descriptive: `SiteService`, `EventRepository`.
- **Tests:** Table-driven tests. Use `testify` for assertions. Every service method has a unit test.
  Integration tests for repositories use a real TimescaleDB instance (no mocks for DB).
- **HTTP:** Use `net/http` stdlib + a lightweight router (chi or stdlib mux).
  No heavy frameworks (no Gin, no Fiber) unless there is a compelling reason.
- **Concurrency:** Use channels and goroutines intentionally. Document why concurrency is needed.
  Prefer `sync.WaitGroup` and `errgroup` over raw goroutines.
- **Logging:** Structured logging with `log/slog` (stdlib). No fmt.Println in production paths.
- **Configuration:** All config via environment variables. Use a struct loaded at startup, never `os.Getenv` inside handlers.

---

## Database — TimescaleDB

- Analytics events are stored in **hypertables** partitioned by time (chunk interval: 1 day).
- **Continuous aggregates** are mandatory for all dashboard queries — never run raw aggregations
  over the full events table on user-facing requests.
- Multi-tenancy: every table with user data has a `site_id` column. Always scope queries by `site_id`.
- Migrations live in `migrations/` and are versioned sequentially (`001_init.sql`, `002_add_funnels.sql`).
- Use `pgx/v5` as the PostgreSQL driver. Never `database/sql` with `lib/pq`.

---

## Security — Non-Negotiable (applies to every feature, every PR)

Security is not a checklist to run at the end. Every handler, every query, every template
must be written with these rules active from the first line.

### XSS Prevention
- `html/template` is mandatory for all HTML rendering — never `text/template` for HTML output.
  `html/template` auto-escapes all values injected into HTML context. This is the first line of defence.
- Never use `template.HTML()`, `template.JS()`, `template.URL()` to bypass escaping unless the
  value is explicitly sanitised and the reason is documented in a comment.
- CMS content from Trix (stored as HTML) must be sanitised with `bluemonday` before storage
  AND before render — never trust stored HTML blindly.
- Content-Security-Policy header (see below) is the second line of defence against XSS.

### SQL Injection
- All database queries use `pgx/v5` parameterised queries — positional parameters (`$1`, `$2`).
- String interpolation into SQL is **never permitted** — not even for table or column names.
  Dynamic identifiers must use a strict allowlist checked in Go before the query is built.
- The repository layer is the only place SQL is written. Handlers and services never touch SQL.

### Security Headers (set on every response via middleware)
```
Strict-Transport-Security: max-age=63072000; includeSubDomains; preload
Content-Security-Policy:   default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; connect-src 'self'; frame-ancestors 'none'
X-Content-Type-Options:    nosniff
X-Frame-Options:           DENY
X-XSS-Protection:          0   (rely on CSP, not this legacy header)
Referrer-Policy:           strict-origin-when-cross-origin
Permissions-Policy:        camera=(), microphone=(), geolocation=()
```
A single `SecurityHeaders` middleware wraps every route. No handler sets headers individually.

### CORS
- CORS headers are set by a dedicated middleware, not per-handler.
- `/collect` endpoint: accepts requests from any origin (tracking script runs on customer sites).
  Allowed methods: `POST` only. No credentials. No CORS for GET on `/collect`.
- All `/api/v1/*` endpoints: allowed origins are configured via `ALLOWED_ORIGINS` env var.
  Default: same-origin only. Never `Access-Control-Allow-Origin: *` on authenticated endpoints.
- Preflight (`OPTIONS`) responses are handled by the middleware and never reach handlers.

### JWT & Authentication
- JWTs are stored in **HTTP-only, Secure, SameSite=Strict cookies** — never in localStorage or
  sessionStorage (XSS would steal them immediately).
- Algorithm: **HS256** minimum, **RS256** preferred when key rotation is needed.
- JWT expiry: **15 minutes** for access tokens. Refresh tokens: 7 days, stored HTTP-only.
- Refresh token rotation: every refresh issues a new refresh token and invalidates the old one.
  Invalidated tokens are stored in a `revoked_tokens` table (checked on refresh).
- JWT claims must include: `sub` (user UUID), `role`, `iat`, `exp`, `jti` (unique ID for revocation).
- Never log JWT values. Never include sensitive data (passwords, PII) in JWT payloads.
- Validate `exp`, `iat`, algorithm, and signature on every request — reject if any check fails.

### CSRF Protection
- All state-changing requests (POST/PUT/PATCH/DELETE) that use cookie-based auth require a
  CSRF token. Use the **double-submit cookie** pattern: Go sets a CSRF cookie (non-HTTP-only),
  the form submits it as a hidden field, the middleware compares them.
- HTMX requests include the CSRF token automatically via a request header configured at setup.
- The `/collect` endpoint is exempt from CSRF (it uses token auth, not cookies).

### Input Validation
- All user input is validated at the HTTP handler boundary before reaching the service layer.
- Use `github.com/go-playground/validator/v10` with struct tags for structured input.
- **Whitelist approach**: validate that input matches expected format/length/range.
  Never try to blacklist bad patterns — reject anything that doesn't match the whitelist.
- Sanitise file paths: never pass user input directly to filesystem operations.
  Use `filepath.Clean` and verify the result is within an allowed directory.
- Never use `os/exec` with user-supplied input. If shell commands are needed, arguments must be
  passed as separate slice elements, never interpolated into a command string.

### Password Security
- Passwords hashed with **bcrypt** (cost ≥ 12) or **argon2id** — never MD5/SHA1/SHA256 for passwords.
- Minimum password length: 12 characters. No maximum (long passwords are fine — hash them all).
- Timing-safe comparison for all secret comparisons: `subtle.ConstantTimeCompare`.
- Password reset tokens: 32 bytes from `crypto/rand`, stored as bcrypt hash, expire in 1 hour.

### Secrets & Configuration
- All secrets (DB password, JWT signing key, API keys) come from environment variables.
- A `Config` struct is populated once at startup from env vars. `os.Getenv` is never called
  inside handlers, services, or repositories.
- `.env` files are in `.gitignore`. `.env.example` contains only placeholder values.
- JWT signing keys are generated with `crypto/rand` — minimum 32 bytes for HMAC, 2048-bit RSA
  for asymmetric. Never hardcode keys, even in tests (use `crypto/rand` generated test keys).
- Never log env var values. Redact them from error messages.

### Rate Limiting & Abuse Prevention
- `/collect` endpoint: rate limited per IP using `golang.org/x/time/rate` token bucket.
  Limit: 100 events/minute per IP. Returns `429 Too Many Requests` when exceeded.
- Login endpoint: 5 failed attempts per IP per 15 minutes triggers a 15-minute lockout.
  Track failed attempts in-memory (go-cache) keyed by IP.
- All API endpoints: 1000 requests/minute per authenticated user.
- `User-Agent` and IP are logged on rate limit violations for abuse monitoring.

### TLS & Transport
- Minimum TLS version: **1.2**. Preferred: **1.3**.
- Disable deprecated cipher suites. Use Go's `tls.Config` with `MinVersion: tls.VersionTLS12`.
- HSTS preload header ensures browsers never connect over HTTP after first visit.
- HTTP → HTTPS redirect: all HTTP traffic redirected with `301 Moved Permanently`.

### IP Address Handling (Privacy + Security)
- IP addresses are **never stored**. They are used only for:
  1. Geolocation lookup (country/city) at ingestion time — result stored, IP discarded.
  2. Rate limiting (hashed in-memory only, not persisted).
- Geolocation: use MaxMind GeoLite2 database locally — never send IPs to third-party APIs.

### Dependency Security
- `govulncheck ./...` runs in CI on every push. Build fails if known vulnerabilities are found.
- `golangci-lint` with `gosec` linter enabled runs in CI. Build fails on security findings.
- `go mod tidy` runs in CI to detect missing or unused dependencies.
- Dependencies are pinned via `go.sum`. Never use `replace` directives pointing to local paths
  in production builds.
- Audit new dependencies before adding: check maintenance status, licence, known CVEs.

### Static Analysis in CI (required, blocks merge)
```
go vet ./...
golangci-lint run         (includes gosec, staticcheck, errcheck)
govulncheck ./...
go test -race ./...
```

### Error Responses
- HTTP error responses return generic messages to the client:
  `{"error": "internal server error"}` — never stack traces, SQL errors, or file paths.
- Full error details are logged server-side with request ID for traceability.
- 404 and 403 responses are identical in format — never reveal whether a resource exists
  to an unauthorised user (prevents enumeration).

### Admin Section Security
- `/admin/*` routes require `role = admin` check in a dedicated middleware — checked on every
  request, not just login.
- Admin actions (create user, delete content, change roles) are logged to an audit table
  with: actor user ID, action, target resource, timestamp, IP hash.
- Admin cannot be created via the signup flow — only via a CLI command or by another admin.

### CMS Security
- Trix HTML output is sanitised with `bluemonday` using a strict allowlist policy before
  storage. Sanitised again on render as defence-in-depth.
- Uploaded images (if any): validate MIME type by reading magic bytes, not file extension.
  Store outside the web root or in object storage — never directly in `static/`.
- Slug generation strips all non-alphanumeric characters. Slugs are validated against a
  regex allowlist (`^[a-z0-9-]+$`) before any database or filesystem operation.

---

## API Design

- REST with JSON. Version prefix: `/api/v1/`.
- Tracking endpoint: `/collect` — must be as fast as possible (no auth overhead, async DB write).
- All dashboard endpoints require bearer token auth.
- Rate limiting on `/collect` via Redis.
- Response envelope: `{ "data": ..., "meta": { "page": 1, "total": 100 } }` for lists.

---

## Frontend — JavaScript / TypeScript

All client-side JavaScript is written in **TypeScript** (strict mode). The following rules apply
to every `.ts` file in the project, including HTMX extensions, Alpine.js components, and chart modules.

### TypeScript Configuration

- `strict: true` always — no exceptions.
- No `any`. Use `unknown` and narrow with type guards when the type is truly unknown.
- Enable `noImplicitReturns`, `noFallthroughCasesInSwitch`, `exactOptionalPropertyTypes`.
- All modules use ES module syntax (`import` / `export`), never CommonJS `require`.

### SOLID Principles (TypeScript)

- **Single Responsibility:** Each module/class/function has one job. A chart module renders charts.
  A data-fetching module fetches data. They do not mix.
- **Open/Closed:** Extend behaviour via composition and interface extension, not by modifying
  existing classes. Prefer `class FunnelChart extends BaseChart` over modifying `BaseChart`.
- **Liskov Substitution:** Any class implementing an interface must honour the full contract.
  No throwing `NotImplementedError` on required methods.
- **Interface Segregation:** Prefer small, focused TypeScript interfaces. A `Renderable` and
  a `Fetchable` are better than a `RenderableAndFetchable` unless always used together.
- **Dependency Inversion:** High-level modules depend on abstractions (interfaces/types),
  not concrete implementations. Pass dependencies as constructor arguments.

### DRY — Don't Repeat Yourself

- Shared utilities live in `static/ts/lib/` — imported, never copy-pasted.
- Chart configuration defaults are defined once and spread/extended per chart type.
- API fetch patterns (with error handling, loading state) are written once as a reusable function.

### OOP Patterns

- Use classes for stateful UI components (charts, date pickers, real-time counters).
- Use plain functions for stateless transformations (formatting numbers, dates, percentages).
- Use the **Module pattern** for singletons that should not be instantiated more than once.
- Prefer **composition over inheritance** — inherit only when there is a genuine "is-a" relationship.
- Private fields use the `#` prefix (native JS private), not TypeScript `private` keyword.

### Best Practices

- **No side effects at module load time** — no DOM manipulation outside of explicit init calls.
- **Event delegation** over per-element listeners for lists and dynamic content.
- **Explicit return types** on all exported functions.
- **Error handling:** never silently swallow errors. Log with context, surface to UI where appropriate.
- **Async:** use `async/await` throughout — no raw `.then()` chains.
- **Naming:** `camelCase` for variables/functions, `PascalCase` for classes/interfaces/types,
  `SCREAMING_SNAKE_CASE` for module-level constants.
- **No global variables** — everything scoped to modules.
- **Tests:** Vitest for unit tests. Test pure functions and class methods. No tests for DOM glue code.

### File Structure

```
static/
  ts/
    lib/           — shared utilities (formatting, fetch, types)
    components/    — Alpine.js component definitions
    charts/        — chart wrappers (uPlot, Chart.js)
    pages/         — page-specific init files (one per route)
  css/
    main.css       — custom CSS layered on top of DaisyUI
```

---

## Deployment

- **GitHub Actions** for CI/CD: lint → test → tailwind build → go build → rsync binary to server → restart systemd service.
- **No Docker.** Go binary runs as a `systemd` service directly on the server.
- TimescaleDB is installed natively on the server as a PostgreSQL extension.
- Secrets via GitHub Actions secrets, never committed to the repo.
- Zero-downtime deploys: new binary copied to server, `systemctl restart analytics` — Go starts in milliseconds.
- Tailwind standalone CLI binary is downloaded in CI, runs once to compile CSS, output committed to `static/css/`.

---

## Privacy & Compliance Constraints

- The tracking script must never set cookies or store personal data.
- No IP addresses stored — hash and discard immediately on ingestion.
- EU data residency: server must be hosted in an EU region.
- Every new feature that touches user data must be reviewed against GDPR Article 5 principles.

---

## What NOT to Do

### Go & Architecture
- Do not add `vendor/` directory — use Go modules.
- Do not use ORM libraries (no GORM, no sqlc unless explicitly approved).
- Do not add dependencies without checking if stdlib covers the need first.
- Do not write SQL inline in handlers — all DB access goes through the repository layer.
- Do not commit `.env` files — use `.env.example` with placeholders.
- Do not skip tests for service layer code.
- Do not call `os.Getenv` inside handlers, services, or repositories — use the config struct.
- Do not use `text/template` for HTML output — always `html/template`.
- Do not use `fmt.Println` in production code — use `log/slog`.
- Do not use `panic` for expected runtime errors.
- Do not ignore the result of `go test -race` — zero races tolerated.

### Security
- Do not interpolate user input into SQL strings — always parameterised queries.
- Do not store JWTs in localStorage or sessionStorage — HTTP-only cookies only.
- Do not use `Access-Control-Allow-Origin: *` on authenticated endpoints.
- Do not store raw IP addresses — geolocate at ingestion and discard.
- Do not use MD5, SHA1, or unsalted SHA256 for passwords — bcrypt or argon2id only.
- Do not call `subtle.CompareByteSlices` or `==` for secret comparison — use `subtle.ConstantTimeCompare`.
- Do not bypass `html/template` escaping with `template.HTML()` without `bluemonday` sanitisation.
- Do not use `template.HTML()`, `template.JS()`, or `template.URL()` without an explicit comment explaining why it is safe.
- Do not expose stack traces, SQL errors, or file paths in HTTP responses.
- Do not hardcode secrets, API keys, or JWT signing keys — environment variables only.
- Do not skip `govulncheck` or `gosec` CI steps — they block merges.
- Do not use `os/exec` with user-supplied input.

### Frontend & Tooling
- Do not use `any` in TypeScript — use `unknown` and type guards.
- Do not write inline scripts in HTML templates — all JS lives in `static/ts/`.
- Do not use CommonJS `require()` — ES modules only.
- Do not install npm packages for functionality that HTMX or Alpine.js already cover.
- Do not use Redis — use in-memory caching (`go-cache`) and `golang.org/x/time/rate`.
- Do not use Docker — deploy Go binary directly via systemd.
- Do not add dark mode — light mode only.
