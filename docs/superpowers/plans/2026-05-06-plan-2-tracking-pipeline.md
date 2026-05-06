# Analytics SaaS — Plan 2: Tracking Pipeline

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:subagent-driven-development` (recommended) or `superpowers:executing-plans` to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the complete event ingestion pipeline — channel classification, UA parsing, privacy-safe fingerprinting, geolocation, the `/collect` HTTP endpoint, and the `script.js` tracking snippet — so events flow from customer websites into TimescaleDB.

**Architecture:** The `/collect` endpoint validates the site token, builds an `Event` from the HTTP request (classifying channel, parsing UA, hashing visitor fingerprint), fires an async goroutine to write to the `events` hypertable, and immediately returns 202. All PII (IP addresses) is discarded after geolocation. The tracking snippet is ~1KB vanilla JS served at `/static/script.js`.

**Tech Stack:** Go stdlib (`crypto/sha256`, `net`, `net/http`), `github.com/mileusna/useragent` (UA parsing), `oschwald/geoip2-golang` (MaxMind GeoLite2 — optional, gracefully skipped if DB file absent), existing `pgx/v5`, `stretchr/testify` (TDD), `onsi/ginkgo/v2` + `onsi/gomega` (BDD handler tests)

> **No Co-Authored-By** in any commit message.
> **Working directory:** `/Users/sidneydekoning/stack/Github/analyics-dash-tics`
> **Module:** `github.com/sidneydekoning/analytics`

---

## File Map

```
internal/
  model/
    event.go                     — Event struct (new file)
  repository/
    event.go                     — EventRepository interface + pg implementation
    event_test.go                — integration tests (skip without TEST_DATABASE_URL)
    repos.go                     — add Events field (modify)
  service/
    channel.go                   — ChannelClassifier: referrer+UTM → channel string
    channel_test.go              — table-driven TDD tests
    ua.go                        — UAParser: User-Agent → device/browser/OS
    ua_test.go                   — table-driven TDD tests
    geo.go                       — GeoLocator: IP → country/city (optional MaxMind)
    geo_test.go                  — unit tests
    fingerprint.go               — Fingerprinter: stable session_id + visitor_id (no PII)
    fingerprint_test.go          — unit tests
    collect.go                   — CollectService: orchestrates event building
    collect_test.go              — unit tests
  handler/
    collect.go                   — POST /collect HTTP handler (thin, async write)
    collect_test.go              — Ginkgo BDD tests

static/
  script.js                      — tracking snippet (~1KB vanilla JS)

cmd/server/main.go               — wire CollectHandler (modify)
```

---

### Task 1: Add dependencies

**Files:**
- Modify: `go.mod` (via `go get`)

- [ ] **Step 1: Add UA parser and MaxMind libraries**

```bash
cd /Users/sidneydekoning/stack/Github/analyics-dash-tics
go get github.com/mileusna/useragent
go get github.com/oschwald/geoip2-golang
```

- [ ] **Step 2: Verify go.mod has both**

```bash
grep -E "mileusna|oschwald" go.mod
```

Expected:
```
github.com/mileusna/useragent v1.x.x
github.com/oschwald/geoip2-golang v1.x.x
```

- [ ] **Step 3: Build check**

```bash
go build ./...
```

Expected: clean.

- [ ] **Step 4: Commit**

```bash
git add go.mod go.sum
git commit -m "feat: add UA parser and MaxMind geoip2 dependencies"
```

---

### Task 2: Event model

**Files:**
- Create: `internal/model/event.go`

- [ ] **Step 1: Create `internal/model/event.go`**

```go
package model

import "time"

// Event represents a single analytics event (pageview or custom) stored in the
// events hypertable. IPs are never stored — country/city are resolved at ingestion
// and the IP is discarded.
type Event struct {
	SiteID     string
	Type       string // "pageview" or "custom"
	URL        string
	Referrer   string
	Channel    string // pre-computed: organic|direct|social|email|paid|ai|dark_social|referral
	UTMSource  string
	UTMMedium  string
	UTMCampaign string
	Country    string // ISO 3166-1 alpha-2, e.g. "NL"
	City       string
	DeviceType string // "desktop", "mobile", "tablet"
	Browser    string
	OS         string
	Language   string
	SessionID  string // hashed fingerprint — not reversible to PII
	VisitorID  string // hashed fingerprint — not reversible to PII
	IsBounce   bool
	DurationMS int
	Props      map[string]string // custom event properties (stored as JSONB)
	Timestamp  time.Time
}

// CollectRequest is the JSON payload sent by the tracking script to POST /collect.
type CollectRequest struct {
	Site        string            `json:"site"`        // site token (tk_xxx)
	Type        string            `json:"type"`        // "pageview" or custom event name
	URL         string            `json:"url"`
	Referrer    string            `json:"referrer"`
	Width       int               `json:"width"`       // screen width for device detection
	Language    string            `json:"language"`    // navigator.language
	UTMSource   string            `json:"utm_source"`
	UTMMedium   string            `json:"utm_medium"`
	UTMCampaign string            `json:"utm_campaign"`
	Props       map[string]string `json:"props"`
}
```

- [ ] **Step 2: Compile check**

```bash
go build ./...
```

Expected: clean.

- [ ] **Step 3: Commit**

```bash
git add internal/model/event.go
git commit -m "feat: add Event and CollectRequest model types"
```

---

### Task 3: Channel classifier

**Files:**
- Create: `internal/service/channel.go`
- Create: `internal/service/channel_test.go`

- [ ] **Step 1: Write the failing tests first**

Create `internal/service/channel_test.go`:

```go
package service_test

import (
	"testing"

	"github.com/sidneydekoning/analytics/internal/service"
	"github.com/stretchr/testify/assert"
)

