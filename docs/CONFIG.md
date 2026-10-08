# Configuration

Everything FleetML reads from the environment, by component. The server checks
its configuration at start-up and exits with a message that names every
missing or invalid variable.

## Server

`.env.example` is a ready-to-use development set; `make dev` copies it to
`.env` and fills in a random `JWT_SECRET`, `SIGNING_MASTER_KEY` and
`FLEETML_ADMIN_KEY` (and adds a `SIGNING_MASTER_KEY` to an older `.env`).

### Required

| Variable | Description |
|---|---|
| `DATABASE_URL` | Postgres 16 + TimescaleDB connection string, e.g. `postgres://fleetml:fleetml_dev@localhost:15432/fleetml?sslmode=disable` |
| `S3_ACCESS_KEY`, `S3_SECRET_KEY` | Credentials for the S3-compatible object store that holds model files |
| `JWT_SECRET` | HMAC secret that signs device tokens and derives the dashboard's CSRF tokens. At least 32 characters (`openssl rand -base64 32`). Changing it invalidates every device token; devices then re-register with their enrollment key |
| `SIGNING_MASTER_KEY` | Encrypts each project's ed25519 artifact signing key (AES-256-GCM with a key derived by HKDF-SHA256). At least 32 characters and different from `JWT_SECRET`. Back it up with the database: if it changes or is lost, the stored signing keys cannot be decrypted and publishing fails; [rotate](ROLLOUTS.md#artifact-signing) after restoring it |

### Network and storage

| Variable | Default | Description |
|---|---|---|
| `REST_PORT` | `18080` | REST API, dashboard API, `/livez`, `/readyz`, `/metrics` |
| `GRPC_PORT` | `19090` | Device gRPC API and `grpc.health.v1`; must differ from `REST_PORT` |
| `S3_ENDPOINT` | `localhost:19000` | Object store as `host:port`, without a scheme (the dev stack runs SeaweedFS) |
| `S3_USE_SSL` | `false` | Reach the object store over HTTPS |
| `S3_BUCKET` | `fleetml-models` | Bucket for model files; created at start-up if missing |
| `S3_PUBLIC_ENDPOINT` | *(unset)* | `host:port` clients (devices, the CLI, scripts) reach the object store at, when it differs from `S3_ENDPOINT`; presigned upload and download URLs are signed for it. `deploy/docker-compose.prod.yml` sets `localhost:19100` |
| `NATS_URL` | `nats://localhost:14222` | NATS server for the telemetry pipeline and alerts. JetStream must be enabled (`nats-server --jetstream`, as in both compose files): the server creates the `FLEETML_TELEMETRY` stream at startup and refuses to start without it. Servers that share a NATS share the stream, so they must also share the database |
| `MIGRATE_ON_START` | `false` | Apply the embedded database migrations before serving (`true` in `deploy/docker-compose.prod.yml`) |

At start-up the server waits up to 60 seconds for Postgres, the object store
and NATS, so it can start together with them.

### Access

| Variable | Default | Description |
|---|---|---|
| `FLEETML_ADMIN_KEY` | *(unset)* | Stored as the `bootstrap` admin API key on first start if no key exists yet (at least 32 characters). If unset, the server generates one and logs it once (`admin_api_key`) |
| `SESSION_TTL` | `12h` | How long a dashboard login lasts (at least `5m`). Sessions end earlier when revoked or when their API key is revoked |
| `DEVICE_TOKEN_TTL` | `720h` | Device token lifetime, at least `24h`; devices refresh tokens in their last 7 days |
| `CORS_ORIGINS` | `http://localhost:5173,http://localhost:18080` | Comma-separated browser origins allowed to call the REST API. `*` allows any origin without credentials |
| `GRPC_REFLECTION` | `false` | Register gRPC server reflection, for tools like `grpcurl` without proto files |

### Devices

Sent to devices at registration and with every heartbeat; the SDKs and the
agent apply them. The background interval is a lower bound for the operating
system's scheduler, which never runs background work more often than every
15 minutes.

