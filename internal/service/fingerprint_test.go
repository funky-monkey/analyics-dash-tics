package service_test

import (
	"testing"
	"time"

	"github.com/funky-monkey/analyics-dash-tics/internal/service"
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

	assert.NotEqual(t, id1, id2)
}

func TestFingerprint_SessionID(t *testing.T) {
	fp := service.NewFingerprinter("daily-salt-secret")
	now := time.Now()

	sid := fp.SessionID("site-abc", "192.168.1.1", "Mozilla/5.0", now)
	require.NotEmpty(t, sid)
	assert.Len(t, sid, 16)

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

	assert.NotContains(t, id, ip)
	assert.NotContains(t, id, "203")
}
