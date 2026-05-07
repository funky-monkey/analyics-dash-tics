package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/funky-monkey/analyics-dash-tics/internal/model"
)

// SiteRepository defines all database operations for sites.
type SiteRepository interface {
	Create(ctx context.Context, s *model.Site) error
	GetByID(ctx context.Context, id string) (*model.Site, error)
	GetByToken(ctx context.Context, token string) (*model.Site, error)
	// GetBySlug looks up a site by its domain slug (dots replaced with dashes),
	// e.g. "acme-io" matches domain "acme.io".
	GetBySlug(ctx context.Context, slug string) (*model.Site, error)
	ListByOwner(ctx context.Context, ownerID string) ([]*model.Site, error)
	Delete(ctx context.Context, id string) error
	Update(ctx context.Context, s *model.Site) error
}

type pgSiteRepository struct {
	pool *pgxpool.Pool
}

func (r *pgSiteRepository) Create(ctx context.Context, s *model.Site) error {
	err := r.pool.QueryRow(ctx, `
		INSERT INTO sites (owner_id, name, domain, token, timezone)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, created_at
	`, s.OwnerID, s.Name, s.Domain, s.Token, s.Timezone).
		Scan(&s.ID, &s.CreatedAt)
	if err != nil {
		return fmt.Errorf("siteRepository.Create: %w", err)
	}
	return nil
}

func (r *pgSiteRepository) GetByID(ctx context.Context, id string) (*model.Site, error) {
	s := &model.Site{}
	err := r.pool.QueryRow(ctx, `
		SELECT id, owner_id, name, domain, token, timezone, created_at
		FROM sites WHERE id = $1
	`, id).Scan(&s.ID, &s.OwnerID, &s.Name, &s.Domain, &s.Token, &s.Timezone, &s.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("siteRepository.GetByID: %w", err)
	}
	return s, nil
}

func (r *pgSiteRepository) GetByToken(ctx context.Context, token string) (*model.Site, error) {
	s := &model.Site{}
	err := r.pool.QueryRow(ctx, `
		SELECT id, owner_id, name, domain, token, timezone, created_at
		FROM sites WHERE token = $1
	`, token).Scan(&s.ID, &s.OwnerID, &s.Name, &s.Domain, &s.Token, &s.Timezone, &s.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("siteRepository.GetByToken: %w", err)
	}
	return s, nil
}

func (r *pgSiteRepository) GetBySlug(ctx context.Context, slug string) (*model.Site, error) {
	s := &model.Site{}
	err := r.pool.QueryRow(ctx, `
		SELECT id, owner_id, name, domain, token, timezone, created_at
		FROM sites WHERE replace(domain, '.', '-') = $1
	`, slug).Scan(&s.ID, &s.OwnerID, &s.Name, &s.Domain, &s.Token, &s.Timezone, &s.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("siteRepository.GetBySlug: %w", err)
	}
	return s, nil
}

func (r *pgSiteRepository) ListByOwner(ctx context.Context, ownerID string) ([]*model.Site, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, owner_id, name, domain, token, timezone, created_at
		FROM sites WHERE owner_id = $1 ORDER BY created_at DESC
	`, ownerID)
	if err != nil {
		return nil, fmt.Errorf("siteRepository.ListByOwner: %w", err)
	}
	defer rows.Close()

	var sites []*model.Site
	for rows.Next() {
		s := &model.Site{}
		if err := rows.Scan(&s.ID, &s.OwnerID, &s.Name, &s.Domain, &s.Token, &s.Timezone, &s.CreatedAt); err != nil {
			return nil, fmt.Errorf("siteRepository.ListByOwner: scan: %w", err)
		}
		sites = append(sites, s)
	}
	return sites, rows.Err()
}

func (r *pgSiteRepository) Delete(ctx context.Context, id string) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM sites WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("siteRepository.Delete: %w", err)
	}
	return nil
}

func (r *pgSiteRepository) Update(ctx context.Context, s *model.Site) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE sites SET name=$2, timezone=$3 WHERE id=$1`,
		s.ID, s.Name, s.Timezone)
	if err != nil {
		return fmt.Errorf("siteRepository.Update: %w", err)
	}
	return nil
}
