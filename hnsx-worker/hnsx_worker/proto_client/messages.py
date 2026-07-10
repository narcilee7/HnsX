"""Pure-Python dataclasses for the HnsX control-plane wire contract.

These types are the public API of ``hnsx_worker.proto_client``. The runtime
(``worker_service``, tests, plugins) should depend on these dataclasses and
never on the generated ``*_pb2`` / ``*_pb2_grpc`` modules directly.

The mapping to protobuf is internal — see ``client.py``.
"""

from __future__ import annotations

from collections.abc import Iterable
from dataclasses import dataclass, field
from typing import Any

# ---------------------------------------------------------------------------
# Worker registration / heartbeat
# ---------------------------------------------------------------------------


@dataclass
class ResourceCapacity:
    """Static capacity the worker advertises at register time."""

    max_concurrent_sessions: int = 0
    providers: list[str] = field(default_factory=list)
    models: list[str] = field(default_factory=list)
    sandbox_runtimes: list[str] = field(default_factory=list)
    memory_total_bytes: int = 0
    cpu_total_cores: float = 0.0


@dataclass
class ResourceUsage:
    """Live resource usage reported on every heartbeat."""

    running_sessions: int = 0
    free_slots: int = 0
    cpu_percent: float = 0.0
    memory_used_bytes: int = 0
    uptime_seconds: int = 0


class WorkerHealthStatus:
    """Worker health states (mirror of ``WorkerHealth.Status``)."""

    UNSPECIFIED = "STATUS_UNSPECIFIED"
    HEALTHY = "STATUS_HEALTHY"
    DEGRADED = "STATUS_DEGRADED"
    DRAINING = "STATUS_DRAINING"


@dataclass
class WorkerHealth:
    status: str = WorkerHealthStatus.HEALTHY
    message: str = ""
    last_event_ms: int = 0


@dataclass
class WorkerInfo:
    worker_id: str = ""
    tenant_id: str = ""
    version: str = ""
    region: str = ""
    hostname: str = ""
    pid: str = ""
    capacity: ResourceCapacity = field(default_factory=ResourceCapacity)
    labels: dict[str, str] = field(default_factory=dict)


@dataclass
class HeartbeatRequest:
    worker_id: str = ""
    timestamp_ms: int = 0
    usage: ResourceUsage = field(default_factory=ResourceUsage)
    running_session_ids: list[str] = field(default_factory=list)
    health: WorkerHealth = field(default_factory=WorkerHealth)


# ---------------------------------------------------------------------------
# Server responses
# ---------------------------------------------------------------------------


@dataclass
class RegisterResponse:
    worker_id: str = ""
    server_time_ms: int = 0
    heartbeat_interval_seconds: int = 0


@dataclass
class HeartbeatResponse:
    server_time_ms: int = 0
    drain_deadline_seconds: int = 0
    drain_reason: str = ""


# ---------------------------------------------------------------------------
# Session assignment / acknowledgment
# ---------------------------------------------------------------------------


@dataclass
class SessionAssignment:
    """One PullSessionResponse — the worker's next job."""

    session_id: str = ""
    domain_id: str = ""
    domain_version: str = ""
    domain_spec_json: str = ""
    trigger_payload_json: str = ""
    trace_id: str = ""
    correlation_id: str = ""
    assigned_at_ms: int = 0
    session_timeout_seconds: int = 0

    def is_empty(self) -> bool:
        """``PullSessionResponse`` with no session_id == "no work, loop again"."""
        return not self.session_id


@dataclass
class AckRequest:
    worker_id: str = ""
    session_id: str = ""
    correlation_id: str = ""
    acknowledged_at_ms: int = 0


@dataclass
class NackRequest:
    worker_id: str = ""
    session_id: str = ""
    correlation_id: str = ""
    reason: str = ""
    error_code: str = ""
    requeue: bool = True


# ---------------------------------------------------------------------------
# Stream channel — outbound (worker → server)
# ---------------------------------------------------------------------------


@dataclass
class Observation:
    """One observation emitted by the session runtime."""

    session_id: str = ""
    domain_id: str = ""
    domain_version: str = ""
    step_id: str = ""
    agent_id: str = ""
    parent_id: str = ""
    kind: str = ""
    role: str = ""
    payload: dict[str, Any] = field(default_factory=dict)
    metadata: dict[str, Any] = field(default_factory=dict)
    trace_id: str = ""
    created_at_ms: int = 0


@dataclass
class SessionStatusUpdate:
    session_id: str = ""
    state: str = ""
    message: str = ""
    timestamp_ms: int = 0


@dataclass
class SessionFinalResult:
    session_id: str = ""
    result: dict[str, Any] = field(default_factory=dict)
    total_cost_usd: float = 0.0
    total_prompt_tokens: int = 0
    total_completion_tokens: int = 0
    duration_ms: int = 0


# One outbound message — exactly one of these fields is set per envelope.
@dataclass
class OutboundMessage:
    """One ``StreamChannelRequest`` payload.

    Use the ``kind`` discriminator to build the right envelope via
    ``client.build_stream_request``.
    """

    kind: str = ""  # 'observations' | 'status' | 'result'
    observations: list[Observation] = field(default_factory=list)
    status: SessionStatusUpdate | None = None
    result: SessionFinalResult | None = None


# ---------------------------------------------------------------------------
# Stream channel — inbound (server → worker)
# ---------------------------------------------------------------------------


@dataclass
class CancelCommand:
    session_id: str = ""
    correlation_id: str = ""
    reason: str = ""
    deadline_ms: int = 0


@dataclass
class DrainCommand:
    deadline_seconds: int = 0
    reason: str = ""


@dataclass
class DomainInvalidation:
    domain_id: str = ""
    version: str = ""


@dataclass
class PingCommand:
    timestamp_ms: int = 0


@dataclass
class ServerEvent:
    """One ``StreamChannelResponse``, normalized into a tagged union.

    The ``kind`` field is always set to one of:
        ``"cancel"`` | ``"drain"`` | ``"invalidate"`` | ``"ping"``.
    The corresponding payload attribute holds the typed payload.
    """

    kind: str = ""
    correlation_id: str = ""
    cancel: CancelCommand | None = None
    drain: DrainCommand | None = None
    invalidate: DomainInvalidation | None = None
    ping: PingCommand | None = None


# ---------------------------------------------------------------------------
# Errors
# ---------------------------------------------------------------------------


class ControlPlaneError(Exception):
    """Base class for control-plane client errors."""


class RpcUnavailable(ControlPlaneError):
    """Raised when the gRPC channel is unreachable / canceled."""


# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------


def split_iter(items: Iterable[Any], n: int) -> list[list[Any]]:
    """Convenience for batching — splits ``items`` into chunks of size ``n``."""
    out: list[list[Any]] = []
    chunk: list[Any] = []
    for it in items:
        chunk.append(it)
        if len(chunk) >= n:
            out.append(chunk)
            chunk = []
    if chunk:
        out.append(chunk)
    return out
