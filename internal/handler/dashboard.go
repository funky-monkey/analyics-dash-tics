package handler

import (
	"context"
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
	auth    service.AuthService
	repos   *repository.Repos
	tmpls   map[string]*template.Template
	baseURL string
}

// NewDashboardHandler constructs a DashboardHandler. repos may be nil in tests.
func NewDashboardHandler(auth service.AuthService, repos *repository.Repos, baseURL string) *DashboardHandler {
	return &DashboardHandler{auth: auth, repos: repos, baseURL: baseURL}
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
		http.Redirect(w, r, "/sites/"+domainSlug(sites[0].Domain)+"/overview", http.StatusSeeOther)
		return
	}

	h.renderDash(w, r, "aggregate.html", map[string]any{
		"Sites": sites, "ActiveNav": "overview",
		"AvailablePeriods": periodsAvailable, "Period": "30d",
		"SiteBaseURL": "/dashboard", "SiteDomain": "All sites",
	})
}

// Overview renders GET /sites/:siteID/overview.
func (h *DashboardHandler) Overview(w http.ResponseWriter, r *http.Request) {
	userID := middleware.UserIDFromContext(r.Context())

	if h.repos == nil {
		http.NotFound(w, r)
		return
	}

	site, err := resolveSite(r.Context(), h.repos, chi.URLParam(r, "siteID"))
	if err != nil {
		http.NotFound(w, r)
		return
	}

	sites, err := h.repos.Sites.ListByOwner(r.Context(), userID)
	if err != nil || !siteInList(sites, site.ID) {
		http.NotFound(w, r)
		return
	}

	period := r.URL.Query().Get("period")
	if period == "" {
		period = "30d"
	}
	from, to := service.DateRange(period)
	slug := domainSlug(site.Domain)

	summary, err := h.repos.Stats.GetSummary(r.Context(), site.ID, from, to)
	if err != nil {
		slog.Error("overview: summary", "error", err)
		summary = &model.StatsSummary{}
	}

	timeseries, _ := h.repos.Stats.GetTimeSeries(r.Context(), site.ID, from, to)
	pages, _ := h.repos.Stats.GetTopPages(r.Context(), site.ID, from, to, 5)
	sources, _ := h.repos.Stats.GetTopSources(r.Context(), site.ID, from, to, 30)
	countries, _ := h.repos.Stats.GetAudienceByDimension(r.Context(), site.ID, "country", from, to, 5)
	devices, _ := h.repos.Stats.GetAudienceByDimension(r.Context(), site.ID, "device_type", from, to, 5)
	browsers, _ := h.repos.Stats.GetAudienceByDimension(r.Context(), site.ID, "browser", from, to, 5)
	// Load up to 3 funnels with full drop-off data for the overview cards.
	funnelList, _ := h.repos.Funnels.ListBySite(r.Context(), site.ID)
	if len(funnelList) > 3 {
		funnelList = funnelList[:3]
	}
	var funnelResults []*model.FunnelResult
	for _, f := range funnelList {
		_, steps, err := h.repos.Funnels.GetWithSteps(r.Context(), f.ID, site.ID)
		if err != nil || len(steps) == 0 {
			continue
		}
		counts, err := h.repos.Funnels.GetDropOff(r.Context(), site.ID, steps, from, to)
		if err != nil {
			counts = make([]int64, len(steps))
		}
		funnelResults = append(funnelResults, service.BuildFunnelResult(f, steps, counts))
	}

	chartTimes, chartPageviews := timeSeriesJSON(timeseries)

	h.renderDash(w, r, "overview.html", map[string]any{
		"SiteID": slug, "SiteDomain": site.Domain,
		"SiteBaseURL": "/sites/" + slug, "ActiveNav": "overview",
		"Period": period, "AvailablePeriods": periodsAvailable,
		"Summary":        summary,
		"ChartTimes":     template.JS(chartTimes),     //nolint:gosec // G203: server-generated JSON, not user input
		"ChartPageviews": template.JS(chartPageviews), //nolint:gosec
		"HasTimeSeries":  len(timeseries) > 0,
		"TopPages":       pages,
		"TopSources":     groupSources(sources),
		"Countries":      countries,
		"Devices":        devices,
		"Browsers":       browsers,
		"FunnelResults":  funnelResults,
		"HasData":        summary.Pageviews > 0 || summary.Visitors > 0,
		"BaseURL":        h.baseURL,
		"SiteToken":      site.Token,
	})
}

