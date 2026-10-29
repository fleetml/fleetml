# Changelog

All notable changes to FleetML are recorded here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and versions follow
[Semantic Versioning](https://semver.org/) (before 1.0, minor versions may
change APIs).

## [0.10.7] - 2026-10-29

### What changed for readers

Fixes every finding of the developer review of 0.10.6: one regression of
0.10.6 in the iOS guide, two 0.10.6 docs fixes that stopped one step short,
seven product bugs and the docs gaps each platform's reviewer hit. Upgrading
from 0.10.6 needs nothing. Requests 0.10.6 accepted that 0.10.7 refuses
(400 naming the reason): a deployment whose `canary_stages`,
`rolling_config` or `health_gate` does not match its strategy (0.10.6
dropped them, so such a deployment went to every device at once), and a
variant pushed into an ARCHIVED version. A version created as `v2.0.0` is
now stored as `2.0.0` (versions stored with a `v` before stay as they are).

### Fixed

- Regression of 0.10.6 (docs): the iOS guide said a registered device that
  starts with another project's key is refused; it moves to that project,
  as the guide says elsewhere.
- Server: `canary_stages` without `"strategy": "CANARY"`, `rolling_config`
  without `"ROLLING"`, and a `health_gate` on an ALL_AT_ONCE deployment were
  dropped silently; they are refused with 400 naming the strategy they need.
  The dashboard sends a gate only for a staged rollout.
- Server, CLI and Python client: `v2.0.0` and `2.0.0` became two versions of
  one model, and `fleetml push --version v2.0.0` printed `vv2.0.0`; a leading
  `v` is dropped, so they are one version.
- Server: a push into an ARCHIVED version, which is final, uploaded the file
  first; it is refused before anything is uploaded.
- CLI and Python client: a push whose upload or check failed left the model
  and an empty DRAFT version behind; the push removes what it created and
  says so.
- CLI and Python client: an `.onnx` file pushed without `--framework`
  (`target_runtime`) became a TFLITE variant; the file's extension picks the
  runtime (`.onnx`, `.mlmodel`/`.mlpackage`, `.gguf`, TFLITE otherwise), and
  a `--framework` the extension contradicts is warned about.
- CLI: `config set-key` saved input that is not a key (UTF-16 without a
  byte order mark, control characters), and every later call said the
  server was unavailable; it refuses such input, and a saved key that cannot
  be sent is named as the problem.
- Dashboard: an ARCHIVED version offered Deploy and was listed in the new
  deployment's picker (the server refused it); neither any more.

### Docs

- Incomplete fixes of 0.10.6, now whole: the "never shows `model active`"
  entry in a new terminal goes into the clone and sources `~/.fleetml.env`
  before step 4; the Kubernetes second shell is one complete block that
  starts in the clone.
- Android: step 5 stops at a failed build or install, and Troubleshooting
  names `INSTALL_FAILED_UPDATE_INCOMPATIBLE` and a deployment that targets
  other labels; a second run of "Publish and deploy".
- iOS: 6a prints the model code when the model name is not known yet; the
  own-app step 3 says when no server runs; the first build's time.
- Linux agent: running the Connect block again after a later deploy moves
  the machines back to 1.0.0.
- Windows: the clean-up names the pulled images and the build cache; `wsl`
  without `--exec` also breaks the server line; after a port move, step 4's
  `export FLEETML_SERVER` line again.
- Helm: after the tour on Kubernetes alone, remove the `~/.fleetml.env` it
  created.
- Docker Engine's port-conflict message; an exported `COMPOSE_PROJECT_NAME`
  is not saved by step 2.

## [0.10.6] - 2026-10-28

### What changed for readers

A short, careful release: the four regressions 0.10.5 brought into the docs
and the CLI's messages are fixed, five small product bugs from the review of
0.10.5 are fixed, and the README was checked line by line, every block run
and every link and claim checked. Upgrading from 0.10.5 needs nothing. A
request 0.10.5 accepted that 0.10.6 refuses: a deployment of an ARCHIVED
version (400 naming the status), a version string that is not semver 2.0.0
(`01.0.0`, `+1.0.0`), and `?status=` on the version list with an unknown
status (400 with the allowed values).

### Fixed

- Regression of 0.10.5 (docs): CONFIG.md said the version list filters by
  `status`, which it ignored; the version list now filters by status (any
  case) and refuses an unknown one with the allowed values.
