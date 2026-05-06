package service

import (
	"log/slog"
	"net"

	"github.com/oschwald/geoip2-golang"
)

// GeoLocator resolves IP addresses to country and city using a local MaxMind database.
// The IP is never sent to any third-party service. Returns ("", "") when no DB is configured.
type GeoLocator interface {
	Lookup(ip string) (country, city string)
}

type geoLocator struct {
	db *geoip2.Reader
}

// NewGeoLocator creates a GeoLocator. If dbPath is empty or the file cannot be opened,
// returns a no-op locator that always returns ("", "").
func NewGeoLocator(dbPath string) GeoLocator {
	if dbPath == "" {
		return &geoLocator{}
	}
	db, err := geoip2.Open(dbPath)
	if err != nil {
		slog.Warn("geolocation unavailable", "path", dbPath, "error", err)
		return &geoLocator{}
	}
	return &geoLocator{db: db}
}

// Lookup returns the ISO 3166-1 alpha-2 country code and city name for ip.
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
