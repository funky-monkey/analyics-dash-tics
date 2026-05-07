package handler

import (
	"html/template"
	"log/slog"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/sidneydekoning/analytics/internal/middleware"
	"github.com/sidneydekoning/analytics/internal/model"
	"github.com/sidneydekoning/analytics/internal/repository"
	"github.com/sidneydekoning/analytics/internal/service"
)

// csrfToken extracts the CSRF token from the cookie. Returns empty string if absent.
func csrfToken(r *http.Request) string {
	if c, err := r.Cookie("csrf_token"); err == nil {
		return c.Value
	}
	return ""
}

// AdminHandler handles all /admin/* routes.
type AdminHandler struct {
	auth  service.AuthService
	repos *repository.Repos
	tmpls map[string]*template.Template
}

// NewAdminHandler constructs an AdminHandler. repos may be nil in tests.
func NewAdminHandler(auth service.AuthService, repos *repository.Repos) *AdminHandler {
	return &AdminHandler{auth: auth, repos: repos}
}

// SetTemplates wires the template map.
func (h *AdminHandler) SetTemplates(tmpls map[string]*template.Template) {
	h.tmpls = tmpls
}

// Index renders GET /admin.
func (h *AdminHandler) Index(w http.ResponseWriter, r *http.Request) {
	data := map[string]any{
		"ActiveNav": "overview",
		"UserCount": int64(0), "SiteCount": int64(0), "EventsToday": int64(0),
	}
	if h.repos != nil {
		data["UserCount"], _ = h.repos.Admin.CountUsers(r.Context())
		data["SiteCount"], _ = h.repos.Admin.CountSites(r.Context())
		data["EventsToday"], _ = h.repos.Admin.CountEventsToday(r.Context())
	}
	h.renderAdmin(w, "index.html", data)
}

// Users renders GET /admin/users.
func (h *AdminHandler) Users(w http.ResponseWriter, r *http.Request) {
	data := map[string]any{
		"ActiveNav":     "users",
		"Users":         []*model.User{},
		"CurrentUserID": middleware.UserIDFromContext(r.Context()),
		"CSRFToken":     csrfToken(r),
	}
	if h.repos != nil {
		users, err := h.repos.Admin.ListAllUsers(r.Context(), 100, 0)
		if err != nil {
			slog.Error("admin.Users", "error", err)
		} else {
			data["Users"] = users
		}
	}
	h.renderAdmin(w, "users.html", data)
}

// NewUserPage renders GET /admin/users/new.
func (h *AdminHandler) NewUserPage(w http.ResponseWriter, r *http.Request) {
	h.renderAdmin(w, "user-form.html", map[string]any{
		"ActiveNav": "users", "User": &model.User{}, "CSRFToken": csrfToken(r),
	})
}

// CreateUser handles POST /admin/users/new.
func (h *AdminHandler) CreateUser(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	name := strings.TrimSpace(r.FormValue("name"))
	email := strings.TrimSpace(r.FormValue("email"))
	password := r.FormValue("password")
	role := r.FormValue("role")
	if role != "admin" && role != "user" {
		role = "user"
	}

	if name == "" || email == "" || len(password) < 12 {
		h.renderAdmin(w, "user-form.html", map[string]any{
			"ActiveNav": "users",
			"User":      &model.User{Email: email, Name: name, Role: model.Role(role)},
			"CSRFToken": csrfToken(r),
			"Error":     "Name, email, and password (min 12 chars) are required.",
		})
		return
	}

	hash, err := h.auth.HashPassword(password)
	if err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	user := &model.User{
		Email: email, PasswordHash: hash,
		Role: model.Role(role), Name: name, IsActive: true,
	}
	if err := h.repos.Users.Create(r.Context(), user); err != nil {
		h.renderAdmin(w, "user-form.html", map[string]any{
			"ActiveNav": "users", "User": user, "CSRFToken": csrfToken(r),
			"Error": "Could not create user. Email may already exist.",
		})
		return
	}
	actorID := middleware.UserIDFromContext(r.Context())
	auditLog(h.repos, r, actorID, "create_user", "user", user.ID)
	http.Redirect(w, r, "/admin/users", http.StatusSeeOther)
}

// EditUserPage renders GET /admin/users/:id.
func (h *AdminHandler) EditUserPage(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	user, err := h.repos.Users.GetByID(r.Context(), id)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	h.renderAdmin(w, "user-form.html", map[string]any{
		"ActiveNav": "users", "User": user, "CSRFToken": csrfToken(r),
	})
}