// Pages renders GET /sites/:siteID/pages.
func (h *DashboardHandler) Pages(w http.ResponseWriter, r *http.Request) {
	if h.repos == nil {
		http.NotFound(w, r)
		return
	}
	site, err := resolveSite(r.Context(), h.repos, chi.URLParam(r, "siteID"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	period := periodParam(r)
	from, to := service.DateRange(period)
	slug := domainSlug(site.Domain)
	pages, _ := h.repos.Stats.GetTopPages(r.Context(), site.ID, from, to, 50)
	h.renderDash(w, r, "pages.html", map[string]any{
		"SiteID": slug, "SiteDomain": site.Domain,
		"SiteBaseURL": "/sites/" + slug, "ActiveNav": "pages",
		"Period": period, "AvailablePeriods": periodsAvailable, "Pages": pages,
	})
}

// Sources renders GET /sites/:siteID/sources.
func (h *DashboardHandler) Sources(w http.ResponseWriter, r *http.Request) {
	if h.repos == nil {
		http.NotFound(w, r)
		return
	}
	site, err := resolveSite(r.Context(), h.repos, chi.URLParam(r, "siteID"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	period := periodParam(r)
	from, to := service.DateRange(period)
	slug := domainSlug(site.Domain)
	sources, _ := h.repos.Stats.GetTopSources(r.Context(), site.ID, from, to, 200)
	h.renderDash(w, r, "sources.html", map[string]any{
		"SiteID": slug, "SiteDomain": site.Domain,
		"SiteBaseURL": "/sites/" + slug, "ActiveNav": "sources",
		"Period": period, "AvailablePeriods": periodsAvailable,
		"SourceGroups": groupSources(sources),
	})
}

// SourceRow is one referrer row inside a channel group.
type SourceRow struct {
	Referrer  string
	Sessions  int64
	Pageviews int64
	BarPct    int // 0–100 for CSS width
}

// SourceGroup is one channel section on the sources page.
type SourceGroup struct {
	Channel string
	Label   string
	Dot     string // accent hex (dot + border)
	Bg      string // badge background hex
	Text    string // badge text hex
	Total   int64
	Rows    []SourceRow
}

// channelOrder defines the display order for channel groups.
var channelOrder = []string{
	"organic", "direct", "referral", "social", "email", "paid", "ai", "dark_social",
}

type channelStyle struct{ dot, bg, text, label string }

var channelStyles = map[string]channelStyle{
	"organic":    {"#22c55e", "#dcfce7", "#15803d", "Organic search"},
	"direct":     {"#94a3b8", "#f1f5f9", "#475569", "Direct"},
	"referral":   {"#06b6d4", "#cffafe", "#0e7490", "Referral"},
	"social":     {"#3b82f6", "#dbeafe", "#1d4ed8", "Social"},
	"email":      {"#f59e0b", "#fef3c7", "#92400e", "Email"},
	"paid":       {"#f97316", "#ffedd5", "#c2410c", "Paid"},
	"ai":         {"#8b5cf6", "#ede9fe", "#6d28d9", "AI"},
	"dark_social":{"#64748b", "#f8fafc", "#334155", "Dark social"},
}

// groupSources converts a flat source list into ordered, colored channel groups.
func groupSources(sources []*model.SourceStat) []SourceGroup {
	byChannel := make(map[string][]model.SourceStat)
	for _, s := range sources {
		ch := s.Channel
		if ch == "" {
			ch = "direct"
		}
		byChannel[ch] = append(byChannel[ch], *s)
	}

	// Build groups in canonical order; unknown channels appended last.
	seen := make(map[string]bool)
	var groups []SourceGroup
	for _, ch := range channelOrder {
		rows, ok := byChannel[ch]
		if !ok {
			continue
		}
		seen[ch] = true
		groups = append(groups, buildGroup(ch, rows))
	}
	for ch, rows := range byChannel {
		if !seen[ch] {
			groups = append(groups, buildGroup(ch, rows))
		}
	}
	return groups
}

func buildGroup(channel string, rows []model.SourceStat) SourceGroup {
	style, ok := channelStyles[channel]
	if !ok {
		style = channelStyle{"#94a3b8", "#f1f5f9", "#475569", strings.Title(channel)} //nolint:staticcheck
	}

	var total int64
	var max int64
	for _, r := range rows {
		total += r.Sessions
		if r.Sessions > max {
			max = r.Sessions
		}
	}

	srows := make([]SourceRow, 0, len(rows))
	for _, r := range rows {
		ref := cleanReferrer(r.Referrer)
		if ref == "" {
			ref = "(direct / none)"
		}
		pct := 0
		if max > 0 {
			pct = int(r.Sessions * 100 / max)
		}
		srows = append(srows, SourceRow{
			Referrer:  ref,
			Sessions:  r.Sessions,
			Pageviews: r.Pageviews,
			BarPct:    pct,
		})
	}

	return SourceGroup{
		Channel: channel,
		Label:   style.label,
		Dot:     style.dot,
		Bg:      style.bg,
		Text:    style.text,
		Total:   total,
		Rows:    srows,
	}
}

// cleanReferrer strips protocol and www from a referrer URL.
func cleanReferrer(ref string) string {
	ref = strings.TrimPrefix(ref, "https://")
	ref = strings.TrimPrefix(ref, "http://")
	ref = strings.TrimPrefix(ref, "www.")
	ref = strings.TrimSuffix(ref, "/")
	return ref
}

// Audience renders GET /sites/:siteID/audience.
func (h *DashboardHandler) Audience(w http.ResponseWriter, r *http.Request) {
	if h.repos == nil {
		http.NotFound(w, r)
		return
	}
	site, err := resolveSite(r.Context(), h.repos, chi.URLParam(r, "siteID"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	period := periodParam(r)
	from, to := service.DateRange(period)
	slug := domainSlug(site.Domain)
	countries, _ := h.repos.Stats.GetAudienceByDimension(r.Context(), site.ID, "country", from, to, 20)
	devices, _ := h.repos.Stats.GetAudienceByDimension(r.Context(), site.ID, "device_type", from, to, 5)
	browsers, _ := h.repos.Stats.GetAudienceByDimension(r.Context(), site.ID, "browser", from, to, 10)
	h.renderDash(w, r, "audience.html", map[string]any{
		"SiteID": slug, "SiteDomain": site.Domain,
		"SiteBaseURL": "/sites/" + slug, "ActiveNav": "audience",
		"Period": period, "AvailablePeriods": periodsAvailable,
		"Countries": countries, "Devices": devices, "Browsers": browsers,
	})
}

// Events renders GET /sites/:siteID/events.
func (h *DashboardHandler) Events(w http.ResponseWriter, r *http.Request) {
	if h.repos == nil {
		http.NotFound(w, r)
		return
	}
	site, err := resolveSite(r.Context(), h.repos, chi.URLParam(r, "siteID"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	period := periodParam(r)
	from, to := service.DateRange(period)
	slug := domainSlug(site.Domain)

	events, err := h.repos.Events.ListCustomEvents(r.Context(), site.ID, from, to, 50)
	if err != nil {
		slog.Error("dashboard.Events", "error", err)
		events = nil
	}

	h.renderDash(w, r, "events.html", map[string]any{
		"SiteID": slug, "SiteDomain": site.Domain,
		"SiteBaseURL": "/sites/" + slug, "ActiveNav": "events",
		"Period": period, "AvailablePeriods": periodsAvailable,
		"Events": events,
	})
}

// Funnels renders GET /sites/:siteID/funnels.
func (h *DashboardHandler) Funnels(w http.ResponseWriter, r *http.Request) {
	if h.repos == nil {
		http.NotFound(w, r)
		return
	}
	site, err := resolveSite(r.Context(), h.repos, chi.URLParam(r, "siteID"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	funnels, err := h.repos.Funnels.ListBySite(r.Context(), site.ID)
	if err != nil {
		slog.Error("dashboard.Funnels", "error", err)
		funnels = nil
	}
	slug := domainSlug(site.Domain)
	var csrf string
	if c, err := r.Cookie("csrf_token"); err == nil {
		csrf = c.Value
	}
	h.renderDash(w, r, "funnels.html", map[string]any{
		"SiteID": slug, "SiteDomain": site.Domain,
		"SiteBaseURL": "/sites/" + slug, "ActiveNav": "funnels",
		"Period": "30d", "AvailablePeriods": periodsAvailable,
		"Funnels": funnels, "CSRFToken": csrf,
	})
}

// CreateFunnel handles POST /sites/:siteID/funnels.
// Form fields: name, step_name[] (repeating), step_type[] (repeating), step_value[] (repeating).
func (h *DashboardHandler) CreateFunnel(w http.ResponseWriter, r *http.Request) {
	site, err := resolveSite(r.Context(), h.repos, chi.URLParam(r, "siteID"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
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
	f := &model.Funnel{SiteID: site.ID, Name: name}
	if err := h.repos.Funnels.Create(r.Context(), f, steps); err != nil {
		slog.Error("dashboard.CreateFunnel", "error", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	slug := domainSlug(site.Domain)
	http.Redirect(w, r, "/sites/"+slug+"/funnels/"+f.ID, http.StatusSeeOther)
}

// DeleteFunnel handles POST /sites/:siteID/funnels/:funnelID/delete.
func (h *DashboardHandler) DeleteFunnel(w http.ResponseWriter, r *http.Request) {
	site, err := resolveSite(r.Context(), h.repos, chi.URLParam(r, "siteID"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	funnelID := chi.URLParam(r, "funnelID")
	if err := h.repos.Funnels.Delete(r.Context(), funnelID, site.ID); err != nil {
		slog.Error("dashboard.DeleteFunnel", "error", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/sites/"+domainSlug(site.Domain)+"/funnels", http.StatusSeeOther)
}

// FunnelDetail renders GET /sites/:siteID/funnels/:funnelID.
func (h *DashboardHandler) FunnelDetail(w http.ResponseWriter, r *http.Request) {
	funnelID := chi.URLParam(r, "funnelID")
	if h.repos == nil {
		http.NotFound(w, r)
		return
	}
	site, err := resolveSite(r.Context(), h.repos, chi.URLParam(r, "siteID"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	funnel, steps, err := h.repos.Funnels.GetWithSteps(r.Context(), funnelID, site.ID)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	period := periodParam(r)
	from, to := service.DateRange(period)
	slug := domainSlug(site.Domain)

	counts, err := h.repos.Funnels.GetDropOff(r.Context(), site.ID, steps, from, to)
	if err != nil {
		slog.Error("dashboard.FunnelDetail", "error", err)
		counts = make([]int64, len(steps))
	}

	result := service.BuildFunnelResult(funnel, steps, counts)

	var csrf string
	if c, err := r.Cookie("csrf_token"); err == nil {
		csrf = c.Value
	}
	h.renderDash(w, r, "funnel-detail.html", map[string]any{
		"SiteID": slug, "SiteDomain": site.Domain,
		"SiteBaseURL": "/sites/" + slug, "ActiveNav": "funnels",
		"Period": period, "AvailablePeriods": periodsAvailable,
		"Result": result, "CSRFToken": csrf,
	})
}

func (h *DashboardHandler) renderDash(w http.ResponseWriter, r *http.Request, name string, data any) {
	injectNonce(r, data)
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

// DomainSlug converts a domain to a URL-safe slug: dots become dashes.
// "acme.io" → "acme-io", "sub.acme.io" → "sub-acme-io"
// Exported so it can be registered as a template FuncMap function.
func DomainSlug(domain string) string {
	return strings.ReplaceAll(domain, ".", "-")
}

func domainSlug(domain string) string { return DomainSlug(domain) }

// resolveSite looks up a site by either its UUID or its domain slug.
// UUID format (36 chars, hyphens at positions 8/13/18/23) goes to GetByID;
// everything else goes to GetBySlug.
func resolveSite(ctx context.Context, repos *repository.Repos, param string) (*model.Site, error) {
	if len(param) == 36 && param[8] == '-' && param[13] == '-' && param[18] == '-' {
		return repos.Sites.GetByID(ctx, param)
	}
	return repos.Sites.GetBySlug(ctx, param)
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
