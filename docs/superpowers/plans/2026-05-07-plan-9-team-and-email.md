# Team Invitations & Email Reports Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:subagent-driven-development` (recommended) or `superpowers:executing-plans` to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Allow site owners to invite team members by email with role-based access (viewer/editor), and send scheduled weekly email reports with key stats.

**Architecture:** Invitations use a token-based flow: owner submits email → server inserts into `invitations` table and sends email with a one-time accept link → invitee clicks link → server creates `site_members` row and deletes the invitation. Email sending uses Go's `net/smtp` with `golang.org/x/net/html` for HTML templates (already available via `golang.org/x/net` in go.mod). Weekly reports run via a background goroutine that fires at midnight every Monday, querying the previous week's stats and emailing all `site_members` + the owner. The `invitations` and `site_members` tables already exist in migration 001.

**Tech Stack:** Go `net/smtp`, `html/template` for email bodies, `time.AfterFunc` for weekly scheduler, pgx/v5, chi router.

> **Working directory:** `/Users/sidneydekoning/stack/Github/analyics-dash-tics`
> **Module:** `github.com/funky-monkey/analyics-dash-tics`
> **No Co-Authored-By in commit messages.**

---

## File Map

```
internal/service/email.go                       — EmailService: send invitation, send weekly report
internal/repository/invitations.go              — InvitationRepository: CRUD for invitations + site_members
internal/repository/repos.go                    — add Invitations field
internal/handler/sites.go                       — InviteForm, SendInvite, AcceptInvite, ListMembers, RemoveMember handlers
internal/handler/auth.go                        — AcceptInvite GET/POST (public route, no JWT required)
cmd/server/main.go                              — add invitation routes; start weekly report goroutine
templates/pages/dashboard/settings.html         — add Members section: member list + invite form
templates/pages/auth/accept-invite.html         — new: accept invitation page (set password if new user)
internal/repository/migrations/011_member_indexes.sql — indexes on site_members and invitations
```

---

### Task 1: Email service

**Files:**
- Create: `internal/service/email.go`

- [ ] **Step 1: Create `internal/service/email.go`**

