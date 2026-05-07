package repository_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/sidneydekoning/analytics/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFunnelRepository_CRUD(t *testing.T) {
	repos := setupTestRepos(t)
	ctx := context.Background()

	owner := &model.User{
		Email: uniqueEmail(), PasswordHash: "$2a$12$x",
		Role: model.RoleUser, Name: "FunnelOwner", IsActive: true,
	}
	require.NoError(t, repos.Users.Create(ctx, owner))
	site := &model.Site{
		OwnerID: owner.ID, Name: "FunnelSite",
		Domain:   fmt.Sprintf("funneltest%d.com", time.Now().UnixNano()),
		Token:    fmt.Sprintf("tk_fn%d", time.Now().UnixNano()),
		Timezone: "UTC",
	}
	require.NoError(t, repos.Sites.Create(ctx, site))

	steps := []*model.FunnelStep{
		{Position: 0, Name: "Homepage", MatchType: "url", Value: "https://funneltest.com/"},
		{Position: 1, Name: "Pricing", MatchType: "url", Value: "https://funneltest.com/pricing"},
		{Position: 2, Name: "Signup", MatchType: "url", Value: "https://funneltest.com/signup"},
	}
	f := &model.Funnel{SiteID: site.ID, Name: "Conversion"}
	require.NoError(t, repos.Funnels.Create(ctx, f, steps))
	assert.NotEmpty(t, f.ID)
	for _, s := range steps {
		assert.NotEmpty(t, s.ID)
	}

	// List
	funnels, err := repos.Funnels.ListBySite(ctx, site.ID)
	require.NoError(t, err)
	require.Len(t, funnels, 1)
	assert.Equal(t, "Conversion", funnels[0].Name)

	// GetWithSteps
	gotF, gotSteps, err := repos.Funnels.GetWithSteps(ctx, f.ID, site.ID)
	require.NoError(t, err)
	assert.Equal(t, f.Name, gotF.Name)
	assert.Len(t, gotSteps, 3)
	assert.Equal(t, 0, gotSteps[0].Position)

	// Delete
	require.NoError(t, repos.Funnels.Delete(ctx, f.ID, site.ID))
	funnels, err = repos.Funnels.ListBySite(ctx, site.ID)
	require.NoError(t, err)
	assert.Empty(t, funnels)
}

func TestFunnelRepository_GetDropOff(t *testing.T) {
	repos := setupTestRepos(t)
	ctx := context.Background()

	owner := &model.User{
		Email: uniqueEmail(), PasswordHash: "$2a$12$x",
		Role: model.RoleUser, Name: "DropOwner", IsActive: true,
	}
	require.NoError(t, repos.Users.Create(ctx, owner))
	site := &model.Site{
		OwnerID: owner.ID, Name: "DropSite",
		Domain:   fmt.Sprintf("droptest%d.com", time.Now().UnixNano()),
		Token:    fmt.Sprintf("tk_drop%d", time.Now().UnixNano()),
		Timezone: "UTC",
	}
	require.NoError(t, repos.Sites.Create(ctx, site))

	now := time.Now()

	// Visitor A hits all 3 steps in order
	for _, ev := range []struct {
		url    string
		offset time.Duration
	}{
		{"https://droptest.com/", 0},
		{"https://droptest.com/pricing", 1 * time.Second},
		{"https://droptest.com/signup", 2 * time.Second},
	} {
		require.NoError(t, repos.Events.Write(ctx, &model.Event{
			SiteID: site.ID, Type: "pageview", URL: ev.url,
			VisitorID: "visitor-a", SessionID: "sess-a",
			Channel: "direct", Timestamp: now.Add(-5*time.Minute + ev.offset),
		}))
	}

	// Visitor B hits only steps 0 and 1
	for _, ev := range []struct {
		url    string
		offset time.Duration
	}{
		{"https://droptest.com/", 0},
		{"https://droptest.com/pricing", 1 * time.Second},
	} {
		require.NoError(t, repos.Events.Write(ctx, &model.Event{
			SiteID: site.ID, Type: "pageview", URL: ev.url,
			VisitorID: "visitor-b", SessionID: "sess-b",
			Channel: "direct", Timestamp: now.Add(-3*time.Minute + ev.offset),
		}))
	}

	steps := []*model.FunnelStep{
		{Position: 0, MatchType: "url", Value: "https://droptest.com/"},
		{Position: 1, MatchType: "url", Value: "https://droptest.com/pricing"},
		{Position: 2, MatchType: "url", Value: "https://droptest.com/signup"},
	}

	counts, err := repos.Funnels.GetDropOff(ctx, site.ID, steps, now.Add(-1*time.Hour), now.Add(time.Hour))
	require.NoError(t, err)
	require.Len(t, counts, 3)
	assert.Equal(t, int64(2), counts[0])
	assert.Equal(t, int64(2), counts[1])
	assert.Equal(t, int64(1), counts[2])
}
