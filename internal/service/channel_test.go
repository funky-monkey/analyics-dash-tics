package service_test

import (
	"testing"

	"github.com/funky-monkey/analyics-dash-tics/internal/service"
	"github.com/stretchr/testify/assert"
)

func TestClassifyChannel(t *testing.T) {
	tests := []struct {
		name      string
		referrer  string
		utmMedium string
		utmSource string
		url       string
		want      string
	}{
		{"chatgpt", "https://chatgpt.com/c/abc", "", "", "https://example.com/", "ai"},
		{"claude", "https://claude.ai/chat/123", "", "", "https://example.com/", "ai"},
		{"perplexity", "https://www.perplexity.ai/search", "", "", "https://example.com/", "ai"},
		{"gemini", "https://gemini.google.com/app", "", "", "https://example.com/", "ai"},
		{"copilot", "https://copilot.microsoft.com/", "", "", "https://example.com/", "ai"},
		{"google organic", "https://www.google.com/search?q=analytics", "", "", "https://example.com/", "organic"},
		{"bing organic", "https://www.bing.com/search?q=foo", "", "", "https://example.com/", "organic"},
		{"duckduckgo", "https://duckduckgo.com/?q=test", "", "", "https://example.com/", "organic"},
		{"google cpc", "https://www.google.com/", "cpc", "google", "https://example.com/", "paid"},
		{"paid medium", "", "ppc", "facebook", "https://example.com/", "paid"},
		{"paid social", "", "paidsocial", "instagram", "https://example.com/", "paid"},
		{"email medium", "", "email", "newsletter", "https://example.com/", "email"},
		{"email referrer", "https://mail.google.com/", "", "", "https://example.com/", "email"},
		{"facebook", "https://www.facebook.com/", "", "", "https://example.com/", "social"},
		{"twitter/x", "https://t.co/abc", "", "", "https://example.com/", "social"},
		{"linkedin", "https://www.linkedin.com/feed/", "", "", "https://example.com/", "social"},
		{"instagram", "https://l.instagram.com/", "", "", "https://example.com/", "social"},
		{"dark social blog", "", "", "", "https://example.com/blog/article", "dark_social"},
		{"direct homepage", "", "", "", "https://example.com/", "direct"},
		{"direct no trailing slash", "", "", "", "https://example.com", "direct"},
		{"referral", "https://someotherdomain.com/page", "", "", "https://example.com/", "referral"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := service.ClassifyChannel(tt.referrer, tt.utmMedium, tt.utmSource, tt.url)
			assert.Equal(t, tt.want, got, "referrer=%q utm_medium=%q url=%q", tt.referrer, tt.utmMedium, tt.url)
		})
	}
}
