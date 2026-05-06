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
	default:
		from = now.Add(-30 * 24 * time.Hour)
	}
	return from, to
}
