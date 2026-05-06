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
// Returns ErrBot if the User-Agent is a known bot — caller should discard the event.
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
