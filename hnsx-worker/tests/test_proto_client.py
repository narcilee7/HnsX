"""Direct tests for ``ControlPlaneClient``.

These tests don't go through ``WorkerService``; they exercise the proto
client directly against an in-process mock gRPC server. The goal is to
verify the wire ↔ dataclass mapping without dragging in subprocess /
worker loop concerns.
"""

from __future__ import annotations

import json
import threading
import time

import grpc
import pytest

from hnsx_worker.proto.gen.hnsx.v1 import (
    control_plane_pb2,
    control_plane_pb2_grpc,
    domain_pb2,
    observation_pb2,
    worker_pb2,
    worker_pb2_grpc,
)
from hnsx_worker.proto_client import (
    AckRequest,
    ControlPlaneClient,
    DomainRef,
    HeartbeatRequest,
    NackRequest,
    Observation,
    OutboundMessage,
    RegisterResponse,
    ResourceCapacity,
    ResourceUsage,
    ServerEvent,
    SessionAssignment,
    SessionStatusUpdate,
    WorkerHealth,
    WorkerHealthStatus,
    WorkerInfo,
)
from hnsx_worker.proto_client.client import ControlPlaneError


# ---------------------------------------------------------------------------
# in-process mock control plane
# ---------------------------------------------------------------------------


class _MockControlPlane:
    """In-process gRPC server that captures every RPC for inspection."""

    def __init__(self) -> None:
        self.registers: list[worker_pb2.RegisterRequest] = []
        self.heartbeats: list[worker_pb2.HeartbeatRequest] = []
        self.pulls: list[worker_pb2.PullSessionRequest] = []
        self.acks: list[worker_pb2.AckSessionRequest] = []
        self.nacks: list[worker_pb2.NackSessionRequest] = []
        self.stream_requests: list[worker_pb2.StreamChannelRequest] = []
        # DomainRegistry traffic (mirrors mock_server.py's servicer).
        self.registered_domains: list[domain_pb2.DomainSpec] = []
        self.unregister_calls: list[domain_pb2.DomainRef] = []
        self.get_domain_calls: list[domain_pb2.DomainRef] = []
        self.list_domain_calls: list[control_plane_pb2.ListDomainsRequest] = []
        # Most recent gRPC metadata received on RegisterDomain — used by the
        # auth-metadata tests to assert interceptor behavior.
        self.last_register_metadata: list[tuple[str, str]] | None = None
        # Next pull-session response (overridable per-test).
        self.next_pull = worker_pb2.PullSessionResponse()
        # Cancel command to push down the bidi stream (overridable).
        self.inbound_cancel: worker_pb2.StreamChannelResponse | None = None
        self._server: grpc.Server | None = None
        self.addr: str = ""
        self._lock = threading.Lock()
        self._domain_store: dict[tuple[str, str], domain_pb2.DomainSpec] = {}

    def start(self) -> None:
        server = grpc.server(__import__("concurrent.futures", fromlist=["ThreadPoolExecutor"]).ThreadPoolExecutor(max_workers=8))
        worker_pb2_grpc.add_WorkerServiceServicer_to_server(_WorkerServicer(self), server)
        worker_pb2_grpc.add_SchedulerServiceServicer_to_server(_SchedulerServicer(self), server)
        control_plane_pb2_grpc.add_DomainRegistryServiceServicer_to_server(
            _DomainRegistryServicer(self), server
        )
        bound = server.add_insecure_port("[::]:0")
        server.start()
        self._server = server
        self.addr = f"127.0.0.1:{bound}"

    def stop(self) -> None:
        if self._server is not None:
            self._server.stop(2.0).wait()
            self._server = None


class _WorkerServicer(worker_pb2_grpc.WorkerServiceServicer):
    def __init__(self, m: _MockControlPlane) -> None:
        self.m = m

    def Register(self, request, context):  # noqa: ARG002
        self.m.registers.append(request)
        return worker_pb2.RegisterResponse(
            worker_id=request.info.worker_id or "w-mock-1",
            server_time_ms=int(time.time() * 1000),
            heartbeat_interval_seconds=5,
        )

    def Heartbeat(self, request, context):  # noqa: ARG002
        self.m.heartbeats.append(request)
        return worker_pb2.HeartbeatResponse(
            server_time_ms=int(time.time() * 1000),
            drain=worker_pb2.DrainHint(deadline_seconds=60, reason="maintenance"),
        )


