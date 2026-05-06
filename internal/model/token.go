package model

import "time"

// RevokedToken marks a JWT jti as revoked before its natural expiry.
type RevokedToken struct {
	JTI       string
	UserID    string
	ExpiresAt time.Time
}

// PasswordResetToken is a short-lived token sent via email for password resets.
type PasswordResetToken struct {
	ID        string
	UserID    string
	TokenHash string
	ExpiresAt time.Time
	UsedAt    *time.Time
}