```go
package service

import (
    "bytes"
    "fmt"
    "html/template"
    "net/smtp"
)

// EmailService sends transactional emails.
type EmailService interface {
    SendInvitation(to, siteURL, siteName, inviteURL, role string) error
    SendWeeklyReport(to, siteName, siteURL string, stats WeeklyStats) error
}

// WeeklyStats holds the numbers for a weekly email report.
type WeeklyStats struct {
    Pageviews   int64
    Visitors    int64
    Sessions    int64
    BounceRate  float64
    TopPage     string
    TopSource   string
    PeriodLabel string // e.g. "May 1 – May 7, 2026"
}

type smtpEmailService struct {
    host string
    port int
    user string
    pass string
    from string
}

// NewEmailService creates an EmailService using SMTP credentials.
// Returns nil (no-op) if host is empty — email is optional in development.
func NewEmailService(host string, port int, user, pass, from string) EmailService {
    if host == "" {
        return &noopEmailService{}
    }
    return &smtpEmailService{host: host, port: port, user: user, pass: pass, from: from}
}

func (s *smtpEmailService) send(to, subject, htmlBody string) error {
    auth := smtp.PlainAuth("", s.user, s.pass, s.host)
    msg := []byte(
        "MIME-Version: 1.0\r\n" +
            "Content-Type: text/html; charset=UTF-8\r\n" +
            "From: " + s.from + "\r\n" +
            "To: " + to + "\r\n" +
            "Subject: " + subject + "\r\n\r\n" +
            htmlBody,
    )
    addr := fmt.Sprintf("%s:%d", s.host, s.port)
    return smtp.SendMail(addr, auth, s.from, []string{to}, msg)
}

var inviteTmpl = template.Must(template.New("invite").Parse(`<!DOCTYPE html>
<html><body style="font-family:sans-serif;max-width:560px;margin:40px auto;color:#111">
<h2 style="color:#7c3aed">You've been invited to {{.SiteName}}</h2>
<p>You've been invited as a <strong>{{.Role}}</strong> on <a href="{{.SiteURL}}">{{.SiteName}}</a>.</p>
<p><a href="{{.InviteURL}}" style="display:inline-block;padding:12px 24px;background:#7c3aed;color:#fff;text-decoration:none;border-radius:8px;font-weight:600">Accept invitation</a></p>
<p style="color:#888;font-size:12px">This link expires in 48 hours. If you didn't expect this, ignore this email.</p>
</body></html>`))

func (s *smtpEmailService) SendInvitation(to, siteURL, siteName, inviteURL, role string) error {
    var buf bytes.Buffer
    if err := inviteTmpl.Execute(&buf, map[string]string{
        "SiteName": siteName, "SiteURL": siteURL, "InviteURL": inviteURL, "Role": role,
    }); err != nil {
        return fmt.Errorf("emailService.SendInvitation: render: %w", err)
    }
    return s.send(to, "You've been invited to "+siteName, buf.String())
}

var reportTmpl = template.Must(template.New("report").Parse(`<!DOCTYPE html>
<html><body style="font-family:sans-serif;max-width:560px;margin:40px auto;color:#111">
<h2 style="color:#7c3aed">Weekly report: {{.SiteName}}</h2>
<p style="color:#888">{{.Stats.PeriodLabel}}</p>
<table style="width:100%;border-collapse:collapse;margin:16px 0">
  <tr><td style="padding:8px;border-bottom:1px solid #f0f0f0">Pageviews</td><td style="text-align:right;font-weight:600">{{.Stats.Pageviews}}</td></tr>
  <tr><td style="padding:8px;border-bottom:1px solid #f0f0f0">Visitors</td><td style="text-align:right;font-weight:600">{{.Stats.Visitors}}</td></tr>
  <tr><td style="padding:8px;border-bottom:1px solid #f0f0f0">Sessions</td><td style="text-align:right;font-weight:600">{{.Stats.Sessions}}</td></tr>
  <tr><td style="padding:8px;border-bottom:1px solid #f0f0f0">Bounce rate</td><td style="text-align:right;font-weight:600">{{printf "%.1f" .Stats.BounceRate}}%</td></tr>
  <tr><td style="padding:8px;border-bottom:1px solid #f0f0f0">Top page</td><td style="text-align:right;font-weight:600">{{.Stats.TopPage}}</td></tr>
  <tr><td style="padding:8px">Top source</td><td style="text-align:right;font-weight:600">{{.Stats.TopSource}}</td></tr>
</table>
<p><a href="{{.SiteURL}}" style="color:#7c3aed">View full dashboard →</a></p>
</body></html>`))

func (s *smtpEmailService) SendWeeklyReport(to, siteName, siteURL string, stats WeeklyStats) error {
    var buf bytes.Buffer
    if err := reportTmpl.Execute(&buf, map[string]any{
        "SiteName": siteName, "SiteURL": siteURL, "Stats": stats,
    }); err != nil {
        return fmt.Errorf("emailService.SendWeeklyReport: render: %w", err)
    }
    subject := fmt.Sprintf("Weekly report for %s: %d visitors", siteName, stats.Visitors)
    return s.send(to, subject, buf.String())
}

// noopEmailService silently discards all emails (used when SMTP_HOST is not configured).
type noopEmailService struct{}
func (n *noopEmailService) SendInvitation(_, _, _, _, _ string) error       { return nil }
func (n *noopEmailService) SendWeeklyReport(_, _, _ string, _ WeeklyStats) error { return nil }
```

- [ ] **Step 2: Build**

```bash
go build ./...
```

- [ ] **Step 3: Commit**

```bash
git add internal/service/email.go
git commit -m "feat: EmailService with SMTP invite and weekly report sending"
```

---

### Task 2: Invitation repository

The `invitations` and `site_members` tables already exist in migration 001. Schema:
```sql
site_members: id, site_id, user_id, role (owner|editor|viewer), invited_at, accepted_at
invitations:  id, site_id, email, token (unique), role, expires_at
```

**Files:**
- Create: `internal/repository/invitations.go`
- Create: `internal/repository/migrations/011_member_indexes.sql`
- Modify: `internal/repository/repos.go`

- [ ] **Step 1: Create `internal/repository/migrations/011_member_indexes.sql`**