class _SchedulerServicer(worker_pb2_grpc.SchedulerServiceServicer):
    def __init__(self, m: _MockControlPlane) -> None:
        self.m = m

    def PullSession(self, request, context):  # noqa: ARG002
        self.m.pulls.append(request)
        return self.m.next_pull

    def AckSession(self, request, context):  # noqa: ARG002
        self.m.acks.append(request)
        return worker_pb2.AckSessionResponse()

    def NackSession(self, request, context):  # noqa: ARG002
        self.m.nacks.append(request)
        return worker_pb2.NackSessionResponse()

    def StreamChannel(self, request_iterator, context):  # noqa: ARG002
        # Push the cancel only after we've observed at least 2 outbound
        # requests, so the worker's queued batch has time to flush.
        for req in request_iterator:
            with self.m._lock:
                self.m.stream_requests.append(req)
                seen_count = len(self.m.stream_requests)
            if self.m.inbound_cancel is not None and seen_count >= 2:
                yield self.m.inbound_cancel
                return


class _DomainRegistryServicer(control_plane_pb2_grpc.DomainRegistryServiceServicer):
    def __init__(self, m: _MockControlPlane) -> None:
        self.m = m

    def RegisterDomain(self, request, context):
        self.m.last_register_metadata = list(context.invocation_metadata())
        spec = request.spec
        self.m.registered_domains.append(spec)
        self.m._domain_store[(spec.id, spec.version)] = spec
        return control_plane_pb2.RegisterDomainResponse(
            domain=domain_pb2.DomainRef(id=spec.id, version=spec.version)
        )

    def UnregisterDomain(self, request, context):  # noqa: ARG002
        self.m.unregister_calls.append(request.domain)
        d = request.domain
        if d.version:
            self.m._domain_store.pop((d.id, d.version), None)
        else:
            for k in [k for k in self.m._domain_store if k[0] == d.id]:
                self.m._domain_store.pop(k, None)
        return control_plane_pb2.UnregisterDomainResponse()

    def GetDomain(self, request, context):
        self.m.get_domain_calls.append(request.domain)
        d = request.domain
        if d.version:
            spec = self.m._domain_store.get((d.id, d.version))
            if spec is None:
                context.abort(
                    grpc.StatusCode.NOT_FOUND,
                    f"domain {d.id}@{d.version} not found",
                )
            return control_plane_pb2.GetDomainResponse(spec=spec)
        versions = [v for (i, v) in self.m._domain_store if i == d.id]
        if not versions:
            context.abort(grpc.StatusCode.NOT_FOUND, f"domain {d.id} not found")
        return control_plane_pb2.GetDomainResponse(
            spec=self.m._domain_store[(d.id, sorted(versions)[-1])]
        )

    def ListDomains(self, request, context):  # noqa: ARG002
        self.m.list_domain_calls.append(request)
        limit = request.limit if request.limit > 0 else 50
        offset = request.offset if request.offset > 0 else 0
        all_specs = list(self.m._domain_store.values())
        return control_plane_pb2.ListDomainsResponse(
            domains=all_specs[offset : offset + limit],
            total=len(all_specs),
        )

    def ValidateDomain(self, request, context):  # noqa: ARG002
        return control_plane_pb2.ValidateDomainResponse(valid=True)


# ---------------------------------------------------------------------------
# fixtures
# ---------------------------------------------------------------------------


@pytest.fixture
def mock_plane():
    mp = _MockControlPlane()
    mp.start()
    try:
        yield mp
    finally:
        mp.stop()


# ---------------------------------------------------------------------------
# register / heartbeat
# ---------------------------------------------------------------------------


def test_register_translates_worker_info_to_proto(mock_plane: _MockControlPlane) -> None:
    client = ControlPlaneClient(mock_plane.addr)
    try:
        info = WorkerInfo(
            worker_id="w-cli",
            tenant_id="t1",
            version="0.2.0",
            region="local",
            hostname="h",
            pid="123",
            capacity=ResourceCapacity(
                max_concurrent_sessions=4,
                providers=["anthropic", "openai"],
                models=["claude-haiku-4-5"],
            ),
        )
        resp = client.register(info)

        assert isinstance(resp, RegisterResponse)
        assert resp.worker_id == "w-cli"
        assert resp.heartbeat_interval_seconds == 5
        assert len(mock_plane.registers) == 1
        sent = mock_plane.registers[0].info
        assert sent.worker_id == "w-cli"
        assert sent.capacity.max_concurrent_sessions == 4
        assert list(sent.capacity.providers) == ["anthropic", "openai"]
        assert list(sent.capacity.models) == ["claude-haiku-4-5"]
    finally:
        client.close()


