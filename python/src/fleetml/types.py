"""The responses users read, as dataclasses.

Each keeps the server's full JSON object in ``raw``, so fields that are not
listed here are still there. Timestamps are the server's RFC 3339 strings.
"""

from __future__ import annotations

from dataclasses import dataclass, field
from typing import Any, Dict, List, Mapping, Optional


def _str(d: Mapping[str, Any], key: str) -> str:
    v = d.get(key)
    return "" if v is None else str(v)


def _opt_str(d: Mapping[str, Any], key: str) -> Optional[str]:
    v = d.get(key)
    return None if v is None else str(v)


def _int(d: Mapping[str, Any], key: str) -> int:
    v = d.get(key)
    return int(v) if isinstance(v, (int, float)) else 0


def _float(d: Mapping[str, Any], key: str) -> float:
    v = d.get(key)
    return float(v) if isinstance(v, (int, float)) else 0.0


def _opt_float(d: Mapping[str, Any], key: str) -> Optional[float]:
    v = d.get(key)
    return float(v) if isinstance(v, (int, float)) else None


def _labels(d: Mapping[str, Any], key: str) -> Dict[str, str]:
    v = d.get(key)
    return {str(k): str(x) for k, x in v.items()} if isinstance(v, dict) else {}


@dataclass(frozen=True)
class Project:
    id: str
    name: str
    slug: str
    description: str
    created_at: str
    raw: Dict[str, Any] = field(repr=False, compare=False)

    @classmethod
    def from_api(cls, d: Mapping[str, Any]) -> Project:
        return cls(
            _str(d, "id"), _str(d, "name"), _str(d, "slug"), _str(d, "description"), _str(d, "created_at"), dict(d)
        )


@dataclass(frozen=True)
class Model:
    id: str
    project_id: str
    name: str
    framework: str
    task_type: str
    created_at: str
    raw: Dict[str, Any] = field(repr=False, compare=False)

    @classmethod
    def from_api(cls, d: Mapping[str, Any]) -> Model:
        return cls(
            _str(d, "id"),
            _str(d, "project_id"),
            _str(d, "name"),
            _str(d, "framework"),
            _str(d, "task_type"),
            _str(d, "created_at"),
            dict(d),
        )


@dataclass(frozen=True)
class Version:
    """A model version. ``status`` is DRAFT, STAGED, ACTIVE or ARCHIVED."""

    id: str
    model_id: str
    version: str
    status: str
    metadata: Dict[str, Any]
    created_at: str
    raw: Dict[str, Any] = field(repr=False, compare=False)

    @classmethod
    def from_api(cls, d: Mapping[str, Any]) -> Version:
        meta = d.get("metadata")
        return cls(
            _str(d, "id"),
            _str(d, "model_id"),
            _str(d, "version"),
            _str(d, "status"),
            dict(meta) if isinstance(meta, dict) else {},
            _str(d, "created_at"),
            dict(d),
        )


@dataclass(frozen=True)
class EnrollmentKey:
    """A project's enrollment key. ``key`` is returned only when it is created."""

    id: str
    project_id: str
    name: str
    key: str
    created_at: str
    raw: Dict[str, Any] = field(repr=False, compare=False)

    @classmethod
    def from_api(cls, d: Mapping[str, Any]) -> EnrollmentKey:
        return cls(
            _str(d, "id"), _str(d, "project_id"), _str(d, "name"), _str(d, "key"), _str(d, "created_at"), dict(d)
        )

    def __repr__(self) -> str:
        return f"EnrollmentKey(id={self.id!r}, project_id={self.project_id!r}, name={self.name!r}, key='…')"


