GO      ?= go
# The application name lives here and in config.EnvPrefix; changing those
# two renames the operational surface (AGENTS.md). identity-check keeps
# them from drifting.
NAME    := delta
BINARY  := bin/$(NAME)d
CTL     := bin/$(NAME)ctl
ENV     := $(shell echo '$(NAME)' | tr 'a-z-' 'A-Z_')__
COMPOSE := docker compose -f deploy/docker-compose.yml

.PHONY: all build run run-docker fmt fmt-check lint proto-lint identity-check test test-race test-integration cover vuln tidy-check generate \
        migrate-up migrate-down migrate-status compose-up compose-down ci

all: build

build:
	$(GO) build -o $(BINARY) ./cmd/daemon
	$(GO) build -o $(CTL) ./cmd/ctl

run: build
	./$(BINARY)

# Same daemon, compose-mapped database ports (make compose-up first).
NATIVE_DSN := postgres://oms:oms@localhost:5432/oms?sslmode=disable
DOCKER_DSN := postgres://oms:oms@localhost:5433/oms?sslmode=disable
run-docker: build
	$(ENV)POSTGRES__DSN="$(DOCKER_DSN)" \
	$(ENV)QUESTDB__CONF="http::addr=localhost:9010;" ./$(BINARY)

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

identity-check:
	@grep -Eq 'EnvPrefix[[:space:]]*=[[:space:]]*"$(ENV)"' internal/config/config.go || \
		{ echo "NAME=$(NAME) implies $(ENV) but config.EnvPrefix differs"; exit 1; }

generate:
	$(GO) tool sqlc generate
	$(GO) tool buf generate

# Breaking-change detection needs a baseline; the git guard makes it a no-op
# until buf.yaml exists on the baseline (CI passes BUF_BASELINE=origin/main).
BUF_BASELINE ?= main
proto-lint:
	$(GO) tool buf lint
	@if git cat-file -e $(BUF_BASELINE):buf.yaml 2>/dev/null; then \
		$(GO) tool buf breaking --against '.git#branch=$(BUF_BASELINE)'; \
	fi

# The env DSN wins; without it, migrate against the native default.
migrate-up migrate-down migrate-status: migrate-%:
	$(GO) tool goose -dir internal/adapters/postgres/migrations postgres "$${$(ENV)POSTGRES__DSN:-$(NATIVE_DSN)}" $*

compose-up:
	$(COMPOSE) up -d

compose-down:
	$(COMPOSE) down

ci: fmt-check lint proto-lint identity-check vuln test-race tidy-check
