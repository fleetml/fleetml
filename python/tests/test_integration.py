"""The quickstart's first three sections against a live server.

Runs only with FLEETML_LIVE=1, against FLEETML_SERVER with FLEETML_API_KEY (an
admin key) and the server's device gRPC address in FLEETML_GRPC (default
localhost:19090). The simulated fleet is the Go CLI's `fleetml simulate`:
FLEETML_CLI is its path, or it is built from this repository with Go. Use a
throwaway server: the test leaves a project, two versions and 20 simulated
devices behind.
"""

from __future__ import annotations

import os
import shutil
import subprocess
import sys
import threading
import time
import uuid
from pathlib import Path
from typing import Callable, Iterator, List

import pytest

from fleetml import FleetML

pytestmark = pytest.mark.skipif(
    os.environ.get("FLEETML_LIVE") != "1", reason="set FLEETML_LIVE=1 to run against a server"
)

REPO = Path(__file__).resolve().parents[2]
MODELS = REPO / "models" / "iris"


def log(msg: str) -> None:
    print(f"[live] {msg}", file=sys.stderr, flush=True)


@pytest.fixture(scope="module")
def cli(tmp_path_factory: pytest.TempPathFactory) -> str:
    path = os.environ.get("FLEETML_CLI")
    if path:
        return path
    if shutil.which("go") is None:
        pytest.skip("set FLEETML_CLI or install Go to build the CLI for `fleetml simulate`")
    out = tmp_path_factory.mktemp("cli") / ("fleetml.exe" if os.name == "nt" else "fleetml")
    subprocess.run(["go", "build", "-o", str(out), "./cmd/fleetml"], cwd=REPO, check=True)
    return str(out)


class Simulator:
    """`fleetml simulate` in the background, with its output lines."""

    def __init__(self, args: List[str]) -> None:
        self.lines: List[str] = []
        self.proc = subprocess.Popen(args, stdout=subprocess.PIPE, stderr=subprocess.STDOUT, text=True)
        threading.Thread(target=self._read, daemon=True).start()

    def _read(self) -> None:
        assert self.proc.stdout is not None
        for line in self.proc.stdout:
            self.lines.append(line.rstrip())

    def fleet(self) -> str:
        """The last fleet status line, such as "iris 1.0.0 on 20"."""
        status = [line for line in self.lines if line.startswith("simulator: ") and " on " in line]
        return status[-1].removeprefix("simulator: ") if status else ""

    def wait_for(self, fleet: str, timeout: float = 90) -> None:
        wait(lambda: self.fleet() == fleet, timeout, f"simulator to report {fleet!r} (last: {self.fleet()!r})")

    def stop(self) -> None:
        self.proc.terminate()
        try:
            self.proc.wait(10)
        except subprocess.TimeoutExpired:
            self.proc.kill()


def wait(cond: Callable[[], bool], timeout: float, what: str) -> None:
    deadline = time.monotonic() + timeout
    while not cond():
        if time.monotonic() > deadline:
            raise AssertionError(f"timed out waiting for {what}")
        time.sleep(1)


@pytest.fixture
def fm() -> Iterator[FleetML]:
    with FleetML() as client:
        yield client


def test_quickstart_sections_1_to_3(fm: FleetML, cli: str) -> None:
    run = uuid.uuid4().hex[:8]
    labels = {"app": "tour", "run": run}

    # 1. Publish a model: two versions of the Iris classifier.
    project = fm.projects.create(f"python-tour-{run}")
    v1 = fm.push(MODELS / "iris_v1.tflite", project=project.id, model="iris", version="1.0.0")
    v2 = fm.push(MODELS / "iris_v2.tflite", project=project.id, model="iris", version="2.0.0")
    log(f"project {project.name} ({project.id}): pushed iris {v1.version} and {v2.version}")
    assert [v.version for v in sorted(fm.versions.list(v1.model_id), key=lambda v: v.version)] == ["1.0.0", "2.0.0"]

    # 2. A fleet and a rollout: 20 devices; 2.0.0 fails 40% of its inferences.
    key = fm.enrollment_keys.create(project.id, "python-tour")
    grpc = os.environ.get("FLEETML_GRPC", "localhost:19090")
    sim = Simulator([
        cli, "simulate", "--grpc", grpc, "--enrollment-key", key.key, "--devices", "20",
        "--label", "app=tour", "--label", f"run={run}", "--seed", f"python-tour-{run}", "--error-rate", "2.0.0=0.4",
    ])  # fmt: skip
    try:
        d1 = fm.deployments.create(project.id, v1.id, target_selector=labels)
        d1 = fm.deployments.start(d1.id)
        log(f"deployment of 1.0.0: {d1.status}")
        assert d1.status == "COMPLETE"
        sim.wait_for("iris 1.0.0 on 20")
        devices = fm.devices.list(labels=labels, limit=100)
        log(f"simulator: {sim.fleet()}; devices with the labels: {len(devices)}")
        assert len(devices) == 20
        assert {d.os for d in devices} == {"ANDROID"}

        # 3. A canary that rolls itself back.
        d2 = fm.deployments.create(
            project.id,
            v2.id,
            strategy="CANARY",
            target_selector=labels,
            canary_stages=[{"percentage": 25, "min_duration": "10m"}, {"percentage": 100, "min_duration": "0s"}],
            health_gate={"max_error_rate": 0.05, "min_events": 30, "on_failure": "ROLLBACK"},
        )
        d2 = fm.deployments.start(d2.id)
        log(f"canary of 2.0.0: {d2.status}")
        assert d2.status == "ROLLING_OUT"
        d2 = fm.wait_for_deployment(d2.id, timeout=300)
        log(f"canary of 2.0.0: {d2.status}: {d2.rollback_reason}")
        assert d2.status == "ROLLED_BACK"
        assert "error rate" in d2.rollback_reason
        sim.wait_for("iris 1.0.0 on 20")
        log(f"simulator after the rollback: {sim.fleet()}")

        summary = fm.telemetry.summary(v1.id)
        log(
            f"telemetry of 1.0.0: {summary.total_events} events, {summary.error_count} errors, "
            f"p95 {summary.p95_latency_ms:.1f} ms"
        )
        assert summary.total_events > 0
        assert fm.telemetry.summary(v2.id).error_count > 0
    finally:
        sim.stop()
