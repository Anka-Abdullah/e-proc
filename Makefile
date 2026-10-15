SHELL := /bin/bash
GOBIN := $(shell go env GOPATH)/bin
SERVICES := api-gateway vendor-service tender-service bid-service po-service approval-service audit-service reporting-service

.PHONY: help
help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-22s\033[0m %s\n", $$1, $$2}'

.PHONY: proto
proto: ## Lint protobuf definitions and regenerate Go/gRPC code
	buf lint
	buf generate

.PHONY: fmt
fmt: ## Format Go and frontend code
	gofmt -w $$(find . -name '*.go' -not -path './proto/gen/*')
	cd web && pnpm exec prettier --write . 2>/dev/null || true

.PHONY: lint
lint: ## Run golangci-lint and Next.js lint
	golangci-lint run ./...
	cd web && pnpm lint

.PHONY: test
test: ## Run Go tests with race detector and coverage
	go test -race -covermode=atomic -coverprofile=coverage.out ./...

.PHONY: test-web
test-web: ## Run frontend unit tests
	cd web && pnpm test

.PHONY: e2e
e2e: ## Run Playwright end-to-end tests
	cd web && pnpm exec playwright test

.PHONY: build
build: ## Build every Go service into bin/
	@mkdir -p bin
	@for svc in $(SERVICES); do \
		echo "building $$svc"; \
		go build -trimpath -o bin/$$svc ./cmd/$$svc || exit 1; \
	done

.PHONY: run
run: ## Run one service locally: make run SERVICE=vendor-service
	go run ./cmd/$(SERVICE)

.PHONY: infra-up
infra-up: ## Start only Mongo, Redis and RabbitMQ
	docker compose -f deploy/docker/docker-compose.yml up -d mongo redis rabbitmq

.PHONY: up
up: ## Start the whole stack in Docker
	docker compose -f deploy/docker/docker-compose.yml up -d --build

.PHONY: down
down: ## Stop the stack
	docker compose -f deploy/docker/docker-compose.yml down

.PHONY: logs
logs: ## Tail stack logs
	docker compose -f deploy/docker/docker-compose.yml logs -f --tail=100

.PHONY: seed
seed: ## Seed demo data (admin, vendors, tender, bids)
	go run ./scripts/seed

.PHONY: ci
ci: proto fmt lint test build ## Everything CI runs