def test_heartbeat_translates_request(mock_plane: _MockControlPlane) -> None:
    client = ControlPlaneClient(mock_plane.addr)
    try:
        req = HeartbeatRequest(
            worker_id="w-cli",
            timestamp_ms=12345,
            usage=ResourceUsage(running_sessions=2, free_slots=2, cpu_percent=12.5),
            running_session_ids=["s1", "s2"],
            health=WorkerHealth(status=WorkerHealthStatus.HEALTHY, message="ok"),
        )
        resp = client.heartbeat(req)
        assert resp.server_time_ms > 0
        assert resp.drain_deadline_seconds == 60
        assert resp.drain_reason == "maintenance"

        sent = mock_plane.heartbeats[0]
        assert sent.worker_id == "w-cli"
        assert sent.timestamp_ms == 12345
        assert sent.usage.running_sessions == 2
        assert sent.usage.cpu_percent == 12.5
        assert list(sent.running_session_ids) == ["s1", "s2"]
        assert sent.health.status == worker_pb2.WorkerHealth.STATUS_HEALTHY
    finally:
        client.close()


# ---------------------------------------------------------------------------
# pull / ack / nack
# ---------------------------------------------------------------------------


def test_pull_session_returns_dataclass(mock_plane: _MockControlPlane) -> None:
    mock_plane.next_pull = worker_pb2.PullSessionResponse(
        session_id="s-1",
        domain_id="d-1",
        domain_version="0.2.0",
        domain_spec_json='{"id":"d-1"}',
        trigger_payload_json='{"q":"hi"}',
        trace_id="t-1",
        correlation_id="c-1",
        assigned_at_ms=999,
        session_timeout_seconds=60,
    )
    client = ControlPlaneClient(mock_plane.addr)
    try:
        sess = client.pull_session(worker_id="w-cli", max_wait_seconds=2)
        assert isinstance(sess, SessionAssignment)
        assert sess.session_id == "s-1"
        assert sess.domain_id == "d-1"
        assert sess.assigned_at_ms == 999
        assert not sess.is_empty()
    finally:
        client.close()


def test_pull_session_empty_response(mock_plane: _MockControlPlane) -> None:
    mock_plane.next_pull = worker_pb2.PullSessionResponse()
    client = ControlPlaneClient(mock_plane.addr)
    try:
        sess = client.pull_session(worker_id="w-cli", max_wait_seconds=1)
        assert sess.is_empty()
        assert sess.session_id == ""
    finally:
        client.close()


def test_ack_session_roundtrip(mock_plane: _MockControlPlane) -> None:
    client = ControlPlaneClient(mock_plane.addr)
    try:
        client.ack_session(
            AckRequest(
                worker_id="w-cli",
                session_id="s-1",
                correlation_id="c-1",
                acknowledged_at_ms=1234,
            )
        )
        sent = mock_plane.acks[0]
        assert sent.worker_id == "w-cli"
        assert sent.session_id == "s-1"
        assert sent.acknowledged_at_ms == 1234
    finally:
        client.close()


def test_nack_session_roundtrip(mock_plane: _MockControlPlane) -> None:
    client = ControlPlaneClient(mock_plane.addr)
    try:
        client.nack_session(
            NackRequest(
                worker_id="w-cli",
                session_id="s-1",
                reason="no_free_slots",
                error_code="CAPACITY",
                requeue=True,
            )
        )
        sent = mock_plane.nacks[0]
        assert sent.error_code == "CAPACITY"
        assert sent.requeue is True
    finally:
        client.close()


# ---------------------------------------------------------------------------
# bidi stream
# ---------------------------------------------------------------------------