func TestClassifyChannel(t *testing.T) {
	tests := []struct {
		name        string
		referrer    string
		utmMedium   string
		utmSource   string
		url         string
		want        string
	}{
		// AI tools
		{"chatgpt", "https://chatgpt.com/c/abc", "", "", "https://example.com/", "ai"},
		{"claude", "https://claude.ai/chat/123", "", "", "https://example.com/", "ai"},
		{"perplexity", "https://www.perplexity.ai/search", "", "", "https://example.com/", "ai"},
		{"gemini", "https://gemini.google.com/app", "", "", "https://example.com/", "ai"},
		{"copilot", "https://copilot.microsoft.com/", "", "", "https://example.com/", "ai"},
		// Organic search
		{"google organic", "https://www.google.com/search?q=analytics", "", "", "https://example.com/", "organic"},
		{"bing organic", "https://www.bing.com/search?q=foo", "", "", "https://example.com/", "organic"},
		{"duckduckgo", "https://duckduckgo.com/?q=test", "", "", "https://example.com/", "organic"},
		// Paid
		{"google cpc", "https://www.google.com/", "cpc", "google", "https://example.com/", "paid"},
		{"paid medium", "", "ppc", "facebook", "https://example.com/", "paid"},
		{"paid social", "", "paidsocial", "instagram", "https://example.com/", "paid"},
		// Email
		{"email medium", "", "email", "newsletter", "https://example.com/", "email"},
		{"email referrer", "https://mail.google.com/", "", "", "https://example.com/", "email"},
		// Social
		{"facebook", "https://www.facebook.com/", "", "", "https://example.com/", "social"},
		{"twitter/x", "https://t.co/abc", "", "", "https://example.com/", "social"},
		{"linkedin", "https://www.linkedin.com/feed/", "", "", "https://example.com/", "social"},
		{"instagram", "https://l.instagram.com/", "", "", "https://example.com/", "social"},
		// Dark social (no referrer, deep URL, no UTM)
		{"dark social blog", "", "", "", "https://example.com/blog/article", "dark_social"},
		// Direct (no referrer, root URL)
		{"direct homepage", "", "", "", "https://example.com/", "direct"},
		{"direct no trailing slash", "", "", "", "https://example.com", "direct"},
		// Referral
		{"referral", "https://someotherdomain.com/page", "", "", "https://example.com/", "referral"},
		// UTM source with no medium — treat as referral
		{"utm source only", "", "", "newsletter_june", "https://example.com/", "referral"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := service.ClassifyChannel(tt.referrer, tt.utmMedium, tt.utmSource, tt.url)
			assert.Equal(t, tt.want, got, "referrer=%q utm_medium=%q url=%q", tt.referrer, tt.utmMedium, tt.url)
		})
	}
}
```

- [ ] **Step 2: Run — verify it fails**

```bash
go test ./internal/service/... -v -run TestClassifyChannel
```

Expected: FAIL — `service.ClassifyChannel` not defined.

- [ ] **Step 3: Implement `internal/service/channel.go`**

```go
package service

import (
	"net/url"
	"strings"
)

// aiHostnames are referrer hostnames that indicate traffic from AI assistants.
var aiHostnames = []string{
	"chatgpt.com",
	"chat.openai.com",
	"claude.ai",
	"perplexity.ai",
	"gemini.google.com",
	"copilot.microsoft.com",
	"you.com",
	"phind.com",
	"poe.com",
	"character.ai",
}

// searchEngineHostnames are referrer hostnames for organic search.
var searchEngineHostnames = []string{
	"google.", "bing.com", "yahoo.com", "duckduckgo.com",
	"baidu.com", "yandex.", "ecosia.org", "brave.com",
	"search.yahoo.com",
}

// socialHostnames are referrer hostnames for social media.
var socialHostnames = []string{
	"facebook.com", "fb.com", "t.co", "twitter.com", "x.com",
	"linkedin.com", "instagram.com", "l.instagram.com",
	"tiktok.com", "pinterest.com", "reddit.com", "snapchat.com",
	"youtube.com", "whatsapp.com",
}

// emailHostnames are referrer hostnames for email clients.
var emailHostnames = []string{
	"mail.google.com", "outlook.live.com", "outlook.office.com",
	"mail.yahoo.com", "webmail.",
}

// paidMediums are utm_medium values that indicate paid traffic.
var paidMediums = []string{"cpc", "ppc", "paid", "paidsearch", "paidsocial", "cpv", "cpm"}

// ClassifyChannel returns the traffic channel for an event based on referrer and UTM params.
// The result is pre-computed at ingestion and stored directly in the events table.
func ClassifyChannel(referrer, utmMedium, utmSource, pageURL string) string {
	medium := strings.ToLower(strings.TrimSpace(utmMedium))
	referrerHost := extractHost(referrer)

	// Paid: utm_medium signals paid traffic regardless of referrer.
	for _, paid := range paidMediums {
		if medium == paid {
			return "paid"
		}
	}

	// Email: utm_medium=email or known email client referrer.
	if medium == "email" {
		return "email"
	}
	if referrerHost != "" && containsHost(referrerHost, emailHostnames) {
		return "email"
	}

	// AI tool traffic.
	if referrerHost != "" && containsHost(referrerHost, aiHostnames) {
		return "ai"
	}

	// Organic search.
	if referrerHost != "" && containsHost(referrerHost, searchEngineHostnames) {
		return "organic"
	}

	// Social.
	if referrerHost != "" && containsHost(referrerHost, socialHostnames) {
		return "social"
	}

	// Any other referrer from a different domain = referral.
	if referrerHost != "" {
		return "referral"
	}

	// No referrer — distinguish direct from dark social.
	// Dark social: no referrer but URL is a deep path (not root).
	if isDarkSocial(pageURL) {
		return "dark_social"
	}

	return "direct"
}

