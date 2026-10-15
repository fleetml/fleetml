"""The FleetML client."""

from __future__ import annotations

import contextlib
import hashlib
import os
import time
from typing import Any, Collection, Dict, List, Mapping, Optional, Sequence, Union

import httpx

from fleetml.errors import FleetMLError
from fleetml.types import (
    Deployment,
    Device,
    DriftResult,
    EnrollmentKey,
    Experiment,
    ExperimentResults,
    Model,
    Project,
    TelemetrySummary,
    Version,
    from_list,
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
    to ``FLEETML_API_KEY``. Every call raises :class:`FleetMLError` when the
    server refuses it or cannot be reached. Use it as a context manager, or
    call :meth:`close`.
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
    def _page(limit: Optional[int], offset: Optional[int]) -> Json:
        return {k: v for k, v in (("limit", limit), ("offset", offset)) if v is not None}

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
        moved to STAGED and ACTIVE. Returns the version.
        """
        size = os.path.getsize(path)
        digest = hashlib.sha256()
        with open(path, "rb") as f:
            for chunk in iter(lambda: f.read(1 << 20), b""):
                digest.update(chunk)

        try:
            model_obj = self.models.create(project, model, framework=target_runtime, task=task)
        except FleetMLError as create_err:
            found = [m for m in self.models.list(project) if m.name == model]
            if not found:
                raise create_err
            model_obj = found[0]

        try:
            version_obj = self.versions.create(model_obj.id, version)
        except FleetMLError as create_err:
            found_v = [v for v in self.versions.list(model_obj.id) if v.version == version]
            if not found_v:
                raise create_err
            version_obj = found_v[0]

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
            # DRAFT -> STAGED -> ACTIVE, as the CLI does (a version that is
            # already further along keeps its status).
            for status in ("STAGED", "ACTIVE"):
                with contextlib.suppress(FleetMLError):
                    self._request("PATCH", f"/versions/{version_obj.id}/status", {"status": status})
        return self.versions.get(version_obj.id)

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

    def list(self, limit: Optional[int] = None, offset: Optional[int] = None) -> List[Project]:
        return from_list(self._c._request("GET", "/projects", params=self._c._page(limit, offset)), Project.from_api)

    def get(self, project_id: str) -> Project:
        return Project.from_api(self._c._request("GET", f"/projects/{project_id}"))


class Models(_API):
    """``fleetml model``."""

    def create(self, project: str, name: str, framework: str = "TFLITE", task: str = "CLASSIFICATION") -> Model:
        body = {"name": name, "framework": framework, "task_type": task}
        return Model.from_api(self._c._request("POST", f"/projects/{project}/models", body))

    def list(self, project: str, limit: Optional[int] = None, offset: Optional[int] = None) -> List[Model]:
        data = self._c._request("GET", f"/projects/{project}/models", params=self._c._page(limit, offset))
        return from_list(data, Model.from_api)


class Versions(_API):
    """``fleetml ver``."""

    def create(self, model: str, version: str, metadata: Optional[Mapping[str, Any]] = None) -> Version:
        body: Json = {"version": version}
        if metadata is not None:
            body["metadata"] = dict(metadata)
        return Version.from_api(self._c._request("POST", f"/models/{model}/versions", body))

    def list(self, model: str, limit: Optional[int] = None, offset: Optional[int] = None) -> List[Version]:
        data = self._c._request("GET", f"/models/{model}/versions", params=self._c._page(limit, offset))
        return from_list(data, Version.from_api)

    def get(self, version_id: str) -> Version:
        return Version.from_api(self._c._request("GET", f"/versions/{version_id}"))


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

    def list(self, limit: Optional[int] = None, offset: Optional[int] = None) -> List[Deployment]:
        return from_list(
            self._c._request("GET", "/deployments", params=self._c._page(limit, offset)), Deployment.from_api
        )

    def rollback(self, deployment_id: str, reason: str = "manual rollback") -> Deployment:
        data = self._c._request("POST", f"/deployments/{deployment_id}/rollback", {"reason": reason})
        return Deployment.from_api(data)


class Devices(_API):
    """``fleetml device``."""

    def list(
        self,
        labels: Optional[Mapping[str, str]] = None,
        limit: Optional[int] = None,
        offset: Optional[int] = None,
    ) -> List[Device]:
        """Lists devices; with ``labels``, only those that have all of them."""
        params = self._c._page(limit, offset)
        for k, v in (labels or {}).items():
            params[f"label.{k}"] = v
        return from_list(self._c._request("GET", "/devices", params=params), Device.from_api)


class Telemetry(_API):
    """``fleetml telemetry``."""

    def summary(self, version_id: str) -> TelemetrySummary:
        return TelemetrySummary.from_api(self._c._request("GET", f"/telemetry/summary/{version_id}"))


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

    def results(self, experiment_id: str) -> ExperimentResults:
        return ExperimentResults.from_api(self._c._request("GET", f"/experiments/{experiment_id}/results"))


class Drift(_API):
    """``fleetml drift``."""

    def check(self, version_id: str) -> List[DriftResult]:
        """Runs a drift check now: one result per metric."""
        return from_list(self._c._request("POST", f"/drift/{version_id}/check"), DriftResult.from_api)

    def history(self, version_id: str) -> List[DriftResult]:
        """The last 50 drift results, newest first."""
        return from_list(self._c._request("GET", f"/drift/{version_id}/history"), DriftResult.from_api)
