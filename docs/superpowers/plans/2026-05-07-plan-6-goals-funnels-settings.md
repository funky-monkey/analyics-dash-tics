# Goals, Funnels & Site Settings Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:subagent-driven-development` (recommended) or `superpowers:executing-plans` to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build goals CRUD, ordered funnel analysis with drop-off charts, site settings (rename/timezone/delete), and a live custom events dashboard so the product can track conversions end-to-end.

**Architecture:** New `internal/repository/goals.go` and `internal/repository/funnels.go` files add repository interfaces following the exact same pattern as `stats.go`. `internal/handler/sites.go` gains settings + goals handlers. A new `FunnelService` in `internal/service/funnel.go` builds dynamic ordered-funnel CTEs. Templates follow the established `{{template "dashboard.html" .}}` + `{{define "content"}}` pattern. No new dependencies.

**Tech Stack:** Go `pgx/v5`, `html/template`, Tailwind CSS, Chart.js (already in vendor), `testify` for repo tests, Ginkgo/Gomega for handler tests.

> **Working directory:** `/Users/sidneydekoning/stack/Github/analyics-dash-tics`
> **Module:** `github.com/sidneydekoning/analytics`
> **No Co-Authored-By in commit messages.**

---

## File Map

```
internal/model/goals.go                   — Goal, Funnel, FunnelStep, FunnelResult, FunnelStepResult types
internal/repository/goals.go             — GoalRepository interface + pgGoalRepository
internal/repository/goals_test.go        — repository-layer tests for goals CRUD
internal/repository/funnels.go           — FunnelRepository interface + pgFunnelRepository + drop-off query builder
internal/repository/funnels_test.go      — repository-layer tests for funnel CRUD + drop-off
internal/repository/repos.go             — add Goals, Funnels fields + wire in New()
internal/repository/migrations/009_update_sites.sql  — add UPDATE capability (no schema change needed, but add index on goals.site_id if missing)
internal/service/funnel.go               — FunnelService: builds dynamic ordered CTE, returns FunnelResult
internal/handler/sites.go                — add Settings, UpdateSite, Goals, CreateGoal, DeleteGoal handlers
internal/repository/event.go             — add ListCustomEvents to EventRepository
templates/pages/dashboard/settings.html  — site settings: rename, timezone, token display, delete
templates/pages/dashboard/goals.html     — goals list + inline create form
templates/pages/dashboard/events.html    — replace stub with real custom event table
templates/pages/dashboard/funnels.html   — funnel list + create funnel form (multi-step)
templates/pages/dashboard/funnel-detail.html — funnel drop-off: step bars + conversion %
cmd/server/main.go                        — wire new routes
```

---

### Task 1: Goals model + repository

The database already has a `goals` table from migration `004_funnels.sql`:
```sql
CREATE TYPE goal_type AS ENUM ('pageview', 'event', 'outbound');
CREATE TABLE goals (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    site_id UUID NOT NULL REFERENCES sites(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    type goal_type NOT NULL,
    value TEXT NOT NULL
);
```

**Files:**
- Create: `internal/model/goals.go`
- Create: `internal/repository/goals.go`
- Create: `internal/repository/goals_test.go`

- [ ] **Step 1: Create `internal/model/goals.go`**

```go
package model

// Goal is a named conversion target attached to a site.
// Type is one of "pageview" (URL match), "event" (custom event name), "outbound" (external link click).
// Value is the URL or event name to match.
type Goal struct {
	ID     string
	SiteID string
	Name   string
	Type   string // "pageview" | "event" | "outbound"
	Value  string
}

// Funnel is an ordered sequence of steps used for drop-off analysis.
type Funnel struct {
	ID        string
	SiteID    string
	Name      string
	CreatedAt string // formatted for templates, e.g. "2026-05-07"
}

// FunnelStep is one step in a funnel.
// MatchType is "url" (exact URL match) or "event" (custom event type match).
type FunnelStep struct {
	ID       string
	FunnelID string
	Position int
	Name     string
	MatchType string // "url" | "event"
	Value     string
}

// FunnelResult holds the drop-off numbers for one funnel over a time range.
type FunnelResult struct {
	FunnelID   string
	FunnelName string
	Steps      []FunnelStepResult
}

// FunnelStepResult is the per-step visitor count returned by the funnel query.
type FunnelStepResult struct {
	Position  int
	Name      string
	Visitors  int64
	DropOff   float64 // percentage lost vs previous step (0 for step 0)
	Converted float64 // percentage of step-0 visitors who reached this step
}
```

- [ ] **Step 2: Create `internal/repository/goals.go`**

```go
package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/sidneydekoning/analytics/internal/model"
)

// GoalRepository handles CRUD for conversion goals.
type GoalRepository interface {
	ListBySite(ctx context.Context, siteID string) ([]*model.Goal, error)
	Create(ctx context.Context, g *model.Goal) error
	Delete(ctx context.Context, id, siteID string) error
}

type pgGoalRepository struct {
	pool *pgxpool.Pool
}

func (r *pgGoalRepository) ListBySite(ctx context.Context, siteID string) ([]*model.Goal, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, site_id, name, type, value FROM goals WHERE site_id=$1 ORDER BY name`, siteID)
	if err != nil {
		return nil, fmt.Errorf("goalRepository.ListBySite: %w", err)
	}
	defer rows.Close()
	var goals []*model.Goal
	for rows.Next() {
		g := &model.Goal{}
		if err := rows.Scan(&g.ID, &g.SiteID, &g.Name, &g.Type, &g.Value); err != nil {
			return nil, fmt.Errorf("goalRepository.ListBySite: scan: %w", err)
		}
		goals = append(goals, g)
	}
	return goals, rows.Err()
}

func (r *pgGoalRepository) Create(ctx context.Context, g *model.Goal) error {
	return r.pool.QueryRow(ctx,
		`INSERT INTO goals (site_id, name, type, value) VALUES ($1,$2,$3,$4) RETURNING id`,
		g.SiteID, g.Name, g.Type, g.Value).Scan(&g.ID)
}

func (r *pgGoalRepository) Delete(ctx context.Context, id, siteID string) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM goals WHERE id=$1 AND site_id=$2`, id, siteID)
	if err != nil {
		return fmt.Errorf("goalRepository.Delete: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// compile-time check
var _ GoalRepository = (*pgGoalRepository)(nil)
```

- [ ] **Step 3: Write failing test in `internal/repository/goals_test.go`**

```go
package repository_test

import (
	"context"
	"testing"

	"github.com/sidneydekoning/analytics/internal/model"
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
	assert.Equal(t, "Signup", goals[0].Name)

	// Delete
	require.NoError(t, repos.Goals.Delete(ctx, g.ID, site.ID))
	goals, err = repos.Goals.ListBySite(ctx, site.ID)
	require.NoError(t, err)
	assert.Empty(t, goals)

	// Delete wrong site → ErrNotFound
	g2 := &model.Goal{SiteID: site.ID, Name: "Download", Type: "event", Value: "file_download"}
	require.NoError(t, repos.Goals.Create(ctx, g2))
	err = repos.Goals.Delete(ctx, g2.ID, "00000000-0000-0000-0000-000000000000")
	assert.ErrorIs(t, err, ErrNotFound)
}
```

- [ ] **Step 4: Add `Goals` to `repos.go`**

In `internal/repository/repos.go`, add the `Goals` field and wire it in `New()`:

