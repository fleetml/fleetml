# Python client

`fleetml` on PyPI is a Python library for the FleetML server's REST API: a
training pipeline, a notebook or a CI job pushes a model, starts a rollout,
waits for it and reads what the fleet reports, on any operating system. It is
a client for the REST API only and has no device-side role (devices use the
[Android](ANDROID_SDK.md) and [iOS](IOS_SDK.md) SDKs or the
[Linux agent](LINUX_AGENT.md)). It uses the same words as the `fleetml` CLI,
which stays the command-line tool; the package installs no command.

## Get started

The client against a server on this machine, with one device on it: ten
minutes, copy and paste, nothing to choose. Each step says what you should
see; if you see something else, stop there and look it up in
[Troubleshooting](ADVANCED.md#troubleshooting). Use one terminal (bash or
zsh, on macOS or Linux) and keep it open until the end.

1. **Check the tools.** You need Docker (Docker Desktop on a Mac), git, curl
   and Python 3.9 or later ([installing them](ADVANCED.md#installing-the-tools)):

   <!-- golden: python-tools -->
   ```bash
   docker compose version && git --version && python3 --version
   ```

   You should see `Docker Compose version v2` and `Python 3.` on the last line.

2. **Start FleetML.** The first start takes a few minutes, to download and
   build the images:

   <!-- golden: start -->
   ```bash
   [ -f deploy/docker-compose.prod.yml ] || { [ -d fleetml ] || git clone https://github.com/fleetml/fleetml.git; cd fleetml; }
   [ -f ~/.fleetml.env ] || (umask 077; printf "export JWT_SECRET='%s' SIGNING_MASTER_KEY='%s' FLEETML_ADMIN_KEY='%s' FLEETML_REST_PORT=%s FLEETML_GRPC_PORT=%s FLEETML_DASHBOARD_PORT=%s FLEETML_S3_PUBLIC_PORT=%s S3_PUBLIC_ENDPOINT=localhost:%s\n" "$(openssl rand -base64 32)" "$(openssl rand -base64 32)" "$(openssl rand -hex 32)" "${FLEETML_REST_PORT:-18080}" "${FLEETML_GRPC_PORT:-19090}" "${FLEETML_DASHBOARD_PORT:-80}" "${FLEETML_S3_PUBLIC_PORT:-19100}" "${FLEETML_S3_PUBLIC_PORT:-19100}" > ~/.fleetml.env)
   source ~/.fleetml.env
   docker compose -f deploy/docker-compose.prod.yml up -d --build && for i in $(seq 90); do curl -sf localhost:${FLEETML_REST_PORT:-18080}/readyz && break; [ $i = 90 ] && echo "Not ready after 3 minutes: see Troubleshooting in docs/ADVANCED.md"; sleep 2; done || echo "Start failed: see Troubleshooting in docs/ADVANCED.md"; echo
   curl -sf -H "Authorization: Bearer $FLEETML_ADMIN_KEY" localhost:${FLEETML_REST_PORT:-18080}/api/v1/projects && echo && echo "Dashboard: http://localhost:${FLEETML_DASHBOARD_PORT:-80}  (log in with FLEETML_ADMIN_KEY from ~/.fleetml.env)"
   ```

   You should see `{"db":"connected","nats":"connected","s3":"connected","status":"ok","version":"<version>"}`.
   The block saved the server's secrets in `~/.fleetml.env`: keep that file.

3. **Install the client** in a virtual environment in the clone:

   <!-- golden: python-install -->
   ```bash
   python3 -m venv .venv && . .venv/bin/activate
   pip install -q fleetml
   python -c 'import fleetml; print("fleetml", fleetml.__version__)'
   ```

   You should see `fleetml <version>`.

4. **Publish the sample model and deploy it**, from Python. The block keeps
   the device's enrollment key in this terminal's environment for step 5,
   without printing it:

   <!-- golden: python-publish -->
   ```bash
   export FLEETML_SERVER=http://localhost:${FLEETML_REST_PORT:-18080} FLEETML_API_KEY=$FLEETML_ADMIN_KEY
   export FLEETML_ENROLLMENT_KEY=$(python - <<'EOF'
   import sys, time
   from fleetml import FleetML
   with FleetML() as fm:
       project = fm.projects.create(time.strftime("first-py-%H%M%S"))
       v = fm.push("models/iris/iris_v1.tflite", project=project.id, model="iris", version="1.0.0")
       print("deployment", fm.deployments.start(fm.deployments.create(project.id, v.id).id).status, file=sys.stderr)
       print(fm.enrollment_keys.create(project.id, "first-device").key)
   EOF
   )
   ```

   You should see `deployment COMPLETE`.

5. **Put a device on it**: the Linux device agent in a container on this
   machine. It registers with the enrollment key from step 4, downloads the
   model, checks it and makes it active:

   <!-- golden: device -->
   ```bash
   ARCH=$(uname -m | sed -e 's/x86_64/amd64/' -e 's/aarch64/arm64/')
   mkdir -p fleetml-agent && curl -fsSL "https://github.com/fleetml/fleetml/releases/latest/download/fleetml-agent-linux-$ARCH.tar.gz" | tar -xz -C fleetml-agent
   docker run -d --init --name fleetml-agent --add-host=host.docker.internal:host-gateway -e FLEETML_ENROLLMENT_KEY \
     -v "$PWD/fleetml-agent:/opt/fleetml-agent:ro" ubuntu:24.04 sh -c "/opt/fleetml-agent/install.sh /opt/fleetml-agent/fleetml-agent > /dev/null &&
     exec setpriv --reuid=fleetml-agent --regid=fleetml-agent --init-groups /usr/local/bin/fleetml-agent -config /etc/fleetml-agent/config.yaml \
       -server host.docker.internal:${FLEETML_GRPC_PORT:-19090} -download-host-override host.docker.internal:${FLEETML_S3_PUBLIC_PORT:-19100}"
   for i in $(seq 60); do docker logs fleetml-agent 2>&1 | grep 'model active' && break; [ $i = 60 ] && echo "No model active after 2 minutes: see Troubleshooting"; sleep 2; done
   ```

   You should see `msg="model active" model=iris version=1.0.0`.
   On a phone instead: the [Android](ANDROID_SDK.md#get-started) guide
   (a phone on a USB cable) or the [iOS](IOS_SDK.md#get-started) guide
   (the simulator, then an iPhone).

6. **See it in the dashboard.** This puts the admin key on the clipboard (on
   a Mac, in WSL, or on Linux with `xclip`; elsewhere it prints it) and prints
   the dashboard's address: open it and paste the key into the login box:

   <!-- golden: dashboard manual -->
   ```bash
   printf %s "$FLEETML_ADMIN_KEY" | { pbcopy || clip.exe || xclip -selection clipboard || cat; } 2> /dev/null; echo
   echo "Dashboard: http://localhost:${FLEETML_DASHBOARD_PORT:-80}"
   ```

   You should see `Dashboard: http://localhost:<port>`, and in the browser the
   Overview page (**Dashboard** in the menu); **Devices** lists one device, and **Deployments** one
   `COMPLETE` deployment of iris 1.0.0.

7. **Clean up**: the device, the server and all its data, the virtual
   environment and the saved secrets:

   <!-- golden: python-cleanup cleanup -->
   ```bash
   deactivate; docker rm -f fleetml-agent
   source ~/.fleetml.env && docker compose -f deploy/docker-compose.prod.yml down -v && rm -rf fleetml-agent .venv ~/.fleetml.env
   ```

   You should see `Network <stack>_default  Removed`.

Then [the tour in Python](#the-tour-in-python): canary rollouts that roll
themselves back, against twenty simulated devices.

## Install

```bash
pip install fleetml
```

Python 3.9 or later; the only dependency is `httpx`. The package is typed
(it ships `py.typed`, and its own code passes `mypy --strict`), so an editor
or mypy checks your calls. To work against unreleased changes,
`pip install ./python` from a clone of the repository. On Windows,
`py -m pip install fleetml` in PowerShell, and `$env:FLEETML_SERVER` and
`$env:FLEETML_API_KEY` in place of `export` (the admin key from WSL with the
`wsl` line in [On Windows](QUICKSTART.md#on-windows)); CI runs the client's
unit tests on `windows-latest`, and its live test on Linux.

## Configure

```python
from fleetml import FleetML

fm = FleetML()                                         # FLEETML_SERVER and FLEETML_API_KEY
fm = FleetML("<server URL>", api_key="...")             # or explicitly
```

The server defaults to `FLEETML_SERVER`, then `http://localhost` on the
quickstart's default REST port; the API key (an admin key, as for the CLI)
to `FLEETML_API_KEY`. The server is a URL with its scheme and without the
API path (`http://localhost:<REST port>`, not `localhost:<REST port>` or
`http://localhost:<REST port>/api/v1`: the client adds `/api/v1` itself); without
`http://` or `https://`, `FleetML()` raises `ValueError` naming the value. With the quickstart's server, both come from
`~/.fleetml.env` ([the settings file](ADVANCED.md#the-settings-file-fleetmlenv)),
the saved REST port included:
`source ~/.fleetml.env && export FLEETML_SERVER=http://localhost:${FLEETML_REST_PORT:-18080} FLEETML_API_KEY=$FLEETML_ADMIN_KEY`.
The simulator line the tour prints reads the stack's gRPC port from
`~/.fleetml.env` (`--grpc localhost:${FLEETML_GRPC_PORT:-19090}`), so a moved
port needs no change. `FleetML` is a
context manager (`with FleetML() as fm:`), or call `fm.close()`.

## The tour in Python

It needs a running stack and the client, Get started's steps 2 to 4 (if
step 7 cleaned up, run steps 2 to 4 again: step 2 reuses the clone), and the
`fleetml` CLI for the simulated fleet: step 3 of the
[README's Get started](../README.md#get-started), once.
Save the script below as `tour.py` in the clone's root (it reads the sample
models in `models/iris`) and run it with `python tour.py` in the Get started
terminal, its virtual environment active and `FLEETML_SERVER` and
`FLEETML_API_KEY` set (step 4 sets them). Run it as a file, not with
`python - <<EOF`: it waits for Enter from the terminal.

The [tour](QUICKSTART.md#tour)'s sections 1 to 3 against the quickstart's
server: publish two versions of the Iris classifier, roll 1.0.0 out to a
fleet, then watch a canary of the failing 2.0.0 roll itself back. The project
gets a name of its own (project names are unique on a server, and the CLI
tour's is `tour`). The fleet is the CLI's simulator: the script prints the
line that starts it, with the enrollment key and a seed of its own (devices
with the same seed are the same devices, so another tour's simulator keeps
its own, and the gRPC port from `~/.fleetml.env`); run that line in another
terminal, in the same folder (it starts with `source ~/.fleetml.env` and puts
the CLI on that terminal's `PATH`), then press Enter in the first one:

```python
import time
from fleetml import FleetML

with FleetML() as fm:
    project = fm.projects.create(time.strftime("tour-py-%Y%m%d-%H%M%S"))
    print("project ID:", project.id)  # for Clean up, also if the tour stops early
    v1 = fm.push("models/iris/iris_v1.tflite", project=project.id, model="iris", version="1.0.0")
    v2 = fm.push("models/iris/iris_v2.tflite", project=project.id, model="iris", version="2.0.0")

    key = fm.enrollment_keys.create(project.id, "tour")
    print(f'source ~/.fleetml.env && export PATH="$HOME/.local/bin:$PATH" && FLEETML_ENROLLMENT_KEY={key.key} fleetml simulate'
          ' --grpc localhost:${FLEETML_GRPC_PORT:-19090}'
          f' --devices 20 --label app=tour --seed {project.name} --error-rate 2.0.0=0.4')
    input("Start the simulator with the line above in another terminal, then press Enter here: ")

    d1 = fm.deployments.create(project.id, v1.id, target_selector={"app": "tour"})
    print(fm.deployments.start(d1.id).status)  # COMPLETE: all at once
    time.sleep(5)
    print(len(fm.devices.list(project=project.id, labels={"app": "tour"}, limit=100)), "devices")

    d2 = fm.deployments.create(
        project.id, v2.id, strategy="CANARY", target_selector={"app": "tour"},
        canary_stages=[{"percentage": 25, "min_duration": "10m"}, {"percentage": 100, "min_duration": "0s"}],
        health_gate={"max_error_rate": 0.05, "min_events": 30, "on_failure": "ROLLBACK"},
    )
    fm.deployments.start(d2.id)
    d2 = fm.wait_for_deployment(d2.id, timeout=600)
    print(d2.status, d2.rollback_reason)  # ROLLED_BACK error rate ... exceeds threshold 5.00% ...

    s = fm.telemetry.summary(v1.id)
    print(f"1.0.0: {s.total_events} events, p95 {s.p95_latency_ms:.0f} ms")
```

It prints the project ID first, then the simulator's line and its prompt,
then `COMPLETE`, `20 devices` (the
project's own, whatever else the server runs), the rollback with its
reason, and the summary. **Clean up**:
stop the simulator (Ctrl-C in its shell), then delete the project with
everything in it (its versions, deployments, the 20 devices and their
telemetry), with the ID the tour printed (or the `tour-py-...` project in
`fm.projects.list()`):

```python
from fleetml import FleetML

with FleetML() as fm:
    fm.projects.delete("<project ID>")
```

`python/tests/test_integration.py` runs these steps, with the simulator
started for you, against a live server (`FLEETML_LIVE=1`; see
`scripts/python-e2e.sh`), and every other method once.

## Errors

Every call to the server raises `fleetml.FleetMLError` when the server
refuses it or cannot be reached (a local file that `push` cannot read raises
Python's own `FileNotFoundError` or `OSError`). It carries `status` (the HTTP
status, 0 when the server was not reached), `code` (the server's error code:
`NOT_FOUND` for an unknown ID, also a version's telemetry summary or drift
(`telemetry.events` filters, so an unknown version gives an empty list; so do
the `list` methods for an unknown project or model: they filter by it);
`CONFLICT`, for example a project name that exists;
`VALIDATION_ERROR`, for example a `limit` that is not an integer or is
outside 1 to the page cap, a negative `offset`, or an unknown version status;
`INVALID_TRANSITION` (409), a deployment, experiment or version asked to do
what its status does not allow, such as starting a COMPLETE deployment,
progressing one that is not ROLLING_OUT, concluding a CONCLUDED experiment
or moving an ACTIVE version back to DRAFT; `INTEGRITY_ERROR`, `UNAUTHORIZED`, `NOT_DEPLOYABLE` and so on;
`UNAVAILABLE` or `UPLOAD_FAILED` from the client) and `message` (the
server's message, such as `project not found`):

```python
from fleetml import FleetMLError

try:
    fm.deployments.start(deployment_id)
except FleetMLError as e:
    print(e.status, e.code, e.message)
```

`wait_for_deployment` raises `TimeoutError` when its timeout passes first.

## Methods

IDs are strings. The responses users read are dataclasses (`Project`,
`Model`, `Version`, `EnrollmentKey`, `Deployment`, `DeploymentHealth`,
`Device`, `TelemetryEvent`, `TelemetrySummary`, `Experiment`,
`ExperimentResults`, `DriftResult`) whose `raw` field holds the server's
full JSON object; list methods take `limit=` and `offset=` (the server's
default page is 20 and its largest 100, 1000 for `telemetry.events`; a
larger `limit` is refused with `VALIDATION_ERROR`, so page with `offset=`),
and `devices.list`, `deployments.list` and
`experiments.list` take `project=` (a project ID); without it they list the
whole server. `project`, `limit` and `offset` are keyword-only on every list
method (`*` in the table), so a project ID passed by position is an error
rather than a page size; only `models.list` and `versions.list` take their
parent (a project, a model) by position, and `devices.list` its `labels`.
A filter value nothing matches (an `event_type` the server does not have, a
label no device carries) gives an empty list, not an error, and the REST API
ignores a query parameter it does not know (`?status=` on deployments, for
example), the way most list endpoints do.

| Python | CLI | What it does |
|---|---|---|
| `fm.projects.create(name, description="")` | `fleetml project create --name` | Creates a project |
| `fm.projects.list(*, limit=None, offset=None)` | `fleetml project list` | Lists projects |
| `fm.projects.get(project_id)` | `fleetml project get` | One project |
| `fm.projects.delete(project_id)` | `fleetml project delete` | Deletes the project with its models, versions, deployments, experiments, enrollment keys, devices and their telemetry |
| `fm.models.create(project, name, framework="TFLITE", task="CLASSIFICATION")` | `fleetml model create` | Creates a model |
| `fm.models.list(project, *, limit=None, offset=None)` | `fleetml model list --project` | Lists a project's models |
| `fm.models.get(model_id)` | `fleetml model get` | One model |
| `fm.versions.create(model, version, metadata=None)` | `fleetml ver create --model --version` | Creates a version, with optional metadata |
| `fm.versions.list(model, *, limit=None, offset=None)` | `fleetml ver list --model` | Lists a model's versions |
| `fm.versions.get(version_id)` | (REST) | One version |
| `fm.versions.promote(version_id, status="ACTIVE")` | `fleetml ver promote` | Moves a version to STAGED, ACTIVE (through STAGED from DRAFT) or ARCHIVED, the status in any case; an ACTIVE version asked for ACTIVE raises `INVALID_TRANSITION` (`push(promote=True)` leaves it ACTIVE) |
| `fm.push(path, project, model, version, quantization="INT8", target_runtime="TFLITE", task="CLASSIFICATION", promote=False)` | `fleetml push` | Creates the model and version if needed, uploads the file with its sha256, has the server verify it; returns the version. It finds an existing model and version on any page. With `promote=True` it takes the version to ACTIVE from wherever it is, as `fleetml push --promote` does, and raises when the server refuses (from ARCHIVED). `target_runtime` is the CLI's `--framework` |
| `fm.enrollment_keys.create(project, name)` | (REST) | A key devices register with; `.key` is shown only now |
| `fm.deployments.create(project, version, strategy="ALL_AT_ONCE", target_selector=None, health_gate=None, canary_stages=None, rolling_config=None, urgent=False)` | `fleetml deploy create` | Creates a deployment; `target_selector` is the CLI's `--target`, the rest as in [Rollouts](ROLLOUTS.md) |
| `fm.deployments.start(deployment_id)` | `fleetml deploy start` | Starts it |
| `fm.deployments.get(deployment_id)` | `fleetml deploy get` | One deployment |
| `fm.deployments.list(*, project=None, limit=None, offset=None)` | `fleetml deploy list --project` | Lists deployments, newest first |
| `fm.deployments.rollback(deployment_id, reason="manual rollback")` | `fleetml deploy rollback` | Rolls it back |
| `fm.deployments.pause(deployment_id)`, `.resume(deployment_id)` | `fleetml deploy pause`, `resume` | Holds a rollout where it is, and lets it go on |
| `fm.deployments.cancel(deployment_id)` | (REST) | Stops it; the devices it reached go back to the deployment before it |
| `fm.deployments.progress(deployment_id)` | `fleetml deploy progress` | Moves a canary or rolling deployment to its next stage now, if the stage's minimum time and health gate allow (`health().can_progress`); otherwise returns it unchanged (compare `current_stage`). A deployment that is not ROLLING_OUT raises `INVALID_TRANSITION` |
| `fm.deployments.health(deployment_id)` | `fleetml deploy health` | The current stage's health gate: `healthy`, `hold`, `reason`, `can_progress` (true when `progress` would move it now; `reason` then names the time a stage still holds) |
| `fm.wait_for_deployment(deployment_id, timeout=600, interval=2, statuses=...)` | | Polls until COMPLETE, ROLLED_BACK, CANCELLED or FAILED (or the given statuses) |
| `fm.devices.list(labels=None, *, project=None, limit=None, offset=None)` | `fleetml device list` | Lists devices, those with all the given labels |
| `fm.devices.get(device_id)` | `fleetml device get` | One device |
| `fm.telemetry.summary(version_id)` | `fleetml telemetry summary` | The last day's telemetry of a version: latency and confidence over its INFERENCE events, counts over all |
| `fm.telemetry.events(version_id=None, device_id=None, event_type=None, *, limit=None, offset=None)` | `fleetml telemetry events` | The last day's events, newest first; `confidence_score` is `None` for an event that reported none (a MODEL_LOAD, for example) |
| `fm.experiments.create(project, name, control, treatment, metric="latency", split=50, auto_conclude=False, target_selector=None, min_sample_size=None)` | `fleetml experiment create` | Creates an A/B experiment between two version IDs |
| `fm.experiments.start(experiment_id)` | `fleetml experiment start` | Starts it |
| `fm.experiments.get(experiment_id)`, `.list(*, project=None, limit=None, offset=None)` | `fleetml experiment get`, `list` | One experiment, or the list |
| `fm.experiments.pause(experiment_id)`, `.resume(experiment_id)` | `fleetml experiment pause`, `resume` | Holds it and lets it go on |
| `fm.experiments.conclude(experiment_id, winner, note="")` | `fleetml experiment conclude` | Ends it with the winning version ID |
| `fm.experiments.cancel(experiment_id)` | `fleetml experiment cancel` | Ends it without a winner |
| `fm.experiments.results(experiment_id)` | `fleetml experiment results` | Both arms, the p-value and the recommendation |
| `fm.drift.status(version_id)` | `fleetml drift status` | The latest result per metric |
| `fm.drift.check(version_id)` | `fleetml drift check` | Runs a drift check now, one result per metric; none until the version has a baseline (telemetry from the 7 days before the last day, which the CLI tour's `fleetml simulate ... --history 168h` provides; Go durations, so `8d` is refused) |
| `fm.drift.history(version_id)` | (REST) | The last 50 drift results, newest first |

Statuses are the server's: versions DRAFT, STAGED, ACTIVE, ARCHIVED;
deployments PENDING, ROLLING_OUT, PAUSED, COMPLETE, ROLLED_BACK, CANCELLED,
FAILED;
experiments DRAFT, RUNNING, PAUSED, CONCLUDED, CANCELLED; drift severity NONE,
WARNING, CRITICAL.

## Contact

- Anything about FleetML: [hello@fleetml.dev](mailto:hello@fleetml.dev).
- A security problem: [security@fleetml.dev](mailto:security@fleetml.dev), see [SECURITY.md](../SECURITY.md).
- Questions in public: [GitHub Discussions](https://github.com/fleetml/fleetml/discussions).
- Bugs: [GitHub Issues](https://github.com/fleetml/fleetml/issues/new/choose), with the FleetML version and the command or log line that shows the problem.
