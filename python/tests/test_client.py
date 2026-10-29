"""Unit tests: every method against httpx.MockTransport, no server."""

from __future__ import annotations

import hashlib
import json
from pathlib import Path
from typing import Any, Callable, Dict, List, Optional, Tuple

import httpx
import pytest

import fleetml
from fleetml import FleetML, FleetMLError

SERVER = "http://fleet.test:18080"
API = SERVER + "/api/v1"
Handler = Callable[[httpx.Request], httpx.Response]


def data(payload: Any, status: int = 200) -> httpx.Response:
    return httpx.Response(status, json={"data": payload})


def error(status: int, code: str, message: str) -> httpx.Response:
    return httpx.Response(status, json={"error": {"code": code, "message": message}})


class Fake:
    """Routes (method, URL path) to handlers and records every request."""

    def __init__(self) -> None:
        self.routes: Dict[Tuple[str, str], Handler] = {}
        self.requests: List[httpx.Request] = []

    def on(self, method: str, path: str, handler: Any) -> None:
        """Answers with ``handler(request)``, or with ``handler`` if it is a response."""
        if isinstance(handler, httpx.Response):
            response = handler

            def handler(_request: httpx.Request) -> httpx.Response:
                return response

        self.routes[(method, path)] = handler

    def __call__(self, request: httpx.Request) -> httpx.Response:
        request.read()
        self.requests.append(request)
        handler = self.routes.get((request.method, request.url.path))
        if handler is None:
            return error(404, "NOT_FOUND", f"no route {request.method} {request.url.path}")
        return handler(request)

    def body(self, i: int = -1) -> Any:
        return json.loads(self.requests[i].content)

    def client(self, api_key: Optional[str] = "k-test") -> FleetML:
        return FleetML(SERVER, api_key=api_key, transport=httpx.MockTransport(self))


@pytest.fixture
def fake() -> Fake:
    return Fake()


PROJECT = {"id": "p1", "name": "demo", "slug": "demo", "description": "", "created_at": "2026-10-15T00:00:00Z"}
MODEL = {"id": "m1", "project_id": "p1", "name": "iris", "framework": "TFLITE", "task_type": "CLASSIFICATION"}
VERSION = {"id": "v1", "model_id": "m1", "version": "1.0.0", "status": "DRAFT", "metadata": {"accuracy": 0.97}}
DEPLOYMENT = {
    "id": "d1",
    "project_id": "p1",
    "model_version_id": "v1",
    "strategy": "CANARY",
    "status": "PENDING",
    "target_selector": {"app": "demo"},
    "current_stage": 0,
    "urgent": False,
}


def test_version_is_the_package_version() -> None:
    assert fleetml.__version__ == "0.10.6"


def test_server_and_key_from_the_environment(monkeypatch: pytest.MonkeyPatch, fake: Fake) -> None:
    monkeypatch.setenv("FLEETML_SERVER", "http://env.test:9000/")
    monkeypatch.setenv("FLEETML_API_KEY", "k-env")
    fake.on("GET", "/api/v1/projects", data([]))
    with FleetML(transport=httpx.MockTransport(fake)) as fm:
        assert fm.server == "http://env.test:9000"
        assert fm.projects.list() == []
    assert str(fake.requests[0].url) == "http://env.test:9000/api/v1/projects"
    assert fake.requests[0].headers["Authorization"] == "Bearer k-env"


def test_server_without_a_scheme_is_refused(monkeypatch: pytest.MonkeyPatch) -> None:
    with pytest.raises(ValueError, match=r"server must start with http:// or https://.*'localhost:18080'"):
        FleetML("localhost:18080", api_key="k")
    monkeypatch.setenv("FLEETML_SERVER", "localhost:18080")
    with pytest.raises(ValueError, match="FLEETML_SERVER must start with http://"):
        FleetML(api_key="k")


