# Changelog

All notable changes to FleetML are recorded here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and versions follow
[Semantic Versioning](https://semver.org/) (before 1.0, minor versions may
change APIs).

## [0.2.0] - Unreleased

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

[0.2.0]: https://github.com/fleetml/fleetml/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/fleetml/fleetml/releases/tag/v0.1.0