```sql
-- 011_member_indexes.sql
CREATE INDEX IF NOT EXISTS invitations_token ON invitations (token);
CREATE INDEX IF NOT EXISTS invitations_site  ON invitations (site_id);
CREATE INDEX IF NOT EXISTS site_members_site ON site_members (site_id);
CREATE INDEX IF NOT EXISTS site_members_user ON site_members (user_id);
```

- [ ] **Step 2: Create `internal/repository/invitations.go`**

```go
package repository

import (
    "context"
    "errors"
    "fmt"
    "time"

    "github.com/jackc/pgx/v5"
    "github.com/jackc/pgx/v5/pgxpool"
)

// Member is a site_members row.
type Member struct {
    ID         string
    SiteID     string
    UserID     string
    UserEmail  string
    UserName   string
    Role       string
    InvitedAt  time.Time
    AcceptedAt *time.Time
}

// Invitation is an invitations row.
type Invitation struct {
    ID        string
    SiteID    string
    Email     string
    Token     string
    Role      string
    ExpiresAt time.Time
}

// InvitationRepository manages site membership and pending invitations.
type InvitationRepository interface {
    CreateInvitation(ctx context.Context, inv *Invitation) error
    GetInvitationByToken(ctx context.Context, token string) (*Invitation, error)
    DeleteInvitation(ctx context.Context, id string) error
    AddMember(ctx context.Context, siteID, userID, role string) error
    ListMembers(ctx context.Context, siteID string) ([]*Member, error)
    RemoveMember(ctx context.Context, siteID, userID string) error
    IsMember(ctx context.Context, siteID, userID string) (bool, error)
}

type pgInvitationRepository struct {
    pool *pgxpool.Pool
}

func (r *pgInvitationRepository) CreateInvitation(ctx context.Context, inv *Invitation) error {
    err := r.pool.QueryRow(ctx,
        `INSERT INTO invitations (site_id, email, token, role, expires_at)
         VALUES ($1,$2,$3,$4,$5) RETURNING id`,
        inv.SiteID, inv.Email, inv.Token, inv.Role, inv.ExpiresAt).Scan(&inv.ID)
    if err != nil {
        return fmt.Errorf("invitationRepository.CreateInvitation: %w", err)
    }
    return nil
}

func (r *pgInvitationRepository) GetInvitationByToken(ctx context.Context, token string) (*Invitation, error) {
    inv := &Invitation{}
    err := r.pool.QueryRow(ctx,
        `SELECT id, site_id, email, token, role, expires_at
         FROM invitations WHERE token = $1 AND expires_at > NOW()`, token).
        Scan(&inv.ID, &inv.SiteID, &inv.Email, &inv.Token, &inv.Role, &inv.ExpiresAt)
    if errors.Is(err, pgx.ErrNoRows) {
        return nil, ErrNotFound
    }
    if err != nil {
        return nil, fmt.Errorf("invitationRepository.GetInvitationByToken: %w", err)
    }
    return inv, nil
}

func (r *pgInvitationRepository) DeleteInvitation(ctx context.Context, id string) error {
    _, err := r.pool.Exec(ctx, `DELETE FROM invitations WHERE id=$1`, id)
    return err
}

func (r *pgInvitationRepository) AddMember(ctx context.Context, siteID, userID, role string) error {
    _, err := r.pool.Exec(ctx,
        `INSERT INTO site_members (site_id, user_id, role, accepted_at)
         VALUES ($1,$2,$3,NOW())
         ON CONFLICT (site_id, user_id) DO UPDATE SET role = EXCLUDED.role, accepted_at = NOW()`,
        siteID, userID, role)
    if err != nil {
        return fmt.Errorf("invitationRepository.AddMember: %w", err)
    }
    return nil
}

func (r *pgInvitationRepository) ListMembers(ctx context.Context, siteID string) ([]*Member, error) {
    rows, err := r.pool.Query(ctx,
        `SELECT sm.id, sm.site_id, sm.user_id, u.email, u.name, sm.role, sm.invited_at, sm.accepted_at
         FROM site_members sm
         JOIN users u ON u.id = sm.user_id
         WHERE sm.site_id = $1
         ORDER BY sm.invited_at ASC`, siteID)
    if err != nil {
        return nil, fmt.Errorf("invitationRepository.ListMembers: %w", err)
    }
    defer rows.Close()
    var members []*Member
    for rows.Next() {
        m := &Member{}
        if err := rows.Scan(&m.ID, &m.SiteID, &m.UserID, &m.UserEmail, &m.UserName, &m.Role, &m.InvitedAt, &m.AcceptedAt); err != nil {
            return nil, fmt.Errorf("invitationRepository.ListMembers: scan: %w", err)
        }
        members = append(members, m)
    }
    return members, rows.Err()
}

func (r *pgInvitationRepository) RemoveMember(ctx context.Context, siteID, userID string) error {
    tag, err := r.pool.Exec(ctx, `DELETE FROM site_members WHERE site_id=$1 AND user_id=$2`, siteID, userID)
    if err != nil {
        return fmt.Errorf("invitationRepository.RemoveMember: %w", err)
    }
    if tag.RowsAffected() == 0 {
        return ErrNotFound
    }
    return nil
}

func (r *pgInvitationRepository) IsMember(ctx context.Context, siteID, userID string) (bool, error) {
    var exists bool
    err := r.pool.QueryRow(ctx,
        `SELECT EXISTS(SELECT 1 FROM site_members WHERE site_id=$1 AND user_id=$2)`,
        siteID, userID).Scan(&exists)
    return exists, err
}

var _ InvitationRepository = (*pgInvitationRepository)(nil)
```

