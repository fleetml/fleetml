# Changelog

All notable changes to FleetML are recorded here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and versions follow
[Semantic Versioning](https://semver.org/) (before 1.0, minor versions may
change APIs).

## [0.4.0] - Unreleased

## [0.3.0] - 2026-10-08

### Added

- Artifact signing: every project gets an ed25519 key (private keys
  encrypted with the new required `SIGNING_MASTER_KEY`), each variant is signed
  when it is finalized, and Register and heartbeat responses carry the keys
  and each assignment its signature. The Linux agent and the Android and iOS
  SDKs verify it before downloading; a failure is reported as the new
  `SIGNATURE_INVALID` event, which counts against the health gate.
  `POST /projects/{id}/signing-key` and `.../rotate` (with a grace period).
  `scripts/signing-e2e.sh` checks a client against the dev stack.
- Feature-level drift: `PUT /models/{id}/features` sets each feature's bin
  edges, devices get them in `DeviceConfig.feature_bins`, and the SDKs
  (`reportInference(..., features)`) and the agent's local endpoint
  (`"features"`) send per-feature histograms. The drift check computes PSI per
  feature (metric `feature:<name>`) with the usual deduplication and alerts.
  Every check's PSI is kept (`GET /drift/{id}/points`) and the Drift page draws
  it over time.
- Helm chart (`deploy/helm/fleetml`): the server, the dashboard and optional
  bundled Postgres (TimescaleDB), NATS (JetStream) and SeaweedFS, secrets from
  values or an existing Secret, and a `helm test` hook. CI lints and renders
  it; `scripts/helm-e2e.sh` installs it into kind and runs the end-to-end suite
  against it (weekly in the E2E workflow).
- Users and roles: every API key belongs to a user; users are global admins
  or have a viewer, operator or admin role per project
  (`POST /users`, `POST /users/{id}/api-keys`, `PUT /projects/{id}/members`).
  A permission matrix covers every route and refuses unlisted ones; lists
  across projects show only the caller's projects. Existing API keys belong to
  the new `bootstrap` user, a global admin, so nothing changes for them.
- Audit log of every change and every refused request (user, key, route,
  project, status, client address): `GET /audit` with filters,
  `GET /audit/export` as CSV, and an Audit page in the dashboard.
- Delta updates: when a variant becomes READY the server builds a binary delta
  (bsdiff with raw DEFLATE sections, `internal/delta`) from the previous READY
  variant of the same model, runtime and quantization; assignments carry
  `delta_url`, `delta_size_bytes`, `delta_sha256` and `delta_base_sha256`, and
  the Linux agent and the Android and iOS SDKs download and apply the delta when
  they have the base version, falling back to the full model.
  `DELTA_MAX_SIZE_MB` (128) bounds the models that get one.
- At-least-once telemetry: accepted batches go into a NATS JetStream stream
  (`FLEETML_TELEMETRY`, file storage) before the device is answered, and a
  durable consumer acknowledges them only after the database commit. Events
  carry an `event_id` (set by the SDKs and the agent) and are stored once per
  ID. New metrics `fleetml_telemetry_consumer_pending`,
  `fleetml_telemetry_redelivered_total` and `fleetml_telemetry_duplicates_total`;
  a Grafana dashboard and Prometheus in the compose profile `observability`.

### Changed

- NATS must run with JetStream; both compose files enable it, pin
  `nats:2.12.1` and keep its data in a volume. `scripts/e2e.sh` and the demos
  start their own NATS, so they no longer share the dev stack's.
- The telemetry consumer drops only events the database can never accept
  (invalid values, unknown model versions); other database errors are retried.
- The Linux agent applies a telemetry flush interval from the server at once
  instead of after its first 30-second wait.
- The CLI, the Linux agent, the dashboard and the Android and iOS SDKs are
  0.3.0.
- `SIGNING_MASTER_KEY` is required; `make env-file` adds one to an existing
  `.env`.
- The quickstart's dashboard hint mentions the login (it still pointed to
  Settings).
- The dashboard's login asks for "your API key": any user's key logs in, not
  only an admin's.
- The 20 MB delta timing test no longer runs under the race detector, where
  it took minutes.

### Fixed

- `DELETE /devices/{id}` failed with a 500 for any device that had reported
  telemetry. Deleting a device now keeps its telemetry, with its model
  version and experiment arm, and drops only the link to the device (migration
  000018); experiment results, drift and telemetry summaries do not change.
