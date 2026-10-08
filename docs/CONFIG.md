# Configuration

Everything FleetML reads from the environment, by component. The server checks
its configuration at start-up and exits with a message that names every
missing or invalid variable.

## Server

`.env.example` is a ready-to-use development set; `make dev` copies it to
`.env` and fills in a random `JWT_SECRET` and `FLEETML_ADMIN_KEY`.

### Required

| Variable | Description |
|---|---|
| `DATABASE_URL` | Postgres 16 + TimescaleDB connection string, e.g. `postgres://fleetml:fleetml_dev@localhost:15432/fleetml?sslmode=disable` |
| `S3_ACCESS_KEY`, `S3_SECRET_KEY` | Credentials for the S3-compatible object store that holds model files |
| `JWT_SECRET` | HMAC secret that signs device tokens. At least 32 characters (`openssl rand -base64 32`). Changing it invalidates every device token; devices then re-register with their enrollment key |

### Network and storage

| Variable | Default | Description |
|---|---|---|
| `REST_PORT` | `18080` | REST API, dashboard API, `/livez`, `/readyz`, `/metrics` |
| `GRPC_PORT` | `19090` | Device gRPC API and `grpc.health.v1`; must differ from `REST_PORT` |
| `S3_ENDPOINT` | `localhost:19000` | Object store as `host:port`, without a scheme (the dev stack runs SeaweedFS) |
| `S3_USE_SSL` | `false` | Reach the object store over HTTPS |
| `S3_BUCKET` | `fleetml-models` | Bucket for model files; created at start-up if missing |
| `S3_PUBLIC_ENDPOINT` | *(unset)* | `host:port` devices download models from, when it differs from `S3_ENDPOINT` (Android emulator: `10.0.2.2:19000`); device download URLs are signed for it |
| `NATS_URL` | `nats://localhost:14222` | NATS server for the telemetry pipeline and alerts |
| `MIGRATE_ON_START` | `false` | Apply the embedded database migrations before serving (`true` in `deploy/docker-compose.prod.yml`) |

At start-up the server waits up to 60 seconds for Postgres, the object store
and NATS, so it can start together with them.

### Access

| Variable | Default | Description |
|---|---|---|
| `FLEETML_ADMIN_KEY` | *(unset)* | Stored as the `bootstrap` admin API key on first start if no key exists yet (at least 32 characters). If unset, the server generates one and logs it once (`admin_api_key`) |
| `DEVICE_TOKEN_TTL` | `720h` | Device token lifetime, at least `24h`; devices refresh tokens in their last 7 days |
| `CORS_ORIGINS` | `http://localhost:5173,http://localhost:18080` | Comma-separated browser origins allowed to call the REST API. `*` allows any origin without credentials |
| `GRPC_REFLECTION` | `false` | Register gRPC server reflection, for tools like `grpcurl` without proto files |

### Background loops

| Variable | Default | Description |
|---|---|---|
| `DEPLOY_TICK_INTERVAL` | `30s` | How often rollouts are evaluated (health gate) and advanced |
| `EXPERIMENT_TICK_INTERVAL` | `60s` | How often running experiments are evaluated and expired ones concluded |
| `DRIFT_TICK_INTERVAL` | `5m` | How often active model versions are checked for drift |

Each must be at least `1s`; short values are for tests and demos.

### Alerts

| Variable | Default | Description |
|---|---|---|
| `ALERT_WEBHOOK_URL` | *(unset)* | `http(s)` URL that receives rollback and drift alerts as JSON POSTs ([details](ROLLOUTS.md#alerts)). Alerts always go to NATS (`fleetml.alerts.*`) |
| `ALERT_WEBHOOK_SECRET` | *(unset)* | Signs webhook bodies: `X-FleetML-Signature: sha256=<HMAC-SHA256>` |

### Logging

| Variable | Default | Description |
|---|---|---|
| `LOG_LEVEL` | `info` | `debug`, `info`, `warn` or `error` |
| `LOG_FORMAT` | `json` | `json` or `text` |

## Docker Compose

`deploy/docker-compose.yml` (the dev stack: Postgres/TimescaleDB, SeaweedFS,
NATS) publishes its services on these host ports:

| Variable | Default | Service |
|---|---|---|
| `FLEETML_PG_PORT` | `15432` | Postgres |
| `FLEETML_S3_PORT` | `19000` | SeaweedFS S3 API |
| `FLEETML_NATS_PORT` | `14222` | NATS |
| `FLEETML_NATS_MONITOR_PORT` | `18222` | NATS monitoring |

`deploy/docker-compose.prod.yml` (server, dashboard and their dependencies)
reads `JWT_SECRET` (required) and `FLEETML_ADMIN_KEY` (optional) from the
shell.

## CLI

`fleetml` reads `~/.fleetml/config.yaml` (written by `fleetml config set-key`
and friends); these override it:

| Variable | Default | Description |
|---|---|---|
| `FLEETML_SERVER` | `http://localhost:18080` | REST API base URL |
| `FLEETML_API_KEY` | *(unset)* | Admin API key, sent as `Authorization: Bearer` |
| `FLEETML_OUTPUT` | `table` | `table` or `json` |

## Dashboard

| Variable | Default | Description |
|---|---|---|
| `VITE_API_URL` | *(empty: same origin)* | REST API base URL, fixed at build time (`npm run build`). The dev server proxies `/api` to `localhost:18080` |

## Tests and scripts

| Variable | Used by | Description |
|---|---|---|
| `FLEETML_TEST_DATABASE_URL` | integration tests (`make test-integration`, `make cover`) | Postgres URL where tests may create and drop databases; tests are skipped if unset |
| `FLEETML_TEST_NATS_URL` | integration tests | NATS for the telemetry pipeline test |
| `FLEETML_E2E_PG` | `scripts/e2e.sh` | Postgres base URL for the throwaway database (default: the dev stack) |
| `FLEETML_E2E_REST_PORT`, `FLEETML_E2E_GRPC_PORT` | `scripts/e2e.sh` | Ports of the server the suite starts (`18180`, `19190`) |
| `FLEETML_E2E_API`, `FLEETML_E2E_GRPC`, `FLEETML_E2E_ADMIN_KEY` | `test/e2e` | Set by `scripts/e2e.sh`; the suite is skipped without them |
| `FLEETML_API_KEY`, `FLEETML_API` | `scripts/android-e2e.sh`, `scripts/demo.sh` | Admin key and REST base URL (`http://localhost:18080`) |
| `JAVA_HOME_17` | `scripts/android-e2e.sh` | JDK 17 home (default `/opt/homebrew/opt/openjdk@17`) |
