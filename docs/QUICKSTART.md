# Quickstart and tour

Run FleetML in containers, then walk through every feature against it with a
fleet of simulated devices. Each block below was run as written, in order.

## Run FleetML

Needs Docker with Compose v2 and `openssl`. The dashboard uses port 80; the
APIs use 18080 (REST) and 19090 (gRPC); model files are served on 19100.

```bash
git clone https://github.com/fleetml/fleetml.git && cd fleetml
export JWT_SECRET=$(openssl rand -base64 32) SIGNING_MASTER_KEY=$(openssl rand -base64 32) FLEETML_ADMIN_KEY=$(openssl rand -hex 32)
docker compose -f deploy/docker-compose.prod.yml up -d --build
until curl -sf localhost:18080/readyz; do sleep 2; done; echo
curl -s -H "Authorization: Bearer $FLEETML_ADMIN_KEY" localhost:18080/api/v1/projects; echo
echo "Dashboard: http://localhost  (log in with the admin API key: $FLEETML_ADMIN_KEY)"
```

`/readyz` answers `{"db":"connected","nats":"connected","s3":"connected","status":"ok"}`
once Postgres, the object store and NATS are reachable. The admin key is
stored on the first start; later starts ignore `FLEETML_ADMIN_KEY` (manage keys
with `/api/v1/api-keys`).

## Tour

The tour needs Go 1.24 and `jq`, and runs from the repository root in the same
shell (it uses `FLEETML_ADMIN_KEY`). It builds the CLI, then drives the server
with it and with the REST API. The blocks are written to paste into bash or zsh
as they are: what a command prints is given in the text, not in `#` comments,
which zsh at the prompt passes to the command as arguments.

```bash
go build -o bin/fleetml ./cmd/fleetml
export FLEETML_SERVER=http://localhost:18080 FLEETML_API_KEY=$FLEETML_ADMIN_KEY
api() { curl -sf -X "$1" -H "Authorization: Bearer $FLEETML_API_KEY" -H 'Content-Type: application/json' -d "$3" "localhost:18080$2"; }
```

### 1. Publish a model

`fleetml push` creates the model and version if needed, uploads the file
straight to the object store with a presigned URL, and asks the server to
verify it. Three versions of the sample Iris classifier
([models/iris](../models/iris)):

```bash
P=$(api POST /api/v1/projects '{"name":"tour"}' | jq -r .data.id)
./bin/fleetml push models/iris/iris_v1.tflite --project $P --model iris --version 1.0.0
./bin/fleetml push models/iris/iris_v2.tflite --project $P --model iris --version 2.0.0
./bin/fleetml push models/iris/iris_v2.tflite --project $P --model iris --version 2.1.0
M=$(api GET /api/v1/projects/$P/models | jq -r '.data[0].id')
vid() { api GET /api/v1/models/$M/versions | jq -r --arg v "$1" '.data[] | select(.version == $v) | .id'; }
V1=$(vid 1.0.0); V2=$(vid 2.0.0); V21=$(vid 2.1.0)
```

An upload that does not match its declared sha256 is refused and never
reaches a device:

```bash
OUT=$(api POST /api/v1/versions/$V2/variants "{\"quantization\":\"FP16\",\"target_runtime\":\"TFLITE\",\"blob_size_bytes\":$(wc -c < models/iris/iris_v1.tflite | tr -d ' '),\"blob_sha256\":\"$(printf '0%.0s' $(seq 64))\"}")
curl -s -o /dev/null -X PUT --data-binary @models/iris/iris_v1.tflite "$(echo "$OUT" | jq -r .data.upload_url)"
curl -s -X POST -H "Authorization: Bearer $FLEETML_API_KEY" localhost:18080/api/v1/variants/$(echo "$OUT" | jq -r .data.variant.id)/finalize | jq -c .error
```

The finalize call answers `{"code":"INTEGRITY_ERROR","message":"..."}`.

### 2. A fleet and a rollout

`fleetml simulate` runs devices on the real device protocol: they register
with the project's enrollment key, heartbeat, download and verify what they
are assigned, and report telemetry. Here 2.0.0 fails 40% of inferences, 2.1.0
is faster, and each device also reports a week of past telemetry as a drift
baseline:

```bash
EK=$(api POST /api/v1/projects/$P/enrollment-keys '{"name":"tour"}' | jq -r .data.key)
./bin/fleetml simulate --enrollment-key $EK --devices 20 --label app=tour \
  --error-rate 2.0.0=0.4 --latency 2.1.0=14 --history 168h > /tmp/fleetml-sim.log 2>&1 &
SIM=$!
D1=$(api POST /api/v1/deployments "{\"project_id\":\"$P\",\"model_version_id\":\"$V1\",\"target_selector\":{\"app\":\"tour\"}}" | jq -r .data.id)
api POST /api/v1/deployments/$D1/start | jq -r .data.status
sleep 5; tail -1 /tmp/fleetml-sim.log
./bin/fleetml device list | head -5
```

The deployment goes to all devices at once, so it starts as `COMPLETE`, and
the simulator's last line reads `iris 1.0.0 on 20`.

### 3. A canary that rolls itself back

A canary of 2.0.0 for 25% of the fleet, with a health gate: more than 5% errors
over at least 30 events from the canary cohort rolls it back. The deploy loop
runs every 30 seconds:

