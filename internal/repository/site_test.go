package repository_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/sidneydekoning/analytics/internal/model"
	"github.com/sidneydekoning/analytics/internal/repository"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSiteRepository_CreateAndGetByToken(t *testing.T) {
	repos := setupTestRepos(t)
	ctx := context.Background()

	owner := &model.User{
		Email: uniqueEmail(), PasswordHash: "$2a$12$x", Role: model.RoleUser, Name: "Owner", IsActive: true,
	}
	require.NoError(t, repos.Users.Create(ctx, owner))

	token := fmt.Sprintf("tk_test%d", time.Now().UnixNano())
	site := &model.Site{
		OwnerID:  owner.ID,
		Name:     "Test Site",
		Domain:   "example.com",
		Token:    token,
		Timezone: "UTC",
	}
	require.NoError(t, repos.Sites.Create(ctx, site))
	assert.NotEmpty(t, site.ID)

	found, err := repos.Sites.GetByToken(ctx, token)
	require.NoError(t, err)
	assert.Equal(t, site.ID, found.ID)
	assert.Equal(t, "example.com", found.Domain)
}

func TestSiteRepository_GetByToken_NotFound(t *testing.T) {
	repos := setupTestRepos(t)
	_, err := repos.Sites.GetByToken(context.Background(), "tk_nonexistent")
	assert.ErrorIs(t, err, repository.ErrNotFound)
}
