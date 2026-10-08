# FleetML

Safe OTA rollouts, A/B experiments and drift detection for ML models running
on devices, self-hosted, runtime-agnostic.

FleetML decides which device runs which version of your model, ships the file,
and watches what happens. Devices download a version only when a deployment or
experiment targets them, verify its sha256, check that it loads before using
it, and keep the previous version if it does not. A canary whose cohort starts
failing rolls itself back. Your app runs the model with TensorFlow Lite,
Core ML, ONNX Runtime or anything else; FleetML moves files and telemetry, not
inferences.

## The 60-second story

<!-- GIF: a screen recording of ./scripts/demo.sh goes here (docs/demo.gif). -->

[`scripts/demo.sh`](scripts/demo.sh) tells it against a real server and twenty
simulated devices that use the same protocol as the SDKs (needs Docker, Go
and jq; 47 seconds from a fresh stack):

```text
▶ 1. Publish three versions of a model
  ✓ Pushed models/iris/iris_v1.tflite → iris v1.0.0 (INT8)          (2.0.0 and 2.1.0 too)
▶ 2. Twenty devices enroll and get 1.0.0
  fleet: iris 1.0.0 on 20
▶ 3. Drift: the last day's latency against the week before
  latency drift for 1.0.0: NONE (PSI 0.044)
  now the devices slow down (say, after an OS update)...
  latency drift for 1.0.0: CRITICAL (PSI 8.511)
  a change into CRITICAL is sent as an alert: 1 drift alert(s) on NATS
▶ 4. A canary of 2.0.0 with a health gate; 2.0.0 is broken
  fleet: iris 1.0.0 on 11, iris 2.0.0 on 9   (the canary cohort only)
  deployment ROLLED_BACK: error rate 32.18% exceeds threshold 5.00% (56/174 events from 9 cohort devices)
  fleet: iris 1.0.0 on 20, nobody outside the canary ever saw 2.0.0
▶ 5. A/B test: 1.0.0 against the fixed 2.1.0
  control 1.0.0:   450 inferences, mean 34 ms
  treatment 2.1.0: 450 inferences, mean 14 ms
  Welch t-test p < 0.0001, recommendation: ADOPT_TREATMENT
  fleet: iris 2.1.0 on 20
```

## Features