def test_defaults_without_environment(monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.delenv("FLEETML_SERVER", raising=False)
    monkeypatch.delenv("FLEETML_API_KEY", raising=False)
    assert FleetML().server == "http://localhost:18080"


def test_no_key_sends_no_authorization(fake: Fake) -> None:
    fake.on("GET", "/api/v1/projects", data([]))
    fake.client(api_key="").projects.list()
    assert "Authorization" not in fake.requests[0].headers


def test_projects(fake: Fake) -> None:
    fake.on("POST", "/api/v1/projects", data(PROJECT, 201))
    fake.on("GET", "/api/v1/projects", data([PROJECT]))
    fake.on("GET", "/api/v1/projects/p1", data(PROJECT))
    fm = fake.client()
    p = fm.projects.create("demo", description="d")
    assert (p.id, p.name, p.slug) == ("p1", "demo", "demo")
    assert fake.body() == {"name": "demo", "description": "d"}
    assert [x.id for x in fm.projects.list(limit=50, offset=10)] == ["p1"]
    assert fake.requests[-1].url.params["limit"] == "50"
    assert fake.requests[-1].url.params["offset"] == "10"
    assert fm.projects.get("p1").raw["created_at"] == "2026-10-15T00:00:00Z"
    assert fake.requests[0].headers["Authorization"] == "Bearer k-test"


def test_models(fake: Fake) -> None:
    fake.on("POST", "/api/v1/projects/p1/models", data(MODEL, 201))
    fake.on("GET", "/api/v1/projects/p1/models", data([MODEL]))
    fm = fake.client()
    m = fm.models.create("p1", "iris", framework="ONNX", task="DETECTION")
    assert m.task_type == "CLASSIFICATION"
    assert fake.body() == {"name": "iris", "framework": "ONNX", "task_type": "DETECTION"}
    assert [x.name for x in fm.models.list("p1")] == ["iris"]


def test_versions(fake: Fake) -> None:
    fake.on("POST", "/api/v1/models/m1/versions", data(VERSION, 201))
    fake.on("GET", "/api/v1/models/m1/versions", data([VERSION]))
    fake.on("GET", "/api/v1/versions/v1", data(VERSION))
    fm = fake.client()
    v = fm.versions.create("m1", "1.0.0", metadata={"accuracy": 0.97})
    assert fake.body() == {"version": "1.0.0", "metadata": {"accuracy": 0.97}}
    assert v.metadata == {"accuracy": 0.97}
    fm.versions.create("m1", "1.0.0")
    assert fake.body() == {"version": "1.0.0"}
    assert [x.version for x in fm.versions.list("m1")] == ["1.0.0"]
    assert fm.versions.get("v1").status == "DRAFT"


def test_enrollment_keys(fake: Fake) -> None:
    fake.on(
        "POST",
        "/api/v1/projects/p1/enrollment-keys",
        data({"id": "e1", "project_id": "p1", "name": "ci", "key": "secret"}, 201),
    )
    k = fake.client().enrollment_keys.create("p1", "ci")
    assert k.key == "secret"
    assert "secret" not in repr(k)
    assert fake.body() == {"name": "ci"}


def test_deployments(fake: Fake) -> None:
    fake.on("POST", "/api/v1/deployments", data(DEPLOYMENT, 201))
    fake.on("POST", "/api/v1/deployments/d1/start", data({**DEPLOYMENT, "status": "ROLLING_OUT"}))
    fake.on("GET", "/api/v1/deployments/d1", data({**DEPLOYMENT, "status": "ROLLING_OUT"}))
    fake.on("GET", "/api/v1/deployments", data([DEPLOYMENT]))
    fake.on(
        "POST",
        "/api/v1/deployments/d1/rollback",
        data({**DEPLOYMENT, "status": "ROLLED_BACK", "rollback_reason": "bad"}),
    )
    fm = fake.client()
    stages = [{"percentage": 25, "min_duration": "10m"}, {"percentage": 100, "min_duration": "0s"}]
    gate = {"max_error_rate": 0.05, "min_events": 30, "on_failure": "ROLLBACK"}
    d = fm.deployments.create(
        "p1",
        "v1",
        strategy="CANARY",
        target_selector={"app": "demo"},
        health_gate=gate,
        canary_stages=stages,
        urgent=True,
    )
    assert d.target_selector == {"app": "demo"}
    assert fake.body() == {
        "project_id": "p1",
        "model_version_id": "v1",
        "strategy": "CANARY",
        "urgent": True,
        "target_selector": {"app": "demo"},
        "health_gate": gate,
        "canary_stages": stages,
    }
    fm.deployments.create("p1", "v1", rolling_config={"batch_size_percent": 20, "batch_interval": "1m"})
    assert fake.body() == {
        "project_id": "p1",
        "model_version_id": "v1",
        "strategy": "ALL_AT_ONCE",
        "urgent": False,
        "rolling_config": {"batch_size_percent": 20, "batch_interval": "1m"},
    }
    assert fm.deployments.start("d1").status == "ROLLING_OUT"
    assert fm.deployments.get("d1").status == "ROLLING_OUT"
    assert [x.id for x in fm.deployments.list()] == ["d1"]
    r = fm.deployments.rollback("d1", reason="bad")
    assert (r.status, r.rollback_reason) == ("ROLLED_BACK", "bad")
    assert fake.body() == {"reason": "bad"}


def test_devices_with_label_filters(fake: Fake) -> None:
    dev = {
        "id": "x",
        "project_id": None,
        "os": "ANDROID",
        "os_version": "14",
        "ram_mb": 6144,
        "chipset": "sim",
        "agent_version": "simulator",
        "labels": {"app": "demo"},
        "last_seen_at": None,
    }
    fake.on("GET", "/api/v1/devices", data([dev]))
    [d] = fake.client().devices.list(labels={"app": "demo", "tier": "beta"}, limit=100)
    assert (d.os, d.ram_mb, d.labels, d.project_id) == ("ANDROID", 6144, {"app": "demo"}, None)
    params = fake.requests[-1].url.params
    assert (params["label.app"], params["label.tier"], params["limit"]) == ("demo", "beta", "100")


def test_telemetry_summary(fake: Fake) -> None:
    fake.on(
        "GET",
        "/api/v1/telemetry/summary/v1",
        data(
            {
                "model_version_id": "v1",
                "total_events": 104,
                "error_count": 2,
                "avg_latency_ms": 19.9,
                "p95_latency_ms": 27.8,
            }
        ),
    )
    s = fake.client().telemetry.summary("v1")
    assert (s.total_events, s.error_count, s.avg_latency_ms, s.p99_latency_ms) == (104, 2, 19.9, 0.0)


def test_experiments(fake: Fake) -> None:
    exp = {
        "id": "x1",
        "project_id": "p1",
        "name": "latency",
        "status": "DRAFT",
        "control_version_id": "v1",
        "treatment_version_id": "v2",
        "primary_metric": "latency",
        "split_ratio": 50,
        "min_sample_size": 300,
    }
    fake.on("POST", "/api/v1/experiments", data(exp, 201))
    fake.on("POST", "/api/v1/experiments/x1/start", data({**exp, "status": "RUNNING"}))
    fake.on(
        "GET",
        "/api/v1/experiments/x1/results",
        data(
            {
                "control": {"version_id": "v1", "sample_size": 385, "avg_latency_ms": 34.0},
                "treatment": {"version_id": "v2", "sample_size": 315, "avg_latency_ms": 14.0},
                "p_value": 0,
                "effect_size": 7.1,
                "is_significant": True,
                "recommendation": "ADOPT_TREATMENT",
                "final": True,
            }
        ),
    )
    fm = fake.client()
    fm.experiments.create(
        "p1", "latency", control="v1", treatment="v2", target_selector={"app": "demo"}, min_sample_size=300
    )
    assert fake.body() == {
        "project_id": "p1",
        "name": "latency",
        "control_version_id": "v1",
        "treatment_version_id": "v2",
        "primary_metric": "latency",
        "split_ratio": 50,
        "auto_conclude": False,
        "target_selector": {"app": "demo"},
        "min_sample_size": 300,
    }
    assert fm.experiments.start("x1").status == "RUNNING"
    r = fm.experiments.results("x1")
    assert (r.control.sample_size, r.treatment.avg_latency_ms, r.p_value, r.final) == (385, 14.0, 0.0, True)
    assert r.recommendation == "ADOPT_TREATMENT"


def test_experiment_results_before_the_test(fake: Fake) -> None:
    fake.on(
        "GET",
        "/api/v1/experiments/x1/results",
        data({"control": {}, "treatment": {}, "p_value": None, "recommendation": "COLLECTING_DATA", "final": False}),
    )
    r = fake.client().experiments.results("x1")
    assert (r.p_value, r.effect_size, r.final) == (None, None, False)


def test_drift(fake: Fake) -> None:
    res = {
        "id": "r1",
        "model_version_id": "v1",
        "metric": "latency",
        "psi_value": 8.18,
        "severity": "CRITICAL",
        "message": "Latency PSI=8.18 (CRITICAL)",
        "checked_at": "2026-10-15T00:00:00Z",
    }
    fake.on("POST", "/api/v1/drift/v1/check", data([res]))
    fake.on("GET", "/api/v1/drift/v1/history", data([res, {**res, "id": "r0", "severity": "NONE"}]))
    fm = fake.client()
    assert [(r.metric, r.severity) for r in fm.drift.check("v1")] == [("latency", "CRITICAL")]
    assert [r.id for r in fm.drift.history("v1")] == ["r1", "r0"]


def push_routes(fake: Fake, model_exists: bool = False, version_exists: bool = False) -> Dict[str, bytes]:
    uploads: Dict[str, bytes] = {}
    if model_exists:
        fake.on("POST", "/api/v1/projects/p1/models", error(409, "CONFLICT", "model iris already exists"))
    else:
        fake.on("POST", "/api/v1/projects/p1/models", data(MODEL, 201))
    fake.on("GET", "/api/v1/projects/p1/models", data([{**MODEL, "id": "m0", "name": "other"}, MODEL]))
    if version_exists:
        fake.on("POST", "/api/v1/models/m1/versions", error(409, "CONFLICT", "version 1.0.0 already exists"))
    else:
        fake.on("POST", "/api/v1/models/m1/versions", data(VERSION, 201))
    fake.on("GET", "/api/v1/models/m1/versions", data([VERSION]))
    fake.on(
        "POST",
        "/api/v1/versions/v1/variants",
        lambda r: data(
            {
                "variant": {"id": "var1", "status": "PENDING_UPLOAD"},
                "upload_url": "http://s3.test:19000/models/var1?X-Amz-Signature=s",
            },
            201,
        ),
    )

    def upload(r: httpx.Request) -> httpx.Response:
        uploads["auth"] = r.headers.get("Authorization", "").encode()
        uploads["length"] = r.headers.get("Content-Length", "").encode()
        uploads["body"] = r.content
        return httpx.Response(200)

    fake.on("PUT", "/models/var1", upload)
    fake.on("POST", "/api/v1/variants/var1/finalize", data({"id": "var1", "status": "READY"}))
    fake.on(
        "PATCH", "/api/v1/versions/v1/status", lambda r: data({**VERSION, "status": json.loads(r.content)["status"]})
    )
    fake.on("GET", "/api/v1/versions/v1", data({**VERSION, "status": "DRAFT"}))
    return uploads


def test_push(fake: Fake, tmp_path: Path) -> None:
    blob = b"TFL3\x00\r\n binary model bytes" * 1000
    model = tmp_path / "my models" / "iris v1.tflite"
    model.parent.mkdir()
    model.write_bytes(blob)
    uploads = push_routes(fake)

    v = fake.client().push(model, project="p1", model="iris", version="1.0.0", quantization="FP16")

    assert v.id == "v1"
    calls = [(r.method, r.url.path) for r in fake.requests]
    assert calls == [
        ("POST", "/api/v1/projects/p1/models"),
        ("POST", "/api/v1/models/m1/versions"),
        ("POST", "/api/v1/versions/v1/variants"),
        ("PUT", "/models/var1"),
        ("POST", "/api/v1/variants/var1/finalize"),
        ("GET", "/api/v1/versions/v1"),
    ]
    assert json.loads(fake.requests[0].content) == {
        "name": "iris",
        "framework": "TFLITE",
        "task_type": "CLASSIFICATION",
    }
    assert json.loads(fake.requests[2].content) == {
        "quantization": "FP16",
        "target_runtime": "TFLITE",
        "blob_size_bytes": len(blob),
        "blob_sha256": hashlib.sha256(blob).hexdigest(),
    }
    assert uploads["body"] == blob
    assert uploads["length"] == str(len(blob)).encode()
    assert uploads["auth"] == b"", "the API key must not be sent to the object store"


def test_push_into_an_existing_model_and_version_and_promote(fake: Fake, tmp_path: Path) -> None:
    model = tmp_path / "m.onnx"
    model.write_bytes(b"onnx")
    push_routes(fake, model_exists=True, version_exists=True)
    fake.client().push(str(model), project="p1", model="iris", version="1.0.0", target_runtime="ONNX", promote=True)
    calls = [(r.method, r.url.path) for r in fake.requests]
    assert ("GET", "/api/v1/projects/p1/models") in calls
    assert ("GET", "/api/v1/models/m1/versions") in calls
    patches = [json.loads(r.content) for r in fake.requests if r.method == "PATCH"]
    assert patches == [{"status": "STAGED"}, {"status": "ACTIVE"}]
    assert json.loads(fake.requests[0].content)["framework"] == "ONNX"


def test_push_finds_an_existing_model_and_version_past_the_first_page(fake: Fake, tmp_path: Path) -> None:
    model = tmp_path / "m.tflite"
    model.write_bytes(b"x")
    push_routes(fake, model_exists=True, version_exists=True)
    others = [{**MODEL, "id": f"m{i}x", "name": f"other{i}"} for i in range(100)]
    fake.on(
        "GET",
        "/api/v1/projects/p1/models",
        lambda r: data(others if r.url.params.get("offset") == "0" else [MODEL]),
    )
    older = [{**VERSION, "id": f"v{i}x", "version": f"0.{i}.0"} for i in range(100)]
    fake.on(
        "GET",
        "/api/v1/models/m1/versions",
        lambda r: data(older if r.url.params.get("offset") == "0" else [VERSION]),
    )
    assert fake.client().push(model, project="p1", model="iris", version="1.0.0").id == "v1"
    pages = [(r.url.path, r.url.params.get("limit"), r.url.params.get("offset")) for r in fake.requests]
    assert ("/api/v1/projects/p1/models", "100", "100") in pages
    assert ("/api/v1/models/m1/versions", "100", "100") in pages


@pytest.mark.parametrize(
    ("status", "patches"), [("DRAFT", ["STAGED", "ACTIVE"]), ("STAGED", ["ACTIVE"]), ("ACTIVE", [])]
)
def test_push_promote_takes_only_the_steps_left(fake: Fake, tmp_path: Path, status: str, patches: List[str]) -> None:
    model = tmp_path / "m.tflite"
    model.write_bytes(b"x")
    push_routes(fake, version_exists=True)
    fake.on("GET", "/api/v1/versions/v1", data({**VERSION, "status": status}))
    fake.client().push(model, project="p1", model="iris", version="1.0.0", promote=True)
    assert [json.loads(r.content)["status"] for r in fake.requests if r.method == "PATCH"] == patches


def test_push_promote_raises_when_the_server_refuses(fake: Fake, tmp_path: Path) -> None:
    model = tmp_path / "m.tflite"
    model.write_bytes(b"x")
    push_routes(fake, version_exists=True)
    fake.on("GET", "/api/v1/versions/v1", data({**VERSION, "status": "ARCHIVED"}))
    fake.on(
        "PATCH",
        "/api/v1/versions/v1/status",
        error(409, "INVALID_TRANSITION", "cannot transition from ARCHIVED to STAGED"),
    )
    with pytest.raises(FleetMLError) as e:
        fake.client().push(model, project="p1", model="iris", version="1.0.0", promote=True)
    assert (e.value.status, e.value.code) == (409, "INVALID_TRANSITION")


def test_push_raises_the_create_error_when_nothing_is_found(fake: Fake, tmp_path: Path) -> None:
    model = tmp_path / "m.tflite"
    model.write_bytes(b"x")
    fake.on("POST", "/api/v1/projects/p1/models", error(403, "FORBIDDEN", "the key's role cannot create models"))
    fake.on("GET", "/api/v1/projects/p1/models", data([]))
    with pytest.raises(FleetMLError) as e:
        fake.client().push(model, project="p1", model="iris", version="1.0.0")
    assert (e.value.status, e.value.code) == (403, "FORBIDDEN")


@pytest.mark.parametrize(
    "kwargs, message",
    [
        ({"quantization": "INT3"}, "quantization must be one of FP32, FP16, INT8, INT4, DYNAMIC"),
        ({"target_runtime": "PYTORCH"}, "target_runtime must be one of TFLITE, ONNX, COREML, GGUF, CUSTOM"),
        ({"version": "v1"}, 'invalid version "v1"'),
        ({"version": "01.0.0"}, 'invalid version "01.0.0"'),
        ({"version": "1.0.0\n"}, 'invalid version "1.0.0\n"'),
    ],
)
def test_push_refused_inputs_create_nothing(fake: Fake, tmp_path: Path, kwargs: Dict[str, str], message: str) -> None:
    model = tmp_path / "m.tflite"
    model.write_bytes(b"x")
    args: Dict[str, str] = {"version": "1.0.0", **kwargs}
    with pytest.raises(FleetMLError) as e:
        fake.client().push(model, project="p1", model="iris", **args)  # type: ignore[arg-type]
    assert (e.value.status, e.value.code) == (400, "VALIDATION_ERROR")
    assert message in e.value.message
    assert fake.requests == [], "nothing is created"


def test_push_accepts_any_case_and_a_prerelease(fake: Fake, tmp_path: Path) -> None:
    model = tmp_path / "m.tflite"
    model.write_bytes(b"x")
    push_routes(fake)
    fake.client().push(
        model, project="p1", model="iris", version="1.0.0-rc.1", quantization="fp16", target_runtime="tflite"
    )
    assert [r.url.path for r in fake.requests][:2] == ["/api/v1/projects/p1/models", "/api/v1/models/m1/versions"]


@pytest.mark.parametrize(
    ("name", "kwargs", "runtime"),
    [
        ("m.onnx", {}, "ONNX"),
        ("Iris.MLModel", {}, "COREML"),
        ("llm.gguf", {}, "GGUF"),
        ("m.bin", {}, "TFLITE"),
        ("m.onnx", {"target_runtime": "TFLITE"}, "TFLITE"),
    ],
)
def test_push_runtime_from_the_extension(
    fake: Fake, tmp_path: Path, name: str, kwargs: Dict[str, str], runtime: str
) -> None:
    model = tmp_path / name
    model.write_bytes(b"x")
    push_routes(fake)
    fake.client().push(model, project="p1", model="iris", version="1.0.0", **kwargs)  # type: ignore[arg-type]
    assert json.loads(fake.requests[0].content)["framework"] == runtime
    assert json.loads(fake.requests[2].content)["target_runtime"] == runtime


def test_push_drops_a_leading_v(fake: Fake, tmp_path: Path) -> None:
    model = tmp_path / "m.tflite"
    model.write_bytes(b"x")
    push_routes(fake, model_exists=True, version_exists=True)
    fake.client().push(model, project="p1", model="iris", version="v1.0.0")
    assert json.loads(fake.requests[2].content) == {"version": "1.0.0"}
    calls = [(r.method, r.url.path) for r in fake.requests]
    assert ("POST", "/api/v1/versions/v1/variants") in calls, "the existing 1.0.0 is found"


@pytest.mark.parametrize(
    ("model_exists", "version_exists", "removed"),
    [
        (False, False, ("/api/v1/models/m1", "model iris")),
        (True, False, ("/api/v1/versions/v1", "version 1.0.0")),
        (True, True, None),
    ],
)
def test_push_failed_upload_removes_what_it_created(
    fake: Fake, tmp_path: Path, model_exists: bool, version_exists: bool, removed: Optional[Tuple[str, str]]
) -> None:
    model = tmp_path / "m.tflite"
    model.write_bytes(b"x")
    push_routes(fake, model_exists=model_exists, version_exists=version_exists)
    fake.on("PUT", "/models/var1", httpx.Response(503))
    fake.on("DELETE", "/api/v1/models/m1", httpx.Response(204))
    fake.on("DELETE", "/api/v1/versions/v1", httpx.Response(204))
    with pytest.raises(FleetMLError) as e:
        fake.client().push(model, project="p1", model="iris", version="1.0.0")
    assert (e.value.status, e.value.code) == (503, "UPLOAD_FAILED")
    deletes = [r.url.path for r in fake.requests if r.method == "DELETE"]
    if removed is None:
        assert deletes == [], "an existing model and version stay"
        assert "removed" not in e.value.message
    else:
        assert deletes == [removed[0]]
        assert f"the {removed[1]} this push created was removed" in e.value.message


def test_push_upload_and_integrity_failures(fake: Fake, tmp_path: Path) -> None:
    model = tmp_path / "m.tflite"
    model.write_bytes(b"x")
    push_routes(fake)
    fake.on("PUT", "/models/var1", httpx.Response(403, text="<Error>SignatureDoesNotMatch</Error>"))
    with pytest.raises(FleetMLError) as e:
        fake.client().push(model, project="p1", model="iris", version="1.0.0")
    assert (e.value.status, e.value.code) == (403, "UPLOAD_FAILED")

    fake.on("PUT", "/models/var1", httpx.Response(200))
    fake.on(
        "POST", "/api/v1/variants/var1/finalize", error(422, "INTEGRITY_ERROR", "uploaded blob sha256 does not match")
    )
    with pytest.raises(FleetMLError) as e:
        fake.client().push(model, project="p1", model="iris", version="1.0.0")
    assert e.value.code == "INTEGRITY_ERROR"


def test_push_missing_file(fake: Fake, tmp_path: Path) -> None:
    with pytest.raises(FileNotFoundError):
        fake.client().push(tmp_path / "none.tflite", project="p1", model="iris", version="1.0.0")
    assert fake.requests == []


@pytest.mark.parametrize(
    ("response", "status", "code", "text"),
    [
        (error(404, "NOT_FOUND", "project not found"), 404, "NOT_FOUND", "404 NOT_FOUND: project not found"),
        (error(409, "CONFLICT", "already exists"), 409, "CONFLICT", "409 CONFLICT: already exists"),
        (error(401, "UNAUTHORIZED", "missing API key"), 401, "UNAUTHORIZED", "FLEETML_API_KEY"),
        (httpx.Response(502, text="Bad Gateway"), 502, "HTTP_ERROR", "502 HTTP_ERROR: Bad Gateway"),
        (
            httpx.Response(
                500,
                json={
                    "error": {"code": "INTERNAL_ERROR", "message": "An internal error occurred", "request_id": "r-1"}
                },
            ),
            500,
            "INTERNAL_ERROR",
            "internal error",
        ),
        (httpx.Response(200, text="not json"), 200, "INVALID_RESPONSE", "not json"),
    ],
)
def test_error_mapping(fake: Fake, response: httpx.Response, status: int, code: str, text: str) -> None:
    fake.on("GET", "/api/v1/projects/p1", response)
    with pytest.raises(FleetMLError) as e:
        fake.client().projects.get("p1")
    assert (e.value.status, e.value.code) == (status, code)
    assert text in str(e.value)


def test_unreachable_server() -> None:
    def refuse(request: httpx.Request) -> httpx.Response:
        raise httpx.ConnectError("connection refused", request=request)

    with pytest.raises(FleetMLError) as e:
        FleetML(SERVER, api_key="k", transport=httpx.MockTransport(refuse)).projects.list()
    assert (e.value.status, e.value.code) == (0, "UNAVAILABLE")
    assert "server unavailable" in e.value.message


def test_wait_for_deployment(fake: Fake, monkeypatch: pytest.MonkeyPatch) -> None:
    statuses = iter(["PENDING", "ROLLING_OUT", "ROLLING_OUT", "ROLLED_BACK"])
    fake.on("GET", "/api/v1/deployments/d1", lambda _r: data({**DEPLOYMENT, "status": next(statuses)}))
    monkeypatch.setattr("time.sleep", lambda _s: None)
    d = fake.client().wait_for_deployment("d1", timeout=60)
    assert d.status == "ROLLED_BACK"
    assert len(fake.requests) == 4


def test_wait_for_deployment_until_given_statuses(fake: Fake, monkeypatch: pytest.MonkeyPatch) -> None:
    fake.on("GET", "/api/v1/deployments/d1", data({**DEPLOYMENT, "status": "ROLLING_OUT"}))
    monkeypatch.setattr("time.sleep", lambda _s: None)
    assert fake.client().wait_for_deployment("d1", statuses={"ROLLING_OUT"}).status == "ROLLING_OUT"


def test_wait_for_deployment_times_out(fake: Fake) -> None:
    fake.on("GET", "/api/v1/deployments/d1", data({**DEPLOYMENT, "status": "ROLLING_OUT"}))
    with pytest.raises(TimeoutError, match="still ROLLING_OUT"):
        fake.client().wait_for_deployment("d1", timeout=0.05, interval=0.01)


def test_license_matches_the_repository() -> None:
    here = Path(__file__).resolve().parents[1]
    repo = here.parent
    for name in ("LICENSE", "NOTICE"):
        if (repo / name).exists():
            assert (here / name).read_bytes() == (repo / name).read_bytes(), f"python/{name} differs from {name}"


# The lifecycle methods added for the fourth developer review: each is one
# REST call, as the CLI makes it.


def test_project_delete_and_model_get(fake: Fake) -> None:
    fake.on("DELETE", "/api/v1/projects/p1", httpx.Response(204))
    fake.on("GET", "/api/v1/models/m1", data(MODEL))
    fm = fake.client()
    fm.projects.delete("p1")
    assert (fake.requests[-1].method, fake.requests[-1].url.path) == ("DELETE", "/api/v1/projects/p1")
    assert fm.models.get("m1").name == "iris"


def test_version_promote_walks_the_lifecycle(fake: Fake) -> None:
    statuses: List[str] = []

    def patch(request: httpx.Request) -> httpx.Response:
        statuses.append(json.loads(request.content)["status"])
        return data({**VERSION, "status": statuses[-1]})

    fake.on("GET", "/api/v1/versions/v1", data(VERSION))  # DRAFT
    fake.on("PATCH", "/api/v1/versions/v1/status", patch)
    fm = fake.client()
    assert fm.versions.promote("v1").status == "ACTIVE"
    assert statuses == ["STAGED", "ACTIVE"]
    statuses.clear()
    assert fm.versions.promote("v1", status="ARCHIVED").status == "ARCHIVED"
    assert statuses == ["ARCHIVED"]
    statuses.clear()
    assert fm.versions.promote("v1", status="active").status == "ACTIVE", "any case, as the server takes it"
    assert statuses == ["STAGED", "ACTIVE"]


def test_deployment_lifecycle(fake: Fake) -> None:
    for action, status in (
        ("pause", "PAUSED"),
        ("resume", "ROLLING_OUT"),
        ("cancel", "CANCELLED"),
        ("progress", "ROLLING_OUT"),
    ):
        fake.on("POST", f"/api/v1/deployments/d1/{action}", data({**DEPLOYMENT, "status": status}))
    fake.on("GET", "/api/v1/deployments/d1/health", data({
        "deployment_id": "d1", "status": "ROLLING_OUT", "current_stage": 1, "healthy": False, "hold": True,
        "reason": "error rate 40.00% exceeds threshold 5.00%", "can_progress": False, "cohort_devices": 5,
    }))  # fmt: skip
    fake.on("GET", "/api/v1/deployments", data([DEPLOYMENT]))
    fm = fake.client()
    assert fm.deployments.pause("d1").status == "PAUSED"
    assert fm.deployments.resume("d1").status == "ROLLING_OUT"
    assert fm.deployments.cancel("d1").status == "CANCELLED"
    assert fm.deployments.progress("d1").status == "ROLLING_OUT"
    h = fm.deployments.health("d1")
    assert (h.healthy, h.hold, h.can_progress, h.cohort_devices, h.current_stage) == (False, True, False, 5, 1)
    assert "error rate" in h.reason
    fm.deployments.list(project="p1", limit=5)
    assert dict(fake.requests[-1].url.params) == {"limit": "5", "project_id": "p1"}


def test_devices_by_project_and_get(fake: Fake) -> None:
    device = {"id": "dev1", "project_id": "p1", "os": "LINUX", "labels": {"app": "demo"}}
    fake.on("GET", "/api/v1/devices", data([device]))
    fake.on("GET", "/api/v1/devices/dev1", data(device))
    fm = fake.client()
    assert [d.id for d in fm.devices.list(project="p1", labels={"app": "demo"})] == ["dev1"]
    assert dict(fake.requests[-1].url.params) == {"project_id": "p1", "label.app": "demo"}
    assert fm.devices.get("dev1").project_id == "p1"


def test_devices_list_with_a_project_id_by_position(fake: Fake) -> None:
    fm = fake.client()
    with pytest.raises(TypeError, match="project=<id>"):
        fm.devices.list("p1")  # type: ignore[arg-type]
    assert fake.requests == [], "nothing is sent"


def test_telemetry_events_without_a_confidence(fake: Fake) -> None:
    # The server stores no confidence as 0, as for a MODEL_LOAD; the CLI shows it blank.
    load = {"device_id": "dev1", "model_version_id": "v1", "event_type": "MODEL_LOAD", "latency_ms": 0,
            "confidence_score": 0, "recorded_at": "2026-10-24T00:00:00Z"}  # fmt: skip
    fake.on("GET", "/api/v1/telemetry/events", data([load, {**load, "confidence_score": None}]))
    assert [e.confidence_score for e in fake.client().telemetry.events(version_id="v1")] == [None, None]


def test_telemetry_events(fake: Fake) -> None:
    event = {"device_id": "dev1", "model_version_id": "v1", "event_type": "INFERENCE", "latency_ms": 12,
             "confidence_score": 0.9700000286102295, "prediction_label": "virginica", "error_message": "",
             "recorded_at": "2026-10-24T00:00:00Z"}  # fmt: skip
    fake.on("GET", "/api/v1/telemetry/events", data([event]))
    fm = fake.client()
    [e] = fm.telemetry.events(version_id="v1", device_id="dev1", event_type="INFERENCE", limit=10)
    assert (e.event_type, e.latency_ms, e.prediction_label) == ("INFERENCE", 12, "virginica")
    assert e.confidence_score is not None and round(e.confidence_score, 3) == 0.97
    assert dict(fake.requests[-1].url.params) == {
        "limit": "10", "model_version_id": "v1", "device_id": "dev1", "event_type": "INFERENCE",
    }  # fmt: skip


def test_experiment_lifecycle(fake: Fake) -> None:
    exp = {"id": "e1", "project_id": "p1", "name": "ab", "status": "RUNNING", "control_version_id": "v1",
           "treatment_version_id": "v2"}  # fmt: skip
    fake.on("GET", "/api/v1/experiments/e1", data(exp))
    fake.on("GET", "/api/v1/experiments", data([exp]))
    for action, status in (
        ("pause", "PAUSED"),
        ("resume", "RUNNING"),
        ("conclude", "CONCLUDED"),
        ("cancel", "CANCELLED"),
    ):
        fake.on("POST", f"/api/v1/experiments/e1/{action}", data({**exp, "status": status}))
    fm = fake.client()
    assert fm.experiments.get("e1").name == "ab"
    assert [e.id for e in fm.experiments.list(project="p1")] == ["e1"]
    assert dict(fake.requests[-1].url.params) == {"project_id": "p1"}
    assert fm.experiments.pause("e1").status == "PAUSED"
    assert fm.experiments.resume("e1").status == "RUNNING"
    assert fm.experiments.conclude("e1", "v2", note="faster").status == "CONCLUDED"
    assert fake.body() == {"winner_version_id": "v2", "note": "faster"}
    assert fm.experiments.cancel("e1").status == "CANCELLED"


def test_drift_status(fake: Fake) -> None:
    result = {"id": "r1", "model_version_id": "v1", "metric": "latency", "psi_value": 0.3, "severity": "CRITICAL"}
    fake.on("GET", "/api/v1/drift/v1", data([result]))
    assert [(r.metric, r.severity) for r in fake.client().drift.status("v1")] == [("latency", "CRITICAL")]


def test_list_arguments_are_keyword_only() -> None:
    """A project ID passed positionally used to land in ``limit`` and list the
    whole server; ``project``, ``limit`` and ``offset`` are keyword-only."""
    fm = FleetML(SERVER, api_key="k", transport=httpx.MockTransport(lambda r: httpx.Response(500)))
    calls: List[Callable[[], Any]] = [
        lambda: fm.deployments.list("p1"),  # type: ignore[call-arg, misc, unused-ignore]
        lambda: fm.experiments.list("p1"),  # type: ignore[call-arg, misc, unused-ignore]
        lambda: fm.devices.list(None, "p1"),  # type: ignore[call-arg, misc, unused-ignore]
        lambda: fm.projects.list(10),  # type: ignore[call-arg, misc, unused-ignore]
        lambda: fm.models.list("p1", 10),  # type: ignore[call-arg, misc, unused-ignore]
        lambda: fm.versions.list("m1", 10),  # type: ignore[call-arg, misc, unused-ignore]
        lambda: fm.telemetry.events("v1", None, None, 10),  # type: ignore[call-arg, misc, unused-ignore]
    ]
    for call in calls:
        with pytest.raises(TypeError, match="positional argument"):
            call()