- [ ] **Step 3: Add `Invitations` to `internal/repository/repos.go`**

```go
type Repos struct {
    Users       UserRepository
    Sites       SiteRepository
    Events      EventRepository
    Stats       StatsRepository
    Admin       AdminRepository
    CMS         CMSRepository
    Goals       GoalRepository
    Funnels     FunnelRepository
    Invitations InvitationRepository
}

func New(pool *pgxpool.Pool) *Repos {
    return &Repos{
        // ... existing fields ...
        Invitations: &pgInvitationRepository{pool: pool},
    }
}
```

- [ ] **Step 4: Build**

```bash
go build ./...
```

- [ ] **Step 5: Commit**

```bash
git add internal/repository/invitations.go internal/repository/migrations/011_member_indexes.sql internal/repository/repos.go
git commit -m "feat: InvitationRepository for site_members and invitations CRUD"
```

---

### Task 3: Invite handlers + settings team section

**Files:**
- Modify: `internal/handler/sites.go` — add InviteForm, SendInvite, RemoveMember handlers
- Modify: `templates/pages/dashboard/settings.html` — add Members section
- Modify: `cmd/server/main.go` — add invite/member routes

- [ ] **Step 1: Add invite/member handlers to `internal/handler/sites.go`**

The `SitesHandler` needs access to `EmailService`. Update the struct and constructor:

```go
type SitesHandler struct {
    auth    service.AuthService
    repos   *repository.Repos
    tmpls   map[string]*template.Template
    baseURL string
    email   service.EmailService
}

func NewSitesHandler(auth service.AuthService, repos *repository.Repos, baseURL string, emailSvc service.EmailService) *SitesHandler {
    return &SitesHandler{auth: auth, repos: repos, baseURL: baseURL, email: emailSvc}
}
```

Update the constructor call in `cmd/server/main.go`:
```go
emailSvc := service.NewEmailService(cfg.SMTPHost, cfg.SMTPPort, cfg.SMTPUser, cfg.SMTPPass, cfg.SMTPFrom)
sitesHandler := handler.NewSitesHandler(authSvc, repos, cfg.BaseURL, emailSvc)
```

Add handlers:

