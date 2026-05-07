package repository_test

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/funky-monkey/analyics-dash-tics/internal/model"
	"github.com/funky-monkey/analyics-dash-tics/internal/repository"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTestRepos(t *testing.T) *repository.Repos {
	t.Helper()
	dbURL := os.Getenv("TEST_DATABASE_URL")
	if dbURL == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	pool, err := repository.NewPool(context.Background(), dbURL)
	require.NoError(t, err)
	t.Cleanup(func() { pool.Close() })
	return repository.New(pool)
}

func uniqueEmail() string {
	return fmt.Sprintf("test+%d@example.com", time.Now().UnixNano())
}

func TestUserRepository_CreateAndGetByEmail(t *testing.T) {
	repos := setupTestRepos(t)
	ctx := context.Background()

	const testHash = "$2a$12$somehash" //nolint:gosec // G101: test fixture, not a real credential
	u := &model.User{
		Email:        uniqueEmail(),
		PasswordHash: testHash,
		Role:         model.RoleUser,
		Name:         "Test User",
		IsActive:     true,
	}

	require.NoError(t, repos.Users.Create(ctx, u))
	assert.NotEmpty(t, u.ID)
	assert.False(t, u.CreatedAt.IsZero())

	found, err := repos.Users.GetByEmail(ctx, u.Email)
	require.NoError(t, err)
	assert.Equal(t, u.ID, found.ID)
	assert.Equal(t, u.Email, found.Email)
}

func TestUserRepository_GetByEmail_NotFound(t *testing.T) {
	repos := setupTestRepos(t)
	_, err := repos.Users.GetByEmail(context.Background(), "nonexistent@example.com")
	assert.ErrorIs(t, err, repository.ErrNotFound)
}

func TestUserRepository_SetActive(t *testing.T) {
	repos := setupTestRepos(t)
	ctx := context.Background()

	u := &model.User{Email: uniqueEmail(), PasswordHash: "x", Role: model.RoleUser, Name: "A", IsActive: true}
	require.NoError(t, repos.Users.Create(ctx, u))

	require.NoError(t, repos.Users.SetActive(ctx, u.ID, false))
	found, err := repos.Users.GetByID(ctx, u.ID)
	require.NoError(t, err)
	assert.False(t, found.IsActive)
}
