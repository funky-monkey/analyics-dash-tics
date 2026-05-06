# Analytics SaaS — Plan 5: Public Site + CI/CD

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:subagent-driven-development` (recommended) or `superpowers:executing-plans` to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the public-facing landing page (the first thing visitors see before signing up), set up GitHub Actions CI/CD pipeline (lint → test → build → deploy), and create the systemd service file for bare-server deployment.

**Architecture:** The landing page is a static Go-served HTML page with Tailwind styling — no JS framework, no SPA. GitHub Actions runs on push to `main`: lints with golangci-lint, runs `go test -race ./...`, compiles Tailwind CSS, builds the Go binary, then deploys via SSH + rsync to the server and restarts the systemd service. The systemd unit file runs the Go binary as a long-lived service.

**Tech Stack:** Go `html/template`, Tailwind CSS (standalone CLI binary in CI), GitHub Actions, systemd, `rsync` + SSH for deployment.

> **No Co-Authored-By** in any commit message.
> **Working directory:** `/Users/sidneydekoning/stack/Github/analyics-dash-tics`
> **Module:** `github.com/sidneydekoning/analytics`

---

## File Map

```
templates/pages/public/
  home.html                      — landing page

.github/
  workflows/
    ci.yml                       — lint + test + build + deploy

deploy/
  analytics.service              — systemd unit file
  deploy.sh                      — deployment script (rsync + restart)

.golangci.yml                    — linter configuration
```

---

### Task 1: Landing page

**Files:**
- Create: `templates/pages/public/home.html`
- Modify: `cmd/server/main.go` — add GET `/` route that renders home.html

- [ ] **Step 1: Create `templates/pages/public/home.html`**

```html
{{template "base.html" .}}
{{define "title"}}Analytics — Privacy-first web analytics for marketers{{end}}
{{define "content"}}

{{/* Hero */}}
<section class="bg-white border-b border-gray-100">
  <div class="max-w-5xl mx-auto px-6 py-24 text-center">
    <div class="inline-flex items-center gap-2 bg-violet-50 text-violet-700 text-xs font-semibold px-3 py-1 rounded-full mb-8">
      <span class="w-1.5 h-1.5 bg-violet-500 rounded-full inline-block"></span>
      Privacy-first · No cookies · GDPR compliant
    </div>
    <h1 class="text-5xl font-bold text-gray-900 leading-tight mb-6">
      Analytics that<br>respect your visitors
    </h1>
    <p class="text-xl text-gray-500 max-w-2xl mx-auto mb-10 leading-relaxed">
      Beautiful, fast, and privacy-first web analytics for marketers.
      No cookie banners. No personal data. Full GDPR compliance out of the box.
    </p>
    <div class="flex items-center justify-center gap-4">
      <a href="/signup" class="btn-primary text-base px-6 py-3">Start for free</a>
      <a href="/blog" class="text-sm text-gray-500 hover:text-gray-700 transition-colors">Read the blog →</a>
    </div>
  </div>
</section>

