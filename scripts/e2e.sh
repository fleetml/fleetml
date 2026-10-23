#!/usr/bin/env bash
# Runs the end-to-end suite (test/e2e) and the CLI smoke test (cmd/fleetml)
# against a fresh server and a throwaway database on the dev stack
# (`make docker-up`). The server listens on spare ports (REST 18180, gRPC
# 19190) with 1s background loops, so it can run next to `make dev`. Extra
# arguments are passed to `go test`.
set -euo pipefail
cd "$(dirname "$0")/.."

PG_HOST_URL=${FLEETML_E2E_PG:-postgres://fleetml:fleetml_dev@localhost:${FLEETML_PG_PORT:-15432}}
DB="fleetml_e2e_${RANDOM}${RANDOM}"
KEY=$(head -c 32 /dev/urandom | base64 | tr '+/' '-_' | tr -d '=\n')
REST_PORT=${FLEETML_E2E_REST_PORT:-18180}
GRPC_PORT=${FLEETML_E2E_GRPC_PORT:-19190}

# The throwaway database lives on the Postgres of one compose project: the dev
# stack (`fleetml`, the name in deploy/docker-compose.yml) unless
# COMPOSE_PROJECT_NAME names a stack of your own. The project is named
# explicitly and must publish Postgres on the port the server connects to, so
# a run meant for another stack never creates or drops a database in the dev
# stack.
PROJECT=${COMPOSE_PROJECT_NAME:-fleetml}
PG_PORT=$(sed -E 's|.*:([0-9]+)/?$|\1|' <<<"$PG_HOST_URL")
psql_admin() { docker compose -p "$PROJECT" -f deploy/docker-compose.yml exec -T postgres psql -U fleetml -d fleetml -qAtc "$1"; }
published=$(docker compose -p "$PROJECT" -f deploy/docker-compose.yml port postgres 5432 2>/dev/null | sed 's/.*://' || true)
if [ "$published" != "$PG_PORT" ]; then
  echo "compose project $PROJECT publishes Postgres on ${published:-no port}, not on $PG_PORT, the port the server connects to;" \
    "set COMPOSE_PROJECT_NAME to the stack you started (make docker-up starts $PROJECT)" >&2
  exit 1
fi
LOG=$(mktemp -t fleetml-e2e-server.XXXXXX)

cleanup() {
  if [ -n "${SERVER_PID:-}" ]; then { kill "$SERVER_PID" && wait "$SERVER_PID"; } 2>/dev/null || true; fi
  psql_admin "DROP DATABASE IF EXISTS $DB WITH (FORCE)" >/dev/null 2>&1 || true
  if [ -n "${NATS_NAME:-}" ]; then docker rm -f "$NATS_NAME" >/dev/null 2>&1 || true; fi
}
trap cleanup EXIT

psql_admin "CREATE DATABASE $DB"
# Its own NATS: telemetry goes through a JetStream work queue, so a second
# server must not share the dev stack's stream with `make dev`.
NATS_NAME="fleetml-e2e-nats-$$"
docker run -d --rm --name "$NATS_NAME" -p 127.0.0.1::4222 nats:2.12.1 --jetstream >/dev/null
NATS_PORT=$(docker port "$NATS_NAME" 4222/tcp | head -1 | sed 's/.*://')
go build -o bin/fleetml-e2e-server ./cmd/server

env DATABASE_URL="$PG_HOST_URL/$DB?sslmode=disable" MIGRATE_ON_START=true \
  S3_ENDPOINT="localhost:${FLEETML_S3_PORT:-19000}" S3_ACCESS_KEY=fleetml S3_SECRET_KEY=fleetml-dev-secret \
  NATS_URL="nats://localhost:$NATS_PORT" JWT_SECRET="$(head -c 32 /dev/urandom | base64)" \
  SIGNING_MASTER_KEY="$(head -c 32 /dev/urandom | base64)" \
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

# The CLI smoke test that CI runs against a fake of the REST API on Windows,
# here against the real server, so the fake and the API cannot drift apart.
FLEETML_SMOKE_SERVER="http://localhost:$REST_PORT" FLEETML_SMOKE_API_KEY="$KEY" \
  go test -count=1 -run TestQuickstartCalls "$@" ./cmd/fleetml/ || { echo "--- server log ($LOG) ---"; tail -50 "$LOG"; exit 1; }