```go
// Repos aggregates all repository interfaces for dependency injection.
type Repos struct {
	Users  UserRepository
	Sites  SiteRepository
	Events EventRepository
	Stats  StatsRepository
	Admin  AdminRepository
	CMS    CMSRepository
	Goals  GoalRepository
	Funnels FunnelRepository  // added in Task 2
}

func New(pool *pgxpool.Pool) *Repos {
	return &Repos{
		Users:   &pgUserRepository{pool: pool},
		Sites:   &pgSiteRepository{pool: pool},
		Events:  &pgEventRepository{pool: pool},
		Stats:   &pgStatsRepository{pool: pool},
		Admin:   &pgAdminRepository{pool: pool},
		CMS:     &pgCMSRepository{pool: pool},
		Goals:   &pgGoalRepository{pool: pool},
		Funnels: &pgFunnelRepository{pool: pool}, // added in Task 2
	}
}
```

Note: `FunnelRepository` doesn't exist yet — add the `Goals` field now and leave a compile error for `Funnels`; it gets fixed in Task 2. Or add both fields in one edit (simpler).

- [ ] **Step 5: Run tests**

```bash
go test ./internal/repository/... -run TestGoalRepository -v
```

Expected: `PASS`

- [ ] **Step 6: Commit**

```bash
git add internal/model/goals.go internal/repository/goals.go internal/repository/goals_test.go internal/repository/repos.go
git commit -m "feat: add Goal model and GoalRepository"
```

---

### Task 2: Funnels repository + drop-off service

The database already has `funnels` and `funnel_steps` from migration `004_funnels.sql`:
```sql
CREATE TABLE funnels (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    site_id UUID NOT NULL REFERENCES sites(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE TABLE funnel_steps (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    funnel_id UUID NOT NULL REFERENCES funnels(id) ON DELETE CASCADE,
    position INTEGER NOT NULL,
    match_type funnel_match NOT NULL,  -- 'url' | 'event' | 'goal'
    value TEXT NOT NULL,
    name TEXT NOT NULL DEFAULT '',
    UNIQUE (funnel_id, position)
);
```

**Files:**
- Create: `internal/repository/funnels.go`
- Create: `internal/repository/funnels_test.go`
- Create: `internal/service/funnel.go`
- Modify: `internal/repository/repos.go` (add Funnels field — done above)

- [ ] **Step 1: Create `internal/repository/funnels.go`**

```go
package repository

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/sidneydekoning/analytics/internal/model"
)

// FunnelRepository handles CRUD for funnels and their steps,
// plus ordered drop-off queries against the events hypertable.
type FunnelRepository interface {
	ListBySite(ctx context.Context, siteID string) ([]*model.Funnel, error)
	GetWithSteps(ctx context.Context, id, siteID string) (*model.Funnel, []*model.FunnelStep, error)
	Create(ctx context.Context, f *model.Funnel, steps []*model.FunnelStep) error
	Delete(ctx context.Context, id, siteID string) error
	GetDropOff(ctx context.Context, siteID string, steps []*model.FunnelStep, from, to time.Time) ([]int64, error)
}

type pgFunnelRepository struct {
	pool *pgxpool.Pool
}

func (r *pgFunnelRepository) ListBySite(ctx context.Context, siteID string) ([]*model.Funnel, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, site_id, name, to_char(created_at,'YYYY-MM-DD') FROM funnels WHERE site_id=$1 ORDER BY created_at DESC`, siteID)
	if err != nil {
		return nil, fmt.Errorf("funnelRepository.ListBySite: %w", err)
	}
	defer rows.Close()
	var funnels []*model.Funnel
	for rows.Next() {
		f := &model.Funnel{}
		if err := rows.Scan(&f.ID, &f.SiteID, &f.Name, &f.CreatedAt); err != nil {
			return nil, fmt.Errorf("funnelRepository.ListBySite: scan: %w", err)
		}
		funnels = append(funnels, f)
	}
	return funnels, rows.Err()
}

func (r *pgFunnelRepository) GetWithSteps(ctx context.Context, id, siteID string) (*model.Funnel, []*model.FunnelStep, error) {
	f := &model.Funnel{}
	err := r.pool.QueryRow(ctx,
		`SELECT id, site_id, name, to_char(created_at,'YYYY-MM-DD') FROM funnels WHERE id=$1 AND site_id=$2`, id, siteID).
		Scan(&f.ID, &f.SiteID, &f.Name, &f.CreatedAt)
	if err != nil {
		return nil, nil, fmt.Errorf("funnelRepository.GetWithSteps: funnel: %w", err)
	}
	rows, err := r.pool.Query(ctx,
		`SELECT id, funnel_id, position, name, match_type, value FROM funnel_steps WHERE funnel_id=$1 ORDER BY position`, id)
	if err != nil {
		return nil, nil, fmt.Errorf("funnelRepository.GetWithSteps: steps: %w", err)
	}
	defer rows.Close()
	var steps []*model.FunnelStep
	for rows.Next() {
		s := &model.FunnelStep{}
		if err := rows.Scan(&s.ID, &s.FunnelID, &s.Position, &s.Name, &s.MatchType, &s.Value); err != nil {
			return nil, nil, fmt.Errorf("funnelRepository.GetWithSteps: scan step: %w", err)
		}
		steps = append(steps, s)
	}
	return f, steps, rows.Err()
}

func (r *pgFunnelRepository) Create(ctx context.Context, f *model.Funnel, steps []*model.FunnelStep) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("funnelRepository.Create: begin: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	if err := tx.QueryRow(ctx,
		`INSERT INTO funnels (site_id, name) VALUES ($1,$2) RETURNING id, to_char(created_at,'YYYY-MM-DD')`,
		f.SiteID, f.Name).Scan(&f.ID, &f.CreatedAt); err != nil {
		return fmt.Errorf("funnelRepository.Create: insert funnel: %w", err)
	}
	for _, s := range steps {
		s.FunnelID = f.ID
		if err := tx.QueryRow(ctx,
			`INSERT INTO funnel_steps (funnel_id, position, name, match_type, value) VALUES ($1,$2,$3,$4,$5) RETURNING id`,
			s.FunnelID, s.Position, s.Name, s.MatchType, s.Value).Scan(&s.ID); err != nil {
			return fmt.Errorf("funnelRepository.Create: insert step %d: %w", s.Position, err)
		}
	}
	return tx.Commit(ctx)
}