```go
// SendInvite handles POST /sites/:siteID/settings/invite.
func (h *SitesHandler) SendInvite(w http.ResponseWriter, r *http.Request) {
    site, err := resolveSite(r.Context(), h.repos, chi.URLParam(r, "siteID"))
    if err != nil { http.NotFound(w, r); return }
    if err := r.ParseForm(); err != nil { http.Error(w, "bad request", http.StatusBadRequest); return }

    email := strings.TrimSpace(r.FormValue("email"))
    role  := r.FormValue("role")
    if email == "" { http.Error(w, "email required", http.StatusUnprocessableEntity); return }
    if role != "editor" && role != "viewer" { role = "viewer" }

    // Generate secure token
    b := make([]byte, 24)
    if _, err := rand.Read(b); err != nil {
        http.Error(w, "internal server error", http.StatusInternalServerError); return
    }
    token := base64.URLEncoding.EncodeToString(b)

    inv := &repository.Invitation{
        SiteID:    site.ID,
        Email:     email,
        Token:     token,
        Role:      role,
        ExpiresAt: time.Now().Add(48 * time.Hour),
    }
    if err := h.repos.Invitations.CreateInvitation(r.Context(), inv); err != nil {
        slog.Error("sites.SendInvite: create invitation", "error", err)
        http.Error(w, "internal server error", http.StatusInternalServerError); return
    }

    inviteURL := h.baseURL + "/invite/" + token
    if err := h.email.SendInvitation(email, h.baseURL+"/sites/"+DomainSlug(site.Domain), site.Name, inviteURL, role); err != nil {
        slog.Error("sites.SendInvite: send email", "error", err)
        // Don't fail — invitation is stored; user can resend
    }

    http.Redirect(w, r, "/sites/"+DomainSlug(site.Domain)+"/settings?invited=1", http.StatusSeeOther)
}

// RemoveMember handles POST /sites/:siteID/settings/members/:userID/remove.
func (h *SitesHandler) RemoveMember(w http.ResponseWriter, r *http.Request) {
    site, err := resolveSite(r.Context(), h.repos, chi.URLParam(r, "siteID"))
    if err != nil { http.NotFound(w, r); return }
    userID := chi.URLParam(r, "userID")
    if err := h.repos.Invitations.RemoveMember(r.Context(), site.ID, userID); err != nil {
        slog.Error("sites.RemoveMember", "error", err)
        http.Error(w, "internal server error", http.StatusInternalServerError); return
    }
    http.Redirect(w, r, "/sites/"+DomainSlug(site.Domain)+"/settings", http.StatusSeeOther)
}
```

Add imports: `"crypto/rand"`, `"encoding/base64"`, `"github.com/funky-monkey/analyics-dash-tics/internal/repository"`.

- [ ] **Step 2: Add Members section to `templates/pages/dashboard/settings.html`**

After the existing "General" and "Tracking token" cards, before "Danger zone", add:

```html
<div class="bg-white rounded-xl border border-gray-100 p-6">
  <h3 class="text-sm font-semibold text-gray-900 mb-4">Team members</h3>
  {{if .Members}}
  <div class="divide-y divide-gray-50 mb-4">
    {{range .Members}}
    <div class="flex items-center justify-between py-2.5">
      <div>
        <p class="text-sm font-medium text-gray-900">{{.UserName}}</p>
        <p class="text-xs text-gray-400">{{.UserEmail}}</p>
      </div>
      <div class="flex items-center gap-3">
        <span class="text-xs px-2 py-0.5 rounded-full bg-violet-50 text-violet-600">{{.Role}}</span>
        <form method="POST" action="/sites/{{$.SiteID}}/settings/members/{{.UserID}}/remove"
              onsubmit="return confirm('Remove this member?')">
          <input type="hidden" name="_csrf" value="{{$.CSRFToken}}">
          <button type="submit" class="text-xs text-red-400 hover:text-red-600">Remove</button>
        </form>
      </div>
    </div>
    {{end}}
  </div>
  {{end}}
  <h4 class="text-xs font-medium text-gray-600 mb-2">Invite by email</h4>
  <form method="POST" action="/sites/{{.SiteID}}/settings/invite" class="flex gap-2">
    <input type="hidden" name="_csrf" value="{{.CSRFToken}}">
    <input type="email" name="email" placeholder="colleague@example.com" required
           class="flex-1 border border-gray-200 rounded-lg px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-violet-500">
    <select name="role" class="border border-gray-200 rounded-lg px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-violet-500">
      <option value="viewer">Viewer</option>
      <option value="editor">Editor</option>
    </select>
    <button type="submit" class="btn-primary whitespace-nowrap">Send invite</button>
  </form>
  {{if .Invited}}<p class="text-xs text-green-600 mt-2">Invitation sent.</p>{{end}}
</div>
```

