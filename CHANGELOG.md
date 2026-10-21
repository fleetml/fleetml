# Changelog

All notable changes to FleetML are recorded here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and versions follow
[Semantic Versioning](https://semver.org/) (before 1.0, minor versions may
change APIs).

## [0.6.0] - Unreleased

### Added

- `GET /api/v1/deployments/{id}` (and `fleetml deploy get`) has
  `devices: {targeted, loaded}`: the project's devices the deployment targets
  and how many report its version loaded, since `COMPLETE` only means
  assigned.
- `GET /api/v1/devices/{id}` (and `fleetml device get`) has `loaded_models`,
  what the device's last heartbeat reported, and `last_model_event`, its
  newest model load, load failure or signature failure with the error.
- The Linux agent reports a failed model download as `MODEL_LOAD_FAILED` with
  the error, once per version, so the server shows it.
- `fleetml deploy list --project`, and `GET /api/v1/deployments` takes
  `project_id` (400 for a value that is not a UUID).
- `scripts/agent-install-test.sh` runs the agent's `install.sh` in Ubuntu
  containers with and without systemd.
- Linux agent: `download_host_override` (`host:port`) sends every model
  download, full or delta, to another address than the server's
  `S3_PUBLIC_ENDPOINT`, keeping the signed URL's path, query and `Host`
  header, for an edge team that cannot restart the server.
- Linux agent: `-retry-failed` retries the versions that failed validation
  at once instead of after the hour's wait, and the rejected file is kept in
  `<state_dir>/rejected/<model>/<version>-<variant>/model.bin` (the last one
  per model) for running the validator by hand.
- Linux agent: `POST /v1/events` answers with `rejected_reasons`, one per
  rejected event (`event 0: unknown model "foo" (active models: iris)`), and
  logs the first one.
- Android SDK: logs `Model <name> <version> passed validation`, as the iOS
  SDK does.

### Changed

- The Linux agent's `GET /v1/models` answers with `model`, `version`,
  `version_id` and `path`, the same snake_case as `POST /v1/events`, instead
  of `Name`, `Version`, `VersionID` and `Path`, and with `[]` instead of
  `null` when no model is active.
- The iOS SDK logs each download (model, version, bytes) and validation
  result at info level and every heartbeat at debug level.

### Fixed

- Linux agent: a fully downloaded model was readable only by the
  `fleetml-agent` user (mode 0600) while a model built from a delta was 0644;
  every active model is now 0644 in 0755 folders whatever the umask, and
  models written by an earlier agent are fixed at startup.
- Linux agent: `install.sh` exited 127 at `systemctl` on a machine without
  systemd after installing everything, and nothing created
  `/var/lib/fleetml-agent` there; it now skips the unit, creates the folder for
  the `fleetml-agent` user and prints how to run the agent.
- A deployment's device lists (rolling batch sizes, `max_unavailable`, the
  health gate's cohort) counted devices of other projects with matching
  labels; they stay in the deployment's project now.
- iOS: every launch logged `Heartbeat failed ... GRPCStatus error 1` and
  reported `connectionChanged(false)` before `true`, because the heartbeat loop
  was started twice and the second start cancelled the first heartbeat; it
  now starts once, and a cancelled heartbeat is not a failure.
- iOS: when the device token cannot be stored in the Keychain (an app built
  without code signing, on the simulator), the SDK now logs a warning instead
  of silently registering again at every launch.
- `fleetml push` showed a model under a kilobyte as `0KB`; small sizes are now
  in bytes.
- Android SDK: a failed heartbeat, token refresh, registration or telemetry
  flush logged a gRPC stack trace of about 30 lines at warning level; it now
  logs one line with the status and cause, and the trace at debug level.
- iOS SDK: an app without the SDK's background task logged
  `Background refresh not scheduled ... BGTaskSchedulerErrorDomain error 3` as
  an error at every launch; it is now an info line that says how to enable
  the task, and only a permitted task that fails to schedule is an error.

### Docs

- Quickstart: save the three secrets right after starting the stack (the
  admin key is honoured on the first start only, and compose needs the two
  secrets for every command), the port-80 override, how the canary cohort is
  chosen, what `experiment conclude --winner` does to devices, what
  `COMPLETE` means, bounded waiting loops; On Windows: administrator
  PowerShell, restart and firmware virtualization for `wsl --install`, Docker
  Desktop running, the tour only in Ubuntu, the clone's models under
  `\\wsl.localhost`, the unsigned exe, the admin key in PowerShell.
- Android: the `com.fleetml.sdk` package and imports, `10.0.2.2` in the init
  snippet, an `ActiveModel` table, `syncNow()` as a suspend function, an own-app
  CLI block, `ANDROID_SERIAL` with a phone attached, and "From Windows".
- iOS: full delegate signatures, `import CoreML`, `log stream --level info`,
  and a "Your own app" checklist.
- Linux agent: `S3_PUBLIC_ENDPOINT` as a prerequisite before the connect
  steps, a self-contained publish and deploy block, "Without systemd", file
  modes, the delta and restart log lines, and the agent in WSL.
