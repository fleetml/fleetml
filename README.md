# FleetML

**Fleet management for on-device ML.**

FleetML is an open-source, runtime-agnostic, self-hosted-first platform for managing the complete lifecycle of ML models on edge devices, from model registry and versioning through OTA deployment, A/B testing, observability, and drift detection.

## Features

- **Model Registry**: Version, store, and manage model variants (FP32, FP16, INT8, INT4) across frameworks (TFLite, ONNX, CoreML, GGUF)
- **Intelligent Variant Selection**: Automatically selects the optimal model variant based on device hardware capabilities
- **Device Fleet Management**: Register, classify, and monitor edge devices with capability scoring (0-100)
- **Deployment Strategies**: ALL_AT_ONCE and CANARY (staged rollout by consistent hashing) with label targeting. Auto-rollback on error rate happens only when the deployment is created with a `health_gate`; there is no default gate yet. ROLLING is accepted by the API but currently delivers to all targeted devices at once (batching is not implemented yet)
- **A/B Experiments**: Deterministic, sticky cohort assignment and per-arm results with Welch's t-test and Cohen's d. Not yet end-to-end: devices do not yet receive a download URL for the arm's model
- **Telemetry**: Device events over gRPC, through NATS into a TimescaleDB hypertable, with summary and time-bucketed series endpoints
- **Drift Detection**: PSI on latency and confidence distributions per model version (results are stored and logged; no alert delivery yet)
- **Dashboard**: React UI for monitoring devices, deployments, experiments, telemetry, drift
- **CLI**: `fleetml push`, `fleetml deploy`, `fleetml experiment`, `fleetml drift`
- **Android SDK**: Kotlin library: fingerprint, register, heartbeat, download, cache, telemetry

## Quick Start

**Prerequisites**

- [Docker](https://docs.docker.com/get-docker/) with Compose v2.20+ (running)
- [Go 1.24+](https://go.dev/dl/)
- [golang-migrate](https://github.com/golang-migrate/migrate) CLI, for manual `make migrate-up` / `make migrate-down` (the quickstart runs migrations in Compose)
- [grpcurl](https://github.com/fullstorydev/grpcurl), to talk to the device gRPC API from a terminal
- [Node.js 20+](https://nodejs.org/), only for the dashboard dev server

**Run**

```bash
git clone https://github.com/fleetml/fleetml.git && cd fleetml
make dev    # creates .env from .env.example if missing, starts Postgres/TimescaleDB, S3 storage (SeaweedFS), NATS, Redis,
            # applies DB migrations, then runs the server on REST :18080 and gRPC :19090
```

In a second terminal:

```bash
curl localhost:18080/healthz                 # {"db":"connected","nats":"connected","s3":"connected","status":"ok"}
grpcurl -plaintext localhost:19090 list      # fleetml.device.v1.DeviceService, fleetml.telemetry.v1.TelemetryService
make dashboard-dev                           # dashboard on http://localhost:5173 (proxies the API on :18080)
```

To run everything in containers instead (server + dashboard): `docker compose -f deploy/docker-compose.prod.yml up -d --build`, then the API is on `:18080` and the dashboard on `:80`.

## Configuration

The server reads its configuration from environment variables (`.env.example` has a ready-to-use dev set). Default ports: REST **18080**, gRPC **19090**, infrastructure on **15xxx/14xxx/16xxx/18xxx/19xxx** host ports so nothing collides with common 8080/5432 users.

| Variable | Default | Description |
|---|---|---|
| `DATABASE_URL` | *(required)* | Postgres/TimescaleDB DSN, e.g. `postgres://fleetml:fleetml_dev@localhost:15432/fleetml?sslmode=disable` |
| `S3_ENDPOINT` | `localhost:19000` | S3-compatible endpoint as `host:port` (no scheme); the dev stack runs SeaweedFS |
| `S3_BUCKET` | `fleetml-models` | Bucket for model blobs (created on server start if missing) |
| `S3_ACCESS_KEY` | *(required)* | S3 access key |
| `S3_SECRET_KEY` | *(required)* | S3 secret key |
| `S3_USE_SSL` | `false` | Use HTTPS to reach S3 |
| `NATS_URL` | `nats://localhost:14222` | NATS server for the telemetry pipeline |
| `REDIS_URL` | `redis://localhost:16379` | Reserved; not used yet |
| `GRPC_PORT` | `19090` | Device gRPC API port |
| `REST_PORT` | `18080` | REST API port (dashboard, CLI) |
| `JWT_SECRET` | *(required)* | HMAC secret for device tokens |
| `LOG_LEVEL` | `info` | `debug`, `info`, `warn`, `error` |
| `LOG_FORMAT` | `json` | `json` or `text` |
| `CORS_ORIGINS` | `*` | Comma-separated allowed origins for the REST API |
| `MIGRATE_ON_START` | `false` | Apply the embedded DB migrations before serving (`true` in `deploy/docker-compose.prod.yml`) |

Docker Compose host ports (dev stack): `FLEETML_PG_PORT=15432`, `FLEETML_S3_PORT=19000`, `FLEETML_NATS_PORT=14222`, `FLEETML_NATS_MONITOR_PORT=18222`, `FLEETML_REDIS_PORT=16379`.

## Architecture

```
Android    : gRPC --> Go Server ──NATS--> TimescaleDB
Dashboard  : REST --> (REST+gRPC)        S3 (SeaweedFS)
CLI        : REST -->
```

## License

Apache 2.0
