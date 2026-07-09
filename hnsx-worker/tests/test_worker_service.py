"""Integration tests for the worker parent process.

Each test boots a ``MockHnsXServer`` in-process, points a real
``WorkerService`` at it, and verifies that the worker registers,
heartbeats, pulls a session, and forwards subprocess observations up the
bidi ``StreamChannel``.
"""

from __future__ import annotations

import logging
import threading
import time

import pytest

from hnsx_worker import WorkerConfig, WorkerService
from hnsx_worker.proto.gen.hnsx.v1 import worker_pb2

from .mock_server import MockHnsXServer, wait_for

log = logging.getLogger(__name__)


@pytest.fixture
def mock_server() -> MockHnsXServer:
    srv = MockHnsXServer()
    srv.start()
    yield srv
    srv.stop()


def _make_config(server_addr: str, *, heartbeat_interval: int = 1) -> WorkerConfig:
    cap = worker_pb2.ResourceCapacity(max_concurrent_sessions=2)
    return WorkerConfig(
        server_addr=server_addr,
        worker_id="w-test",
        region="local",
        capacity=cap,
        heartbeat_interval_seconds=heartbeat_interval,
    )


def _start_worker_in_thread(config: WorkerConfig) -> tuple[WorkerService, threading.Thread]:
    svc = WorkerService(config)
    t = threading.Thread(target=svc.run, daemon=True, name="worker-under-test")
    t.start()
    return svc, t


def test_register_and_heartbeat(mock_server: MockHnsXServer) -> None:
    svc, t = _start_worker_in_thread(_make_config(mock_server.addr))
    try:
        assert wait_for(lambda: len(mock_server.registers) >= 1, timeout=5.0)
        assert wait_for(lambda: len(mock_server.heartbeats) >= 1, timeout=5.0)
        assert mock_server.registers[0].info.worker_id == "w-test"
    finally:
        svc.shutdown()
        t.join(timeout=5.0)


def test_pull_session_fork_subprocess_and_stream_observations(mock_server: MockHnsXServer) -> None:
    # Pre-load the mock with a single noop session.
    mock_server.session_complete()

    svc, t = _start_worker_in_thread(_make_config(mock_server.addr))
    try:
        # Wait for the worker to receive the pull assignment and ack it.
        assert wait_for(lambda: len(mock_server.acks) >= 1, timeout=10.0), (
            f"acks={mock_server.acks} pulls={mock_server.pulls}"
        )

        # Wait for the subprocess to emit session_start, agent_invoke, agent_text, session_end.
        assert wait_for(
            lambda: "session_start" in mock_server.snapshot_stream_kinds(),
            timeout=10.0,
        ), f"kinds={mock_server.snapshot_stream_kinds()}"

        assert wait_for(
            lambda: "status:completed" in mock_server.snapshot_stream_kinds(),
            timeout=10.0,
        ), f"kinds={mock_server.snapshot_stream_kinds()}"
    finally:
        svc.shutdown()
        t.join(timeout=5.0)

    # Final assertion: the full observation stream should look like a real session.
    kinds = mock_server.snapshot_stream_kinds()
    assert "session_start" in kinds
    assert "agent_invoke" in kinds
    assert "agent_text" in kinds
    assert "status:completed" in kinds


def test_nack_when_no_free_slots(mock_server: MockHnsXServer) -> None:
    """If the worker is at capacity, it NACKs incoming sessions with requeue=True."""
    cap = worker_pb2.ResourceCapacity(max_concurrent_sessions=0)
    config = WorkerConfig(
        server_addr=mock_server.addr,
        worker_id="w-zero",
        region="local",
        capacity=cap,
        heartbeat_interval_seconds=1,
    )
    mock_server.session_complete()  # the next PullSession will hand this back

    svc, t = _start_worker_in_thread(config)
    try:
        assert wait_for(lambda: len(mock_server.nacks) >= 1, timeout=10.0)
        assert mock_server.nacks[0].requeue is True
        assert mock_server.nacks[0].error_code == "CAPACITY"
    finally:
        svc.shutdown()
        t.join(timeout=5.0)
