package handler

import (
	"html/template"
	"log/slog"
	"net/http"
	"strings"

	"github.com/sidneydekoning/analytics/internal/middleware"
	"github.com/sidneydekoning/analytics/internal/model"
	"github.com/sidneydekoning/analytics/internal/repository"
	"github.com/sidneydekoning/analytics/internal/service"
)

// SitesHandler handles site registration and management routes.
type SitesHandler struct {
	auth  service.AuthService
	repos *repository.Repos
	tmpl  *template.Template
}

// NewSitesHandler constructs a SitesHandler.
func NewSitesHandler(auth service.AuthService, repos *repository.Repos) *SitesHandler {
	return &SitesHandler{auth: auth, repos: repos}
}

// SetTemplates wires the parsed template set. Called once after templates are loaded.
func (h *SitesHandler) SetTemplates(tmpl *template.Template) {
	h.tmpl = tmpl
}

type newSiteData struct {
	Error     string
	CSRFToken string
}

// NewSitePage renders GET /account/sites/new.
func (h *SitesHandler) NewSitePage(w http.ResponseWriter, r *http.Request) {
	data := newSiteData{}
	if c, err := r.Cookie("csrf_token"); err == nil {
		data.CSRFToken = c.Value
	}
	h.renderTemplate(w, "new-site.html", data)
}

// CreateSite handles POST /account/sites/new.
// Strips protocol prefix from domain, generates a unique site token, and saves the site.
func (h *SitesHandler) CreateSite(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	name := strings.TrimSpace(r.FormValue("name"))
	domain := strings.TrimSpace(r.FormValue("domain"))
	timezone := r.FormValue("timezone")

	if name == "" || domain == "" {
		data := newSiteData{Error: "Name and domain are required."}
		if c, err := r.Cookie("csrf_token"); err == nil {
			data.CSRFToken = c.Value
		}
		w.WriteHeader(http.StatusUnprocessableEntity)
		h.renderTemplate(w, "new-site.html", data)
		return
	}

	// Normalise domain — strip protocol and trailing slash if user included them.
	domain = strings.TrimPrefix(domain, "https://")
	domain = strings.TrimPrefix(domain, "http://")
	domain = strings.TrimSuffix(domain, "/")

	if timezone == "" {
		timezone = "UTC"
	}

	token, err := h.auth.GenerateSiteToken()
	if err != nil {
		slog.Error("generate site token", "error", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	userID := middleware.UserIDFromContext(r.Context())
	site := &model.Site{
		OwnerID:  userID,
		Name:     name,
		Domain:   domain,
		Token:    token,
		Timezone: timezone,
	}

	if err := h.repos.Sites.Create(r.Context(), site); err != nil {
		slog.Error("create site", "error", err)
		data := newSiteData{Error: "Could not create site. Please try again."}
		if c, err := r.Cookie("csrf_token"); err == nil {
			data.CSRFToken = c.Value
		}
		w.WriteHeader(http.StatusInternalServerError)
		h.renderTemplate(w, "new-site.html", data)
		return
	}

	http.Redirect(w, r, "/sites/"+site.ID+"/overview", http.StatusSeeOther)
}

func (h *SitesHandler) renderTemplate(w http.ResponseWriter, name string, data any) {
	if h.tmpl == nil {
		w.Header().Set("Content-Type", "text/html")
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := h.tmpl.ExecuteTemplate(w, name, data); err != nil {
		slog.Error("render template", "name", name, "error", err)
	}
}
