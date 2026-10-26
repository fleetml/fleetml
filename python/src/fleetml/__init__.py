"""FleetML client: push models and run rollouts on a FleetML server from Python.

The library speaks the server's REST API, the same API the ``fleetml`` CLI uses,
with the same words: projects, models, versions, push, enrollment keys,
deployments, devices, telemetry, experiments and drift.
"""

from fleetml.client import TERMINAL_DEPLOYMENT_STATUSES, FleetML
from fleetml.errors import FleetMLError
from fleetml.types import (
    ArmResult,
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
)

# The package version: the single place it is set (pyproject.toml reads it),
# equal to the FleetML release it ships with.
__version__ = "0.10.2"

__all__ = [
    "TERMINAL_DEPLOYMENT_STATUSES",
    "ArmResult",
    "Deployment",
    "DeploymentHealth",
    "Device",
    "DriftResult",
    "EnrollmentKey",
    "Experiment",
    "ExperimentResults",
    "FleetML",
    "FleetMLError",
    "Model",
    "Project",
    "TelemetryEvent",
    "TelemetrySummary",
    "Version",
    "__version__",
]
