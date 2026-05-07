package repository_test

import (
	"context"
	"os"
	"testing"

	"github.com/funky-monkey/analyics-dash-tics/internal/repository"
	"github.com/stretchr/testify/require"
)

func TestNewPool_ConnectsSuccessfully(t *testing.T) {
	dbURL := os.Getenv("TEST_DATABASE_URL")
	if dbURL == "" {
		t.Skip("TEST_DATABASE_URL not set — skipping integration test")
	}

	pool, err := repository.NewPool(context.Background(), dbURL)
	require.NoError(t, err)
	defer pool.Close()

	err = pool.Ping(context.Background())
	require.NoError(t, err)
}
