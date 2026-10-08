#!/usr/bin/env bash
# Runs the end-to-end suite (test/e2e) against a fresh server and a throwaway
# database on the dev stack (`make docker-up`). The server listens on spare
# ports (REST 18180, gRPC 19190) with 1s background loops, so it can run
# next to `make dev`. Extra arguments are passed to `go test`.
set -euo pipefail
cd "$(dirname "$0")/.."

PG_HOST_URL=${FLEETML_E2E_PG:-postgres://fleetml:fleetml_dev@localhost:${FLEETML_PG_PORT:-15432}}
DB="fleetml_e2e_${RANDOM}${RANDOM}"
KEY=$(head -c 32 /dev/urandom | base64 | tr '+/' '-_' | tr -d '=\n')
REST_PORT=${FLEETML_E2E_REST_PORT:-18180}
GRPC_PORT=${FLEETML_E2E_GRPC_PORT:-19190}
LOG=$(mktemp -t fleetml-e2e-server.XXXXXX)

psql_admin() { docker compose -f deploy/docker-compose.yml exec -T postgres psql -U fleetml -d fleetml -qAtc "$1"; }

cleanup() {
  [ -n "${SERVER_PID:-}" ] && kill "$SERVER_PID" 2>/dev/null && wait "$SERVER_PID" 2>/dev/null || true
  psql_admin "DROP DATABASE IF EXISTS $DB WITH (FORCE)" >/dev/null 2>&1 || true
}
trap cleanup EXIT

psql_admin "CREATE DATABASE $DB"
go build -o bin/fleetml-e2e-server ./cmd/server

env DATABASE_URL="$PG_HOST_URL/$DB?sslmode=disable" MIGRATE_ON_START=true \
  S3_ENDPOINT="localhost:${FLEETML_S3_PORT:-19000}" S3_ACCESS_KEY=fleetml S3_SECRET_KEY=fleetml-dev-secret \
  NATS_URL="nats://localhost:${FLEETML_NATS_PORT:-14222}" JWT_SECRET="$(head -c 32 /dev/urandom | base64)" \
  REST_PORT="$REST_PORT" GRPC_PORT="$GRPC_PORT" FLEETML_ADMIN_KEY="$KEY" CORS_ORIGINS="" \
  DEPLOY_TICK_INTERVAL=1s EXPERIMENT_TICK_INTERVAL=2s LOG_LEVEL=warn LOG_FORMAT=text \
  bin/fleetml-e2e-server >"$LOG" 2>&1 &
SERVER_PID=$!

for _ in $(seq 1 60); do
  curl -sf "localhost:$REST_PORT/healthz" >/dev/null && break
  kill -0 "$SERVER_PID" 2>/dev/null || { cat "$LOG"; exit 1; }
  sleep 0.5
done

FLEETML_E2E_API="http://localhost:$REST_PORT" FLEETML_E2E_GRPC="localhost:$GRPC_PORT" FLEETML_E2E_ADMIN_KEY="$KEY" \
  go test -tags e2e -count=1 "$@" ./test/e2e/... || { echo "--- server log ($LOG) ---"; tail -50 "$LOG"; exit 1; }
