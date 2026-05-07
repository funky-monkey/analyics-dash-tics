package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/funky-monkey/analyics-dash-tics/internal/model"
)

// AdminRepository handles admin-only database operations.
type AdminRepository interface {
	ListAllUsers(ctx context.Context, limit, offset int) ([]*model.User, error)
	CountUsers(ctx context.Context) (int64, error)
	CountSites(ctx context.Context) (int64, error)
	CountEventsToday(ctx context.Context) (int64, error)
	ListAllSites(ctx context.Context, limit, offset int) ([]*model.Site, error)
	WriteAuditLog(ctx context.Context, actorID, action, resourceType, resourceID, ipHash string) error
	ListAuditLog(ctx context.Context, limit, offset int) ([]*AuditEntry, error)
}

// AuditEntry is a single row from the audit_log table.
type AuditEntry struct {
	ID           string
	ActorID      string
	Action       string
	ResourceType string
	ResourceID   string
	IPHash       string
	CreatedAt    time.Time
}

type pgAdminRepository struct {
	pool *pgxpool.Pool
}

func (r *pgAdminRepository) ListAllUsers(ctx context.Context, limit, offset int) ([]*model.User, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, email, role, name, is_active, created_at, last_login_at
		 FROM users ORDER BY created_at DESC LIMIT $1 OFFSET $2`, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("adminRepository.ListAllUsers: %w", err)
	}
	defer rows.Close()
	var users []*model.User
	for rows.Next() {
		u := &model.User{}
		if err := rows.Scan(&u.ID, &u.Email, &u.Role, &u.Name, &u.IsActive, &u.CreatedAt, &u.LastLoginAt); err != nil {
			return nil, fmt.Errorf("adminRepository.ListAllUsers: scan: %w", err)
		}
		users = append(users, u)
	}
	return users, rows.Err()
}

func (r *pgAdminRepository) CountUsers(ctx context.Context) (int64, error) {
	var n int64
	return n, r.pool.QueryRow(ctx, `SELECT COUNT(*) FROM users`).Scan(&n)
}

func (r *pgAdminRepository) CountSites(ctx context.Context) (int64, error) {
	var n int64
	return n, r.pool.QueryRow(ctx, `SELECT COUNT(*) FROM sites`).Scan(&n)
}

func (r *pgAdminRepository) CountEventsToday(ctx context.Context) (int64, error) {
	var n int64
	return n, r.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM events WHERE timestamp >= CURRENT_DATE`).Scan(&n)
}

func (r *pgAdminRepository) ListAllSites(ctx context.Context, limit, offset int) ([]*model.Site, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, owner_id, name, domain, token, timezone, created_at
		 FROM sites ORDER BY created_at DESC LIMIT $1 OFFSET $2`, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("adminRepository.ListAllSites: %w", err)
	}
	defer rows.Close()
	var sites []*model.Site
	for rows.Next() {
		s := &model.Site{}
		if err := rows.Scan(&s.ID, &s.OwnerID, &s.Name, &s.Domain, &s.Token, &s.Timezone, &s.CreatedAt); err != nil {
			return nil, fmt.Errorf("adminRepository.ListAllSites: scan: %w", err)
		}
		sites = append(sites, s)
	}
	return sites, rows.Err()
}

func (r *pgAdminRepository) WriteAuditLog(ctx context.Context, actorID, action, resourceType, resourceID, ipHash string) error {
	// actor_id is UUID — pass nil when actorID is empty to avoid a type-cast error.
	var actor interface{}
	if actorID != "" {
		actor = actorID
	}
	_, err := r.pool.Exec(ctx,
		`INSERT INTO audit_log (actor_id, action, resource_type, resource_id, ip_hash)
		 VALUES ($1, $2, $3, $4, $5)`,
		actor, action, resourceType, resourceID, ipHash)
	if err != nil {
		return fmt.Errorf("adminRepository.WriteAuditLog: %w", err)
	}
	return nil
}

func (r *pgAdminRepository) ListAuditLog(ctx context.Context, limit, offset int) ([]*AuditEntry, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, COALESCE(actor_id::text,''), action, resource_type, resource_id, ip_hash, created_at
		 FROM audit_log ORDER BY created_at DESC LIMIT $1 OFFSET $2`, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("adminRepository.ListAuditLog: %w", err)
	}
	defer rows.Close()
	var entries []*AuditEntry
	for rows.Next() {
		e := &AuditEntry{}
		if err := rows.Scan(&e.ID, &e.ActorID, &e.Action, &e.ResourceType, &e.ResourceID, &e.IPHash, &e.CreatedAt); err != nil {
			return nil, fmt.Errorf("adminRepository.ListAuditLog: scan: %w", err)
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}