func (r *pgFunnelRepository) Delete(ctx context.Context, id, siteID string) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM funnels WHERE id=$1 AND site_id=$2`, id, siteID)
	if err != nil {
		return fmt.Errorf("funnelRepository.Delete: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// GetDropOff runs an ordered funnel query. It returns one visitor count per step,
// where step N count = visitors who completed steps 0..N in chronological order.
// Returns a slice of length len(steps). Returns an error if steps is empty.
//
// The query is built dynamically as a chain of CTEs:
//   step_0: visitors who hit step 0 condition in [from,to]
//   step_1: visitors in step_0 who ALSO hit step 1 AFTER their step_0 timestamp
//   step_2: visitors in step_1 who ALSO hit step 2 AFTER their step_1 timestamp
//   ...
//
// MatchType "url" matches e.url = step.Value on type='pageview'.
// MatchType "event" matches e.type = step.Value on any event.
func (r *pgFunnelRepository) GetDropOff(ctx context.Context, siteID string, steps []*model.FunnelStep, from, to time.Time) ([]int64, error) {
	if len(steps) == 0 {
		return nil, fmt.Errorf("funnelRepository.GetDropOff: no steps")
	}

	// Build positional query args: $1=siteID, $2=from, $3=to, $4...$N+3=step values
	args := []any{siteID, from, to}
	for _, s := range steps {
		args = append(args, s.Value)
	}

	var sb strings.Builder

	// First CTE — step_0
	sb.WriteString("WITH step_0 AS (\n")
	sb.WriteString("  SELECT DISTINCT visitor_id, MIN(timestamp) AS reached_at\n")
	sb.WriteString("  FROM events WHERE site_id=$1 AND timestamp BETWEEN $2 AND $3\n")
	writeStepCondition(&sb, steps[0], 4)
	sb.WriteString("  GROUP BY visitor_id\n),\n")

	// Chained CTEs for step_1..N-1
	for i := 1; i < len(steps); i++ {
		fmt.Fprintf(&sb, "step_%d AS (\n", i)
		sb.WriteString("  SELECT DISTINCT e.visitor_id, MIN(e.timestamp) AS reached_at\n")
		sb.WriteString("  FROM events e\n")
		fmt.Fprintf(&sb, "  JOIN step_%d s ON e.visitor_id = s.visitor_id AND e.timestamp > s.reached_at\n", i-1)
		sb.WriteString("  WHERE e.site_id=$1\n")
		writeStepCondition(&sb, steps[i], 4+i)
		sb.WriteString("  GROUP BY e.visitor_id\n")
		if i < len(steps)-1 {
			sb.WriteString("),\n")
		} else {
			sb.WriteString(")\n")
		}
	}

	// Final SELECT — one COUNT per step
	sb.WriteString("SELECT")
	for i := range steps {
		if i > 0 {
			sb.WriteString(",")
		}
		fmt.Fprintf(&sb, " (SELECT COUNT(*) FROM step_%d)", i)
	}

	rows := make([]int64, len(steps))
	scanArgs := make([]any, len(steps))
	for i := range rows {
		scanArgs[i] = &rows[i]
	}

	if err := r.pool.QueryRow(ctx, sb.String(), args...).Scan(scanArgs...); err != nil {
		return nil, fmt.Errorf("funnelRepository.GetDropOff: %w", err)
	}
	return rows, nil
}

// writeStepCondition appends the WHERE clause fragment for one step.
// argN is the positional placeholder index ($argN) for the step value.
func writeStepCondition(sb *strings.Builder, step *model.FunnelStep, argN int) {
	if step.MatchType == "event" {
		fmt.Fprintf(sb, "  AND e.type = $%d\n", argN)
	} else {
		// "url" or "goal" (treat goal as url match for now)
		fmt.Fprintf(sb, "  AND e.type = 'pageview' AND e.url = $%d\n", argN)
	}
}

// compile-time check
var _ FunnelRepository = (*pgFunnelRepository)(nil)
```

- [ ] **Step 2: Create `internal/repository/funnels_test.go`**

```go
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
		Domain: fmt.Sprintf("funneltest%d.com", time.Now().UnixNano()), Token: fmt.Sprintf("tk_fn%d", time.Now().UnixNano()), Timezone: "UTC",
	}
	require.NoError(t, repos.Sites.Create(ctx, site))

	steps := []*model.FunnelStep{
		{Position: 0, Name: "Homepage", MatchType: "url", Value: "https://funneltest.com/"},
		{Position: 1, Name: "Pricing",  MatchType: "url", Value: "https://funneltest.com/pricing"},
		{Position: 2, Name: "Signup",   MatchType: "url", Value: "https://funneltest.com/signup"},
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
		Domain: fmt.Sprintf("droptest%d.com", time.Now().UnixNano()), Token: fmt.Sprintf("tk_drop%d", time.Now().UnixNano()), Timezone: "UTC",
	}
	require.NoError(t, repos.Sites.Create(ctx, site))

	now := time.Now()

	// Write 3 events for visitor A: hits all 3 steps in order
	eventsA := []struct{ url string; offset time.Duration }{
		{"https://droptest.com/", 0},
		{"https://droptest.com/pricing", 1 * time.Second},
		{"https://droptest.com/signup", 2 * time.Second},
	}
	for _, ev := range eventsA {
		require.NoError(t, repos.Events.Write(ctx, &model.Event{
			SiteID: site.ID, Type: "pageview", URL: ev.url,
			VisitorID: "visitor-a", SessionID: "sess-a",
			Channel: "direct", Timestamp: now.Add(-5*time.Minute + ev.offset),
		}))
	}

	// Write 2 events for visitor B: hits steps 0 and 1 only
	for _, ev := range eventsA[:2] {
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
	assert.Equal(t, int64(2), counts[0]) // both visitors hit step 0
	assert.Equal(t, int64(2), counts[1]) // both visitors hit step 1
	assert.Equal(t, int64(1), counts[2]) // only visitor A hit step 2
}
```

- [ ] **Step 3: Update `internal/repository/repos.go`** to add `Funnels` field (alongside the `Goals` field added in Task 1):

```go
package repository

import "github.com/jackc/pgx/v5/pgxpool"

// Repos aggregates all repository interfaces for dependency injection.
type Repos struct {
	Users   UserRepository
	Sites   SiteRepository
	Events  EventRepository
	Stats   StatsRepository
	Admin   AdminRepository
	CMS     CMSRepository
	Goals   GoalRepository
	Funnels FunnelRepository
}

// New creates a Repos with all pg implementations wired up.
func New(pool *pgxpool.Pool) *Repos {
	return &Repos{
		Users:   &pgUserRepository{pool: pool},
		Sites:   &pgSiteRepository{pool: pool},
		Events:  &pgEventRepository{pool: pool},
		Stats:   &pgStatsRepository{pool: pool},
		Admin:   &pgAdminRepository{pool: pool},
		CMS:     &pgCMSRepository{pool: pool},
		Goals:   &pgGoalRepository{pool: pool},
		Funnels: &pgFunnelRepository{pool: pool},
	}
}
```

- [ ] **Step 4: Create `internal/service/funnel.go`**

This service converts raw `[]int64` drop-off counts from the repository into the `FunnelResult` presentation model used by templates.

```go
package service

import (
	"github.com/sidneydekoning/analytics/internal/model"
)

// BuildFunnelResult converts raw step visitor counts into a FunnelResult with
// drop-off percentages. step0Count is counts[0].
func BuildFunnelResult(funnel *model.Funnel, steps []*model.FunnelStep, counts []int64) *model.FunnelResult {
	result := &model.FunnelResult{
		FunnelID:   funnel.ID,
		FunnelName: funnel.Name,
		Steps:      make([]model.FunnelStepResult, len(steps)),
	}
	for i, s := range steps {
		sr := model.FunnelStepResult{
			Position: s.Position,
			Name:     s.Name,
			Visitors: counts[i],
		}
		if i == 0 && counts[0] > 0 {
			sr.Converted = 100.0
		} else if counts[0] > 0 {
			sr.Converted = float64(counts[i]) / float64(counts[0]) * 100
			if counts[i-1] > 0 {
				sr.DropOff = (1.0 - float64(counts[i])/float64(counts[i-1])) * 100
			}
		}
		result.Steps[i] = sr
	}
	return result
}
```

- [ ] **Step 5: Run tests**

```bash
go test ./internal/repository/... -run TestFunnelRepository -v
go build ./...
```

Expected: all PASS, binary builds.

- [ ] **Step 6: Commit**

```bash
git add internal/repository/funnels.go internal/repository/funnels_test.go internal/repository/repos.go internal/service/funnel.go
git commit -m "feat: add FunnelRepository with ordered drop-off CTE + FunnelService"
```

---

### Task 3: Site update + custom events query

**Files:**
- Modify: `internal/repository/site.go` — add `Update` method to interface + implementation
- Modify: `internal/repository/event.go` — add `ListCustomEvents` to interface + implementation

- [ ] **Step 1: Add `Update` to `SiteRepository` in `internal/repository/site.go`**

Add to the interface (after `Delete`):