@dataclass(frozen=True)
class Deployment:
    """A rollout. ``status`` is PENDING, ROLLING_OUT, PAUSED, COMPLETE,
    ROLLED_BACK, CANCELLED or FAILED; ``strategy`` is ALL_AT_ONCE, CANARY or
    ROLLING."""

    id: str
    project_id: str
    model_version_id: str
    strategy: str
    status: str
    target_selector: Dict[str, str]
    current_stage: int
    urgent: bool
    rollback_reason: str
    created_at: str
    raw: Dict[str, Any] = field(repr=False, compare=False)

    @classmethod
    def from_api(cls, d: Mapping[str, Any]) -> Deployment:
        return cls(
            _str(d, "id"),
            _str(d, "project_id"),
            _str(d, "model_version_id"),
            _str(d, "strategy"),
            _str(d, "status"),
            _labels(d, "target_selector"),
            _int(d, "current_stage"),
            bool(d.get("urgent")),
            _str(d, "rollback_reason"),
            _str(d, "created_at"),
            dict(d),
        )


@dataclass(frozen=True)
class DeploymentHealth:
    """A deployment's health gate now: whether its current stage is healthy
    (``reason`` says why not), whether it is held, and whether it may move on."""

    deployment_id: str
    status: str
    current_stage: int
    healthy: bool
    hold: bool
    reason: str
    can_progress: bool
    cohort_devices: int
    raw: Dict[str, Any] = field(repr=False, compare=False)

    @classmethod
    def from_api(cls, d: Mapping[str, Any]) -> DeploymentHealth:
        return cls(
            _str(d, "deployment_id"),
            _str(d, "status"),
            _int(d, "current_stage"),
            bool(d.get("healthy")),
            bool(d.get("hold")),
            _str(d, "reason"),
            bool(d.get("can_progress")),
            _int(d, "cohort_devices"),
            dict(d),
        )


@dataclass(frozen=True)
class Device:
    id: str
    project_id: Optional[str]
    os: str
    os_version: str
    ram_mb: int
    chipset: str
    agent_version: str
    labels: Dict[str, str]
    last_seen_at: Optional[str]
    raw: Dict[str, Any] = field(repr=False, compare=False)

    @classmethod
    def from_api(cls, d: Mapping[str, Any]) -> Device:
        return cls(
            _str(d, "id"),
            _opt_str(d, "project_id"),
            _str(d, "os"),
            _str(d, "os_version"),
            _int(d, "ram_mb"),
            _str(d, "chipset"),
            _str(d, "agent_version"),
            _labels(d, "labels"),
            _opt_str(d, "last_seen_at"),
            dict(d),
        )


@dataclass(frozen=True)
class TelemetryEvent:
    """One event a device reported. ``event_type`` is INFERENCE, MODEL_LOAD,
    MODEL_LOAD_FAILED, OOM, PREDICTION_DRIFT, ERROR or SIGNATURE_INVALID."""

    device_id: str
    model_version_id: str
    event_type: str
    latency_ms: int
    #: None for an event that reported no confidence (MODEL_LOAD, for example).
    confidence_score: Optional[float]
    prediction_label: str
    error_message: str
    recorded_at: str
    raw: Dict[str, Any] = field(repr=False, compare=False)

    @classmethod
    def from_api(cls, d: Mapping[str, Any]) -> TelemetryEvent:
        return cls(
            _str(d, "device_id"),
            _str(d, "model_version_id"),
            _str(d, "event_type"),
            _int(d, "latency_ms"),
            _opt_float(d, "confidence_score") or None,
            _str(d, "prediction_label"),
            _str(d, "error_message"),
            _str(d, "recorded_at"),
            dict(d),
        )


@dataclass(frozen=True)
class TelemetrySummary:
    """Inference telemetry of one model version over the last day."""

    model_version_id: str
    total_events: int
    error_count: int
    avg_latency_ms: float
    p50_latency_ms: float
    p95_latency_ms: float
    p99_latency_ms: float
    avg_confidence: float
    raw: Dict[str, Any] = field(repr=False, compare=False)

    @classmethod
    def from_api(cls, d: Mapping[str, Any]) -> TelemetrySummary:
        return cls(
            _str(d, "model_version_id"),
            _int(d, "total_events"),
            _int(d, "error_count"),
            _float(d, "avg_latency_ms"),
            _float(d, "p50_latency_ms"),
            _float(d, "p95_latency_ms"),
            _float(d, "p99_latency_ms"),
            _float(d, "avg_confidence"),
            dict(d),
        )


