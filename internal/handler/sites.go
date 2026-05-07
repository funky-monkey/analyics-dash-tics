package handler

import (
	"html/template"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/sidneydekoning/analytics/internal/middleware"
	"github.com/sidneydekoning/analytics/internal/model"
	"github.com/sidneydekoning/analytics/internal/repository"
	"github.com/sidneydekoning/analytics/internal/service"
)

// SitesHandler handles site registration and management routes.
type SitesHandler struct {
	auth    service.AuthService
	repos   *repository.Repos
	baseURL string
	tmpls   map[string]*template.Template
}

// NewSitesHandler constructs a SitesHandler.
func NewSitesHandler(auth service.AuthService, repos *repository.Repos, baseURL string) *SitesHandler {
	return &SitesHandler{auth: auth, repos: repos, baseURL: baseURL}
}

// SetTemplates wires the template map. Each key is a page name, each value is
// a self-contained template set (base + page).
func (h *SitesHandler) SetTemplates(tmpls map[string]*template.Template) {
	h.tmpls = tmpls
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

	http.Redirect(w, r, "/sites/"+site.ID+"/setup", http.StatusSeeOther)
}

// Setup renders GET /sites/:siteID/setup — the post-creation onboarding page.
func (h *SitesHandler) Setup(w http.ResponseWriter, r *http.Request) {
	siteID := r.PathValue("siteID")
	if h.repos == nil {
		http.NotFound(w, r)
		return
	}
	site, err := h.repos.Sites.GetByID(r.Context(), siteID)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	h.renderTemplate(w, "site-setup.html", map[string]any{
		"Site":    site,
		"BaseURL": h.baseURL,
	})
}

// CheckTracking handles GET /sites/:siteID/check-tracking — returns JSON indicating
// whether any events have been received for this site in the last 30 minutes.
func (h *SitesHandler) CheckTracking(w http.ResponseWriter, r *http.Request) {
	siteID := r.PathValue("siteID")
	if h.repos == nil {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"detected":false}`))
		return
	}
	from := time.Now().Add(-30 * time.Minute)
	count, err := h.repos.Events.CountBySite(r.Context(), siteID, from, time.Now())
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"detected":false}`))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if count > 0 {
		w.Write([]byte(`{"detected":true}`))
	} else {
		w.Write([]byte(`{"detected":false}`))
	}
}

func (h *SitesHandler) renderTemplate(w http.ResponseWriter, name string, data any) {
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
