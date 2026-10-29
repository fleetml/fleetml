"""The FleetML client."""

from __future__ import annotations

import hashlib
import os
import re
import time
from typing import Any, Callable, Collection, Dict, List, Mapping, Optional, Sequence, TypeVar, Union

import httpx

from fleetml.errors import FleetMLError
from fleetml.types import (
    Deployment,
    DeploymentHealth,
    Device,
    DriftResult,
    EnrollmentKey,
    Experiment,
    ExperimentResults,
    Model,
    Project,
    TelemetryEvent,
    TelemetrySummary,
    Version,
    from_list,
)

T = TypeVar("T")

# The status changes push(promote=True) makes from each status to reach
# ACTIVE, one step at a time as the server requires; an ACTIVE version keeps
# its status, and from ARCHIVED the server refuses the first step.
_PROMOTE_STEPS: Dict[str, Sequence[str]] = {"DRAFT": ("STAGED", "ACTIVE"), "STAGED": ("ACTIVE",), "ACTIVE": ()}

# What the server accepts for a push (any case), checked before anything is created.
_QUANTIZATIONS = ("FP32", "FP16", "INT8", "INT4", "DYNAMIC")
_RUNTIMES = ("TFLITE", "ONNX", "COREML", "GGUF", "CUSTOM")
# Semver 2.0.0 (prerelease and build allowed, no leading zeros), with the optional "v".
_SEMVER = re.compile(
    r"^v?(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)"
    r"(-(0|[1-9]\d*|\d*[a-zA-Z-][0-9a-zA-Z-]*)(\.(0|[1-9]\d*|\d*[a-zA-Z-][0-9a-zA-Z-]*))*)?"
    r"(\+[0-9a-zA-Z-]+(\.[0-9a-zA-Z-]+)*)?$"
)

DEFAULT_SERVER = "http://localhost:18080"

#: Deployment statuses after which a deployment no longer changes by itself.
TERMINAL_DEPLOYMENT_STATUSES = frozenset({"COMPLETE", "ROLLED_BACK", "CANCELLED", "FAILED"})

Json = Dict[str, Any]
PathLike = Union[str, "os.PathLike[str]"]


