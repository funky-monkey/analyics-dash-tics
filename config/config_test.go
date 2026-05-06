package config_test

import (
	"os"
	"testing"

	"github.com/sidneydekoning/analytics/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoad_AllRequired(t *testing.T) {
	os.Setenv("DATABASE_URL", "postgres://localhost/test")
	os.Setenv("JWT_SECRET", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	os.Setenv("JWT_REFRESH_SECRET", "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")
	os.Setenv("BASE_URL", "http://localhost:8090")
	os.Setenv("PORT", "8090")
	defer func() {
		os.Unsetenv("DATABASE_URL")
		os.Unsetenv("JWT_SECRET")
		os.Unsetenv("JWT_REFRESH_SECRET")
		os.Unsetenv("BASE_URL")
		os.Unsetenv("PORT")
	}()

	cfg, err := config.Load()
	require.NoError(t, err)
	assert.Equal(t, "postgres://localhost/test", cfg.DatabaseURL)
	assert.Equal(t, 8090, cfg.Port)
	assert.Equal(t, "http://localhost:8090", cfg.BaseURL)
}

func TestLoad_MissingRequired(t *testing.T) {
	os.Unsetenv("DATABASE_URL")
	_, err := config.Load()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "DATABASE_URL")
}

func TestLoad_IsDev(t *testing.T) {
	os.Setenv("DATABASE_URL", "postgres://localhost/test")
	os.Setenv("JWT_SECRET", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	os.Setenv("JWT_REFRESH_SECRET", "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")
	os.Setenv("BASE_URL", "http://localhost:8090")
	os.Setenv("ENV", "development")
	defer func() {
		os.Unsetenv("DATABASE_URL")
		os.Unsetenv("JWT_SECRET")
		os.Unsetenv("JWT_REFRESH_SECRET")
		os.Unsetenv("BASE_URL")
		os.Unsetenv("ENV")
	}()

	cfg, err := config.Load()
	require.NoError(t, err)
	assert.True(t, cfg.IsDev())
}
