# Rollouts, experiments and alerts

How FleetML decides which model version each device runs, how it protects a
rollout, how A/B experiments work, and what alerts it sends. The
[runbook](#runbook) at the end reproduces every behaviour described here with
copy-paste commands; `make e2e` runs the same scenarios as automated tests.

## How a device gets a model

A **deployment** delivers one model version to the devices whose labels match
its `target_selector` (`{"region": "eu"}` matches every device whose labels
include `region=eu`; `{}` matches all devices).

On every heartbeat the server picks, **for each model**, the newest active
deployment (`ROLLING_OUT` or `COMPLETE`) that targets the device, exposes it
(see the strategies below) and has a READY variant the device can run. Older
deployments of the same model are superseded. Consequences:

- devices outside a canary keep running the previously completed version;
- rolling back or cancelling a deployment hands its devices back to the
  deployment before it;
- a device never receives two versions of the same model.

Each assignment carries `model_id`, `version_id`, the version string, a
presigned download URL (valid 1 hour), the blob's sha256 and its signature
([artifact signing](#artifact-signing)). Devices report
what they have loaded in the next heartbeat; the server answers with `LOAD`,
`UPDATE` (same model, other version) or `UNLOAD`.

### Did the model arrive?

A deployment's status says what was assigned: an all-at-once deployment is
`COMPLETE` as soon as it starts, before any device has downloaded anything.
What the devices report is on two other objects:

- `GET /api/v1/deployments/{id}` (`fleetml deploy get`) has
  `devices: {"targeted": N, "loaded": M}`: the project's devices whose labels
  match the target selector, and how many of them reported a variant of the
  deployment's version loaded in their last heartbeat, so `loaded` turns one
  heartbeat (60 seconds by default) after the device logs the model active.
  Both are counted each time you ask: a deployment that a newer one replaced
  counts down to 0 as its devices move to the newer version. Lists of
  deployments leave it out.
- `GET /api/v1/devices/{id}` (`fleetml device get`) has `loaded_models`, the
  versions the device's last heartbeat reported (model, version, version and
  variant IDs, quantization, runtime), and `last_model_event`, its newest
  `MODEL_LOAD`, `MODEL_LOAD_FAILED` or `SIGNATURE_INVALID` event (type, model,
  version, error, time), or `null`. The Linux agent reports a failed download
  as `MODEL_LOAD_FAILED` with the error, once per version; the SDKs report
  validation failures that way.

`GET /api/v1/deployments` and `GET /api/v1/devices` take `project_id`
(`fleetml deploy list --project`, `fleetml device list --project`).

### Artifact integrity

A variant is created as `UPLOADING`. `POST /variants/{id}/finalize` streams the
uploaded blob from storage and compares its sha256 with the declared
`blob_sha256`. On a mismatch (or a size mismatch) the blob is deleted, the
variant becomes `FAILED` and the API returns `422 INTEGRITY_ERROR`; upload the
right bytes and finalize again. Only `READY` variants are ever assigned, and a
deployment or experiment for a version without a READY variant is rejected with
`422 NOT_DEPLOYABLE`.

### Delta updates

When a variant becomes READY, the server diffs it against the previous READY
variant of the same model, runtime and quantization (bsdiff, with raw DEFLATE
sections; `internal/delta`) and keeps the delta if it is under half the
model's size. A heartbeat that sends a device a model it already has another
version of includes `delta_url`, `delta_size_bytes`, `delta_sha256` and
`delta_base_sha256`. If the device has the base version (its active or
last-known-good file has that sha256), the SDK or agent downloads the delta,
checks its sha256, applies it and checks the result against `blob_sha256`;
on any failure it downloads the full model. The signature check is unchanged,
since it covers `blob_sha256`.

A 20 MB model whose last 2% changed (a retrained head) goes over the wire as a
440 KB delta. A model retrained end to end changes nearly every byte, so its
delta is skipped and devices download the full file. Deltas are built for
models up to `DELTA_MAX_SIZE_MB` (128 MB by default; building one needs about
ten times the model's size in memory, and `0` turns deltas off), one at a time
in the background; a delta is offered only once it is built.

### Artifact signing

Every project has an ed25519 signing key, created with its first published
variant (or by `POST /projects/{id}/signing-key`). When a variant is finalized,
after its sha256 is checked, the server signs

```text
fleetml-artifact-v1\n<model_id>\n<version>\n<sha256, lowercase hex>
```

with the project's key and stores the signature. Devices receive the
project's public keys with `Register` and every heartbeat (`signing_keys`),
and each assignment carries `signature` and `signing_key_id`. Before
downloading or activating a model, the Android and iOS SDKs and the Linux
agent verify the signature; if it does not verify, they keep the current
version and report `SIGNATURE_INVALID`, which counts against a canary's health
gate like `MODEL_LOAD_FAILED`. A model file, hash or version changed after
publishing, in the object store or in the database, is therefore rejected on
the device.

The private keys are stored encrypted with a key derived from
`SIGNING_MASTER_KEY` ([CONFIG.md](CONFIG.md#required)); variants published
before signing existed are signed when the server starts.

```bash
api GET  /api/v1/projects/$P/signing-key
api POST /api/v1/projects/$P/signing-key/rotate '{"grace":"24h"}'
```

The GET lists the keys devices accept, the active one first.

Rotating creates a new active key and signs every READY variant again with
it, but only those whose current signature verifies with the old key. The old
key stays in `signing_keys` with `not_after` set to the end of the grace
period (`0s` to `720h`, default `24h`): until then devices still accept what it
signed, such as a download that started before the rotation; after it they
reject it, even offline.

## Strategies

Exposure uses one consistent-hash ring per deployment:
`position = first 4 bytes of sha256(device_id + deployment_id)`. A device keeps
its position for the life of the deployment, so a cohort only ever grows.

| Strategy | Who gets the new version |
|---|---|
| `ALL_AT_ONCE` | Every targeted device, immediately (the deployment is `COMPLETE` on start). |
| `CANARY` | Stage by stage: devices whose `position % 100` is below the current stage's `percentage`. A stage advances after its `min_duration` if the health gate passes; after the last stage the deployment is `COMPLETE`. Default stages: 1% (5m), 10% (15m), 50% (30m), 100%. |
| `ROLLING` | Stage N exposes N batches: N × `batch_size_percent` percent of the ring, or exactly N × `batch_size` devices (targeted devices sorted by ring position). A stage advances after `batch_interval` if the health gate passes; once every targeted device is exposed, the deployment completes one interval later. Default: `{"batch_size_percent": 10, "batch_interval": "5m"}`. |

`ROLLING` also honours `max_unavailable`: the rollout holds while more exposed
devices than allowed, seen in the last 5 minutes, have not reported the new
version loaded (0 disables the check).

The deploy loop runs every `DEPLOY_TICK_INTERVAL` (default 30s);
`POST /deployments/{id}/progress` evaluates one step immediately: it
advances once the health gate passes and the stage's minimum time
(`min_duration`, or `batch_interval` for a rolling deployment) has elapsed,
returns the deployment unchanged before that, and answers
`409 INVALID_TRANSITION` for a deployment that is not `ROLLING_OUT`.

## Health gate

Every `CANARY` and `ROLLING` deployment has a health gate. Without one in the
request it gets the default:

```json
{"max_error_rate": 0.05, "min_events": 20, "max_latency_regression": 0, "on_failure": "ROLLBACK"}
```

A gate that sets only some of these keys gets the default for the others
(`{"min_events": 30}` still allows a 5% error rate; `"max_error_rate": 0`
allows none). A key the request does not have, in `health_gate`,
`rolling_config`, a canary stage or the deployment body itself, is refused
with `400` naming it
(`invalid JSON body: health_gate: unknown key "max_error" (known: ...)`).
So is a setting of another strategy: `canary_stages` without
`"strategy": "CANARY"`, `rolling_config` without `"ROLLING"`, and a
`health_gate` on an `ALL_AT_ONCE` deployment, which has no stage to judge
(`canary_stages needs strategy CANARY (this deployment is ALL_AT_ONCE)`).

On each tick the gate looks only at telemetry for the new version **from the
devices the current stage exposes**, recorded **since the stage started**.
Errors from devices outside the cohort never count.

- **Errors** are events of type `ERROR`, `OOM` or `MODEL_LOAD_FAILED`, and any
  event with a non-empty `error_message`.
- With fewer than `min_events` events the gate does not judge, and the stage may
  advance on time. Set `min_events` to what your traffic can deliver in one stage.
- `error_rate > max_error_rate` fails the gate.
- `max_latency_regression` (e.g. `0.2`) fails the gate when the cohort's p95
  inference latency exceeds the previous version's p95 over the last 24 hours by
  more than that fraction (only when both sides have `min_events`).
- If any query fails, the rollout **holds**: it neither advances nor rolls back.
- `on_failure`: `ROLLBACK` (default) or `PAUSE`.

`GET /deployments/{id}/health` shows the effective gate, cohort size, stats and
the verdict; `can_progress` is true when `progress` would advance the stage
now, and while the stage's minimum time runs `reason` adds how much is left
(`...; stage 0 holds for its minimum time, 9m12s left`).

## Urgent deployments and the download policy

Devices download a new model only when their download policy allows it:
by default on an unmetered network (Wi-Fi, Ethernet) or while charging, set
for all devices with `DEVICE_DOWNLOAD_POLICY` ([CONFIG.md](CONFIG.md#devices)).
A deployment created with `"urgent": true` (`fleetml deploy create --urgent`,
or the dashboard's **Urgent** box) marks its assignments urgent, and devices
download them on any connected network straight away. Use it for fixes that
cannot wait for Wi-Fi; rollbacks are always urgent.

## Rollback, cancel and status

| From \ to | ROLLING_OUT | PAUSED | COMPLETE | ROLLED_BACK | CANCELLED |
|---|---|---|---|---|---|
| PENDING | start | | | | cancel |
| ROLLING_OUT | | pause | promote | rollback | cancel |
| PAUSED | resume | | promote | rollback | cancel |
| COMPLETE | | | | rollback | |

`ROLLED_BACK` and `CANCELLED` are final. `PATCH /deployments/{id}/status` goes
through the same table (with the same side effects as the dedicated endpoints)
and returns `409 INVALID_TRANSITION` for anything else.

- Creating a deployment records `previous_version_id`: the version of the newest
  `COMPLETE` deployment of the same model in the project.
- **Rollback** (automatic or `POST /deployments/{id}/rollback`) marks the
  deployment `ROLLED_BACK` and, if the previous version is known, creates a
  `COMPLETE` revert deployment of it with the same target selector. Devices get
  an `UPDATE` back to the previous version with its URL and sha256. The revert
  is urgent: devices fetch it on any network (usually they still have it
  cached as their last-known-good version).
- **Cancel** (`POST /deployments/{id}/cancel`) stops an unfinished deployment;
  its devices fall back to the previous active deployment of the model.

## A/B experiments

An experiment splits the targeted devices between a `control` and a
`treatment` model version (`split_ratio`, the treatment's share, 1 to 99 and
50 when left out; `traffic_percent`). Both versions belong to the
experiment's project, as a deployment's does: a version of another project
is refused with `400`, an unknown project or version with `404`. Assignment is
sticky (stored on first contact, derived from sha256 of device and experiment).

- Each device receives its arm's model like any other assignment (READY variant,
  URL, sha256, real version string) and an entry in `HeartbeatResponse.experiments`
  with the arm's `arm_id`.
- While an experiment runs it controls that model on its devices: deployments
  cannot change it, the arm's model is never unloaded, and the previously
  deployed variant is unloaded once the arm's model is loaded.
- Devices tag telemetry with `experiment_arm_id = arm_id`. Per-arm results count
  tagged events; untagged events fall back to the device's cohort.

### Statistics: fixed horizon

Results stay descriptive (`final: false`, `p_value: null`,
`recommendation: COLLECTING_DATA`) until **both** arms have `min_sample_size`
events. Then a single Welch t-test runs at α = 1 − `confidence_level` and the
result is frozen (`final: true`, `tested_at`); later data does not change it,
so repeated looks cannot inflate false positives. Metrics: `latency` (mean
inference latency), `error_rate` (Bernoulli, SD √(p(1−p))) and `confidence`
(measured SD). With `auto_conclude` the experiment concludes at that test: the
recommended arm wins if significant, control otherwise. At `max_duration`
(`"7d"`, `"36h"`) a running experiment concludes the same way.

## Alerts

Every alert is published on NATS and delivered to the project's alert sinks,
and to `ALERT_WEBHOOK_URL` if it is set.

| Event | NATS subject | When |
|---|---|---|
| `rollback` | `fleetml.alerts.rollback` | Any rollback; `trigger` is `health_gate` or `manual`. |
| `drift` | `fleetml.alerts.drift` | A drift result changes into `CRITICAL`. |

```json
{
  "id": "6f1c…",
  "type": "rollback",
  "occurred_at": "2026-10-08T09:30:12Z",
  "project_id": "…",
  "model": { "name": "detector", "version": "2.0.0" },
  "rollback": {
    "deployment_id": "…", "project_id": "…", "model_version_id": "…",
    "previous_version_id": "…", "strategy": "CANARY", "stage": 0,
    "trigger": "health_gate",
    "reason": "error rate 100.00% exceeds threshold 5.00% (30/30 events from 6 cohort devices)"
  }
}
```

```json
{
  "id": "374d…",
  "type": "drift",
  "occurred_at": "2026-10-08T09:31:40Z",
  "project_id": "…",
  "model": { "name": "detector", "version": "2.0.0" },
  "drift": {
    "model_version_id": "…", "metric": "latency", "psi": 44.77,
    "severity": "CRITICAL", "previous_severity": "NONE",
    "baseline_sample": 60, "current_sample": 60, "message": "Latency PSI=44.7700 (CRITICAL)"
  }
}
```

### Alert sinks

Each project has its own sinks, managed on the dashboard's **Alerts** page or
over REST:

| Kind | Config | Delivers |
|---|---|---|
| `webhook` | `url`, optional `secret` | The alert JSON above, signed with the secret |
| `slack` | `webhook_url` (a Slack incoming webhook) | A Block Kit message: what happened, the reason or PSI, and a link to the dashboard page (`DASHBOARD_URL`) |
| `email` | `to` (comma separated) | A plain-text mail through the server's SMTP settings (`SMTP_HOST`, `SMTP_FROM`; [CONFIG.md](CONFIG.md#alerts)) |

```bash
api POST /api/v1/projects/$P/alert-sinks \
  '{"name":"on-call","kind":"webhook","config":{"url":"https://example.com/fleetml","secret":"change-me"}}'
api POST /api/v1/projects/$P/alert-sinks \
  '{"name":"#ml-alerts","kind":"slack","config":{"webhook_url":"https://hooks.slack.com/services/…"}}'
api GET    /api/v1/projects/$P/alert-sinks
api PATCH  /api/v1/alert-sinks/$SINK '{"enabled":false}'
api POST   /api/v1/alert-sinks/$SINK/test
api GET    /api/v1/alert-sinks/$SINK/deliveries
api DELETE /api/v1/alert-sinks/$SINK
```

The list shows secrets redacted; `test` sends one test alert, without
retries, and returns the attempt; `deliveries` lists the last 50 attempts,
newest first.

A PATCH may send the redacted config back unchanged; redacted values keep
their stored secret.

Every sink, and `ALERT_WEBHOOK_URL`, gets up to five attempts per alert, after
1s, 2s, 4s and 8s. Each attempt is recorded in the sink's delivery log with its
status (`ok`, `error`) and error; when the fifth attempt fails it is recorded
as `dead` (the dead letter) and the alert is not tried again. The log keeps the
last 50 attempts per sink. `ALERT_WEBHOOK_URL` has no delivery log; its
failures are in the server log and in `fleetml_alerts_sent_total`.

Webhook requests carry `X-FleetML-Event` (the type), `X-FleetML-Delivery` (the
alert `id`, unchanged across retries, so receivers can deduplicate) and, when
the sink has a secret (`ALERT_WEBHOOK_SECRET` for `ALERT_WEBHOOK_URL`),
`X-FleetML-Signature: sha256=<hex HMAC-SHA256 of the raw body>`. Any non-2xx
response counts as a failed attempt. Verify a signature in Python:

```python
import hashlib, hmac
expected = "sha256=" + hmac.new(secret.encode(), raw_body, hashlib.sha256).hexdigest()
ok = hmac.compare_digest(expected, request.headers["X-FleetML-Signature"])
```

Drift is checked every `DRIFT_TICK_INTERVAL` (default 5m) for ACTIVE versions
(PSI of the last 24h against the 7 days before, at least 50 samples each) and on
`POST /drift/{model_version_id}/check`. An unchanged severity updates the latest
result in place; only a change of severity adds a row, and only a change into
CRITICAL sends an alert. Every check also records its PSI
(`GET /drift/{model_version_id}/points?days=7`), which the dashboard's Drift
page draws as PSI over time.

### Feature-level drift

Latency and confidence say that something changed; input features say what.
Declare a model's features and their bin edges:

```bash
api PUT /api/v1/models/$M/features \
  '{"features":[{"name":"petal_length","edges":[3,3.5,4,4.5,5,5.5,6]},{"name":"sepal_width","edges":[2.5,2.75,3,3.25,3.5]}]}'
```

Devices receive the edges in `DeviceConfig.feature_bins`. The app passes each
inference's input values by feature name (Android
`reportInference(model, ..., features = mapOf("petal_length" to floatArrayOf(5.1f)))`,
iOS `reportInference(model:..., features: ["petal_length": [5.1]])`, or
`"features"` on the Linux agent's local endpoint); the SDK bins them and sends
counts per bin, never the values themselves. Edges are strictly increasing
(1 to 50); `n` edges make `n + 1` bins, from below the first edge to at or
above the last. The drift check compares each feature's histogram of the last
24 hours with the 7 days before (at least 50 values each) and records it as
metric `feature:<name>`, with the same severities, deduplication and alerts
as latency and confidence.

## Runbook

These commands reproduce each behaviour against the dev stack. They need
`curl`, `jq`, `grpcurl` and `openssl`, and run from the repository root.

Start a webhook receiver that prints each alert (any endpoint that answers 2xx
works) and the server with fast loops:

```bash
python3 -c '
import http.server
class H(http.server.BaseHTTPRequestHandler):
    def do_POST(self):
        print(self.headers["X-FleetML-Event"], self.rfile.read(int(self.headers["Content-Length"])).decode(), flush=True)
        self.send_response(204); self.end_headers()
    def log_message(self, *a): pass
http.server.HTTPServer(("127.0.0.1", 18999), H).serve_forever()' &
DEPLOY_TICK_INTERVAL=5s ALERT_WEBHOOK_URL=http://127.0.0.1:18999/hook ALERT_WEBHOOK_SECRET=change-me make dev
```

In a second terminal, define a few helpers:

```bash
export FLEETML_API_KEY=$(grep '^FLEETML_ADMIN_KEY=' .env | cut -d= -f2)
api() {
  if [ -n "$3" ]; then curl -s -X "$1" -H "Authorization: Bearer $FLEETML_API_KEY" -H 'Content-Type: application/json' -d "$3" "localhost:${FLEETML_REST_PORT:-18080}$2"
  else curl -s -X "$1" -H "Authorization: Bearer $FLEETML_API_KEY" "localhost:${FLEETML_REST_PORT:-18080}$2"; fi
}
upload() {
  local out=$(api POST /api/v1/versions/$1/variants "{\"quantization\":\"INT8\",\"target_runtime\":\"TFLITE\",\"blob_size_bytes\":$(wc -c < "$2" | tr -d ' '),\"blob_sha256\":\"${3:-$(shasum -a 256 "$2" | cut -d' ' -f1)}\"}")
  curl -s -o /dev/null -X PUT --data-binary @"$2" "$(echo "$out" | jq -r .data.upload_url)"
  api POST /api/v1/variants/$(echo "$out" | jq -r .data.variant.id)/finalize
}
g() { grpcurl -plaintext -import-path proto -proto fleetml/v1/device.proto -proto fleetml/v1/telemetry.proto "$@"; }
register() { g -d "{\"fingerprint\":\"$(openssl rand -hex 32)\",\"os\":\"ANDROID\",\"ram_mb\":4096,\"enrollment_key\":\"$1\",\"labels\":$2}" \
  localhost:${FLEETML_GRPC_PORT:-19090} fleetml.device.v1.DeviceService/Register | jq -r '"\(.deviceId) \(.authToken)"'; }
heartbeat() { g -H "authorization: Bearer $1" -d "{\"loaded_models\":${2:-[]}}" localhost:${FLEETML_GRPC_PORT:-19090} fleetml.device.v1.DeviceService/Heartbeat; }
report() { local ev="{\"model_version_id\":\"$2\",\"type\":\"$4\",\"latency_ms\":20,\"experiment_arm_id\":\"${5:-}\"}"
  g -H "authorization: Bearer $1" -d "{\"events\":[$(for i in $(seq $3); do printf '%s,' "$ev"; done | sed 's/,$//')]}" \
  localhost:${FLEETML_GRPC_PORT:-19090} fleetml.telemetry.v1.TelemetryService/ReportBatch; }
```

`upload <version ID> <file> [sha256]` creates an INT8/TFLITE variant, PUTs the
blob and finalizes it; `g` is grpcurl with the FleetML protos (the server has
no reflection unless `GRPC_REFLECTION=true`); `register <enrollment key>
<labels JSON>` prints `<device ID> <token>`; `heartbeat <token> [loaded models
JSON]`; and `report <token> <version ID> <count> <type> [arm ID]` sends that
many telemetry events.

Create a project, a model with two versions and an enrollment key:

```bash
P=$(api POST /api/v1/projects '{"name":"runbook"}' | jq -r .data.id)
M=$(api POST /api/v1/projects/$P/models '{"name":"clf","framework":"TFLITE","task_type":"CLASSIFICATION"}' | jq -r .data.id)
V1=$(api POST /api/v1/models/$M/versions '{"version":"1.0.0"}' | jq -r .data.id)
V2=$(api POST /api/v1/models/$M/versions '{"version":"2.0.0"}' | jq -r .data.id)
head -c 4096 /dev/urandom > v1.bin; head -c 5000 /dev/urandom > v2.bin
upload $V1 v1.bin | jq -c '.data.status'
upload $V2 v2.bin | jq -c '.data.status'
EK=$(api POST /api/v1/projects/$P/enrollment-keys '{"name":"runbook"}' | jq -r .data.key)
```

Both uploads print `"READY"`.

**Integrity.** A wrong declared hash is rejected and the version cannot be deployed:

```bash
VBAD=$(api POST /api/v1/models/$M/versions '{"version":"9.9.9"}' | jq -r .data.id)
upload $VBAD v1.bin 0000000000000000000000000000000000000000000000000000000000000000 | jq -c .error.code
api POST /api/v1/deployments "{\"project_id\":\"$P\",\"model_version_id\":\"$VBAD\"}" | jq -c .error.code
```

The upload prints `"INTEGRITY_ERROR"`, the deployment `"NOT_DEPLOYABLE"`.

**Canary with the default gate and an automatic rollback.** Register six
devices, ship v1 to all, then start a v2 canary at 50% (stage held for an hour)
and make the canary devices report errors:

```bash
for i in 1 2 3 4 5 6; do register $EK '{"fleet":"runbook"}'; done > devices.txt
D1=$(api POST /api/v1/deployments "{\"project_id\":\"$P\",\"model_version_id\":\"$V1\",\"target_selector\":{\"fleet\":\"runbook\"}}" | jq -r .data.id)
api POST /api/v1/deployments/$D1/start | jq -r .data.status
D2=$(api POST /api/v1/deployments "{\"project_id\":\"$P\",\"model_version_id\":\"$V2\",\"strategy\":\"CANARY\",\"target_selector\":{\"fleet\":\"runbook\"},\"canary_stages\":[{\"percentage\":50,\"min_duration\":\"1h\"},{\"percentage\":100}]}" | jq -r .data.id)
api GET /api/v1/deployments/$D2 | jq -c .data.health_gate
api POST /api/v1/deployments/$D2/start >/dev/null
while read -r dev tok; do
  if heartbeat $tok | jq -e --arg v "$V2" '.assignments[]? | select(.versionId == $v)' >/dev/null; then
    report $tok $V2 10 ERROR >/dev/null; echo "canary device $dev reported 10 errors"
  fi
done < devices.txt
sleep 15; api GET /api/v1/deployments/$D2 | jq -c '.data | {status, rollback_reason}'
```

The deployment of v1 starts as `COMPLETE`, the canary shows the default
health gate, and 15 seconds after the errors the canary is `ROLLED_BACK` with
a rollback reason that starts with `error rate`. Devices that were in the canary are offered v1 again on their next heartbeat
(an `UPDATE` with v1's URL and sha256 when they report v2 loaded), and the
webhook receiver prints one `rollback` alert.

**Rolling in batches.** Twenty devices, five per batch, 30 seconds apart:

```bash
for i in $(seq 20); do register $EK '{"fleet":"rolling"}'; done > fleet.txt
D3=$(api POST /api/v1/deployments "{\"project_id\":\"$P\",\"model_version_id\":\"$V2\",\"strategy\":\"ROLLING\",\"target_selector\":{\"fleet\":\"rolling\"},\"rolling_config\":{\"batch_size\":5,\"batch_interval\":\"30s\"}}" | jq -r .data.id)
api POST /api/v1/deployments/$D3/start >/dev/null
last=""
while true; do
  before=$(api GET /api/v1/deployments/$D3 | jq -r '"\(.data.status) stage \(.data.current_stage)"')
  n=0; while read -r dev tok; do heartbeat $tok | jq -e --arg v "$V2" '.assignments[]? | select(.versionId == $v)' >/dev/null && n=$((n+1)); done < fleet.txt
  st=$(api GET /api/v1/deployments/$D3 | jq -r '"\(.data.status) stage \(.data.current_stage)"')
  [ "$st" = "$before" ] && [ "$st: $n" != "$last" ] && echo "$(date +%T) $st: $n/20 devices offered v2" && last="$st: $n"
  case "$st" in COMPLETE*) break;; esac
  sleep 5
done
```

It prints 5, 10, 15 and 20 of 20 devices offered v2, one batch at a time,
then `COMPLETE`.

**Manual rollback of a completed deployment** and **cancel**:

```bash
api POST /api/v1/deployments/$D3/rollback '{"reason":"runbook"}' | jq -r .data.status
api POST /api/v1/deployments/$D3/rollback | jq -r .error.code
D4=$(api POST /api/v1/deployments "{\"project_id\":\"$P\",\"model_version_id\":\"$V2\",\"strategy\":\"CANARY\"}" | jq -r .data.id)
api POST /api/v1/deployments/$D4/start >/dev/null; api POST /api/v1/deployments/$D4/cancel | jq -r .data.status
```

The first rollback prints `ROLLED_BACK`, the second `INVALID_TRANSITION`
(it is already rolled back), and the cancel `CANCELLED`.

**A/B experiment.** Devices learn their arm and download their arm's model;
tagged telemetry is attributed per arm:

```bash
E=$(api POST /api/v1/experiments "{\"project_id\":\"$P\",\"name\":\"runbook-ab\",\"control_version_id\":\"$V1\",\"treatment_version_id\":\"$V2\",\"target_selector\":{\"fleet\":\"ab\"},\"min_sample_size\":20}" | jq -r .data.id)
api POST /api/v1/experiments/$E/start >/dev/null
for i in 1 2 3 4 5 6; do
  read -r dev tok < <(register $EK '{"fleet":"ab"}')
  hb=$(heartbeat $tok); arm=$(echo "$hb" | jq -r '.experiments[0].armId'); ver=$(echo "$hb" | jq -r '.experiments[0].modelVersionId')
  echo "$(echo "$hb" | jq -r '.experiments[0].armName'): version $(echo "$hb" | jq -r --arg v "$ver" '.assignments[] | select(.versionId == $v) | .version')"
  report $tok $ver 25 INFERENCE $arm >/dev/null
done
sleep 6; api GET /api/v1/experiments/$E/results | jq -c '.data | {final, p_value, recommendation, control: .control.sample_size, treatment: .treatment.sample_size}'
```
