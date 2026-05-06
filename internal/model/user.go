package model

import "time"

// Role is the system-level access role for a user.
type Role string

const (
	RoleAdmin Role = "admin"
	RoleUser  Role = "user"
)

// MemberRole is the per-site access role for a site member.
type MemberRole string

const (
	MemberRoleOwner  MemberRole = "owner"
	MemberRoleEditor MemberRole = "editor"
	MemberRoleViewer MemberRole = "viewer"
)

// User represents a registered account.
type User struct {
	ID           string
	Email        string
	PasswordHash string
	Role         Role
	Name         string
	IsActive     bool
	CreatedAt    time.Time
	LastLoginAt  *time.Time
}

// SiteMember represents a user's access to a specific site.
type SiteMember struct {
	ID         string
	SiteID     string
	UserID     string
	Role       MemberRole
	InvitedAt  time.Time
	AcceptedAt *time.Time
}

// Invitation is a pending invite to a site sent to an email address.
type Invitation struct {
	ID        string
	SiteID    string
	Email     string
	Token     string
	Role      MemberRole
	ExpiresAt time.Time
}
