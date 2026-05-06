package service

import "github.com/mileusna/useragent"

// UAResult holds parsed User-Agent information.
type UAResult struct {
	DeviceType string // "desktop", "mobile", "tablet"
	Browser    string
	OS         string
	IsBot      bool
}

// ParseUA parses a User-Agent string into device type, browser, OS, and bot flag.
// Returns IsBot=true for known bots — the caller should discard bot events.
func ParseUA(uaString string) UAResult {
	ua := useragent.Parse(uaString)

	if ua.Bot {
		return UAResult{IsBot: true}
	}

	device := "desktop"
	if ua.Mobile {
		device = "mobile"
	} else if ua.Tablet {
		device = "tablet"
	}

	return UAResult{
		DeviceType: device,
		Browser:    ua.Name,
		OS:         normaliseOS(ua.OS),
		IsBot:      false,
	}
}

func normaliseOS(raw string) string {
	switch {
	case raw == "Mac OS X" || raw == "macOS":
		return "macOS"
	case raw == "Windows" || raw == "Windows 10" || raw == "Windows 11":
		return "Windows"
	case raw == "Linux":
		return "Linux"
	case raw == "iPhone OS" || raw == "iOS":
		return "iOS"
	case raw == "Android":
		return "Android"
	case raw == "Chrome OS" || raw == "ChromeOS":
		return "ChromeOS"
	default:
		return raw
	}
}