{{/* Features */}}
<section class="py-24 bg-gray-50">
  <div class="max-w-5xl mx-auto px-6">
    <h2 class="text-3xl font-bold text-gray-900 text-center mb-16">Everything you need, nothing you don't</h2>
    <div class="grid grid-cols-3 gap-8">

      <div class="bg-white rounded-2xl border border-gray-100 p-6">
        <div class="w-10 h-10 bg-violet-100 rounded-xl flex items-center justify-center mb-4">
          <svg class="w-5 h-5 text-violet-600" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 12l2 2 4-4m5.618-4.016A11.955 11.955 0 0112 2.944a11.955 11.955 0 01-8.618 3.04A12.02 12.02 0 003 9c0 5.591 3.824 10.29 9 11.622 5.176-1.332 9-6.03 9-11.622 0-1.042-.133-2.052-.382-3.016z"/>
          </svg>
        </div>
        <h3 class="font-semibold text-gray-900 mb-2">Privacy by design</h3>
        <p class="text-sm text-gray-500 leading-relaxed">No cookies. No personal data stored. No consent banners needed. GDPR, CCPA, and PECR compliant from day one.</p>
      </div>

      <div class="bg-white rounded-2xl border border-gray-100 p-6">
        <div class="w-10 h-10 bg-violet-100 rounded-xl flex items-center justify-center mb-4">
          <svg class="w-5 h-5 text-violet-600" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 10V3L4 14h7v7l9-11h-7z"/>
          </svg>
        </div>
        <h3 class="font-semibold text-gray-900 mb-2">AI traffic detection</h3>
        <p class="text-sm text-gray-500 leading-relaxed">See exactly how much of your traffic comes from ChatGPT, Claude, Perplexity, Gemini, and other AI assistants.</p>
      </div>

      <div class="bg-white rounded-2xl border border-gray-100 p-6">
        <div class="w-10 h-10 bg-violet-100 rounded-xl flex items-center justify-center mb-4">
          <svg class="w-5 h-5 text-violet-600" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 19v-6a2 2 0 00-2-2H5a2 2 0 00-2 2v6a2 2 0 002 2h2a2 2 0 002-2zm0 0V9a2 2 0 012-2h2a2 2 0 012 2v10m-6 0a2 2 0 002 2h2a2 2 0 002-2m0 0V5a2 2 0 012-2h2a2 2 0 012 2v14a2 2 0 01-2 2h-2a2 2 0 01-2-2z"/>
          </svg>
        </div>
        <h3 class="font-semibold text-gray-900 mb-2">Clean dashboard</h3>
        <p class="text-sm text-gray-500 leading-relaxed">A single, focused dashboard. Real-time visitors, traffic sources, top pages, funnels, and audience — all in one view.</p>
      </div>

      <div class="bg-white rounded-2xl border border-gray-100 p-6">
        <div class="w-10 h-10 bg-violet-100 rounded-xl flex items-center justify-center mb-4">
          <svg class="w-5 h-5 text-violet-600" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M3 4a1 1 0 011-1h16a1 1 0 011 1v2.586a1 1 0 01-.293.707l-6.414 6.414a1 1 0 00-.293.707V17l-4 4v-6.586a1 1 0 00-.293-.707L3.293 7.293A1 1 0 013 6.586V4z"/>
          </svg>
        </div>
        <h3 class="font-semibold text-gray-900 mb-2">Conversion funnels</h3>
        <p class="text-sm text-gray-500 leading-relaxed">Build funnels in seconds. See where visitors drop off and which channels convert best for your business.</p>
      </div>

      <div class="bg-white rounded-2xl border border-gray-100 p-6">
        <div class="w-10 h-10 bg-violet-100 rounded-xl flex items-center justify-center mb-4">
          <svg class="w-5 h-5 text-violet-600" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M7 7h.01M7 3h5c.512 0 1.024.195 1.414.586l7 7a2 2 0 010 2.828l-7 7a2 2 0 01-2.828 0l-7-7A1.994 1.994 0 013 12V7a4 4 0 014-4z"/>
          </svg>
        </div>
        <h3 class="font-semibold text-gray-900 mb-2">Auto channel grouping</h3>
        <p class="text-sm text-gray-500 leading-relaxed">Organic, direct, social, email, paid — traffic automatically classified into channels so you never need to configure UTM rules.</p>
      </div>

      <div class="bg-white rounded-2xl border border-gray-100 p-6">
        <div class="w-10 h-10 bg-violet-100 rounded-xl flex items-center justify-center mb-4">
          <svg class="w-5 h-5 text-violet-600" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M10 20l4-16m4 4l4 4-4 4M6 16l-4-4 4-4"/>
          </svg>
        </div>
        <h3 class="font-semibold text-gray-900 mb-2">Lightweight script</h3>
        <p class="text-sm text-gray-500 leading-relaxed">Under 2KB. Add one script tag and you're tracking. Supports SPAs, custom events, and respects Do Not Track.</p>
      </div>

    </div>
  </div>