def test_stream_observation_roundtrip(mock_plane: _MockControlPlane) -> None:
    mock_plane.inbound_cancel = worker_pb2.StreamChannelResponse(
        correlation_id="c-1",
        cancel=worker_pb2.CancelSessionCommand(
            session_id="s-1", reason="user_cancel", deadline_ms=999,
        ),
    )
    client = ControlPlaneClient(mock_plane.addr)
    try:

        def outbound():
            yield OutboundMessage(
                kind="observations",
                observations=[
                    Observation(
                        session_id="s-1",
                        domain_id="d-1",
                        kind="agent_text_delta",
                        payload={"content": "hi"},
                        created_at_ms=int(time.time() * 1000),
                    )
                ],
            )
            yield OutboundMessage(
                kind="status",
                status=SessionStatusUpdate(
                    session_id="s-1", state="completed", message="done",
                    timestamp_ms=int(time.time() * 1000),
                ),
            )

        events: list[ServerEvent] = list(client.open_stream(outbound()))
        assert len(events) == 1
        evt = events[0]
        assert evt.kind == "cancel"
        assert evt.correlation_id == "c-1"
        assert evt.cancel is not None
        assert evt.cancel.session_id == "s-1"
        assert evt.cancel.reason == "user_cancel"
        assert evt.cancel.deadline_ms == 999

        # Both outbound messages were serialized correctly before the cancel
        # tore the stream down.
        assert len(mock_plane.stream_requests) == 2
        first = mock_plane.stream_requests[0]
        assert first.WhichOneof("payload") == "observations"
        assert len(first.observations.observations) == 1
        proto_obs = first.observations.observations[0]
        assert proto_obs.session_id == "s-1"
        assert proto_obs.kind == "agent_text_delta"
        # Payload dict was serialized to JSON.
        assert json.loads(proto_obs.payload) == {"content": "hi"}

        second = mock_plane.stream_requests[1]
        assert second.WhichOneof("payload") == "status"
        assert second.status.session_id == "s-1"
        assert second.status.state == "completed"
    finally:
        client.close()


def test_observation_serializes_payload_dict() -> None:
    """Smoke test for ``Observation`` payload → JSON → ``Observation.payload``."""
    obs = Observation(
        session_id="s",
        kind="x",
        payload={"a": 1, "b": ["c", "d"], "e": {"f": True}},
    )
    client = ControlPlaneClient("127.0.0.1:1")  # never connects
    proto = client._build_stream_request(  # noqa: SLF001 — internal but stable
        OutboundMessage(kind="observations", observations=[obs])
    )
    assert proto.WhichOneof("payload") == "observations"
    sent = proto.observations.observations[0]
    assert json.loads(sent.payload) == {"a": 1, "b": ["c", "d"], "e": {"f": True}}
    client.close()


# ---------------------------------------------------------------------------
# DomainRegistry — RegisterDomain / UnregisterDomain / GetDomain / ListDomains
# ---------------------------------------------------------------------------


def test_register_domain_accepts_dict(mock_plane: _MockControlPlane) -> None:
    client = ControlPlaneClient(mock_plane.addr)
    try:
        ref = client.register_domain(
            {
                "id": "demo",
                "version": "1.0.0",
                "description": "from python",
                "harness": {"agents": []},
            }
        )
        assert isinstance(ref, DomainRef)
        assert ref.id == "demo"
        assert ref.version == "1.0.0"
        assert len(mock_plane.registered_domains) == 1
        assert mock_plane.registered_domains[0].id == "demo"
    finally:
        client.close()


def test_register_domain_accepts_json_string(mock_plane: _MockControlPlane) -> None:
    client = ControlPlaneClient(mock_plane.addr)
    try:
        ref = client.register_domain('{"id":"s","version":"0.1.0"}')
        assert ref == DomainRef(id="s", version="0.1.0")
    finally:
        client.close()


def test_register_domain_rejects_invalid_json(mock_plane: _MockControlPlane) -> None:
    client = ControlPlaneClient(mock_plane.addr)
    try:
        with pytest.raises(ControlPlaneError, match="invalid JSON"):
            client.register_domain("{not json")
    finally:
        client.close()


def test_unregister_domain_specific_version(mock_plane: _MockControlPlane) -> None:
    client = ControlPlaneClient(mock_plane.addr)
    try:
        client.register_domain({"id": "d", "version": "1.0.0"})
        client.register_domain({"id": "d", "version": "2.0.0"})
        client.unregister_domain("d", "1.0.0")
        # only v2.0.0 should remain
        assert ("d", "1.0.0") not in mock_plane._domain_store
        assert ("d", "2.0.0") in mock_plane._domain_store
        assert mock_plane.unregister_calls[-1] == domain_pb2.DomainRef(
            id="d", version="1.0.0"
        )
    finally:
        client.close()


