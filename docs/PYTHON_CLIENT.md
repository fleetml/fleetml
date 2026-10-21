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

`pip install fleetml` works from 0.6.0; until then, `pip install ./python`
from a clone of the repository (also the way to work against unreleased
changes). Python 3.9 or later; the only dependency is `httpx`.

## Configure

```python
from fleetml import FleetML

fm = FleetML()                                         # FLEETML_SERVER and FLEETML_API_KEY
fm = FleetML("http://localhost:18080", api_key="...")  # or explicitly
```

The server defaults to `FLEETML_SERVER`, then `http://localhost:18080`; the
API key (an admin key, as for the CLI) to `FLEETML_API_KEY`. `FleetML` is a
context manager (`with FleetML() as fm:`), or call `fm.close()`.

## The tour in Python

Run it from the root of a clone of the repository (it reads the sample models
in `models/iris`), with the `fleetml` CLI installed for the simulated fleet
([Get the CLI](QUICKSTART.md#get-the-cli)).

The [tour](QUICKSTART.md#tour)'s sections 1 to 3 against the quickstart's
server: publish two versions of the Iris classifier, roll 1.0.0 out to a
fleet, then watch a canary of the failing 2.0.0 roll itself back. The fleet is
the CLI's simulator; start it in another shell with the enrollment key the
script prints (`fleetml simulate --enrollment-key <key> --devices 20 --label
app=tour --error-rate 2.0.0=0.4`):

```python
from fleetml import FleetML

with FleetML() as fm:
    project = fm.projects.create("tour")
    v1 = fm.push("models/iris/iris_v1.tflite", project=project.id, model="iris", version="1.0.0")
    v2 = fm.push("models/iris/iris_v2.tflite", project=project.id, model="iris", version="2.0.0")

    key = fm.enrollment_keys.create(project.id, "tour")
    input(f"Start the simulator with --enrollment-key {key.key}, then press Enter")

    d1 = fm.deployments.create(project.id, v1.id, target_selector={"app": "tour"})
    print(fm.deployments.start(d1.id).status)  # COMPLETE: all at once
    print(len(fm.devices.list(labels={"app": "tour"}, limit=100)), "devices")

    d2 = fm.deployments.create(
        project.id, v2.id, strategy="CANARY", target_selector={"app": "tour"},
        canary_stages=[{"percentage": 25, "min_duration": "10m"}, {"percentage": 100, "min_duration": "0s"}],
        health_gate={"max_error_rate": 0.05, "min_events": 30, "on_failure": "ROLLBACK"},
    )
    fm.deployments.start(d2.id)
    d2 = fm.wait_for_deployment(d2.id, timeout=600)
    print(d2.status, d2.rollback_reason)  # ROLLED_BACK: error rate ... exceeds threshold 5.00% ...

    s = fm.telemetry.summary(v1.id)
    print(f"1.0.0: {s.total_events} inferences, p95 {s.p95_latency_ms:.0f} ms")
```

`python/tests/test_integration.py` runs these steps, with the simulator
started for you, against a live server (`FLEETML_LIVE=1`; see
`scripts/python-e2e.sh`).

## Errors

Every call raises `fleetml.FleetMLError` when the server refuses it or cannot
be reached. It carries `status` (the HTTP status, 0 when the server was not
reached), `code` (the server's error code: `NOT_FOUND`, `CONFLICT`,
`VALIDATION_ERROR`, `INTEGRITY_ERROR`, `UNAUTHORIZED`, `NOT_DEPLOYABLE` and so
on; `UNAVAILABLE` or `UPLOAD_FAILED` from the client) and `message` (the
server's message):

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
`Model`, `Version`, `EnrollmentKey`, `Deployment`, `Device`,
`TelemetrySummary`, `Experiment`, `ExperimentResults`, `DriftResult`) whose
`raw` field holds the server's full JSON object; list methods take `limit`
and `offset` (the server's default page is 20).

| Python | CLI | What it does |
|---|---|---|
| `fm.projects.create(name, description="")` | `fleetml project create --name` | Creates a project |
| `fm.projects.list()` | `fleetml project list` | Lists projects |
| `fm.projects.get(project_id)` | `fleetml project get` | One project |
| `fm.models.create(project, name, framework="TFLITE", task="CLASSIFICATION")` | `fleetml model create` | Creates a model |
| `fm.models.list(project)` | `fleetml model list --project` | Lists a project's models |
| `fm.versions.create(model, version, metadata=None)` | `fleetml ver create --model --version` | Creates a version, with optional metadata |
| `fm.versions.list(model)` | `fleetml ver list --model` | Lists a model's versions |
| `fm.versions.get(version_id)` | (REST) | One version |
| `fm.push(path, project, model, version, quantization="INT8", target_runtime="TFLITE", task="CLASSIFICATION", promote=False)` | `fleetml push` | Creates the model and version if needed, uploads the file with its sha256, has the server verify it; returns the version. `target_runtime` is the CLI's `--framework` |
| `fm.enrollment_keys.create(project, name)` | (REST) | A key devices register with; `.key` is shown only now |
| `fm.deployments.create(project, version, strategy="ALL_AT_ONCE", target_selector=None, health_gate=None, canary_stages=None, rolling_config=None, urgent=False)` | `fleetml deploy create` | Creates a deployment; `target_selector` is the CLI's `--target`, the rest as in [Rollouts](ROLLOUTS.md) |
| `fm.deployments.start(deployment_id)` | `fleetml deploy start` | Starts it |
| `fm.deployments.get(deployment_id)` | `fleetml deploy get` | One deployment |
| `fm.deployments.list()` | `fleetml deploy list` | Lists deployments |
| `fm.deployments.rollback(deployment_id, reason="manual rollback")` | `fleetml deploy rollback` | Rolls it back |
| `fm.wait_for_deployment(deployment_id, timeout=600, interval=2, statuses=...)` | | Polls until COMPLETE, ROLLED_BACK, CANCELLED or FAILED (or the given statuses) |
| `fm.devices.list(labels=None)` | `fleetml device list` | Lists devices, those with all the given labels |
| `fm.telemetry.summary(version_id)` | `fleetml telemetry summary` | The last day's inference telemetry of a version |
| `fm.experiments.create(project, name, control, treatment, metric="latency", split=50, auto_conclude=False, target_selector=None, min_sample_size=None)` | `fleetml experiment create` | Creates an A/B experiment between two version IDs |
| `fm.experiments.start(experiment_id)` | `fleetml experiment start` | Starts it |
| `fm.experiments.results(experiment_id)` | `fleetml experiment results` | Both arms, the p-value and the recommendation |
| `fm.drift.check(version_id)` | `fleetml drift check` | Runs a drift check now, one result per metric |
| `fm.drift.history(version_id)` | (REST) | The last 50 drift results, newest first |

Statuses are the server's: versions DRAFT, STAGED, ACTIVE, ARCHIVED;
deployments PENDING, ROLLING_OUT, PAUSED, COMPLETE, ROLLED_BACK, CANCELLED;
experiments DRAFT, RUNNING, PAUSED, CONCLUDED, CANCELLED; drift severity NONE,
WARNING, CRITICAL.
