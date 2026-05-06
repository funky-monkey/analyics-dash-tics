package middleware

import (
	"crypto/rand"
	"encoding/base64"
	"net/http"
	"strings"
)

// SecurityHeaders sets all required security response headers on every response.
// Must be the outermost middleware so headers are present even on error responses.
func SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Strict-Transport-Security", "max-age=63072000; includeSubDomains; preload")
		h.Set("Content-Security-Policy",
			"default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; connect-src 'self'; frame-ancestors 'none'")
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("X-XSS-Protection", "0")
		h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		h.Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		next.ServeHTTP(w, r)
	})
}

// CORS returns middleware that handles Cross-Origin Resource Sharing.
// collectPaths lists paths (e.g. "/collect") that accept requests from any origin.
// allowedOrigins is the list of allowed origins for all other routes.
func CORS(allowedOrigins []string, collectPaths ...string) func(http.Handler) http.Handler {
	collectSet := make(map[string]bool, len(collectPaths))
	for _, p := range collectPaths {
		collectSet[p] = true
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")

			if collectSet[r.URL.Path] {
				// /collect accepts any origin — tracking script runs on customer sites.
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Access-Control-Allow-Methods", "POST")
				w.Header().Set("Access-Control-Max-Age", "86400")
				if r.Method == http.MethodOptions {
					w.WriteHeader(http.StatusNoContent)
					return
				}
				next.ServeHTTP(w, r)
				return
			}

			// All other routes: only allowed origins (never wildcard on authenticated endpoints).
			for _, allowed := range allowedOrigins {
				if origin == allowed {
					w.Header().Set("Access-Control-Allow-Origin", origin)
					w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
					w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, X-CSRF-Token")
					w.Header().Set("Access-Control-Allow-Credentials", "true")
					break
				}
			}

			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// CSRF implements the double-submit cookie pattern.
// Sets a non-HTTP-only cookie with a random token.
// For state-changing requests (POST/PUT/PATCH/DELETE), validates the token from
// X-CSRF-Token header or _csrf form field against the cookie value.
// The /collect endpoint is exempt — it uses site token auth, not cookies.
func CSRF(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// /collect is exempt from CSRF — it authenticates via site token.
		if r.URL.Path == "/collect" {
			next.ServeHTTP(w, r)
			return
		}

		token := csrfTokenFromRequest(r)
		if token == "" {
			b := make([]byte, 32)
			if _, err := rand.Read(b); err != nil {
				http.Error(w, "internal error", http.StatusInternalServerError)
				return
			}
			token = base64.URLEncoding.EncodeToString(b)
			http.SetCookie(w, &http.Cookie{
				Name:     "csrf_token",
				Value:    token,
				Path:     "/",
				HttpOnly: false, // must be JS-readable for HTMX to submit in header
				Secure:   true,
				SameSite: http.SameSiteStrictMode,
			})
		}

		if isStateMutating(r.Method) {
			submitted := r.Header.Get("X-CSRF-Token")
			if submitted == "" {
				submitted = r.FormValue("_csrf")
			}
			if submitted == "" || !strings.EqualFold(submitted, token) {
				http.Error(w, "invalid CSRF token", http.StatusForbidden)
				return
			}
		}

		next.ServeHTTP(w, r)
	})
}

func csrfTokenFromRequest(r *http.Request) string {
	if c, err := r.Cookie("csrf_token"); err == nil {
		return c.Value
	}
	return ""
}

func isStateMutating(method string) bool {
	switch method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	}
	return false
}