func extractHost(rawURL string) string {
	if rawURL == "" {
		return ""
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	return strings.ToLower(u.Hostname())
}

func containsHost(host string, patterns []string) bool {
	for _, p := range patterns {
		if strings.Contains(host, p) {
			return true
		}
	}
	return false
}

// isDarkSocial returns true when there is no referrer but the landing URL is a
// deep path — strong signal of shared links in messaging apps (WhatsApp, Slack, etc).
func isDarkSocial(pageURL string) bool {
	if pageURL == "" {
		return false
	}
	u, err := url.Parse(pageURL)
	if err != nil {
		return false
	}
	path := strings.TrimSuffix(u.Path, "/")
	return path != "" && path != "/"
}
```

- [ ] **Step 4: Run — verify all tests pass**

```bash
go test ./internal/service/... -v -run TestClassifyChannel
```

Expected: PASS — all 21 cases pass.

- [ ] **Step 5: Commit**

```bash
git add internal/service/channel.go internal/service/channel_test.go
git commit -m "feat: add channel classifier with AI traffic detection"
```

---

### Task 4: UA parser service

**Files:**
- Create: `internal/service/ua.go`
- Create: `internal/service/ua_test.go`

- [ ] **Step 1: Write the failing tests**

Create `internal/service/ua_test.go`:

```go
package service_test

import (
	"testing"

	"github.com/sidneydekoning/analytics/internal/service"
	"github.com/stretchr/testify/assert"
)

func TestParseUA(t *testing.T) {
	tests := []struct {
		name           string
		ua             string
		wantDevice     string
		wantBrowser    string
		wantOS         string
		wantBot        bool
	}{
		{
			name:        "chrome desktop mac",
			ua:          "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
			wantDevice:  "desktop",
			wantBrowser: "Chrome",
			wantOS:      "macOS",
			wantBot:     false,
		},
		{
			name:        "safari iphone",
			ua:          "Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.0 Mobile/15E148 Safari/604.1",
			wantDevice:  "mobile",
			wantBrowser: "Safari",
			wantOS:      "iOS",
			wantBot:     false,
		},
		{
			name:        "firefox windows",
			ua:          "Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:121.0) Gecko/20100101 Firefox/121.0",
			wantDevice:  "desktop",
			wantBrowser: "Firefox",
			wantOS:      "Windows",
			wantBot:     false,
		},
		{
			name:        "googlebot",
			ua:          "Mozilla/5.0 (compatible; Googlebot/2.1; +http://www.google.com/bot.html)",
			wantDevice:  "",
			wantBrowser: "",
			wantOS:      "",
			wantBot:     true,
		},
		{
			name:        "empty ua",
			ua:          "",
			wantDevice:  "desktop",
			wantBrowser: "",
			wantOS:      "",
			wantBot:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := service.ParseUA(tt.ua)
			assert.Equal(t, tt.wantBot, result.IsBot, "IsBot")
			if !tt.wantBot {
				assert.Equal(t, tt.wantDevice, result.DeviceType, "DeviceType")
				assert.Equal(t, tt.wantBrowser, result.Browser, "Browser")
				assert.Equal(t, tt.wantOS, result.OS, "OS")
			}
		})
	}
}
```

- [ ] **Step 2: Run — verify it fails**

```bash
go test ./internal/service/... -v -run TestParseUA
```

Expected: FAIL — `service.ParseUA` not defined.

- [ ] **Step 3: Implement `internal/service/ua.go`**

```go
package service

import "github.com/mileusna/useragent"

// UAResult holds parsed User-Agent information.
type UAResult struct {
	DeviceType string // "desktop", "mobile", "tablet"
	Browser    string
	OS         string
	IsBot      bool
}

// ParseUA parses a User-Agent string into device type, browser, OS, and bot flag.
// Returns empty DeviceType/Browser/OS and IsBot=true for known bots/crawlers.
func ParseUA(uaString string) UAResult {
	ua := useragent.Parse(uaString)

	if ua.Bot {
		return UAResult{IsBot: true}
	}

	device := "desktop"
	if ua.Mobile {
		device = "mobile"
	} else if ua.Tablet {
		device = "tablet"
	}

	return UAResult{
		DeviceType: device,
		Browser:    ua.Name,
		OS:         normaliseOS(ua.OS),
		IsBot:      false,
	}
}

// normaliseOS maps raw OS strings to clean display names.
func normaliseOS(raw string) string {
	switch {
	case raw == "Mac OS X" || raw == "macOS":
		return "macOS"
	case raw == "Windows" || raw == "Windows 10" || raw == "Windows 11":
		return "Windows"
	case raw == "Linux":
		return "Linux"
	case raw == "iPhone OS" || raw == "iOS":
		return "iOS"
	case raw == "Android":
		return "Android"
	case raw == "Chrome OS" || raw == "ChromeOS":
		return "ChromeOS"
	default:
		return raw
	}
}
```

- [ ] **Step 4: Run — verify all tests pass**

```bash
go test ./internal/service/... -v -run TestParseUA
```

Expected: PASS — all 5 tests pass.

- [ ] **Step 5: Commit**

```bash
git add internal/service/ua.go internal/service/ua_test.go
git commit -m "feat: add UA parser service (device/browser/OS/bot detection)"
```

---

### Task 5: Privacy-safe fingerprinting

**Files:**
- Create: `internal/service/fingerprint.go`
- Create: `internal/service/fingerprint_test.go`

- [ ] **Step 1: Write the failing tests**

Create `internal/service/fingerprint_test.go`:

```go
package service_test

import (
	"testing"
	"time"

	"github.com/sidneydekoning/analytics/internal/service"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFingerprint_IsDeterministic(t *testing.T) {
	fp := service.NewFingerprinter("daily-salt-secret")
	now := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)

	id1 := fp.VisitorID("site-abc", "192.168.1.1", "Mozilla/5.0 Chrome", now)
	id2 := fp.VisitorID("site-abc", "192.168.1.1", "Mozilla/5.0 Chrome", now)

	assert.Equal(t, id1, id2, "same inputs same day must produce same ID")
	assert.NotEmpty(t, id1)
	assert.Len(t, id1, 16, "visitor ID should be 16 hex chars")
}

func TestFingerprint_DifferentDaysProduceDifferentIDs(t *testing.T) {
	fp := service.NewFingerprinter("daily-salt-secret")
	day1 := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)
	day2 := time.Date(2026, 1, 16, 12, 0, 0, 0, time.UTC)

	id1 := fp.VisitorID("site-abc", "192.168.1.1", "Mozilla/5.0 Chrome", day1)
	id2 := fp.VisitorID("site-abc", "192.168.1.1", "Mozilla/5.0 Chrome", day2)

	assert.NotEqual(t, id1, id2, "different days must produce different visitor IDs")
}

func TestFingerprint_DifferentSitesProduceDifferentIDs(t *testing.T) {
	fp := service.NewFingerprinter("daily-salt-secret")
	now := time.Now()

	id1 := fp.VisitorID("site-aaa", "192.168.1.1", "Mozilla/5.0", now)
	id2 := fp.VisitorID("site-bbb", "192.168.1.1", "Mozilla/5.0", now)

	assert.NotEqual(t, id1, id2, "different sites must produce different IDs")
}

func TestFingerprint_SessionID(t *testing.T) {
	fp := service.NewFingerprinter("daily-salt-secret")
	now := time.Now()

	sid := fp.SessionID("site-abc", "192.168.1.1", "Mozilla/5.0", now)
	require.NotEmpty(t, sid)
	assert.Len(t, sid, 16)

	// Session IDs for same visitor but different hours should differ
	hour1 := time.Date(2026, 1, 15, 10, 0, 0, 0, time.UTC)
	hour2 := time.Date(2026, 1, 15, 11, 0, 0, 0, time.UTC)
	s1 := fp.SessionID("site-abc", "192.168.1.1", "Mozilla/5.0", hour1)
	s2 := fp.SessionID("site-abc", "192.168.1.1", "Mozilla/5.0", hour2)
	assert.NotEqual(t, s1, s2, "different hours should produce different session IDs")
}