- The next heartbeat is 60 seconds with the quickstart's server; the SDKs' and
  agent's 30 s applies only until the server answers.
- `docs/ROLLOUTS.md` "Did the model arrive?" describes the new fields.
- Second developer review: Android's emulator port forward for model files
  next to the init snippet, the enrollment key from a Gradle property,
  the first-run logcat lines, retrying without waiting for a heartbeat, the
  permissions the SDK brings, airplane mode for an own app, PowerShell lines
  in "From Windows"; iOS's key from the `Info.plist`, an own-app runbook with
  an XcodeGen project, `connectionFailed` being registration-only; the Linux
  agent's version lookup by number, `sed` instead of `sudoedit`, the
  agent-side `download_host_override`, the rejected file and
  `-retry-failed`, the server-side telemetry check, containers' PID 1; the
  tour's variables saved to `~/.fleetml.env` with a resume from another
  shell, every printed line and the real waits; Windows: the shell on each
  block, the admin key from `~/.fleetml.env` in one line,
  `fleetml config set-key`, reserved port ranges, `docker-users`, and what
  the Windows run does and does not verify; `loaded` turns one heartbeat
  after the device has the model and is counted each time you ask;
  RELEASING: tarballs built by hand on a Mac.

## [0.5.0] - 2026-10-19

### Added

- A Python client, `pip install fleetml` (Python 3.9 or later, `httpx` only):
  projects, models, versions, `push`, enrollment keys, deployments (with
  `wait_for_deployment`), devices, telemetry, experiments and drift, with the
  same words as the CLI and a `FleetMLError` for every refused request; typed
  (`py.typed`). CI runs its lint, type and unit checks on Linux, macOS and
  Windows and a live test of the tour's sections 1 to 3; the hand-run Python
  publish workflow publishes it to PyPI with Trusted Publishing. See
  `docs/PYTHON_CLIENT.md`.
- Release binaries: every `vX.Y.Z` tag gets the CLI for Linux, macOS and
  Windows (amd64 and arm64; `.tar.gz`, and `.zip` with `fleetml.exe` for
  Windows) and the Linux agent with its installer, plus `SHA256SUMS`, attached
  to its GitHub release by the new Release workflow, which can also back-fill
  an existing tag. `make release-binaries VERSION=X.Y.Z` builds the same files
  into `dist/`.
- A "CLI on Windows" CI job builds `fleetml.exe`, runs it, and runs the CLI's
  unit tests and a smoke test of the quickstart's CLI calls (project create,
  push, deploy create and start, device list) against a fake of the REST API;
  `scripts/e2e.sh` runs the same smoke test against a real server.
- A hand-run "Windows run" workflow on GitHub's Windows runners
  (`windows-latest`, `windows-11-arm`): the "Get the CLI" PowerShell block
  with a release's assets in Windows PowerShell 5.1, the released
  `fleetml.exe` against the CLI's fake of the REST API (`clitest-server`),
  and the quickstart with the tour's sections 1 to 3 in WSL 2, reached from
  Windows at `localhost`.
- `fleetml enrollment-key create --project <id> [--name <name>]` prints a new
  enrollment key once (`-o json` gives it as `.key`), and
  `fleetml enrollment-key list --project <id>` lists a project's keys without
  them; every device path needs one, and it took a curl call before.
- CLI flags the tour needed: `deploy create --canary-stages` and
  `--rolling-config`, `experiment create --target` and `--min-sample-size`,
  `push --metadata` for a new version's metadata (such as the requirements
  override a Core ML FP32 model needs on an iPhone), and
  `device list --project`.
- Container images for the Helm chart: the Release workflow's new images job
  builds the server and dashboard for linux/amd64 and linux/arm64 and pushes
  `ghcr.io/fleetml/fleetml-server` and `ghcr.io/fleetml/fleetml-dashboard`
  tagged with the version and `latest`.
- iOS SDK: `FleetMLDelegate` gains `fleetML(connectionFailed:)` (with an empty
  default), called with the error on the first registration failure after
  start and whenever the error changes, and the SDK's log messages are no
  longer redacted as `<private>` in Console.app.

### Changed

- `fleetml version` and `fleetml-agent -version` report the version set at
  build time from the release tag (`-ldflags -X`), and `dev` for a build from
  source outside a clean checkout of a tag; the release commit no longer bumps
  them by hand.
- `fleetml push` draws its progress bar only when stderr is a terminal.

### Fixed

- The Postgres health check in both compose files and the Helm chart now
  connects over TCP (`pg_isready -h 127.0.0.1`). The socket check reported
  ready during the image's first-start initialisation, about a second before
  the real server listened, so `migrate` could start too early and exit with
  "connection refused", and `docker compose up` then failed with "service
  migrate didn't complete successfully". Seen on GitHub's Windows runner under
  WSL 2; possible on any first start.
- `scripts/demo.sh` prints the Docker Compose output when the dev stack fails
  to start, instead of exiting without a message; `scripts/android-e2e.sh`
  and `scripts/signing-e2e.sh` use `sha256sum` where `shasum` is missing.
