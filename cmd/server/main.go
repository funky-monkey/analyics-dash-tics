package main

import (
	"context"
	"fmt"
	"html/template"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/sidneydekoning/analytics/config"
	"github.com/sidneydekoning/analytics/internal/handler"
	"github.com/sidneydekoning/analytics/internal/middleware"
	"github.com/sidneydekoning/analytics/internal/repository"
	"github.com/sidneydekoning/analytics/internal/service"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))

	cfg, err := config.Load()
	if err != nil {
		slog.Error("config", "error", err)
		os.Exit(1)
	}

	ctx := context.Background()

	pool, err := repository.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		slog.Error("database", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	if err := repository.Migrate(ctx, pool); err != nil {
		slog.Error("migrations", "error", err)
		os.Exit(1)
	}

	repos := repository.New(pool)
	authSvc := service.NewAuth(cfg.JWTSecret, cfg.JWTRefreshSecret)
	geo := service.NewGeoLocator(cfg.MaxMindDBPath)
	fp := service.NewFingerprinter(string(cfg.JWTSecret))
	collectSvc := service.NewCollectService(geo, fp)
	collectHandler := handler.NewCollectHandler(collectSvc, repos)

	tmpls, err := buildTemplateMap("templates/layout/base.html", "templates")
	if err != nil {
		slog.Error("templates", "error", err)
		os.Exit(1)
	}

	authHandler := handler.NewAuthHandler(authSvc, repos, cfg.BaseURL)
	authHandler.SetTemplates(tmpls)

	sitesHandler := handler.NewSitesHandler(authSvc, repos, cfg.BaseURL)
	sitesHandler.SetTemplates(tmpls)

	dashHandler := handler.NewDashboardHandler(authSvc, repos)
	dashHandler.SetTemplates(tmpls)

	adminHandler := handler.NewAdminHandler(authSvc, repos)
	adminHandler.SetTemplates(tmpls)

	cmsHandler := handler.NewCMSHandler(authSvc, repos)
	cmsHandler.SetTemplates(tmpls)

	r := chi.NewRouter()

	// Global middleware — order matters: logger and security headers wrap everything.
	r.Use(middleware.Logger)
	r.Use(middleware.SecurityHeaders)
	r.Use(middleware.CORS(cfg.AllowedOrigins, "/collect"))
	r.Use(middleware.CSRF)
	r.Use(chimiddleware.Recoverer)

	// Static files
	r.Handle("/static/*", http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))

	// Public routes
	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		t, ok := tmpls["home.html"]
		if !ok {
			http.Redirect(w, r, "/login", http.StatusTemporaryRedirect)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := t.ExecuteTemplate(w, "base.html", nil); err != nil {
			slog.Error("render home", "error", err)
		}
	})

	// Public CMS routes
	r.Get("/blog", cmsHandler.BlogList)
	r.Get("/blog/{slug}", cmsHandler.BlogPost)
	r.Get("/p/{slug}", cmsHandler.GenericPage)

	// Auth routes
	r.Get("/login", authHandler.LoginPage)
	r.Post("/login", authHandler.Login)
	r.Get("/signup", authHandler.SignupPage)
	r.Post("/signup", authHandler.Signup)
	r.Post("/logout", authHandler.Logout)
	r.Get("/forgot-password", authHandler.ForgotPasswordPage)
	r.Post("/forgot-password", authHandler.ForgotPassword)
	r.Get("/reset-password/{token}", authHandler.ResetPasswordPage)
	r.Post("/reset-password/{token}", authHandler.ResetPassword)

	// Tracking endpoint — no JWT, site token auth only, tighter rate limit
	collectLimiter := middleware.RateLimiter(100.0/60.0, 20)
	r.With(collectLimiter).Post("/collect", collectHandler.Collect)

	// Authenticated routes
	jwtAuth := middleware.JWTAuth(authSvc)

	r.With(jwtAuth).Group(func(r chi.Router) {
		r.Get("/dashboard", dashHandler.Aggregate)
		r.Get("/account/sites/new", sitesHandler.NewSitePage)
		r.Post("/account/sites/new", sitesHandler.CreateSite)
		r.Get("/sites/{siteID}/setup", sitesHandler.Setup)
		r.Get("/sites/{siteID}/check-tracking", sitesHandler.CheckTracking)
		r.Get("/sites/{siteID}/overview", dashHandler.Overview)
		r.Get("/sites/{siteID}/pages", dashHandler.Pages)
		r.Get("/sites/{siteID}/sources", dashHandler.Sources)
		r.Get("/sites/{siteID}/audience", dashHandler.Audience)
		r.Get("/sites/{siteID}/events", dashHandler.Events)
		r.Get("/sites/{siteID}/funnels", dashHandler.Funnels)
		r.Post("/sites/{siteID}/funnels", dashHandler.CreateFunnel)
		r.Post("/sites/{siteID}/funnels/{funnelID}/delete", dashHandler.DeleteFunnel)
		r.Get("/sites/{siteID}/funnels/{funnelID}", dashHandler.FunnelDetail)
		r.Get("/sites/{siteID}/settings", sitesHandler.Settings)
		r.Post("/sites/{siteID}/settings", sitesHandler.UpdateSite)
		r.Post("/sites/{siteID}/delete", sitesHandler.DeleteSite)
		r.Get("/sites/{siteID}/goals", sitesHandler.GoalsList)
		r.Post("/sites/{siteID}/goals", sitesHandler.CreateGoal)
		r.Post("/sites/{siteID}/goals/{goalID}/delete", sitesHandler.DeleteGoal)
	})

	// Admin routes — role=admin required
	adminOnly := middleware.RequireRole("admin")
	r.With(jwtAuth, adminOnly).Group(func(r chi.Router) {
		r.Get("/admin", adminHandler.Index)
		r.Get("/admin/users", adminHandler.Users)
		r.Get("/admin/users/new", adminHandler.NewUserPage)
		r.Post("/admin/users/new", adminHandler.CreateUser)
		r.Get("/admin/users/{id}", adminHandler.EditUserPage)
		r.Post("/admin/users/{id}", adminHandler.UpdateUser)
		r.Post("/admin/users/{id}/delete", adminHandler.DeleteUser)
		r.Get("/admin/sites", adminHandler.Sites)
		r.Post("/admin/sites/{id}/delete", adminHandler.DeleteSite)
		r.Get("/admin/cms", cmsHandler.CMSList)
		r.Get("/admin/cms/new", cmsHandler.NewPageForm)
		r.Post("/admin/cms/new", cmsHandler.CreatePage)
		r.Get("/admin/cms/{id}/edit", cmsHandler.EditPageForm)
		r.Post("/admin/cms/{id}/edit", cmsHandler.UpdatePage)
		r.Post("/admin/cms/{id}/publish", cmsHandler.TogglePublish)
		r.Post("/admin/cms/{id}/delete", cmsHandler.DeletePage)
		r.Get("/admin/audit-log", adminHandler.AuditLog)
	})

	srv := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.Port),
		Handler:      r,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	slog.Info("server starting", "port", cfg.Port, "env", cfg.Env)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		slog.Error("server", "error", err)
		os.Exit(1)
	}
}

