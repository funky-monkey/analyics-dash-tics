package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/sidneydekoning/analytics/internal/model"
)

// CMSRepository handles CRUD for CMS layouts and pages.
type CMSRepository interface {
	ListLayouts(ctx context.Context) ([]*model.CMSLayout, error)
	GetLayout(ctx context.Context, id string) (*model.CMSLayout, error)
	CreatePage(ctx context.Context, p *model.CMSPage) error
	UpdatePage(ctx context.Context, p *model.CMSPage) error
	GetPageByID(ctx context.Context, id string) (*model.CMSPage, error)
	GetPageBySlug(ctx context.Context, slug string) (*model.CMSPage, error)
	ListPages(ctx context.Context, limit, offset int) ([]*model.CMSPage, error)
	ListPublishedByType(ctx context.Context, pageType string, limit, offset int) ([]*model.CMSPage, error)
	SetPageStatus(ctx context.Context, id, status string, publishedAt *time.Time) error
	DeletePage(ctx context.Context, id string) error
}

type pgCMSRepository struct {
	pool *pgxpool.Pool
}

func (r *pgCMSRepository) ListLayouts(ctx context.Context) ([]*model.CMSLayout, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, name, template_file, description FROM cms_layouts ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("cmsRepository.ListLayouts: %w", err)
	}
	defer rows.Close()
	var layouts []*model.CMSLayout
	for rows.Next() {
		l := &model.CMSLayout{}
		if err := rows.Scan(&l.ID, &l.Name, &l.TemplateFile, &l.Description); err != nil {
			return nil, fmt.Errorf("cmsRepository.ListLayouts: scan: %w", err)
		}
		layouts = append(layouts, l)
	}
	return layouts, rows.Err()
}

func (r *pgCMSRepository) GetLayout(ctx context.Context, id string) (*model.CMSLayout, error) {
	l := &model.CMSLayout{}
	err := r.pool.QueryRow(ctx,
		`SELECT id, name, template_file, description FROM cms_layouts WHERE id = $1`, id).
		Scan(&l.ID, &l.Name, &l.TemplateFile, &l.Description)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("cmsRepository.GetLayout: %w", err)
	}
	return l, nil
}

func (r *pgCMSRepository) CreatePage(ctx context.Context, p *model.CMSPage) error {
	err := r.pool.QueryRow(ctx, `
		INSERT INTO cms_pages
			(layout_id, author_id, title, slug, type, content_html, excerpt,
			 cover_image_url, meta_title, meta_description, status)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
		RETURNING id, created_at, updated_at
	`, p.LayoutID, p.AuthorID, p.Title, p.Slug, p.Type, p.ContentHTML, p.Excerpt,
		p.CoverImageURL, p.MetaTitle, p.MetaDescription, p.Status).
		Scan(&p.ID, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		return fmt.Errorf("cmsRepository.CreatePage: %w", err)
	}
	return nil
}

func (r *pgCMSRepository) UpdatePage(ctx context.Context, p *model.CMSPage) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE cms_pages SET
			title=$2, slug=$3, content_html=$4, excerpt=$5,
			cover_image_url=$6, meta_title=$7, meta_description=$8,
			updated_at=NOW()
		WHERE id=$1
	`, p.ID, p.Title, p.Slug, p.ContentHTML, p.Excerpt,
		p.CoverImageURL, p.MetaTitle, p.MetaDescription)
	if err != nil {
		return fmt.Errorf("cmsRepository.UpdatePage: %w", err)
	}
	return nil
}

func (r *pgCMSRepository) GetPageByID(ctx context.Context, id string) (*model.CMSPage, error) {
	return r.scanOnePage(r.pool.QueryRow(ctx, `
		SELECT id,layout_id,author_id,title,slug,type,content_html,excerpt,
		       cover_image_url,meta_title,meta_description,status,published_at,created_at,updated_at
		FROM cms_pages WHERE id=$1`, id))
}

func (r *pgCMSRepository) GetPageBySlug(ctx context.Context, slug string) (*model.CMSPage, error) {
	// A page is publicly visible when:
	//   status='published'  — published immediately (published_at may be null or in the past)
	//   OR published_at is set and <= NOW() — scheduled post whose time has arrived
	return r.scanOnePage(r.pool.QueryRow(ctx, `
		SELECT id,layout_id,author_id,title,slug,type,content_html,excerpt,
		       cover_image_url,meta_title,meta_description,status,published_at,created_at,updated_at
		FROM cms_pages
		WHERE slug=$1
		  AND (status='published' OR (published_at IS NOT NULL AND published_at <= NOW()))`, slug))
}

func (r *pgCMSRepository) scanOnePage(row pgx.Row) (*model.CMSPage, error) {
	p := &model.CMSPage{}
	err := row.Scan(
		&p.ID, &p.LayoutID, &p.AuthorID, &p.Title, &p.Slug, &p.Type,
		&p.ContentHTML, &p.Excerpt, &p.CoverImageURL, &p.MetaTitle,
		&p.MetaDescription, &p.Status, &p.PublishedAt, &p.CreatedAt, &p.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("cmsRepository.scanOnePage: %w", err)
	}
	return p, nil
}

func (r *pgCMSRepository) ListPages(ctx context.Context, limit, offset int) ([]*model.CMSPage, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id,layout_id,author_id,title,slug,type,content_html,excerpt,
		       cover_image_url,meta_title,meta_description,status,published_at,created_at,updated_at
		FROM cms_pages ORDER BY created_at DESC LIMIT $1 OFFSET $2`, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("cmsRepository.ListPages: %w", err)
	}
	defer rows.Close()
	return r.scanManyPages(rows)
}

func (r *pgCMSRepository) ListPublishedByType(ctx context.Context, pageType string, limit, offset int) ([]*model.CMSPage, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id,layout_id,author_id,title,slug,type,content_html,excerpt,
		       cover_image_url,meta_title,meta_description,status,published_at,created_at,updated_at
		FROM cms_pages
		WHERE type=$1
		  AND (status='published' OR (published_at IS NOT NULL AND published_at <= NOW()))
		ORDER BY COALESCE(published_at, created_at) DESC LIMIT $2 OFFSET $3`, pageType, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("cmsRepository.ListPublishedByType: %w", err)
	}
	defer rows.Close()
	return r.scanManyPages(rows)
}

func (r *pgCMSRepository) SetPageStatus(ctx context.Context, id, status string, publishedAt *time.Time) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE cms_pages SET status=$2, published_at=$3, updated_at=NOW() WHERE id=$1`,
		id, status, publishedAt)
	if err != nil {
		return fmt.Errorf("cmsRepository.SetPageStatus: %w", err)
	}
	return nil
}

func (r *pgCMSRepository) DeletePage(ctx context.Context, id string) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM cms_pages WHERE id=$1`, id)
	if err != nil {
		return fmt.Errorf("cmsRepository.DeletePage: %w", err)
	}
	return nil
}

func (r *pgCMSRepository) scanManyPages(rows pgx.Rows) ([]*model.CMSPage, error) {
	var pages []*model.CMSPage
	for rows.Next() {
		p := &model.CMSPage{}
		if err := rows.Scan(
			&p.ID, &p.LayoutID, &p.AuthorID, &p.Title, &p.Slug, &p.Type,
			&p.ContentHTML, &p.Excerpt, &p.CoverImageURL, &p.MetaTitle,
			&p.MetaDescription, &p.Status, &p.PublishedAt, &p.CreatedAt, &p.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("cmsRepository.scanManyPages: scan: %w", err)
		}
		pages = append(pages, p)
	}
	return pages, rows.Err()
}