```bash
D2=$(api POST /api/v1/deployments "{\"project_id\":\"$P\",\"model_version_id\":\"$V2\",\"strategy\":\"CANARY\",\"target_selector\":{\"app\":\"tour\"},
  \"canary_stages\":[{\"percentage\":25,\"min_duration\":\"10m\"},{\"percentage\":100,\"min_duration\":\"0s\"}],
  \"health_gate\":{\"max_error_rate\":0.05,\"min_events\":30,\"on_failure\":\"ROLLBACK\"}}" | jq -r .data.id)
api POST /api/v1/deployments/$D2/start | jq -r .data.status
sleep 5; tail -1 /tmp/fleetml-sim.log
until [ "$(api GET /api/v1/deployments/$D2 | jq -r .data.status)" = ROLLED_BACK ]; do sleep 5; done
api GET /api/v1/deployments/$D2 | jq -r .data.rollback_reason
sleep 5; tail -1 /tmp/fleetml-sim.log
```

The deployment starts as `ROLLING_OUT` and the simulator shows the cohort on
2.0.0. Once the gate trips, the rollback reason reads `error rate ... exceeds
threshold 5.00% ...` and the simulator is back to `iris 1.0.0 on 20`.

The rollback also creates a deployment that returns the cohort to 1.0.0, and
sends a rollback alert (NATS subject `fleetml.alerts.rollback`, the
project's alert sinks from the dashboard's Alerts page, and the webhook if
`ALERT_WEBHOOK_URL` is set). Rolling deployments
(`"strategy":"ROLLING"`, `"rolling_config":{"batch_size_percent":20,"batch_interval":"1m"}`)
use the same gate; see [ROLLOUTS.md](ROLLOUTS.md).

### 4. An A/B experiment

Half the fleet runs 1.0.0, half 2.1.0. The test runs once both arms have 300
inferences:

```bash
EXP=$(api POST /api/v1/experiments "{\"project_id\":\"$P\",\"name\":\"iris 2.1.0 latency\",\"control_version_id\":\"$V1\",\"treatment_version_id\":\"$V21\",\"target_selector\":{\"app\":\"tour\"},\"min_sample_size\":300}" | jq -r .data.id)
api POST /api/v1/experiments/$EXP/start | jq -r .data.status
until [ "$(api GET /api/v1/experiments/$EXP/results | jq -r .data.final)" = true ]; do sleep 3; done
api GET /api/v1/experiments/$EXP/results | jq '.data | {control_ms: .control.avg_latency_ms, treatment_ms: .treatment.avg_latency_ms, p_value, recommendation}'
api POST /api/v1/experiments/$EXP/conclude "{\"winner_version_id\":\"$V21\"}" | jq -r .data.status
```

The experiment is `RUNNING` once started and `CONCLUDED` at the end. Each
device keeps its arm for the experiment's life, and its telemetry carries the
arm ID, so results are per arm even when a device's arm model changes.

### 5. Drift

Drift compares the last 24 hours of a version's inference latency and
confidence with the 7 days before (PSI). Against the week of history the
devices reported, 1.0.0 looks stable:

```bash
./bin/fleetml drift check $V1
```

Now the devices get slower:

```bash
kill $SIM; sleep 1
./bin/fleetml simulate --enrollment-key $EK --devices 20 --label app=tour --latency 1.0.0=34 > /tmp/fleetml-sim.log 2>&1 &
SIM=$!
sleep 20
./bin/fleetml drift check $V1
api GET /api/v1/drift/$V1/history | jq -r '.data[] | "\(.checked_at) \(.metric) \(.severity)"'
```

The second check reports latency as CRITICAL. The drift loop runs every 5
minutes (`DRIFT_TICK_INTERVAL`); a change into CRITICAL sends a drift alert.

### 6. Dashboard

Open <http://localhost> and log in with the admin key
(`echo $FLEETML_ADMIN_KEY`); the dashboard keeps a session cookie, not the key. **Models** shows iris with its three versions and
their variants (one FAILED from step 1), **Deployments** the rolled-back
canary with its reason, **Experiments** the concluded A/B test with per-arm
results, and **Drift** the history for 1.0.0. Deployments and experiments can
be created, rolled back, cancelled and concluded there too.

![Deployments](screenshots/dashboard-1280-deployments.png)

### 7. Operations

```bash
curl -s localhost:18080/livez; echo
curl -s localhost:18080/readyz; echo
curl -s localhost:18080/metrics | grep -E '^fleetml_(deployments|alerts_sent_total|telemetry_events_accepted_total)'
```

`/livez` answers `{"status":"ok"}`. The gRPC port also serves the standard
`grpc.health.v1` health service.

Prometheus and Grafana with the FleetML dashboard (telemetry lag,
redeliveries, request rates, deployments, alerts) run as a compose profile:

```bash
docker compose -f deploy/docker-compose.prod.yml --profile observability up -d
open http://localhost:13000
```

Grafana opens on port 13000 with the "FleetML" dashboard.

Run it in the same shell as the stack, since compose needs the same secrets.
It runs the one-shot `migrate` service again and restarts the server once; the
data stays.

With the dev stack (`make dev`, server on the host), use
`docker compose -f deploy/docker-compose.yml --profile observability up -d`;
Prometheus is then on `:19091` and scrapes the server on `:18080`.

### Clean up

```bash
kill $SIM
docker compose -f deploy/docker-compose.prod.yml down -v
```