- Regression of 0.10.5 (docs): the iOS guide's own-app step 3 kept a
  `FLEETML_API_KEY` exported for a stack Get started had removed, and found
  no CLI in a new terminal; the block now works in any terminal, and Get
  started's clean-up unsets the key.
- Regression of 0.10.5 (docs): the Helm chart's rollback sentence said the
  server stays Not Ready; it now says the old revision's pod restarts while
  the current one keeps serving.
- Regression of 0.10.5 (CLI message): the no-key hint gave a bash line, also
  in PowerShell; it now names `fleetml config set-key` and FLEETML_API_KEY.
- Server: an ARCHIVED version could be deployed; it is refused with 400
  naming the status (DRAFT versions deploy as before: a new version is DRAFT).
- CLI: `config set-key` saved a UTF-16 key file or pipe (what PowerShell 5.1
  writes with `>`) as garbage; it decodes UTF-16 with a byte order mark.
- CLI and Python client: a push with a quantization, runtime, version or
  `--metadata` the server would refuse created the model and an empty DRAFT
  version first; it is refused before anything is created.
- Server: versions follow semver 2.0.0: `1.0.0-rc.1` and `1.0.0+build` are
  accepted, leading zeros (`01.0.0`) and a sign (`+1.0.0`) refused, and the
  message names the input.
- Server: a health gate's `on_failure` is accepted in any case, like
  `strategy`.

### Docs

- Android: the launch lines use `adb shell am start -n <id>/.MainActivity`
  (the `monkey` form launches nothing on API 35 emulators); the
  `In the background: finishing the sync through WorkManager` line explained;
  step 5 run again, a restart's log, one record per FleetML app.
- iOS: `The specified device was not found`, `this device is already
  registered`, the own app's `FleetML cannot register:`, the masked key field.
- Linux agent: a deleted project's `registration failed` lines, the window
  while a new `models_dir` cannot be written, the Upgrade block, a
  `models_dir` that is a mount point.
