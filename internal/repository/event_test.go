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

func TestEventRepository_Write(t *testing.T) {
	repos := setupTestRepos(t)
	ctx := context.Background()

	owner := &model.User{
		Email: uniqueEmail(), PasswordHash: "$2a$12$x",
		Role: model.RoleUser, Name: "Owner", IsActive: true,
	}
	require.NoError(t, repos.Users.Create(ctx, owner))

	site := &model.Site{
		OwnerID:  owner.ID,
		Name:     "Test",
		Domain:   "test.com",
		Token:    fmt.Sprintf("tk_evtest%d", time.Now().UnixNano()),
		Timezone: "UTC",
	}
	require.NoError(t, repos.Sites.Create(ctx, site))

	ev := &model.Event{
		SiteID:     site.ID,
		Type:       "pageview",
		URL:        "https://test.com/",
		Channel:    "direct",
		Country:    "NL",
		City:       "Amsterdam",
		DeviceType: "desktop",
		Browser:    "Chrome",
		OS:         "macOS",
		SessionID:  "abc123def456abcd",
		VisitorID:  "def456abc123defa",
		Timestamp:  time.Now(),
	}

	err := repos.Events.Write(ctx, ev)
	require.NoError(t, err)
}

func TestEventRepository_CountBySite(t *testing.T) {
	repos := setupTestRepos(t)
	ctx := context.Background()

	owner := &model.User{
		Email: uniqueEmail(), PasswordHash: "$2a$12$x",
		Role: model.RoleUser, Name: "Owner", IsActive: true,
	}
	require.NoError(t, repos.Users.Create(ctx, owner))

	site := &model.Site{
		OwnerID:  owner.ID,
		Name:     "Count Test",
		Domain:   "count.com",
		Token:    fmt.Sprintf("tk_count%d", time.Now().UnixNano()),
		Timezone: "UTC",
	}
	require.NoError(t, repos.Sites.Create(ctx, site))

	for i := 0; i < 3; i++ {
		ev := &model.Event{
			SiteID:    site.ID,
			Type:      "pageview",
			URL:       "https://count.com/",
			Channel:   "direct",
			SessionID: fmt.Sprintf("sess%d", i),
			VisitorID: fmt.Sprintf("vis%d", i),
			Timestamp: time.Now(),
		}
		require.NoError(t, repos.Events.Write(ctx, ev))
	}

	count, err := repos.Events.CountBySite(ctx, site.ID,
		time.Now().Add(-time.Minute), time.Now().Add(time.Minute))
	require.NoError(t, err)
	assert.Equal(t, int64(3), count)
}
