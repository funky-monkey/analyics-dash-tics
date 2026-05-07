package handler

import (
	"html/template"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/microcosm-cc/bluemonday"
	"github.com/sidneydekoning/analytics/internal/middleware"
	"github.com/sidneydekoning/analytics/internal/model"
	"github.com/sidneydekoning/analytics/internal/repository"
	"github.com/sidneydekoning/analytics/internal/service"
)

var (
	slugPattern = regexp.MustCompile(`^[a-z0-9-]+$`)
	cmsPolicy   = bluemonday.UGCPolicy()
)

// CMSHandler handles /admin/cms/* (admin CMS editing) and public /blog/:slug, /p/:slug.
type CMSHandler struct {
	auth  service.AuthService
	repos *repository.Repos
	tmpls map[string]*template.Template
}

// NewCMSHandler constructs a CMSHandler.
func NewCMSHandler(auth service.AuthService, repos *repository.Repos) *CMSHandler {
	return &CMSHandler{auth: auth, repos: repos}
}

// SetTemplates wires the template map.
func (h *CMSHandler) SetTemplates(tmpls map[string]*template.Template) {
	h.tmpls = tmpls
}

// CMSList renders GET /admin/cms.
func (h *CMSHandler) CMSList(w http.ResponseWriter, r *http.Request) {
	data := map[string]any{"ActiveNav": "cms", "Pages": []*model.CMSPage{}, "CSRFToken": csrfToken(r)}
	if h.repos != nil {
		pages, err := h.repos.CMS.ListPages(r.Context(), 100, 0)
		if err != nil {
			slog.Error("cms.CMSList", "error", err)
		} else {
			data["Pages"] = pages
		}
	}
	h.renderAdmin(w, "cms-list.html", data)
}

// NewPageForm renders GET /admin/cms/new.
func (h *CMSHandler) NewPageForm(w http.ResponseWriter, r *http.Request) {
	h.renderAdmin(w, "cms-edit.html", map[string]any{
		"ActiveNav": "cms",
		"Page":      &model.CMSPage{Type: "blog", Status: "draft"},
		"CSRFToken": csrfToken(r),
	})
}

// CreatePage handles POST /admin/cms/new.
func (h *CMSHandler) CreatePage(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	title := strings.TrimSpace(r.FormValue("title"))
	slug := strings.TrimSpace(r.FormValue("slug"))
	pageType := r.FormValue("type")

	if title == "" || slug == "" {
		h.renderAdmin(w, "cms-edit.html", map[string]any{
			"ActiveNav": "cms", "CSRFToken": csrfToken(r),
			"Page":  &model.CMSPage{Title: title, Slug: slug, Type: pageType},
			"Error": "Title and slug are required.",
		})
		return
	}
	if !slugPattern.MatchString(slug) {
		h.renderAdmin(w, "cms-edit.html", map[string]any{
			"ActiveNav": "cms", "CSRFToken": csrfToken(r),
			"Page":  &model.CMSPage{Title: title, Slug: slug, Type: pageType},
			"Error": "Slug may only contain lowercase letters, numbers, and hyphens.",
		})
		return
	}
	if pageType != "blog" && pageType != "page" {
		pageType = "blog"
	}

	cleanHTML := cmsPolicy.Sanitize(r.FormValue("content_html"))
	status, publishedAt := resolvePublishState(r.FormValue("action"), r.FormValue("scheduled_at"))

	page := &model.CMSPage{
		LayoutID:        "00000000-0000-0000-0000-000000000001",
		AuthorID:        middleware.UserIDFromContext(r.Context()),
		Title:           title,
		Slug:            slug,
		Type:            pageType,
		ContentHTML:     cleanHTML,
		Excerpt:         strings.TrimSpace(r.FormValue("excerpt")),
		CoverImageURL:   strings.TrimSpace(r.FormValue("cover_image_url")),
		MetaTitle:       strings.TrimSpace(r.FormValue("meta_title")),
		MetaDescription: strings.TrimSpace(r.FormValue("meta_description")),
		Status:          status,
		PublishedAt:     publishedAt,
	}

	if err := h.repos.CMS.CreatePage(r.Context(), page); err != nil {
		h.renderAdmin(w, "cms-edit.html", map[string]any{
			"ActiveNav": "cms", "CSRFToken": csrfToken(r),
			"Page": page, "Error": "Could not save. Slug may already exist.",
		})
		return
	}
	actorID := middleware.UserIDFromContext(r.Context())
	auditLog(h.repos, r, actorID, "create_page:"+status, "cms_page", page.ID)
	http.Redirect(w, r, "/admin/cms/"+page.ID+"/edit", http.StatusSeeOther)
}

// EditPageForm renders GET /admin/cms/:id/edit.
func (h *CMSHandler) EditPageForm(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	page, err := h.repos.CMS.GetPageByID(r.Context(), id)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	h.renderAdmin(w, "cms-edit.html", map[string]any{
		"ActiveNav": "cms", "Page": page, "CSRFToken": csrfToken(r),
	})
}

