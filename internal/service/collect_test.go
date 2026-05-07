package service_test

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/funky-monkey/analyics-dash-tics/internal/model"
	"github.com/funky-monkey/analyics-dash-tics/internal/service"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestCollectService() service.CollectService {
	geo := service.NewGeoLocator("")
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
	assert.Equal(t, "organic", ev.Channel)
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
