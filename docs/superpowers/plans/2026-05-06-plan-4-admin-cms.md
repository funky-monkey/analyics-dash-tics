# Analytics SaaS — Plan 4: Admin + CMS

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:subagent-driven-development` (recommended) or `superpowers:executing-plans` to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the admin panel (user management, site oversight, system stats, audit log) and the CMS (Trix rich-text editor, page/blog creation, publish/draft workflow) — giving the super-admin full control over users and site content.

**Architecture:** Admin routes live under `/admin/*` and are protected by both `JWTAuth` and `RequireRole("admin")` middleware. The CMS uses Trix (loaded via CDN) for rich-text editing; all HTML output is sanitised with `bluemonday` before storage and before render. CMS pages are served publicly at `/blog/:slug` and `/p/:slug`. All admin actions are written to the `audit_log` table.

**Tech Stack:** Go `html/template`, HTMX (CDN), Tailwind (existing), `microcosm-cc/bluemonday` (HTML sanitisation), Trix (CDN), existing `repository.Repos` and `middleware` packages.

> **No Co-Authored-By** in any commit message.
> **Working directory:** `/Users/sidneydekoning/stack/Github/analyics-dash-tics`
> **Module:** `github.com/sidneydekoning/analytics`

---

## File Map

```
internal/
  repository/
    admin.go                     — AdminRepository: user list, audit log write, site list
    cms.go                       — CMSRepository: CRUD for cms_layouts, cms_pages, cms_tags
  handler/
    admin.go                     — AdminHandler: /admin/* routes
    cms.go                       — CMSHandler: /admin/cms/* routes + public /blog/:slug, /p/:slug
    admin_test.go                — Ginkgo BDD tests

templates/
  layout/
    admin.html                   — admin shell (top nav)
  pages/
    admin/
      index.html                 — admin overview (user count, site count, event volume)
      users.html                 — user list with search
      user-form.html             — create/edit user form
      sites.html                 — all sites list
      cms-list.html              — CMS page list
      cms-edit.html              — Trix editor page
      audit.html                 — audit log viewer
    public/
      blog-list.html             — public blog listing
      blog-post.html             — public blog post
      page.html                  — public generic page
```

---

### Task 1: Admin + CMS repositories

**Files:**
- Create: `internal/repository/admin.go`
- Create: `internal/repository/cms.go`
- Modify: `internal/repository/repos.go`

- [ ] **Step 1: Implement `internal/repository/admin.go`**

```go
package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/sidneydekoning/analytics/internal/model"
)

// AdminRepository handles admin-only database operations.
type AdminRepository interface {
	ListAllUsers(ctx context.Context, limit, offset int) ([]*model.User, error)
	CountUsers(ctx context.Context) (int64, error)
	CountSites(ctx context.Context) (int64, error)
	CountEventsToday(ctx context.Context) (int64, error)
	ListAllSites(ctx context.Context, limit, offset int) ([]*model.Site, error)
	WriteAuditLog(ctx context.Context, actorID, action, resourceType, resourceID, ipHash string) error
	ListAuditLog(ctx context.Context, limit, offset int) ([]*AuditEntry, error)
}

// AuditEntry is a single row from the audit_log table.
type AuditEntry struct {
	ID           string
	ActorID      string
	Action       string
	ResourceType string
	ResourceID   string
	IPHash       string
	CreatedAt    time.Time
}

type pgAdminRepository struct {
	pool *pgxpool.Pool
}

func (r *pgAdminRepository) ListAllUsers(ctx context.Context, limit, offset int) ([]*model.User, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, email, role, name, is_active, created_at, last_login_at
		 FROM users ORDER BY created_at DESC LIMIT $1 OFFSET $2`, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("adminRepository.ListAllUsers: %w", err)
	}
	defer rows.Close()
	var users []*model.User
	for rows.Next() {
		u := &model.User{}
		if err := rows.Scan(&u.ID, &u.Email, &u.Role, &u.Name, &u.IsActive, &u.CreatedAt, &u.LastLoginAt); err != nil {
			return nil, fmt.Errorf("adminRepository.ListAllUsers: scan: %w", err)
		}
		users = append(users, u)
	}
	return users, rows.Err()
}

func (r *pgAdminRepository) CountUsers(ctx context.Context) (int64, error) {
	var n int64
	err := r.pool.QueryRow(ctx, `SELECT COUNT(*) FROM users`).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("adminRepository.CountUsers: %w", err)
	}
	return n, nil
}

func (r *pgAdminRepository) CountSites(ctx context.Context) (int64, error) {
	var n int64
	err := r.pool.QueryRow(ctx, `SELECT COUNT(*) FROM sites`).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("adminRepository.CountSites: %w", err)
	}
	return n, nil
}

func (r *pgAdminRepository) CountEventsToday(ctx context.Context) (int64, error) {
	var n int64
	err := r.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM events WHERE timestamp >= CURRENT_DATE`).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("adminRepository.CountEventsToday: %w", err)
	}
	return n, nil
}

func (r *pgAdminRepository) ListAllSites(ctx context.Context, limit, offset int) ([]*model.Site, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, owner_id, name, domain, token, timezone, created_at
		 FROM sites ORDER BY created_at DESC LIMIT $1 OFFSET $2`, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("adminRepository.ListAllSites: %w", err)
	}
	defer rows.Close()
	var sites []*model.Site
	for rows.Next() {
		s := &model.Site{}
		if err := rows.Scan(&s.ID, &s.OwnerID, &s.Name, &s.Domain, &s.Token, &s.Timezone, &s.CreatedAt); err != nil {
			return nil, fmt.Errorf("adminRepository.ListAllSites: scan: %w", err)
		}
		sites = append(sites, s)
	}
	return sites, rows.Err()
}

func (r *pgAdminRepository) WriteAuditLog(ctx context.Context, actorID, action, resourceType, resourceID, ipHash string) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO audit_log (actor_id, action, resource_type, resource_id, ip_hash)
		 VALUES ($1, $2, $3, $4, $5)`,
		actorID, action, resourceType, resourceID, ipHash)
	if err != nil {
		return fmt.Errorf("adminRepository.WriteAuditLog: %w", err)
	}
	return nil
}

func (r *pgAdminRepository) ListAuditLog(ctx context.Context, limit, offset int) ([]*AuditEntry, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, COALESCE(actor_id::text,''), action, resource_type, resource_id, ip_hash, created_at
		 FROM audit_log ORDER BY created_at DESC LIMIT $1 OFFSET $2`, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("adminRepository.ListAuditLog: %w", err)
	}
	defer rows.Close()
	var entries []*AuditEntry
	for rows.Next() {
		e := &AuditEntry{}
		if err := rows.Scan(&e.ID, &e.ActorID, &e.Action, &e.ResourceType, &e.ResourceID, &e.IPHash, &e.CreatedAt); err != nil {
			return nil, fmt.Errorf("adminRepository.ListAuditLog: scan: %w", err)
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}
```

- [ ] **Step 2: Add CMS model types — append to `internal/model/event.go`**

Add to the end of `internal/model/event.go`:

```go
// CMSPage represents a blog post or generic page created via the admin CMS.
type CMSPage struct {
	ID              string
	LayoutID        string
	AuthorID        string
	Title           string
	Slug            string
	Type            string // "blog" or "page"
	ContentHTML     string // bluemonday-sanitised Trix output
	Excerpt         string
	CoverImageURL   string
	MetaTitle       string
	MetaDescription string
	Status          string // "draft" or "published"
	PublishedAt     *time.Time
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// CMSLayout is a named template file that a CMS page uses.
type CMSLayout struct {
	ID           string
	Name         string
	TemplateFile string
	Description  string
}
```

- [ ] **Step 3: Implement `internal/repository/cms.go`**

```go
package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/sidneydekoning/analytics/internal/model"
)

// CMSRepository handles CRUD for CMS layouts and pages.
type CMSRepository interface {
	ListLayouts(ctx context.Context) ([]*model.CMSLayout, error)
	GetLayout(ctx context.Context, id string) (*model.CMSLayout, error)
	CreatePage(ctx context.Context, p *model.CMSPage) error
	UpdatePage(ctx context.Context, p *model.CMSPage) error
	GetPageByID(ctx context.Context, id string) (*model.CMSPage, error)
	GetPageBySlug(ctx context.Context, slug string) (*model.CMSPage, error)
	ListPages(ctx context.Context, limit, offset int) ([]*model.CMSPage, error)
	ListPublishedByType(ctx context.Context, pageType string, limit, offset int) ([]*model.CMSPage, error)
	SetPageStatus(ctx context.Context, id, status string, publishedAt *time.Time) error
}

type pgCMSRepository struct {
	pool *pgxpool.Pool
}

func (r *pgCMSRepository) ListLayouts(ctx context.Context) ([]*model.CMSLayout, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, name, template_file, description FROM cms_layouts ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("cmsRepository.ListLayouts: %w", err)
	}
	defer rows.Close()
	var layouts []*model.CMSLayout
	for rows.Next() {
		l := &model.CMSLayout{}
		if err := rows.Scan(&l.ID, &l.Name, &l.TemplateFile, &l.Description); err != nil {
			return nil, fmt.Errorf("cmsRepository.ListLayouts: scan: %w", err)
		}
		layouts = append(layouts, l)
	}
	return layouts, rows.Err()
}

func (r *pgCMSRepository) GetLayout(ctx context.Context, id string) (*model.CMSLayout, error) {
	l := &model.CMSLayout{}
	err := r.pool.QueryRow(ctx,
		`SELECT id, name, template_file, description FROM cms_layouts WHERE id = $1`, id).
		Scan(&l.ID, &l.Name, &l.TemplateFile, &l.Description)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("cmsRepository.GetLayout: %w", err)
	}
	return l, nil
}

func (r *pgCMSRepository) CreatePage(ctx context.Context, p *model.CMSPage) error {
	err := r.pool.QueryRow(ctx, `
		INSERT INTO cms_pages
			(layout_id, author_id, title, slug, type, content_html, excerpt,
			 cover_image_url, meta_title, meta_description, status)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
		RETURNING id, created_at, updated_at
	`, p.LayoutID, p.AuthorID, p.Title, p.Slug, p.Type, p.ContentHTML, p.Excerpt,
		p.CoverImageURL, p.MetaTitle, p.MetaDescription, p.Status).
		Scan(&p.ID, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		return fmt.Errorf("cmsRepository.CreatePage: %w", err)
	}
	return nil
}

func (r *pgCMSRepository) UpdatePage(ctx context.Context, p *model.CMSPage) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE cms_pages SET
			title=$2, slug=$3, content_html=$4, excerpt=$5,
			cover_image_url=$6, meta_title=$7, meta_description=$8,
			updated_at=NOW()
		WHERE id=$1
	`, p.ID, p.Title, p.Slug, p.ContentHTML, p.Excerpt,
		p.CoverImageURL, p.MetaTitle, p.MetaDescription)
	if err != nil {
		return fmt.Errorf("cmsRepository.UpdatePage: %w", err)
	}
	return nil
}

func (r *pgCMSRepository) GetPageByID(ctx context.Context, id string) (*model.CMSPage, error) {
	return r.scanPage(r.pool.QueryRow(ctx, `
		SELECT id,layout_id,author_id,title,slug,type,content_html,excerpt,
		       cover_image_url,meta_title,meta_description,status,published_at,created_at,updated_at
		FROM cms_pages WHERE id=$1`, id))
}

func (r *pgCMSRepository) GetPageBySlug(ctx context.Context, slug string) (*model.CMSPage, error) {
	return r.scanPage(r.pool.QueryRow(ctx, `
		SELECT id,layout_id,author_id,title,slug,type,content_html,excerpt,
		       cover_image_url,meta_title,meta_description,status,published_at,created_at,updated_at
		FROM cms_pages WHERE slug=$1 AND status='published'`, slug))
}

func (r *pgCMSRepository) scanPage(row pgx.Row) (*model.CMSPage, error) {
	p := &model.CMSPage{}
	err := row.Scan(
		&p.ID, &p.LayoutID, &p.AuthorID, &p.Title, &p.Slug, &p.Type,
		&p.ContentHTML, &p.Excerpt, &p.CoverImageURL, &p.MetaTitle,
		&p.MetaDescription, &p.Status, &p.PublishedAt, &p.CreatedAt, &p.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("cmsRepository.scanPage: %w", err)
	}
	return p, nil
}

func (r *pgCMSRepository) ListPages(ctx context.Context, limit, offset int) ([]*model.CMSPage, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id,layout_id,author_id,title,slug,type,content_html,excerpt,
		       cover_image_url,meta_title,meta_description,status,published_at,created_at,updated_at
		FROM cms_pages ORDER BY created_at DESC LIMIT $1 OFFSET $2`, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("cmsRepository.ListPages: %w", err)
	}
	defer rows.Close()
	return r.scanPages(rows)
}

func (r *pgCMSRepository) ListPublishedByType(ctx context.Context, pageType string, limit, offset int) ([]*model.CMSPage, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id,layout_id,author_id,title,slug,type,content_html,excerpt,
		       cover_image_url,meta_title,meta_description,status,published_at,created_at,updated_at
		FROM cms_pages
		WHERE type=$1 AND status='published'
		ORDER BY published_at DESC LIMIT $2 OFFSET $3`, pageType, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("cmsRepository.ListPublishedByType: %w", err)
	}
	defer rows.Close()
	return r.scanPages(rows)
}

func (r *pgCMSRepository) SetPageStatus(ctx context.Context, id, status string, publishedAt *time.Time) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE cms_pages SET status=$2, published_at=$3, updated_at=NOW() WHERE id=$1`,
		id, status, publishedAt)
	if err != nil {
		return fmt.Errorf("cmsRepository.SetPageStatus: %w", err)
	}
	return nil
}

func (r *pgCMSRepository) scanPages(rows pgx.Rows) ([]*model.CMSPage, error) {
	var pages []*model.CMSPage
	for rows.Next() {
		p := &model.CMSPage{}
		if err := rows.Scan(
			&p.ID, &p.LayoutID, &p.AuthorID, &p.Title, &p.Slug, &p.Type,
			&p.ContentHTML, &p.Excerpt, &p.CoverImageURL, &p.MetaTitle,
			&p.MetaDescription, &p.Status, &p.PublishedAt, &p.CreatedAt, &p.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("cmsRepository.scanPages: scan: %w", err)
		}
		pages = append(pages, p)
	}
	return pages, rows.Err()
}
```

- [ ] **Step 4: Update `internal/repository/repos.go`**

```go
package repository

import "github.com/jackc/pgx/v5/pgxpool"

// Repos aggregates all repository interfaces for dependency injection.
type Repos struct {
	Users  UserRepository
	Sites  SiteRepository
	Events EventRepository
	Stats  StatsRepository
	Admin  AdminRepository
	CMS    CMSRepository
}

// New creates a Repos with all pg implementations wired up.
func New(pool *pgxpool.Pool) *Repos {
	return &Repos{
		Users:  &pgUserRepository{pool: pool},
		Sites:  &pgSiteRepository{pool: pool},
		Events: &pgEventRepository{pool: pool},
		Stats:  &pgStatsRepository{pool: pool},
		Admin:  &pgAdminRepository{pool: pool},
		CMS:    &pgCMSRepository{pool: pool},
	}
}
```

- [ ] **Step 5: Build check**

```bash
go build ./...
```

Expected: clean.

- [ ] **Step 6: Commit**

```bash
git add internal/model/event.go internal/repository/admin.go internal/repository/cms.go internal/repository/repos.go
git commit -m "feat: add admin and CMS repositories"
```

---

### Task 2: Admin layout + page templates

**Files:**
- Create: `templates/layout/admin.html`
- Create: `templates/pages/admin/index.html`
- Create: `templates/pages/admin/users.html`
- Create: `templates/pages/admin/user-form.html`
- Create: `templates/pages/admin/sites.html`
- Create: `templates/pages/admin/cms-list.html`
- Create: `templates/pages/admin/cms-edit.html`
- Create: `templates/pages/admin/audit.html`

- [ ] **Step 1: Create `templates/layout/admin.html`**

```html
<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>{{block "title" .}}Admin — Analytics{{end}}</title>
  <link rel="stylesheet" href="/static/css/output.css">
  <script src="https://unpkg.com/alpinejs@3.x.x/dist/cdn.min.js" defer></script>
  {{block "head" .}}{{end}}
</head>
<body class="bg-gray-50 text-gray-900 antialiased">
  <nav class="bg-gray-900 text-white px-6 py-3 flex items-center gap-6">
    <span class="font-bold text-violet-400 text-sm">ADMIN</span>
    <a href="/admin" class="text-sm text-gray-300 hover:text-white transition-colors {{if eq .ActiveNav "overview"}}text-white font-medium{{end}}">Overview</a>
    <a href="/admin/users" class="text-sm text-gray-300 hover:text-white transition-colors {{if eq .ActiveNav "users"}}text-white font-medium{{end}}">Users</a>
    <a href="/admin/sites" class="text-sm text-gray-300 hover:text-white transition-colors {{if eq .ActiveNav "sites"}}text-white font-medium{{end}}">Sites</a>
    <a href="/admin/cms" class="text-sm text-gray-300 hover:text-white transition-colors {{if eq .ActiveNav "cms"}}text-white font-medium{{end}}">CMS</a>
    <a href="/admin/audit-log" class="text-sm text-gray-300 hover:text-white transition-colors {{if eq .ActiveNav "audit"}}text-white font-medium{{end}}">Audit log</a>
    <div class="flex-1"></div>
    <a href="/dashboard" class="text-xs text-gray-500 hover:text-gray-300">← Dashboard</a>
  </nav>
  <main class="p-8 max-w-6xl mx-auto">
    {{block "content" .}}{{end}}
  </main>
</body>
</html>
```

- [ ] **Step 2: Create `templates/pages/admin/index.html`**

```html
{{template "admin.html" .}}
{{define "title"}}Admin overview{{end}}
{{define "content"}}
<h1 class="text-2xl font-bold text-gray-900 mb-8">System overview</h1>
<div class="grid grid-cols-3 gap-4 mb-8">
  <div class="bg-white rounded-xl border border-gray-100 p-6">
    <p class="text-xs font-medium text-gray-400 uppercase tracking-wide mb-1">Total users</p>
    <p class="text-3xl font-bold text-gray-900">{{.UserCount}}</p>
  </div>
  <div class="bg-white rounded-xl border border-gray-100 p-6">
    <p class="text-xs font-medium text-gray-400 uppercase tracking-wide mb-1">Total sites</p>
    <p class="text-3xl font-bold text-gray-900">{{.SiteCount}}</p>
  </div>
  <div class="bg-white rounded-xl border border-gray-100 p-6">
    <p class="text-xs font-medium text-gray-400 uppercase tracking-wide mb-1">Events today</p>
    <p class="text-3xl font-bold text-gray-900">{{formatNumber .EventsToday}}</p>
  </div>
</div>
{{end}}
```

- [ ] **Step 3: Create `templates/pages/admin/users.html`**

```html
{{template "admin.html" .}}
{{define "title"}}Users — Admin{{end}}
{{define "content"}}
<div class="flex items-center justify-between mb-6">
  <h1 class="text-2xl font-bold text-gray-900">Users</h1>
  <a href="/admin/users/new" class="btn-primary">+ New user</a>
</div>
{{if .Error}}
<div class="mb-4 p-3 bg-red-50 border border-red-200 rounded-lg text-red-700 text-sm">{{.Error}}</div>
{{end}}
<div class="bg-white rounded-xl border border-gray-100 overflow-hidden">
  <table class="w-full text-sm">
    <thead class="bg-gray-50">
      <tr class="text-xs text-gray-400 border-b border-gray-100">
        <th class="text-left px-4 py-3 font-medium">Email</th>
        <th class="text-left px-4 py-3 font-medium">Name</th>
        <th class="text-left px-4 py-3 font-medium">Role</th>
        <th class="text-left px-4 py-3 font-medium">Status</th>
        <th class="text-left px-4 py-3 font-medium">Joined</th>
        <th class="px-4 py-3"></th>
      </tr>
    </thead>
    <tbody>
      {{range .Users}}
      <tr class="border-b border-gray-50 last:border-0 hover:bg-gray-50">
        <td class="px-4 py-3 font-mono text-xs">{{.Email}}</td>
        <td class="px-4 py-3">{{.Name}}</td>
        <td class="px-4 py-3">
          <span class="px-2 py-0.5 text-xs rounded-full {{if eq .Role "admin"}}bg-red-50 text-red-600{{else}}bg-violet-50 text-violet-600{{end}}">{{.Role}}</span>
        </td>
        <td class="px-4 py-3">
          {{if .IsActive}}<span class="text-green-600 text-xs">Active</span>{{else}}<span class="text-gray-400 text-xs">Inactive</span>{{end}}
        </td>
        <td class="px-4 py-3 text-gray-400 text-xs">{{.CreatedAt.Format "2006-01-02"}}</td>
        <td class="px-4 py-3">
          <a href="/admin/users/{{.ID}}" class="text-xs text-violet-600 hover:underline">Edit</a>
        </td>
      </tr>
      {{else}}
      <tr><td colspan="6" class="px-4 py-8 text-center text-gray-400 text-sm">No users found.</td></tr>
      {{end}}
    </tbody>
  </table>
</div>
{{end}}
```

- [ ] **Step 4: Create `templates/pages/admin/user-form.html`**

```html
{{template "admin.html" .}}
{{define "title"}}{{if .User.ID}}Edit user{{else}}New user{{end}} — Admin{{end}}
{{define "content"}}
<h1 class="text-2xl font-bold text-gray-900 mb-8">{{if .User.ID}}Edit user{{else}}Create user{{end}}</h1>
{{if .Error}}
<div class="mb-4 p-3 bg-red-50 border border-red-200 rounded-lg text-red-700 text-sm">{{.Error}}</div>
{{end}}
<div class="bg-white rounded-xl border border-gray-100 p-6 max-w-md">
  <form method="POST" action="{{if .User.ID}}/admin/users/{{.User.ID}}{{else}}/admin/users/new{{end}}" class="space-y-4">
    <input type="hidden" name="_csrf" value="{{.CSRFToken}}">
    <div>
      <label class="block text-sm font-medium text-gray-700 mb-1">Email</label>
      <input type="email" name="email" value="{{.User.Email}}" required class="input">
    </div>
    <div>
      <label class="block text-sm font-medium text-gray-700 mb-1">Name</label>
      <input type="text" name="name" value="{{.User.Name}}" required class="input">
    </div>
    {{if not .User.ID}}
    <div>
      <label class="block text-sm font-medium text-gray-700 mb-1">Password</label>
      <input type="password" name="password" minlength="12" required class="input">
    </div>
    {{end}}
    <div>
      <label class="block text-sm font-medium text-gray-700 mb-1">Role</label>
      <select name="role" class="input">
        <option value="user" {{if eq .User.Role "user"}}selected{{end}}>User</option>
        <option value="admin" {{if eq .User.Role "admin"}}selected{{end}}>Admin</option>
      </select>
    </div>
    {{if .User.ID}}
    <div class="flex items-center gap-2">
      <input type="checkbox" name="is_active" id="is_active" value="true" {{if .User.IsActive}}checked{{end}}>
      <label for="is_active" class="text-sm text-gray-700">Active</label>
    </div>
    {{end}}
    <button type="submit" class="btn-primary">
      {{if .User.ID}}Save changes{{else}}Create user{{end}}
    </button>
  </form>
</div>
{{end}}
```

- [ ] **Step 5: Create `templates/pages/admin/sites.html`**

```html
{{template "admin.html" .}}
{{define "title"}}Sites — Admin{{end}}
{{define "content"}}
<h1 class="text-2xl font-bold text-gray-900 mb-6">All sites</h1>
<div class="bg-white rounded-xl border border-gray-100 overflow-hidden">
  <table class="w-full text-sm">
    <thead class="bg-gray-50">
      <tr class="text-xs text-gray-400 border-b border-gray-100">
        <th class="text-left px-4 py-3 font-medium">Domain</th>
        <th class="text-left px-4 py-3 font-medium">Name</th>
        <th class="text-left px-4 py-3 font-medium">Token</th>
        <th class="text-left px-4 py-3 font-medium">Created</th>
      </tr>
    </thead>
    <tbody>
      {{range .Sites}}
      <tr class="border-b border-gray-50 last:border-0">
        <td class="px-4 py-3 font-medium text-gray-900">{{.Domain}}</td>
        <td class="px-4 py-3 text-gray-600">{{.Name}}</td>
        <td class="px-4 py-3 font-mono text-xs text-gray-400">{{.Token}}</td>
        <td class="px-4 py-3 text-gray-400 text-xs">{{.CreatedAt.Format "2006-01-02"}}</td>
      </tr>
      {{else}}
      <tr><td colspan="4" class="px-4 py-8 text-center text-gray-400 text-sm">No sites.</td></tr>
      {{end}}
    </tbody>
  </table>
</div>
{{end}}
```

- [ ] **Step 6: Create `templates/pages/admin/cms-list.html`**

```html
{{template "admin.html" .}}
{{define "title"}}CMS — Admin{{end}}
{{define "content"}}
<div class="flex items-center justify-between mb-6">
  <h1 class="text-2xl font-bold text-gray-900">Pages & blog posts</h1>
  <a href="/admin/cms/new" class="btn-primary">+ New page</a>
</div>
<div class="bg-white rounded-xl border border-gray-100 overflow-hidden">
  <table class="w-full text-sm">
    <thead class="bg-gray-50">
      <tr class="text-xs text-gray-400 border-b border-gray-100">
        <th class="text-left px-4 py-3 font-medium">Title</th>
        <th class="text-left px-4 py-3 font-medium">Type</th>
        <th class="text-left px-4 py-3 font-medium">Status</th>
        <th class="text-left px-4 py-3 font-medium">Published</th>
        <th class="px-4 py-3"></th>
      </tr>
    </thead>
    <tbody>
      {{range .Pages}}
      <tr class="border-b border-gray-50 last:border-0 hover:bg-gray-50">
        <td class="px-4 py-3 font-medium text-gray-900">{{.Title}}</td>
        <td class="px-4 py-3"><span class="px-2 py-0.5 text-xs rounded-full bg-blue-50 text-blue-600">{{.Type}}</span></td>
        <td class="px-4 py-3">
          {{if eq .Status "published"}}<span class="text-green-600 text-xs font-medium">Published</span>
          {{else}}<span class="text-gray-400 text-xs">Draft</span>{{end}}
        </td>
        <td class="px-4 py-3 text-gray-400 text-xs">
          {{if .PublishedAt}}{{.PublishedAt.Format "2006-01-02"}}{{else}}—{{end}}
        </td>
        <td class="px-4 py-3 flex gap-3">
          <a href="/admin/cms/{{.ID}}/edit" class="text-xs text-violet-600 hover:underline">Edit</a>
          <form method="POST" action="/admin/cms/{{.ID}}/publish" style="display:inline">
            <input type="hidden" name="_csrf" value="{{$.CSRFToken}}">
            <button type="submit" class="text-xs text-gray-400 hover:text-gray-700">
              {{if eq .Status "published"}}Unpublish{{else}}Publish{{end}}
            </button>
          </form>
        </td>
      </tr>
      {{else}}
      <tr><td colspan="5" class="px-4 py-8 text-center text-gray-400 text-sm">No pages yet. Create your first post.</td></tr>
      {{end}}
    </tbody>
  </table>
</div>
{{end}}
```

- [ ] **Step 7: Create `templates/pages/admin/cms-edit.html`**

```html
{{template "admin.html" .}}
{{define "title"}}{{if .Page.ID}}Edit page{{else}}New page{{end}} — Admin{{end}}
{{define "head"}}
<link rel="stylesheet" type="text/css" href="https://unpkg.com/trix@2.0.8/dist/trix.css">
<script type="text/javascript" src="https://unpkg.com/trix@2.0.8/dist/trix.umd.min.js"></script>
{{end}}
{{define "content"}}
<h1 class="text-2xl font-bold text-gray-900 mb-8">{{if .Page.ID}}Edit page{{else}}New page{{end}}</h1>
{{if .Error}}
<div class="mb-4 p-3 bg-red-50 border border-red-200 rounded-lg text-red-700 text-sm">{{.Error}}</div>
{{end}}
<form method="POST" action="{{if .Page.ID}}/admin/cms/{{.Page.ID}}/edit{{else}}/admin/cms/new{{end}}" class="space-y-5">
  <input type="hidden" name="_csrf" value="{{.CSRFToken}}">
  <input type="hidden" name="content_html" id="content_html_input">
  <div class="grid grid-cols-2 gap-4">
    <div>
      <label class="block text-sm font-medium text-gray-700 mb-1">Title</label>
      <input type="text" name="title" value="{{.Page.Title}}" required class="input">
    </div>
    <div>
      <label class="block text-sm font-medium text-gray-700 mb-1">Slug</label>
      <input type="text" name="slug" value="{{.Page.Slug}}" required pattern="[a-z0-9\-]+" class="input">
      <p class="text-xs text-gray-400 mt-1">Lowercase letters, numbers, and hyphens only</p>
    </div>
  </div>
  <div class="grid grid-cols-2 gap-4">
    <div>
      <label class="block text-sm font-medium text-gray-700 mb-1">Type</label>
      <select name="type" class="input">
        <option value="blog" {{if eq .Page.Type "blog"}}selected{{end}}>Blog post</option>
        <option value="page" {{if eq .Page.Type "page"}}selected{{end}}>Page</option>
      </select>
    </div>
    <div>
      <label class="block text-sm font-medium text-gray-700 mb-1">Cover image URL</label>
      <input type="url" name="cover_image_url" value="{{.Page.CoverImageURL}}" class="input" placeholder="https://...">
    </div>
  </div>
  <div>
    <label class="block text-sm font-medium text-gray-700 mb-1">Excerpt</label>
    <textarea name="excerpt" class="input" rows="2">{{.Page.Excerpt}}</textarea>
  </div>
  <div>
    <label class="block text-sm font-medium text-gray-700 mb-2">Content</label>
    <input id="trix_content" type="hidden" name="trix_content" value="{{.Page.ContentHTML}}">
    <trix-editor input="trix_content" class="rounded-lg border border-gray-200 min-h-64 p-3 text-sm"></trix-editor>
  </div>
  <div class="grid grid-cols-2 gap-4">
    <div>
      <label class="block text-sm font-medium text-gray-700 mb-1">Meta title</label>
      <input type="text" name="meta_title" value="{{.Page.MetaTitle}}" class="input">
    </div>
    <div>
      <label class="block text-sm font-medium text-gray-700 mb-1">Meta description</label>
      <input type="text" name="meta_description" value="{{.Page.MetaDescription}}" class="input">
    </div>
  </div>
  <div class="flex gap-3 pt-2">
    <button type="submit" class="btn-primary">Save</button>
    <a href="/admin/cms" class="px-4 py-2 text-sm text-gray-500 hover:text-gray-700">Cancel</a>
  </div>
</form>
<script>
  // Copy Trix content to hidden field before form submit
  document.querySelector('form').addEventListener('submit', function() {
    var trix = document.querySelector('trix-editor');
    document.getElementById('content_html_input').value = trix ? trix.innerHTML : '';
  });
</script>
{{end}}
```

- [ ] **Step 8: Create `templates/pages/admin/audit.html`**

```html
{{template "admin.html" .}}
{{define "title"}}Audit log — Admin{{end}}
{{define "content"}}
<h1 class="text-2xl font-bold text-gray-900 mb-6">Audit log</h1>
<div class="bg-white rounded-xl border border-gray-100 overflow-hidden">
  <table class="w-full text-sm">
    <thead class="bg-gray-50">
      <tr class="text-xs text-gray-400 border-b border-gray-100">
        <th class="text-left px-4 py-3 font-medium">Time</th>
        <th class="text-left px-4 py-3 font-medium">Action</th>
        <th class="text-left px-4 py-3 font-medium">Resource</th>
        <th class="text-left px-4 py-3 font-medium">Actor</th>
      </tr>
    </thead>
    <tbody>
      {{range .Entries}}
      <tr class="border-b border-gray-50 last:border-0">
        <td class="px-4 py-3 text-xs text-gray-400">{{.CreatedAt.Format "2006-01-02 15:04"}}</td>
        <td class="px-4 py-3 font-medium">{{.Action}}</td>
        <td class="px-4 py-3 text-gray-500 text-xs">{{.ResourceType}} {{.ResourceID}}</td>
        <td class="px-4 py-3 text-xs font-mono text-gray-400">{{slice .ActorID 0 8}}…</td>
      </tr>
      {{else}}
      <tr><td colspan="4" class="px-4 py-8 text-center text-gray-400 text-sm">No audit entries.</td></tr>
      {{end}}
    </tbody>
  </table>
</div>
{{end}}
```

- [ ] **Step 9: Compile check**

```bash
go build ./...
```

- [ ] **Step 10: Commit**

```bash
git add templates/layout/admin.html templates/pages/admin/
git commit -m "feat: add admin layout and all admin page templates"
```

---

### Task 3: Public CMS templates

**Files:**
- Create: `templates/pages/public/blog-list.html`
- Create: `templates/pages/public/blog-post.html`
- Create: `templates/pages/public/page.html`

- [ ] **Step 1: Create `templates/pages/public/blog-list.html`**

```html
{{template "base.html" .}}
{{define "title"}}Blog — Analytics{{end}}
{{define "content"}}
<div class="max-w-3xl mx-auto px-4 py-16">
  <h1 class="text-3xl font-bold text-gray-900 mb-12">Blog</h1>
  {{if .Posts}}
  <div class="space-y-8">
    {{range .Posts}}
    <article class="border-b border-gray-100 pb-8 last:border-0">
      <a href="/blog/{{.Slug}}" class="group">
        <h2 class="text-xl font-semibold text-gray-900 group-hover:text-violet-600 transition-colors mb-2">{{.Title}}</h2>
      </a>
      {{if .Excerpt}}<p class="text-gray-500 text-sm leading-relaxed">{{.Excerpt}}</p>{{end}}
      <div class="mt-3 flex items-center gap-4">
        {{if .PublishedAt}}<time class="text-xs text-gray-400">{{.PublishedAt.Format "January 2, 2006"}}</time>{{end}}
        <a href="/blog/{{.Slug}}" class="text-xs text-violet-600 hover:underline">Read →</a>
      </div>
    </article>
    {{end}}
  </div>
  {{else}}
  <p class="text-gray-400">No posts yet.</p>
  {{end}}
</div>
{{end}}
```

- [ ] **Step 2: Create `templates/pages/public/blog-post.html`**

```html
{{template "base.html" .}}
{{define "title"}}{{.Page.MetaTitle | default .Page.Title}}{{end}}
{{define "content"}}
<article class="max-w-3xl mx-auto px-4 py-16">
  <header class="mb-10">
    {{if .Page.PublishedAt}}
    <time class="text-xs text-gray-400 block mb-3">{{.Page.PublishedAt.Format "January 2, 2006"}}</time>
    {{end}}
    <h1 class="text-3xl font-bold text-gray-900 mb-4">{{.Page.Title}}</h1>
    {{if .Page.Excerpt}}<p class="text-lg text-gray-500">{{.Page.Excerpt}}</p>{{end}}
  </header>
  {{if .Page.CoverImageURL}}
  <img src="{{.Page.CoverImageURL}}" alt="{{.Page.Title}}" class="w-full rounded-xl mb-10">
  {{end}}
  <div class="prose prose-sm max-w-none text-gray-700 leading-relaxed">
    {{.Page.ContentHTML | safeHTML}}
  </div>
</article>
{{end}}
```

- [ ] **Step 3: Create `templates/pages/public/page.html`**

```html
{{template "base.html" .}}
{{define "title"}}{{.Page.MetaTitle | default .Page.Title}}{{end}}
{{define "content"}}
<div class="max-w-3xl mx-auto px-4 py-16">
  <h1 class="text-3xl font-bold text-gray-900 mb-8">{{.Page.Title}}</h1>
  <div class="prose prose-sm max-w-none text-gray-700 leading-relaxed">
    {{.Page.ContentHTML | safeHTML}}
  </div>
</div>
{{end}}
```

- [ ] **Step 4: Compile check**

```bash
go build ./...
```

- [ ] **Step 5: Commit**

```bash
git add templates/pages/public/
git commit -m "feat: add public blog and CMS page templates"
```

---

### Task 4: Admin + CMS handlers and wiring

**Files:**
- Create: `internal/handler/admin.go`
- Create: `internal/handler/cms.go`
- Create: `internal/handler/admin_test.go`
- Modify: `cmd/server/main.go`

- [ ] **Step 1: Write failing Ginkgo tests — `internal/handler/admin_test.go`**

```go
package handler_test

import (
	"net/http"
	"net/http/httptest"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/sidneydekoning/analytics/internal/handler"
	"github.com/sidneydekoning/analytics/internal/middleware"
	"github.com/sidneydekoning/analytics/internal/service"
)

var _ = Describe("AdminHandler", func() {
	var h *handler.AdminHandler

	BeforeEach(func() {
		authSvc := service.NewAuth(
			[]byte("test-access-secret-32-bytes-xxxxx"),
			[]byte("test-refresh-secret-32-bytes-xxxx"),
		)
		h = handler.NewAdminHandler(authSvc, nil)
	})

	Describe("GET /admin", func() {
		Context("with nil repos", func() {
			It("returns 200", func() {
				req := httptest.NewRequest(http.MethodGet, "/admin", nil)
				ctx := middleware.WithUserID(req.Context(), "admin-user-123")
				ctx = middleware.WithRole(ctx, "admin")
				req = req.WithContext(ctx)
				rec := httptest.NewRecorder()

				h.Index(rec, req)

				Expect(rec.Code).To(Equal(http.StatusOK))
			})
		})
	})

	Describe("GET /admin/users", func() {
		Context("with nil repos", func() {
			It("returns 200", func() {
				req := httptest.NewRequest(http.MethodGet, "/admin/users", nil)
				ctx := middleware.WithUserID(req.Context(), "admin-user-123")
				req = req.WithContext(ctx)
				rec := httptest.NewRecorder()

				h.Users(rec, req)

				Expect(rec.Code).To(Equal(http.StatusOK))
			})
		})
	})
})
```

Run: `go test ./internal/handler/... -v` — expect compile error.

- [ ] **Step 2: Implement `internal/handler/admin.go`**

```go
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
	data := map[string]any{"ActiveNav": "overview", "UserCount": int64(0), "SiteCount": int64(0), "EventsToday": int64(0)}
	if h.repos != nil {
		data["UserCount"], _ = h.repos.Admin.CountUsers(r.Context())
		data["SiteCount"], _ = h.repos.Admin.CountSites(r.Context())
		data["EventsToday"], _ = h.repos.Admin.CountEventsToday(r.Context())
	}
	h.renderAdmin(w, "index.html", data)
}

// Users renders GET /admin/users.
func (h *AdminHandler) Users(w http.ResponseWriter, r *http.Request) {
	data := map[string]any{"ActiveNav": "users", "Users": []*model.User{}}
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
	data := map[string]any{
		"ActiveNav": "users",
		"User":      &model.User{},
		"CSRFToken": csrfToken(r),
	}
	h.renderAdmin(w, "user-form.html", data)
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
		Email:        email,
		PasswordHash: hash,
		Role:         model.Role(role),
		Name:         name,
		IsActive:     true,
	}
	if err := h.repos.Users.Create(r.Context(), user); err != nil {
		h.renderAdmin(w, "user-form.html", map[string]any{
			"ActiveNav": "users",
			"User":      user,
			"CSRFToken": csrfToken(r),
			"Error":     "Could not create user. Email may already exist.",
		})
		return
	}

	actorID := middleware.UserIDFromContext(r.Context())
	_ = h.repos.Admin.WriteAuditLog(r.Context(), actorID, "create_user", "user", user.ID, "")
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
		"ActiveNav": "users",
		"User":      user,
		"CSRFToken": csrfToken(r),
	})
}

// UpdateUser handles PUT /admin/users/:id.
func (h *AdminHandler) UpdateUser(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	role := r.FormValue("role")
	if role != "admin" && role != "user" {
		role = "user"
	}
	active := r.FormValue("is_active") == "true"

	if err := h.repos.Users.SetActive(r.Context(), id, active); err != nil {
		slog.Error("admin.UpdateUser: set active", "error", err)
	}

	actorID := middleware.UserIDFromContext(r.Context())
	_ = h.repos.Admin.WriteAuditLog(r.Context(), actorID, "update_user", "user", id, "")
	http.Redirect(w, r, "/admin/users", http.StatusSeeOther)
}

// Sites renders GET /admin/sites.
func (h *AdminHandler) Sites(w http.ResponseWriter, r *http.Request) {
	data := map[string]any{"ActiveNav": "sites", "Sites": []*model.Site{}}
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

func csrfToken(r *http.Request) string {
	if c, err := r.Cookie("csrf_token"); err == nil {
		return c.Value
	}
	return ""
}
```

- [ ] **Step 3: Implement `internal/handler/cms.go`**

```go
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
	// slugPattern validates slugs — only lowercase letters, numbers, and hyphens.
	slugPattern = regexp.MustCompile(`^[a-z0-9-]+$`)
	// policy is the bluemonday allow-list for CMS HTML content.
	policy = bluemonday.UGCPolicy()
)

// CMSHandler handles /admin/cms/* (admin CMS editing) and public /blog/:slug, /p/:slug routes.
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
	rawHTML := r.FormValue("content_html")
	excerpt := strings.TrimSpace(r.FormValue("excerpt"))
	coverURL := strings.TrimSpace(r.FormValue("cover_image_url"))
	metaTitle := strings.TrimSpace(r.FormValue("meta_title"))
	metaDesc := strings.TrimSpace(r.FormValue("meta_description"))

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

	// Sanitise HTML before storage — defence against stored XSS
	cleanHTML := policy.Sanitize(rawHTML)

	// Use a default layout ID — for V1 there's one layout; in V2 a picker can be added
	defaultLayoutID := "00000000-0000-0000-0000-000000000001"

	page := &model.CMSPage{
		LayoutID:        defaultLayoutID,
		AuthorID:        middleware.UserIDFromContext(r.Context()),
		Title:           title,
		Slug:            slug,
		Type:            pageType,
		ContentHTML:     cleanHTML,
		Excerpt:         excerpt,
		CoverImageURL:   coverURL,
		MetaTitle:       metaTitle,
		MetaDescription: metaDesc,
		Status:          "draft",
	}

	if err := h.repos.CMS.CreatePage(r.Context(), page); err != nil {
		h.renderAdmin(w, "cms-edit.html", map[string]any{
			"ActiveNav": "cms", "CSRFToken": csrfToken(r),
			"Page": page, "Error": "Could not save page. Slug may already exist.",
		})
		return
	}

	actorID := middleware.UserIDFromContext(r.Context())
	_ = h.repos.Admin.WriteAuditLog(r.Context(), actorID, "create_page", "cms_page", page.ID, "")
	http.Redirect(w, r, "/admin/cms", http.StatusSeeOther)
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

// UpdatePage handles PUT /admin/cms/:id/edit.
func (h *CMSHandler) UpdatePage(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	title := strings.TrimSpace(r.FormValue("title"))
	slug := strings.TrimSpace(r.FormValue("slug"))
	rawHTML := r.FormValue("content_html")

	if title == "" || slug == "" {
		http.Error(w, "title and slug required", http.StatusUnprocessableEntity)
		return
	}
	if !slugPattern.MatchString(slug) {
		http.Error(w, "invalid slug", http.StatusUnprocessableEntity)
		return
	}

	// Sanitise again on update — defence-in-depth
	cleanHTML := policy.Sanitize(rawHTML)

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

	actorID := middleware.UserIDFromContext(r.Context())
	_ = h.repos.Admin.WriteAuditLog(r.Context(), actorID, "update_page", "cms_page", id, "")
	http.Redirect(w, r, "/admin/cms", http.StatusSeeOther)
}

// TogglePublish handles POST /admin/cms/:id/publish — toggles draft/published.
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
	_ = h.repos.Admin.WriteAuditLog(r.Context(), actorID, "set_page_status:"+newStatus, "cms_page", id, "")
	http.Redirect(w, r, "/admin/cms", http.StatusSeeOther)
}

// BlogList renders GET /blog — public blog listing.
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
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := t.ExecuteTemplate(w, "base.html", data); err != nil {
		slog.Error("render cms public template", "name", name, "error", err)
	}
}
```

**Note:** Fix the typo: in `renderPublic` replace `http.NotFound(w, r)` with `http.Error(w, "not found", http.StatusNotFound)` since `r` is not in scope.

- [ ] **Step 4: Run handler tests**

```bash
go test ./internal/handler/... -v
```

Expected: all specs pass (12 existing + 2 new admin specs = 14 total).

- [ ] **Step 5: Update `cmd/server/main.go`**

**a. Update `buildTemplateMap` to handle admin pages using `admin.html` layout:**

In the walk function, add this condition alongside the dashboard check:

```go
// Admin pages use the admin layout
if strings.Contains(path, "/admin/") {
    layoutPath = "templates/layout/admin.html"
}
```

**b. Add `safeHTML` to the FuncMap (for CMS public page rendering):**

```go
"safeHTML": func(s string) template.HTML { return template.HTML(s) },
"default": func(def, val string) string {
    if val == "" {
        return def
    }
    return val
},
```

**c. Initialise handlers after `dashHandler`:**

```go
adminHandler := handler.NewAdminHandler(authSvc, repos)
adminHandler.SetTemplates(tmpls)

cmsHandler := handler.NewCMSHandler(authSvc, repos)
cmsHandler.SetTemplates(tmpls)
```

**d. Add public CMS routes (before auth routes):**

```go
r.Get("/blog", cmsHandler.BlogList)
r.Get("/blog/{slug}", cmsHandler.BlogPost)
r.Get("/p/{slug}", cmsHandler.GenericPage)
```

**e. Replace the admin placeholder with full admin routes:**

```go
r.With(jwtAuth, adminOnly).Group(func(r chi.Router) {
    r.Get("/admin", adminHandler.Index)
    r.Get("/admin/users", adminHandler.Users)
    r.Get("/admin/users/new", adminHandler.NewUserPage)
    r.Post("/admin/users/new", adminHandler.CreateUser)
    r.Get("/admin/users/{id}", adminHandler.EditUserPage)
    r.Post("/admin/users/{id}", adminHandler.UpdateUser)
    r.Get("/admin/sites", adminHandler.Sites)
    r.Get("/admin/cms", cmsHandler.CMSList)
    r.Get("/admin/cms/new", cmsHandler.NewPageForm)
    r.Post("/admin/cms/new", cmsHandler.CreatePage)
    r.Get("/admin/cms/{id}/edit", cmsHandler.EditPageForm)
    r.Post("/admin/cms/{id}/edit", cmsHandler.UpdatePage)
    r.Post("/admin/cms/{id}/publish", cmsHandler.TogglePublish)
    r.Get("/admin/audit-log", adminHandler.AuditLog)
})
```

- [ ] **Step 6: Add a default CMS layout to the database**

The `CreatePage` handler uses a hardcoded default layout ID. Insert the default layout row via a migration:

Create `internal/repository/migrations/008_default_cms_layout.sql`:

```sql
-- 008_default_cms_layout.sql
INSERT INTO cms_layouts (id, name, template_file, description)
VALUES (
    '00000000-0000-0000-0000-000000000001',
    'Default',
    'blog-post.html',
    'Default layout for blog posts and pages'
) ON CONFLICT DO NOTHING;
```

- [ ] **Step 7: Build, test, rebuild CSS**

```bash
go build -o bin/analytics ./cmd/server
go test -race ./...
./bin/tailwindcss -i static/css/input.css -o static/css/output.css --minify
```

Expected: clean build, all tests pass.

- [ ] **Step 8: Smoke test**

```bash
pkill -f bin/analytics 2>/dev/null; sleep 1
DATABASE_URL="postgres://sidneydekoning@localhost:5432/analytics?sslmode=disable" \
JWT_SECRET="55b8fa86529f04fbf54de43cfa221b57795b63166c6cab23881ee9693698ff91" \
JWT_REFRESH_SECRET="73c246e9baeb07f098c8b9c1a5d98e53fcd7d19defaa9af76f39cb0c1c90d03c" \
BASE_URL="https://dash.local" PORT="8090" ENV="development" \
./bin/analytics &
sleep 2
curl -sk https://dash.local/login | grep '<title>'
curl -sk https://dash.local/blog | grep '<title>'
```

Expected: both return valid HTML with correct titles.

- [ ] **Step 9: Commit and push**

```bash
git add internal/handler/admin.go internal/handler/cms.go \
        internal/handler/admin_test.go \
        internal/repository/migrations/008_default_cms_layout.sql \
        static/css/output.css \
        cmd/server/main.go
git commit -m "feat: add admin panel (users, sites, audit log) and CMS with Trix editor"
git push origin main
```

---

## Self-Review

**Spec coverage check:**

| Requirement | Task |
|---|---|
| Admin panel: user list | Tasks 2+4 (users.html + AdminHandler.Users) |
| Admin panel: create user (admin-only path) | Tasks 2+4 (user-form.html + CreateUser) |
| Admin panel: edit user (role, active) | Tasks 2+4 (user-form.html + UpdateUser) |
| Admin panel: all sites | Tasks 2+4 (sites.html + Sites) |
| Admin panel: system overview (counts) | Tasks 2+4 (index.html + Index) |
| Admin panel: audit log viewer | Tasks 2+4 (audit.html + AuditLog) |
| Audit logging on all admin actions | Task 4 (WriteAuditLog in all mutating handlers) |
| CMS: page list | Tasks 2+4 (cms-list.html + CMSList) |
| CMS: create page with Trix editor | Tasks 2+4 (cms-edit.html + CreatePage) |
| CMS: edit page | Tasks 2+4 (EditPageForm + UpdatePage) |
| CMS: publish/unpublish | Tasks 2+4 (TogglePublish) |
| CMS: bluemonday sanitisation before storage | Task 4 (policy.Sanitize in CreatePage + UpdatePage) |
| CMS: slug validation (allowlist pattern) | Task 4 (slugPattern regex check) |
| Public blog listing: /blog | Tasks 3+4 (blog-list.html + BlogList) |
| Public blog post: /blog/:slug | Tasks 3+4 (blog-post.html + BlogPost) |
| Public generic page: /p/:slug | Tasks 3+4 (page.html + GenericPage) |
| Default CMS layout in DB | Task 4 (migration 008) |
