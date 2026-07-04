GO      ?= go
BINARY  := bin/deltad
COMPOSE := docker compose -f deploy/docker-compose.yml

.PHONY: all build run fmt fmt-check lint proto-lint test test-race test-integration cover vuln tidy-check generate \
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
	$(GO) tool buf generate

# Breaking-change detection needs a baseline; the git guard makes it a no-op
# until buf.yaml exists on the baseline (CI passes BUF_BASELINE=origin/main).
BUF_BASELINE ?= main
proto-lint:
	$(GO) tool buf lint
	@if git cat-file -e $(BUF_BASELINE):buf.yaml 2>/dev/null; then \
		$(GO) tool buf breaking --against '.git#branch=$(BUF_BASELINE)'; \
	fi

migrate-up migrate-down migrate-status: migrate-%:
	$(GO) tool goose -dir internal/adapters/postgres/migrations postgres "$$DELTA__POSTGRES__DSN" $*

compose-up:
	$(COMPOSE) up -d

compose-down:
	$(COMPOSE) down

ci: fmt-check lint proto-lint vuln test-race tidy-check
