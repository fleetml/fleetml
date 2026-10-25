# Changelog

All notable changes to FleetML are recorded here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and versions follow
[Semantic Versioning](https://semver.org/) (before 1.0, minor versions may
change APIs).

## [Unreleased]

### Changed

- REST: an experiment or a model version asked for a status change its
  status does not allow answers `409 INVALID_TRANSITION`, as deployments
  always did, instead of `400 VALIDATION_ERROR` (the message text is the
  same); `POST /deployments/{id}/progress` on a deployment that is not
  `ROLLING_OUT` answers `409 INVALID_TRANSITION` instead of returning it
  unchanged; a `limit` or `offset` that is not an integer answers
  `400 VALIDATION_ERROR` naming the parameter instead of being ignored;
  `can_progress` in a deployment's health is true only when `progress`
  would move it now, and `reason` names the time a stage still holds
  (`min_duration`, `batch_interval`). A client that branched on
  `VALIDATION_ERROR` for those status changes checks `INVALID_TRANSITION`
  (review 5).
- Python client: `project`, `limit` and `offset` are keyword-only on the
  list methods (`deployments.list`, `devices.list` after `labels`,
  `experiments.list`, `projects.list`, `models.list` and `versions.list`
  after their parent, `telemetry.events`). A call that passed them by
  position, `fm.deployments.list(project_id)`, raised nothing and listed the
  whole server; write `fm.deployments.list(project=project_id)` (review 5).

### Added

- The server reports its version: `"version"` in `/readyz` and `/livez`,
  and in the start log line. A server built by `docker compose` in a clone
  takes it from the clone's tag; both images carry the
  `org.opencontainers.image.version` label (review 5).

- Android SDK: the background job's log line tells a refused request from an
  unreachable server: `Background sync: server reached, request refused
  (UNAUTHENTICATED: ...)` with the server's gRPC status and message, where
  it said `server unreachable` before (review 5).
- ANDROID_SDK.md: the command-line project has its manifest (AppCompat
  theme, launcher activity) and a `MainActivity`; built from the guide and
  run on an emulator (review 5).
- CI's "Android sample app on Windows" job checks the JDK is 17 and builds
  with a key and a dotted `-PfleetmlDebugHost`, quoted as the guide writes
  it, in PowerShell 7 and in Windows PowerShell 5.1 (review 5).

- `DEPENDENCY_TIMEOUT` (default `60s`): how long the server waits at
  start-up for each of Postgres, the object store and NATS; the Helm chart
  sets `180s` (`server.dependencyTimeout`), inside its 200 s startup probe
  (review 5).

### Fixed

- Helm: a cold install no longer restarts the server once while the
  Postgres image is still being pulled (the 60 s dependency wait ran out
  first); a `secrets.adminKey` shorter than 32 characters is refused at
  install time instead of crash-looping the server; the README's install
  block names its local ports in one line, used everywhere, and says what
  `existingSecret` does not roll and how to remove the namespace;
  `fleetml-secrets*.yaml` is git- and helm-ignored (review 5).
- ANDROID_SDK.md "From Windows" uses JDK 17 (Temurin, through winget):
  Android Studio 2026.1 bundles Java 25, on which Gradle 8.7 stops with
  `What went wrong: 25.0.2`; the `-P` arguments are quoted, since
  PowerShell cuts an unquoted `-PfleetmlDebugHost=<LAN IP>` at the first
  dot (review 5).
- README: `tensorflow-lite:2.17.0`, as the guide and the sample app.
- Compose: `FLEETML_S3_PUBLIC_PORT` also moves `S3_PUBLIC_ENDPOINT`'s
  default (`localhost:<port>`), and the Run block saves the port and the
  address together; before, a moved port left every upload and download URL
  on 19100 (review 5).
- QUICKSTART "Upgrading" keeps `S3_PUBLIC_ENDPOINT` for a `~/.fleetml.env`
  written by 0.7.0 or earlier (which set it on the command line): the block
  copies the running server's value into the file first, so phones and
  agents keep receiving models after the upgrade (review 5).