- Windows: the stack's images go with `docker image rm`; run `fleetml config
  set server` again after moving the REST port.
- Python: what a version string may be; the status transitions listed.
- Helm: a second shell for the tour sources `~/.fleetml.env`;
  `fleetml-secrets.env` is in `.gitignore` and removed at clean-up; the
  `broken pipe` form of a port-forward ending; StatefulSet names for another
  release name.
- README and every Get started: step 1 expects Docker Compose v2 or later
  (current Compose prints v5); the step-4 PATH line in Troubleshooting.

## [0.10.5] - 2026-10-28

### What changed for readers

Fixes a regression of 0.10.4 in the Android SDK and every finding of the
developer review of 0.10.4. An experiment can only be concluded with one of
its two versions, and a deployment request with a key the server does not
know is refused instead of ignored. Upgrading from 0.10.4 needs nothing; a
request that 0.10.4 accepted with a misspelled key (`stratgy`, a canary
stage's `duration`) now answers 400 naming the key.

### Fixed

- Android SDK (regression of 0.10.4): a cold start in the foreground with a
  stored registration took the background path, logging `In the background:
  finishing the sync through WorkManager` and scheduling a job; it now
  starts the timers as before 0.10.4, and a start that really happens in the
  background still finishes its first sync.
- Android SDK: a model loaded from the cache at start logs `Model <name>
  <version> is active`, so Get started's step 5 passes when run again.
- CLI: `config set-key` from a pipe or a file drops a UTF-8 BOM and CR/LF
  (a PowerShell pipe saved an unusable key).
- Server: `conclude` accepted a winner that was neither arm; it must be the
  control or the treatment version, and a malformed `winner_version_id`
  says so instead of "required".
- Server: unknown keys in a deployment body or a canary stage were ignored;
  they are refused with 400 naming the key.
- iOS sample app: with `-fleetmlPort` and `FLEETML_PORT` both given, the
  config screen showed the argument's port while the SDK used the
  environment's; it shows the port in use.

### Changed

- The CLI's missing-key error names `~/.fleetml.env`; `server unavailable`
  names `FLEETML_SERVER`.
- An experiment's `split_ratio: 0` is refused with the value given; an
  invalid semver names the input as given.
- The Linux agent's unit points at a drop-in for `ReadWritePaths=` instead
  of a line to uncomment; `install.sh` replaces the 0.10.4 unit as shipped.

### Docs

- Android: the two-device clean-up names `ANDROID_SERIAL`'s device and the
  line for the other.
- iOS: own-app step 3 pastes as written; 6c says when the screen shows iris
  and what "bundled iris (no assignment yet)" means; step 8 runs from the
  clone.
- Linux agent: the `download_host_override` block can run again and enables
  the service; Remove also takes the drop-ins, the `.new` unit and a moved
  `models_dir`; the `read-only file system` error quoted; `docker pause`
  timing and wording.
- Windows: `config set server` in the second "for every window" place,
  Docker Desktop's port message, the clean-up of the `Path` entry and the
  clone.
- Python: `push(promote=True)` of the same file again is 409; a call after
  `close()`; unknown list filters are ignored (CONFIG too).
- Helm: an own-Secret release upgrades without `-f fleetml-secrets.yaml`;
  a rollback to a 0.10.2 revision writes that revision's secrets first;
  `helm lint` does not check `server.env` ports; the CLI on `PATH` in a new
  terminal; the `kubectl apply` warning and the port-forward error lines.
- Backend: the tour's second shell goes into the clone; step 2 again
  recreates the server and dashboard; a program on all interfaces on macOS
  goes unnoticed too; a second stack's home folder can be anywhere.

## [0.10.4] - 2026-10-27

### What changed for readers

An experiment now belongs to its project the way a deployment does, so one
project's devices can no longer be given another project's model. The Linux
agent's upgrade keeps a unit you edited, the Helm upgrade note is the first
step of Upgrading, and a clean-up whose compose command fails no longer
deletes the settings of a running stack. Upgrading from 0.10.3 needs
nothing; an experiment request with `split_ratio` outside 1-99 or with a
version of another project, which 0.10.3 accepted, is now refused.

### Fixed

- Server: an experiment accepted model versions of another project (an
  operator of one project could run another project's model on its
  devices), answered 500 for an unknown project and stored a `split_ratio`
  outside 1-99 as 50; arms of another project are refused with 400,
  unknown projects and versions are 404, and `split_ratio` must be 1-99.
- Server: a deployment of an unknown version answers 404 instead of "not
  deployable"; `experiments?project_id=` that is not a UUID answers 400.
- Linux agent: `install.sh` replaced an edited unit on every run, dropping a
  `ReadWritePaths=` line, so models outside the state folder failed after an
  upgrade; an edited unit is kept and the release's goes next to it.
- Android SDK: an app sent to the background before its first screen
  registered and then did nothing until opened again; the first sync now
  finishes in the background.
- Get started's clean-up in every guide removes `~/.fleetml.env` only after
  `docker compose ... down` succeeded.

### Changed

- `fleetml enrollment-key revoke <key ID>`.
- The CLI's 401 names the server that refused; `config set-key` and
  `config set` save only what was set (not the default server).
- A STAGED model version can be archived.
- The Windows run's WSL job warns when the runner has no WSL 2 instead of
  passing silently.

### Docs

- Android: the emulator runbook builds in a subshell; install lines pass
  `-PfleetmlDebugPort`; Wi-Fi after a second step 4 times out once, and how
  to finish it.
- iOS: the offline test's closed port through `SIMCTL_CHILD_FLEETML_PORT`;
  own-app step 2 waits for the server.
- Linux agent: `mkdir -p` in the Install block, `models_dir` on another disk
  through a systemd drop-in, `sudo journalctl`, revoking a key.
- Windows: `fleetml config set server` with the stack's port; the clean-up
  removes the saved key.
- Python: the README snippet runs as written, twice.
- Helm: the upgrade note first (it also finishes a failed upgrade), `git
  fetch --tags`, `helm rollback`, changing an own Secret, the dashboard
  port-forward in NOTES.txt and after an upgrade.
- Smaller lines in ADVANCED, QUICKSTART and CONFIG.

## [0.10.3] - 2026-10-27

### What changed for readers

A first start whose port is taken now stops in seconds with a pointer to
Troubleshooting instead of waiting three minutes. The Linux agent puts
models on any file system, the Helm chart upgrades across chart versions
without a manual step (once more for releases installed with chart 0.10.2
or older: see the Helm upgrade note below), and a misspelled value is
refused instead of ignored, in the chart's values and in a deployment's
health gate. Upgrading the compose stack from 0.10.2 needs nothing.

### Helm upgrade note

Releases installed with chart 0.10.2 or older need one step before their
first upgrade to 0.10.3: `kubectl -n fleetml delete statefulset
fleetml-postgres fleetml-nats fleetml-s3 --cascade=orphan` (pods and data
stay; the upgrade takes them over). Those charts put the chart version in
the bundled StatefulSets' volume templates, which Kubernetes does not let
an upgrade change. From 0.10.3 on the labels stay the same.

### Fixed

- Get started step 2 skips its wait when `docker compose up` fails and says
  "Start failed: see Troubleshooting", and prints the project list and the
  dashboard line only when the server answers.
- Linux agent: a `models_dir` on another file system than the state folder
  never activated a model (`invalid cross-device link` on every heartbeat);
  the agent now copies the download into place, and reports a model it
  cannot put into `models_dir` once as `MODEL_LOAD_FAILED`. After a change
  of `models_dir` it downloads its models again instead of listing a path
  that does not exist.
- Helm: every upgrade to a new chart version failed on the bundled
  StatefulSets (`spec.volumeClaimTemplates: Invalid value`); their volume
  templates now carry only labels that do not change.
- Server: a deployment's health gate that sets only some keys keeps the
  defaults of the others (`{"min_events": 30}` allowed no error at all);
  an unknown key in `health_gate` or `rolling_config` is refused with 400
  naming it instead of being ignored.
- Server: the all-zeros UUID as a project or version ID of a deployment or
  experiment is an unknown ID (404 or 422) instead of "are required"; a
  malformed experiment body says "invalid JSON body: ...".
- iOS sample app: a refused enrollment key shows "Not registered · key
  refused" instead of "Server unreachable".

### Changed

- `fleetml config set-key` without the key reads it at a hidden prompt or
  from a pipe, so it stays out of the command line and the shell history.
- Helm: `values.schema.json` checks the values (`helm lint`, `install`,
  `upgrade`); `server.env.REST_PORT` and `GRPC_PORT` are refused, since the
  pod listens on fixed ports.
- Python client: a server URL without `http://` or `https://` raises a
  `ValueError` naming it.

