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
- **Android SDK**: Kotlin library: fingerprint, register with an enrollment key, heartbeat, token refresh, download with sha256 check, cache, telemetry
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
make dev    # creates .env (with a random FLEETML_ADMIN_KEY) if missing, starts Postgres/TimescaleDB,
            # S3 storage (SeaweedFS), NATS, Redis, applies DB migrations, then runs the server
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
grpcurl -plaintext -d "{\"fingerprint\":\"$(openssl rand -hex 32)\",\"os\":\"ANDROID\",\"enrollment_key\":\"$ENROLL_KEY\"}" \
  localhost:19090 fleetml.device.v1.DeviceService/Register             # returns deviceId, authToken, tokenExpiresAt

curl -s -H "$AUTH" localhost:18080/api/v1/devices | jq '.data[] | {id, os, project_id}'
```

**4. Dashboard and CLI**

```bash
make dashboard-dev                          # http://localhost:5173 → Settings → paste the admin key
go run ./cmd/fleetml config set-key "$FLEETML_API_KEY"   # or keep FLEETML_API_KEY exported
go run ./cmd/fleetml project list
```

In the Android SDK, pass the enrollment key as `FleetMLConfig(enrollmentKey = ...)`; the sample app reads `fleetml.enrollmentKey` from `android/local.properties`.

**Containers only** (server + dashboard): `docker compose -f deploy/docker-compose.prod.yml up -d --build`. The API is on `:18080` and the dashboard on `:80`. Set `FLEETML_ADMIN_KEY` in your shell before the first start, or read the generated key with `docker compose -f deploy/docker-compose.prod.yml logs server | grep admin_api_key`.

### Credentials at a glance

| Credential | Who uses it | Create / manage |
|---|---|---|
| Admin API key | Dashboard, CLI, scripts (`Authorization: Bearer`) | `POST/GET /api/v1/api-keys`, `POST /api/v1/api-keys/{id}/revoke` |
| Enrollment key | A device's first `Register` (per project) | `POST/GET /api/v1/projects/{id}/enrollment-keys`, `POST /api/v1/enrollment-keys/{id}/rotate`, `.../revoke` |
| Device token | Every device RPC after `Register` (30 days) | `RefreshToken` RPC in the last 7 days; `POST /api/v1/devices/{id}/revoke` |

## Configuration

The server reads its configuration from environment variables (`.env.example` has a ready-to-use dev set). Default ports: REST **18080**, gRPC **19090**, infrastructure on **15xxx/14xxx/16xxx/18xxx/19xxx** host ports so nothing collides with common 8080/5432 users.

| Variable | Default | Description |
|---|---|---|
| `DATABASE_URL` | *(required)* | Postgres/TimescaleDB DSN, e.g. `postgres://fleetml:fleetml_dev@localhost:15432/fleetml?sslmode=disable` |
| `S3_ENDPOINT` | `localhost:19000` | S3-compatible endpoint as `host:port` (no scheme); the dev stack runs SeaweedFS |
| `S3_BUCKET` | `fleetml-models` | Bucket for model blobs (created on server start if missing) |
| `S3_ACCESS_KEY` | *(required)* | S3 access key |
| `S3_SECRET_KEY` | *(required)* | S3 secret key |
| `S3_PUBLIC_ENDPOINT` | *(unset)* | `host:port` devices download models from, if not `S3_ENDPOINT` (Android emulator: `10.0.2.2:19000`); device download URLs are signed for it |
| `S3_USE_SSL` | `false` | Use HTTPS to reach S3 |
| `NATS_URL` | `nats://localhost:14222` | NATS server for the telemetry pipeline |
| `REDIS_URL` | `redis://localhost:16379` | Reserved; not used yet |
| `GRPC_PORT` | `19090` | Device gRPC API port |
| `REST_PORT` | `18080` | REST API port (dashboard, CLI) |
| `JWT_SECRET` | *(required)* | HMAC secret for device tokens |
| `DEVICE_TOKEN_TTL` | `720h` | Device token lifetime (minimum `24h`); tokens can be refreshed in their last 7 days |
| `FLEETML_ADMIN_KEY` | *(unset)* | Stored as the `bootstrap` admin API key on first start if no key exists (min. 32 chars). If unset, the server generates one and logs it once |
| `LOG_LEVEL` | `info` | `debug`, `info`, `warn`, `error` |
| `LOG_FORMAT` | `json` | `json` or `text` |
| `CORS_ORIGINS` | `http://localhost:5173,http://localhost:18080` | Comma-separated browser origins allowed to call the REST API. `*` allows any origin without credentials |
| `ALERT_WEBHOOK_URL` | *(unset)* | Receives rollback and drift alerts as JSON POSTs (see [docs/ROLLOUTS.md](docs/ROLLOUTS.md#alerts)) |
| `ALERT_WEBHOOK_SECRET` | *(unset)* | Signs webhook bodies: `X-FleetML-Signature: sha256=<HMAC-SHA256>` |
| `DEPLOY_TICK_INTERVAL` | `30s` | How often rollouts are evaluated and advanced |
| `EXPERIMENT_TICK_INTERVAL` | `60s` | How often running experiments are evaluated |
| `DRIFT_TICK_INTERVAL` | `5m` | How often ACTIVE model versions are checked for drift |
| `MIGRATE_ON_START` | `false` | Apply the embedded DB migrations before serving (`true` in `deploy/docker-compose.prod.yml`) |

Docker Compose host ports (dev stack): `FLEETML_PG_PORT=15432`, `FLEETML_S3_PORT=19000`, `FLEETML_NATS_PORT=14222`, `FLEETML_NATS_MONITOR_PORT=18222`, `FLEETML_REDIS_PORT=16379`.

## Architecture

```
Android    : gRPC --> Go Server ──NATS--> TimescaleDB
Dashboard  : REST --> (REST+gRPC)        S3 (SeaweedFS)
CLI        : REST -->
```

## Security

Please report vulnerabilities privately; see [SECURITY.md](SECURITY.md). FleetML does not terminate TLS itself: put the REST, gRPC and storage ports behind a TLS proxy before exposing them.

## License

Apache 2.0