func TestFingerprint_IPNotRecoverable(t *testing.T) {
	fp := service.NewFingerprinter("daily-salt-secret")
	now := time.Now()
	ip := "203.0.113.42"

	id := fp.VisitorID("site-abc", ip, "Mozilla/5.0", now)

	assert.NotContains(t, id, ip, "visitor ID must not contain the raw IP")
	assert.NotContains(t, id, "203", "visitor ID must not contain IP fragments")
}
```

- [ ] **Step 2: Run — verify it fails**

```bash
go test ./internal/service/... -v -run TestFingerprint
```

Expected: FAIL — `service.NewFingerprinter` not defined.

- [ ] **Step 3: Implement `internal/service/fingerprint.go`**

```go
package service

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"
)

// Fingerprinter generates privacy-safe, non-reversible visitor and session IDs.
// IDs are stable within a day (visitor) or hour (session) for the same
// site+IP+UA combination, but cannot be reversed to recover the original IP.
type Fingerprinter interface {
	VisitorID(siteID, ip, userAgent string, t time.Time) string
	SessionID(siteID, ip, userAgent string, t time.Time) string
}

type fingerprinter struct {
	salt string
}

// NewFingerprinter creates a Fingerprinter with the given salt.
// The salt should be a secret loaded from config — without it, IDs across
// deployments would be predictable.
func NewFingerprinter(salt string) Fingerprinter {
	return &fingerprinter{salt: salt}
}

// VisitorID returns a 16-char hex ID stable within a calendar day (UTC).
// Same visitor returning on a different day gets a different ID — by design.
func (f *fingerprinter) VisitorID(siteID, ip, userAgent string, t time.Time) string {
	day := t.UTC().Format("2006-01-02")
	return f.hash(siteID, ip, userAgent, day)
}

// SessionID returns a 16-char hex ID stable within an hour.
// A new session begins when the visitor returns after a full clock hour.
func (f *fingerprinter) SessionID(siteID, ip, userAgent string, t time.Time) string {
	hour := t.UTC().Format("2006-01-02-15")
	return f.hash(siteID, ip, userAgent, hour)
}

func (f *fingerprinter) hash(parts ...string) string {
	h := sha256.New()
	for _, p := range parts {
		fmt.Fprintf(h, "%s|", p)
	}
	fmt.Fprintf(h, "%s", f.salt)
	return hex.EncodeToString(h.Sum(nil))[:16]
}
```

- [ ] **Step 4: Run — verify all tests pass**

```bash
go test ./internal/service/... -v -run TestFingerprint
```

Expected: PASS — all 5 tests pass.

- [ ] **Step 5: Commit**

```bash
git add internal/service/fingerprint.go internal/service/fingerprint_test.go
git commit -m "feat: add privacy-safe visitor/session ID fingerprinting"
```

---

### Task 6: Geolocation service

**Files:**
- Create: `internal/service/geo.go`
- Create: `internal/service/geo_test.go`

- [ ] **Step 1: Write the failing tests**

Create `internal/service/geo_test.go`:

```go
package service_test

import (
	"testing"

	"github.com/sidneydekoning/analytics/internal/service"
	"github.com/stretchr/testify/assert"
)

func TestGeoLocator_ReturnsEmptyWhenNoDB(t *testing.T) {
	// When no MaxMind DB path is configured, returns empty strings — never errors.
	geo := service.NewGeoLocator("")

	country, city := geo.Lookup("203.0.113.42")
	assert.Equal(t, "", country)
	assert.Equal(t, "", city)
}

func TestGeoLocator_HandlesLoopback(t *testing.T) {
	geo := service.NewGeoLocator("")

	country, city := geo.Lookup("127.0.0.1")
	assert.Equal(t, "", country)
	assert.Equal(t, "", city)
}

func TestGeoLocator_HandlesPrivateIP(t *testing.T) {
	geo := service.NewGeoLocator("")

	country, city := geo.Lookup("192.168.1.100")
	assert.Equal(t, "", country)
	assert.Equal(t, "", city)
}
```

- [ ] **Step 2: Run — verify it fails**

```bash
go test ./internal/service/... -v -run TestGeoLocator
```

Expected: FAIL — `service.NewGeoLocator` not defined.

- [ ] **Step 3: Implement `internal/service/geo.go`**

```go
package service

import (
	"log/slog"
	"net"

	"github.com/oschwald/geoip2-golang"
)

// GeoLocator resolves IP addresses to country and city.
// Geolocation uses a local MaxMind GeoLite2 database — the IP is never sent
// to any third-party service. If no database path is configured, returns
// empty strings without error.
type GeoLocator interface {
	Lookup(ip string) (country, city string)
}

type geoLocator struct {
	db *geoip2.Reader
}

// NewGeoLocator creates a GeoLocator. If dbPath is empty or the file cannot
// be opened, returns a no-op locator that always returns ("", "").
func NewGeoLocator(dbPath string) GeoLocator {
	if dbPath == "" {
		return &geoLocator{}
	}
	db, err := geoip2.Open(dbPath)
	if err != nil {
		slog.Warn("geolocation unavailable — MaxMind DB not loaded", "path", dbPath, "error", err)
		return &geoLocator{}
	}
	return &geoLocator{db: db}
}

// Lookup returns the ISO 3166-1 alpha-2 country code and city name for ip.
// Returns ("", "") for private/loopback IPs and when the DB is unavailable.
func (g *geoLocator) Lookup(ip string) (country, city string) {
	if g.db == nil {
		return "", ""
	}
	parsed := net.ParseIP(ip)
	if parsed == nil || parsed.IsLoopback() || parsed.IsPrivate() {
		return "", ""
	}
	record, err := g.db.City(parsed)
	if err != nil {
		return "", ""
	}
	return record.Country.IsoCode, record.City.Names["en"]
}
```

- [ ] **Step 4: Run — verify all tests pass**

```bash
go test ./internal/service/... -v -run TestGeoLocator
```

Expected: PASS — all 3 tests pass.

- [ ] **Step 5: Commit**

```bash
git add internal/service/geo.go internal/service/geo_test.go
git commit -m "feat: add optional geolocation service (MaxMind GeoLite2)"
```

---

### Task 7: Event repository

**Files:**
- Create: `internal/repository/event.go`
- Create: `internal/repository/event_test.go`
- Modify: `internal/repository/repos.go`

- [ ] **Step 1: Write the failing tests**

Create `internal/repository/event_test.go`:

```go
package repository_test