- Android and iOS SDKs: the start-up registration and a `syncNow()` right
  after `init()` could both register; each Register issues a token that
  supersedes the previous one, so the device could keep the superseded token
  and be refused on its next start. Registrations now run one at a time, and
  a caller that waited does not register again.
- iOS SDK: `flushNow()` called while another flush was running (for example
  the one a reconnect starts) returned at once without sending; it now waits
  for that flush and sends what is left, as on Android.
- The Android live tests failed when run again against the same server:
  `LiveServerTest` cleared the stored token after each run while the
  emulator's fingerprint stayed registered in that run's project, so the next
  run (a new project) was refused; and it took the first model reported,
  which could be one `SignatureLiveTest` had left in storage. Each run now
  registers with a fingerprint of its own (as the iOS live tests do) and
  waits for the model it deployed. `scripts/android-e2e.sh` runs only
  `LiveServerTest`; `SignatureLiveTest` is skipped unless
  `scripts/signing-e2e.sh` runs it.

## [0.2.0] - 2026-10-08

### Added

- Linux device agent (`cmd/fleetml-agent`, `make build-agent`): a static
  binary for linux/amd64 and linux/arm64 with a systemd unit and install
  script (`deploy/agent`). It registers with an enrollment key and a
  machine-ID-based fingerprint, downloads with Range resume and sha256
  checks, validates with a configurable command, switches a `current`
  symlink per model, keeps the last-known-good version, runs a
  post-activate hook, queues telemetry on disk, offers a local HTTP endpoint
  for inference events, and honours the download policy and urgent flag.
  `docs/LINUX_AGENT.md`.
- `scripts/demo-agent.sh`: four Linux agents take a canary of a model that
  fails at inference; the health gate rolls it back. Its recording, with two
  dashboard screenshots, is the README's GIF (`docs/screenshots/demo.gif`),
  regenerated by `docs/screenshots/demo/record.sh` (vhs, Playwright, ffmpeg).
- Documentation site: `make docs-site` builds the guides, README and
  changelog with MkDocs Material (pinned, in Docker, strict) into `site/` and
  checks every internal link and anchor offline; CI builds it on every pull
  request.
- Alert sinks per project (webhook with HMAC signing, Slack Block Kit,
  SMTP email), managed over REST (`/projects/{id}/alert-sinks`,
  `/alert-sinks/{id}`) and on the dashboard's Alerts page, with a test-send
  button and a delivery log of the last 50 attempts. Alerts now carry
  `project_id` and the model name and version. New settings:
  `DASHBOARD_URL`, `SMTP_HOST`, `SMTP_PORT`, `SMTP_USERNAME`,
  `SMTP_PASSWORD`, `SMTP_FROM`.
- Dashboard login: `POST /api/v1/auth/login` exchanges an admin API key for
  an `httpOnly`, `SameSite=Strict` session cookie backed by a server-side
  session (`SESSION_TTL`, 12 hours by default). Writes with a session need its
  CSRF token (`X-CSRF-Token`). `GET /auth/session`, `POST /auth/logout`,
  `GET /auth/sessions` and `POST /auth/sessions/{id}/revoke`; the dashboard
  has a login page, redirects to it when the session ends, and lists and
  revokes sessions in Settings. The REST API still accepts
  `Authorization: Bearer <key>`.

### Changed

- Linux devices accept `CUSTOM` model variants, and a `linux/arm64` chipset is
  profiled as arm64.
- The dashboard no longer stores the API key in the browser's local storage,
  and no longer supports `VITE_API_URL`: it calls the API on its own origin
  (`FLEETML_API` sets the dev and preview proxy target).
- The revert deployment of an unnamed deployment is named `revert-<id
  prefix>` instead of `revert-`, and health gate reasons say "1 cohort device".
- Alert deliveries make up to five attempts (after 1s, 2s, 4s and 8s) instead
  of four; the last failure is recorded as a dead letter.

## [0.1.0] - 2026-10-08

The first release.

### Server

- Model registry: projects, models, versions and variants (runtime and
  quantization). Files are uploaded straight to an S3-compatible store with
  presigned URLs; finalizing streams the object and checks its sha256 and size,
  and only READY variants are ever assigned to devices.
- Variant selection per device by OS runtime, RAM and capability score, with
  per-version requirement overrides (`fleetml_requirements` metadata).
