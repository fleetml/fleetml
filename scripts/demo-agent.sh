#!/usr/bin/env bash
# FleetML on edge boxes, in under a minute: four Linux device agents run
# 1.0.0; a canary of 2.0.0 reaches half of them, their inference process
# reports errors with it, the health gate rolls the release back, and the
# canary boxes switch their `current` link back to 1.0.0.
#
# Like scripts/demo.sh it starts the dev stack if needed and runs its own
# server (REST 18380, gRPC 19390) with a throwaway database. The agents are
# real fleetml-agent processes on this machine, each with a small stand-in
# inference loop that posts events to the agent's local endpoint. Needs Go,
# Docker, curl and jq.
#
# Recording hooks (docs/screenshots/demo/record.sh): with WAIT_FOR_ENTER=1 the
# story starts on Enter, after the setup; with DEMO_INFO=<file> the server's
# address and key are written to <file> after the story, and the script keeps
# the server running until <file>.done exists.
set -euo pipefail
cd "$(dirname "$0")/.."

REST_PORT=${FLEETML_DEMO_REST_PORT:-18380}
GRPC_PORT=${FLEETML_DEMO_GRPC_PORT:-19390}
API="http://localhost:$REST_PORT"
PG_HOST_URL=${FLEETML_DEMO_PG:-postgres://fleetml:fleetml_dev@localhost:${FLEETML_PG_PORT:-15432}}
DB="fleetml_demo_${RANDOM}${RANDOM}"
KEY=$(head -c 32 /dev/urandom | base64 | tr '+/' '-_' | tr -d '=\n')
WORK=$(mktemp -d -t fleetml-demo-agent.XXXXXX)
BOXES=(edge-a edge-b edge-c edge-d)
PIDS=()

if [ -t 1 ]; then B=$'\033[1m'; D=$'\033[2m'; G=$'\033[32m'; R=$'\033[31m'; Y=$'\033[33m'; N=$'\033[0m'; else B= D= G= R= Y= N=; fi
say() { printf '\n%s▶ %s%s\n' "$B" "$*" "$N"; }
note() { printf '  %s\n' "$*"; }
show() { printf '  %s$ %s%s\n' "$D" "$*" "$N"; }
api() { curl -sf -X "$1" -H "Authorization: Bearer $KEY" -H 'Content-Type: application/json' ${3:+-d "$3"} "$API$2"; }
wait_for() { # <seconds> <description> <command...>
  local limit=$1 what=$2; shift 2
  for _ in $(seq 1 $((limit * 4))); do "$@" >/dev/null 2>&1 && return 0; sleep 0.25; done
  printf '%stimed out waiting for %s%s\n' "$R" "$what" "$N"; exit 1
}
psql_admin() { docker compose -f deploy/docker-compose.yml exec -T postgres psql -U fleetml -d fleetml -qAtc "$1"; }
port() { echo $((19600 + $1)); } # local endpoint of box $1
# active <box index>: the version behind the box's models/iris/current link
active() { basename "$(readlink "$WORK/${BOXES[$1]}/models/iris/current" 2>/dev/null || echo none-)" | cut -d- -f1; }
fleet() { local i out=""; for i in "${!BOXES[@]}"; do out+="${BOXES[$i]} $(active "$i")   "; done; echo "$out"; }
all_on() { local i; for i in "${!BOXES[@]}"; do [ "$(active "$i")" = "$1" ] || return 1; done; }

cleanup() {
  for p in "${PIDS[@]}"; do kill "$p" 2>/dev/null || true; done
  [ -n "${SERVER_PID:-}" ] && kill "$SERVER_PID" 2>/dev/null && wait "$SERVER_PID" 2>/dev/null || true
  psql_admin "DROP DATABASE IF EXISTS $DB WITH (FORCE)" >/dev/null 2>&1 || true
  [ -n "${NATS_NAME:-}" ] && docker rm -f "$NATS_NAME" >/dev/null 2>&1 || true
  rm -rf "$WORK"
}
trap cleanup EXIT

say "Starting the dev stack, a demo server and four edge boxes"
docker compose -f deploy/docker-compose.yml up -d >/dev/null 2>&1
./scripts/wait-migrate.sh
psql_admin "CREATE DATABASE $DB"
# Its own NATS: telemetry goes through a JetStream work queue, so a second
# server must not share the dev stack's stream with `make dev`.
NATS_NAME="fleetml-demo-agent-nats-$$"
docker run -d --rm --name "$NATS_NAME" -p 127.0.0.1::4222 nats:2.12.1 --jetstream >/dev/null
NATS_PORT=$(docker port "$NATS_NAME" 4222/tcp | head -1 | sed 's/.*://')
go build -o "$WORK/server" ./cmd/server
go build -o "$WORK/fleetml" ./cmd/fleetml
go build -o "$WORK/fleetml-agent" ./cmd/fleetml-agent
env DATABASE_URL="$PG_HOST_URL/$DB?sslmode=disable" MIGRATE_ON_START=true \
  S3_ENDPOINT="localhost:${FLEETML_S3_PORT:-19000}" S3_ACCESS_KEY=fleetml S3_SECRET_KEY=fleetml-dev-secret \
  NATS_URL="nats://localhost:$NATS_PORT" JWT_SECRET="$(head -c 32 /dev/urandom | base64)" \
  SIGNING_MASTER_KEY="$(head -c 32 /dev/urandom | base64)" \
  REST_PORT="$REST_PORT" GRPC_PORT="$GRPC_PORT" FLEETML_ADMIN_KEY="$KEY" \
  DEPLOY_TICK_INTERVAL=1s DEVICE_HEARTBEAT_INTERVAL=5s DEVICE_TELEMETRY_FLUSH_INTERVAL=1s \
  "$WORK/server" > "$WORK/server.log" 2>&1 &
SERVER_PID=$!
wait_for 60 "the server" curl -sf "$API/readyz"
export FLEETML_SERVER=$API FLEETML_API_KEY=$KEY
P=$(api POST /api/v1/projects '{"name":"edge"}' | jq -r .data.id)
EK=$(api POST "/api/v1/projects/$P/enrollment-keys" '{"name":"edge boxes"}' | jq -r .data.key)

# The stand-in inference process of box $1: it runs the active model (as
# GET /v1/models reports it) and posts what happened. 2.0.0 loads fine but
# fails at inference, like a model exported with the wrong output shape.
inference() {
  local ok='{"model":"iris","latency_ms":8,"confidence":0.97,"label":"setosa"}'
  local bad='{"model":"iris","latency_ms":9,"error":"output shape (1,4), expected (1,3)"}'
  while sleep 0.5; do
    case "$(curl -s "localhost:$(port "$1")/v1/models" | jq -r '.[0].Version // empty' 2>/dev/null)" in
      2.0.0) curl -s -X POST "localhost:$(port "$1")/v1/events" -d "[$bad,$bad,$bad,$bad]" >/dev/null || true ;;
      "") ;;
      *) curl -s -X POST "localhost:$(port "$1")/v1/events" -d "[$ok,$ok,$ok,$ok]" >/dev/null || true ;;
    esac
  done
}
for i in "${!BOXES[@]}"; do
  "$WORK/fleetml-agent" -server "localhost:$GRPC_PORT" -enrollment-key "$EK" -label site=depot -label box="${BOXES[$i]}" \
    -state-dir "$WORK/${BOXES[$i]}" -telemetry-listen "127.0.0.1:$(port "$i")" \
    -validator 'test -s {file}' > "$WORK/${BOXES[$i]}.log" 2>&1 &
  PIDS+=($!)
  inference "$i" & PIDS+=($!)