</section>

{{/* Snippet preview */}}
<section class="py-24 bg-white border-t border-gray-100">
  <div class="max-w-3xl mx-auto px-6 text-center">
    <h2 class="text-3xl font-bold text-gray-900 mb-4">Up in 30 seconds</h2>
    <p class="text-gray-500 mb-10">Add one line to your site. That's it.</p>
    <div class="bg-gray-900 rounded-xl p-6 text-left font-mono text-sm text-green-400 mb-10">
      &lt;script src="https://yourdomain.com/static/script.js"<br>
      &nbsp;&nbsp;data-site="<span class="text-yellow-300">tk_your_token</span>" async&gt;&lt;/script&gt;
    </div>
    <a href="/signup" class="btn-primary text-base px-8 py-3">Create free account</a>
  </div>
</section>

{{/* Footer */}}
<footer class="bg-gray-50 border-t border-gray-100 py-12">
  <div class="max-w-5xl mx-auto px-6 flex items-center justify-between">
    <div class="flex items-center gap-2">
      <div class="w-6 h-6 bg-violet-600 rounded-md"></div>
      <span class="font-semibold text-gray-700 text-sm">Analytics</span>
    </div>
    <div class="flex gap-6">
      <a href="/blog" class="text-sm text-gray-400 hover:text-gray-600">Blog</a>
      <a href="/login" class="text-sm text-gray-400 hover:text-gray-600">Sign in</a>
      <a href="/signup" class="text-sm text-gray-400 hover:text-gray-600">Sign up</a>
    </div>
    <p class="text-xs text-gray-400">Privacy-first analytics. EU hosted.</p>
  </div>
</footer>

{{end}}
```

- [ ] **Step 2: Update `cmd/server/main.go` — replace the `/` redirect with home page**

Read the current main.go. Find:
```go
r.Get("/", func(w http.ResponseWriter, r *http.Request) {
    http.Redirect(w, r, "/login", http.StatusTemporaryRedirect)
})
```

Replace with:
```go
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
```

- [ ] **Step 3: Build and test**

```bash
go build -o bin/analytics ./cmd/server
go test -race ./...
```

- [ ] **Step 4: Rebuild Tailwind**

```bash
./bin/tailwindcss -i static/css/input.css -o static/css/output.css --minify
```

- [ ] **Step 5: Smoke test**

Start the server and visit the home page:
```bash
pkill -f bin/analytics 2>/dev/null; sleep 1
DATABASE_URL="postgres://sidneydekoning@localhost:5432/analytics?sslmode=disable" \
JWT_SECRET="55b8fa86529f04fbf54de43cfa221b57795b63166c6cab23881ee9693698ff91" \
JWT_REFRESH_SECRET="73c246e9baeb07f098c8b9c1a5d98e53fcd7d19defaa9af76f39cb0c1c90d03c" \
BASE_URL="https://dash.local" PORT="8090" ENV="development" \
./bin/analytics &
sleep 2
curl -sk https://dash.local/ | grep '<title>'
```

Expected: `<title>Analytics — Privacy-first web analytics for marketers</title>`

- [ ] **Step 6: Commit**

```bash
git add templates/pages/public/home.html static/css/output.css cmd/server/main.go
git commit -m "feat: add public landing page"
```

---

### Task 2: golangci-lint config

**Files:**
- Create: `.golangci.yml`

- [ ] **Step 1: Create `.golangci.yml`**

```yaml
# .golangci.yml
run:
  timeout: 5m
  go: "1.22"

linters:
  enable:
    - errcheck      # check all errors are handled
    - gosimple      # simplification suggestions
    - govet         # vet checks
    - ineffassign   # unused assignments
    - staticcheck   # comprehensive static analysis
    - unused        # unused code
    - gosec         # security checks
    - gofmt         # formatting
    - goimports     # import organisation

