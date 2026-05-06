# Makefile
.PHONY: dev test lint build

# Load .env if it exists
ifneq (,$(wildcard .env))
  include .env
  export
endif

dev:
	go run ./cmd/server

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
