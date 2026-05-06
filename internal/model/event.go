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
