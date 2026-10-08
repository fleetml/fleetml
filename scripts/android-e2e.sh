#!/usr/bin/env bash
# Runs the Android SDK's instrumented test (LiveServerTest) on a connected
# emulator against the local dev server:
#
#   1. start the server: make dev
#   2. start an emulator (Android Studio, or: emulator -avd <name>); the script
#      forwards the object store port so download URLs (localhost:19000) work
#   3. FLEETML_API_KEY=<admin key> ./scripts/android-e2e.sh
#
# The script creates a project, a model version with the Iris v2 model, an
# enrollment key and a deployment targeting a fresh label, then runs the test
# with JDK 17. The test registers, downloads, validates (TFLite), runs the
# model, reports telemetry and checks the server stored it.
set -euo pipefail
cd "$(dirname "$0")/.."

: "${FLEETML_API_KEY:?set FLEETML_API_KEY to an admin API key}"
API=${FLEETML_API:-http://localhost:18080}
MODEL=models/iris/iris_v2.tflite
LABEL="run-$RANDOM$RANDOM"

api() { curl -sf -X "$1" -H "Authorization: Bearer $FLEETML_API_KEY" -H 'Content-Type: application/json' ${3:+-d "$3"} "$API$2"; }

SHA=$(shasum -a 256 "$MODEL" | cut -d' ' -f1)
P=$(api POST /api/v1/projects "{\"name\":\"android-e2e-$LABEL\"}" | jq -r .data.id)
M=$(api POST "/api/v1/projects/$P/models" '{"name":"iris","framework":"TFLITE","task_type":"CLASSIFICATION"}' | jq -r .data.id)
# A 2.5 KB model fits any device: lift the selector's FP32 defaults.
V=$(api POST "/api/v1/models/$M/versions" '{"version":"2.0.0","metadata":{"fleetml_requirements":{"min_ram_mb":64,"min_score":0}}}' | jq -r .data.id)
OUT=$(api POST "/api/v1/versions/$V/variants" "{\"quantization\":\"FP32\",\"target_runtime\":\"TFLITE\",\"blob_size_bytes\":$(wc -c < "$MODEL" | tr -d ' '),\"blob_sha256\":\"$SHA\"}")
curl -sf -o /dev/null -X PUT --data-binary @"$MODEL" "$(echo "$OUT" | jq -r .data.upload_url)"
api POST "/api/v1/variants/$(echo "$OUT" | jq -r .data.variant.id)/finalize" >/dev/null
EK=$(api POST "/api/v1/projects/$P/enrollment-keys" '{"name":"android-e2e"}' | jq -r .data.key)
D=$(api POST /api/v1/deployments "{\"project_id\":\"$P\",\"model_version_id\":\"$V\",\"target_selector\":{\"e2e-run\":\"$LABEL\"}}" | jq -r .data.id)
api POST "/api/v1/deployments/$D/start" >/dev/null
echo "fixtures: project $P, version $V, deployment $D, label e2e-run=$LABEL"

ADB=$(command -v adb || echo "${ANDROID_HOME:-$HOME/Library/Android/sdk}/platform-tools/adb")
"$ADB" reverse tcp:19000 tcp:19000 >/dev/null

cd android
JAVA_HOME=${JAVA_HOME_17:-/opt/homebrew/opt/openjdk@17} ./gradlew :fleetml-sdk:connectedDebugAndroidTest \
  -Pandroid.testInstrumentationRunnerArguments.class=com.fleetml.sdk.LiveServerTest \
  -Pandroid.testInstrumentationRunnerArguments.enrollmentKey="$EK" \
  -Pandroid.testInstrumentationRunnerArguments.label="$LABEL" \
  -Pandroid.testInstrumentationRunnerArguments.expectedVersion=2.0.0 \
  -Pandroid.testInstrumentationRunnerArguments.expectedSha="$SHA" \
  -Pandroid.testInstrumentationRunnerArguments.apiKey="$FLEETML_API_KEY"
