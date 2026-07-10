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
    observation_pb2,
    worker_pb2,
    worker_pb2_grpc,
)
from hnsx_worker.proto_client import (
    AckRequest,
    ControlPlaneClient,
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
        # Next pull-session response (overridable per-test).
        self.next_pull = worker_pb2.PullSessionResponse()
        # Cancel command to push down the bidi stream (overridable).
        self.inbound_cancel: worker_pb2.StreamChannelResponse | None = None
        self._server: grpc.Server | None = None
        self.addr: str = ""
        self._lock = threading.Lock()

    def start(self) -> None:
        server = grpc.server(__import__("concurrent.futures", fromlist=["ThreadPoolExecutor"]).ThreadPoolExecutor(max_workers=8))
        worker_pb2_grpc.add_WorkerServiceServicer_to_server(_WorkerServicer(self), server)
        worker_pb2_grpc.add_SchedulerServiceServicer_to_server(_SchedulerServicer(self), server)
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