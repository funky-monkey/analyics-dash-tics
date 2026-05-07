package service

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"
)

// Fingerprinter generates privacy-safe, non-reversible visitor and session IDs.
type Fingerprinter interface {
	VisitorID(siteID, ip, userAgent string, t time.Time) string
	SessionID(siteID, ip, userAgent string, t time.Time) string
}

type fingerprinter struct {
	salt string
}

// NewFingerprinter creates a Fingerprinter with the given salt.
func NewFingerprinter(salt string) Fingerprinter {
	return &fingerprinter{salt: salt}
}

// VisitorID returns a 16-char hex ID stable within a calendar day (UTC).
func (f *fingerprinter) VisitorID(siteID, ip, userAgent string, t time.Time) string {
	day := t.UTC().Format("2006-01-02")
	return f.hash(siteID, ip, userAgent, day)
}

// SessionID returns a 16-char hex ID stable within an hour.
func (f *fingerprinter) SessionID(siteID, ip, userAgent string, t time.Time) string {
	hour := t.UTC().Format("2006-01-02-15")
	return f.hash(siteID, ip, userAgent, hour)
}

func (f *fingerprinter) hash(parts ...string) string {
	h := sha256.New()
	for _, p := range parts {
		_, _ = fmt.Fprintf(h, "%s|", p)
	}
	_, _ = fmt.Fprintf(h, "%s", f.salt)
	return hex.EncodeToString(h.Sum(nil))[:16]
}
