# FleetML

**Fleet management for on-device ML.**

FleetML is an open-source, runtime-agnostic, self-hosted-first platform for managing the complete lifecycle of ML models on edge devices, from model registry and versioning through OTA deployment, A/B testing, observability, and drift detection.

## Features

- **Model Registry**: Version, store, and manage model variants (FP32, FP16, INT8, INT4) across frameworks (TFLite, ONNX, CoreML, GGUF); uploads are verified against their declared sha256 before a variant can be deployed
- **Intelligent Variant Selection**: Automatically selects the optimal model variant based on device hardware capabilities
- **Device Fleet Management**: Register, classify, and monitor edge devices with capability scoring (0-100)
- **Deployment Strategies**: ALL_AT_ONCE, CANARY (staged, consistent hashing) and ROLLING (batches on the same hash ring, `max_unavailable`) with label targeting. Each device runs one version per model: the newest deployment that reaches it. See [docs/ROLLOUTS.md](docs/ROLLOUTS.md)
- **Health gate and rollback**: on by default for canary and rolling (error rate over the current cohort's stage window, optional p95 latency regression against the previous version); a failing gate rolls back automatically. Manual rollback works after completion too and returns devices to the previous version; deployments can be cancelled
- **A/B Experiments**: sticky cohorts, each arm's model delivered with a verified download, telemetry attributed per arm, and a fixed-horizon Welch t-test (no peeking) with `max_duration`
- **Telemetry**: Device events over gRPC, through NATS into a TimescaleDB hypertable, with summary and time-bucketed series endpoints
- **Drift Detection**: PSI on latency and confidence distributions per model version; a change into CRITICAL sends an alert
- **Alerts**: rollback and drift alerts on NATS (`fleetml.alerts.*`) and an HMAC-signed, retried webhook
- **Dashboard**: React UI for monitoring devices, deployments, experiments, telemetry, drift
- **CLI**: `fleetml push`, `fleetml deploy`, `fleetml experiment`, `fleetml drift`
- **Android SDK** ([guide](docs/ANDROID_SDK.md)): Kotlin, minSdk 26: register with an enrollment key, resumable sha256-verified downloads, validation before activation, last-known-good, offline start, persistent telemetry queue
- **iOS SDK** ([guide](docs/IOS_SDK.md)): Swift package, iOS 15: the same lifecycle with Core ML validation and Keychain credentials
- **Security**: admin API keys on the REST API, per-project device enrollment keys, revocable and refreshable device tokens, locked-down CORS (see [SECURITY.md](SECURITY.md))

## Quick Start

**Prerequisites**

- [Docker](https://docs.docker.com/get-docker/) with Compose v2.20+ (running)
- [Go 1.24+](https://go.dev/dl/)
- [grpcurl](https://github.com/fullstorydev/grpcurl) and [jq](https://jqlang.github.io/jq/), to register a device from a terminal
- [golang-migrate](https://github.com/golang-migrate/migrate) CLI, only for manual `make migrate-up` / `make migrate-down` (the quickstart runs migrations in Compose)
- [Node.js 20+](https://nodejs.org/), only for the dashboard dev server

**1. Run the server**

```bash
git clone https://github.com/fleetml/fleetml.git && cd fleetml
make dev    # creates .env (random JWT_SECRET and FLEETML_ADMIN_KEY) if missing, starts Postgres/TimescaleDB,
            # S3 storage (SeaweedFS), NATS, applies DB migrations, then runs the server
            # on REST :18080 and gRPC :19090
```

**2. Get your admin API key** (second terminal, same directory)

Every `/api/v1` call needs `Authorization: Bearer <admin key>`. On first start the server stores `FLEETML_ADMIN_KEY` from `.env` as the bootstrap key:

```bash
export FLEETML_API_KEY=$(grep '^FLEETML_ADMIN_KEY=' .env | cut -d= -f2)
curl localhost:18080/healthz                                                  # {"db":"connected",...,"status":"ok"}
curl -s -H "Authorization: Bearer $FLEETML_API_KEY" localhost:18080/api/v1/projects   # 200; without the header: 401
```

If your `.env` predates `FLEETML_ADMIN_KEY`, the server generated a key on first start and logged it once: look for `admin_api_key` in its output.

**3. Create a project and an enrollment key, then register a device**

Devices can only register with a per-project enrollment key. The key is shown once.

```bash
AUTH="Authorization: Bearer $FLEETML_API_KEY"
PROJECT_ID=$(curl -s -H "$AUTH" -H 'Content-Type: application/json' -d '{"name":"demo"}' \
  localhost:18080/api/v1/projects | jq -r .data.id)
ENROLL_KEY=$(curl -s -H "$AUTH" -H 'Content-Type: application/json' -d '{"name":"quickstart"}' \
  localhost:18080/api/v1/projects/$PROJECT_ID/enrollment-keys | jq -r .data.key)

# Register a device the way the SDK does (fingerprint = 64 hex chars)
grpcurl -plaintext -import-path proto -proto fleetml/v1/device.proto -d "{\"fingerprint\":\"$(openssl rand -hex 32)\",\"os\":\"ANDROID\",\"enrollment_key\":\"$ENROLL_KEY\"}" \
  localhost:19090 fleetml.device.v1.DeviceService/Register             # returns deviceId, authToken, tokenExpiresAt

curl -s -H "$AUTH" localhost:18080/api/v1/devices | jq '.data[] | {id, os, project_id}'
```

**4. Dashboard and CLI**

```bash
make dashboard-dev                          # http://localhost:5173 → Settings → paste the admin key
go run ./cmd/fleetml config set-key "$FLEETML_API_KEY"   # or keep FLEETML_API_KEY exported
go run ./cmd/fleetml project list
```

In the Android SDK, pass the enrollment key as `FleetMLConfig(enrollmentKey = ...)`; the sample app reads `fleetml.enrollmentKey` from `android/local.properties`. In the iOS SDK it is `FleetMLConfig(serverHost:enrollmentKey:)`; see [docs/IOS_SDK.md](docs/IOS_SDK.md).

**Containers only** (server + dashboard): `export JWT_SECRET=$(openssl rand -base64 32)`, then `docker compose -f deploy/docker-compose.prod.yml up -d --build`. The API is on `:18080` and the dashboard on `:80`. Set `FLEETML_ADMIN_KEY` in your shell before the first start, or read the generated key with `docker compose -f deploy/docker-compose.prod.yml logs server | grep admin_api_key`.

### Credentials at a glance

| Credential | Who uses it | Create / manage |
|---|---|---|
| Admin API key | Dashboard, CLI, scripts (`Authorization: Bearer`) | `POST/GET /api/v1/api-keys`, `POST /api/v1/api-keys/{id}/revoke` |
| Enrollment key | A device's first `Register` (per project) | `POST/GET /api/v1/projects/{id}/enrollment-keys`, `POST /api/v1/enrollment-keys/{id}/rotate`, `.../revoke` |
| Device token | Every device RPC after `Register` (30 days) | `RefreshToken` RPC in the last 7 days; `POST /api/v1/devices/{id}/revoke` |

## Configuration

The server is configured with environment variables; [docs/CONFIG.md](docs/CONFIG.md) lists every one, for the server, Docker Compose, the CLI, the dashboard and the test scripts. `.env.example` is a ready-to-use dev set. The essentials:

| Variable | Default | Description |
|---|---|---|
| `DATABASE_URL` | *(required)* | Postgres/TimescaleDB connection string |
| `S3_ACCESS_KEY`, `S3_SECRET_KEY` | *(required)* | Object store credentials; `S3_ENDPOINT` defaults to `localhost:19000` |
| `JWT_SECRET` | *(required)* | Signs device tokens; at least 32 characters |
| `FLEETML_ADMIN_KEY` | *(generated)* | First admin API key; if unset, one is generated and logged once |
| `REST_PORT`, `GRPC_PORT` | `18080`, `19090` | REST API and device gRPC API |
| `NATS_URL` | `nats://localhost:14222` | Telemetry pipeline and alerts |
| `ALERT_WEBHOOK_URL`, `ALERT_WEBHOOK_SECRET` | *(unset)* | Signed webhook for rollback and drift alerts |

The server checks its configuration at start-up and names every missing or invalid variable. Operations endpoints: `GET /livez`, `GET /readyz` (Postgres, S3 and NATS reachable), `GET /metrics` (Prometheus) on the REST port, and `grpc.health.v1` on the gRPC port.

## Architecture

```
Android/iOS: gRPC --> Go Server ──NATS--> TimescaleDB
Dashboard  : REST --> (REST+gRPC)        S3 (SeaweedFS)
CLI        : REST -->
```

## Security

Please report vulnerabilities privately; see [SECURITY.md](SECURITY.md). FleetML does not terminate TLS itself: put the REST, gRPC and storage ports behind a TLS proxy before exposing them.

## License

Apache 2.0
