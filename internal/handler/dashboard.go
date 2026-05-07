package handler

import (
	"encoding/json"
	"fmt"
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

// DashboardHandler handles all analytics dashboard routes.
type DashboardHandler struct {
	auth  service.AuthService
	repos *repository.Repos
	tmpls map[string]*template.Template
}

// NewDashboardHandler constructs a DashboardHandler. repos may be nil in tests.
func NewDashboardHandler(auth service.AuthService, repos *repository.Repos) *DashboardHandler {
	return &DashboardHandler{auth: auth, repos: repos}
}

// SetTemplates wires the template map.
func (h *DashboardHandler) SetTemplates(tmpls map[string]*template.Template) {
	h.tmpls = tmpls
}

var periodsAvailable = []struct{ Value, Label string }{
	{"today", "Today"},
	{"7d", "7 days"},
	{"30d", "30 days"},
	{"90d", "90 days"},
}

// Aggregate renders GET /dashboard — redirects to first site or site list.
func (h *DashboardHandler) Aggregate(w http.ResponseWriter, r *http.Request) {
	userID := middleware.UserIDFromContext(r.Context())

	if h.repos == nil {
		http.Redirect(w, r, "/account/sites/new", http.StatusSeeOther)
		return
	}

	sites, err := h.repos.Sites.ListByOwner(r.Context(), userID)
	if err != nil || len(sites) == 0 {
		http.Redirect(w, r, "/account/sites/new", http.StatusSeeOther)
		return
	}
	if len(sites) == 1 {
		http.Redirect(w, r, "/sites/"+sites[0].ID+"/overview", http.StatusSeeOther)
		return
	}

	h.renderDash(w, "aggregate.html", map[string]any{
		"Sites": sites, "ActiveNav": "overview",
		"AvailablePeriods": periodsAvailable, "Period": "30d",
		"SiteBaseURL": "/dashboard", "SiteDomain": "All sites",
	})
}

// Overview renders GET /sites/:siteID/overview.
func (h *DashboardHandler) Overview(w http.ResponseWriter, r *http.Request) {
	siteID := chi.URLParam(r, "siteID")
	userID := middleware.UserIDFromContext(r.Context())

	if h.repos == nil {
		http.NotFound(w, r)
		return
	}

	site, err := h.repos.Sites.GetByID(r.Context(), siteID)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	sites, err := h.repos.Sites.ListByOwner(r.Context(), userID)
	if err != nil || !siteInList(sites, siteID) {
		http.NotFound(w, r)
		return
	}

	period := r.URL.Query().Get("period")
	if period == "" {
		period = "30d"
	}
	from, to := service.DateRange(period)

	summary, err := h.repos.Stats.GetSummary(r.Context(), siteID, from, to)
	if err != nil {
		slog.Error("overview: summary", "error", err)
		summary = &model.StatsSummary{}
	}

	timeseries, _ := h.repos.Stats.GetTimeSeries(r.Context(), siteID, from, to)
	pages, _ := h.repos.Stats.GetTopPages(r.Context(), siteID, from, to, 10)
	sources, _ := h.repos.Stats.GetTopSources(r.Context(), siteID, from, to, 10)

	chartTimes, chartPageviews := timeSeriesJSON(timeseries)

	h.renderDash(w, "overview.html", map[string]any{
		"SiteID": siteID, "SiteDomain": site.Domain,
		"SiteBaseURL": "/sites/" + siteID, "ActiveNav": "overview",
		"Period": period, "AvailablePeriods": periodsAvailable,
		"Summary":    summary,
		"ChartTimes": template.JS(chartTimes), "ChartPageviews": template.JS(chartPageviews), //nolint:gosec // G203: server-generated JSON, not user input
		"TopPages": pages, "TopSources": sources,
	})
}

// Pages renders GET /sites/:siteID/pages.
func (h *DashboardHandler) Pages(w http.ResponseWriter, r *http.Request) {
	siteID := chi.URLParam(r, "siteID")
	if h.repos == nil {
		http.NotFound(w, r)
		return
	}
	site, err := h.repos.Sites.GetByID(r.Context(), siteID)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	period := periodParam(r)
	from, to := service.DateRange(period)
	pages, _ := h.repos.Stats.GetTopPages(r.Context(), siteID, from, to, 50)
	h.renderDash(w, "pages.html", map[string]any{
		"SiteID": siteID, "SiteDomain": site.Domain,
		"SiteBaseURL": "/sites/" + siteID, "ActiveNav": "pages",
		"Period": period, "AvailablePeriods": periodsAvailable, "Pages": pages,
	})
}

// Sources renders GET /sites/:siteID/sources.
func (h *DashboardHandler) Sources(w http.ResponseWriter, r *http.Request) {
	siteID := chi.URLParam(r, "siteID")
	if h.repos == nil {
		http.NotFound(w, r)
		return
	}
	site, err := h.repos.Sites.GetByID(r.Context(), siteID)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	period := periodParam(r)
	from, to := service.DateRange(period)
	sources, _ := h.repos.Stats.GetTopSources(r.Context(), siteID, from, to, 50)
	h.renderDash(w, "sources.html", map[string]any{
		"SiteID": siteID, "SiteDomain": site.Domain,
		"SiteBaseURL": "/sites/" + siteID, "ActiveNav": "sources",
		"Period": period, "AvailablePeriods": periodsAvailable, "Sources": sources,
	})
}

// Audience renders GET /sites/:siteID/audience.
func (h *DashboardHandler) Audience(w http.ResponseWriter, r *http.Request) {
	siteID := chi.URLParam(r, "siteID")
	if h.repos == nil {
		http.NotFound(w, r)
		return
	}
	site, err := h.repos.Sites.GetByID(r.Context(), siteID)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	period := periodParam(r)
	from, to := service.DateRange(period)
	countries, _ := h.repos.Stats.GetAudienceByDimension(r.Context(), siteID, "country", from, to, 20)
	devices, _ := h.repos.Stats.GetAudienceByDimension(r.Context(), siteID, "device_type", from, to, 5)
	browsers, _ := h.repos.Stats.GetAudienceByDimension(r.Context(), siteID, "browser", from, to, 10)
	h.renderDash(w, "audience.html", map[string]any{
		"SiteID": siteID, "SiteDomain": site.Domain,
		"SiteBaseURL": "/sites/" + siteID, "ActiveNav": "audience",
		"Period": period, "AvailablePeriods": periodsAvailable,
		"Countries": countries, "Devices": devices, "Browsers": browsers,
	})
}

// Events renders GET /sites/:siteID/events.
func (h *DashboardHandler) Events(w http.ResponseWriter, r *http.Request) {
	siteID := chi.URLParam(r, "siteID")
	if h.repos == nil {
		http.NotFound(w, r)
		return
	}
	site, err := h.repos.Sites.GetByID(r.Context(), siteID)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	period := periodParam(r)
	from, to := service.DateRange(period)

	events, err := h.repos.Events.ListCustomEvents(r.Context(), siteID, from, to, 50)
	if err != nil {
		slog.Error("dashboard.Events", "error", err)
		events = nil
	}

	h.renderDash(w, "events.html", map[string]any{
		"SiteID": siteID, "SiteDomain": site.Domain,
		"SiteBaseURL": "/sites/" + siteID, "ActiveNav": "events",
		"Period": period, "AvailablePeriods": periodsAvailable,
		"Events": events,
	})
}

// Funnels renders GET /sites/:siteID/funnels.
func (h *DashboardHandler) Funnels(w http.ResponseWriter, r *http.Request) {
	siteID := chi.URLParam(r, "siteID")
	if h.repos == nil {
		http.NotFound(w, r)
		return
	}
	site, err := h.repos.Sites.GetByID(r.Context(), siteID)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	funnels, err := h.repos.Funnels.ListBySite(r.Context(), siteID)
	if err != nil {
		slog.Error("dashboard.Funnels", "error", err)
		funnels = nil
	}
	var csrf string
	if c, err := r.Cookie("csrf_token"); err == nil {
		csrf = c.Value
	}
	h.renderDash(w, "funnels.html", map[string]any{
		"SiteID": siteID, "SiteDomain": site.Domain,
		"SiteBaseURL": "/sites/" + siteID, "ActiveNav": "funnels",
		"Period": "30d", "AvailablePeriods": periodsAvailable,
		"Funnels": funnels, "CSRFToken": csrf,
	})
}

// CreateFunnel handles POST /sites/:siteID/funnels.
// Form fields: name, step_name[] (repeating), step_type[] (repeating), step_value[] (repeating).
func (h *DashboardHandler) CreateFunnel(w http.ResponseWriter, r *http.Request) {
	siteID := chi.URLParam(r, "siteID")
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	name := strings.TrimSpace(r.FormValue("name"))
	if name == "" {
		http.Error(w, "name required", http.StatusUnprocessableEntity)
		return
	}
	stepNames := r.Form["step_name"]
	stepTypes := r.Form["step_type"]
	stepValues := r.Form["step_value"]
	if len(stepNames) < 2 {
		http.Error(w, "at least 2 steps required", http.StatusUnprocessableEntity)
		return
	}
	var steps []*model.FunnelStep
	for i := range stepNames {
		mt := "url"
		if i < len(stepTypes) && stepTypes[i] == "event" {
			mt = "event"
		}
		val := ""
		if i < len(stepValues) {
			val = strings.TrimSpace(stepValues[i])
		}
		sname := strings.TrimSpace(stepNames[i])
		if sname == "" || val == "" {
			continue
		}
		steps = append(steps, &model.FunnelStep{
			Position: len(steps), Name: sname, MatchType: mt, Value: val,
		})
	}
	if len(steps) < 2 {
		http.Error(w, "at least 2 valid steps required", http.StatusUnprocessableEntity)
		return
	}
	f := &model.Funnel{SiteID: siteID, Name: name}
	if err := h.repos.Funnels.Create(r.Context(), f, steps); err != nil {
		slog.Error("dashboard.CreateFunnel", "error", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/sites/"+siteID+"/funnels/"+f.ID, http.StatusSeeOther)
}

// DeleteFunnel handles POST /sites/:siteID/funnels/:funnelID/delete.
func (h *DashboardHandler) DeleteFunnel(w http.ResponseWriter, r *http.Request) {
	siteID := chi.URLParam(r, "siteID")
	funnelID := chi.URLParam(r, "funnelID")
	if err := h.repos.Funnels.Delete(r.Context(), funnelID, siteID); err != nil {
		slog.Error("dashboard.DeleteFunnel", "error", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/sites/"+siteID+"/funnels", http.StatusSeeOther)
}

// FunnelDetail renders GET /sites/:siteID/funnels/:funnelID.
// Full implementation in Task 8.
func (h *DashboardHandler) FunnelDetail(w http.ResponseWriter, r *http.Request) {
	http.NotFound(w, r)
}

func (h *DashboardHandler) renderDash(w http.ResponseWriter, name string, data any) {
	if h.tmpls == nil {
		w.Header().Set("Content-Type", "text/html")
		return
	}
	t, ok := h.tmpls[name]
	if !ok {
		slog.Error("dashboard template not found", "name", name)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := t.ExecuteTemplate(w, "dashboard.html", data); err != nil {
		slog.Error("render dashboard template", "name", name, "error", err)
	}
}

func siteInList(sites []*model.Site, id string) bool {
	for _, s := range sites {
		if s.ID == id {
			return true
		}
	}
	return false
}

func periodParam(r *http.Request) string {
	if p := r.URL.Query().Get("period"); p != "" {
		return p
	}
	return "30d"
}

func timeSeriesJSON(points []*model.TimePoint) (times, pageviews string) {
	if len(points) == 0 {
		return "[]", "[]"
	}
	ts := make([]int64, len(points))
	pvs := make([]int64, len(points))
	for i, p := range points {
		ts[i] = p.Time.Unix()
		pvs[i] = p.Pageviews
	}
	tb, _ := json.Marshal(ts)
	pb, _ := json.Marshal(pvs)
	return string(tb), string(pb)
}

// FormatNumber formats large integers with k/M suffix. Exported for template use.
func FormatNumber(n int64) string {
	if n >= 1_000_000 {
		return fmt.Sprintf("%.1fM", float64(n)/1_000_000)
	}
	if n >= 1000 {
		return fmt.Sprintf("%.1fk", float64(n)/1000)
	}
	return fmt.Sprintf("%d", n)
}

// FormatDuration converts milliseconds to "Xm Ys" display string. Exported for template use.
func FormatDuration(ms int64) string {
	if ms == 0 {
		return "0s"
	}
	total := ms / 1000
	m := total / 60
	s := total % 60
	if m == 0 {
		return fmt.Sprintf("%ds", s)
	}
	return fmt.Sprintf("%dm %ds", m, s)
}
