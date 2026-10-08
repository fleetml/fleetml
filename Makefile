.PHONY: env-file build build-server build-cli install-cli test test-integration cover e2e lint proto proto-swift ios-test build-agent migrate-up migrate-down docker-up docker-down dev clean all dashboard-build dashboard-dev docs-site

-include .env

BINARY=bin/fleetml-server
DATABASE_URL?=postgres://fleetml:fleetml_dev@localhost:15432/fleetml?sslmode=disable
MIGRATIONS_PATH=internal/db/migrations

# Export only docker-compose port mappings and DATABASE_URL for migrations
export FLEETML_PG_PORT
export FLEETML_NATS_PORT
export FLEETML_NATS_MONITOR_PORT
export FLEETML_S3_PORT
export DATABASE_URL

all: build test lint

build: build-server build-cli

build-server:
	@mkdir -p bin
	go build -o $(BINARY) ./cmd/server

# The device agent as static binaries for Linux (amd64, arm64) and macOS (local testing).
build-agent:
	@mkdir -p bin
	@for target in linux/amd64 linux/arm64 darwin/arm64; do \
		os=$${target%/*}; arch=$${target#*/}; \
		CGO_ENABLED=0 GOOS=$$os GOARCH=$$arch go build -trimpath -ldflags="-s -w" -o bin/fleetml-agent-$$os-$$arch ./cmd/fleetml-agent || exit 1; \
	done
	@ls -la bin/fleetml-agent-*

# The documentation site into site/ (MkDocs Material, pinned, in Docker), then
# an offline check of every internal link and anchor. Nothing is deployed.
MKDOCS_IMAGE := squidfunk/mkdocs-material:9.6.14@sha256:eb04b60c566a8862be6b553157c16a92fbbfc45d71b7e4e8593526aecca63f52
docs-site:
	docker run --rm -e PYTHONDONTWRITEBYTECODE=1 -u "$$(id -u):$$(id -g)" -v "$(CURDIR)":/docs $(MKDOCS_IMAGE) build
	docker run --rm -e PYTHONDONTWRITEBYTECODE=1 -u "$$(id -u):$$(id -g)" -v "$(CURDIR)":/docs --entrypoint python3 $(MKDOCS_IMAGE) scripts/check-site-links.py site

build-cli:
	@mkdir -p bin
	go build -o bin/fleetml ./cmd/fleetml

install-cli:
	go install ./cmd/fleetml

test:
	go test -race -count=1 ./...

# Repository and pipeline tests against throwaway databases on the dev stack.
TEST_DATABASE_URL?=postgres://fleetml:fleetml_dev@localhost:$(or $(FLEETML_PG_PORT),15432)/fleetml?sslmode=disable
TEST_NATS_URL?=nats://localhost:$(or $(FLEETML_NATS_PORT),14222)

test-integration: docker-up
	docker compose -f deploy/docker-compose.yml wait migrate
	FLEETML_TEST_DATABASE_URL="$(TEST_DATABASE_URL)" FLEETML_TEST_NATS_URL="$(TEST_NATS_URL)" \
		go test -tags integration -race -count=1 ./...

# Unit + integration coverage, written to coverage.out.
cover: docker-up
	docker compose -f deploy/docker-compose.yml wait migrate
	FLEETML_TEST_DATABASE_URL="$(TEST_DATABASE_URL)" FLEETML_TEST_NATS_URL="$(TEST_NATS_URL)" \
		go test -tags integration -count=1 -coverprofile=coverage.out ./internal/...
	go tool cover -func=coverage.out | tail -1

# End-to-end rollout, A/B and integrity scenarios against a fresh server (see scripts/e2e.sh).
e2e: docker-up
	docker compose -f deploy/docker-compose.yml wait migrate
	./scripts/e2e.sh -v

lint:
	golangci-lint run ./...

proto:
	./scripts/proto-gen.sh

proto-swift:
	./scripts/proto-gen-swift.sh

ios-test:
	cd ios/FleetMLSDK && swift test

migrate-up:
	migrate -database "$(DATABASE_URL)" -path $(MIGRATIONS_PATH) up

migrate-down:
	migrate -database "$(DATABASE_URL)" -path $(MIGRATIONS_PATH) down 1

docker-up:
	docker compose -f deploy/docker-compose.yml up -d

docker-down:
	docker compose -f deploy/docker-compose.yml down

dashboard-build:
	cd dashboard && npm ci && npm run build

dashboard-dev:
	cd dashboard && { [ -d node_modules ] || npm ci; } && npm run dev

env-file:
	@if [ ! -f .env ]; then \
		cp .env.example .env; \
		printf 'FLEETML_ADMIN_KEY=%s\n' "$$(head -c 32 /dev/urandom | base64 | tr '+/' '-_' | tr -d '=\n')" >> .env; \
		sed -i.bak "s|^JWT_SECRET=$$|JWT_SECRET=$$(head -c 32 /dev/urandom | base64 | tr -d '\n')|" .env && rm -f .env.bak; \
		echo "Created .env from .env.example (REST :18080, gRPC :19090, infra on 15xxx host ports)"; \
		echo "Your admin API key is FLEETML_ADMIN_KEY in .env (used on first start only)"; \
	fi
	@# .env files from before v0.3.0 have no signing master key yet.
	@if ! grep -q '^SIGNING_MASTER_KEY=.' .env; then \
		sed -i.bak '/^SIGNING_MASTER_KEY=$$/d' .env && rm -f .env.bak; \
		printf 'SIGNING_MASTER_KEY=%s\n' "$$(head -c 32 /dev/urandom | base64 | tr -d '\n')" >> .env; \
		echo "Added a random SIGNING_MASTER_KEY to .env"; \
	fi

dev: env-file docker-up
	@echo "Waiting for database migrations (compose 'migrate' service)..."
	docker compose -f deploy/docker-compose.yml wait migrate
	env $$(grep -v '^\#' .env | xargs) go run ./cmd/server

clean:
	rm -rf bin/ gen/ site/
