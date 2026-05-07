package model

// Goal is a named conversion target attached to a site.
type Goal struct {
	ID     string
	SiteID string
	Name   string
	Type   string // "pageview" | "event" | "outbound"
	Value  string
}

// Funnel is an ordered sequence of steps used for drop-off analysis.
type Funnel struct {
	ID        string
	SiteID    string
	Name      string
	CreatedAt string // formatted date string for templates, e.g. "2026-05-07"
}

// FunnelStep is one step in a funnel.
type FunnelStep struct {
	ID        string
	FunnelID  string
	Position  int
	Name      string
	MatchType string // "url" | "event"
	Value     string
}

// FunnelResult holds the drop-off numbers for one funnel over a time range.
type FunnelResult struct {
	FunnelID   string
	FunnelName string
	Steps      []FunnelStepResult
}

// FunnelStepResult is the per-step visitor count.
type FunnelStepResult struct {
	Position  int
	Name      string
	Visitors  int64
	DropOff   float64 // % lost vs previous step (0 for step 0)
	Converted float64 // % of step-0 visitors who reached this step
}

// CustomEventStat is one row of the custom events dashboard.
type CustomEventStat struct {
	EventType string
	URL       string
	Count     int64
}