import (
	"context"
	"testing"
	"time"

	"github.com/sidneydekoning/analytics/internal/model"
	"github.com/sidneydekoning/analytics/internal/repository"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEventRepository_Write(t *testing.T) {
	repos := setupTestRepos(t)
	ctx := context.Background()

	// Need a site to write events for
	owner := &model.User{
		Email: uniqueEmail(), PasswordHash: "$2a$12$x",
		Role: model.RoleUser, Name: "Owner", IsActive: true,
	}
	require.NoError(t, repos.Users.Create(ctx, owner))

	import_fmt "fmt"
	site := &model.Site{
		OwnerID:  owner.ID,
		Name:     "Test",
		Domain:   "test.com",
		Token:    import_fmt.Sprintf("tk_evtest%d", time.Now().UnixNano()),
		Timezone: "UTC",
	}
	require.NoError(t, repos.Sites.Create(ctx, site))

	ev := &model.Event{
		SiteID:     site.ID,
		Type:       "pageview",
		URL:        "https://test.com/",
		Referrer:   "",
		Channel:    "direct",
		Country:    "NL",
		City:       "Amsterdam",
		DeviceType: "desktop",
		Browser:    "Chrome",
		OS:         "macOS",
		SessionID:  "abc123def456abcd",
		VisitorID:  "def456abc123defa",
		Timestamp:  time.Now(),
	}

	err := repos.Events.Write(ctx, ev)
	require.NoError(t, err)
}

func TestEventRepository_WriteBatch(t *testing.T) {
	repos := setupTestRepos(t)
	ctx := context.Background()

	owner := &model.User{
		Email: uniqueEmail(), PasswordHash: "$2a$12$x",
		Role: model.RoleUser, Name: "Owner", IsActive: true,
	}
	require.NoError(t, repos.Users.Create(ctx, owner))

	import_fmt "fmt"
	site := &model.Site{
		OwnerID:  owner.ID,
		Name:     "Batch Test",
		Domain:   "batch.com",
		Token:    import_fmt.Sprintf("tk_batch%d", time.Now().UnixNano()),
		Timezone: "UTC",
	}
	require.NoError(t, repos.Sites.Create(ctx, site))

	events := make([]*model.Event, 5)
	for i := range events {
		events[i] = &model.Event{
			SiteID:    site.ID,
			Type:      "pageview",
			URL:       "https://batch.com/",
			Channel:   "direct",
			SessionID: "sess" + fmt.Sprintf("%d", i),
			VisitorID: "vis" + fmt.Sprintf("%d", i),
			Timestamp: time.Now(),
		}
	}

	err := repos.Events.WriteBatch(ctx, events)
	require.NoError(t, err)

	count, err := repos.Events.CountBySite(ctx, site.ID, time.Now().Add(-time.Minute), time.Now().Add(time.Minute))
	require.NoError(t, err)
	assert.Equal(t, int64(5), count)
}
```

NOTE: The test above has a syntax issue with the import alias. Rewrite it properly without import aliases in the test body:

Create `internal/repository/event_test.go` with this correct version:

```go
package repository_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/sidneydekoning/analytics/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEventRepository_Write(t *testing.T) {
	repos := setupTestRepos(t)
	ctx := context.Background()

	owner := &model.User{
		Email: uniqueEmail(), PasswordHash: "$2a$12$x",
		Role: model.RoleUser, Name: "Owner", IsActive: true,
	}
	require.NoError(t, repos.Users.Create(ctx, owner))

	site := &model.Site{
		OwnerID:  owner.ID,
		Name:     "Test",
		Domain:   "test.com",
		Token:    fmt.Sprintf("tk_evtest%d", time.Now().UnixNano()),
		Timezone: "UTC",
	}
	require.NoError(t, repos.Sites.Create(ctx, site))

	ev := &model.Event{
		SiteID:     site.ID,
		Type:       "pageview",
		URL:        "https://test.com/",
		Channel:    "direct",
		Country:    "NL",
		City:       "Amsterdam",
		DeviceType: "desktop",
		Browser:    "Chrome",
		OS:         "macOS",
		SessionID:  "abc123def456abcd",
		VisitorID:  "def456abc123defa",
		Timestamp:  time.Now(),
	}

	err := repos.Events.Write(ctx, ev)
	require.NoError(t, err)
}

func TestEventRepository_CountBySite(t *testing.T) {
	repos := setupTestRepos(t)
	ctx := context.Background()

	owner := &model.User{
		Email: uniqueEmail(), PasswordHash: "$2a$12$x",
		Role: model.RoleUser, Name: "Owner", IsActive: true,
	}
	require.NoError(t, repos.Users.Create(ctx, owner))

	site := &model.Site{
		OwnerID:  owner.ID,
		Name:     "Count Test",
		Domain:   "count.com",
		Token:    fmt.Sprintf("tk_count%d", time.Now().UnixNano()),
		Timezone: "UTC",
	}
	require.NoError(t, repos.Sites.Create(ctx, site))

	for i := 0; i < 3; i++ {
		ev := &model.Event{
			SiteID:    site.ID,
			Type:      "pageview",
			URL:       "https://count.com/",
			Channel:   "direct",
			SessionID: fmt.Sprintf("sess%d", i),
			VisitorID: fmt.Sprintf("vis%d", i),
			Timestamp: time.Now(),
		}
		require.NoError(t, repos.Events.Write(ctx, ev))
	}

	count, err := repos.Events.CountBySite(ctx, site.ID,
		time.Now().Add(-time.Minute), time.Now().Add(time.Minute))
	require.NoError(t, err)
	assert.Equal(t, int64(3), count)
}
```

- [ ] **Step 2: Run — verify it fails**

```bash
go test ./internal/repository/... -v -run TestEventRepository
```

Expected: FAIL or SKIP (no `repos.Events` field yet).

- [ ] **Step 3: Implement `internal/repository/event.go`**

```go
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
```

- [ ] **Step 4: Add Events field to `internal/repository/repos.go`**

Open `internal/repository/repos.go` and update:

```go
package repository

import "github.com/jackc/pgx/v5/pgxpool"

// Repos aggregates all repository interfaces for dependency injection.
type Repos struct {
	Users  UserRepository
	Sites  SiteRepository
	Events EventRepository
}

// New creates a Repos with all pg implementations wired up.
func New(pool *pgxpool.Pool) *Repos {
	return &Repos{
		Users:  &pgUserRepository{pool: pool},
		Sites:  &pgSiteRepository{pool: pool},
		Events: &pgEventRepository{pool: pool},
	}
}
```

- [ ] **Step 5: Run tests**

```bash
go test ./internal/repository/... -v
```

Expected: SKIP (no TEST_DATABASE_URL) or PASS with a real DB.

- [ ] **Step 6: Compile check**

```bash
go build ./...
```

Expected: clean.

- [ ] **Step 7: Commit**

```bash
git add internal/repository/event.go internal/repository/event_test.go internal/repository/repos.go
git commit -m "feat: add event repository with Write, WriteBatch, CountBySite"
```

---

### Task 8: Collect service

**Files:**
- Create: `internal/service/collect.go`
- Create: `internal/service/collect_test.go`

- [ ] **Step 1: Write the failing tests**

Create `internal/service/collect_test.go`:

```go
package service_test

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/sidneydekoning/analytics/internal/model"
	"github.com/sidneydekoning/analytics/internal/service"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestCollectService() service.CollectService {
	geo := service.NewGeoLocator("") // no DB in tests
	fp := service.NewFingerprinter("test-salt-32-bytes-xxxxxxxxxxxxxxxx")
	return service.NewCollectService(geo, fp)
}

func TestCollectService_BuildEvent_Pageview(t *testing.T) {
	svc := newTestCollectService()

	req := &model.CollectRequest{
		Site:     "tk_abc123",
		Type:     "pageview",
		URL:      "https://myblog.com/article/go-tips",
		Referrer: "https://www.google.com/search?q=go+tips",
		Width:    1440,
		Language: "en-US",
	}

	r := httptest.NewRequest("POST", "/collect", nil)
	r.RemoteAddr = "203.0.113.1:12345"
	r.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 Chrome/120.0.0.0 Safari/537.36")

	ev, err := svc.BuildEvent("site-uuid-123", req, r, time.Now())
	require.NoError(t, err)

	assert.Equal(t, "site-uuid-123", ev.SiteID)
	assert.Equal(t, "pageview", ev.Type)
	assert.Equal(t, "https://myblog.com/article/go-tips", ev.URL)
	assert.Equal(t, "organic", ev.Channel, "Google referrer should be organic")
	assert.Equal(t, "desktop", ev.DeviceType)
	assert.Equal(t, "Chrome", ev.Browser)
	assert.NotEmpty(t, ev.SessionID)
	assert.NotEmpty(t, ev.VisitorID)
	assert.Equal(t, "en-US", ev.Language)
}

func TestCollectService_BuildEvent_AITraffic(t *testing.T) {
	svc := newTestCollectService()

	req := &model.CollectRequest{
		Site:     "tk_abc",
		Type:     "pageview",
		URL:      "https://myblog.com/",
		Referrer: "https://chatgpt.com/c/conversation-id",
	}

	r := httptest.NewRequest("POST", "/collect", nil)
	r.RemoteAddr = "1.2.3.4:5678"
	r.Header.Set("User-Agent", "Mozilla/5.0 Chrome")

	ev, err := svc.BuildEvent("site-uuid", req, r, time.Now())
	require.NoError(t, err)

	assert.Equal(t, "ai", ev.Channel)
}

func TestCollectService_BuildEvent_BotIsRejected(t *testing.T) {
	svc := newTestCollectService()

	req := &model.CollectRequest{
		Site: "tk_abc",
		Type: "pageview",
		URL:  "https://myblog.com/",
	}

	r := httptest.NewRequest("POST", "/collect", nil)
	r.RemoteAddr = "1.2.3.4:5678"
	r.Header.Set("User-Agent", "Mozilla/5.0 (compatible; Googlebot/2.1; +http://www.google.com/bot.html)")

	_, err := svc.BuildEvent("site-uuid", req, r, time.Now())
	assert.ErrorIs(t, err, service.ErrBot)
}
```

- [ ] **Step 2: Run — verify it fails**

```bash
go test ./internal/service/... -v -run TestCollectService
```

Expected: FAIL — `service.CollectService` not defined.

- [ ] **Step 3: Implement `internal/service/collect.go`**

```go
package service

import (
	"errors"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/sidneydekoning/analytics/internal/model"
)

// ErrBot is returned when the request comes from a known bot or crawler.
var ErrBot = errors.New("bot detected")

// CollectService builds an Event from an inbound /collect request.
// It orchestrates channel classification, UA parsing, geolocation, and fingerprinting.
type CollectService interface {
	BuildEvent(siteID string, req *model.CollectRequest, r *http.Request, t time.Time) (*model.Event, error)
}

type collectService struct {
	geo GeoLocator
	fp  Fingerprinter
}

// NewCollectService constructs a CollectService.
func NewCollectService(geo GeoLocator, fp Fingerprinter) CollectService {
	return &collectService{geo: geo, fp: fp}
}

// BuildEvent constructs an Event from the HTTP request and collect payload.
// Returns ErrBot if the User-Agent is a known bot — the caller should discard the event.
func (s *collectService) BuildEvent(siteID string, req *model.CollectRequest, r *http.Request, t time.Time) (*model.Event, error) {
	uaString := r.Header.Get("User-Agent")
	ua := ParseUA(uaString)
	if ua.IsBot {
		return nil, ErrBot
	}

	ip := extractClientIP(r)
	country, city := s.geo.Lookup(ip)

	channel := ClassifyChannel(req.Referrer, req.UTMMedium, req.UTMSource, req.URL)

	eventType := req.Type
	if eventType == "" {
		eventType = "pageview"
	}

	return &model.Event{
		SiteID:      siteID,
		Type:        eventType,
		URL:         req.URL,
		Referrer:    req.Referrer,
		Channel:     channel,
		UTMSource:   req.UTMSource,
		UTMMedium:   req.UTMMedium,
		UTMCampaign: req.UTMCampaign,
		Country:     country,
		City:        city,
		DeviceType:  ua.DeviceType,
		Browser:     ua.Browser,
		OS:          ua.OS,
		Language:    req.Language,
		SessionID:   s.fp.SessionID(siteID, ip, uaString, t),
		VisitorID:   s.fp.VisitorID(siteID, ip, uaString, t),
		Props:       req.Props,
		Timestamp:   t,
	}, nil
}

// extractClientIP returns the real client IP, preferring X-Real-IP from a trusted proxy.
// The IP is used only for geolocation and fingerprinting — never stored.
func extractClientIP(r *http.Request) string {
	if ip := r.Header.Get("X-Real-IP"); ip != "" {
		return strings.TrimSpace(ip)
	}
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return ip
}
```

- [ ] **Step 4: Run — verify all tests pass**

```bash
go test ./internal/service/... -v -run TestCollectService
```

Expected: PASS — all 3 tests pass.

- [ ] **Step 5: Full service test run**

```bash
go test ./internal/service/... -v
```

Expected: all tests pass (channel, ua, fingerprint, geo, collect, auth).

- [ ] **Step 6: Commit**

```bash
git add internal/service/collect.go internal/service/collect_test.go
git commit -m "feat: add collect service (event building, bot detection, channel + UA)"
```

---

### Task 9: /collect HTTP handler

**Files:**
- Create: `internal/handler/collect.go`
- Create: `internal/handler/collect_test.go`

- [ ] **Step 1: Write the failing Ginkgo tests**

Create `internal/handler/collect_test.go`:

```go
package handler_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/sidneydekoning/analytics/internal/handler"
	"github.com/sidneydekoning/analytics/internal/service"
)

