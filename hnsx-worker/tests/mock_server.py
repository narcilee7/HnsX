"""In-process mock gRPC server for WorkerService tests.

Spins up ``WorkerService`` and ``SchedulerService`` servicers on an
ephemeral port. Captures every Register / Heartbeat / PullSession /
AckSession / NackSession call and every observation flowing up the
bidi ``StreamChannel``.

Usage:

    server = MockHnsXServer()
    server.start()
    try:
        ...run worker pointing at server.addr...
    finally:
        server.stop()
"""

from __future__ import annotations

import logging
import queue
import threading
import time
from collections.abc import Iterator
from concurrent import futures

import grpc

from hnsx_worker.proto.gen.hnsx.v1 import worker_pb2, worker_pb2_grpc

log = logging.getLogger("hnsx_worker.tests.mock_server")


class MockHnsXServer:
    def __init__(self) -> None:
        self.registers: list[worker_pb2.RegisterRequest] = []
        self.heartbeats: list[worker_pb2.HeartbeatRequest] = []
        self.pulls: list[worker_pb2.PullSessionRequest] = []
        self.acks: list[worker_pb2.AckSessionRequest] = []
        self.nacks: list[worker_pb2.NackSessionRequest] = []
        # Inbox of canned session assignments the mock will return on PullSession.
        self.session_queue: queue.Queue[worker_pb2.PullSessionResponse] = queue.Queue()
        # Capture of every stream request the worker sent up.
        self.stream_requests: list[worker_pb2.StreamChannelRequest] = []
        self._server: grpc.Server | None = None
        self.addr: str = ""
        self._lock = threading.Lock()

    # ------------------------------------------------------------------ control

    def start(self) -> None:
        server = grpc.server(futures.ThreadPoolExecutor(max_workers=8))
        worker_pb2_grpc.add_WorkerServiceServicer_to_server(_WorkerServicer(self), server)
        worker_pb2_grpc.add_SchedulerServiceServicer_to_server(_SchedulerServicer(self), server)
        bound = server.add_insecure_port("[::]:0")
        server.start()
        self._server = server
        self.addr = f"127.0.0.1:{bound}"
        log.info("mock server listening on %s", self.addr)

    def stop(self, grace: float = 2.0) -> None:
        if self._server is not None:
            self._server.stop(grace).wait()
            self._server = None

    def offer(self, resp: worker_pb2.PullSessionResponse) -> None:
        self.session_queue.put(resp)

    def session_complete(self) -> None:
        """Push a final session whose spec triggers immediate session_end."""
        self.session_queue.put(_noop_session())

    # ------------------------------------------------------------------ capture

    def record_stream(self, req: worker_pb2.StreamChannelRequest) -> None:
        with self._lock:
            self.stream_requests.append(req)

    def snapshot_stream_kinds(self) -> list[str]:
        """Flatten every observed observation/status/result payload kind."""
        kinds: list[str] = []
        with self._lock:
            for req in self.stream_requests:
                which = req.WhichOneof("payload")
                if which == "observations":
                    for obs in req.observations.observations:
                        kinds.append(obs.kind)
                elif which == "status":
                    kinds.append(f"status:{req.status.state}")
                elif which == "result":
                    kinds.append("result")
        return kinds


# ---------------------------------------------------------------------------
# Servicers
# ---------------------------------------------------------------------------


class _WorkerServicer(worker_pb2_grpc.WorkerServiceServicer):
    def __init__(self, server: MockHnsXServer) -> None:
        self.server = server

    def Register(self, request, context):  # noqa: ARG002 — gRPC signature
        self.server.registers.append(request)
        wid = request.info.worker_id or f"w-mock-{int(time.time() * 1000)}"
        return worker_pb2.RegisterResponse(
            worker_id=wid,
            server_time_ms=int(time.time() * 1000),
            heartbeat_interval_seconds=request.info.capacity.max_concurrent_sessions and 5 or 5,
        )

    def Heartbeat(self, request, context):  # noqa: ARG002
        self.server.heartbeats.append(request)
        return worker_pb2.HeartbeatResponse(server_time_ms=int(time.time() * 1000))


class _SchedulerServicer(worker_pb2_grpc.SchedulerServiceServicer):
    def __init__(self, server: MockHnsXServer) -> None:
        self.server = server

    def PullSession(self, request, context):  # noqa: ARG002
        self.server.pulls.append(request)
        try:
            return self.server.session_queue.get(timeout=2.0)
        except queue.Empty:
            # Tell the worker there's no work; the worker will loop.
            return worker_pb2.PullSessionResponse()

    def AckSession(self, request, context):  # noqa: ARG002
        self.server.acks.append(request)
        return worker_pb2.AckSessionResponse()

    def NackSession(self, request, context):  # noqa: ARG002
        self.server.nacks.append(request)
        return worker_pb2.NackSessionResponse()

    def StreamChannel(self, request_iterator, context):  # noqa: ARG002
        for req in request_iterator:
            self.server.record_stream(req)
        # Block until client closes; emit a single ping then idle.
        try:
            while True:
                yield worker_pb2.StreamChannelResponse(
                    ping=worker_pb2.Ping(timestamp_ms=int(time.time() * 1000))
                )
                time.sleep(0.5)
        except Exception:
            return


def _noop_session() -> worker_pb2.PullSessionResponse:
    import json as _json

    spec = {
        "id": "mock-domain",
        "version": "0.1.0",
        "harness": {
            "agents": {
                "primary": {
                    "id": "primary",
                    "provider": "noop",
                    "model": "noop-1",
                    "adapter": {"kind": "noop"},
                    "system_prompt": "You are a primary agent.",
                }
            },
            "session": {"mode": "single-task", "agent": "primary"},
        },
    }
    return worker_pb2.PullSessionResponse(
        session_id=f"s-mock-{int(time.time() * 1000)}",
        domain_id="mock-domain",
        domain_version="0.1.0",
        domain_spec_json=_json.dumps(spec),
        trigger_payload_json=_json.dumps({"question": "hello from mock"}),
        trace_id="t-mock-1",
        assigned_at_ms=int(time.time() * 1000),
        session_timeout_seconds=60,
        correlation_id=f"c-mock-{int(time.time() * 1000)}",
    )


def wait_for(predicate, timeout: float = 5.0, interval: float = 0.05) -> bool:
    """Poll until ``predicate()`` returns truthy or the timeout elapses."""
    deadline = time.monotonic() + timeout
    while time.monotonic() < deadline:
        if predicate():
            return True
        time.sleep(interval)
    return False