| Feature | Status | See it |
|---|---|---|
| **Model registry with integrity checks.** Files go straight to the S3-compatible store with presigned URLs; the server checks each upload's sha256 and size before the variant can be deployed | DONE | [Tour §1](docs/QUICKSTART.md#1-publish-a-model), e2e `TestE2E_ArtifactIntegrity` |
| **Variant selection.** Several builds per version (runtime, quantization); each device gets one it can run, by OS runtime, RAM and capability score | DONE | [Tour §2](docs/QUICKSTART.md#2-a-fleet-and-a-rollout), [ROLLOUTS.md](docs/ROLLOUTS.md) |
| **Canary rollouts with a health gate.** Staged percentages on a consistent-hash ring; the gate compares the exposed cohort's error rate (and optionally p95 latency against the previous version) with thresholds and rolls back or pauses | DONE | [Tour §3](docs/QUICKSTART.md#3-a-canary-that-rolls-itself-back), e2e `TestE2E_HealthGateIsCohortScoped` |
| **Rolling rollouts.** Batches by device count or percentage at an interval, with the same gate | DONE | e2e `TestE2E_RollingBatches` |
| **Rollback, cancel, one version per model.** Manual rollback (also after completion) returns devices to the previous version; the newest deployment that reaches a device wins | DONE | e2e `TestE2E_RollbackFromComplete` |
| **A/B experiments.** Sticky cohorts, each arm's model delivered and verified, telemetry attributed per arm, a fixed-horizon Welch t-test and a maximum duration | DONE | [Tour §4](docs/QUICKSTART.md#4-an-ab-experiment), e2e `TestE2E_ABExperiment` |
| **Drift detection.** PSI on inference latency and confidence, last 24 hours against the 7 days before, with history; a change into CRITICAL raises an alert | DONE | [Tour §5](docs/QUICKSTART.md#5-drift) |
| **Alerts.** Rollback and drift alerts on NATS and to an HMAC-signed webhook with retries | DONE | [ROLLOUTS.md](docs/ROLLOUTS.md#alerts) |
| **Android SDK.** Kotlin, minSdk 26: resumable verified downloads, validation before activation, last-known-good, offline start, persistent telemetry | DONE | [ANDROID_SDK.md](docs/ANDROID_SDK.md), instrumented `LiveServerTest` |
| **iOS SDK.** Swift package, iOS 15: the same lifecycle with Core ML validation and Keychain credentials | DONE | [IOS_SDK.md](docs/IOS_SDK.md) |
| **Battery and data.** Telemetry batched in a persistent queue, model downloads on Wi-Fi only by default, heartbeat and telemetry intervals set by the server | DONE | [ANDROID_SDK.md](docs/ANDROID_SDK.md#telemetry), [IOS_SDK.md](docs/IOS_SDK.md#telemetry) |
| **Background scheduling and charging-aware downloads** in the SDKs | PLANNED | [Roadmap](docs/ROADMAP.md#next) |
| **Dashboard.** Models, versions and variants; create and control deployments and experiments; drift history; works on phones | DONE | [Tour §6](docs/QUICKSTART.md#6-dashboard), [screenshots](docs/screenshots) |
| **CLI.** `fleetml push`, `deploy`, `experiment`, `drift`, and `simulate` for a fleet of simulated devices | DONE | [Tour](docs/QUICKSTART.md#tour) |
| **Security.** Admin API keys, per-project enrollment keys, revocable device tokens, devices confined to their project | DONE | [SECURITY.md](SECURITY.md), e2e `TestE2E_ProjectIsolation` |
| **Operations.** `/livez`, `/readyz`, Prometheus `/metrics`, `grpc.health.v1` | DONE | [Tour §7](docs/QUICKSTART.md#7-operations), e2e `TestE2E_OpsEndpoints` |
| **Linux device agent** for Jetson, Raspberry Pi and x86 edge boxes | PLANNED | [Roadmap](docs/ROADMAP.md#next) |
| **Artifact signing** verified on the device | PLANNED | [Roadmap](docs/ROADMAP.md#later) |
| **Feature-level drift** from device-side input histograms | PLANNED | [Roadmap](docs/ROADMAP.md#later) |

The end-to-end tests live in [test/e2e](test/e2e) and run with `make e2e`.

## Quickstart

Needs Docker with Compose v2 and `openssl`. The dashboard uses port 80; the
APIs use 18080 (REST) and 19090 (gRPC).

```bash
git clone https://github.com/fleetml/fleetml.git && cd fleetml
export JWT_SECRET=$(openssl rand -base64 32) FLEETML_ADMIN_KEY=$(openssl rand -hex 32)
docker compose -f deploy/docker-compose.prod.yml up -d --build
until curl -sf localhost:18080/readyz; do sleep 2; done; echo
curl -s -H "Authorization: Bearer $FLEETML_ADMIN_KEY" localhost:18080/api/v1/projects
echo "Dashboard: http://localhost  (Settings → admin API key: $FLEETML_ADMIN_KEY)"
```

Measured on a laptop, cold: **QUICKSTART_TIME** from `git clone` to a working
UI and API, with no FleetML images or build cache present.

Next:

- **See it work:** `./scripts/demo.sh` (Go and jq).
- **Take the tour:** [docs/QUICKSTART.md](docs/QUICKSTART.md) walks through
  publishing, rollouts, a canary rollback, an A/B test, drift and the
  dashboard against these containers.
- **Connect an app:** [Android](docs/ANDROID_SDK.md) or [iOS](docs/IOS_SDK.md).
- **Develop:** `make dev` runs the server from source against the dev stack;
  see [CONTRIBUTING.md](CONTRIBUTING.md).

To stop: `docker compose -f deploy/docker-compose.prod.yml down` (add `-v` to
delete the data).

## How it works

```
Android / iOS SDK ──gRPC :19090──▶ FleetML server ──▶ Postgres + TimescaleDB
Dashboard, CLI ─────REST :18080──▶  (Go)           ──▶ NATS (telemetry, alerts)
        └──────── presigned URLs ─────────────────────▶ S3-compatible object store
```

Devices heartbeat with the models they run and receive assignments: what to
load, update or unload, with presigned download URLs and their experiment
arms. Three background loops advance rollouts through the health gate,
evaluate experiments and check drift. [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md)
has the request paths, data model, device protocol and trust boundaries.

| Credential | Who uses it | Manage |
|---|---|---|
| Admin API key | Dashboard, CLI, scripts (`Authorization: Bearer`) | `POST/GET /api/v1/api-keys`, `POST /api/v1/api-keys/{id}/revoke` |
| Enrollment key | An app's first device registration, per project | `POST/GET /api/v1/projects/{id}/enrollment-keys`, `POST /api/v1/enrollment-keys/{id}/rotate`, `.../revoke` |
| Device token | Every device call after registering (30 days, refreshed by the SDK) | `POST /api/v1/devices/{id}/revoke` |

## Configuration

The server is configured with environment variables; [docs/CONFIG.md](docs/CONFIG.md)
lists every one, for the server, Docker Compose, the CLI, the dashboard and
the test scripts. The essentials:

| Variable | Default | Description |
|---|---|---|
| `DATABASE_URL` | *(required)* | Postgres/TimescaleDB connection string |
| `S3_ACCESS_KEY`, `S3_SECRET_KEY` | *(required)* | Object store credentials; `S3_ENDPOINT` defaults to `localhost:19000` |
| `S3_PUBLIC_ENDPOINT` | *(unset)* | Where devices and the CLI reach the object store, if not `S3_ENDPOINT` |
| `JWT_SECRET` | *(required)* | Signs device tokens; at least 32 characters |
| `FLEETML_ADMIN_KEY` | *(generated)* | First admin API key; if unset, one is generated and logged once |
| `REST_PORT`, `GRPC_PORT` | `18080`, `19090` | REST API and device gRPC API |
| `NATS_URL` | `nats://localhost:14222` | Telemetry pipeline and alerts |
| `ALERT_WEBHOOK_URL`, `ALERT_WEBHOOK_SECRET` | *(unset)* | Signed webhook for rollback and drift alerts |

The server checks its configuration at start-up and names every missing or
invalid variable.

## Documentation

| | |
|---|---|
| [Quickstart and tour](docs/QUICKSTART.md) | Run it, then every feature step by step |
| [Rollouts, experiments and alerts](docs/ROLLOUTS.md) | Strategies, the health gate, statistics, webhooks |
| [Architecture](docs/ARCHITECTURE.md) | Components, request paths, data model, trust boundaries |
| [Configuration](docs/CONFIG.md) | Every environment variable |
| [Android SDK](docs/ANDROID_SDK.md), [iOS SDK](docs/IOS_SDK.md) | Integration guides and device runbooks |
| [Roadmap](docs/ROADMAP.md) | What comes next |

## Contributing, security and license

Contributions are welcome: see [CONTRIBUTING.md](CONTRIBUTING.md) and the
[Code of Conduct](CODE_OF_CONDUCT.md). Please report vulnerabilities privately
as described in [SECURITY.md](SECURITY.md); FleetML does not terminate TLS
itself, so put its ports behind a TLS proxy before exposing them.

FleetML is licensed under the [Apache License 2.0](LICENSE).
