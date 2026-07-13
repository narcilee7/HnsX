"""``ControlPlaneClient`` — the runtime's only entry point to the wire.

This module owns every reference to the generated proto stubs. Runtime code
should depend on ``ControlPlaneClient`` and the dataclasses in ``messages``,
not on ``worker_pb2`` / ``observation_pb2``.

Mapping rule:

  - Inputs   — accept dataclasses (and a few primitives); translate to proto
                on the call site.
  - Outputs  — return dataclasses; never expose proto types.
  - Stream   — accept ``Iterable[OutboundMessage]`` outbound and yield
                ``ServerEvent`` inbound.
"""

from __future__ import annotations

import json
import logging
from collections.abc import Callable, Iterable, Iterator
from typing import Any

import grpc

from hnsx_worker.proto.gen.hnsx.v1 import (
    control_plane_pb2,
    control_plane_pb2_grpc,
    observation_pb2,
    worker_pb2,
    worker_pb2_grpc,
)

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

log = logging.getLogger("hnsx_worker.proto_client")


# Map our string health status → proto enum int.
_HEALTH_TO_PROTO: dict[str, int] = {
    WorkerHealthStatus.UNSPECIFIED: worker_pb2.WorkerHealth.STATUS_UNSPECIFIED,
    WorkerHealthStatus.HEALTHY: worker_pb2.WorkerHealth.STATUS_HEALTHY,
    WorkerHealthStatus.DEGRADED: worker_pb2.WorkerHealth.STATUS_DEGRADED,
    WorkerHealthStatus.DRAINING: worker_pb2.WorkerHealth.STATUS_DRAINING,
}


