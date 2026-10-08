.PHONY: env-file build build-server build-cli install-cli test lint proto migrate-up migrate-down docker-up docker-down dev clean all dashboard-build dashboard-dev

-include .env

BINARY=bin/fleetml-server
DATABASE_URL?=postgres://fleetml:fleetml_dev@localhost:15432/fleetml?sslmode=disable
MIGRATIONS_PATH=internal/db/migrations

# Export only docker-compose port mappings and DATABASE_URL for migrations
export FLEETML_PG_PORT
export FLEETML_NATS_PORT
export FLEETML_NATS_MONITOR_PORT
export FLEETML_REDIS_PORT
export FLEETML_S3_PORT
export DATABASE_URL

all: build test lint

build: build-server build-cli

build-server:
	@mkdir -p bin
	go build -o $(BINARY) ./cmd/server

build-cli:
	@mkdir -p bin
	go build -o bin/fleetml ./cmd/fleetml

install-cli:
	go install ./cmd/fleetml

test:
	go test -race -count=1 ./...

lint:
	golangci-lint run ./...

proto:
	./scripts/proto-gen.sh

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
		echo "Created .env from .env.example (REST :18080, gRPC :19090, infra on 15xxx host ports)"; \
		echo "Your admin API key is FLEETML_ADMIN_KEY in .env (used on first start only)"; \
	fi

dev: env-file docker-up
	@echo "Waiting for database migrations (compose 'migrate' service)..."
	docker compose -f deploy/docker-compose.yml wait migrate
	env $$(grep -v '^\#' .env | xargs) go run ./cmd/server

clean:
	rm -rf bin/ gen/
