"""HnsX control-plane gRPC client.

Runtime code (``worker_service``, tests, plugins) should depend on the
types in this package and **never** on the generated ``*_pb2`` modules.
The mapping between our dataclass API and the wire types lives entirely
inside ``client.py``.
"""

from .client import ControlPlaneClient
from .messages import (
    AckRequest,
    CancelCommand,
    ControlPlaneError,
    DomainInvalidation,
    DrainCommand,
    HeartbeatRequest,
    HeartbeatResponse,
    NackRequest,
    Observation,
    OutboundMessage,
    PingCommand,
    RegisterResponse,
    ResourceCapacity,
    ResourceUsage,
    ServerEvent,
    SessionAssignment,
    SessionFinalResult,
    SessionStatusUpdate,
    WorkerHealth,
    WorkerHealthStatus,
    WorkerInfo,
)

__all__ = [
    "ControlPlaneClient",
    "ControlPlaneError",
    "AckRequest",
    "CancelCommand",
    "DomainInvalidation",
    "DrainCommand",
    "HeartbeatRequest",
    "HeartbeatResponse",
    "NackRequest",
    "Observation",
    "OutboundMessage",
    "PingCommand",
    "RegisterResponse",
    "ResourceCapacity",
    "ResourceUsage",
    "ServerEvent",
    "SessionAssignment",
    "SessionFinalResult",
    "SessionStatusUpdate",
    "WorkerHealth",
    "WorkerHealthStatus",
    "WorkerInfo",
]