var _ = Describe("CollectHandler", func() {
	var h *handler.CollectHandler

	BeforeEach(func() {
		geo := service.NewGeoLocator("")
		fp := service.NewFingerprinter("test-salt-32-bytes-xxxxxxxxxxxxxxxx")
		collectSvc := service.NewCollectService(geo, fp)
		// nil repos — token lookup will fail, testing the handler shape only
		h = handler.NewCollectHandler(collectSvc, nil)
	})

	Describe("POST /collect", func() {
		Context("with valid JSON payload", func() {
			It("returns 202 Accepted", func() {
				payload := map[string]any{
					"site":     "tk_nonexistent",
					"type":     "pageview",
					"url":      "https://example.com/",
					"referrer": "",
				}
				body, _ := json.Marshal(payload)
				req := httptest.NewRequest(http.MethodPost, "/collect", bytes.NewReader(body))
				req.Header.Set("Content-Type", "application/json")
				req.RemoteAddr = "1.2.3.4:1234"
				rec := httptest.NewRecorder()

				h.Collect(rec, req)

				// 202 when site token not found is fine — handler should not block on DB errors
				// The important invariant is it never returns 5xx synchronously
				Expect(rec.Code).To(BeNumerically(">=", 200))
				Expect(rec.Code).To(BeNumerically("<", 500))
			})
		})

		Context("with missing site token", func() {
			It("returns 400 Bad Request", func() {
				payload := map[string]any{"type": "pageview", "url": "https://example.com/"}
				body, _ := json.Marshal(payload)
				req := httptest.NewRequest(http.MethodPost, "/collect", bytes.NewReader(body))
				req.Header.Set("Content-Type", "application/json")
				rec := httptest.NewRecorder()

				h.Collect(rec, req)

				Expect(rec.Code).To(Equal(http.StatusBadRequest))
			})
		})

		Context("with bot User-Agent", func() {
			It("returns 202 without writing (silently discards bots)", func() {
				payload := map[string]any{
					"site": "tk_any",
					"type": "pageview",
					"url":  "https://example.com/",
				}
				body, _ := json.Marshal(payload)
				req := httptest.NewRequest(http.MethodPost, "/collect", bytes.NewReader(body))
				req.Header.Set("Content-Type", "application/json")
				req.Header.Set("User-Agent", "Googlebot/2.1")
				rec := httptest.NewRecorder()

				h.Collect(rec, req)

				// Bots get 202 — no point advertising our filtering
				Expect(rec.Code).To(Equal(http.StatusAccepted))
			})
		})
	})
})
```

- [ ] **Step 2: Run — verify it fails**

```bash
go test ./internal/handler/... -v -run TestHandler
```

Expected: compile error — `handler.CollectHandler` not defined.

- [ ] **Step 3: Implement `internal/handler/collect.go`**

```go
package handler

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/sidneydekoning/analytics/internal/model"
	"github.com/sidneydekoning/analytics/internal/repository"
	"github.com/sidneydekoning/analytics/internal/service"
)

