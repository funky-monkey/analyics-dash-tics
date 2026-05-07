package repository_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/funky-monkey/analyics-dash-tics/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStatsRepository_GetSummary_Empty(t *testing.T) {
	repos := setupTestRepos(t)
	ctx := context.Background()

	owner := &model.User{Email: uniqueEmail(), PasswordHash: "x", Role: model.RoleUser, Name: "O", IsActive: true}
	require.NoError(t, repos.Users.Create(ctx, owner))
	site := &model.Site{
		OwnerID: owner.ID, Name: "S", Domain: "s.com",
		Token: fmt.Sprintf("tk_s%d", time.Now().UnixNano()), Timezone: "UTC",
	}
	require.NoError(t, repos.Sites.Create(ctx, site))

	summary, err := repos.Stats.GetSummary(ctx, site.ID, time.Now().Add(-24*time.Hour), time.Now())
	require.NoError(t, err)
	assert.Equal(t, int64(0), summary.Pageviews)
	assert.Equal(t, int64(0), summary.Visitors)
}

func TestStatsRepository_GetTopPages_Empty(t *testing.T) {
	repos := setupTestRepos(t)
	ctx := context.Background()

	owner := &model.User{Email: uniqueEmail(), PasswordHash: "x", Role: model.RoleUser, Name: "O", IsActive: true}
	require.NoError(t, repos.Users.Create(ctx, owner))
	site := &model.Site{
		OwnerID: owner.ID, Name: "S", Domain: "s2.com",
		Token: fmt.Sprintf("tk_tp%d", time.Now().UnixNano()), Timezone: "UTC",
	}
	require.NoError(t, repos.Sites.Create(ctx, site))

	pages, err := repos.Stats.GetTopPages(ctx, site.ID, time.Now().Add(-time.Hour), time.Now(), 10)
	require.NoError(t, err)
	assert.Empty(t, pages)
}
