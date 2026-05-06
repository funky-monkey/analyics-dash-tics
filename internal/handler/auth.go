package handler

import (
	"html/template"
	"log/slog"
	"net/http"
	"time"

	"github.com/sidneydekoning/analytics/internal/middleware"
	"github.com/sidneydekoning/analytics/internal/model"
	"github.com/sidneydekoning/analytics/internal/repository"
	"github.com/sidneydekoning/analytics/internal/service"
)

// AuthHandler handles all authentication routes.
type AuthHandler struct {
	auth    service.AuthService
	repos   *repository.Repos
	baseURL string
	tmpls   map[string]*template.Template
}

// NewAuthHandler constructs an AuthHandler. repos may be nil in tests that only test
// routes which do not hit the database (e.g. validation-only paths).
func NewAuthHandler(auth service.AuthService, repos *repository.Repos, baseURL string) *AuthHandler {
	return &AuthHandler{auth: auth, repos: repos, baseURL: baseURL}
}

// SetTemplates wires the template map. Each key is a page name (e.g. "login.html"),
// each value is a self-contained template set (base + page) so defines don't bleed across pages.
func (h *AuthHandler) SetTemplates(tmpls map[string]*template.Template) {
	h.tmpls = tmpls
}

type authPageData struct {
	Error     string
	CSRFToken string
}

// LoginPage renders GET /login.
func (h *AuthHandler) LoginPage(w http.ResponseWriter, r *http.Request) {
	h.renderAuth(w, r, "login.html", authPageData{})
}

// Login handles POST /login.
func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	email := r.FormValue("email")
	password := r.FormValue("password")

	if email == "" || password == "" {
		w.WriteHeader(http.StatusUnprocessableEntity)
		h.renderAuth(w, r, "login.html", authPageData{Error: "Email and password are required."})
		return
	}

	user, err := h.repos.Users.GetByEmail(r.Context(), email)
	if err != nil || !h.auth.CheckPassword(password, user.PasswordHash) || !user.IsActive {
		// Same error for wrong email, wrong password, and inactive — prevents user enumeration.
		w.WriteHeader(http.StatusUnauthorized)
		h.renderAuth(w, r, "login.html", authPageData{Error: "Invalid email or password."})
		return
	}

	h.issueTokensAndRedirect(w, r, user.ID, string(user.Role), "/dashboard")

	if err := h.repos.Users.UpdateLastLogin(r.Context(), user.ID); err != nil {
		slog.Error("update last login", "error", err, "user_id", user.ID)
	}
}

// SignupPage renders GET /signup.
func (h *AuthHandler) SignupPage(w http.ResponseWriter, r *http.Request) {
	h.renderAuth(w, r, "signup.html", authPageData{})
}