### Docs

- README step 7 is QUICKSTART's clean-up, which also stops the tour's
  simulator and Grafana; the tour and Windows lines list Get started's
  project.
- Android: the Wi-Fi Troubleshooting covers `this device is already
  registered`, a block puts the download address back to `localhost`, the
  phone keeps its device ID over Wi-Fi, the Emulator block starts the app.
- iOS: 6c's message in ADVANCED, a device keeps its project whatever the key
  once registered (by design), the offline test's real log lines.
- Linux agent: the models folder's owner, the override block restarts the
  agent, `network is unreachable` as a TLS-mismatch form, what `-label` does.
- Helm: the upgrade note, the dashboard port-forward after an upgrade, the
  values schema.
- Python: the server URL's form, the README snippet's project, empty lists
  for an unknown project; ROLLOUTS: partial health gates; the Grafana port.

## [0.10.2] - 2026-10-26

### What changed for readers

On an iPhone you trust the developer once per phone: Get started's clean-up
now leaves the sample on the phone, because iOS forgets the trust when the
last app from your Mac is removed, and every later install from that Mac
needs no tap. Every step that waits gives up with a pointer to
Troubleshooting, and Troubleshooting covers the reason it gave up. Upgrading
from 0.10.1 needs nothing; Android and iOS apps register once more after the
update (with their token, so nothing to do).

### Fixed

- Get started step 2 waited forever when the stack could not start (a port
  another program holds); it gives up after 3 minutes and points to the
  Troubleshooting entry, which shows the taken-port case and the fix.
- Android and iOS SDKs: an app built with another enrollment key or server
  registered only when its labels or SDK version changed, so it stayed in its
  old project; a new key, host or port now registers it again, and the
  server moves it to the new key's project.
- `fleetml simulate`: a device whose token the server refuses (after a JWT
  secret rotation, for example) registers again with the enrollment key and
  says so, where it retried the refused token forever without a word.
- Server: a deployment of a model version from another project is refused
  with 400 (deleting that other project then failed with 500), a deployment
  in a project that does not exist answers 404 instead of 500, and a
  deployment body that is not valid JSON says "invalid JSON body: ..."
  instead of naming the required fields.