linters-settings:
  gosec:
    excludes:
      - G304   # file path from variable (acceptable in our template loader)

issues:
  exclude-rules:
    # Test files can use assertions without checking errors
    - path: "_test.go"
      linters:
        - errcheck
```

- [ ] **Step 2: Install golangci-lint (if not present)**

```bash
which golangci-lint || brew install golangci-lint
```

- [ ] **Step 3: Run lint**

```bash
golangci-lint run ./... 2>&1 | head -30
```

Fix any errors reported. Common issues to watch for:
- `errcheck`: unhandled errors — add `_ =` or proper error handling
- `gosec`: security warnings — review and fix or add `//nolint:gosec` with a comment explaining why

- [ ] **Step 4: Commit**

```bash
git add .golangci.yml
git commit -m "chore: add golangci-lint config"
```

---

### Task 3: systemd service file

**Files:**
- Create: `deploy/analytics.service`

- [ ] **Step 1: Create `deploy/` directory and service file**

```bash
mkdir -p deploy
```

Create `deploy/analytics.service`:

```ini
[Unit]
Description=Analytics SaaS
After=network.target postgresql.service
Wants=postgresql.service

[Service]
Type=simple
User=analytics
WorkingDirectory=/opt/analytics
ExecStart=/opt/analytics/bin/analytics
Restart=on-failure
RestartSec=5s

# Environment — loaded from /opt/analytics/.env
EnvironmentFile=/opt/analytics/.env

# Security hardening
NoNewPrivileges=yes
PrivateTmp=yes
ProtectSystem=strict
ReadWritePaths=/opt/analytics

# Logging — captured by journald
StandardOutput=journal
StandardError=journal
SyslogIdentifier=analytics

[Install]
WantedBy=multi-user.target
```

- [ ] **Step 2: Commit**

```bash
git add deploy/analytics.service
git commit -m "chore: add systemd service file for bare-server deployment"
```

---

### Task 4: GitHub Actions CI/CD

**Files:**
- Create: `.github/workflows/ci.yml`

- [ ] **Step 1: Create `.github/workflows/ci.yml`**

```bash
mkdir -p .github/workflows
```

```yaml
# .github/workflows/ci.yml
name: CI/CD

on:
  push:
    branches: [main]
  pull_request:
    branches: [main]

env:
  GO_VERSION: "1.22"

jobs:
  ci:
    name: Test & Build
    runs-on: ubuntu-latest

    steps:
      - uses: actions/checkout@v4

      - name: Set up Go
        uses: actions/setup-go@v5
        with:
          go-version: ${{ env.GO_VERSION }}
          cache: true

      - name: go vet
        run: go vet ./...

      - name: golangci-lint
        uses: golangci/golangci-lint-action@v6
        with:
          version: latest
          args: --timeout=5m

      - name: govulncheck
        run: |
          go install golang.org/x/vuln/cmd/govulncheck@latest
          govulncheck ./...

      - name: Test (with race detector)
        run: go test -race ./...

      - name: Download Tailwind CSS standalone CLI
        run: |
          curl -sLo tailwindcss https://github.com/tailwindlabs/tailwindcss/releases/latest/download/tailwindcss-linux-x64
          chmod +x tailwindcss

      - name: Build CSS
        run: ./tailwindcss -i static/css/input.css -o static/css/output.css --minify

      - name: Build binary
        run: go build -o bin/analytics ./cmd/server

      - name: Upload binary artifact
        uses: actions/upload-artifact@v4
        with:
          name: analytics-binary
          path: |
            bin/analytics
            static/css/output.css
          retention-days: 1

  deploy:
    name: Deploy to server
    runs-on: ubuntu-latest
    needs: ci
    if: github.ref == 'refs/heads/main' && github.event_name == 'push'

    steps:
      - uses: actions/checkout@v4

      - name: Download binary artifact
        uses: actions/download-artifact@v4
        with:
          name: analytics-binary

      - name: Set up SSH
        uses: webfactory/ssh-agent@v0.9.0
        with:
          ssh-private-key: ${{ secrets.DEPLOY_SSH_KEY }}

      - name: Add server to known hosts
        run: |
          mkdir -p ~/.ssh
          ssh-keyscan -H ${{ secrets.DEPLOY_HOST }} >> ~/.ssh/known_hosts

      - name: Deploy
        env:
          DEPLOY_HOST: ${{ secrets.DEPLOY_HOST }}
          DEPLOY_USER: ${{ secrets.DEPLOY_USER }}
          DEPLOY_PATH: ${{ secrets.DEPLOY_PATH }}
        run: |
          chmod +x bin/analytics
          # Sync binary and static assets
          rsync -avz --delete \
            bin/analytics \
            static/ \
            templates/ \
            ${DEPLOY_USER}@${DEPLOY_HOST}:${DEPLOY_PATH}/

          # Restart the service
          ssh ${DEPLOY_USER}@${DEPLOY_HOST} "sudo systemctl restart analytics"
```

