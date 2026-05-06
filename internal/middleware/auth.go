package middleware

import (
	"context"
	"net/http"
	"strings"

	"github.com/sidneydekoning/analytics/internal/service"
)

type contextKey string

const (
	// ContextKeyUserID is the context key for the authenticated user's ID.
	ContextKeyUserID contextKey = "user_id"
	// ContextKeyRole is the context key for the authenticated user's role.
	ContextKeyRole contextKey = "role"
)

// JWTAuth validates the access token from the auth cookie or Authorization header.
// On success, sets user_id and role in the request context.
// Returns 401 if no valid token is present.
func JWTAuth(authSvc service.AuthService) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			tokenString := tokenFromRequest(r)
			if tokenString == "" {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			claims, err := authSvc.ParseAccessToken(tokenString)
			if err != nil {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			ctx := context.WithValue(r.Context(), ContextKeyUserID, claims.UserID)
			ctx = context.WithValue(ctx, ContextKeyRole, claims.Role)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// RequireRole returns 404 if the authenticated user's role does not match.
// Returns 404 (not 403) to avoid leaking that the route exists to unauthorised callers.
func RequireRole(role string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			userRole, _ := r.Context().Value(ContextKeyRole).(string)
			if !strings.EqualFold(userRole, role) {
				http.NotFound(w, r)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// UserIDFromContext extracts the authenticated user ID from the request context.
func UserIDFromContext(ctx context.Context) string {
	id, _ := ctx.Value(ContextKeyUserID).(string)
	return id
}

// RoleFromContext extracts the role from the request context.
func RoleFromContext(ctx context.Context) string {
	role, _ := ctx.Value(ContextKeyRole).(string)
	return role
}

// tokenFromRequest extracts the JWT from the HTTP-only auth cookie (browser)
// or the Authorization: Bearer header (Stats API).
func tokenFromRequest(r *http.Request) string {
	if c, err := r.Cookie("access_token"); err == nil {
		return c.Value
	}
	if auth := r.Header.Get("Authorization"); strings.HasPrefix(auth, "Bearer ") {
		return strings.TrimPrefix(auth, "Bearer ")
	}
	return ""
}