// UpdateUser handles POST /admin/users/:id.
func (h *AdminHandler) UpdateUser(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	actorID := middleware.UserIDFromContext(r.Context())

	// Prevent admins from deactivating themselves or other admins.
	target, err := h.repos.Users.GetByID(r.Context(), id)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if target.Role == model.RoleAdmin {
		h.renderAdmin(w, "user-form.html", map[string]any{
			"ActiveNav": "users", "User": target, "CSRFToken": csrfToken(r),
			"Error": "Admin accounts cannot be deactivated. Remove the admin role first.",
		})
		return
	}
	if id == actorID {
		h.renderAdmin(w, "user-form.html", map[string]any{
			"ActiveNav": "users", "User": target, "CSRFToken": csrfToken(r),
			"Error": "You cannot deactivate your own account.",
		})
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	active := r.FormValue("is_active") == "true"
	if err := h.repos.Users.SetActive(r.Context(), id, active); err != nil {
		slog.Error("admin.UpdateUser", "error", err)
	}
	auditLog(h.repos, r, actorID, "update_user", "user", id)
	http.Redirect(w, r, "/admin/users", http.StatusSeeOther)
}

// DeleteUser handles POST /admin/users/:id/delete — soft-deletes a non-admin user.
func (h *AdminHandler) DeleteUser(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	actorID := middleware.UserIDFromContext(r.Context())

	if id == actorID {
		http.Error(w, "cannot delete your own account", http.StatusForbidden)
		return
	}
	target, err := h.repos.Users.GetByID(r.Context(), id)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if target.Role == model.RoleAdmin {
		http.Error(w, "cannot delete admin accounts", http.StatusForbidden)
		return
	}
	if err := h.repos.Users.SetActive(r.Context(), id, false); err != nil {
		slog.Error("admin.DeleteUser", "error", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	auditLog(h.repos, r, actorID, "delete_user", "user", id)
	http.Redirect(w, r, "/admin/users", http.StatusSeeOther)
}

// DeleteSite handles POST /admin/sites/:id/delete.
func (h *AdminHandler) DeleteSite(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	actorID := middleware.UserIDFromContext(r.Context())
	if err := h.repos.Sites.Delete(r.Context(), id); err != nil {
		slog.Error("admin.DeleteSite", "error", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	auditLog(h.repos, r, actorID, "delete_site", "site", id)
	http.Redirect(w, r, "/admin/sites", http.StatusSeeOther)
}

// Sites renders GET /admin/sites.
func (h *AdminHandler) Sites(w http.ResponseWriter, r *http.Request) {
	data := map[string]any{"ActiveNav": "sites", "Sites": []*model.Site{}, "CSRFToken": csrfToken(r)}
	if h.repos != nil {
		sites, err := h.repos.Admin.ListAllSites(r.Context(), 100, 0)
		if err != nil {
			slog.Error("admin.Sites", "error", err)
		} else {
			data["Sites"] = sites
		}
	}
	h.renderAdmin(w, "sites.html", data)
}

// AuditLog renders GET /admin/audit-log.
func (h *AdminHandler) AuditLog(w http.ResponseWriter, r *http.Request) {
	data := map[string]any{"ActiveNav": "audit", "Entries": []*repository.AuditEntry{}}
	if h.repos != nil {
		entries, err := h.repos.Admin.ListAuditLog(r.Context(), 100, 0)
		if err != nil {
			slog.Error("admin.AuditLog", "error", err)
		} else {
			data["Entries"] = entries
		}
	}
	h.renderAdmin(w, "audit.html", data)
}

// auditLog writes an audit entry, logging any error rather than silently discarding it.
func auditLog(repos *repository.Repos, r *http.Request, actorID, action, resourceType, resourceID string) {
	if repos == nil {
		return
	}
	if err := repos.Admin.WriteAuditLog(r.Context(), actorID, action, resourceType, resourceID, ""); err != nil {
		slog.Error("audit log write failed", "action", action, "error", err)
	}
}

func (h *AdminHandler) renderAdmin(w http.ResponseWriter, name string, data any) {
	if h.tmpls == nil {
		w.Header().Set("Content-Type", "text/html")
		return
	}
	t, ok := h.tmpls[name]
	if !ok {
		slog.Error("admin template not found", "name", name)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := t.ExecuteTemplate(w, "admin.html", data); err != nil {
		slog.Error("render admin template", "name", name, "error", err)
	}
}
