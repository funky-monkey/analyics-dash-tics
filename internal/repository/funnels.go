package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/funky-monkey/analyics-dash-tics/internal/model"
)

// FunnelRepository handles CRUD for funnels + ordered drop-off analysis.
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
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil, ErrNotFound
	}
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
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("funnelRepository.Create: commit: %w", err)
	}
	return nil
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

// GetDropOff runs an ordered funnel query and returns one visitor count per step.
// Step N count = visitors who completed steps 0..N in chronological order.
//
// The query is built as a chain of CTEs:
//
//	step_0: visitors who hit step 0 condition in [from,to]
//	step_1: visitors in step_0 who ALSO hit step 1 AFTER their step_0 timestamp
//	...
//
// MatchType "url" matches on type='pageview' AND url=value.
// MatchType "event" or "goal" matches on type=value (custom event name).
func (r *pgFunnelRepository) GetDropOff(ctx context.Context, siteID string, steps []*model.FunnelStep, from, to time.Time) ([]int64, error) {
	if len(steps) == 0 {
		return nil, fmt.Errorf("funnelRepository.GetDropOff: no steps")
	}

	// args: $1=siteID, $2=from, $3=to, $4...$N+3=step values
	args := []any{siteID, from, to}
	for _, s := range steps {
		args = append(args, s.Value)
	}

	var sb strings.Builder

	// step_0: no table alias, direct FROM events
	sb.WriteString("WITH step_0 AS (\n")
	sb.WriteString("  SELECT DISTINCT visitor_id, MIN(timestamp) AS reached_at\n")
	sb.WriteString("  FROM events WHERE site_id=$1 AND timestamp BETWEEN $2 AND $3\n")
	writeStepCond(&sb, steps[0], 4, "")
	if len(steps) > 1 {
		sb.WriteString("  GROUP BY visitor_id\n),\n")
	} else {
		sb.WriteString("  GROUP BY visitor_id\n)\n")
	}

	// step_1..N-1: aliased as "e", joined to previous step
	for i := 1; i < len(steps); i++ {
		fmt.Fprintf(&sb, "step_%d AS (\n", i)
		sb.WriteString("  SELECT DISTINCT e.visitor_id, MIN(e.timestamp) AS reached_at\n")
		sb.WriteString("  FROM events e\n")
		fmt.Fprintf(&sb, "  JOIN step_%d s ON e.visitor_id = s.visitor_id AND e.timestamp > s.reached_at\n", i-1)
		sb.WriteString("  WHERE e.site_id=$1\n")
		writeStepCond(&sb, steps[i], 4+i, "e")
		sb.WriteString("  GROUP BY e.visitor_id\n")
		if i < len(steps)-1 {
			sb.WriteString("),\n")
		} else {
			sb.WriteString(")\n")
		}
	}

	// Final SELECT: one COUNT per step
	sb.WriteString("SELECT")
	for i := range steps {
		if i > 0 {
			sb.WriteString(",")
		}
		fmt.Fprintf(&sb, " (SELECT COUNT(*) FROM step_%d)", i)
	}

	result := make([]int64, len(steps))
	scanArgs := make([]any, len(steps))
	for i := range result {
		scanArgs[i] = &result[i]
	}

	if err := r.pool.QueryRow(ctx, sb.String(), args...).Scan(scanArgs...); err != nil {
		return nil, fmt.Errorf("funnelRepository.GetDropOff: %w", err)
	}
	return result, nil
}

// writeStepCond appends the WHERE condition for one step.
// alias is "" for step_0 (unaliased FROM events) or "e" for subsequent steps.
func writeStepCond(sb *strings.Builder, step *model.FunnelStep, argN int, alias string) {
	col := func(name string) string {
		if alias == "" {
			return name
		}
		return alias + "." + name
	}
	if step.MatchType == "event" || step.MatchType == "goal" {
		fmt.Fprintf(sb, "  AND %s = $%d\n", col("type"), argN)
	} else {
		fmt.Fprintf(sb, "  AND %s = 'pageview' AND %s = $%d\n", col("type"), col("url"), argN)
	}
}

var _ FunnelRepository = (*pgFunnelRepository)(nil)