done
devices() { [ "$(api GET /api/v1/devices | jq '.data | length')" -ge "${#BOXES[@]}" ]; }
wait_for 30 "the agents to register" devices

if [ "${WAIT_FOR_ENTER:-}" = 1 ]; then read -r -p "  ready; press Enter to start the story " _; printf '\033[H\033[2J'; fi
STORY=$(date +%s)

say "1. Four edge boxes run iris 1.0.0"
for v in 1.0.0:iris_v1 2.0.0:iris_v2; do
  show "fleetml push models/iris/${v#*:}.tflite --model iris --version ${v%%:*}"
  "$WORK/fleetml" push "models/iris/${v#*:}.tflite" --project "$P" --model iris --version "${v%%:*}" >/dev/null 2>&1
done
M=$(api GET "/api/v1/projects/$P/models" | jq -r '.data[0].id')
version_id() { api GET "/api/v1/models/$M/versions" | jq -r --arg v "$1" '.data[] | select(.version == $v) | .id'; }
V1=$(version_id 1.0.0); V2=$(version_id 2.0.0)
show "POST /api/v1/deployments  {version: 1.0.0, target: site=depot}"
D1=$(api POST /api/v1/deployments "{\"project_id\":\"$P\",\"model_version_id\":\"$V1\",\"target_selector\":{\"site\":\"depot\"}}" | jq -r .data.id)
api POST "/api/v1/deployments/$D1/start" >/dev/null
wait_for 30 "the boxes to load 1.0.0" all_on 1.0.0
note "fleet: $(fleet)"
note "each agent verified the sha256, ran its validator and switched models/iris/current"