class ControlPlaneClient:
    """Thin wrapper around the ``WorkerService`` and ``SchedulerService`` stubs.

    Lifecycle::

        client = ControlPlaneClient("127.0.0.1:50061")
        resp = client.register(worker_info)
        try:
            while True:
                sess = client.pull_session(worker_id=resp.worker_id, max_wait_seconds=30)
                if sess.is_empty():
                    continue
                client.ack_session(AckRequest(...))
                for event in client.open_stream(outbound_iter()):
                    ...
        finally:
            client.close()
    """

    def __init__(self, server_addr: str, *, auth_token: str = "") -> None:
        self._server_addr = server_addr
        self._auth_token = auth_token
        self._channel = grpc.insecure_channel(server_addr)
        self._worker = worker_pb2_grpc.WorkerServiceStub(self._channel)
        self._scheduler = worker_pb2_grpc.SchedulerServiceStub(self._channel)
        self._domain_registry = control_plane_pb2_grpc.DomainRegistryServiceStub(self._channel)

    # ------------------------------------------------------------------ lifecycle

    def close(self) -> None:
        try:
            self._channel.close()
        except Exception:  # noqa: BLE001
            pass

    # ------------------------------------------------------------------ unary RPCs

    def register(self, info: WorkerInfo, *, timeout: float = 10.0) -> RegisterResponse:
        """WorkerService.Register."""
        proto_info = worker_pb2.WorkerInfo(
            worker_id=info.worker_id,
            tenant_id=info.tenant_id,
            version=info.version,
            region=info.region,
            hostname=info.hostname,
            pid=info.pid,
            capacity=_to_proto_capacity(info.capacity),
        )
        info.labels and proto_info.labels.update(info.labels)
        req = worker_pb2.RegisterRequest(info=proto_info, auth_token=self._auth_token)
        resp = self._worker.Register(req, timeout=timeout)
        return RegisterResponse(
            worker_id=resp.worker_id,
            server_time_ms=int(resp.server_time_ms),
            heartbeat_interval_seconds=int(resp.heartbeat_interval_seconds),
        )

    def heartbeat(self, req: HeartbeatRequest, *, timeout: float = 5.0) -> HeartbeatResponse:
        """WorkerService.Heartbeat."""
        proto_req = worker_pb2.HeartbeatRequest(
            worker_id=req.worker_id,
            timestamp_ms=int(req.timestamp_ms),
            usage=worker_pb2.ResourceUsage(
                running_sessions=req.usage.running_sessions,
                free_slots=req.usage.free_slots,
                cpu_percent=req.usage.cpu_percent,
                memory_used_bytes=req.usage.memory_used_bytes,
                uptime_seconds=req.usage.uptime_seconds,
            ),
            running_session_ids=list(req.running_session_ids),
            health=worker_pb2.WorkerHealth(
                status=_HEALTH_TO_PROTO.get(
                    req.health.status, worker_pb2.WorkerHealth.STATUS_UNSPECIFIED
                ),
                message=req.health.message,
                last_event_ms=int(req.health.last_event_ms),
            ),
        )
        resp = self._worker.Heartbeat(proto_req, timeout=timeout)
        return HeartbeatResponse(
            server_time_ms=int(resp.server_time_ms),
            drain_deadline_seconds=int(resp.drain.deadline_seconds),
            drain_reason=resp.drain.reason,
        )

    def pull_session(
        self,
        *,
        worker_id: str,
        max_wait_seconds: int = 30,
        required_capabilities: Iterable[str] = (),
        timeout: float | None = None,
    ) -> SessionAssignment:
        """SchedulerService.PullSession.

        Returns a SessionAssignment with ``session_id == ""`` when the server
        has no work for us. Callers should treat that as "loop again".
        """
        req = worker_pb2.PullSessionRequest(
            worker_id=worker_id,
            max_wait_seconds=int(max_wait_seconds),
            required_capabilities=list(required_capabilities),
        )
        if timeout is None:
            timeout = max_wait_seconds + 10.0
        resp = self._scheduler.PullSession(req, timeout=timeout)
        return SessionAssignment(
            session_id=resp.session_id,
            domain_id=resp.domain_id,
            domain_version=resp.domain_version,
            domain_spec_json=resp.domain_spec_json,
            trigger_payload_json=resp.trigger_payload_json,
            trace_id=resp.trace_id,
            correlation_id=resp.correlation_id,
            assigned_at_ms=int(resp.assigned_at_ms),
            session_timeout_seconds=int(resp.session_timeout_seconds),
        )

    def ack_session(self, req: AckRequest, *, timeout: float = 5.0) -> None:
        """SchedulerService.AckSession."""
        proto_req = worker_pb2.AckSessionRequest(
            worker_id=req.worker_id,
            session_id=req.session_id,
            acknowledged_at_ms=int(req.acknowledged_at_ms),
            correlation_id=req.correlation_id,
        )
        self._scheduler.AckSession(proto_req, timeout=timeout)

    def nack_session(self, req: NackRequest, *, timeout: float = 5.0) -> None:
        """SchedulerService.NackSession."""
        proto_req = worker_pb2.NackSessionRequest(
            worker_id=req.worker_id,
            session_id=req.session_id,
            reason=req.reason,
            error_code=req.error_code,
            requeue=req.requeue,
        )
        self._scheduler.NackSession(proto_req, timeout=timeout)

    def validate_domain(self, domain_spec_json: str, *, timeout: float = 10.0) -> tuple[bool, list[str]]:
        """DomainRegistryService.ValidateDomain.

        Returns ``(valid, error_messages)``. An unreachable server is reported
        as ``(False, ["..."])`` so the caller can decide whether to fail hard.
        """
        proto_req = control_plane_pb2.ValidateDomainRequest(
            domain_spec_json=domain_spec_json
        )
        try:
            resp = self._domain_registry.ValidateDomain(proto_req, timeout=timeout)
        except grpc.RpcError as exc:
            return False, [f"ValidateDomain RPC failed: {exc.code()}: {exc.details() or exc}"]
        except Exception as exc:  # noqa: BLE001
            return False, [f"ValidateDomain RPC failed: {exc}"]
        if resp.valid:
            return True, []
        return False, [f"{err.field}: {err.message}" for err in resp.errors]

    # ------------------------------------------------------------------ bidi stream

    def open_stream(
        self,
        outbound: Iterable[OutboundMessage],
        *,
        timeout: float | None = None,
    ) -> Iterator[ServerEvent]:
        """Open the bidi ``StreamChannel`` and yield ``ServerEvent`` values.

        ``outbound`` is drained in the same thread that consumes the response
        (gRPC's client-streaming contract). The returned iterator is
        single-pass.
        """
        proto_iter: Iterable[worker_pb2.StreamChannelRequest] = (
            self._build_stream_request(msg) for msg in outbound
        )
        stream = self._scheduler.StreamChannel(proto_iter, timeout=timeout)
        for proto_event in stream:
            yield _from_proto_stream_response(proto_event)

    # ------------------------------------------------------------------ internals

    def _build_stream_request(self, msg: OutboundMessage) -> worker_pb2.StreamChannelRequest:
        req = worker_pb2.StreamChannelRequest()
        if msg.kind == "observations" and msg.observations:
            req.observations.CopyFrom(
                worker_pb2.ObservationBatch(
                    observations=[_to_proto_observation(o) for o in msg.observations]
                )
            )
        elif msg.kind == "status" and msg.status is not None:
            req.status.CopyFrom(_to_proto_status(msg.status))
        elif msg.kind == "result" and msg.result is not None:
            req.result.CopyFrom(_to_proto_result(msg.result))
        return req


