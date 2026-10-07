# FleetML

**Fleet management for on-device ML.**

FleetML is an open-source, runtime-agnostic, self-hosted-first platform for managing the complete lifecycle of ML models on edge devices, from model registry and versioning through OTA deployment, A/B testing, observability, and drift detection.

## Features

- **Model Registry**: Version, store, and manage model variants (FP32, FP16, INT8, INT4) across frameworks (TFLite, ONNX, CoreML, GGUF)
- **Intelligent Variant Selection**: Automatically selects the optimal model variant based on device hardware capabilities
- **Device Fleet Management**: Register, classify, and monitor edge devices with capability scoring (0-100)
- **Deployment Strategies**: ALL_AT_ONCE, CANARY (staged with health gates + auto-rollback), ROLLING (batched)
- **A/B Experiments**: Controlled experiments with deterministic cohort assignment, Welch's t-test, Cohen's d
- **Telemetry**: Async NATS pipeline into TimescaleDB with time-series aggregation
- **Drift Detection**: PSI-based distribution shift monitoring
- **Dashboard**: React UI for monitoring devices, deployments, experiments, telemetry, drift
- **CLI**: `fleetml push`, `fleetml deploy`, `fleetml experiment`, `fleetml drift`
- **Android SDK**: Kotlin library: fingerprint, register, heartbeat, download, cache, telemetry

## Quick Start

```bash
git clone https://github.com/fleetml/fleetml.git && cd fleetml
docker compose -f deploy/docker-compose.yml up -d
make migrate-up
make dev
```

## Configuration

The server reads its configuration from environment variables (`.env.example` has a ready-to-use dev set). Default ports: REST **18080**, gRPC **19090**, infrastructure on **15xxx/14xxx/16xxx/18xxx/19xxx** host ports so nothing collides with common 8080/5432 users.

| Variable | Default | Description |
|---|---|---|
| `DATABASE_URL` | *(required)* | Postgres/TimescaleDB DSN, e.g. `postgres://fleetml:fleetml_dev@localhost:15432/fleetml?sslmode=disable` |
| `S3_ENDPOINT` | `localhost:19000` | MinIO/S3 endpoint as `host:port` (no scheme) |
| `S3_BUCKET` | `fleetml-models` | Bucket for model blobs |
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

Docker Compose host ports (dev stack): `FLEETML_PG_PORT=15432`, `FLEETML_MINIO_PORT=19000`, `FLEETML_MINIO_CONSOLE_PORT=19001`, `FLEETML_NATS_PORT=14222`, `FLEETML_NATS_MONITOR_PORT=18222`, `FLEETML_REDIS_PORT=16379`.

## Architecture

```
Android/iOS: gRPC --> Go Server ──NATS--> TimescaleDB
Dashboard  : REST --> (REST+gRPC)        MinIO (S3)
CLI        : REST -->
```

## License

Apache 2.0
