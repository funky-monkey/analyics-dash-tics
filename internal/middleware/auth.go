package middleware

import (
	"context"
	"net/http"
	"net/url"
	"strings"

	"github.com/funky-monkey/analyics-dash-tics/internal/service"
)

type contextKey string

const (
	// ContextKeyUserID is the context key for the authenticated user's ID.
	ContextKeyUserID contextKey = "user_id"
	// contextKeyNonce is the context key for the per-request CSP nonce.
	contextKeyNonce contextKey = "csp_nonce"
	// ContextKeyRole is the context key for the authenticated user's role.
	ContextKeyRole contextKey = "role"
)

// JWTAuth validates the access token from the auth cookie or Authorization header.
// On success, sets user_id and role in the request context.
// For browser requests (Accept: text/html or cookie-based), redirects to /login?expired=1&next=<url>.
// For API requests (Authorization header), returns 401.
func JWTAuth(authSvc service.AuthService) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			tokenString := tokenFromRequest(r)
			valid := tokenString != ""
			var claims *service.TokenClaims
			if valid {
				var err error
				claims, err = authSvc.ParseAccessToken(tokenString)
				if err != nil {
					valid = false
				}
			}
			if !valid {
				// API clients (Bearer token) get a 401 JSON response.
				if strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") {
					http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
					return
				}
				// Browser: redirect to login with session-expired notice + return URL.
				returnTo := r.URL.RequestURI()
				loginURL := "/login?expired=1&next=" + url.QueryEscape(returnTo)
				http.Redirect(w, r, loginURL, http.StatusSeeOther)
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

// WithUserID returns a new context with the given user ID set.
// Used in tests to inject authentication context.
func WithUserID(ctx context.Context, userID string) context.Context {
	return context.WithValue(ctx, ContextKeyUserID, userID)
}

// WithRole returns a new context with the given role set.
// Used in tests to inject authentication context.
func WithRole(ctx context.Context, role string) context.Context {
	return context.WithValue(ctx, ContextKeyRole, role)
}

// tokenFromRequest extracts the JWT from the HTTP-only auth cookie (browser)
// NonceFromContext returns the per-request CSP nonce set by SecurityHeaders.
func NonceFromContext(ctx context.Context) string {
	v, _ := ctx.Value(contextKeyNonce).(string)
	return v
}

func withNonce(ctx context.Context, nonce string) context.Context {
	return context.WithValue(ctx, contextKeyNonce, nonce)
}

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