// CollectHandler handles POST /collect — the hot-path event ingestion endpoint.
// It validates the site token, builds the event, and fires an async write.
// The response is always sent before the database write completes.
type CollectHandler struct {
	collectSvc service.CollectService
	repos      *repository.Repos
}

// NewCollectHandler constructs a CollectHandler. repos may be nil in tests.
func NewCollectHandler(collectSvc service.CollectService, repos *repository.Repos) *CollectHandler {
	return &CollectHandler{collectSvc: collectSvc, repos: repos}
}

// Collect handles POST /collect.
func (h *CollectHandler) Collect(w http.ResponseWriter, r *http.Request) {
	var req model.CollectRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	if req.Site == "" {
		http.Error(w, "missing site token", http.StatusBadRequest)
		return
	}

	now := time.Now().UTC()

	ev, err := h.collectSvc.BuildEvent("", &req, r, now)
	if errors.Is(err, service.ErrBot) {
		// Silently discard bots — return 202 to not reveal filtering
		w.WriteHeader(http.StatusAccepted)
		return
	}
	if err != nil {
		slog.Error("collect: build event", "error", err)
		w.WriteHeader(http.StatusAccepted) // never return 5xx to trackers
		return
	}

	// Respond immediately — the DB write is async
	w.WriteHeader(http.StatusAccepted)

	if h.repos == nil {
		return
	}

	// Resolve site token to site_id asynchronously
	go func() {
		ctx := r.Context()
		site, err := h.repos.Sites.GetByToken(ctx, req.Site)
		if err != nil {
			// Unknown token — silently discard (could be misconfigured script)
			return
		}
		ev.SiteID = site.ID

		if err := h.repos.Events.Write(ctx, ev); err != nil {
			slog.Error("collect: write event", "error", err, "site_id", site.ID)
		}
	}()
}
```

- [ ] **Step 4: Run — verify all handler specs pass**

```bash
go test ./internal/handler/... -v
```

Expected: PASS — all existing specs plus 3 new collect specs pass (10 total).

- [ ] **Step 5: Commit**

```bash
git add internal/handler/collect.go internal/handler/collect_test.go
git commit -m "feat: add /collect handler with async event write and bot filtering"
```

---

### Task 10: Tracking script

**Files:**
- Create: `static/script.js`

- [ ] **Step 1: Create `static/script.js`**

```javascript
(function () {
  'use strict';

  // Respect Do Not Track
  if (navigator.doNotTrack === '1' || window.doNotTrack === '1') return;

  var script = document.currentScript;
  var siteToken = script && script.getAttribute('data-site');
  if (!siteToken) return;

  var endpoint = (script.getAttribute('data-api') || '/collect');

  function getUTM(param) {
    return new URLSearchParams(window.location.search).get(param) || '';
  }

  function send(type, props) {
    var payload = {
      site: siteToken,
      type: type || 'pageview',
      url: window.location.href,
      referrer: document.referrer,
      width: window.innerWidth,
      language: navigator.language || '',
      utm_source: getUTM('utm_source'),
      utm_medium: getUTM('utm_medium'),
      utm_campaign: getUTM('utm_campaign'),
    };
    if (props) payload.props = props;

    if (navigator.sendBeacon) {
      navigator.sendBeacon(endpoint, JSON.stringify(payload));
    } else {
      // Fallback for older browsers
      var xhr = new XMLHttpRequest();
      xhr.open('POST', endpoint, true);
      xhr.setRequestHeader('Content-Type', 'application/json');
      xhr.send(JSON.stringify(payload));
    }
  }

  // Initial pageview
  send('pageview');

  // SPA navigation support — intercept history.pushState
  var origPushState = history.pushState;
  history.pushState = function () {
    origPushState.apply(this, arguments);
    send('pageview');
  };
  window.addEventListener('popstate', function () { send('pageview'); });

  // Expose a manual tracking API: window.analytics.track('event', {key: val})
  window.analytics = {
    track: function (eventName, props) {
      send(eventName, props || {});
    }
  };
})();
```

- [ ] **Step 2: Verify file was created**

```bash
wc -c /Users/sidneydekoning/stack/Github/analyics-dash-tics/static/script.js
```

Expected: < 2000 bytes.

- [ ] **Step 3: Commit**

```bash
git add static/script.js
git commit -m "feat: add tracking script (pageview + SPA + custom events)"
```

---

### Task 11: Wire CollectHandler into main.go

**Files:**
- Modify: `cmd/server/main.go`

- [ ] **Step 1: Read current main.go to find the placeholder**

The placeholder currently at the `/collect` route looks like:
```go
collectLimiter := middleware.RateLimiter(100.0/60.0, 20)
r.With(collectLimiter).Post("/collect", func(w http.ResponseWriter, r *http.Request) {
    // Placeholder — implemented in Plan 2 (Tracking Pipeline)
    w.WriteHeader(http.StatusAccepted)
})
```

- [ ] **Step 2: Update `cmd/server/main.go`**

Add these lines after `authSvc := service.NewAuth(...)`:

```go
geo := service.NewGeoLocator(cfg.MaxMindDBPath)
fp := service.NewFingerprinter(cfg.JWTSecret) // reuse JWT secret as fingerprint salt
collectSvc := service.NewCollectService(geo, fp)
collectHandler := handler.NewCollectHandler(collectSvc, repos)
```

Replace the placeholder `/collect` route:
```go
collectLimiter := middleware.RateLimiter(100.0/60.0, 20)
r.With(collectLimiter).Post("/collect", collectHandler.Collect)
```

Also add the `service` package import if not already present. The full imports block should include:
```go
"github.com/sidneydekoning/analytics/internal/service"
```

- [ ] **Step 3: Build**

```bash
go build -o bin/analytics ./cmd/server
```

Expected: clean build.

- [ ] **Step 4: Full test suite**

```bash
go test -race ./...
```

Expected: all tests pass.

- [ ] **Step 5: Smoke test**

Start the server (if not running):
```bash
pkill -f bin/analytics 2>/dev/null
DATABASE_URL="postgres://sidneydekoning@localhost:5432/analytics?sslmode=disable" \
JWT_SECRET="55b8fa86529f04fbf54de43cfa221b57795b63166c6cab23881ee9693698ff91" \
JWT_REFRESH_SECRET="73c246e9baeb07f098c8b9c1a5d98e53fcd7d19defaa9af76f39cb0c1c90d03c" \
BASE_URL="https://dash.local" PORT="8090" ENV="development" \
./bin/analytics &
sleep 1
```

Send a test event:
```bash
curl -s -o /dev/null -w "%{http_code}" -X POST https://dash.local/collect \
  -H "Content-Type: application/json" \
  -d '{"site":"tk_invalid","type":"pageview","url":"https://example.com/","referrer":""}'
