package service

import (
	"net/url"
	"strings"
)

var aiHostnames = []string{
	"chatgpt.com", "chat.openai.com", "claude.ai", "perplexity.ai",
	"gemini.google.com", "copilot.microsoft.com", "you.com", "phind.com",
	"poe.com", "character.ai",
}

var searchEngineHostnames = []string{
	"google.", "bing.com", "yahoo.com", "duckduckgo.com",
	"baidu.com", "yandex.", "ecosia.org", "brave.com",
}

var socialHostnames = []string{
	"facebook.com", "fb.com", "t.co", "twitter.com", "x.com",
	"linkedin.com", "instagram.com", "l.instagram.com",
	"tiktok.com", "pinterest.com", "reddit.com", "snapchat.com",
	"youtube.com", "whatsapp.com",
}

var emailHostnames = []string{
	"mail.google.com", "outlook.live.com", "outlook.office.com",
	"mail.yahoo.com", "webmail.",
}

var paidMediums = []string{"cpc", "ppc", "paid", "paidsearch", "paidsocial", "cpv", "cpm"}

// ClassifyChannel returns the traffic channel for an event based on referrer and UTM params.
// The result is pre-computed at ingestion and stored directly in the events table.
func ClassifyChannel(referrer, utmMedium, utmSource, pageURL string) string {
	medium := strings.ToLower(strings.TrimSpace(utmMedium))
	referrerHost := extractHost(referrer)

	for _, paid := range paidMediums {
		if medium == paid {
			return "paid"
		}
	}
	if medium == "email" {
		return "email"
	}
	if referrerHost != "" && containsHost(referrerHost, emailHostnames) {
		return "email"
	}
	if referrerHost != "" && containsHost(referrerHost, aiHostnames) {
		return "ai"
	}
	if referrerHost != "" && containsHost(referrerHost, searchEngineHostnames) {
		return "organic"
	}
	if referrerHost != "" && containsHost(referrerHost, socialHostnames) {
		return "social"
	}
	if referrerHost != "" {
		return "referral"
	}
	if isDarkSocial(pageURL) {
		return "dark_social"
	}
	return "direct"
}

func extractHost(rawURL string) string {
	if rawURL == "" {
		return ""
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	return strings.ToLower(u.Hostname())
}

func containsHost(host string, patterns []string) bool {
	for _, p := range patterns {
		if strings.Contains(host, p) {
			return true
		}
	}
	return false
}

func isDarkSocial(pageURL string) bool {
	if pageURL == "" {
		return false
	}
	u, err := url.Parse(pageURL)
	if err != nil {
		return false
	}
	path := strings.TrimSuffix(u.Path, "/")
	return path != "" && path != "/"
}
