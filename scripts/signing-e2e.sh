#!/usr/bin/env bash
# Runtime check of artifact signing on one client against the local dev
# server: the Linux agent, the Android SDK (emulator) or the iOS SDK
# (simulator).
#
#   1. start the server: make dev
#   2. for android, start an emulator; for ios, have a simulator
#   3. FLEETML_API_KEY=<admin key> ./scripts/signing-e2e.sh agent|android|ios ['<xcodebuild destination>']
#
# It publishes four models, rotates the project's signing key with a grace
# period (GRACE seconds, default 180), and then changes signatures in the
# database the way an attacker or a stale artifact would:
#
#   good          signed with the new key                     -> activated
#   tampered      its signature overwritten                   -> rejected (SIGNATURE_INVALID)
#   old-key       its pre-rotation signature, during grace    -> activated
#   old-key-late  its pre-rotation signature, after the grace -> rejected (SIGNATURE_INVALID)
#
# The client runs twice: once during the grace period, once after it. The
# agent and the Android test keep their device between the runs, so old-key,
# accepted during the grace, stays active; the iOS test registers a new device
# each run, which after the grace rejects old-key as well.
set -euo pipefail
cd "$(dirname "$0")/.."

CLIENT=${1:?usage: signing-e2e.sh agent|android|ios [destination]}
DEST=${2:-platform=iOS Simulator,name=iPhone 17 Pro}
: "${FLEETML_API_KEY:?set FLEETML_API_KEY to an admin API key}"
API=${FLEETML_API:-http://localhost:18080}
GRACE=${GRACE:-180}
LABEL="sig-$RANDOM$RANDOM"
WORK=$(mktemp -d -t fleetml-signing.XXXXXX)
trap 'rm -rf "$WORK"' EXIT

api() { curl -sf -X "$1" -H "Authorization: Bearer $FLEETML_API_KEY" -H 'Content-Type: application/json' ${3:+-d "$3"} "$API$2"; }
psql_() { docker compose -f deploy/docker-compose.yml exec -T postgres psql -U fleetml -d fleetml -qAtc "$1"; }
say() { printf '\n== %s\n' "$*"; }

case "$CLIENT" in
  agent | android) MODEL=models/iris/iris_v2.tflite FRAMEWORK=TFLITE ;;
  ios) MODEL=models/iris/iris_v1.mlmodel FRAMEWORK=COREML ;;
  *) echo "unknown client $CLIENT" >&2; exit 2 ;;
esac
SHA=$(shasum -a 256 "$MODEL" | cut -d' ' -f1)

P=$(api POST /api/v1/projects "{\"name\":\"signing-$LABEL\"}" | jq -r .data.id)
EK=$(api POST "/api/v1/projects/$P/enrollment-keys" '{"name":"signing-e2e"}' | jq -r .data.key)
NAMES=(good tampered old-key old-key-late)
VERSIONS=() VARIANTS=()
for name in "${NAMES[@]}"; do
  M=$(api POST "/api/v1/projects/$P/models" "{\"name\":\"$name\",\"framework\":\"$FRAMEWORK\",\"task_type\":\"CLASSIFICATION\"}" | jq -r .data.id)
  V=$(api POST "/api/v1/models/$M/versions" '{"version":"1.0.0","metadata":{"fleetml_requirements":{"min_ram_mb":64,"min_score":0}}}' | jq -r .data.id)
  OUT=$(api POST "/api/v1/versions/$V/variants" "{\"quantization\":\"FP32\",\"target_runtime\":\"$FRAMEWORK\",\"blob_size_bytes\":$(wc -c < "$MODEL" | tr -d ' '),\"blob_sha256\":\"$SHA\"}")
  VERSIONS+=("$V") VARIANTS+=("$(echo "$OUT" | jq -r .data.variant.id)")
  curl -sf -o /dev/null -X PUT --data-binary @"$MODEL" "$(echo "$OUT" | jq -r .data.upload_url)"
  api POST "/api/v1/variants/$(echo "$OUT" | jq -r .data.variant.id)/finalize" >/dev/null
done
# version <name>, variant <name>: IDs of a model's version and variant
idx() { local i; for i in "${!NAMES[@]}"; do [ "${NAMES[$i]}" = "$1" ] && echo "$i"; done; }
version() { echo "${VERSIONS[$(idx "$1")]}"; }
variant() { echo "${VARIANTS[$(idx "$1")]}"; }
sig() { psql_ "SELECT signing_key_id || ' ' || encode(signature, 'hex') FROM model_variants WHERE id = '$1'"; }
OLD_KEY_SIG=$(sig "$(variant old-key)") LATE_SIG=$(sig "$(variant old-key-late)")
K1=${OLD_KEY_SIG%% *}
ROT=$(api POST "/api/v1/projects/$P/signing-key/rotate" "{\"grace\":\"${GRACE}s\"}")
K2=$(echo "$ROT" | jq -r .data.active.key_id)
NOT_AFTER=$(psql_ "SELECT extract(epoch FROM not_after)::bigint FROM signing_keys WHERE key_id = '$K1'")
restore() { psql_ "UPDATE model_variants SET signing_key_id = '${2%% *}', signature = decode('${2#* }', 'hex') WHERE id = '$1'"; }
psql_ "UPDATE model_variants SET signature = decode(repeat('00', 64), 'hex') WHERE id = '$(variant tampered)'"
restore "$(variant old-key)" "$OLD_KEY_SIG"
deploy() {
  local d
  d=$(api POST /api/v1/deployments "{\"project_id\":\"$P\",\"model_version_id\":\"$(version "$1")\",\"target_selector\":{\"e2e-run\":\"$LABEL\"},\"urgent\":true}" | jq -r .data.id)
  api POST "/api/v1/deployments/$d/start" >/dev/null
}
for name in good tampered old-key; do deploy "$name"; done
echo "project $P, label e2e-run=$LABEL; key $K1 rotated to $K2 ($(echo "$ROT" | jq -r .data.resigned_variants) variants signed again), $K1 accepted for ${GRACE}s"
echo "in the database: tampered has a zeroed signature, old-key its $K1 signature again"

