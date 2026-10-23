#!/usr/bin/env bash
# Runs the Python client's live test (python/tests/test_integration.py: the
# quickstart's sections 1 to 3) against a fresh server and a throwaway
# database on the dev stack (`make docker-up`). The server listens on spare
# ports (REST 18380, gRPC 19390) with 2s background loops, so it can run next
# to `make dev`. Needs Go, and the python package installed with its dev
# extra (pip install -e 'python[dev]'); PYTHON picks the interpreter. Extra
# arguments are passed to pytest.
set -euo pipefail
cd "$(dirname "$0")/.."

PYTHON=${PYTHON:-python3}
PG_HOST_URL=${FLEETML_E2E_PG:-postgres://fleetml:fleetml_dev@localhost:${FLEETML_PG_PORT:-15432}}
DB="fleetml_pye2e_${RANDOM}${RANDOM}"
KEY=$(head -c 32 /dev/urandom | base64 | tr '+/' '-_' | tr -d '=\n')
REST_PORT=${FLEETML_PY_E2E_REST_PORT:-18380}
GRPC_PORT=${FLEETML_PY_E2E_GRPC_PORT:-19390}

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
WORK=$(mktemp -d -t fleetml-py-e2e.XXXXXX)

cleanup() {
  [ -n "${SERVER_PID:-}" ] && kill "$SERVER_PID" 2>/dev/null && wait "$SERVER_PID" 2>/dev/null || true
  psql_admin "DROP DATABASE IF EXISTS $DB WITH (FORCE)" >/dev/null 2>&1 || true
  [ -n "${NATS_NAME:-}" ] && docker rm -f "$NATS_NAME" >/dev/null 2>&1 || true
  rm -rf "$WORK"
}
trap cleanup EXIT

psql_admin "CREATE DATABASE $DB"
# Its own NATS: telemetry goes through a JetStream work queue, so a second
# server must not share the dev stack's stream with `make dev`.
NATS_NAME="fleetml-py-e2e-nats-$$"
docker run -d --rm --name "$NATS_NAME" -p 127.0.0.1::4222 nats:2.12.1 --jetstream >/dev/null
NATS_PORT=$(docker port "$NATS_NAME" 4222/tcp | head -1 | sed 's/.*://')
go build -o "$WORK/server" ./cmd/server
go build -o "$WORK/fleetml" ./cmd/fleetml

env DATABASE_URL="$PG_HOST_URL/$DB?sslmode=disable" MIGRATE_ON_START=true \
  S3_ENDPOINT="localhost:${FLEETML_S3_PORT:-19000}" S3_ACCESS_KEY=fleetml S3_SECRET_KEY=fleetml-dev-secret \
  NATS_URL="nats://localhost:$NATS_PORT" JWT_SECRET="$(head -c 32 /dev/urandom | base64)" \
  SIGNING_MASTER_KEY="$(head -c 32 /dev/urandom | base64)" \
  REST_PORT="$REST_PORT" GRPC_PORT="$GRPC_PORT" FLEETML_ADMIN_KEY="$KEY" CORS_ORIGINS="" \
  DEPLOY_TICK_INTERVAL=2s EXPERIMENT_TICK_INTERVAL=2s LOG_LEVEL=warn LOG_FORMAT=text \
  "$WORK/server" >"$WORK/server.log" 2>&1 &
SERVER_PID=$!

for _ in $(seq 1 60); do
  curl -sf "localhost:$REST_PORT/readyz" >/dev/null && break
  kill -0 "$SERVER_PID" 2>/dev/null || { cat "$WORK/server.log"; exit 1; }
  sleep 0.5
done

FLEETML_LIVE=1 FLEETML_SERVER="http://localhost:$REST_PORT" FLEETML_API_KEY="$KEY" \
  FLEETML_GRPC="localhost:$GRPC_PORT" FLEETML_CLI="$WORK/fleetml" \
  "$PYTHON" -m pytest python/tests/test_integration.py -v -s "$@" \
  || { echo "--- server log ---"; tail -50 "$WORK/server.log"; exit 1; }