// Signup handles POST /signup.
func (h *AuthHandler) Signup(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	name := r.FormValue("name")
	email := r.FormValue("email")
	password := r.FormValue("password")

	if name == "" || email == "" {
		w.WriteHeader(http.StatusUnprocessableEntity)
		h.renderAuth(w, r, "signup.html", authPageData{Error: "Name and email are required."})
		return
	}
	if len(password) < 12 {
		w.WriteHeader(http.StatusUnprocessableEntity)
		h.renderAuth(w, r, "signup.html", authPageData{Error: "Password must be at least 12 characters."})
		return
	}

	hash, err := h.auth.HashPassword(password)
	if err != nil {
		slog.Error("hash password", "error", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	user := &model.User{
		Email:        email,
		PasswordHash: hash,
		Role:         model.RoleUser,
		Name:         name,
		IsActive:     true,
	}
	if err := h.repos.Users.Create(r.Context(), user); err != nil {
		w.WriteHeader(http.StatusUnprocessableEntity)
		h.renderAuth(w, r, "signup.html", authPageData{Error: "An account with this email already exists."})
		return
	}

	h.issueTokensAndRedirect(w, r, user.ID, string(user.Role), "/dashboard")
}

// Logout handles POST /logout — clears auth cookies.
func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	clearCookie(w, "access_token")
	clearCookie(w, "refresh_token")
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

type forgotPasswordData struct {
	Error     string
	CSRFToken string
	Success   bool
}

// ForgotPasswordPage renders GET /forgot-password.
func (h *AuthHandler) ForgotPasswordPage(w http.ResponseWriter, r *http.Request) {
	data := forgotPasswordData{}
	if c, err := r.Cookie("csrf_token"); err == nil {
		data.CSRFToken = c.Value
	}
	h.renderTemplate(w, "forgot-password.html", data)
}

// ForgotPassword handles POST /forgot-password.
// Always shows the success message — never reveals whether an email exists (prevents enumeration).
func (h *AuthHandler) ForgotPassword(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	email := r.FormValue("email")
	data := forgotPasswordData{Success: true}
	if c, err := r.Cookie("csrf_token"); err == nil {
		data.CSRFToken = c.Value
	}

	if email != "" && h.repos != nil {
		user, err := h.repos.Users.GetByEmail(r.Context(), email)
		if err == nil && user.IsActive {
			token, err := h.auth.GenerateSecureToken()
			if err != nil {
				slog.Error("generate reset token", "error", err)
			} else {
				// TODO: store token hash in DB and send email (Plan 4 — email service)
				slog.Info("password reset token (dev — replace with email send)", "user_id", user.ID, "token_prefix", token[:8])
			}
		}
	}

	h.renderTemplate(w, "forgot-password.html", data)
}

type resetPasswordData struct {
	Token     string
	Error     string
	CSRFToken string
}

// ResetPasswordPage renders GET /reset-password/{token}.
func (h *AuthHandler) ResetPasswordPage(w http.ResponseWriter, r *http.Request) {
	token := r.PathValue("token")
	data := resetPasswordData{Token: token}
	if c, err := r.Cookie("csrf_token"); err == nil {
		data.CSRFToken = c.Value
	}
	h.renderTemplate(w, "reset-password.html", data)
}

// ResetPassword handles POST /reset-password/{token}.
func (h *AuthHandler) ResetPassword(w http.ResponseWriter, r *http.Request) {
	token := r.PathValue("token")
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	password := r.FormValue("password")
	data := resetPasswordData{Token: token}
	if c, err := r.Cookie("csrf_token"); err == nil {
		data.CSRFToken = c.Value
	}

	if len(password) < 12 {
		w.WriteHeader(http.StatusUnprocessableEntity)
		data.Error = "Password must be at least 12 characters."
		h.renderTemplate(w, "reset-password.html", data)
		return
	}
	// TODO: validate token hash from DB, update password, mark token used (Plan 4)
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

func (h *AuthHandler) issueTokensAndRedirect(w http.ResponseWriter, r *http.Request, userID, role, dest string) {
	access, err := h.auth.IssueAccessToken(userID, role)
	if err != nil {
		slog.Error("issue access token", "error", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	refresh, err := h.auth.IssueRefreshToken(userID)
	if err != nil {
		slog.Error("issue refresh token", "error", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	setAuthCookie(w, "access_token", access.TokenString, 15*time.Minute)
	setAuthCookie(w, "refresh_token", refresh.TokenString, 7*24*time.Hour)
	http.Redirect(w, r, dest, http.StatusSeeOther)
}

func (h *AuthHandler) renderAuth(w http.ResponseWriter, r *http.Request, name string, data authPageData) {
	if data.CSRFToken == "" {
		if c, err := r.Cookie("csrf_token"); err == nil {
			data.CSRFToken = c.Value
		}
	}
	h.renderTemplate(w, name, data)
}

// renderTemplate executes the named page template. Each page has its own isolated
// template set (base + page) to prevent define blocks bleeding across pages.
func (h *AuthHandler) renderTemplate(w http.ResponseWriter, name string, data any) {
	if h.tmpls == nil {
		w.Header().Set("Content-Type", "text/html")
		return
	}
	t, ok := h.tmpls[name]
	if !ok {
		slog.Error("template not found", "name", name)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := t.ExecuteTemplate(w, "base.html", data); err != nil {
		slog.Error("render template", "name", name, "error", err)
	}
}

func setAuthCookie(w http.ResponseWriter, name, value string, maxAge time.Duration) {
	http.SetCookie(w, &http.Cookie{
		Name:     name,
		Value:    value,
		Path:     "/",
		MaxAge:   int(maxAge.Seconds()),
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteStrictMode,
	})
}

func clearCookie(w http.ResponseWriter, name string) {
	http.SetCookie(w, &http.Cookie{
		Name:     name,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteStrictMode,
	})
}

// UserIDFromRequest is a convenience wrapper around middleware.UserIDFromContext.
func UserIDFromRequest(r *http.Request) string {
	return middleware.UserIDFromContext(r.Context())
}
