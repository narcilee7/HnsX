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
from google.protobuf.json_format import MessageToDict, ParseDict

from hnsx_worker.proto.gen.hnsx.v1 import (
    control_plane_pb2,
    control_plane_pb2_grpc,
    domain_pb2,
    observation_pb2,
    worker_pb2,
    worker_pb2_grpc,
)

from .messages import (
    AckRequest,
    CancelCommand,
    ControlPlaneError,
    DomainInvalidation,
    DomainRef,
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


# ---------------------------------------------------------------------------
# gRPC interceptor — inject static metadata (x-tenant-id, x-api-key) on every
# outgoing RPC. Connect handlers on the server read tenant via metadata; the
# REST/Connect auth split lives in pkg/auth (REST only). Once a Connect-side
# auth interceptor lands server-side, the same `x-api-key` header will be
# honored without further client changes.
# ---------------------------------------------------------------------------


class _MetadataClientInterceptor(
    grpc.UnaryUnaryClientInterceptor,
    grpc.UnaryStreamClientInterceptor,
    grpc.StreamUnaryClientInterceptor,
    grpc.StreamStreamClientInterceptor,
):
    """Inject a fixed set of ``(key, value)`` metadata pairs on every RPC."""

    def __init__(self, static_metadata: tuple[tuple[str, str], ...] = ()):
        self._static_metadata = static_metadata

    def _inject(self, client_call_details):
        existing = list(client_call_details.metadata or ())
        return client_call_details._replace(metadata=existing + list(self._static_metadata))

    def intercept_unary_unary(self, continuation, client_call_details, request):
        return continuation(self._inject(client_call_details), request)

    def intercept_unary_stream(self, continuation, client_call_details, request):
        return continuation(self._inject(client_call_details), request)

    def intercept_stream_unary(self, continuation, client_call_details, request_iterator):
        return continuation(self._inject(client_call_details), request_iterator)

    def intercept_stream_stream(self, continuation, client_call_details, request_iterator):
        return continuation(self._inject(client_call_details), request_iterator)


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

    def __init__(
        self,
        server_addr: str,
        *,
        auth_token: str = "",
        tenant_id: str = "",
        api_key: str = "",
    ) -> None:
        """Connect to the control plane at ``server_addr``.

        Args:
            server_addr: ``host:port`` of the gRPC endpoint (not REST). Workers
                historically use ``127.0.0.1:50061``.
            auth_token: Legacy token field carried on ``WorkerService.Register``
                payloads. New callers should prefer ``api_key`` (sent as
                ``x-api-key`` metadata).
            tenant_id: Sent as ``x-tenant-id`` metadata on every RPC. Connect
                handlers scope reads/writes by tenant.
            api_key: Sent as ``x-api-key`` metadata on every RPC. Not yet
                enforced server-side on the Connect mux, but kept here so the
                wire shape is ready when the Connect auth interceptor lands.
        """
        self._server_addr = server_addr
        self._auth_token = auth_token

        static_md: list[tuple[str, str]] = []
        if tenant_id:
            static_md.append(("x-tenant-id", tenant_id))
        if api_key:
            static_md.append(("x-api-key", api_key))

        raw_channel = grpc.insecure_channel(server_addr)
        if static_md:
            self._channel = grpc.intercept_channel(
                raw_channel, _MetadataClientInterceptor(tuple(static_md))
            )
        else:
            self._channel = raw_channel

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

    # ------------------------------------------------------------------ DomainRegistry

    def register_domain(
        self,
        spec: dict[str, Any] | str,
        *,
        timeout: float = 10.0,
    ) -> DomainRef:
        """DomainRegistryService.RegisterDomain.

        Args:
            spec: Authoring-format spec as a Python ``dict`` (will be
                JSON-encoded here) or a raw JSON string. The dict shape
                mirrors the YAML/JSON the CLI's ``hnsx domain apply`` accepts:
                ``{id, version, description, harness:{...}, eval:{...}}``.

        Returns:
            ``DomainRef(id, version)`` as assigned by the server.

        Raises:
            ControlPlaneError: on RPC failure or invalid JSON.
        """
        if isinstance(spec, str):
            try:
                spec_dict = json.loads(spec)
            except json.JSONDecodeError as exc:
                raise ControlPlaneError(f"register_domain: invalid JSON: {exc}") from exc
        else:
            spec_dict = spec

        # Translate the dict to the proto using the standard JSON mapping.
        # This honors nested harness / eval fields automatically and stays in
        # lockstep with the .proto definitions.
        proto_spec = ParseDict(spec_dict, domain_pb2.DomainSpec(), ignore_unknown_fields=True)
        proto_req = control_plane_pb2.RegisterDomainRequest(spec=proto_spec)

        try:
            resp = self._domain_registry.RegisterDomain(proto_req, timeout=timeout)
        except grpc.RpcError as exc:
            raise ControlPlaneError(
                f"RegisterDomain RPC failed: {exc.code()}: {exc.details() or exc}"
            ) from exc
        return DomainRef(id=resp.domain.id, version=resp.domain.version)

    def unregister_domain(
        self,
        domain_id: str,
        version: str = "",
        *,
        timeout: float = 10.0,
    ) -> None:
        """DomainRegistryService.UnregisterDomain.

        ``version`` is optional; pass ``""`` to unregister all versions of
        the domain. The server interprets an empty version field as "any".
        """
        proto_req = control_plane_pb2.UnregisterDomainRequest(
            domain=domain_pb2.DomainRef(id=domain_id, version=version)
        )
        try:
            self._domain_registry.UnregisterDomain(proto_req, timeout=timeout)
        except grpc.RpcError as exc:
            raise ControlPlaneError(
                f"UnregisterDomain RPC failed: {exc.code()}: {exc.details() or exc}"
            ) from exc

    def get_domain(
        self,
        domain_id: str,
        version: str = "",
        *,
        timeout: float = 10.0,
    ) -> dict[str, Any]:
        """DomainRegistryService.GetDomain.

        Returns the spec as a Python dict (parsed from the proto). ``version``
        empty means "latest".
        """
        proto_req = control_plane_pb2.GetDomainRequest(
            domain=domain_pb2.DomainRef(id=domain_id, version=version)
        )
        try:
            resp = self._domain_registry.GetDomain(proto_req, timeout=timeout)
        except grpc.RpcError as exc:
            raise ControlPlaneError(
                f"GetDomain RPC failed: {exc.code()}: {exc.details() or exc}"
            ) from exc
        spec = resp.spec
        return {
            "id": spec.id,
            "version": spec.version,
            "description": spec.description,
            "harness": _harness_proto_to_dict(spec.harness),
            "eval": _eval_proto_to_dict(spec.eval) if spec.HasField("eval") else {},
        }

    def list_domains(
        self,
        *,
        limit: int = 50,
        offset: int = 0,
        timeout: float = 10.0,
    ) -> list[dict[str, Any]]:
        """DomainRegistryService.ListDomains.

        Returns the page of spec dicts. ``total`` is not returned — call again
        with a higher ``offset`` until the response is shorter than ``limit``.
        """
        proto_req = control_plane_pb2.ListDomainsRequest(
            limit=int(limit), offset=int(offset)
        )
        try:
            resp = self._domain_registry.ListDomains(proto_req, timeout=timeout)
        except grpc.RpcError as exc:
            raise ControlPlaneError(
                f"ListDomains RPC failed: {exc.code()}: {exc.details() or exc}"
            ) from exc
        return [
            {
                "id": s.id,
                "version": s.version,
                "description": s.description,
                "harness": _harness_proto_to_dict(s.harness),
            }
            for s in resp.domains
        ]

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


def _harness_proto_to_dict(harness_proto) -> dict[str, Any]:
    """Convert a Harness proto to a plain dict via protobuf's standard helper.

    We use ``MessageToDict`` rather than hand-rolling each nested type so we
    stay in lockstep with the .proto definitions. ``including_default_value_fields``
    is left off (default) to keep the dict readable — empty fields stay empty.
    """
    return MessageToDict(
        harness_proto,
        preserving_proto_field_name=True,
        use_integers_for_enums=True,
    )


def _eval_proto_to_dict(eval_proto) -> dict[str, Any]:
    return MessageToDict(
        eval_proto,
        preserving_proto_field_name=True,
        use_integers_for_enums=True,
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
    "DomainRef",
    "PingCommand",
]


def _silence_unused() -> Callable[[Any], Any]:
    """Keeps ruff quiet about future imports."""
    return lambda x: x