| Variable | Default | Description |
|---|---|---|
| `DEVICE_HEARTBEAT_INTERVAL` | `60s` | Heartbeat interval while the app is in the foreground (at least `5s`) |
| `DEVICE_BACKGROUND_HEARTBEAT_INTERVAL` | `1h` | Interval of the background check (one heartbeat and one telemetry flush) while the app is in the background (at least `15m`) |
| `DEVICE_TELEMETRY_BATCH_SIZE` | `100` | Events per telemetry batch (1 to 1000) |
| `DEVICE_TELEMETRY_FLUSH_INTERVAL` | `30s` | How often queued telemetry is sent while the app is in the foreground |
| `DEVICE_DOWNLOAD_POLICY` | `unmetered_or_charging` | Networks for non-urgent model downloads: `unmetered_or_charging`, `unmetered`, or `any`. Urgent deployments ignore it |

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
| `DASHBOARD_URL` | *(unset)* | `http(s)` URL of the dashboard; Slack and email alerts link to its pages |
| `SMTP_HOST`, `SMTP_PORT` | *(unset)*, `587` | Mail server for email sinks; STARTTLS is used when the server offers it |
| `SMTP_USERNAME`, `SMTP_PASSWORD` | *(unset)* | SMTP login (PLAIN), if the server needs one |
| `SMTP_FROM` | *(unset)* | Sender address; set together with `SMTP_HOST` |

Per-project sinks (webhook, Slack, email) are configured in the database
through the dashboard or REST, not in the environment; see
[ROLLOUTS.md](ROLLOUTS.md#alert-sinks).

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
| `FLEETML_PROMETHEUS_PORT` | `19091` | Prometheus (profile `observability`) |
| `FLEETML_GRAFANA_PORT` | `13000` | Grafana with the FleetML dashboard (profile `observability`) |

`deploy/docker-compose.prod.yml` (server, dashboard and their dependencies)
publishes the dashboard on `:80`, the REST and gRPC APIs on `:18080` and
`:19090`, and the object store on `FLEETML_S3_PUBLIC_PORT` (`19100`, so it can
run next to the dev stack). It reads `JWT_SECRET` and `SIGNING_MASTER_KEY`
(required),
`FLEETML_ADMIN_KEY` (optional) and `S3_PUBLIC_ENDPOINT` (default
`localhost:19100`; set it to the object store's public `host:port` when
devices are not on this machine) from the shell.

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
| `FLEETML_API` | `http://localhost:18080` | Where `npm run dev` and `npm run preview` proxy `/api`. The dashboard always calls the API on its own origin, so its session cookie is first-party; in production nginx proxies `/api` to the server |

## Tests and scripts

| Variable | Used by | Description |
|---|---|---|
| `FLEETML_TEST_DATABASE_URL` | integration tests (`make test-integration`, `make cover`) | Postgres URL where tests may create and drop databases; tests are skipped if unset |
| `FLEETML_TEST_NATS_URL` | integration tests | NATS for the telemetry pipeline test |
| `FLEETML_E2E_PG` | `scripts/e2e.sh` | Postgres base URL for the throwaway database (default: the dev stack) |
| `FLEETML_E2E_REST_PORT`, `FLEETML_E2E_GRPC_PORT` | `scripts/e2e.sh` | Ports of the server the suite starts (`18180`, `19190`) |
| `FLEETML_E2E_API`, `FLEETML_E2E_GRPC`, `FLEETML_E2E_ADMIN_KEY` | `test/e2e` | Set by `scripts/e2e.sh`; the suite is skipped without them |
| `FLEETML_API_KEY`, `FLEETML_API` | `scripts/android-e2e.sh`, `scripts/ios-e2e.sh`, `scripts/demo.sh` | Admin key and REST base URL (`http://localhost:18080`) |
| `JAVA_HOME_17` | `scripts/android-e2e.sh` | JDK 17 home (default `/opt/homebrew/opt/openjdk@17`) |