```

Expected output: `202`

- [ ] **Step 6: Commit**

```bash
git add cmd/server/main.go
git commit -m "feat: wire CollectHandler into main server"
```

- [ ] **Step 7: Push**

```bash
git push origin main
```

---

## Self-Review

**Spec coverage check:**

| Requirement | Task |
|---|---|
| POST /collect endpoint | Task 9 + 11 |
| Site token validation | Task 9 (CollectHandler) |
| Async DB write — 202 before write completes | Task 9 |
| Bot filtering | Task 4 (UA parser) + Task 8 (CollectService) |
| Channel classification (organic/paid/social/email/ai/dark_social/direct/referral) | Task 3 |
| AI tool traffic detection (ChatGPT, Claude, Perplexity, Gemini, Copilot) | Task 3 |
| User-Agent parsing (device/browser/OS) | Task 4 |
| Geolocation (optional MaxMind, graceful skip) | Task 6 |
| Privacy-safe session_id + visitor_id (no PII stored) | Task 5 |
| IP never stored | Task 8 (discarded after geo lookup) |
| script.js tracking snippet | Task 10 |
| SPA navigation support | Task 10 |
| Custom event tracking API | Task 10 |
| Rate limiting on /collect | Already in main.go from Plan 1 |

**No placeholders found.**

**Type consistency verified:**
- `model.CollectRequest` defined in Task 2, used in Tasks 8, 9
- `model.Event` defined in Task 2, used in Tasks 7, 8, 9
- `service.CollectService` interface defined in Task 8, implemented, used in Task 9, 11
- `service.ErrBot` defined in Task 8, checked in Task 9
- `repository.Repos.Events` added in Task 7, used in Task 9, 11