class FleetML:
    """A client for a FleetML server's REST API.

    ``server`` defaults to ``FLEETML_SERVER``, then ``http://localhost:18080``;
    ``api_key`` (an admin API key, sent as ``Authorization: Bearer``) defaults
    to ``FLEETML_API_KEY``. Every call to the server raises
    :class:`FleetMLError` when the server refuses it or cannot be reached; a
    local file that :meth:`push` cannot read raises Python's own ``OSError``.
    Use it as a context manager, or call :meth:`close`.
    """

    def __init__(
        self,
        server: Optional[str] = None,
        api_key: Optional[str] = None,
        *,
        timeout: float = 30.0,
        transport: Optional[httpx.BaseTransport] = None,
    ) -> None:
        self.server = (server or os.environ.get("FLEETML_SERVER") or DEFAULT_SERVER).rstrip("/")
        if not self.server.startswith(("http://", "https://")):
            source = "server" if server else "FLEETML_SERVER"
            raise ValueError(
                f"{source} must start with http:// or https:// (for example http://localhost:18080), "
                f"got {self.server!r}"
            )
        self._api_key = api_key if api_key is not None else os.environ.get("FLEETML_API_KEY", "")
        self._http = httpx.Client(timeout=timeout, transport=transport)
        self.projects = Projects(self)
        self.models = Models(self)
        self.versions = Versions(self)
        self.enrollment_keys = EnrollmentKeys(self)
        self.deployments = Deployments(self)
        self.devices = Devices(self)
        self.telemetry = Telemetry(self)
        self.experiments = Experiments(self)
        self.drift = Drift(self)

    def close(self) -> None:
        self._http.close()

    def __enter__(self) -> FleetML:
        return self

    def __exit__(self, *_: object) -> None:
        self.close()

    def __repr__(self) -> str:
        return f"FleetML(server={self.server!r})"

    def _request(
        self,
        method: str,
        path: str,
        body: Optional[Mapping[str, Any]] = None,
        params: Optional[Mapping[str, Any]] = None,
    ) -> Any:
        """Calls ``/api/v1`` + path and returns the response's ``data``."""
        headers = {"Authorization": f"Bearer {self._api_key}"} if self._api_key else {}
        try:
            resp = self._http.request(method, f"{self.server}/api/v1{path}", json=body, params=params, headers=headers)
        except httpx.HTTPError as e:
            raise FleetMLError(0, "UNAVAILABLE", f"server unavailable: {e}") from e
        if resp.status_code == 204:
            return None
        try:
            payload = resp.json()
        except ValueError:
            payload = None
        if not isinstance(payload, dict):
            if resp.is_error:
                raise FleetMLError(resp.status_code, "HTTP_ERROR", resp.text[:500] or resp.reason_phrase)
            raise FleetMLError(resp.status_code, "INVALID_RESPONSE", resp.text[:500])
        err = payload.get("error")
        if isinstance(err, dict) or resp.is_error:
            err = err if isinstance(err, dict) else {}
            code = str(err.get("code") or "HTTP_ERROR")
            message = str(err.get("message") or resp.reason_phrase)
            if resp.status_code == 401:
                message += " (pass api_key= or set FLEETML_API_KEY to an admin API key)"
            raise FleetMLError(resp.status_code, code, message)
        return payload.get("data")

    @staticmethod
    def _page(limit: Optional[int], offset: Optional[int], project: Optional[str] = None) -> Json:
        """List parameters; ``project`` filters by project ID (``project_id``)."""
        q = (("limit", limit), ("offset", offset), ("project_id", project))
        return {k: v for k, v in q if v is not None}

    def push(
        self,
        path: PathLike,
        project: str,
        model: str,
        version: str,
        quantization: str = "INT8",
        target_runtime: str = "TFLITE",
        task: str = "CLASSIFICATION",
        promote: bool = False,
    ) -> Version:
        """Uploads a model file as a variant of ``model`` ``version`` in
        ``project``, like ``fleetml push``: it creates the model (with
        ``target_runtime`` as its framework and ``task``) and the version if
        they do not exist, declares the variant with the file's size and
        sha256, uploads the file to the presigned URL (without the API key),
        and asks the server to verify it. With ``promote`` the version is then
        moved to STAGED and ACTIVE. Returns the version. A quantization,
        target_runtime or version the server would refuse raises
        ``FleetMLError`` (400 VALIDATION_ERROR) before anything is created.
        """
        _check_push_inputs(quantization, target_runtime, version)
        # The server keeps a version without its leading "v"; find it that way.
        version = version[1:] if version.startswith("v") else version
        size = os.path.getsize(path)
        digest = hashlib.sha256()
        with open(path, "rb") as f:
            for chunk in iter(lambda: f.read(1 << 20), b""):
                digest.update(chunk)

        try:
            model_obj = self.models.create(project, model, framework=target_runtime, task=task)
        except FleetMLError as create_err:
            found = self._find(
                lambda limit, offset: self.models.list(project, limit=limit, offset=offset), lambda m: m.name == model
            )
            if found is None:
                raise create_err
            model_obj = found

        try:
            version_obj = self.versions.create(model_obj.id, version)
        except FleetMLError as create_err:
            found_v = self._find(
                lambda limit, offset: self.versions.list(model_obj.id, limit=limit, offset=offset),
                lambda v: v.version == version,
            )
            if found_v is None:
                raise create_err
            version_obj = found_v

        created = self._request(
            "POST",
            f"/versions/{version_obj.id}/variants",
            {
                "quantization": quantization,
                "target_runtime": target_runtime,
                "blob_size_bytes": size,
                "blob_sha256": digest.hexdigest(),
            },
        )
        upload_url = str(created["upload_url"])
        variant_id = str(created["variant"]["id"])

        with open(path, "rb") as f:
            try:
                resp = self._http.put(upload_url, content=f, headers={"Content-Length": str(size)})
            except httpx.HTTPError as e:
                raise FleetMLError(0, "UPLOAD_FAILED", f"upload failed: {e}") from e
        if resp.status_code >= 300:
            raise FleetMLError(resp.status_code, "UPLOAD_FAILED", f"upload failed: HTTP {resp.status_code}")

        self._request("POST", f"/variants/{variant_id}/finalize")
        if promote:
            # From the version's status to ACTIVE, as the CLI does: an ACTIVE
            # version stays as it is, and a refused step (from ARCHIVED) raises.
            current = self.versions.get(version_obj.id).status
            for status in _PROMOTE_STEPS.get(current, ("STAGED", "ACTIVE")):
                self._request("PATCH", f"/versions/{version_obj.id}/status", {"status": status})
        return self.versions.get(version_obj.id)

    @staticmethod
    def _find(fetch: Callable[[int, int], List[T]], match: Callable[[T], bool]) -> Optional[T]:
        """The first item of a list that matches, read 100 at a time (the server's page cap)."""
        offset = 0
        while True:
            page = fetch(100, offset)
            for item in page:
                if match(item):
                    return item
            if len(page) < 100:
                return None
            offset += 100

    def wait_for_deployment(
        self,
        deployment_id: str,
        timeout: float = 600.0,
        interval: float = 2.0,
        statuses: Collection[str] = TERMINAL_DEPLOYMENT_STATUSES,
    ) -> Deployment:
        """Polls a deployment until its status is one of ``statuses`` (by
        default COMPLETE, ROLLED_BACK, CANCELLED or FAILED) and returns it.
        Raises :class:`TimeoutError` after ``timeout`` seconds."""
        deadline = time.monotonic() + timeout
        while True:
            d = self.deployments.get(deployment_id)
            if d.status in statuses:
                return d
            if time.monotonic() >= deadline:
                raise TimeoutError(f"deployment {deployment_id} is still {d.status} after {timeout:g}s")
            time.sleep(interval)


