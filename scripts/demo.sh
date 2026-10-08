#!/usr/bin/env bash
# FleetML in about a minute, with narration: publish a model, roll it out to a
# simulated fleet, catch latency drift, watch a bad canary roll itself back,
# and pick the next version with an A/B test.
#
# It starts the dev stack if needed (Docker), then runs its own server with
# fast background loops on spare ports (REST 18280, gRPC 19290) and a
# throwaway database, so it does not touch `make dev` data. Needs Go, Docker,
# curl and jq. KEEP=1 leaves the demo server running afterwards.
set -euo pipefail
cd "$(dirname "$0")/.."

REST_PORT=${FLEETML_DEMO_REST_PORT:-18280}
GRPC_PORT=${FLEETML_DEMO_GRPC_PORT:-19290}
API="http://localhost:$REST_PORT"
PG_HOST_URL=${FLEETML_DEMO_PG:-postgres://fleetml:fleetml_dev@localhost:${FLEETML_PG_PORT:-15432}}
DB="fleetml_demo_${RANDOM}${RANDOM}"
KEY=$(head -c 32 /dev/urandom | base64 | tr '+/' '-_' | tr -d '=\n')
WORK=$(mktemp -d -t fleetml-demo.XXXXXX)
START=$(date +%s)

if [ -t 1 ]; then B=$'\033[1m'; D=$'\033[2m'; G=$'\033[32m'; R=$'\033[31m'; N=$'\033[0m'; else B= D= G= R= N=; fi
say() { printf '\n%s▶ %s%s\n' "$B" "$*" "$N"; }
note() { printf '  %s\n' "$*"; }
show() { printf '  %s$ %s%s\n' "$D" "$*" "$N"; }
api() { curl -sf -X "$1" -H "Authorization: Bearer $KEY" -H 'Content-Type: application/json' ${3:+-d "$3"} "$API$2"; }
# wait_for <seconds> <description> <command...>
wait_for() {
  local limit=$1 what=$2; shift 2
  for _ in $(seq 1 $((limit * 2))); do "$@" >/dev/null 2>&1 && return 0; sleep 0.5; done
  printf '%stimed out waiting for %s%s\n' "$R" "$what" "$N"; exit 1
}
fleet() { tail -n 1 "$WORK/sim.log" | sed 's/^simulator: //'; }
fleet_is() { [ "$(fleet)" = "$1" ]; }
psql_admin() { docker compose -f deploy/docker-compose.yml exec -T postgres psql -U fleetml -d fleetml -qAtc "$1"; }

cleanup() {
  [ -n "${SIM_PID:-}" ] && kill "$SIM_PID" 2>/dev/null || true
  if [ "${KEEP:-}" != 1 ]; then
    [ -n "${SERVER_PID:-}" ] && kill "$SERVER_PID" 2>/dev/null && wait "$SERVER_PID" 2>/dev/null || true
    psql_admin "DROP DATABASE IF EXISTS $DB WITH (FORCE)" >/dev/null 2>&1 || true
    rm -rf "$WORK"
  fi
}
trap cleanup EXIT

say "Starting the dev stack and a demo server"
docker compose -f deploy/docker-compose.yml up -d >/dev/null 2>&1
docker compose -f deploy/docker-compose.yml wait migrate >/dev/null
psql_admin "CREATE DATABASE $DB"
go build -o "$WORK/server" ./cmd/server
go build -o "$WORK/fleetml" ./cmd/fleetml
env DATABASE_URL="$PG_HOST_URL/$DB?sslmode=disable" MIGRATE_ON_START=true \
  S3_ENDPOINT="localhost:${FLEETML_S3_PORT:-19000}" S3_ACCESS_KEY=fleetml S3_SECRET_KEY=fleetml-dev-secret \
  NATS_URL="nats://localhost:${FLEETML_NATS_PORT:-14222}" JWT_SECRET="$(head -c 32 /dev/urandom | base64)" \
  REST_PORT="$REST_PORT" GRPC_PORT="$GRPC_PORT" FLEETML_ADMIN_KEY="$KEY" \
  DEPLOY_TICK_INTERVAL=1s EXPERIMENT_TICK_INTERVAL=1s DRIFT_TICK_INTERVAL=1h \
  "$WORK/server" > "$WORK/server.log" 2>&1 &
SERVER_PID=$!
wait_for 60 "the server" curl -sf "$API/readyz"
export FLEETML_SERVER=$API FLEETML_API_KEY=$KEY
note "server ready on REST :$REST_PORT and gRPC :$GRPC_PORT ($(( $(date +%s) - START ))s)"
STORY=$(date +%s)

say "1. Publish three versions of a model"
P=$(api POST /api/v1/projects '{"name":"demo"}' | jq -r .data.id)
for v in 1.0.0:iris_v1 2.0.0:iris_v2 2.1.0:iris_v2; do
  show "fleetml push models/iris/${v#*:}.tflite --model iris --version ${v%%:*}"
  "$WORK/fleetml" push "models/iris/${v#*:}.tflite" --project "$P" --model iris --version "${v%%:*}" 2>/dev/null | grep -i pushed | sed 's/^/  /'
done
M=$(api GET "/api/v1/projects/$P/models" | jq -r '.data[0].id')
version_id() { api GET "/api/v1/models/$M/versions" | jq -r --arg v "$1" '.data[] | select(.version == $v) | .id'; }
V1=$(version_id 1.0.0); V2=$(version_id 2.0.0); V21=$(version_id 2.1.0)
note "each file was sha256-verified on upload; only verified files are ever sent to devices"

say "2. Twenty devices enroll and get 1.0.0"
EK=$(api POST "/api/v1/projects/$P/enrollment-keys" '{"name":"demo"}' | jq -r .data.key)
SIM=(--grpc "localhost:$GRPC_PORT" --enrollment-key "$EK" --devices 20 --label app=demo --error-rate 2.0.0=0.4 --latency 2.1.0=14 --history 168h)
show "fleetml simulate --devices 20 --label app=demo   # simulated phones: real protocol, real downloads"
"$WORK/fleetml" simulate "${SIM[@]}" > "$WORK/sim.log" 2>&1 & SIM_PID=$!
show "POST /api/v1/deployments  {version: 1.0.0, target: app=demo}"
D1=$(api POST /api/v1/deployments "{\"project_id\":\"$P\",\"model_version_id\":\"$V1\",\"target_selector\":{\"app\":\"demo\"}}" | jq -r .data.id)
api POST "/api/v1/deployments/$D1/start" >/dev/null
wait_for 30 "the fleet to load 1.0.0" fleet_is "iris 1.0.0 on 20"
note "fleet: $(fleet)"

say "3. Drift: the last day's latency against the week before"
drift() { api POST "/api/v1/drift/$V1/check" | jq -r '.data[] | select(.metric == "latency") | "\(.severity) (PSI \(.psi_value * 1000 | round / 1000))"'; }
sleep 6 # let the telemetry pipeline flush
note "latency drift for 1.0.0: $(drift)"
note "now the devices slow down (say, after an OS update)..."
kill "$SIM_PID"; wait "$SIM_PID" 2>/dev/null || true
"$WORK/fleetml" simulate "${SIM[@]}" --latency 1.0.0=34 > "$WORK/sim.log" 2>&1 & SIM_PID=$!
sleep 12
note "latency drift for 1.0.0: ${R}$(drift)${N}"
note "a change into CRITICAL is sent as an alert: $(curl -s "$API/metrics" | grep '^fleetml_alerts_sent_total{channel="nats",result="ok",type="drift"}' | awk '{print $2}') drift alert(s) on NATS"

say "4. A canary of 2.0.0 with a health gate; 2.0.0 is broken"
show "POST /api/v1/deployments  {version: 2.0.0, strategy: CANARY 25% -> 100%, gate: errors <= 5% over 30 events, else ROLLBACK}"
D2=$(api POST /api/v1/deployments "{\"project_id\":\"$P\",\"model_version_id\":\"$V2\",\"strategy\":\"CANARY\",\"target_selector\":{\"app\":\"demo\"},\"canary_stages\":[{\"percentage\":25,\"min_duration\":\"2m\"},{\"percentage\":100,\"min_duration\":\"0s\"}],\"health_gate\":{\"max_error_rate\":0.05,\"min_events\":30,\"on_failure\":\"ROLLBACK\"}}" | jq -r .data.id)
api POST "/api/v1/deployments/$D2/start" >/dev/null
wait_for 30 "the canary cohort" grep -q "iris 2.0.0 on" "$WORK/sim.log"
note "fleet: $(grep "iris 2.0.0 on" "$WORK/sim.log" | head -1 | sed 's/^simulator: //')   (the canary cohort only)"
rolled_back() { [ "$(api GET "/api/v1/deployments/$D2" | jq -r .data.status)" = ROLLED_BACK ]; }
wait_for 30 "the health gate" rolled_back
note "${R}deployment ROLLED_BACK${N}: $(api GET "/api/v1/deployments/$D2" | jq -r .data.rollback_reason)"
wait_for 30 "the cohort to return to 1.0.0" fleet_is "iris 1.0.0 on 20"
note "fleet: $(fleet), nobody outside the canary ever saw 2.0.0"

say "5. A/B test: 1.0.0 against the fixed 2.1.0"
show "POST /api/v1/experiments  {control: 1.0.0, treatment: 2.1.0, target: app=demo, min_sample_size: 300}"
EXP=$(api POST /api/v1/experiments "{\"project_id\":\"$P\",\"name\":\"iris 2.1.0 latency\",\"control_version_id\":\"$V1\",\"treatment_version_id\":\"$V21\",\"target_selector\":{\"app\":\"demo\"},\"min_sample_size\":300}" | jq -r .data.id)
api POST "/api/v1/experiments/$EXP/start" >/dev/null
final() { [ "$(api GET "/api/v1/experiments/$EXP/results" | jq -r .data.final)" = true ]; }
wait_for 60 "both arms to reach 300 samples" final
api GET "/api/v1/experiments/$EXP/results" | jq -r '.data | "  control 1.0.0:   \(.control.sample_size) inferences, mean \(.control.avg_latency_ms | round) ms\n  treatment 2.1.0: \(.treatment.sample_size) inferences, mean \(.treatment.avg_latency_ms | round) ms\n  Welch t-test p \(if .p_value < 0.0001 then "< 0.0001" else "= \(.p_value * 10000 | round / 10000)" end), recommendation: \(.recommendation)"'
api POST "/api/v1/experiments/$EXP/conclude" "{\"winner_version_id\":\"$V21\",\"note\":\"faster\"}" >/dev/null
show "POST /api/v1/deployments  {version: 2.1.0, target: app=demo}"
D3=$(api POST /api/v1/deployments "{\"project_id\":\"$P\",\"model_version_id\":\"$V21\",\"target_selector\":{\"app\":\"demo\"}}" | jq -r .data.id)
api POST "/api/v1/deployments/$D3/start" >/dev/null
wait_for 30 "the fleet to load 2.1.0" fleet_is "iris 2.1.0 on 20"
note "${G}fleet: $(fleet)${N}"

say "Done in $(( $(date +%s) - STORY ))s (plus $(( STORY - START ))s of setup)"
note "The same steps work against your devices with the Android and iOS SDKs (docs/ANDROID_SDK.md, docs/IOS_SDK.md)."
if [ "${KEEP:-}" = 1 ]; then
  note "The demo server keeps running: FLEETML_SERVER=$API FLEETML_API_KEY=$KEY ./bin/fleetml deploy list"
  note "Stop it with: kill $SERVER_PID"
fi