# ---------------------------------------------------------------------------
# proto ⇄ dataclass helpers (private — module-local so they don't leak)
# ---------------------------------------------------------------------------


def _to_proto_capacity(c: ResourceCapacity) -> worker_pb2.ResourceCapacity:
    out = worker_pb2.ResourceCapacity(
        max_concurrent_sessions=int(c.max_concurrent_sessions),
        sandbox_runtimes=list(c.sandbox_runtimes),
        memory_total_bytes=int(c.memory_total_bytes),
        cpu_total_cores=float(c.cpu_total_cores),
    )
    out.providers.extend(c.providers)
    out.models.extend(c.models)
    return out


def _to_proto_observation(o: Observation) -> observation_pb2.Observation:
    return observation_pb2.Observation(
        session_id=o.session_id,
        domain_id=o.domain_id,
        domain_version=o.domain_version,
        step_id=o.step_id,
        agent_id=o.agent_id,
        parent_id=o.parent_id,
        kind=o.kind,
        role=o.role,
        payload=json.dumps(o.payload, ensure_ascii=False, default=str),
        metadata=json.dumps(o.metadata, ensure_ascii=False, default=str),
        trace_id=o.trace_id,
        created_at_ms=int(o.created_at_ms),
    )


def _to_proto_status(s: SessionStatusUpdate) -> worker_pb2.SessionStatusUpdate:
    return worker_pb2.SessionStatusUpdate(
        session_id=s.session_id,
        state=s.state,
        message=s.message,
        timestamp_ms=int(s.timestamp_ms),
    )


def _to_proto_result(r: SessionFinalResult) -> worker_pb2.SessionFinalResult:
    return worker_pb2.SessionFinalResult(
        session_id=r.session_id,
        result_json=json.dumps(r.result, ensure_ascii=False, default=str),
        total_cost_usd=float(r.total_cost_usd),
        total_prompt_tokens=int(r.total_prompt_tokens),
        total_completion_tokens=int(r.total_completion_tokens),
        duration_ms=int(r.duration_ms),
    )


def _from_proto_stream_response(proto: worker_pb2.StreamChannelResponse) -> ServerEvent:
    """Translate one ``StreamChannelResponse`` into a tagged ``ServerEvent``."""
    which = proto.WhichOneof("payload")
    if which == "cancel":
        return ServerEvent(
            kind="cancel",
            correlation_id=proto.correlation_id,
            cancel=CancelCommand(
                session_id=proto.cancel.session_id,
                correlation_id=proto.correlation_id,
                reason=proto.cancel.reason,
                deadline_ms=int(proto.cancel.deadline_ms),
            ),
        )
    if which == "drain":
        return ServerEvent(
            kind="drain",
            drain=DrainCommand(
                deadline_seconds=int(proto.drain.deadline_seconds),
                reason=proto.drain.reason,
            ),
        )
    if which == "invalidate":
        return ServerEvent(
            kind="invalidate",
            invalidate=DomainInvalidation(
                domain_id=proto.invalidate.domain_id,
                version=proto.invalidate.version,
            ),
        )
    if which == "ping":
        return ServerEvent(
            kind="ping",
            ping=PingCommand(timestamp_ms=int(proto.ping.timestamp_ms)),
        )
    return ServerEvent(kind="unknown")


# Re-exported so callers can build messages inline.
__all__ = [
    "ControlPlaneClient",
    "ControlPlaneError",
    "RpcUnavailable",
    "WorkerInfo",
    "WorkerHealth",
    "WorkerHealthStatus",
    "ResourceCapacity",
    "ResourceUsage",
    "HeartbeatRequest",
    "RegisterResponse",
    "HeartbeatResponse",
    "SessionAssignment",
    "AckRequest",
    "NackRequest",
    "Observation",
    "SessionStatusUpdate",
    "SessionFinalResult",
    "OutboundMessage",
    "ServerEvent",
    "CancelCommand",
    "DrainCommand",
    "DomainInvalidation",
    "PingCommand",
]


def _silence_unused() -> Callable[[Any], Any]:
    """Keeps ruff quiet about future imports."""
    return lambda x: x