class _API:
    def __init__(self, client: FleetML) -> None:
        self._c = client


class Projects(_API):
    """``fleetml project``."""

    def create(self, name: str, description: str = "") -> Project:
        return Project.from_api(self._c._request("POST", "/projects", {"name": name, "description": description}))

    def list(self, *, limit: Optional[int] = None, offset: Optional[int] = None) -> List[Project]:
        return from_list(self._c._request("GET", "/projects", params=self._c._page(limit, offset)), Project.from_api)

    def get(self, project_id: str) -> Project:
        return Project.from_api(self._c._request("GET", f"/projects/{project_id}"))

    def delete(self, project_id: str) -> None:
        """Deletes the project with everything in it: its models, versions,
        deployments, experiments, enrollment keys and devices, and the
        telemetry of its versions (``fleetml project delete``)."""
        self._c._request("DELETE", f"/projects/{project_id}")


class Models(_API):
    """``fleetml model``."""

    def create(self, project: str, name: str, framework: str = "TFLITE", task: str = "CLASSIFICATION") -> Model:
        body = {"name": name, "framework": framework, "task_type": task}
        return Model.from_api(self._c._request("POST", f"/projects/{project}/models", body))

    def list(self, project: str, *, limit: Optional[int] = None, offset: Optional[int] = None) -> List[Model]:
        data = self._c._request("GET", f"/projects/{project}/models", params=self._c._page(limit, offset))
        return from_list(data, Model.from_api)

    def get(self, model_id: str) -> Model:
        return Model.from_api(self._c._request("GET", f"/models/{model_id}"))


class Versions(_API):
    """``fleetml ver``."""

    def create(self, model: str, version: str, metadata: Optional[Mapping[str, Any]] = None) -> Version:
        body: Json = {"version": version}
        if metadata is not None:
            body["metadata"] = dict(metadata)
        return Version.from_api(self._c._request("POST", f"/models/{model}/versions", body))

    def list(self, model: str, *, limit: Optional[int] = None, offset: Optional[int] = None) -> List[Version]:
        data = self._c._request("GET", f"/models/{model}/versions", params=self._c._page(limit, offset))
        return from_list(data, Version.from_api)

    def get(self, version_id: str) -> Version:
        return Version.from_api(self._c._request("GET", f"/versions/{version_id}"))

    def promote(self, version_id: str, status: str = "ACTIVE") -> Version:
        """Moves a version to ``status`` (STAGED, ACTIVE or ARCHIVED), like
        ``fleetml ver promote``. To ACTIVE, a DRAFT version goes through
        STAGED first, as the server's lifecycle requires."""
        status = status.upper()
        if status == "ACTIVE" and self.get(version_id).status == "DRAFT":
            self._c._request("PATCH", f"/versions/{version_id}/status", {"status": "STAGED"})
        return Version.from_api(self._c._request("PATCH", f"/versions/{version_id}/status", {"status": status}))


class EnrollmentKeys(_API):
    """A project's enrollment keys, which devices register with."""

    def create(self, project: str, name: str) -> EnrollmentKey:
        return EnrollmentKey.from_api(self._c._request("POST", f"/projects/{project}/enrollment-keys", {"name": name}))