say "2. A canary of 2.0.0 on half of them, with a health gate"
show "POST /api/v1/deployments  {2.0.0, CANARY 50% -> 100%, gate: errors <= 5% over 20 events, else ROLLBACK}"
canary_boxes() { local i n=0; for i in "${!BOXES[@]}"; do [ "$(active "$i")" = 2.0.0 ] && n=$((n + 1)); done; echo $n; }
canary() {
  D2=$(api POST /api/v1/deployments "{\"project_id\":\"$P\",\"model_version_id\":\"$V2\",\"strategy\":\"CANARY\",\"target_selector\":{\"site\":\"depot\"},\"canary_stages\":[{\"percentage\":50,\"min_duration\":\"5m\"},{\"percentage\":100,\"min_duration\":\"0s\"}],\"health_gate\":{\"max_error_rate\":0.05,\"min_events\":20,\"on_failure\":\"ROLLBACK\"}}" | jq -r .data.id)
  api POST "/api/v1/deployments/$D2/start" >/dev/null
}
in_cohort() { [ "$(canary_boxes)" -gt 0 ]; }
# The cohort is a hash of the deployment and the device; with four boxes it is
# empty one time in sixteen, and a new deployment draws a new one.
for attempt in 1 2 3 4 5; do
  canary
  for _ in $(seq 1 48); do in_cohort && break 2; sleep 0.25; done
  api POST "/api/v1/deployments/$D2/cancel" >/dev/null
done
in_cohort || { printf '%sno box reached the canary%s\n' "$R" "$N"; exit 1; }
note "fleet: $(fleet | sed "s/2\.0\.0/${Y}2.0.0${N}/g")"
note "the canary boxes run 2.0.0; their inference process now reports errors"

say "3. The health gate trips and rolls the canary back"
rolled_back() { [ "$(api GET "/api/v1/deployments/$D2" | jq -r .data.status)" = ROLLED_BACK ]; }
wait_for 30 "the health gate" rolled_back
note "${R}deployment ROLLED_BACK${N}: $(api GET "/api/v1/deployments/$D2" | jq -r .data.rollback_reason)"
wait_for 30 "the canary boxes to return to 1.0.0" all_on 1.0.0
note "${G}fleet: $(fleet)${N}"
for i in "${!BOXES[@]}"; do
  if grep -q 'version=2.0.0' "$WORK/${BOXES[$i]}.log"; then
    note "${BOXES[$i]} switched back without a download: 1.0.0 was still on disk as its last-known-good version"
  fi
done

say "Done in $(( $(date +%s) - STORY ))s"
note "Install the agent on a real box: docs/LINUX_AGENT.md. Phones: scripts/demo.sh, docs/ANDROID_SDK.md, docs/IOS_SDK.md."
if [ -n "${DEMO_INFO:-}" ]; then
  printf 'API=%s\nKEY=%s\nV2=%s\n' "$API" "$KEY" "$V2" > "$DEMO_INFO"
  until [ -f "$DEMO_INFO.done" ]; do sleep 0.5; done
fi
