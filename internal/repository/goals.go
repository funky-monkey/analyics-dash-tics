package repository

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/funky-monkey/analyics-dash-tics/internal/model"
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
	err := r.pool.QueryRow(ctx,
		`INSERT INTO goals (site_id, name, type, value) VALUES ($1,$2,$3,$4) RETURNING id`,
		g.SiteID, g.Name, g.Type, g.Value).Scan(&g.ID)
	if err != nil {
		return fmt.Errorf("goalRepository.Create: %w", err)
	}
	return nil
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

var _ GoalRepository = (*pgGoalRepository)(nil)