class Deployments(_API):
    """``fleetml deploy``."""

    def create(
        self,
        project: str,
        version: str,
        strategy: str = "ALL_AT_ONCE",
        target_selector: Optional[Mapping[str, str]] = None,
        health_gate: Optional[Mapping[str, Any]] = None,
        canary_stages: Optional[Sequence[Mapping[str, Any]]] = None,
        rolling_config: Optional[Mapping[str, Any]] = None,
        urgent: bool = False,
    ) -> Deployment:
        """Creates a deployment of model version ``version`` (an ID) in
        ``project``. ``health_gate``, ``canary_stages`` and ``rolling_config``
        are passed through as the REST API documents them (docs/ROLLOUTS.md)."""
        body: Json = {"project_id": project, "model_version_id": version, "strategy": strategy, "urgent": urgent}
        if target_selector is not None:
            body["target_selector"] = dict(target_selector)
        if health_gate is not None:
            body["health_gate"] = dict(health_gate)
        if canary_stages is not None:
            body["canary_stages"] = [dict(s) for s in canary_stages]
        if rolling_config is not None:
            body["rolling_config"] = dict(rolling_config)
        return Deployment.from_api(self._c._request("POST", "/deployments", body))

    def start(self, deployment_id: str) -> Deployment:
        return Deployment.from_api(self._c._request("POST", f"/deployments/{deployment_id}/start"))

    def get(self, deployment_id: str) -> Deployment:
        return Deployment.from_api(self._c._request("GET", f"/deployments/{deployment_id}"))

    def list(
        self, *, project: Optional[str] = None, limit: Optional[int] = None, offset: Optional[int] = None
    ) -> List[Deployment]:
        """Lists deployments, newest first; with ``project``, that project's only."""
        params = self._c._page(limit, offset, project)
        return from_list(self._c._request("GET", "/deployments", params=params), Deployment.from_api)

    def rollback(self, deployment_id: str, reason: str = "manual rollback") -> Deployment:
        data = self._c._request("POST", f"/deployments/{deployment_id}/rollback", {"reason": reason})
        return Deployment.from_api(data)

    def pause(self, deployment_id: str) -> Deployment:
        """Pauses a rolling-out deployment where it is."""
        return Deployment.from_api(self._c._request("POST", f"/deployments/{deployment_id}/pause"))

    def resume(self, deployment_id: str) -> Deployment:
        return Deployment.from_api(self._c._request("POST", f"/deployments/{deployment_id}/resume"))

    def cancel(self, deployment_id: str) -> Deployment:
        """Stops a rollout; the devices it reached return to the deployment before it."""
        return Deployment.from_api(self._c._request("POST", f"/deployments/{deployment_id}/cancel"))

    def progress(self, deployment_id: str) -> Deployment:
        """Moves a canary or rolling deployment to its next stage now, if its
        health gate passes and the stage's minimum time has elapsed
        (``health().can_progress``); otherwise returns it unchanged. A
        deployment that is not ROLLING_OUT raises ``INVALID_TRANSITION``."""
        return Deployment.from_api(self._c._request("POST", f"/deployments/{deployment_id}/progress"))

    def health(self, deployment_id: str) -> DeploymentHealth:
        """The health gate of the deployment's current stage."""
        return DeploymentHealth.from_api(self._c._request("GET", f"/deployments/{deployment_id}/health"))


class Devices(_API):
    """``fleetml device``."""

    def list(
        self,
        labels: Optional[Mapping[str, str]] = None,
        *,
        project: Optional[str] = None,
        limit: Optional[int] = None,
        offset: Optional[int] = None,
    ) -> List[Device]:
        """Lists devices; with ``project=`` (a project ID), that project's
        only, and with ``labels`` (a mapping, the one positional argument),
        only those that have all of them."""
        if labels is not None and not isinstance(labels, Mapping):
            raise TypeError(
                "devices.list() takes labels as a mapping; pass a project ID as project=<id>"
                f" (got {type(labels).__name__} {labels!r} by position)"
            )
        params = self._c._page(limit, offset, project)
        for k, v in (labels or {}).items():
            params[f"label.{k}"] = v
        return from_list(self._c._request("GET", "/devices", params=params), Device.from_api)

    def get(self, device_id: str) -> Device:
        return Device.from_api(self._c._request("GET", f"/devices/{device_id}"))


class Telemetry(_API):
    """``fleetml telemetry``."""

    def summary(self, version_id: str) -> TelemetrySummary:
        """The last day's telemetry of a version: latency and confidence over
        its INFERENCE events, counts over every event."""
        return TelemetrySummary.from_api(self._c._request("GET", f"/telemetry/summary/{version_id}"))

    def events(
        self,
        version_id: Optional[str] = None,
        device_id: Optional[str] = None,
        event_type: Optional[str] = None,
        *,
        limit: Optional[int] = None,
        offset: Optional[int] = None,
    ) -> List[TelemetryEvent]:
        """The last day's events, newest first, of a version, a device or a type."""
        params = self._c._page(limit, offset)
        for k, v in (("model_version_id", version_id), ("device_id", device_id), ("event_type", event_type)):
            if v is not None:
                params[k] = v
        return from_list(self._c._request("GET", "/telemetry/events", params=params), TelemetryEvent.from_api)