- `scripts/demo.sh` and `scripts/demo-agent.sh` set their server's download
  address themselves (in a terminal that had sourced `~/.fleetml.env` every
  upload went to that stack's port) and print a failed push instead of
  exiting without a word.
- iOS Get started step 6c succeeds when run again on a phone that already
  registered (it waits for the phone to check in, not for a new device), and
  it recognises the untrusted and expired-profile launch failures itself.
- Python client: `devices.list("<project ID>")` raises a TypeError that
  names `project=` instead of an AttributeError.

### Changed

- iOS: trust the developer once per phone. Get started's clean-up no longer
  uninstalls the sample from the phone; a separate line does, for when you
  are done with FleetML.

### Docs

- ADVANCED's port check prints the ports in effect; the tour's second-shell
  pickup puts the CLI on the `PATH`; the second-stack recipe keeps the
  Android SDK, `adb`'s key, Gradle's cache and the keychain.
- Android: `this device is already registered` with the way out, and what
  `pm clear` costs.
- Helm: a Secret of your own step by step, port-forwards after an upgrade or
  uninstall, the dashboard's local port in "Try it".
- Linux agent: the Connect and verify block runs again, the heartbeat
  errors of a restarted server, the device ID in "Remove the agent".
- Python: unknown filter values and query parameters give an empty list or
  are ignored.

## [0.10.1] - 2026-10-26

### What changed for readers

The phone steps of Get started now work as written on a phone seen for the
first time, and every step that waits says so when it gives up. The iOS
guide's iPhone step picks the phone (or the one you name), builds for it so
Xcode adds it to the signing profile, and tells you each one-time tap
before it is due; the Android guide's phone step says which device it took
and stops after three minutes with a pointer to Troubleshooting. ADVANCED's
"Android: a phone over Wi-Fi" is a complete path, and "Other ports" is
right about the settings file and has a recipe for a second stack. Step 2
can run again from inside the clone. Upgrading from 0.10.0 needs nothing.

### Fixed

- Android SDK: going to the background in the middle of a sync (a phone
  locking between download and validation) is no longer logged as
  `Heartbeat failed` and does not mark the device offline; the background
  job finishes the interrupted sync, or a first sync that had not happened
  yet (a phone locking right after it registers), at once rather than an
  hour later.
- Android sample app: a build with another `-PfleetmlDebugHost` or
  `-PfleetmlDebugPort` and the same enrollment key now connects to the new
  address; it kept the one it had saved, so a phone moved from USB to
  Wi-Fi went on dialling `localhost`.
- iOS guide: Get started's iPhone step built for any iPhone, so a phone not
  yet in the signing profile was refused at install (`This provisioning
  profile cannot be installed on this device`); it needed the simulator
  step first, took the first paired iPhone when there were two, waited for
  a second device that only the simulator gave, and printed an `ERROR` line
  from `devicectl` every time.
- Docs: the port check in ADVANCED and the iOS guide missed
  `FLEETML_S3_PUBLIC_PORT`; "Other ports" said an exported port moves a
  stack that has started once (the saved settings file wins); the Wi-Fi
  section used a `$EK` that Get started never sets; the Python tour's
  simulator line ignored a moved gRPC port; `fleetml-prod-server-1` was
  hard-coded where the stack can have another name.

### Changed

- Docs: every guide creates and reads the enrollment key as
  `FLEETML_ENROLLMENT_KEY`. The dashboard step puts the admin key on the
  clipboard in WSL and on Linux with `xclip` too, and prints the dashboard's
  address. The waits for a device print a line when they give up.
  Troubleshooting covers the Android phone, the iPhone, the simulator and a
  dashboard that does not open. The Linux agent guide gains the config
  lines in Connect and verify and a Remove the agent section; the Helm
  chart's Get started has a local port for the dashboard and stops only its
  own background jobs at clean-up.

## [0.10.0] - 2026-10-26

### What changed for readers

Every guide now opens with **Get started**: a numbered list of copy-paste
blocks with nothing to choose, from a fresh clone to one device with a
model on it, the dashboard and a clean up, each block followed by the line
it prints. The README's device is the Linux agent in a container on the
same machine; the Android guide puts the sample app on a phone over USB,
the iOS guide on the simulator and then an iPhone, the Python guide and the
Helm chart have their own. Everything that is a choice (other ports, a
phone over Wi-Fi, Android Studio on Windows, the Helm variants, upgrades,
the settings file) moved, unchanged, to the new
[docs/ADVANCED.md](docs/ADVANCED.md), with a pointer where it was. Two CI
jobs keep it that way: `Docs golden path` runs the Get started blocks of
the README, QUICKSTART, LINUX_AGENT and PYTHON_CLIENT against each change,
and `Docs lint` fails on a port number in a command, a key on a command
line, a version that disagrees with this file, a malformed entry here, or a
golden block that differs between two guides. Upgrading from 0.9.1 needs
nothing for the server, the CLI or the SDKs; Python callers see the two
client changes under Changed.

### Changed

- Python client: `TelemetryEvent.confidence_score` is `Optional[float]`,
  `None` for an event that reported no confidence (a MODEL_LOAD, for
  example), where it was `0.0`; `push(promote=True)` raises when the server
  refuses a step (from ARCHIVED), where it returned the version unchanged.
- Docs: the guides open with Get started, and their options moved to
  docs/ADVANCED.md (above). Every command reads the stack's saved ports
  (`${FLEETML_REST_PORT:-18080}`, `${FLEETML_GRPC_PORT:-19090}`,
  `${FLEETML_S3_PUBLIC_PORT:-19100}`), so a stack started on other ports
  needs no edits; prose names the defaults in words.
- Docs: no command puts an enrollment key on a command line: Android's
  Gradle lines take `ORG_GRADLE_PROJECT_fleetmlEnrollmentKey`, the iOS
  sample app `SIMCTL_CHILD_`/`DEVICECTL_CHILD_` variables, the Linux
  container `-e FLEETML_ENROLLMENT_KEY` and a machine's config `read -rs`
  and `printf | tee`.
- `install.sh` without systemd says the state folder "is ready" and prints
  the restart hint only when it replaced a binary.

### Added

- `fleetml-agent -download-host-override`, the config's
  `download_host_override` as a flag; `FLEETML_ENROLLMENT_KEY` also
  replaces config.example.yaml's placeholder key, and `-h` names it.
- The Android sample app connects by itself on the first start of a build
  that carries an enrollment key, again whenever a build brings a new one,
  and takes its gRPC port from `-PfleetmlDebugPort`; the iOS sample app
  takes `FLEETML_HOST`, `FLEETML_PORT` and `FLEETML_ENROLLMENT_KEY` from its
  environment and logs line by line.
- `fleetml push --promote` prints what it promoted
  (`v1.0.0 promoted to STAGED → ACTIVE`).
- The prod compose file publishes Grafana on `FLEETML_GRAFANA_PORT`.
- `scripts/docs-golden-path.sh` and `scripts/docs-lint.sh`, and the CI jobs
  `Docs golden path` and `Docs lint`.
- The Helm chart's NOTES.txt names the reader's own port variables and
  prints the object store's port-forward only with the bundled SeaweedFS.

### Fixed

The findings of the seventh developer review (review 7, on 0.9.1):

- Python client: `push` found an existing model or version only on the
  server's first page of 20 and then failed with `409 CONFLICT`; it now
  pages 100 at a time, as the CLI does. `versions.promote(status="active")`
  skipped the STAGED step; the status is now taken in any case.
- `fleetml simulate --help`'s example put the key on the command line; it
  uses `FLEETML_ENROLLMENT_KEY` now.
- iOS: step 2's network check and the never-downloads diagnosis named
  19100 on a stack with another model files' port (the review's iOS 4);
  step 6 now says Ctrl-C on the `--console` line ends the app.