- Deployments: all at once, canary stages and rolling batches on a
  consistent-hash ring, targeted by device labels; one version per model per
  device (the newest deployment that reaches it).
- Health gate on by default for canary and rolling rollouts: the exposed
  cohort's error rate and, optionally, its p95 latency against the previous
  version; a failing gate rolls back or pauses.
- Rollback (also after completion) to the previous version with a reason and
  an alert, cancel, pause and resume, with a validated state machine.
- Urgent deployments: their assignments download on any network, ignoring the
  device download policy; rollbacks are always urgent.
- Device settings from the server (`DEVICE_*` variables), sent at
  registration and with every heartbeat: foreground and background heartbeat
  intervals, telemetry batch size and flush interval, and the download policy.
- A/B experiments: sticky cohorts with stable arm IDs, both arms' models
  delivered and kept on the device, telemetry attributed per arm, a
  fixed-horizon Welch t-test, maximum duration and conclusion with a winner.
- Telemetry over gRPC through NATS into a TimescaleDB hypertable (30-day
  retention, hourly continuous aggregate), with summaries and time series.
- Drift detection: PSI on inference latency and confidence, last 24 hours
  against the 7 days before, with history; a change into CRITICAL raises an
  alert.
- Alerts for rollbacks and drift on NATS (`fleetml.alerts.*`) and to an
  HMAC-SHA256-signed webhook with retries.
- Security: admin API keys on the REST API, per-project enrollment keys,
  revocable and refreshable device tokens, devices confined to their project's
  deployments, experiments and telemetry (a device moves to another project
  only with its own token and that project's enrollment key), locked-down
  CORS.
- Operations: `/livez`, `/readyz`, Prometheus `/metrics`, `grpc.health.v1`,
  gRPC reflection behind `GRPC_REFLECTION`, start-up that waits for its
  dependencies, and configuration checked at start-up with every invalid
  variable named.
- Device protocol v1 (`proto/fleetml/v1`): Register, Heartbeat, RefreshToken
  and ReportBatch.

### SDKs and samples

- Android SDK (Kotlin, minSdk 26): enrollment, token refresh, resumable
  sha256-verified downloads, validation with TensorFlow Lite or ONNX Runtime
  before activation, last-known-good, offline start, a persistent telemetry
  queue, and server-set intervals. A sample app runs the Iris TensorFlow Lite
  model.
- iOS SDK (Swift package, iOS 15): the same lifecycle with Core ML validation
  and Keychain credentials, and a SwiftUI sample app running the Iris Core ML
  model.
- Battery and data policy in both SDKs: heartbeats and telemetry flushes on a
  timer only in the foreground; in the background one check (WorkManager on
  Android, BGAppRefreshTask on iOS) at the server's interval, hourly by
  default; telemetry always batched; model downloads on an unmetered network
  or while charging unless the deployment is urgent. `syncNow()` runs a check
  on demand.
- Sample Iris classifiers in TensorFlow Lite and Core ML (`models/iris`).

### Tools

- Dashboard: models with versions and variants, deployment and experiment
  creation and control (rollback and cancel with confirmation), drift history,
  telemetry charts; works on phones.
- CLI: `fleetml push`, `deploy`, `experiment`, `drift`, `device`, `telemetry`,
  and `simulate` for fleets of simulated devices.
- `scripts/demo.sh`, the 60-second story against a simulated fleet.
- Docker Compose files for development and for running the server and
  dashboard in containers.
- CI for the server, dashboard and both SDKs, and a weekly end-to-end run.

### Known limitations

- No TLS termination: put the REST, gRPC and object store ports behind a TLS
  proxy before exposing them.
- The dashboard keeps the admin API key in the browser's local storage.
- One admin role; no organisations or per-project permissions yet.
- The SDKs are used from source; they are not yet published to Maven Central
  or a Swift package registry.
- The iOS SDK targets iOS 15 and was tested on the iOS 26.5 simulator. The
  Simulator cannot run background tasks, so the iOS background refresh has
  been checked on the Simulator only up to scheduling.

[0.4.0]: https://github.com/fleetml/fleetml/compare/v0.3.0...v0.4.0
[0.3.0]: https://github.com/fleetml/fleetml/compare/v0.2.0...v0.3.0
[0.2.0]: https://github.com/fleetml/fleetml/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/fleetml/fleetml/releases/tag/v0.1.0
