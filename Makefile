# Makefile
.PHONY: dev seed stop restart test lint build tidy css tailwind

# Load .env if it exists
ifneq (,$(wildcard .env))
  include .env
  export
endif

# ── Tailwind binary (auto-download for the current platform) ─────────────────
OS   := $(shell uname -s)
ARCH := $(shell uname -m)

ifeq ($(OS),Darwin)
  ifeq ($(ARCH),arm64)
    TW_ASSET := tailwindcss-macos-arm64
  else
    TW_ASSET := tailwindcss-macos-x64
  endif
else
  TW_ASSET := tailwindcss-linux-x64
endif

TW_BIN := bin/tailwindcss
TW_URL := https://github.com/tailwindlabs/tailwindcss/releases/latest/download/$(TW_ASSET)

$(TW_BIN):
	@mkdir -p bin
	@echo "Downloading Tailwind CSS CLI ($(TW_ASSET))…"
	@curl -sLo $(TW_BIN) $(TW_URL)
	@chmod +x $(TW_BIN)
	@echo "Tailwind CSS CLI ready."

# ── CSS ───────────────────────────────────────────────────────────────────────
css: $(TW_BIN)
	$(TW_BIN) -i static/css/input.css -o static/css/output.css --minify

tailwind: css   # alias

# ── Dev server ────────────────────────────────────────────────────────────────
dev: css
	go run ./cmd/server

stop:
	@lsof -ti :$(PORT) | xargs kill -9 2>/dev/null && echo "stopped" || echo "nothing running on :$(PORT)"

restart: stop css
	go run ./cmd/server

# ── Other ─────────────────────────────────────────────────────────────────────
seed:
	go run ./cmd/seed

test:
	go test -race ./...

lint:
	golangci-lint run

build: css
	go build -o bin/analytics ./cmd/server

tidy:
	go mod tidy