- Android: the `adb reverse` lines, the emulator section and From Windows
  named the default ports; the backup rules are in the skeleton's manifest
  and the Production checklist.
- Linux agent: the agent-side `download_host_override` lines and
  config.example.yaml named 19100; `-retry-failed` without systemd uses
  `setpriv`.
- QUICKSTART: the tour's `simulate` lines follow a moved gRPC port
  (`--grpc`); README: the port-80 override paragraph, which QUICKSTART had
  replaced, is gone, and Upgrading says what to do with a 0.9.0 override
  file.
- Helm chart README: its example ports collided with the quickstart's, and
  on macOS a port-forward takes `localhost` from Docker Desktop without an
  error; Get started uses 38080, 39090 and 39100, and ADVANCED says how to
  check other ports. Each restart past `server.dependencyTimeout` waits
  another full budget, as the README now says.
- The 0.9.1 entry's paging sentence, which did not parse.

## [0.9.1] - 2026-10-25

The fixes from the sixth developer review (seven reviewers on the released
0.9.0, each rating it 5): one Android crash, one Helm restart, and the
rest of what they found. A client that sends valid requests needs no
change. Two refused requests answer differently (under Changed), and
`DEPENDENCY_TIMEOUT` now counts the whole start-up wait. The upgrade from
0.9.0 is the [Upgrading](docs/QUICKSTART.md#upgrading) block.

### Changed

- REST: a `limit` outside 1 to 100 (1 to 1000 for `GET /telemetry/events`)
  or a negative `offset` answers `400 VALIDATION_ERROR` naming the range,
  for example `limit must be between 1 and 100, got 500`, instead of being
  clamped without a word (`GET /devices` had no cap at all). A client that
  asked for more than 100 must page with `offset`, as the CLI and the
  dashboard already do (review 6; the Python client's `push` pages since
  0.10.0).
- `DEPENDENCY_TIMEOUT` is the total time start-up waits for Postgres, the
  object store and NATS together, not the time for each; the Helm chart's
  `180s` and the compose default `60s` stay. Meanwhile `/livez` answers 200
  and `/readyz` 503 with `{"status":"starting","waiting_for":"<dependency>"}`
  (review 6).
- Docs: every `fleetml simulate` line (the tour, the chart README, the line
  the Python tour prints, `scripts/demo.sh`) passes the enrollment key in
  `FLEETML_ENROLLMENT_KEY` instead of `--enrollment-key` (review 6).

### Added

- Compose: `FLEETML_REST_PORT`, `FLEETML_GRPC_PORT` and
  `FLEETML_DASHBOARD_PORT` (defaults 18080, 19090, 80) move the published
  ports, next to `FLEETML_S3_PUBLIC_PORT`; the Run block saves all four in
  `~/.fleetml.env`. Moving the dashboard off a taken port 80 is one line
  in that file instead of an override file.
- Android SDK: backup rules for the registration
  (`@xml/fleetml_backup_rules`, `@xml/fleetml_data_extraction_rules`) for
  an app to point at or copy; the SDK does not set them itself, which would
  break the build of every app with its own rules.

### Fixed

- Android SDK: an app whose stored registration cannot be decrypted (restored
  from a backup, moved to a new phone, or copied from another install) no
  longer crashes at every launch with `AEADBadTagException`. The SDK logs
  `Stored registration unreadable (...); deleted it, registering again with
  the enrollment key`, deletes the file and registers again with the same
  device ID (review 6).
- Helm: a server that waits for its dependencies on a cold node is no
  longer restarted by the 200 s startup probe; `/livez` answers before the
  waits (review 6).
- CLI: `fleetml push --promote` of a further variant into an ACTIVE version
  succeeds (`vX was already ACTIVE`) instead of failing with `cannot
  transition from ACTIVE to STAGED`; it takes only the steps a version has
  not passed. `push` finds an existing model or version past the first 20,
  and `project delete`'s summary counts past 1000 models.
- REST: an unknown version status (`"LIVE"`) answers `400 VALIDATION_ERROR`
  listing DRAFT, STAGED, ACTIVE and ARCHIVED instead of `409
  INVALID_TRANSITION` (review 6).
- CLI: `fleetml telemetry events` leaves the confidence blank for an event
  without one instead of printing `0.000` (review 6).
- Linux agent: no `heartbeat failed` or `sending telemetry failed` warning
  for a call its own shutdown cancelled (review 6).
- Docs: the device guides' `S3_PUBLIC_ENDPOINT` lines keep a moved
  `FLEETML_S3_PUBLIC_PORT` instead of writing 19100; the Upgrading block
  finds the server container through compose, so it works with
  `COMPOSE_PROJECT_NAME` (review 6).
- `scripts/demo-agent.sh` runs its clean-up on macOS's bash 3.2 when it
  fails early, and shows why compose did not start (review 6).
- Dashboard: the Drift chart's time axis shows seconds for a short history
  instead of repeating a minute; the login page names `~/.fleetml.env`.
- CI: on `windows-11-arm` the Windows run's WSL row reads `skipped (WSL 2
  not available on this runner)` instead of `pass`.
- CHANGELOG 0.9.0: the version label is on the release images; an image
  built by `docker compose` is labelled `dev` unless `FLEETML_VERSION` is
  set (the server's own version comes from the clone's tag either way).

## [0.9.0] - 2026-10-25

The fixes from the fifth developer review (seven reviewers on the released
0.8.0), and dependency updates. Two changes are visible to API clients: a
few refused requests answer with other error codes, and three Python client
arguments are keyword-only; both are under Changed with what to do. The
upgrade from 0.8.0 is the [Upgrading](docs/QUICKSTART.md#upgrading) block,
which now also keeps a 0.7.0 setup's `S3_PUBLIC_ENDPOINT`.

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
- Dependencies: github.com/jackc/pgx/v5 5.11.0, lucide-react 1.61.0 in the
  dashboard, org.json 20260814 in the Android SDK, actions/setup-python 7
  in CI. The Android toolchain (AGP, Kotlin 2, Gradle 9, protobuf 4, gRPC
  1.85) and the libraries that need it (kotlinx-coroutines 1.11, mockk
  1.14, robolectric 4.17) move in an SDK release of their own.

### Added

- The Android SDK 0.9.0 is on Maven Central and the Python client 0.9.0 on
  PyPI.
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
- CLI: `fleetml telemetry events` has Time and Version columns (each
  version looked up once); `fleetml simulate` reads the enrollment key from
  `FLEETML_ENROLLMENT_KEY` when `--enrollment-key` is not given, which keeps
  it out of `ps` (review 5).

### Fixed

- CLI: `fleetml push --promote` reports a refused promotion (`uploaded ...
  but promoting it to STAGED failed: ...`, non-zero exit) instead of
  printing `✓ Pushed` with the version left at DRAFT; `fleetml project
  delete --help` says it asks for no confirmation and cannot be undone
  (review 5).
- Docs: `fleetml project delete $P` in QUICKSTART "Clean up" and the
  Android guide, for a server you keep; ANDROID_SDK "From Windows": x64
  only, a new window after winget, a time-stamped project name so the block
  runs twice, logcat runs until Ctrl+C, `Get-NetConnectionProfile`; the
  `deploy get` counts before the app registers (review 5).
- IOS_SDK.md: step 1 lists what needs a hand at the phone once (Developer
  Mode, trusting the Mac, Connect via network, Auto-Lock Never, the first
  launch's two prompts), so the terminal steps can run unattended; step 2
  has a Safari check of `/readyz` and the object store from the phone;
  step 5 quotes Xcode 26's `CoreDeviceService was unable to locate a
  device ...` next to `Connection interrupted`, and says a locked phone
  takes the install but not the launch; `log collect` takes the phone's
  name; step 9 says it needs Xcode and someone at the phone (review 5).
- Linux agent: the last telemetry flush on SIGTERM is bounded at 1.5 s
  (was 5 s), so a stop with the server unreachable takes under 2 s
  (`systemctl stop`: 1.64 s, 5.11 s before); `install.sh` prints
  `exec setpriv --reuid=fleetml-agent --regid=fleetml-agent --init-groups ...`
  for a supervisor, where `su` stayed as the agent's parent and SIGKILLed
  it 2 s after a SIGTERM (BusyBox keeps the `su` form, which works there).
  LINUX_AGENT.md: when the offline warning comes, the demo's server ports
  and its `down`, the enrollment key in `docker inspect` (review 5).
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

### Security

- pgx 5.11.0 includes 5.10's hardening of its decoders and SCRAM against a
  malicious or compromised database server.
- `fleetml simulate` reads its enrollment key from `FLEETML_ENROLLMENT_KEY`,
  which keeps it out of the process list; LINUX_AGENT.md says the
  container one-liner's key is visible to `docker inspect` and how to keep
  it in a read-only `config.yaml` instead.
- Helm: `fleetml-secrets*.yaml` is git- and helm-ignored, and a short
  `adminKey` is refused at install time.

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

[0.10.7]: https://github.com/fleetml/fleetml/compare/v0.10.6...v0.10.7
[0.10.6]: https://github.com/fleetml/fleetml/compare/v0.10.5...v0.10.6
[0.10.5]: https://github.com/fleetml/fleetml/compare/v0.10.4...v0.10.5
[0.10.4]: https://github.com/fleetml/fleetml/compare/v0.10.3...v0.10.4
[0.10.3]: https://github.com/fleetml/fleetml/compare/v0.10.2...v0.10.3
[0.10.2]: https://github.com/fleetml/fleetml/compare/v0.10.1...v0.10.2
[0.10.1]: https://github.com/fleetml/fleetml/compare/v0.10.0...v0.10.1
[0.10.0]: https://github.com/fleetml/fleetml/compare/v0.9.1...v0.10.0
[0.9.1]: https://github.com/fleetml/fleetml/compare/v0.9.0...v0.9.1
[0.9.0]: https://github.com/fleetml/fleetml/compare/v0.8.0...v0.9.0
[0.8.0]: https://github.com/fleetml/fleetml/compare/v0.7.0...v0.8.0
[0.7.0]: https://github.com/fleetml/fleetml/compare/v0.6.0...v0.7.0
[0.6.0]: https://github.com/fleetml/fleetml/compare/v0.5.0...v0.6.0
[0.5.0]: https://github.com/fleetml/fleetml/compare/v0.4.0...v0.5.0
[0.4.0]: https://github.com/fleetml/fleetml/compare/v0.3.0...v0.4.0
[0.3.0]: https://github.com/fleetml/fleetml/compare/v0.2.0...v0.3.0
[0.2.0]: https://github.com/fleetml/fleetml/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/fleetml/fleetml/releases/tag/v0.1.0