## [0.8.0] - 2026-10-24

The fixes from the fourth developer review (seven reviewers, a real Android
phone and iPhone among them), with its bug numbers, and dependency and
toolchain updates. No breaking API changes; see
[Upgrading](docs/QUICKSTART.md#upgrading).

### Added

- The Android SDK 0.8.0 is on Maven Central and the Python client 0.8.0 on
  PyPI (`pip install fleetml`).
- Python client: `project=` on `devices.list`, `deployments.list` and
  `experiments.list`; `projects.delete`, `models.get`, `versions.promote`
  (through STAGED from DRAFT), `deployments.pause`, `resume`, `cancel`,
  `progress` and `health` (`DeploymentHealth`), `devices.get`,
  `telemetry.events` (`TelemetryEvent`), `experiments.get`, `list`, `pause`,
  `resume`, `conclude` and `cancel`, and `drift.status`, each one REST call
  as the CLI makes it. The live test calls every one of them and deletes
  its projects (review 4, bug 8).
- CI builds the Android sample app with `gradlew.bat` in PowerShell on
  `windows-latest` (the "Android sample app on Windows" job).
- CI has a `Shell scripts` job: shellcheck on every shell script and
  actionlint on the workflows.
- iOS SDK: a unit test asserts `FleetML.sdkVersion`, as the Python client's
  test asserts its `__version__`.

### Changed

- Deleting a project also deletes its devices; before, a project without
  telemetry was deleted and its devices stayed, enrolled in no project
  (with telemetry, the delete failed, see Fixed).
- The server accepts gRPC keepalive pings every 20 seconds or more, also on a
  connection without a call in progress (gRPC's default answers a client
  that pings more often than every 5 minutes with `too_many_pings`).
- `scripts/e2e.sh` and `scripts/python-e2e.sh` name their compose project
  (`COMPOSE_PROJECT_NAME`, default `fleetml`) and stop before creating a
  database unless that project publishes Postgres on the port the server
  uses, so a run meant for a second stack cannot touch the dev stack.
  `scripts/wait-migrate.sh` names the project the same way.
- Building from source needs Go 1.26 or later (golang.org/x/net 0.60,
  nats.go 1.54 and minio-go 7.4 require it); the server image builds with
  Go 1.26.
- Server dependencies: gin 1.12, nats.go 1.54, minio-go 7.4, pgx 5.9,
  golang-migrate 4.20, the Prometheus client 1.25 and gRPC 1.84.
- CI and the release workflow build with Go 1.26.10 (`toolchain go1.26.10`
  in `go.mod`, which `actions/setup-go` reads); building from source still
  needs Go 1.26 or later.

### Fixed

- `fleetml project delete` (`DELETE /api/v1/projects/:id`) works for a
  project whose devices sent telemetry; it answered 500. The project goes
  with its models, versions, deployments, experiments, enrollment keys and
  devices, and its versions' telemetry, drift results and feature
  histograms, in one transaction. Deleting a version with telemetry deletes
  that telemetry (migration 000019). The CLI prints what it removed
  (fourth developer review, bug 1).
- `fleetml telemetry summary` (and the dashboard's telemetry chart) take
  latency and confidence over INFERENCE events only; a MODEL_LOAD or ERROR
  event no longer counts as a 0 ms inference, and an event without a
  confidence no longer counts as 0. The event counts are unchanged
  (review 4, bug 11).
- The telemetry and drift routes for one model version answer 404
  `NOT_FOUND` ("model version not found") for an unknown version instead of
  an empty result (review 4, bug 12).
- API error messages no longer carry internal function names:
  `project not found` instead of `project.GetByID: not found`,
  `project already exists: slug "tour"` instead of
  `project.Create: already exists: slug "tour"`. The error codes are
  unchanged (review 4, bug 12).
- Linux agent: a connection that dies without a reset (a Wi-Fi roam, a NAT
  or router restart) is found within 40 seconds (gRPC keepalive: a ping
  after 30 s idle, 10 s to answer), every call to the server has a 30 s
  deadline, and reconnect attempts back off to at most 15 s, so the first
  heartbeat after the server is back succeeds. Before, the agent hung
  without a log line until TCP gave up, about 16 minutes. The first failed
  heartbeat is logged at WARN once, the recovery at INFO
  (`server reachable again`). A model download that receives nothing for
  60 s is cut off and resumed on the next attempt (review 4, bug 3).
- Linux agent: SIGTERM stops it within 5 seconds while the server is
  unreachable; the last telemetry flush is bounded and the rest stays queued
  (review 4, bug 3).
- `install.sh` prints the supervisor command with both `exec`s
  (`exec su -s /bin/sh fleetml-agent -c 'exec /usr/local/bin/fleetml-agent ...'`):
  without the inner one a supervisor's SIGTERM stopped only `su`'s shell
  and left the agent running (review 4, bug 10). Run again on a box where
  the service runs, `install.sh` restarts it, so an upgrade takes effect
  (review 4, bug 12).
- Dashboard: the Overview tiles show the server's totals, and Devices,
  Deployments and Experiments page through the server (Previous and Next,
  `21–26 of 26`), where they stopped at the first 20 records. The project,
  model and version pickers (Models, Telemetry, Drift, Alerts, Audit, new
  deployments and experiments) load every record. Deployments has a Project
  column (review 4, bug 4).
- Android sample app: the status reads `online, assigned model failing` and
  the model line names the assigned version and the reason while its
  download or validation fails, instead of `online` and
  `no assignment yet`. The SDK logs a download under 1 KB in bytes
  (`(603 bytes)`), not `(0KB)` (review 4, cosmetic).
- Helm: `s3.publicUseSSL` (the server's `S3_PUBLIC_USE_SSL`, which defaults
  to `S3_USE_SSL`) sets whether the download URLs are `https`, apart from
  `s3.useSSL`, the server's own connection to the store. The chart README's
  first block (`s3.useSSL=true` with the bundled SeaweedFS) never became
  ready, and `false` signed `http://...:443` URLs (review 4, bug 2).
- Helm: SeaweedFS restarts when its S3 credentials change (it read them only
  at start, so `helm upgrade` with a new `s3SecretKey` crash-looped the
  server); the server has a startup probe that covers its 60-second wait for
  each dependency, where the liveness probe could restart it on a cold
  install (review 4, bug 9).
- Helm: a `server.env` name the chart sets itself (`S3_BUCKET`, say) is set
  once, to the `server.env` value, instead of twice; a `jwtSecret` or
  `signingMasterKey` shorter than 32 characters, or the two equal, is
  refused at install; `NOTES.txt` names the release's namespace (review 4,
  bug 12).
- `android/gradlew.bat` exists: the Android guide's Windows steps ran
  `.\gradlew.bat`, which was never in the repository. The wrapper scripts
  and jar are the ones Gradle 8.7 generates (the jar's sha256 is Gradle's
  published one), and `.gitattributes` keeps `*.bat` CRLF (review 4, bug 6).
- Android sample app: `-PfleetmlDebugHost=<host>` (or `fleetml.debugHost` in
  `android/local.properties`) sets the address the debug build may reach
  over plain HTTP and prefills it on the config screen; the debug network
  security config is generated from it. A phone over Wi-Fi no longer needs a
  source edit, and `-PfleetmlDebugHost=localhost` gives a phone over USB the
  right host instead of the emulator's `10.0.2.2`.
- A failed `fleetml push` upload says `upload failed:` once, and
  `fleetml telemetry events` prints confidence with 3 decimals instead of
  float32 noise such as `0.9700000286102295` (review 4, cosmetic).
- The Windows run's quickstart script cloned into `~/fleetml` after
  `rm -rf fleetml`; on a case-insensitive file system that is a checkout at
  `~/fleetML`. It now clones into a directory it creates with `mktemp` and
  removes nothing.
- Every shell script passes shellcheck 0.9 and 0.11; `docs/screenshots/demo/record.sh`
  no longer leaves its temporary directory behind when it stops early.

### Security

- Dependencies: google.golang.org/grpc 1.83.2 (GO-2026-4762, GO-2026-6061,
  GO-2026-6348, GO-2026-6443), golang.org/x/text 0.41.0 (GO-2026-5970,
  GO-2026-6629), golang.org/x/crypto 0.55.0 and
  github.com/klauspost/compress 1.18.7.
- Dependencies: golang.org/x/net 0.60.0 (GO-2026-6603, GO-2026-6610,
  GO-2026-6611, GO-2026-6612, GO-2026-6617) and github.com/jackc/pgx/v5
  5.9.2 (GO-2026-5004).
- Release binaries are built with Go 1.26.10 (0.7.0's were built with Go
  1.24.0); with it, govulncheck finds no reachable vulnerability in the
  server, CLI or agent, where Go 1.26.2 leaves 29 in the standard library
  (review 4, bug 5).

### Docs

- Quickstart: "Next: a real device" before Clean up (Android, iOS, the Linux
  agent, Python); an "Upgrading" section for the stack (run as written from
  a 0.7.0 stack with data and devices: data, the admin key and the devices
  kept, migrations applied), the CLI, the SDKs, the agent and the chart;
  Clean up says what `-v` deletes and removes the env file, and no longer
  claims a Grafana volume; versions' DRAFT status and `push --promote` are
  explained once; a Kubernetes pointer; the tour's curl lines use
  `$FLEETML_SERVER` (review 4, docs gaps 3, 4 and 14).
- README: `scripts/demo.sh` leaves the dev stack running and how to stop it;
  links to Upgrading and the Linux agent. README, the quickstart, the guides
  and the Python README: Contact names GitHub Issues for bugs; the stale
  "`pip install fleetml` from 0.6.0" sentence is gone (review 4, docs gaps 6
  and 13).
- The device guides save `S3_PUBLIC_ENDPOINT` in `~/.fleetml.env` instead of
  setting it for one command, and say what it changes for USB, the
  emulator and the sample app (review 4, docs gap 1).
- Android guide: keep the phone unlocked with the app in front; Publish and
  deploy comes before On your phone; a new project from the command line
  (AGP 8.5.0, Kotlin 1.9.24, Gradle 8.7; built as written against the SDK on
  Maven Central); the first-run logcat matches the code (the background
  line comes when the app goes to the background); the stale-settings note
  and its error for a phone; `lifecycle-runtime-ktx` 2.8.7 with an AndroidX
  activity; TensorFlow Lite 2.17.0 throughout; the lock screen in the
  production checklist (review 4, docs gaps 2 and 9).
- iOS guide: the team ID from a terminal (the certificate's `OU`, not the ID
  `find-identity` prints); keep the phone unlocked for steps 6 to 8; the
  closed-port offline test on the phone; `fleetml telemetry events --device`
  as the telemetry check; `log collect` needs root, Console.app does not;
  `Secrets.xcconfig` written 0600; `UIRequiresFullScreen` in `project.yml`
  (the device build's orientation warning is gone); the trust error after a
  free-team rebuild; an unavailable wireless phone; a simulator of your own
  (review 4, docs gaps 2 and 8).
- Linux agent guide: `apt-get install -y curl ca-certificates` for images
  without curl; `iris` in the telemetry example; an Upgrade section
  (`install.sh` again keeps the config and state and restarts the agent; a
  0.8.0 agent works with a 0.7.0 server); "Try it locally" names what it
  needs, the compose project it starts and the variables that move it; the
  archive's LICENSE and NOTICE (review 4, docs gap 10).
- Python client: the tour creates a project of its own name, prints the
  simulator line with a seed of its own, counts the project's devices
  (`project=`), and ends with a Clean up step that deletes the project; the
  page says where the admin key comes from, that the package is typed,
  that only server calls raise `FleetMLError`, and lists `INVALID_TRANSITION`,
  the `FAILED` deployment status, drift's baseline and every new method. The
  stale "`pip install fleetml` from 0.6.0" sentence is gone from the page and
  from the package README (review 4, Python docs gaps).
- Android guide, "From Windows": one linear path in PowerShell (Git for
  Windows, a clone, publish and deploy with `ConvertFrom-Json` so the
  enrollment key stays in one window, the three `adb reverse` lines,
  `gradlew.bat` with the key and `-PfleetmlDebugHost=localhost`, the logcat
  lines, the stale-key reset), then Wi-Fi with a firewall rule for private
  networks only and its removal line, then the emulator. The quickstart's
  "On Windows" links to it and names Docker Desktop's licence terms and the
  Docker Engine route the Windows run uses; "Get the CLI" names Smart App
  Control and `$ProgressPreference`; the Python client's page has a Windows
  line (review 4, Windows docs gaps).
- Quickstart and README "Run FleetML": the block writes the three secrets and
  `S3_PUBLIC_ENDPOINT` to `~/.fleetml.env` (0600, only if the file does not
  exist yet) and sources it before anything starts, and it prints no secret
  (the admin key was echoed before the "save them" step, and §6 said
  `echo $FLEETML_ADMIN_KEY`). The observability step and the port-80
  override source the file first, so a saved `S3_PUBLIC_ENDPOINT` survives
  the `up -d` (review 4, docs gaps 1 and 5).
- The prod compose file's errors for a missing `JWT_SECRET` or
  `SIGNING_MASTER_KEY` say to `source ~/.fleetml.env` and never to generate
  new values for an existing stack; they suggested `openssl rand`, which on
  an existing stack breaks publishing and every device token (review 4,
  bug 7).
- The Helm chart's README is rewritten and was run as written on kind: the
  published images (no build or `kind load`), the secrets generated once
  into a 0600 file with what may and may not change, the install with the
  bundled store and `s3.publicEndpoint` explained, a try-it block with
  `fleetml simulate --grpc`, Upgrading (`helm upgrade --reuse-values`, what
  restarts), Production (TLS in front, `s3.publicUseSSL`, TimescaleDB for an
  external Postgres, what the chart does not set, backups), every value, and
  Uninstall (review 4, docs gap 7).
- Linux agent: "Identity and offline behaviour" gives the measured bound (a
  3-minute server pause: the warning after 32 s, the first heartbeat 42 s
  after the server was back) and the two log lines; the container one-liner
  keeps the state folder in a volume, so a recreated container is the same
  device; the supervisor and `-retry-failed` commands use `exec`.
- Quickstart: the arm64 Windows binary is now run on `windows-11-arm` by the
  Windows run workflow, next to the amd64 one on `windows-latest`.
- RELEASING.md describes the public repository: its code security, Actions
  and package settings, the branch rule on `main`, and both version tests
  among the files a release bumps.
- A Contact section (hello@fleetml.dev, security@fleetml.dev, GitHub
  Discussions) ends the README, the guides and the Python client's README,
  and `SUPPORT.md` says where questions and bugs go.

## [0.7.0] - 2026-10-23

### Added

- The Android SDK 0.7.0 is on Maven Central and the Python client 0.7.0 on
  PyPI (`pip install fleetml`).
- `scripts/agent-install-test.sh` has two more cases: an image with the
  systemd package but without systemd running, and an Alpine (BusyBox)
  container without bash or `useradd`. CI runs the Android sample app's unit
  tests next to the SDK's.

### Changed

- Linux agent: `install.sh` is POSIX `sh` and creates its user with BusyBox's
  `addgroup` and `adduser` where `useradd` is missing, so it runs on BusyBox
  images. It decides on the service unit by whether systemd is running
  (`/run/systemd/system`), not by whether `systemctl` is installed; without a
  running systemd it prints `No running systemd on this machine: ...`.
- Linux agent: the example config's `post_activate` touches
  `/var/lib/fleetml-agent/model-switched`, which the shipped unit can write,
  instead of a file under `/run`, which it cannot.

### Fixed

- Android SDK: `FleetML.syncNow()` called at start-up (as in the guide's
  snippet) ran next to the first automatic heartbeat, so the first run
  downloaded, validated and activated the model twice, `onModelReady` fired
  twice and an offline start logged `Heartbeat failed` twice; two installs of
  one model at once could also break each other's download and leave the
  device without the model for an hour. A sync while a heartbeat is in
  flight now joins it, and the `ModelManager` skips a variant that is
  already being installed, so each version is downloaded and activated once
  and `onModelReady` fires once.
- iOS SDK: an app that lists `com.fleetml.sdk.refresh` in
  `BGTaskSchedulerPermittedIdentifiers` without calling
  `FleetML.registerBackgroundTask()` crashed the first time it went to the
  background (`No launch handler registered for task with identifier`); the
  SDK now skips the request and logs an info line naming the missing call.
- iOS SDK: no more Swift 6 warnings from `FleetML.swift` (`'bgTask' mutated
  after capture by sendable closure` and the main-actor `shared` access) in
  apps that build it.
- Linux agent: `install.sh` on an image with the systemd package but without
  systemd running copied the unit and exited 1 at `systemctl daemon-reload`.
- Android sample app: its debug network security config has a line for the
  Mac's LAN address (`mac-lan-ip.invalid` to replace), without which a phone
  over Wi-Fi registered and then failed every download with `CLEARTEXT
  communication ... not permitted`.

### Docs

- The Android, iOS and Linux agent guides lead with the real device and the
  real deployment: a production-shaped configuration, the phone (USB, then
  Wi-Fi) or the server on another machine, publish and deploy with the key
  an app or edge developer is given, offline and update tests, and a
  production checklist; the emulator, the simulator and the local try-out
  follow as shorter variants, then Windows, the sample apps and the
  contributor sections.
- Android: the host, port and TLS from Gradle properties, with debug builds
  on `localhost`, `10.0.2.2` or the LAN address; AGP 8's `buildConfig`
  switch; the API's signatures; which key does what (checked with project
  `admin` and `operator` keys); the offline lines and reconnect times as
  measured; the README snippet gains the TFLite line.
- iOS: the app's settings from an `.xcconfig` through `Info.plist`, an
  `AppModel` that prints its callbacks, a complete `project.yml` for a phone,
  `devicectl` over USB and Wi-Fi, `log collect`, the -1009 and -1004 cases,
  the `package:` identity, `from: "0.7.0"` throughout.
- Linux agent: "Server on another machine" as its own section with a `sed`
  line for `download_host_override`; a `docker run --init` example and the
  entrypoint `exec` line; `-o json | jq -r .key`; the validator by hand with
  `su` too; removing the rejected copy; reloading the inference process with
  a path unit, and why `sudo` cannot work under the unit's
  `NoNewPrivileges=true`.
- Quickstart: section 3 always shows the canary cohort, `D2`, `EXP` and one
  `SIM` line are saved for a second shell; the CLI installs without `sudo`;
  "On Windows" opens with how the two shells and two clones fit together,
  and adds the PowerShell version check, `docker compose version`,
  `.wslconfig` and `Unblock-File`.

## [0.6.0] - 2026-10-21

### Added

- The Android SDK 0.6.0 is on Maven Central and the Python client 0.6.0 on
  PyPI (`pip install fleetml`), the first version of each published since
  0.4.0 and the first on PyPI.
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
- Android "From a clone": the two Gradle plugins and the `sdkVersion`
  property the SDK module needs from the app's build, without which Gradle
  stops with `Plugin [id: 'com.google.protobuf'] was not found`.

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

[0.8.0]: https://github.com/fleetml/fleetml/compare/v0.7.0...v0.8.0
[0.7.0]: https://github.com/fleetml/fleetml/compare/v0.6.0...v0.7.0
[0.6.0]: https://github.com/fleetml/fleetml/compare/v0.5.0...v0.6.0
[0.5.0]: https://github.com/fleetml/fleetml/compare/v0.4.0...v0.5.0
[0.4.0]: https://github.com/fleetml/fleetml/compare/v0.3.0...v0.4.0
[0.3.0]: https://github.com/fleetml/fleetml/compare/v0.2.0...v0.3.0
[0.2.0]: https://github.com/fleetml/fleetml/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/fleetml/fleetml/releases/tag/v0.1.0