@dataclass(frozen=True)
class Experiment:
    """An A/B experiment. ``status`` is DRAFT, RUNNING, PAUSED, CONCLUDED or CANCELLED."""

    id: str
    project_id: str
    name: str
    status: str
    control_version_id: str
    treatment_version_id: str
    primary_metric: str
    split_ratio: int
    min_sample_size: int
    raw: Dict[str, Any] = field(repr=False, compare=False)

    @classmethod
    def from_api(cls, d: Mapping[str, Any]) -> Experiment:
        return cls(
            _str(d, "id"),
            _str(d, "project_id"),
            _str(d, "name"),
            _str(d, "status"),
            _str(d, "control_version_id"),
            _str(d, "treatment_version_id"),
            _str(d, "primary_metric"),
            _int(d, "split_ratio"),
            _int(d, "min_sample_size"),
            dict(d),
        )


@dataclass(frozen=True)
class ArmResult:
    """One arm (control or treatment) of an experiment's results."""

    version_id: str
    sample_size: int
    avg_latency_ms: float
    p95_latency_ms: float
    error_rate: float
    avg_confidence: float
    raw: Dict[str, Any] = field(repr=False, compare=False)

    @classmethod
    def from_api(cls, d: Mapping[str, Any]) -> ArmResult:
        return cls(
            _str(d, "version_id"),
            _int(d, "sample_size"),
            _float(d, "avg_latency_ms"),
            _float(d, "p95_latency_ms"),
            _float(d, "error_rate"),
            _float(d, "avg_confidence"),
            dict(d),
        )


@dataclass(frozen=True)
class ExperimentResults:
    """``recommendation`` is COLLECTING_DATA until both arms reach
    ``min_sample_size``, then ADOPT_TREATMENT, KEEP_CONTROL or INCONCLUSIVE;
    ``final`` is true once the test has run."""

    control: ArmResult
    treatment: ArmResult
    p_value: Optional[float]
    effect_size: Optional[float]
    is_significant: bool
    recommendation: str
    final: bool
    raw: Dict[str, Any] = field(repr=False, compare=False)

    @classmethod
    def from_api(cls, d: Mapping[str, Any]) -> ExperimentResults:
        control, treatment = d.get("control"), d.get("treatment")
        return cls(
            ArmResult.from_api(control if isinstance(control, dict) else {}),
            ArmResult.from_api(treatment if isinstance(treatment, dict) else {}),
            _opt_float(d, "p_value"),
            _opt_float(d, "effect_size"),
            bool(d.get("is_significant")),
            _str(d, "recommendation"),
            bool(d.get("final")),
            dict(d),
        )


@dataclass(frozen=True)
class DriftResult:
    """One drift check of one metric. ``severity`` is NONE, WARNING or CRITICAL."""

    id: str
    model_version_id: str
    metric: str
    psi_value: float
    severity: str
    message: str
    checked_at: str
    raw: Dict[str, Any] = field(repr=False, compare=False)

    @classmethod
    def from_api(cls, d: Mapping[str, Any]) -> DriftResult:
        return cls(
            _str(d, "id"),
            _str(d, "model_version_id"),
            _str(d, "metric"),
            _float(d, "psi_value"),
            _str(d, "severity"),
            _str(d, "message"),
            _str(d, "checked_at"),
            dict(d),
        )


def from_list(items: Any, parse: Any) -> List[Any]:
    """Parses a JSON array with ``parse``; anything else is an empty list."""
    return [parse(x) for x in items] if isinstance(items, list) else []
