package service_test

import (
	"testing"

	"github.com/funky-monkey/analyics-dash-tics/internal/service"
	"github.com/stretchr/testify/assert"
)

func TestParseUA(t *testing.T) {
	tests := []struct {
		name        string
		ua          string
		wantDevice  string
		wantBrowser string
		wantOS      string
		wantBot     bool
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
			name:    "googlebot",
			ua:      "Mozilla/5.0 (compatible; Googlebot/2.1; +http://www.google.com/bot.html)",
			wantBot: true,
		},
		{
			name:       "empty ua",
			ua:         "",
			wantDevice: "desktop",
			wantBot:    false,
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