- Shell scripts and the Makefile keep LF line endings in Windows checkouts
  (`.gitattributes`).
- `deploy/agent/config.example.yaml` set `tls: true`, so an agent configured
  as the Linux agent guide said failed against the quickstart's plain-gRPC
  server with "transport: authentication handshake failed"; it is now
  `false`, the documented default, with a comment on when to set it, and
  `install.sh` names `tls` among the settings to edit.
- A push whose upload failed (an unreachable `S3_PUBLIC_ENDPOINT`, say) can
  be run again: the server replaces a variant that was never verified instead
  of refusing the retry with "variant.Create: already exists"; a READY
  variant is still refused.
- `GET /api/v1/devices` honours `project_id` (it returned every device, even
  for a bogus project), refuses one that is not a UUID, and keeps its label
  filter within the caller's projects.
- `fleetml experiment conclude` no longer panics on a winner ID shorter than
  12 characters.

### Docs

- Windows: the README's requirements and the quickstart's new "On Windows"
  section run FleetML in a WSL 2 shell with Docker Desktop's WSL 2 backend, and
  reach the dashboard and the API from Windows at `localhost`.
- "Get the CLI" in the README and the quickstart: the release assets, the
  checksum check and the commands for macOS and Linux (curl) and Windows
  (PowerShell); the Linux agent installs from its release archive.
- Windows: Docker Desktop's requirements and its early-access status on Arm,
  a TLS 1.2 note for the PowerShell download, and what the Windows run
  workflow exercises (the amd64 binary; arm64 not yet run on Windows on Arm).
- First-run guides rewritten against the production compose stack from the
  quickstart, with the CLI instead of curl, for Android, iOS, the Linux agent
  and Windows.

## [0.4.0] - 2026-10-15

### Added

- The Android SDK is published to Maven Central as `dev.fleetml:fleetml-sdk`
  (with sources and javadoc jars, every file signed) by the hand-run Android
  publish workflow; the version comes from `sdkVersion` in
  `android/gradle.properties`, and protobuf-lite is now an `api` dependency
  because its types appear in the SDK's public API.
- The iOS SDK installs by the repository URL: `Package.swift` is now at the
  repository root (the code stays in `ios/FleetMLSDK`), so Xcode's Add Package
  Dependencies with `https://github.com/fleetml/fleetml` and
  `.package(url: "https://github.com/fleetml/fleetml", from: "0.4.0")` resolve
  it. Breaking for apps that added the package by the local path
  `ios/FleetMLSDK`: the local path is now the repository root.

### Changed

- The README opens with what FleetML does and why it exists, describes each
  feature in one sentence, lists the quickstart's requirements, shows how to
  move the dashboard off port 80 with a compose override file, and has a CI
  badge.
- `SECURITY.md` adds GitHub's private vulnerability reporting as a second way
  to report a vulnerability.
- Docs: first-run fixes from a stranger test on macOS and a physical Android
  phone (zsh comments, port override, ANDROID_HOME and JDK 17, physical device
  runbook, offline and metered testing, p-value note).
- Docs: a physical device runbook for the iOS SDK (Developer Mode, the Mac's
  LAN address and `S3_PUBLIC_ENDPOINT`, personal team signing,
  `ENABLE_PREVIEWS=NO` for command-line builds), and both SDKs are stated as
  verified on a physical phone (an iPhone on iOS 26.6.1, an Android 15 phone).
- Docs: the Android install section leads with the Maven Central dependency
  and keeps including the module from a clone for unreleased changes; the iOS
  install section leads with the repository URL and keeps the local clone as a
  second option; the README shows both, and `RELEASING.md` has a release
  checklist and a step that publishes the Android SDK.

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
- Both compose files and CI pin `timescale/timescaledb:2.30.2-pg16`, the
  tag the Helm chart uses, instead of `latest-pg16`.
- The iOS SDK's tests, live test and signing check also pass on the iOS 16.4
  simulator (before, only iOS 26.5 had been tested); the deployment target
  stays iOS 15.

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
- `make dev`, `make test-integration`, `make e2e`, `make cover`,
  `scripts/demo.sh`, `scripts/demo-agent.sh` and the E2E workflow waited for
  migrations with `docker compose wait migrate`, which fails with "no
  containers" when the migrations have already finished. They now use
  `scripts/wait-migrate.sh`, which waits for the container and returns its
  exit code.
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

[0.6.0]: https://github.com/fleetml/fleetml/compare/v0.5.0...HEAD
[0.5.0]: https://github.com/fleetml/fleetml/compare/v0.4.0...v0.5.0
[0.4.0]: https://github.com/fleetml/fleetml/compare/v0.3.0...v0.4.0
[0.3.0]: https://github.com/fleetml/fleetml/compare/v0.2.0...v0.3.0
[0.2.0]: https://github.com/fleetml/fleetml/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/fleetml/fleetml/releases/tag/v0.1.0
