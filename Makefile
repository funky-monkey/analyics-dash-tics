# Makefile
.PHONY: dev seed stop restart test lint build tidy tailwind

# Load .env if it exists
ifneq (,$(wildcard .env))
  include .env
  export
endif

dev:
	go run ./cmd/server

stop:
	@lsof -ti :$(PORT) | xargs kill -9 2>/dev/null && echo "stopped" || echo "nothing running on :$(PORT)"

restart: stop dev

seed:
	go run ./cmd/seed

test:
	go test -race ./...

lint:
	golangci-lint run

build:
	go build -o bin/analytics ./cmd/server

tidy:
	go mod tidy

tailwind:
	./bin/tailwindcss -i static/css/input.css -o static/css/output.css --minify