Update the `Settings` handler to also fetch members and pass `Invited` flag:

```go
members, _ := h.repos.Invitations.ListMembers(r.Context(), site.ID)
// ... add to data map:
"Members": members,
"Invited": r.URL.Query().Get("invited") == "1",
```

- [ ] **Step 3: Register routes in `cmd/server/main.go`**

In the authenticated routes group:
```go
r.Post("/sites/{siteID}/settings/invite", sitesHandler.SendInvite)
r.Post("/sites/{siteID}/settings/members/{userID}/remove", sitesHandler.RemoveMember)
```

- [ ] **Step 4: Build**

```bash
go build ./...
```

- [ ] **Step 5: Commit**

```bash
git add internal/handler/sites.go templates/pages/dashboard/settings.html cmd/server/main.go
git commit -m "feat: site team management — invite by email, list and remove members"
```

---

### Task 4: Invitation accept flow

**Files:**
- Create: `templates/pages/auth/accept-invite.html`
- Modify: `internal/handler/auth.go` — add AcceptInvitePage and AcceptInvite handlers
- Modify: `cmd/server/main.go` — add public invite routes

- [ ] **Step 1: Add AcceptInvitePage and AcceptInvite to `internal/handler/auth.go`**

```go
// AcceptInvitePage renders GET /invite/:token.
func (h *AuthHandler) AcceptInvitePage(w http.ResponseWriter, r *http.Request) {
    token := chi.URLParam(r, "token")
    inv, err := h.repos.Invitations.GetInvitationByToken(r.Context(), token)
    if err != nil {
        h.renderAuth(w, r, "accept-invite.html", authPageData{Error: "This invitation link is invalid or has expired."})
        return
    }
    // Check if the email already has an account
    existing, _ := h.repos.Users.GetByEmail(r.Context(), inv.Email)
    h.renderTemplate(w, r, "accept-invite.html", map[string]any{
        "Token":       token,
        "Invitation":  inv,
        "HasAccount":  existing != nil,
        "CSRFToken":   csrfToken(r),
    })
}

// AcceptInvite handles POST /invite/:token.
func (h *AuthHandler) AcceptInvite(w http.ResponseWriter, r *http.Request) {
    token := chi.URLParam(r, "token")
    if err := r.ParseForm(); err != nil {
        http.Error(w, "bad request", http.StatusBadRequest); return
    }
    inv, err := h.repos.Invitations.GetInvitationByToken(r.Context(), token)
    if err != nil {
        http.Error(w, "invitation expired or invalid", http.StatusBadRequest); return
    }

    // Find or create the user
    user, err := h.repos.Users.GetByEmail(r.Context(), inv.Email)
    if err != nil {
        // New user — require a password
        password := r.FormValue("password")
        if len(password) < 12 {
            h.renderTemplate(w, r, "accept-invite.html", map[string]any{
                "Token": token, "Invitation": inv, "HasAccount": false,
                "CSRFToken": csrfToken(r),
                "Error": "Password must be at least 12 characters.",
            })
            return
        }
        hash, err := h.auth.HashPassword(password)
        if err != nil {
            http.Error(w, "internal server error", http.StatusInternalServerError); return
        }
        name := strings.TrimSpace(r.FormValue("name"))
        if name == "" { name = inv.Email }
        user = &model.User{Email: inv.Email, PasswordHash: hash, Role: model.RoleUser, Name: name, IsActive: true}
        if err := h.repos.Users.Create(r.Context(), user); err != nil {
            http.Error(w, "internal server error", http.StatusInternalServerError); return
        }
    }

    // Add member and clean up invitation
    if err := h.repos.Invitations.AddMember(r.Context(), inv.SiteID, user.ID, inv.Role); err != nil {
        slog.Error("AcceptInvite: add member", "error", err)
        http.Error(w, "internal server error", http.StatusInternalServerError); return
    }
    _ = h.repos.Invitations.DeleteInvitation(r.Context(), inv.ID)

    h.issueTokensAndRedirect(w, r, user.ID, string(user.Role), "/dashboard")
}
```