- [ ] **Step 2: Create `deploy/README.md` with setup instructions**

```bash
cat > deploy/README.md << 'EOF'
# Deployment Setup

## Required GitHub Secrets

Set these in Settings → Secrets → Actions:

| Secret | Description | Example |
|---|---|---|
| `DEPLOY_SSH_KEY` | Private SSH key for deployment user | `-----BEGIN OPENSSH...` |
| `DEPLOY_HOST` | Server hostname or IP | `analytics.yourdomain.com` |
| `DEPLOY_USER` | SSH user on the server | `analytics` |
| `DEPLOY_PATH` | Deployment directory on server | `/opt/analytics` |

## Server Setup (one-time)

```bash
# On the server:
# 1. Create deployment user
useradd -m -s /bin/bash analytics

# 2. Create deployment directory
mkdir -p /opt/analytics/{bin,static,templates}
chown -R analytics:analytics /opt/analytics

# 3. Create .env file
cp .env.example /opt/analytics/.env
# Edit /opt/analytics/.env with production values

# 4. Install the systemd service
cp deploy/analytics.service /etc/systemd/system/
systemctl daemon-reload
systemctl enable analytics
systemctl start analytics

# 5. Allow analytics user to restart the service without password
echo "analytics ALL=(ALL) NOPASSWD: /bin/systemctl restart analytics" >> /etc/sudoers.d/analytics

# 6. Add CI deployment key to authorized_keys
cat deploy_key.pub >> /home/analytics/.ssh/authorized_keys
```

## PostgreSQL Setup

```bash
# Install TimescaleDB
sudo add-apt-repository ppa:timescale/timescaledb-ppa
sudo apt-get update
sudo apt-get install timescaledb-2-postgresql-17

# Configure
sudo timescaledb-tune --quiet --yes
sudo systemctl restart postgresql

# Create database
sudo -u postgres createdb analytics
sudo -u postgres psql analytics -c "CREATE EXTENSION IF NOT EXISTS timescaledb CASCADE;"
```
EOF
```

- [ ] **Step 3: Commit**

```bash
git add .github/workflows/ci.yml deploy/README.md
git commit -m "feat: add GitHub Actions CI/CD pipeline and deployment docs"
git push origin main
```

---

## Self-Review

**Spec coverage check:**

| Requirement | Task |
|---|---|
| Public landing page (/) | Task 1 |
| Home page with features, tracking snippet preview, CTA | Task 1 |
| golangci-lint + gosec config | Task 2 |
| systemd service file | Task 3 |
| GitHub Actions: go vet + golangci-lint + govulncheck | Task 4 |
| GitHub Actions: go test -race | Task 4 |
| GitHub Actions: Tailwind build (standalone CLI, no Node.js) | Task 4 |
| GitHub Actions: go build | Task 4 |
| GitHub Actions: rsync → SSH → systemctl restart | Task 4 |
| Server setup documentation | Task 4 |
| Secrets documented | Task 4 |

**No placeholders.**