// UpdatePage handles POST /admin/cms/:id/edit.
func (h *CMSHandler) UpdatePage(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	title := strings.TrimSpace(r.FormValue("title"))
	slug := strings.TrimSpace(r.FormValue("slug"))
	if title == "" || slug == "" || !slugPattern.MatchString(slug) {
		http.Error(w, "invalid input", http.StatusUnprocessableEntity)
		return
	}
	cleanHTML := cmsPolicy.Sanitize(r.FormValue("content_html"))
	status, publishedAt := resolvePublishState(r.FormValue("action"), r.FormValue("scheduled_at"))

	page := &model.CMSPage{
		ID:              id,
		Title:           title,
		Slug:            slug,
		ContentHTML:     cleanHTML,
		Excerpt:         strings.TrimSpace(r.FormValue("excerpt")),
		CoverImageURL:   strings.TrimSpace(r.FormValue("cover_image_url")),
		MetaTitle:       strings.TrimSpace(r.FormValue("meta_title")),
		MetaDescription: strings.TrimSpace(r.FormValue("meta_description")),
	}
	if err := h.repos.CMS.UpdatePage(r.Context(), page); err != nil {
		slog.Error("cms.UpdatePage", "error", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	// Update publish state separately so status + published_at are always in sync
	if err := h.repos.CMS.SetPageStatus(r.Context(), id, status, publishedAt); err != nil {
		slog.Error("cms.UpdatePage: set status", "error", err)
	}
	actorID := middleware.UserIDFromContext(r.Context())
	auditLog(h.repos, r, actorID, "update_page:"+status, "cms_page", id)
	http.Redirect(w, r, "/admin/cms/"+id+"/edit", http.StatusSeeOther)
}

// DeletePage handles POST /admin/cms/:id/delete.
func (h *CMSHandler) DeletePage(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	actorID := middleware.UserIDFromContext(r.Context())
	if err := h.repos.CMS.DeletePage(r.Context(), id); err != nil {
		slog.Error("cms.DeletePage", "error", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	auditLog(h.repos, r, actorID, "delete_page", "cms_page", id)
	http.Redirect(w, r, "/admin/cms", http.StatusSeeOther)
}

// resolvePublishState converts a form action + optional scheduled_at string
// into a (status, *publishedAt) pair ready to store.
func resolvePublishState(action, scheduledAt string) (status string, publishedAt *time.Time) {
	switch action {
	case "publish":
		now := time.Now()
		return "published", &now
	case "schedule":
		t, err := time.ParseInLocation("2006-01-02T15:04", scheduledAt, time.UTC)
		if err == nil && t.After(time.Now()) {
			return "draft", &t
		}
		// Fallback: treat as publish now if date is invalid or in the past
		now := time.Now()
		return "published", &now
	default: // "draft" or anything else
		return "draft", nil
	}
}

// TogglePublish handles POST /admin/cms/:id/publish.
func (h *CMSHandler) TogglePublish(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	page, err := h.repos.CMS.GetPageByID(r.Context(), id)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	newStatus := "published"
	var publishedAt *time.Time
	if page.Status == "published" {
		newStatus = "draft"
	} else {
		now := time.Now()
		publishedAt = &now
	}
	if err := h.repos.CMS.SetPageStatus(r.Context(), id, newStatus, publishedAt); err != nil {
		slog.Error("cms.TogglePublish", "error", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	actorID := middleware.UserIDFromContext(r.Context())
	auditLog(h.repos, r, actorID, "set_page_status:"+newStatus, "cms_page", id)
	http.Redirect(w, r, "/admin/cms", http.StatusSeeOther)
}

// BlogList renders GET /blog.
func (h *CMSHandler) BlogList(w http.ResponseWriter, r *http.Request) {
	if h.repos == nil {
		h.renderPublic(w, "blog-list.html", map[string]any{"Posts": []*model.CMSPage{}})
		return
	}
	posts, err := h.repos.CMS.ListPublishedByType(r.Context(), "blog", 20, 0)
	if err != nil {
		slog.Error("cms.BlogList", "error", err)
	}
	h.renderPublic(w, "blog-list.html", map[string]any{"Posts": posts})
}

// BlogPost renders GET /blog/:slug.
func (h *CMSHandler) BlogPost(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	if h.repos == nil {
		http.NotFound(w, r)
		return
	}
	page, err := h.repos.CMS.GetPageBySlug(r.Context(), slug)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	h.renderPublic(w, "blog-post.html", map[string]any{"Page": page})
}

// GenericPage renders GET /p/:slug.
func (h *CMSHandler) GenericPage(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	if h.repos == nil {
		http.NotFound(w, r)
		return
	}
	page, err := h.repos.CMS.GetPageBySlug(r.Context(), slug)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	h.renderPublic(w, "page.html", map[string]any{"Page": page})
}

func (h *CMSHandler) renderAdmin(w http.ResponseWriter, name string, data any) {
	if h.tmpls == nil {
		w.Header().Set("Content-Type", "text/html")
		return
	}
	t, ok := h.tmpls[name]
	if !ok {
		slog.Error("cms admin template not found", "name", name)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := t.ExecuteTemplate(w, "admin.html", data); err != nil {
		slog.Error("render cms admin template", "name", name, "error", err)
	}
}

func (h *CMSHandler) renderPublic(w http.ResponseWriter, name string, data any) {
	if h.tmpls == nil {
		w.Header().Set("Content-Type", "text/html")
		return
	}
	t, ok := h.tmpls[name]
	if !ok {
		slog.Error("cms public template not found", "name", name)
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := t.ExecuteTemplate(w, "base.html", data); err != nil {
		slog.Error("render cms public template", "name", name, "error", err)
	}
}
