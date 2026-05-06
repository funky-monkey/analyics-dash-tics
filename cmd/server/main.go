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

	tmpls, err := buildTemplateMap("templates/layout/base.html", "templates")
	if err != nil {
		slog.Error("templates", "error", err)
		os.Exit(1)
	}

	authHandler := handler.NewAuthHandler(authSvc, repos, cfg.BaseURL)
	authHandler.SetTemplates(tmpls)

	sitesHandler := handler.NewSitesHandler(authSvc, repos)
	sitesHandler.SetTemplates(tmpls)

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
		http.Redirect(w, r, "/login", http.StatusTemporaryRedirect)
	})

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
	r.With(collectLimiter).Post("/collect", func(w http.ResponseWriter, r *http.Request) {
		// Placeholder — implemented in Plan 2 (Tracking Pipeline)
		w.WriteHeader(http.StatusAccepted)
	})

	// Authenticated routes
	jwtAuth := middleware.JWTAuth(authSvc)

	r.With(jwtAuth).Group(func(r chi.Router) {
		r.Get("/dashboard", func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprintln(w, "dashboard — coming in Plan 3")
		})
		r.Get("/account/sites/new", sitesHandler.NewSitePage)
		r.Post("/account/sites/new", sitesHandler.CreateSite)
		r.Get("/sites/{siteID}/overview", func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprintf(w, "site overview %s — coming in Plan 3", chi.URLParam(r, "siteID"))
		})
	})

	// Admin routes — role=admin required
	adminOnly := middleware.RequireRole("admin")
	r.With(jwtAuth, adminOnly).Group(func(r chi.Router) {
		r.Get("/admin", func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprintln(w, "admin dashboard — coming in Plan 4")
		})
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
// Each entry contains base.html + the specific page file so that {{define}} blocks
// from different pages never overwrite each other in a shared template set.
func buildTemplateMap(basePath, pagesRoot string) (map[string]*template.Template, error) {
	tmpls := make(map[string]*template.Template)

	err := filepath.WalkDir(pagesRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		// Skip the layout directory — base.html is the layout, not a page.
		if !d.IsDir() && strings.HasSuffix(path, ".html") && path != basePath {
			name := filepath.Base(path)
			t, err := template.ParseFiles(basePath, path)
			if err != nil {
				return fmt.Errorf("parse %s: %w", path, err)
			}
			tmpls[name] = t
		}
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
