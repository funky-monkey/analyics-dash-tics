package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Config holds all application configuration loaded from environment variables.
// Never call os.Getenv outside this package.
type Config struct {
	DatabaseURL      string
	JWTSecret        []byte
	JWTRefreshSecret []byte
	BaseURL          string
	Port             int
	AllowedOrigins   []string
	SMTPHost         string
	SMTPPort         int
	SMTPUser         string
	SMTPPass         string
	SMTPFrom         string
	MaxMindDBPath    string
	Env              string
}

// Load reads all required env vars and returns a Config.
// Returns an error listing every missing required variable.
func Load() (*Config, error) {
	var missing []string

	require := func(key string) string {
		v := os.Getenv(key)
		if v == "" {
			missing = append(missing, key)
		}
		return v
	}

	optional := func(key, fallback string) string {
		if v := os.Getenv(key); v != "" {
			return v
		}
		return fallback
	}

	dbURL := require("DATABASE_URL")
	jwtSecret := require("JWT_SECRET")
	jwtRefresh := require("JWT_REFRESH_SECRET")
	baseURL := require("BASE_URL")

	portStr := optional("PORT", "8090")
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return nil, fmt.Errorf("config: PORT must be an integer, got %q", portStr)
	}

	smtpPortStr := optional("SMTP_PORT", "587")
	smtpPort, err := strconv.Atoi(smtpPortStr)
	if err != nil {
		return nil, fmt.Errorf("config: SMTP_PORT must be an integer, got %q", smtpPortStr)
	}

	if len(missing) > 0 {
		return nil, fmt.Errorf("config: missing required environment variables: %s", strings.Join(missing, ", "))
	}

	originsRaw := optional("ALLOWED_ORIGINS", "")
	var origins []string
	for _, o := range strings.Split(originsRaw, ",") {
		if trimmed := strings.TrimSpace(o); trimmed != "" {
			origins = append(origins, trimmed)
		}
	}

	return &Config{
		DatabaseURL:      dbURL,
		JWTSecret:        []byte(jwtSecret),
		JWTRefreshSecret: []byte(jwtRefresh),
		BaseURL:          baseURL,
		Port:             port,
		AllowedOrigins:   origins,
		SMTPHost:         optional("SMTP_HOST", ""),
		SMTPPort:         smtpPort,
		SMTPUser:         optional("SMTP_USER", ""),
		SMTPPass:         optional("SMTP_PASS", ""),
		SMTPFrom:         optional("SMTP_FROM", ""),
		MaxMindDBPath:    optional("MAXMIND_DB_PATH", ""),
		Env:              optional("ENV", "production"),
	}, nil
}

// IsDev reports whether the application is running in development mode.
func (c *Config) IsDev() bool {
	return c.Env == "development"
}
