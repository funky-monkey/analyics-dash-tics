package repository_test

import (
	"context"
	"testing"

	"github.com/funky-monkey/analyics-dash-tics/internal/model"
	"github.com/funky-monkey/analyics-dash-tics/internal/repository"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGoalRepository_CRUD(t *testing.T) {
	repos := setupTestRepos(t)
	ctx := context.Background()

	owner := &model.User{
		Email: uniqueEmail(), PasswordHash: "$2a$12$x",
		Role: model.RoleUser, Name: "GoalOwner", IsActive: true,
	}
	require.NoError(t, repos.Users.Create(ctx, owner))

	site := &model.Site{
		OwnerID: owner.ID, Name: "GoalSite", Domain: "goaltest.com",
		Token: "tk_goaltest01", Timezone: "UTC",
	}
	require.NoError(t, repos.Sites.Create(ctx, site))

	// Create
	g := &model.Goal{SiteID: site.ID, Name: "Signup", Type: "pageview", Value: "/signup"}
	require.NoError(t, repos.Goals.Create(ctx, g))
	assert.NotEmpty(t, g.ID)

	// List
	goals, err := repos.Goals.ListBySite(ctx, site.ID)
	require.NoError(t, err)
	require.Len(t, goals, 1)
	assert.Equal(t, g.SiteID, goals[0].SiteID)
	assert.Equal(t, "pageview", goals[0].Type)
	assert.Equal(t, "/signup", goals[0].Value)

	// Delete
	require.NoError(t, repos.Goals.Delete(ctx, g.ID, site.ID))
	goals, err = repos.Goals.ListBySite(ctx, site.ID)
	require.NoError(t, err)
	assert.Empty(t, goals)

	// Delete wrong site → ErrNotFound
	g2 := &model.Goal{SiteID: site.ID, Name: "Download", Type: "event", Value: "file_download"}
	require.NoError(t, repos.Goals.Create(ctx, g2))
	err = repos.Goals.Delete(ctx, g2.ID, "00000000-0000-0000-0000-000000000000")
	assert.ErrorIs(t, err, repository.ErrNotFound)
}