def test_unregister_domain_all_versions_when_no_version(
    mock_plane: _MockControlPlane,
) -> None:
    client = ControlPlaneClient(mock_plane.addr)
    try:
        client.register_domain({"id": "d", "version": "1.0.0"})
        client.register_domain({"id": "d", "version": "2.0.0"})
        client.register_domain({"id": "other", "version": "1.0.0"})
        client.unregister_domain("d")  # empty version → wipe all
        assert all(k[0] != "d" for k in mock_plane._domain_store)
        assert ("other", "1.0.0") in mock_plane._domain_store
    finally:
        client.close()


def test_get_domain_returns_dict(mock_plane: _MockControlPlane) -> None:
    client = ControlPlaneClient(mock_plane.addr)
    try:
        client.register_domain(
            {
                "id": "demo",
                "version": "1.0.0",
                "description": "hello",
                "harness": {"agents": [{"id": "main"}]},
            }
        )
        spec = client.get_domain("demo", "1.0.0")
        assert spec["id"] == "demo"
        assert spec["version"] == "1.0.0"
        assert spec["description"] == "hello"
        # MessageToDict preserves proto field names (snake_case in proto).
        assert "agents" in spec["harness"]
    finally:
        client.close()


def test_get_domain_latest_when_version_omitted(mock_plane: _MockControlPlane) -> None:
    client = ControlPlaneClient(mock_plane.addr)
    try:
        client.register_domain({"id": "d", "version": "1.0.0"})
        client.register_domain({"id": "d", "version": "2.0.0"})
        spec = client.get_domain("d")  # no version
        assert spec["version"] == "2.0.0"
    finally:
        client.close()


def test_get_domain_not_found_raises(mock_plane: _MockControlPlane) -> None:
    client = ControlPlaneClient(mock_plane.addr)
    try:
        with pytest.raises(ControlPlaneError, match="RPC failed"):
            client.get_domain("does-not-exist")
    finally:
        client.close()


def test_list_domains_returns_page(mock_plane: _MockControlPlane) -> None:
    client = ControlPlaneClient(mock_plane.addr)
    try:
        for v in ["1.0.0", "2.0.0", "3.0.0"]:
            client.register_domain({"id": "d", "version": v})
        page = client.list_domains(limit=2, offset=0)
        assert len(page) == 2
        page2 = client.list_domains(limit=2, offset=2)
        assert len(page2) == 1
        # all 3 still came through, just paginated
        all_three = page + page2
        assert {s["version"] for s in all_three} == {"1.0.0", "2.0.0", "3.0.0"}
    finally:
        client.close()


# ---------------------------------------------------------------------------
# auth metadata interceptor
# ---------------------------------------------------------------------------


def test_tenant_id_and_api_key_sent_as_metadata(mock_plane: _MockControlPlane) -> None:
    """``ControlPlaneClient(..., tenant_id=..., api_key=...)`` must inject
    ``x-tenant-id`` and ``x-api-key`` on every outgoing RPC, including
    DomainRegistry ones. The mock's servicer reads ``invocation_metadata()``
    so we can assert the headers actually arrived.
    """
    client = ControlPlaneClient(
        mock_plane.addr,
        tenant_id="tenant-7",
        api_key="key-abc",
    )
    try:
        client.register_domain({"id": "x", "version": "0.1.0"})
    finally:
        client.close()
    md = mock_plane.last_register_metadata
    assert md is not None, "RegisterDomain RPC never reached the mock"
    keys = {k: v for k, v in md}
    assert keys.get("x-tenant-id") == "tenant-7"
    assert keys.get("x-api-key") == "key-abc"


def test_no_metadata_when_constructor_omits_credentials(
    mock_plane: _MockControlPlane,
) -> None:
    """When neither tenant_id nor api_key is set, neither header is sent —
    the interceptor must be a no-op (no empty entries leaking onto the wire).
    """
    client = ControlPlaneClient(mock_plane.addr)
    try:
        client.register_domain({"id": "x", "version": "0.1.0"})
    finally:
        client.close()
    md = mock_plane.last_register_metadata
    keys = {k for k, _ in (md or [])}
    assert "x-tenant-id" not in keys
    assert "x-api-key" not in keys