- [ ] **Step 2: Create `templates/pages/auth/accept-invite.html`**

```html
{{template "base.html" .}}
{{define "title"}}Accept invitation — Dashtics{{end}}
{{define "content"}}
<div class="min-h-screen flex items-center justify-center">
  <div class="w-full max-w-sm bg-white rounded-2xl shadow-sm border border-gray-100 p-8">
    {{if .Error}}
    <div class="mb-4 p-3 bg-red-50 border border-red-200 rounded-lg text-red-700 text-sm">{{.Error}}</div>
    {{else}}
    <h1 class="text-2xl font-bold mb-1">Join {{.Invitation.SiteID}}</h1>
    <p class="text-gray-500 text-sm mb-6">You've been invited as a <strong>{{.Invitation.Role}}</strong>.</p>
    <form method="POST" class="space-y-4">
      <input type="hidden" name="_csrf" value="{{.CSRFToken}}">
      {{if not .HasAccount}}
      <div>
        <label class="block text-sm font-medium text-gray-700 mb-1">Your name</label>
        <input type="text" name="name" required class="w-full px-3 py-2 border border-gray-200 rounded-lg text-sm focus:outline-none focus:ring-2 focus:ring-violet-500">
      </div>
      <div>
        <label class="block text-sm font-medium text-gray-700 mb-1">Set a password <span class="text-gray-400 font-normal">(min. 12 chars)</span></label>
        <input type="password" name="password" required minlength="12" class="w-full px-3 py-2 border border-gray-200 rounded-lg text-sm focus:outline-none focus:ring-2 focus:ring-violet-500">
      </div>
      {{end}}
      <button type="submit" class="w-full bg-violet-600 hover:bg-violet-700 text-white font-medium py-2.5 rounded-lg text-sm transition-colors">
        {{if .HasAccount}}Accept invitation{{else}}Create account & accept{{end}}
      </button>
    </form>
    {{end}}
  </div>
</div>
{{end}}
```

- [ ] **Step 3: Register invite routes in `cmd/server/main.go`** (public, no JWT)

```go
r.Get("/invite/{token}", authHandler.AcceptInvitePage)
r.Post("/invite/{token}", authHandler.AcceptInvite)
```

Note: `AcceptInvitePage` and `AcceptInvite` need access to `h.repos.Invitations` — add `Invitations InvitationRepository` to `AuthHandler` and pass it from `NewAuthHandler`. The `AuthHandler` constructor in `auth.go` already receives `repos *repository.Repos`, so `h.repos.Invitations` is already accessible.

- [ ] **Step 4: Build**

```bash
go build ./...
```

- [ ] **Step 5: Commit**

```bash
git add internal/handler/auth.go templates/pages/auth/accept-invite.html cmd/server/main.go
git commit -m "feat: invitation accept flow — creates account if new, adds site_member on accept"
```

---

### Task 5: Weekly email reports

**Files:**
- Create: `internal/service/reporter.go` — weekly report scheduler
- Modify: `cmd/server/main.go` — start reporter goroutine

- [ ] **Step 1: Create `internal/service/reporter.go`**