class Experiments(_API):
    """``fleetml experiment``."""

    def create(
        self,
        project: str,
        name: str,
        control: str,
        treatment: str,
        metric: str = "latency",
        split: int = 50,
        auto_conclude: bool = False,
        target_selector: Optional[Mapping[str, str]] = None,
        min_sample_size: Optional[int] = None,
    ) -> Experiment:
        """Creates an A/B experiment between the model versions ``control``
        and ``treatment`` (IDs); ``split`` is the treatment's share in percent."""
        body: Json = {
            "project_id": project,
            "name": name,
            "control_version_id": control,
            "treatment_version_id": treatment,
            "primary_metric": metric,
            "split_ratio": split,
            "auto_conclude": auto_conclude,
        }
        if target_selector is not None:
            body["target_selector"] = dict(target_selector)
        if min_sample_size is not None:
            body["min_sample_size"] = min_sample_size
        return Experiment.from_api(self._c._request("POST", "/experiments", body))

    def start(self, experiment_id: str) -> Experiment:
        return Experiment.from_api(self._c._request("POST", f"/experiments/{experiment_id}/start"))

    def get(self, experiment_id: str) -> Experiment:
        return Experiment.from_api(self._c._request("GET", f"/experiments/{experiment_id}"))

    def list(
        self, *, project: Optional[str] = None, limit: Optional[int] = None, offset: Optional[int] = None
    ) -> List[Experiment]:
        """Lists experiments; with ``project``, that project's only."""
        params = self._c._page(limit, offset, project)
        return from_list(self._c._request("GET", "/experiments", params=params), Experiment.from_api)

    def pause(self, experiment_id: str) -> Experiment:
        return Experiment.from_api(self._c._request("POST", f"/experiments/{experiment_id}/pause"))

    def resume(self, experiment_id: str) -> Experiment:
        return Experiment.from_api(self._c._request("POST", f"/experiments/{experiment_id}/resume"))

    def conclude(self, experiment_id: str, winner: str, note: str = "") -> Experiment:
        """Ends the experiment with ``winner``, the control's or the
        treatment's version ID."""
        body = {"winner_version_id": winner, "note": note}
        return Experiment.from_api(self._c._request("POST", f"/experiments/{experiment_id}/conclude", body))

    def cancel(self, experiment_id: str) -> Experiment:
        """Ends the experiment without a winner."""
        return Experiment.from_api(self._c._request("POST", f"/experiments/{experiment_id}/cancel"))

    def results(self, experiment_id: str) -> ExperimentResults:
        return ExperimentResults.from_api(self._c._request("GET", f"/experiments/{experiment_id}/results"))


class Drift(_API):
    """``fleetml drift``."""

    def check(self, version_id: str) -> List[DriftResult]:
        """Runs a drift check now: one result per metric, once a baseline
        exists (telemetry from the 7 days before the last day); none before."""
        return from_list(self._c._request("POST", f"/drift/{version_id}/check"), DriftResult.from_api)

    def status(self, version_id: str) -> List[DriftResult]:
        """The latest result per metric (``fleetml drift status``)."""
        return from_list(self._c._request("GET", f"/drift/{version_id}"), DriftResult.from_api)

    def history(self, version_id: str) -> List[DriftResult]:
        """The last 50 drift results, newest first."""
        return from_list(self._c._request("GET", f"/drift/{version_id}/history"), DriftResult.from_api)


def _check_push_inputs(quantization: str, target_runtime: str, version: str) -> None:
    """Refuses what the server would refuse, so a refused push leaves no model or version."""
    if quantization.strip().upper() not in _QUANTIZATIONS:
        raise FleetMLError(
            400, "VALIDATION_ERROR", f"quantization must be one of {', '.join(_QUANTIZATIONS)}, got {quantization!r}"
        )
    if target_runtime.strip().upper() not in _RUNTIMES:
        raise FleetMLError(
            400, "VALIDATION_ERROR", f"target_runtime must be one of {', '.join(_RUNTIMES)}, got {target_runtime!r}"
        )
    if not _SEMVER.fullmatch(version):
        raise FleetMLError(
            400,
            "VALIDATION_ERROR",
            f'invalid version "{version}": expected MAJOR.MINOR.PATCH (semver 2.0.0), such as 1.0.0 or 1.0.0-rc.1',
        )