// buildTemplateMap builds a map from page basename → isolated template set.
// Each entry contains the appropriate layout + partials + the specific page file
// so that {{define}} blocks from different pages never overwrite each other.
// Dashboard pages use templates/layout/dashboard.html; admin pages use
// templates/layout/admin.html; all others use basePath.
func buildTemplateMap(basePath, pagesRoot string) (map[string]*template.Template, error) {
	funcs := template.FuncMap{
		"formatNumber":   handler.FormatNumber,
		"formatDuration": handler.FormatDuration,
		"slice": func(s string, i, j int) string {
			if i >= len(s) {
				return ""
			}
			if j > len(s) {
				j = len(s)
			}
			return s[i:j]
		},
		"safeHTML": func(s string) template.HTML { return template.HTML(s) }, //nolint:gosec // G203: trusted CMS content only
		"defaultStr": func(def, val string) string {
			if val == "" {
				return def
			}
			return val
		},
		"inc": func(i int) int { return i + 1 },
		"dec": func(i int) int { return i - 1 },
	}

	partials, err := filepath.Glob("templates/partials/*.html")
	if err != nil {
		return nil, fmt.Errorf("buildTemplateMap: glob partials: %w", err)
	}

	tmpls := make(map[string]*template.Template)

	err = filepath.WalkDir(pagesRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".html") {
			return nil
		}
		if strings.Contains(path, "/layout/") || strings.Contains(path, "/partials/") {
			return nil
		}

		layoutPath := basePath
		if strings.Contains(path, "/dashboard/") {
			layoutPath = "templates/layout/dashboard.html"
		} else if strings.Contains(path, "/admin/") {
			layoutPath = "templates/layout/admin.html"
		}

		files := []string{layoutPath}
		files = append(files, partials...)
		files = append(files, path)

		t, err := template.New(filepath.Base(layoutPath)).Funcs(funcs).ParseFiles(files...)
		if err != nil {
			return fmt.Errorf("parse %s: %w", path, err)
		}
		tmpls[filepath.Base(path)] = t
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("buildTemplateMap: %w", err)
	}
	if len(tmpls) == 0 {
		return nil, fmt.Errorf("buildTemplateMap: no page templates found under %s", pagesRoot)
	}
	return tmpls, nil
}
