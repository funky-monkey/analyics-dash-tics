package handler

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/funky-monkey/analyics-dash-tics/internal/model"
	"github.com/funky-monkey/analyics-dash-tics/internal/repository"
	"github.com/funky-monkey/analyics-dash-tics/internal/service"
)

// CollectHandler handles POST /collect — the hot-path event ingestion endpoint.
// The response is sent before the database write completes.
type CollectHandler struct {
	collectSvc service.CollectService
	repos      *repository.Repos
}

// NewCollectHandler constructs a CollectHandler. repos may be nil in tests.
func NewCollectHandler(collectSvc service.CollectService, repos *repository.Repos) *CollectHandler {
	return &CollectHandler{collectSvc: collectSvc, repos: repos}
}

// Collect handles POST /collect.
func (h *CollectHandler) Collect(w http.ResponseWriter, r *http.Request) {
	var req model.CollectRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	if req.Site == "" {
		http.Error(w, "missing site token", http.StatusBadRequest)
		return
	}

	now := time.Now().UTC()

	ev, err := h.collectSvc.BuildEvent("", &req, r, now)
	if errors.Is(err, service.ErrBot) {
		w.WriteHeader(http.StatusAccepted)
		return
	}
	if err != nil {
		slog.Error("collect: build event", "error", err)
		w.WriteHeader(http.StatusAccepted)
		return
	}

	w.WriteHeader(http.StatusAccepted)

	if h.repos == nil {
		return
	}

	go func() {
		ctx := r.Context()
		site, err := h.repos.Sites.GetByToken(ctx, req.Site)
		if err != nil {
			return
		}
		ev.SiteID = site.ID
		if err := h.repos.Events.Write(ctx, ev); err != nil {
			slog.Error("collect: write event", "error", err, "site_id", site.ID)
			return
		}
		// Best-effort: first-seen data is advisory; ignore errors.
		_ = h.repos.Events.UpsertVisitorFirstSeen(ctx, ev.SiteID, ev.VisitorID)
	}()
}
