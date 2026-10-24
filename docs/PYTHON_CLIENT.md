# Python client

`fleetml` on PyPI is a Python library for the FleetML server's REST API: a
training pipeline, a notebook or a CI job pushes a model, starts a rollout,
waits for it and reads what the fleet reports, on any operating system. It is
a client for the REST API only and has no device-side role (devices use the
[Android](ANDROID_SDK.md) and [iOS](IOS_SDK.md) SDKs or the
[Linux agent](LINUX_AGENT.md)). It uses the same words as the `fleetml` CLI,
which stays the command-line tool; the package installs no command.

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
fm = FleetML("http://localhost:18080", api_key="...")  # or explicitly
```

The server defaults to `FLEETML_SERVER`, then `http://localhost:18080`; the
API key (an admin key, as for the CLI) to `FLEETML_API_KEY`. With the
quickstart's server, the admin key is `FLEETML_ADMIN_KEY` in
`~/.fleetml.env` ([Run FleetML](QUICKSTART.md#run-fleetml)):
`source ~/.fleetml.env && export FLEETML_API_KEY=$FLEETML_ADMIN_KEY`. `FleetML` is a
context manager (`with FleetML() as fm:`), or call `fm.close()`.

## The tour in Python

Run it from the root of a clone of the repository (it reads the sample models
in `models/iris`), with the `fleetml` CLI installed for the simulated fleet
([Get the CLI](QUICKSTART.md#get-the-cli)).

The [tour](QUICKSTART.md#tour)'s sections 1 to 3 against the quickstart's
server: publish two versions of the Iris classifier, roll 1.0.0 out to a
fleet, then watch a canary of the failing 2.0.0 roll itself back. The project
gets a name of its own (project names are unique on a server, and the CLI
tour's is `tour`). The fleet is the CLI's simulator: the script prints the
line that starts it, with the enrollment key and a seed of its own (devices
with the same seed are the same devices, so another tour's simulator keeps
its own); run that line in another shell from the same folder, then press
Enter:

```python
import time
from fleetml import FleetML

with FleetML() as fm:
    project = fm.projects.create(time.strftime("tour-py-%Y%m%d-%H%M%S"))
    v1 = fm.push("models/iris/iris_v1.tflite", project=project.id, model="iris", version="1.0.0")
    v2 = fm.push("models/iris/iris_v2.tflite", project=project.id, model="iris", version="2.0.0")

    key = fm.enrollment_keys.create(project.id, "tour")
    print(f"fleetml simulate --enrollment-key {key.key} --devices 20 --label app=tour --seed {project.name} --error-rate 2.0.0=0.4")
    input("Start the simulator with the line above in another shell, then press Enter")

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
    print("project ID:", project.id)
```

It prints `COMPLETE`, `20 devices` (the project's own, whatever else the
server runs), the rollback with its reason, and the summary. **Clean up**:
stop the simulator (Ctrl-C in its shell), then delete the project with
everything in it (its versions, deployments, the 20 devices and their
telemetry):

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
`NOT_FOUND` for an unknown ID, also a version's telemetry or drift;
`CONFLICT`, for example a project name that exists;
`VALIDATION_ERROR`; `INVALID_TRANSITION`, a deployment or experiment asked
to do what its status does not allow, such as starting a COMPLETE
deployment; `INTEGRITY_ERROR`, `UNAUTHORIZED`, `NOT_DEPLOYABLE` and so on;
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
full JSON object; list methods take `limit` and `offset` (the server's
default page is 20), and `devices.list`, `deployments.list` and
`experiments.list` take `project=` (a project ID); without it they list the
whole server.

| Python | CLI | What it does |
|---|---|---|
| `fm.projects.create(name, description="")` | `fleetml project create --name` | Creates a project |
| `fm.projects.list()` | `fleetml project list` | Lists projects |
| `fm.projects.get(project_id)` | `fleetml project get` | One project |
| `fm.projects.delete(project_id)` | `fleetml project delete` | Deletes the project with its models, versions, deployments, experiments, enrollment keys, devices and their telemetry |
| `fm.models.create(project, name, framework="TFLITE", task="CLASSIFICATION")` | `fleetml model create` | Creates a model |
| `fm.models.list(project)` | `fleetml model list --project` | Lists a project's models |
| `fm.models.get(model_id)` | `fleetml model get` | One model |
| `fm.versions.create(model, version, metadata=None)` | `fleetml ver create --model --version` | Creates a version, with optional metadata |
| `fm.versions.list(model)` | `fleetml ver list --model` | Lists a model's versions |
| `fm.versions.get(version_id)` | (REST) | One version |
| `fm.versions.promote(version_id, status="ACTIVE")` | `fleetml ver promote` | Moves a version to STAGED, ACTIVE (through STAGED from DRAFT) or ARCHIVED |
| `fm.push(path, project, model, version, quantization="INT8", target_runtime="TFLITE", task="CLASSIFICATION", promote=False)` | `fleetml push` | Creates the model and version if needed, uploads the file with its sha256, has the server verify it; returns the version. `target_runtime` is the CLI's `--framework` |
| `fm.enrollment_keys.create(project, name)` | (REST) | A key devices register with; `.key` is shown only now |
| `fm.deployments.create(project, version, strategy="ALL_AT_ONCE", target_selector=None, health_gate=None, canary_stages=None, rolling_config=None, urgent=False)` | `fleetml deploy create` | Creates a deployment; `target_selector` is the CLI's `--target`, the rest as in [Rollouts](ROLLOUTS.md) |
| `fm.deployments.start(deployment_id)` | `fleetml deploy start` | Starts it |
| `fm.deployments.get(deployment_id)` | `fleetml deploy get` | One deployment |
| `fm.deployments.list(project=None)` | `fleetml deploy list --project` | Lists deployments, newest first |
| `fm.deployments.rollback(deployment_id, reason="manual rollback")` | `fleetml deploy rollback` | Rolls it back |
| `fm.deployments.pause(deployment_id)`, `.resume(deployment_id)` | `fleetml deploy pause`, `resume` | Holds a rollout where it is, and lets it go on |
| `fm.deployments.cancel(deployment_id)` | (REST) | Stops it; the devices it reached go back to the deployment before it |
| `fm.deployments.progress(deployment_id)` | `fleetml deploy progress` | Moves a canary or rolling deployment to its next stage now, if the stage's minimum time and health gate allow |
| `fm.deployments.health(deployment_id)` | `fleetml deploy health` | The current stage's health gate: `healthy`, `hold`, `reason`, `can_progress` |
| `fm.wait_for_deployment(deployment_id, timeout=600, interval=2, statuses=...)` | | Polls until COMPLETE, ROLLED_BACK, CANCELLED or FAILED (or the given statuses) |
| `fm.devices.list(labels=None, project=None)` | `fleetml device list` | Lists devices, those with all the given labels |
| `fm.devices.get(device_id)` | `fleetml device get` | One device |
| `fm.telemetry.summary(version_id)` | `fleetml telemetry summary` | The last day's telemetry of a version: latency and confidence over its INFERENCE events, counts over all |
| `fm.telemetry.events(version_id=None, device_id=None, event_type=None)` | `fleetml telemetry events` | The last day's events, newest first |
| `fm.experiments.create(project, name, control, treatment, metric="latency", split=50, auto_conclude=False, target_selector=None, min_sample_size=None)` | `fleetml experiment create` | Creates an A/B experiment between two version IDs |
| `fm.experiments.start(experiment_id)` | `fleetml experiment start` | Starts it |
| `fm.experiments.get(experiment_id)`, `.list(project=None)` | `fleetml experiment get`, `list` | One experiment, or the list |
| `fm.experiments.pause(experiment_id)`, `.resume(experiment_id)` | `fleetml experiment pause`, `resume` | Holds it and lets it go on |
| `fm.experiments.conclude(experiment_id, winner, note="")` | `fleetml experiment conclude` | Ends it with the winning version ID |
| `fm.experiments.cancel(experiment_id)` | `fleetml experiment cancel` | Ends it without a winner |
| `fm.experiments.results(experiment_id)` | `fleetml experiment results` | Both arms, the p-value and the recommendation |
| `fm.drift.status(version_id)` | `fleetml drift status` | The latest result per metric |
| `fm.drift.check(version_id)` | `fleetml drift check` | Runs a drift check now, one result per metric; none until the version has a baseline (telemetry from the 7 days before the last day, which the CLI tour's `--history` provides) |
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
