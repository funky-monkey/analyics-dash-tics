package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/funky-monkey/analyics-dash-tics/internal/model"
)

// ErrNotFound is returned when a requested record does not exist.
var ErrNotFound = errors.New("record not found")

// UserRepository defines all database operations for users.
type UserRepository interface {
	Create(ctx context.Context, u *model.User) error
	GetByID(ctx context.Context, id string) (*model.User, error)
	GetByEmail(ctx context.Context, email string) (*model.User, error)
	UpdateLastLogin(ctx context.Context, id string) error
	SetActive(ctx context.Context, id string, active bool) error
}

type pgUserRepository struct {
	pool *pgxpool.Pool
}

func (r *pgUserRepository) Create(ctx context.Context, u *model.User) error {
	err := r.pool.QueryRow(ctx, `
		INSERT INTO users (email, password_hash, role, name, is_active)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, created_at
	`, u.Email, u.PasswordHash, u.Role, u.Name, u.IsActive).
		Scan(&u.ID, &u.CreatedAt)
	if err != nil {
		return fmt.Errorf("userRepository.Create: %w", err)
	}
	return nil
}

func (r *pgUserRepository) GetByID(ctx context.Context, id string) (*model.User, error) {
	u := &model.User{}
	err := r.pool.QueryRow(ctx, `
		SELECT id, email, password_hash, role, name, is_active, created_at, last_login_at
		FROM users WHERE id = $1
	`, id).Scan(
		&u.ID, &u.Email, &u.PasswordHash, &u.Role,
		&u.Name, &u.IsActive, &u.CreatedAt, &u.LastLoginAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("userRepository.GetByID: %w", err)
	}
	return u, nil
}

func (r *pgUserRepository) GetByEmail(ctx context.Context, email string) (*model.User, error) {
	u := &model.User{}
	err := r.pool.QueryRow(ctx, `
		SELECT id, email, password_hash, role, name, is_active, created_at, last_login_at
		FROM users WHERE email = $1
	`, email).Scan(
		&u.ID, &u.Email, &u.PasswordHash, &u.Role,
		&u.Name, &u.IsActive, &u.CreatedAt, &u.LastLoginAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("userRepository.GetByEmail: %w", err)
	}
	return u, nil
}

func (r *pgUserRepository) UpdateLastLogin(ctx context.Context, id string) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE users SET last_login_at = NOW() WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("userRepository.UpdateLastLogin: %w", err)
	}
	return nil
}

func (r *pgUserRepository) SetActive(ctx context.Context, id string, active bool) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE users SET is_active = $2 WHERE id = $1`, id, active)
	if err != nil {
		return fmt.Errorf("userRepository.SetActive: %w", err)
	}
	return nil
}
