package config_test

import (
	"testing"

	"github.com/funky-monkey/analyics-dash-tics/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoad_AllRequired(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://localhost/test")
	t.Setenv("JWT_SECRET", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	t.Setenv("JWT_REFRESH_SECRET", "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")
	t.Setenv("BASE_URL", "http://localhost:8090")
	t.Setenv("PORT", "8090")

	cfg, err := config.Load()
	require.NoError(t, err)
	assert.Equal(t, "postgres://localhost/test", cfg.DatabaseURL)
	assert.Equal(t, 8090, cfg.Port)
	assert.Equal(t, "http://localhost:8090", cfg.BaseURL)
}

func TestLoad_MissingRequired(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	_, err := config.Load()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "DATABASE_URL")
}

func TestLoad_IsDev(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://localhost/test")
	t.Setenv("JWT_SECRET", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	t.Setenv("JWT_REFRESH_SECRET", "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")
	t.Setenv("BASE_URL", "http://localhost:8090")
	t.Setenv("ENV", "development")

	cfg, err := config.Load()
	require.NoError(t, err)
	assert.True(t, cfg.IsDev())
}
