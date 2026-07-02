GO      ?= go
BINARY  := bin/deltad
COMPOSE := docker compose -f deploy/docker-compose.yml

.PHONY: all build run fmt fmt-check lint test test-race test-integration cover vuln tidy-check generate \
        migrate-up migrate-down migrate-status compose-up compose-down ci

all: build

build:
	$(GO) build -o $(BINARY) ./cmd/deltad

run: build
	./$(BINARY)

fmt:
	golangci-lint fmt

fmt-check:
	golangci-lint fmt --diff

lint:
	golangci-lint run

test:
	$(GO) test -shuffle=on ./...

test-race:
	$(GO) test -race -shuffle=on ./...

test-integration:
	$(GO) test -race -tags integration ./...

cover:
	$(GO) test -race -coverprofile=coverage.out ./...
	$(GO) tool cover -html=coverage.out -o coverage.html

vuln:
	$(GO) tool govulncheck ./...

tidy-check:
	$(GO) mod tidy
	git diff --exit-code go.mod go.sum

generate:
	$(GO) tool sqlc generate

migrate-up migrate-down migrate-status: migrate-%:
	$(GO) tool goose -dir internal/adapters/postgres/migrations postgres "$$DELTA__POSTGRES__DSN" $*

compose-up:
	$(COMPOSE) up -d

compose-down:
	$(COMPOSE) down

ci: fmt-check lint vuln test-race tidy-check