# run_client <expect active> <expect rejected>
run_client() {
  case "$CLIENT" in
    agent)
      [ -x "$WORK/fleetml-agent" ] || go build -o "$WORK/fleetml-agent" ./cmd/fleetml-agent
      "$WORK/fleetml-agent" -server "localhost:${FLEETML_GRPC_PORT:-19090}" -enrollment-key "$EK" -label "e2e-run=$LABEL" \
        -state-dir "$WORK/agent" -validator 'test -s {file}' >> "$WORK/agent.log" 2>&1 &
      local pid=$! ok=0
      for _ in $(seq 1 60); do
        sleep 1
        ok=1
        for name in ${1//,/ }; do [ -L "$WORK/agent/models/$name/current" ] || ok=0; done
        [ $ok = 1 ] && break
      done
      sleep 3; kill $pid; wait $pid 2>/dev/null || true
      for name in ${1//,/ }; do [ -L "$WORK/agent/models/$name/current" ] || { echo "FAIL: $name is not active"; return 1; }; done
      for name in ${2//,/ }; do
        [ ! -e "$WORK/agent/models/$name/current" ] || { echo "FAIL: $name is active"; return 1; }
        grep "signature rejected" "$WORK/agent.log" | grep "model=$name" | tail -1 | sed 's/^/  agent: /'
      done
      echo "  active on the agent: $(cd "$WORK/agent/models" && ls -d */current 2>/dev/null | cut -d/ -f1 | tr '\n' ' ')"
      ;;
    android)
      ADB=$(command -v adb || echo "${ANDROID_HOME:-$HOME/Library/Android/sdk}/platform-tools/adb")
      "$ADB" reverse tcp:19000 tcp:19000 >/dev/null
      (cd android && JAVA_HOME=${JAVA_HOME_17:-/opt/homebrew/opt/openjdk@17} ./gradlew -q :fleetml-sdk:connectedDebugAndroidTest \
        -Pandroid.testInstrumentationRunnerArguments.class=com.fleetml.sdk.SignatureLiveTest \
        -Pandroid.testInstrumentationRunnerArguments.enrollmentKey="$EK" \
        -Pandroid.testInstrumentationRunnerArguments.label="$LABEL" \
        -Pandroid.testInstrumentationRunnerArguments.expectActive="$1" \
        -Pandroid.testInstrumentationRunnerArguments.expectRejected="$2")
      "$ADB" logcat -d -s FleetMLSignatureTest:I | tail -1 | sed 's/^/  /'
      ;;
    ios)
      (cd ios/FleetMLSDK && TEST_RUNNER_FLEETML_LIVE_HOST=localhost TEST_RUNNER_FLEETML_LIVE_ENROLLMENT_KEY="$EK" \
        TEST_RUNNER_FLEETML_LIVE_SIG_LABEL="$LABEL" TEST_RUNNER_FLEETML_LIVE_EXPECT_ACTIVE="$1" TEST_RUNNER_FLEETML_LIVE_EXPECT_REJECTED="$2" \
        xcodebuild test -scheme FleetMLSDK -destination "$DEST" -only-testing:FleetMLSDKTests/LiveServerTests/testSignaturesAreVerifiedBeforeActivation) \
        > "$WORK/xcodebuild.log" 2>&1 || { grep -E "error:|failed" "$WORK/xcodebuild.log" | head -5; return 1; }
      grep -E "^FleetMLSignatureTest|Test Case .*testSignatures.*(passed|failed)" "$WORK/xcodebuild.log" | sed 's/^/  /'
      ;;
  esac
}
signature_invalid() { psql_ "SELECT count(*) FROM telemetry_events WHERE model_version_id = '$(version "$1")' AND event_type = 'SIGNATURE_INVALID'"; }
wait_event() {
  for _ in $(seq 1 60); do [ "$(signature_invalid "$1")" -gt 0 ] && return 0; sleep 1; done
  echo "FAIL: no SIGNATURE_INVALID event for $1"; return 1
}

say "1. during the grace period ($(( NOT_AFTER - $(date +%s) ))s left): expect good and old-key active, tampered rejected"
run_client good,old-key tampered
wait_event tampered
echo "  server: SIGNATURE_INVALID events: tampered $(signature_invalid tampered), old-key $(signature_invalid old-key), good $(signature_invalid good)"

say "2. after the grace period: old-key-late carries a $K1 signature"
while [ "$(date +%s)" -le "$NOT_AFTER" ]; do sleep 2; done
restore "$(variant old-key-late)" "$LATE_SIG"
deploy old-key-late
echo "  keys the server sends now: $(api GET "/api/v1/projects/$P/signing-key" | jq -r '[.data.keys[].key_id] | join(", ")')"
if [ "$CLIENT" = ios ]; then run_client good old-key,old-key-late; else run_client good,old-key old-key-late; fi
wait_event old-key-late
echo "  server: SIGNATURE_INVALID events: old-key-late $(signature_invalid old-key-late)"
say "PASS ($CLIENT)"
