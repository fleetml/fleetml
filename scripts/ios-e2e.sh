#!/usr/bin/env bash
# Runs the iOS SDK's live test (LiveServerTests) on a simulator against the
# local dev server:
#
#   1. start the server: make dev
#   2. FLEETML_API_KEY=<admin key> ./scripts/ios-e2e.sh ['platform=iOS Simulator,name=iPhone 17 Pro']
#
# The script creates a project, a Core ML model version, an enrollment key and
# a non-urgent deployment for a fresh label. The test registers a device on an
# "expensive network" (cellular), checks the model waits, then creates an
# urgent deployment and checks it downloads and activates.
set -euo pipefail
cd "$(dirname "$0")/.."

: "${FLEETML_API_KEY:?set FLEETML_API_KEY to an admin API key}"
API=${FLEETML_API:-http://localhost:18080}
DEST=${1:-platform=iOS Simulator,name=iPhone 17 Pro}
MODEL=models/iris/iris_v1.mlmodel
LABEL="run-$RANDOM$RANDOM"

api() { curl -sf -X "$1" -H "Authorization: Bearer $FLEETML_API_KEY" -H 'Content-Type: application/json' -d "${3:-}" "$API$2"; }

P=$(api POST /api/v1/projects "{\"name\":\"ios-e2e-$LABEL\"}" | jq -r .data.id)
M=$(api POST "/api/v1/projects/$P/models" '{"name":"iris","framework":"COREML","task_type":"CLASSIFICATION"}' | jq -r .data.id)
V=$(api POST "/api/v1/models/$M/versions" '{"version":"1.0.0","metadata":{"fleetml_requirements":{"min_ram_mb":64,"min_score":0}}}' | jq -r .data.id)
OUT=$(api POST "/api/v1/versions/$V/variants" "{\"quantization\":\"FP32\",\"target_runtime\":\"COREML\",\"blob_size_bytes\":$(wc -c < "$MODEL" | tr -d ' '),\"blob_sha256\":\"$(shasum -a 256 "$MODEL" | cut -d' ' -f1)\"}")
curl -sf -o /dev/null -X PUT --data-binary @"$MODEL" "$(echo "$OUT" | jq -r .data.upload_url)"
api POST "/api/v1/variants/$(echo "$OUT" | jq -r .data.variant.id)/finalize" >/dev/null
EK=$(api POST "/api/v1/projects/$P/enrollment-keys" '{"name":"ios-e2e"}' | jq -r .data.key)
D=$(api POST /api/v1/deployments "{\"project_id\":\"$P\",\"model_version_id\":\"$V\",\"target_selector\":{\"ios-live\":\"$LABEL\"}}" | jq -r .data.id)
api POST "/api/v1/deployments/$D/start" >/dev/null
echo "fixtures: project $P, version $V, deployment $D, label ios-live=$LABEL"

# The package manifest is at the repository root (the current directory).
TEST_RUNNER_FLEETML_LIVE_HOST=localhost TEST_RUNNER_FLEETML_LIVE_ENROLLMENT_KEY="$EK" TEST_RUNNER_FLEETML_LIVE_LABEL="$LABEL" \
TEST_RUNNER_FLEETML_LIVE_API="$API" TEST_RUNNER_FLEETML_LIVE_ADMIN_KEY="$FLEETML_API_KEY" \
TEST_RUNNER_FLEETML_LIVE_PROJECT="$P" TEST_RUNNER_FLEETML_LIVE_VERSION="$V" \
  xcodebuild test -scheme FleetMLSDK -destination "$DEST" -only-testing:FleetMLSDKTests/LiveServerTests