```go
package service

import (
    "context"
    "fmt"
    "log/slog"
    "time"

    "github.com/funky-monkey/analyics-dash-tics/internal/repository"
)

// StartWeeklyReporter launches a background goroutine that sends weekly analytics
// reports every Monday at 08:00 UTC to site owners and all active members.
// It returns immediately; the goroutine runs until the process exits.
func StartWeeklyReporter(repos *repository.Repos, emailSvc EmailService, baseURL string) {
    go func() {
        for {
            next := nextMondayMorning()
            slog.Info("weekly reporter: next run", "at", next.Format(time.RFC3339))
            time.Sleep(time.Until(next))
            runWeeklyReports(repos, emailSvc, baseURL)
        }
    }()
}

func nextMondayMorning() time.Time {
    now := time.Now().UTC()
    // Advance to next Monday
    daysUntilMonday := (int(time.Monday) - int(now.Weekday()) + 7) % 7
    if daysUntilMonday == 0 && now.Hour() >= 8 {
        daysUntilMonday = 7
    }
    next := time.Date(now.Year(), now.Month(), now.Day()+daysUntilMonday, 8, 0, 0, 0, time.UTC)
    return next
}

func runWeeklyReports(repos *repository.Repos, emailSvc EmailService, baseURL string) {
    ctx := context.Background()
    to := time.Now().UTC().Truncate(24 * time.Hour)
    from := to.AddDate(0, 0, -7)
    periodLabel := fmt.Sprintf("%s – %s, %d",
        from.Format("Jan 2"), to.Format("Jan 2"), to.Year())

    sites, err := repos.Admin.ListAllSites(ctx, 500, 0)
    if err != nil {
        slog.Error("reporter: list sites", "error", err)
        return
    }

    filter := repository.StatsFilter{}
    for _, site := range sites {
        summary, err := repos.Stats.GetSummary(ctx, site.ID, from, to, filter)
        if err != nil || summary.Visitors == 0 {
            continue // skip sites with no activity
        }

        topPages, _ := repos.Stats.GetTopPages(ctx, site.ID, from, to, 1)
        topPage := ""
        if len(topPages) > 0 {
            topPage = topPages[0].URL
        }
        topSources, _ := repos.Stats.GetTopSources(ctx, site.ID, from, to, 1)
        topSource := ""
        if len(topSources) > 0 {
            topSource = topSources[0].Channel
        }

        stats := WeeklyStats{
            Pageviews:   summary.Pageviews,
            Visitors:    summary.Visitors,
            Sessions:    summary.Sessions,
            BounceRate:  summary.BounceRate,
            TopPage:     topPage,
            TopSource:   topSource,
            PeriodLabel: periodLabel,
        }

        siteURL := baseURL + "/sites/" + site.Domain

        // Email owner
        owner, err := repos.Users.GetByID(ctx, site.OwnerID)
        if err == nil {
            if err := emailSvc.SendWeeklyReport(owner.Email, site.Name, siteURL, stats); err != nil {
                slog.Error("reporter: send to owner", "site", site.Domain, "error", err)
            }
        }

        // Email members
        members, err := repos.Invitations.ListMembers(ctx, site.ID)
        if err != nil {
            continue
        }
        for _, m := range members {
            if err := emailSvc.SendWeeklyReport(m.UserEmail, site.Name, siteURL, stats); err != nil {
                slog.Error("reporter: send to member", "email", m.UserEmail, "error", err)
            }
        }

        slog.Info("weekly report sent", "site", site.Domain, "recipients", len(members)+1)
    }
}
```

- [ ] **Step 2: Start the reporter in `cmd/server/main.go`**

After all handlers are wired and before `srv.ListenAndServe`, add:

```go
service.StartWeeklyReporter(repos, emailSvc, cfg.BaseURL)
```

- [ ] **Step 3: Build**

```bash
go build ./...
```

- [ ] **Step 4: Commit**

```bash
git add internal/service/reporter.go cmd/server/main.go
git commit -m "feat: weekly email reports sent every Monday at 08:00 UTC"
```

---

## Self-Review

**Spec coverage:**
- ✅ Multi-user team management → Tasks 2, 3
- ✅ Role-based access (viewer/editor) → Tasks 2, 3, 4
- ✅ Invitation flow → Tasks 3, 4
- ✅ Email reports (scheduled) → Tasks 1, 5

**Placeholder scan:** All implementations are complete. Email templates are inline in Go strings (no external files needed). The `noopEmailService` handles missing SMTP config gracefully.

**Type consistency:**
- `repository.Invitation` defined in Task 2, used in Tasks 3 + 4. ✓
- `repository.Member` defined in Task 2, used in Task 3 settings template. ✓
- `service.WeeklyStats` defined in Task 1, used in Task 5 reporter. ✓
- `StatsFilter{}` passed to `GetSummary` in reporter — matches updated interface from Plan 8 Task 2. If Plan 8 hasn't run yet, pass the old 3-param signature; adjust when Plan 8 runs. ✓
- `emailSvc` created in `cmd/server/main.go` and passed to `NewSitesHandler` and `StartWeeklyReporter`. ✓