```go
type SiteRepository interface {
	Create(ctx context.Context, s *model.Site) error
	GetByID(ctx context.Context, id string) (*model.Site, error)
	GetByToken(ctx context.Context, token string) (*model.Site, error)
	ListByOwner(ctx context.Context, ownerID string) ([]*model.Site, error)
	Delete(ctx context.Context, id string) error
	Update(ctx context.Context, s *model.Site) error
}
```

Add the implementation after the existing `Delete` method:

```go
func (r *pgSiteRepository) Update(ctx context.Context, s *model.Site) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE sites SET name=$2, timezone=$3 WHERE id=$1`,
		s.ID, s.Name, s.Timezone)
	if err != nil {
		return fmt.Errorf("siteRepository.Update: %w", err)
	}
	return nil
}
```

- [ ] **Step 2: Add `CustomEventStat` to `internal/model/goals.go`** (append to the file):

```go
// CustomEventStat is one row of the custom events dashboard — event type + count.
type CustomEventStat struct {
	EventType string
	URL       string
	Count     int64
}
```

- [ ] **Step 3: Add `ListCustomEvents` to `EventRepository` in `internal/repository/event.go`**

Add to the interface:

```go
type EventRepository interface {
	Write(ctx context.Context, e *model.Event) error
	WriteBatch(ctx context.Context, events []*model.Event) error
	CountBySite(ctx context.Context, siteID string, from, to time.Time) (int64, error)
	ListCustomEvents(ctx context.Context, siteID string, from, to time.Time, limit int) ([]*model.CustomEventStat, error)
}
```

Add the implementation at the bottom of `event.go`:

```go
func (r *pgEventRepository) ListCustomEvents(ctx context.Context, siteID string, from, to time.Time, limit int) ([]*model.CustomEventStat, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT type, url, COUNT(*) AS cnt
		FROM events
		WHERE site_id=$1 AND type != 'pageview' AND timestamp BETWEEN $2 AND $3
		GROUP BY type, url
		ORDER BY cnt DESC
		LIMIT $4
	`, siteID, from, to, limit)
	if err != nil {
		return nil, fmt.Errorf("eventRepository.ListCustomEvents: %w", err)
	}
	defer rows.Close()
	var stats []*model.CustomEventStat
	for rows.Next() {
		s := &model.CustomEventStat{}
		if err := rows.Scan(&s.EventType, &s.URL, &s.Count); err != nil {
			return nil, fmt.Errorf("eventRepository.ListCustomEvents: scan: %w", err)
		}
		stats = append(stats, s)
	}
	return stats, rows.Err()
}
```

- [ ] **Step 4: Build to verify no compile errors**

```bash
go build ./...
```

Expected: no output (success).

- [ ] **Step 5: Commit**

```bash
git add internal/repository/site.go internal/repository/event.go internal/model/goals.go
git commit -m "feat: add SiteRepository.Update and EventRepository.ListCustomEvents"
```

---

### Task 4: Site settings handler + template

**Files:**
- Modify: `internal/handler/sites.go` — add `Settings` (GET) and `UpdateSite` (POST) handlers
- Create: `templates/pages/dashboard/settings.html`
- Modify: `cmd/server/main.go` — add settings routes

- [ ] **Step 1: Add `Settings` and `UpdateSite` to `internal/handler/sites.go`**

Append after the existing `CheckTracking` handler:

```go
// Settings renders GET /sites/:siteID/settings.
func (h *SitesHandler) Settings(w http.ResponseWriter, r *http.Request) {
	siteID := chi.URLParam(r, "siteID")
	if h.repos == nil {
		http.NotFound(w, r)
		return
	}
	site, err := h.repos.Sites.GetByID(r.Context(), siteID)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	var csrf string
	if c, err := r.Cookie("csrf_token"); err == nil {
		csrf = c.Value
	}
	h.renderTemplate(w, "settings.html", map[string]any{
		"SiteID":           siteID,
		"SiteDomain":       site.Domain,
		"SiteBaseURL":      "/sites/" + siteID,
		"Site":             site,
		"ActiveNav":        "settings",
		"Period":           "30d",
		"AvailablePeriods": []struct{ Value, Label string }{},
		"CSRFToken":        csrf,
	})
}

// UpdateSite handles POST /sites/:siteID/settings.
func (h *SitesHandler) UpdateSite(w http.ResponseWriter, r *http.Request) {
	siteID := chi.URLParam(r, "siteID")
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	name := strings.TrimSpace(r.FormValue("name"))
	timezone := strings.TrimSpace(r.FormValue("timezone"))
	if name == "" {
		http.Error(w, "name required", http.StatusUnprocessableEntity)
		return
	}
	if timezone == "" {
		timezone = "UTC"
	}
	if err := h.repos.Sites.Update(r.Context(), &model.Site{ID: siteID, Name: name, Timezone: timezone}); err != nil {
		slog.Error("sites.UpdateSite", "error", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/sites/"+siteID+"/settings?updated=1", http.StatusSeeOther)
}
```

You will also need these imports in `sites.go` — verify the existing import block already includes `"strings"` and `"log/slog"`. If not, add them. The file uses `model.Site` so ensure `"github.com/sidneydekoning/analytics/internal/model"` is imported.

- [ ] **Step 2: Create `templates/pages/dashboard/settings.html`**

```html
{{template "dashboard.html" .}}
{{define "title"}}{{.SiteDomain}} — Settings{{end}}
{{define "content"}}
<h2 class="text-lg font-semibold text-gray-900 mb-6">Site settings</h2>

{{if .Updated}}
<div class="mb-4 p-3 bg-green-50 border border-green-200 rounded-lg text-green-700 text-sm">Settings saved.</div>
{{end}}

<div class="grid grid-cols-1 gap-6 max-w-2xl">

  {{/* General */}}
  <div class="bg-white rounded-xl border border-gray-100 p-6">
    <h3 class="text-sm font-semibold text-gray-900 mb-4">General</h3>
    <form method="POST" action="/sites/{{.SiteID}}/settings" class="space-y-4">
      <input type="hidden" name="_csrf" value="{{.CSRFToken}}">
      <div>
        <label class="block text-xs font-medium text-gray-600 mb-1">Site name</label>
        <input type="text" name="name" value="{{.Site.Name}}" required
               class="w-full border border-gray-200 rounded-lg px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-violet-500">
      </div>
      <div>
        <label class="block text-xs font-medium text-gray-600 mb-1">Timezone</label>
        <input type="text" name="timezone" value="{{.Site.Timezone}}" placeholder="UTC"
               class="w-full border border-gray-200 rounded-lg px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-violet-500">
        <p class="text-xs text-gray-400 mt-1">Use IANA timezone name, e.g. Europe/Amsterdam</p>
      </div>
      <button type="submit" class="btn-primary">Save changes</button>
    </form>
  </div>

  {{/* Tracking token */}}
  <div class="bg-white rounded-xl border border-gray-100 p-6">
    <h3 class="text-sm font-semibold text-gray-900 mb-2">Tracking token</h3>
    <p class="text-xs text-gray-500 mb-3">Embed this token in your tracking script's <code>data-site</code> attribute.</p>
    <div class="bg-gray-50 rounded-lg px-4 py-3 font-mono text-sm text-gray-700 select-all">{{.Site.Token}}</div>
  </div>

  {{/* Danger zone */}}
  <div class="bg-white rounded-xl border border-red-100 p-6">
    <h3 class="text-sm font-semibold text-red-700 mb-2">Danger zone</h3>
    <p class="text-xs text-gray-500 mb-3">Permanently deletes this site and all its analytics data. This cannot be undone.</p>
    <form method="POST" action="/sites/{{.SiteID}}/delete" onsubmit="return confirm('Delete {{.Site.Domain}} and all its data? This cannot be undone.')">
      <input type="hidden" name="_csrf" value="{{.CSRFToken}}">
      <button type="submit" class="px-4 py-2 text-sm font-medium bg-red-600 text-white rounded-lg hover:bg-red-700">Delete site</button>
    </form>
  </div>

</div>
{{end}}
```

- [ ] **Step 3: Add `DeleteSite` (user-facing) to `internal/handler/sites.go`**

```go
// DeleteSite handles POST /sites/:siteID/delete (owner-only delete from settings page).
func (h *SitesHandler) DeleteSite(w http.ResponseWriter, r *http.Request) {
	siteID := chi.URLParam(r, "siteID")
	if err := h.repos.Sites.Delete(r.Context(), siteID); err != nil {
		slog.Error("sites.DeleteSite", "error", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
}
```

- [ ] **Step 4: Add settings + delete routes to `cmd/server/main.go`**

In the authenticated routes group (`r.With(jwtAuth).Group`), add after the existing `/sites/{siteID}/funnels` route:

```go
r.Get("/sites/{siteID}/settings", sitesHandler.Settings)
r.Post("/sites/{siteID}/settings", sitesHandler.UpdateSite)
r.Post("/sites/{siteID}/delete", sitesHandler.DeleteSite)
```

Also handle the `?updated=1` query param in the `Settings` handler. Update `Settings` in `sites.go` to add `"Updated": r.URL.Query().Get("updated") == "1"` to the template data map.

- [ ] **Step 5: Build**

```bash
go build ./...
```

Expected: success.

- [ ] **Step 6: Commit**

```bash
git add internal/handler/sites.go templates/pages/dashboard/settings.html cmd/server/main.go
git commit -m "feat: site settings page (rename, timezone, token display, delete)"
```

---

### Task 5: Goals management UI

**Files:**
- Modify: `internal/handler/sites.go` — add `GoalsList`, `CreateGoal`, `DeleteGoal` handlers
- Create: `templates/pages/dashboard/goals.html`
- Modify: `cmd/server/main.go` — add goals routes

- [ ] **Step 1: Add goals handlers to `internal/handler/sites.go`**

```go
// GoalsList renders GET /sites/:siteID/goals.
func (h *SitesHandler) GoalsList(w http.ResponseWriter, r *http.Request) {
	siteID := chi.URLParam(r, "siteID")
	site, err := h.repos.Sites.GetByID(r.Context(), siteID)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	goals, err := h.repos.Goals.ListBySite(r.Context(), siteID)
	if err != nil {
		slog.Error("sites.GoalsList", "error", err)
		goals = nil
	}
	var csrf string
	if c, err := r.Cookie("csrf_token"); err == nil {
		csrf = c.Value
	}
	h.renderTemplate(w, "goals.html", map[string]any{
		"SiteID": siteID, "SiteDomain": site.Domain,
		"SiteBaseURL": "/sites/" + siteID, "ActiveNav": "settings",
		"Period": "30d", "AvailablePeriods": []struct{ Value, Label string }{},
		"Goals": goals, "CSRFToken": csrf,
	})
}

// CreateGoal handles POST /sites/:siteID/goals.
func (h *SitesHandler) CreateGoal(w http.ResponseWriter, r *http.Request) {
	siteID := chi.URLParam(r, "siteID")
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	name := strings.TrimSpace(r.FormValue("name"))
	goalType := r.FormValue("type")
	value := strings.TrimSpace(r.FormValue("value"))
	if name == "" || value == "" {
		http.Error(w, "name and value required", http.StatusUnprocessableEntity)
		return
	}
	if goalType != "pageview" && goalType != "event" && goalType != "outbound" {
		goalType = "pageview"
	}
	g := &model.Goal{SiteID: siteID, Name: name, Type: goalType, Value: value}
	if err := h.repos.Goals.Create(r.Context(), g); err != nil {
		slog.Error("sites.CreateGoal", "error", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/sites/"+siteID+"/goals", http.StatusSeeOther)
}

// DeleteGoal handles POST /sites/:siteID/goals/:goalID/delete.
func (h *SitesHandler) DeleteGoal(w http.ResponseWriter, r *http.Request) {
	siteID := chi.URLParam(r, "siteID")
	goalID := chi.URLParam(r, "goalID")
	if err := h.repos.Goals.Delete(r.Context(), goalID, siteID); err != nil {
		slog.Error("sites.DeleteGoal", "error", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/sites/"+siteID+"/goals", http.StatusSeeOther)
}
```

- [ ] **Step 2: Create `templates/pages/dashboard/goals.html`**

```html
{{template "dashboard.html" .}}
{{define "title"}}{{.SiteDomain}} — Goals{{end}}
{{define "content"}}
<div class="flex items-center justify-between mb-6">
  <h2 class="text-lg font-semibold text-gray-900">Conversion goals</h2>
</div>

{{/* Create form */}}
<div class="bg-white rounded-xl border border-gray-100 p-6 mb-6 max-w-2xl">
  <h3 class="text-sm font-semibold text-gray-700 mb-4">Add goal</h3>
  <form method="POST" action="/sites/{{.SiteID}}/goals" class="grid grid-cols-1 gap-3">
    <input type="hidden" name="_csrf" value="{{.CSRFToken}}">
    <div class="grid grid-cols-3 gap-3">
      <div>
        <label class="block text-xs font-medium text-gray-600 mb-1">Name</label>
        <input type="text" name="name" placeholder="e.g. Signup" required
               class="w-full border border-gray-200 rounded-lg px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-violet-500">
      </div>
      <div>
        <label class="block text-xs font-medium text-gray-600 mb-1">Type</label>
        <select name="type" class="w-full border border-gray-200 rounded-lg px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-violet-500">
          <option value="pageview">Pageview</option>
          <option value="event">Custom event</option>
          <option value="outbound">Outbound click</option>
        </select>
      </div>
      <div>
        <label class="block text-xs font-medium text-gray-600 mb-1">Value (URL or event name)</label>
        <input type="text" name="value" placeholder="/signup or file_download" required
               class="w-full border border-gray-200 rounded-lg px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-violet-500">
      </div>
    </div>
    <div>
      <button type="submit" class="btn-primary">Add goal</button>
    </div>
  </form>
</div>

{{/* Goals list */}}
<div class="bg-white rounded-xl border border-gray-100 overflow-hidden max-w-2xl">
  <table class="w-full text-sm">
    <thead class="bg-gray-50">
      <tr class="text-xs text-gray-400 border-b border-gray-100">
        <th class="text-left px-4 py-3 font-medium">Name</th>
        <th class="text-left px-4 py-3 font-medium">Type</th>
        <th class="text-left px-4 py-3 font-medium">Value</th>
        <th class="px-4 py-3"></th>
      </tr>
    </thead>
    <tbody>
      {{range .Goals}}
      <tr class="border-b border-gray-50 last:border-0 hover:bg-gray-50">
        <td class="px-4 py-3 font-medium text-gray-900">{{.Name}}</td>
        <td class="px-4 py-3"><span class="px-2 py-0.5 text-xs rounded-full bg-violet-50 text-violet-600">{{.Type}}</span></td>
        <td class="px-4 py-3 font-mono text-xs text-gray-500">{{.Value}}</td>
        <td class="px-4 py-3">
          <form method="POST" action="/sites/{{$.SiteID}}/goals/{{.ID}}/delete" onsubmit="return confirm('Delete this goal?')">
            <input type="hidden" name="_csrf" value="{{$.CSRFToken}}">
            <button type="submit" class="text-xs text-red-400 hover:text-red-600">Delete</button>
          </form>
        </td>
      </tr>
      {{else}}
      <tr><td colspan="4" class="px-4 py-8 text-center text-gray-400 text-sm">No goals yet. Add one above.</td></tr>
      {{end}}
    </tbody>
  </table>
</div>
{{end}}
```

- [ ] **Step 3: Add goals routes in `cmd/server/main.go`**

In the authenticated routes group, add:

```go
r.Get("/sites/{siteID}/goals", sitesHandler.GoalsList)
r.Post("/sites/{siteID}/goals", sitesHandler.CreateGoal)
r.Post("/sites/{siteID}/goals/{goalID}/delete", sitesHandler.DeleteGoal)
```

- [ ] **Step 4: Build**

```bash
go build ./...
```

Expected: success.

- [ ] **Step 5: Commit**

```bash
git add internal/handler/sites.go templates/pages/dashboard/goals.html cmd/server/main.go
git commit -m "feat: goals management UI (list, create, delete)"
```

---

### Task 6: Live events dashboard

Replace the "coming soon" events stub with a real table of custom event types and their counts.

**Files:**
- Modify: `internal/handler/dashboard.go` — update `Events` handler to query real data
- Modify: `templates/pages/dashboard/events.html` — replace stub with data table

- [ ] **Step 1: Update `Events` handler in `internal/handler/dashboard.go`**

Replace the existing `Events` function:

```go
// Events renders GET /sites/:siteID/events.
func (h *DashboardHandler) Events(w http.ResponseWriter, r *http.Request) {
	siteID := chi.URLParam(r, "siteID")
	if h.repos == nil {
		http.NotFound(w, r)
		return
	}
	site, err := h.repos.Sites.GetByID(r.Context(), siteID)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	period := periodParam(r)
	from, to := periodRange(period)

	events, err := h.repos.Events.ListCustomEvents(r.Context(), siteID, from, to, 50)
	if err != nil {
		slog.Error("dashboard.Events", "error", err)
		events = nil
	}

	h.renderDash(w, "events.html", map[string]any{
		"SiteID": siteID, "SiteDomain": site.Domain,
		"SiteBaseURL": "/sites/" + siteID, "ActiveNav": "events",
		"Period": period, "AvailablePeriods": periodsAvailable,
		"Events": events,
	})
}
```

The `periodRange` helper already exists in `dashboard.go` — verify it's named exactly that. If it's named differently (e.g., `dateRange`), use the correct name. Search with: `grep -n "func.*[Pp]eriod[Rr]ange\|func.*[Dd]ate[Rr]ange" internal/handler/dashboard.go`

- [ ] **Step 2: Replace `templates/pages/dashboard/events.html`**

```html
{{template "dashboard.html" .}}
{{define "title"}}{{.SiteDomain}} — Events{{end}}
{{define "content"}}
<div class="flex items-center justify-between mb-6">
  <h2 class="text-lg font-semibold text-gray-900">Custom events</h2>
  <a href="/sites/{{.SiteID}}/goals" class="text-xs text-violet-600 hover:underline">Manage goals →</a>
</div>

<div class="bg-white rounded-xl border border-gray-100 overflow-hidden mb-6">
  <table class="w-full text-sm">
    <thead class="bg-gray-50">
      <tr class="text-xs text-gray-400 border-b border-gray-100">
        <th class="text-left px-4 py-3 font-medium">Event</th>
        <th class="text-left px-4 py-3 font-medium">Page / URL</th>
        <th class="text-right px-4 py-3 font-medium">Count</th>
      </tr>
    </thead>
    <tbody>
      {{range .Events}}
      <tr class="border-b border-gray-50 last:border-0 hover:bg-gray-50">
        <td class="px-4 py-3"><span class="px-2 py-0.5 text-xs rounded-full bg-violet-50 text-violet-700 font-medium">{{.EventType}}</span></td>
        <td class="px-4 py-3 text-gray-500 text-xs truncate max-w-xs">{{.URL}}</td>
        <td class="px-4 py-3 text-right font-medium text-gray-900">{{formatNumber .Count}}</td>
      </tr>
      {{else}}
      <tr>
        <td colspan="3" class="px-4 py-10 text-center text-gray-400 text-sm">
          <p class="mb-2">No custom events yet.</p>
          <p class="text-xs">Fire events with: <code class="bg-gray-100 px-2 py-0.5 rounded">window.analytics.track('event-name', {key: 'value'})</code></p>
        </td>
      </tr>
      {{end}}
    </tbody>
  </table>
</div>
{{end}}
```

- [ ] **Step 3: Check the `periodRange` helper name**

```bash
grep -n "func.*[Pp]eriod\|func.*[Dd]ate[Rr]ange" internal/handler/dashboard.go
```

If the function is named differently than `periodRange`, update the Events handler accordingly.

- [ ] **Step 4: Build**

```bash
go build ./...
```

Expected: success.

- [ ] **Step 5: Commit**

```bash
git add internal/handler/dashboard.go templates/pages/dashboard/events.html
git commit -m "feat: live custom events dashboard"
```

---

### Task 7: Funnels list + create funnel form

**Files:**
- Modify: `internal/handler/dashboard.go` — update `Funnels` handler, add `CreateFunnel`, `DeleteFunnel`
- Modify: `templates/pages/dashboard/funnels.html` — replace stub with real list + create form
- Modify: `cmd/server/main.go` — add funnel create/delete routes

- [ ] **Step 1: Update `Funnels` handler and add `CreateFunnel`/`DeleteFunnel` in `internal/handler/dashboard.go`**

Replace the existing `Funnels` function and add two more:

```go
// Funnels renders GET /sites/:siteID/funnels.
func (h *DashboardHandler) Funnels(w http.ResponseWriter, r *http.Request) {
	siteID := chi.URLParam(r, "siteID")
	if h.repos == nil {
		http.NotFound(w, r)
		return
	}
	site, err := h.repos.Sites.GetByID(r.Context(), siteID)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	funnels, err := h.repos.Funnels.ListBySite(r.Context(), siteID)
	if err != nil {
		slog.Error("dashboard.Funnels", "error", err)
		funnels = nil
	}
	var csrf string
	if c, err := r.Cookie("csrf_token"); err == nil {
		csrf = c.Value
	}
	h.renderDash(w, "funnels.html", map[string]any{
		"SiteID": siteID, "SiteDomain": site.Domain,
		"SiteBaseURL": "/sites/" + siteID, "ActiveNav": "funnels",
		"Period": "30d", "AvailablePeriods": periodsAvailable,
		"Funnels": funnels, "CSRFToken": csrf,
	})
}

// CreateFunnel handles POST /sites/:siteID/funnels.
// Accepts form fields: name (string), step_name[] ([]string), step_type[] ([]string), step_value[] ([]string).
func (h *DashboardHandler) CreateFunnel(w http.ResponseWriter, r *http.Request) {
	siteID := chi.URLParam(r, "siteID")
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	name := strings.TrimSpace(r.FormValue("name"))
	if name == "" {
		http.Error(w, "name required", http.StatusUnprocessableEntity)
		return
	}
	stepNames := r.Form["step_name"]
	stepTypes := r.Form["step_type"]
	stepValues := r.Form["step_value"]
	if len(stepNames) < 2 {
		http.Error(w, "at least 2 steps required", http.StatusUnprocessableEntity)
		return
	}
	var steps []*model.FunnelStep
	for i := range stepNames {
		mt := "url"
		if i < len(stepTypes) && stepTypes[i] == "event" {
			mt = "event"
		}
		val := ""
		if i < len(stepValues) {
			val = strings.TrimSpace(stepValues[i])
		}
		sname := strings.TrimSpace(stepNames[i])
		if sname == "" || val == "" {
			continue
		}
		steps = append(steps, &model.FunnelStep{
			Position: i, Name: sname, MatchType: mt, Value: val,
		})
	}
	if len(steps) < 2 {
		http.Error(w, "at least 2 valid steps required", http.StatusUnprocessableEntity)
		return
	}
	f := &model.Funnel{SiteID: siteID, Name: name}
	if err := h.repos.Funnels.Create(r.Context(), f, steps); err != nil {
		slog.Error("dashboard.CreateFunnel", "error", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/sites/"+siteID+"/funnels/"+f.ID, http.StatusSeeOther)
}

// DeleteFunnel handles POST /sites/:siteID/funnels/:funnelID/delete.
func (h *DashboardHandler) DeleteFunnel(w http.ResponseWriter, r *http.Request) {
	siteID := chi.URLParam(r, "siteID")
	funnelID := chi.URLParam(r, "funnelID")
	if err := h.repos.Funnels.Delete(r.Context(), funnelID, siteID); err != nil {
		slog.Error("dashboard.DeleteFunnel", "error", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/sites/"+siteID+"/funnels", http.StatusSeeOther)
}
```

You need `"strings"` imported in `dashboard.go`. Check the existing import block. If absent, add it.

- [ ] **Step 2: Replace `templates/pages/dashboard/funnels.html`**

```html
{{template "dashboard.html" .}}
{{define "title"}}{{.SiteDomain}} — Funnels{{end}}
{{define "content"}}
<div class="flex items-center justify-between mb-6">
  <h2 class="text-lg font-semibold text-gray-900">Funnels</h2>
</div>

{{/* Create funnel form */}}
<div class="bg-white rounded-xl border border-gray-100 p-6 mb-6 max-w-2xl" x-data="{ steps: [{name:'',type:'url',value:''},{name:'',type:'url',value:''}] }">
  <h3 class="text-sm font-semibold text-gray-700 mb-4">New funnel</h3>
  <form method="POST" action="/sites/{{.SiteID}}/funnels" class="space-y-4">
    <input type="hidden" name="_csrf" value="{{.CSRFToken}}">
    <div>
      <label class="block text-xs font-medium text-gray-600 mb-1">Funnel name</label>
      <input type="text" name="name" placeholder="e.g. Signup flow" required
             class="w-full border border-gray-200 rounded-lg px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-violet-500">
    </div>
    <div class="space-y-2">
      <label class="block text-xs font-medium text-gray-600">Steps (in order, minimum 2)</label>
      <template x-for="(step, idx) in steps" :key="idx">
        <div class="flex gap-2 items-center">
          <span class="text-xs text-gray-400 w-4" x-text="idx + 1 + '.'"></span>
          <input type="text" name="step_name" x-model="step.name" placeholder="Step name" required
                 class="border border-gray-200 rounded-lg px-3 py-1.5 text-sm focus:outline-none focus:ring-2 focus:ring-violet-500 w-32">
          <select name="step_type" x-model="step.type"
                  class="border border-gray-200 rounded-lg px-3 py-1.5 text-sm focus:outline-none focus:ring-2 focus:ring-violet-500">
            <option value="url">URL</option>
            <option value="event">Event</option>
          </select>
          <input type="text" name="step_value" x-model="step.value" placeholder="https://... or event-name" required
                 class="border border-gray-200 rounded-lg px-3 py-1.5 text-sm focus:outline-none focus:ring-2 focus:ring-violet-500 flex-1">
          <button type="button" x-show="steps.length > 2" @click="steps.splice(idx, 1)"
                  class="text-gray-300 hover:text-red-400 text-lg leading-none">&times;</button>
        </div>
      </template>
      <button type="button" @click="steps.push({name:'',type:'url',value:''})"
              class="text-xs text-violet-600 hover:underline mt-1">+ Add step</button>
    </div>
    <button type="submit" class="btn-primary">Create funnel</button>
  </form>
</div>

{{/* Funnel list */}}
{{if .Funnels}}
<div class="bg-white rounded-xl border border-gray-100 overflow-hidden max-w-2xl">
  <table class="w-full text-sm">
    <thead class="bg-gray-50">
      <tr class="text-xs text-gray-400 border-b border-gray-100">
        <th class="text-left px-4 py-3 font-medium">Name</th>
        <th class="text-left px-4 py-3 font-medium">Created</th>
        <th class="px-4 py-3"></th>
      </tr>
    </thead>
    <tbody>
      {{range .Funnels}}
      <tr class="border-b border-gray-50 last:border-0 hover:bg-gray-50">
        <td class="px-4 py-3 font-medium text-gray-900">
          <a href="/sites/{{$.SiteID}}/funnels/{{.ID}}" class="hover:text-violet-600">{{.Name}}</a>
        </td>
        <td class="px-4 py-3 text-gray-400 text-xs">{{.CreatedAt}}</td>
        <td class="px-4 py-3 flex gap-3 items-center">
          <a href="/sites/{{$.SiteID}}/funnels/{{.ID}}" class="text-xs text-violet-600 hover:underline">View</a>
          <form method="POST" action="/sites/{{$.SiteID}}/funnels/{{.ID}}/delete" onsubmit="return confirm('Delete this funnel?')" style="display:inline">
            <input type="hidden" name="_csrf" value="{{$.CSRFToken}}">
            <button type="submit" class="text-xs text-red-400 hover:text-red-600">Delete</button>
          </form>
        </td>
      </tr>
      {{end}}
    </tbody>
  </table>
</div>
{{end}}
{{end}}
```

- [ ] **Step 3: Add funnel routes to `cmd/server/main.go`**

In the authenticated routes group, add after the existing `r.Get("/sites/{siteID}/funnels", ...)` route:

```go
r.Post("/sites/{siteID}/funnels", dashHandler.CreateFunnel)
r.Post("/sites/{siteID}/funnels/{funnelID}/delete", dashHandler.DeleteFunnel)
r.Get("/sites/{siteID}/funnels/{funnelID}", dashHandler.FunnelDetail)
```

The `FunnelDetail` handler is added in Task 8. Add all three routes now — Go will give a compile error until Task 8 is done, so add just the first two routes for now and add the third in Task 8.

- [ ] **Step 4: Build**

```bash
go build ./...
```

Expected: success (or compile error only for `FunnelDetail` which is fine — add that method as a stub if needed).

To unblock the build, add a temporary stub in `dashboard.go`:

```go
// FunnelDetail is implemented in Task 8.
func (h *DashboardHandler) FunnelDetail(w http.ResponseWriter, r *http.Request) {
	http.NotFound(w, r)
}
```

- [ ] **Step 5: Commit**

```bash
git add internal/handler/dashboard.go templates/pages/dashboard/funnels.html cmd/server/main.go
git commit -m "feat: funnels list + create funnel form"
```

---

### Task 8: Funnel detail view with drop-off chart

**Files:**
- Modify: `internal/handler/dashboard.go` — replace stub `FunnelDetail` with real implementation
- Create: `templates/pages/dashboard/funnel-detail.html`

- [ ] **Step 1: Replace `FunnelDetail` stub in `internal/handler/dashboard.go`**

```go
// FunnelDetail renders GET /sites/:siteID/funnels/:funnelID.
func (h *DashboardHandler) FunnelDetail(w http.ResponseWriter, r *http.Request) {
	siteID := chi.URLParam(r, "siteID")
	funnelID := chi.URLParam(r, "funnelID")
	if h.repos == nil {
		http.NotFound(w, r)
		return
	}
	site, err := h.repos.Sites.GetByID(r.Context(), siteID)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	funnel, steps, err := h.repos.Funnels.GetWithSteps(r.Context(), funnelID, siteID)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	period := periodParam(r)
	from, to := periodRange(period)

	counts, err := h.repos.Funnels.GetDropOff(r.Context(), siteID, steps, from, to)
	if err != nil {
		slog.Error("dashboard.FunnelDetail", "error", err)
		counts = make([]int64, len(steps))
	}

	result := service.BuildFunnelResult(funnel, steps, counts)

	var csrf string
	if c, err := r.Cookie("csrf_token"); err == nil {
		csrf = c.Value
	}
	h.renderDash(w, "funnel-detail.html", map[string]any{
		"SiteID": siteID, "SiteDomain": site.Domain,
		"SiteBaseURL": "/sites/" + siteID, "ActiveNav": "funnels",
		"Period": period, "AvailablePeriods": periodsAvailable,
		"Result": result, "CSRFToken": csrf,
	})
}
```

You need to import `"github.com/sidneydekoning/analytics/internal/service"` in `dashboard.go` if not already imported. Check the import block; add it if absent.

- [ ] **Step 2: Create `templates/pages/dashboard/funnel-detail.html`**

```html
{{template "dashboard.html" .}}
{{define "title"}}{{.SiteDomain}} — {{.Result.FunnelName}}{{end}}
{{define "content"}}
<div class="flex items-center gap-3 mb-6">
  <a href="/sites/{{.SiteID}}/funnels" class="text-xs text-gray-400 hover:text-gray-700">← Funnels</a>
  <h2 class="text-lg font-semibold text-gray-900">{{.Result.FunnelName}}</h2>
</div>

{{if .Result.Steps}}
<div class="bg-white rounded-xl border border-gray-100 p-6 max-w-3xl">
  <div class="space-y-3">
    {{range .Result.Steps}}
    <div>
      <div class="flex items-center justify-between mb-1">
        <span class="text-sm font-medium text-gray-700">
          <span class="text-gray-400 text-xs mr-2">{{.Position | inc}}</span>{{.Name}}
        </span>
        <div class="text-right">
          <span class="text-sm font-semibold text-gray-900">{{.Visitors}}</span>
          <span class="text-xs text-gray-400 ml-1">({{printf "%.1f" .Converted}}%)</span>
          {{if gt .Position 0}}
          <span class="text-xs text-red-400 ml-2">−{{printf "%.1f" .DropOff}}% drop</span>
          {{end}}
        </div>
      </div>
      <div class="w-full bg-gray-100 rounded-full h-6 overflow-hidden">
        <div class="h-6 rounded-full bg-violet-500 transition-all"
             style="width: {{printf "%.1f" .Converted}}%"></div>
      </div>
    </div>
    {{end}}
  </div>

  {{with index .Result.Steps 0}}
  {{with index $.Result.Steps (len $.Result.Steps | dec)}}
  <div class="mt-6 pt-4 border-t border-gray-100 flex justify-between text-sm">
    <span class="text-gray-500">Overall conversion</span>
    <span class="font-semibold text-gray-900">{{printf "%.1f" .Converted}}%</span>
  </div>
  {{end}}
  {{end}}
</div>
{{else}}
<div class="bg-white rounded-xl border border-gray-100 p-12 text-center">
  <p class="text-gray-400 text-sm">No data yet for this funnel in the selected period.</p>
</div>
{{end}}
{{end}}
```

The template uses `inc` and `dec` template functions to compute `Position + 1` and `len - 1`. Add these to the `FuncMap` in `cmd/server/main.go`:

```go
"inc": func(i int) int { return i + 1 },
"dec": func(i int) int { return i - 1 },
```

Add them to the `funcs` map in the `buildTemplateMap` function in `cmd/server/main.go`, alongside the existing entries.

- [ ] **Step 3: Verify the import of `service` in `dashboard.go`**

```bash
grep "service" internal/handler/dashboard.go
```

If the `service` package isn't imported, add it to the import block:
`"github.com/sidneydekoning/analytics/internal/service"`

- [ ] **Step 4: Run tests**

```bash
go test ./... -v 2>&1 | tail -20
```

Expected: all packages pass.

- [ ] **Step 5: Build**

```bash
go build ./...
```

Expected: success.

- [ ] **Step 6: Commit**

```bash
git add internal/handler/dashboard.go templates/pages/dashboard/funnel-detail.html cmd/server/main.go
git commit -m "feat: funnel detail view with ordered drop-off bars"
```

---

## Self-Review

**Spec coverage check:**

| Spec requirement | Task |
|---|---|
| Funnels with drop-off analysis | Tasks 2, 7, 8 |
| Goals: pageview / event / outbound | Tasks 1, 5 |
| Custom events dashboard | Task 6 |
| Site settings (rename, timezone, token, delete) | Task 4 |
| Goals management UI | Task 5 |

**Placeholder scan:** No TBD, no "implement later", no vague steps — all code shown.

**Type consistency check:**
- `model.FunnelStep.MatchType` defined in Task 1, used in Tasks 2, 7, 8 — consistent.
- `model.FunnelResult.Steps` is `[]FunnelStepResult` (value, not pointer) — used consistently in Tasks 2 and 8.
- `repos.Goals` and `repos.Funnels` wired in Task 1/2, used in Tasks 5, 7, 8 — consistent.
- `periodRange` — Task 6 uses it; if the existing helper is named differently, Task 6 step 3 instructs to verify the name.
- `inc`/`dec` FuncMap functions added in Task 8 step 2 — template uses them in `funnel-detail.html`.

**Fixed issues:**
- `writeStepCondition` in `funnels.go` references `e.url` in the first CTE (step_0) but the alias is different — corrected: step_0 uses `events` table directly (no alias), so `e.` should be omitted from `writeStepCondition` for step 0. Fixed: `writeStepCondition` only writes the condition fragment (no table prefix needed because the field name is the same in both aliased and non-aliased contexts). The CTE bodies themselves use the correct reference — step_0 references `events.url` via `url` (no alias), step_1+ references `e.url` (aliased). The `writeStepCondition` helper writes `AND e.type...` which is wrong for step_0. **Fix**: pass a `tableAlias string` to `writeStepCondition` — `"events"` for step_0 and `"e"` for steps 1+. Update the implementation accordingly:

```go
func (r *pgFunnelRepository) GetDropOff(...) {
    // ...
    // First CTE
    sb.WriteString("WITH step_0 AS (\n")
    sb.WriteString("  SELECT DISTINCT visitor_id, MIN(timestamp) AS reached_at\n")
    sb.WriteString("  FROM events WHERE site_id=$1 AND timestamp BETWEEN $2 AND $3\n")
    writeStepCond(&sb, steps[0], 4, "")  // no table alias in WHERE after FROM events
    sb.WriteString("  GROUP BY visitor_id\n),\n")

    for i := 1; i < len(steps); i++ {
        fmt.Fprintf(&sb, "step_%d AS (\n", i)
        sb.WriteString("  SELECT DISTINCT e.visitor_id, MIN(e.timestamp) AS reached_at\n")
        sb.WriteString("  FROM events e\n")
        fmt.Fprintf(&sb, "  JOIN step_%d s ON e.visitor_id = s.visitor_id AND e.timestamp > s.reached_at\n", i-1)
        sb.WriteString("  WHERE e.site_id=$1\n")
        writeStepCond(&sb, steps[i], 4+i, "e")  // table alias "e"
        // ...
    }
}

func writeStepCond(sb *strings.Builder, step *model.FunnelStep, argN int, alias string) {
    col := func(name string) string {
        if alias == "" {
            return name
        }
        return alias + "." + name
    }
    if step.MatchType == "event" {
        fmt.Fprintf(sb, "  AND %s = $%d\n", col("type"), argN)
    } else {
        fmt.Fprintf(sb, "  AND %s = 'pageview' AND %s = $%d\n", col("type"), col("url"), argN)
    }
}
```

Replace the `writeStepCondition` function and its call sites in `funnels.go` with `writeStepCond` as shown above.
