package model

import "time"

// Event represents a single analytics event (pageview or custom) stored in the
// events hypertable. IPs are never stored — country/city are resolved at ingestion
// and the IP is discarded.
type Event struct {
	SiteID      string
	Type        string
	URL         string
	Referrer    string
	Channel     string
	UTMSource   string
	UTMMedium   string
	UTMCampaign string
	Country     string
	City        string
	DeviceType  string
	Browser     string
	OS          string
	Language    string
	SessionID   string
	VisitorID   string
	IsBounce    bool
	DurationMS  int
	Props       map[string]string
	Timestamp   time.Time
}

// CollectRequest is the JSON payload sent by the tracking script to POST /collect.
type CollectRequest struct {
	Site        string            `json:"site"`
	Type        string            `json:"type"`
	URL         string            `json:"url"`
	Referrer    string            `json:"referrer"`
	Width       int               `json:"width"`
	Language    string            `json:"language"`
	UTMSource   string            `json:"utm_source"`
	UTMMedium   string            `json:"utm_medium"`
	UTMCampaign string            `json:"utm_campaign"`
	Props       map[string]string `json:"props"`
}

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
	Channel   string
	Referrer  string
	Sessions  int64
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

// CMSPage represents a blog post or generic page created via the admin CMS.
type CMSPage struct {
	ID              string
	LayoutID        string
	AuthorID        string
	Title           string
	Slug            string
	Type            string // "blog" or "page"
	ContentHTML     string // bluemonday-sanitised Trix output
	Excerpt         string
	CoverImageURL   string
	MetaTitle       string
	MetaDescription string
	Status          string // "draft" or "published"
	PublishedAt     *time.Time
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// CMSLayout is a named template file that a CMS page uses.
type CMSLayout struct {
	ID           string
	Name         string
	TemplateFile string
	Description  string
}
