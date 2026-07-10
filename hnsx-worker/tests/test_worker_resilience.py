"""Tests for W7 — worker performance and resilience.
"""

from __future__ import annotations

import logging
import threading
import time
from unittest import mock

import pytest

from hnsx_worker import WorkerConfig, WorkerService
from hnsx_worker.proto.gen.hnsx.v1 import worker_pb2
from hnsx_worker.proto_client import (
    DomainInvalidation,
    DrainCommand,
    ServerEvent,
)
from hnsx_worker.session_runtime import _timeout_self

from .mock_server import MockHnsXServer, wait_for

log = logging.getLogger(__name__)


def _make_config(
    server_addr: str, *, heartbeat_interval: int = 1, pool_size: int = 0
) -> WorkerConfig:
    cap = worker_pb2.ResourceCapacity(max_concurrent_sessions=2)
    return WorkerConfig(
        server_addr=server_addr,
        worker_id="w-test",
        region="local",
        capacity=cap,
        heartbeat_interval_seconds=heartbeat_interval,
        pool_size=pool_size,
    )


def _start_worker_in_thread(config: WorkerConfig) -> tuple[WorkerService, threading.Thread]:
    svc = WorkerService(config)
    t = threading.Thread(target=svc.run, daemon=True, name="worker-under-test")
    t.start()
    return svc, t


@pytest.fixture
def mock_server() -> MockHnsXServer:
    srv = MockHnsXServer()
    srv.start()
    yield srv
    srv.stop()


def test_prefork_pool_warms_processes(mock_server: MockHnsXServer) -> None:
    svc, t = _start_worker_in_thread(_make_config(mock_server.addr, pool_size=2))
    try:
        # Give the pool warmer time to spawn processes.
        time.sleep(0.6)
        assert svc._pool.qsize() == 2
    finally:
        svc.shutdown()
        t.join(timeout=5.0)


def test_drain_event_stops_pulling_new_sessions(mock_server: MockHnsXServer) -> None:
    svc, t = _start_worker_in_thread(_make_config(mock_server.addr))
    try:
        assert wait_for(lambda: len(mock_server.registers) >= 1, timeout=5.0)
        svc._handle_server_event(
            ServerEvent(
                kind="drain", drain=DrainCommand(reason="deploy", deadline_seconds=5)
            )
        )
        assert svc._drain_event.is_set()
        # The pull loop should exit shortly after drain.
        assert wait_for(lambda: not t.is_alive(), timeout=5.0)
    finally:
        svc.shutdown()
        t.join(timeout=5.0)


def test_domain_invalidation_drops_cache() -> None:
    config = _make_config("127.0.0.1:0")
    svc = WorkerService(config)
    svc._domain_cache["domain-1"] = {"id": "domain-1"}
    svc._handle_server_event(
        ServerEvent(
            kind="invalidate",
            invalidate=DomainInvalidation(domain_id="domain-1", version="2"),
        )
    )
    assert "domain-1" not in svc._domain_cache


def test_get_domain_spec_uses_cache() -> None:
    config = _make_config("127.0.0.1:0")
    svc = WorkerService(config)
    svc._domain_cache["d"] = {"cached": True}
    assert svc._get_domain_spec("d", '{"cached":false}')["cached"] is True


def test_timeout_self_sets_stop_event() -> None:
    stop_event = threading.Event()
    with mock.patch("os.kill") as mock_kill:
        _timeout_self("s-1", stop_event)
    assert stop_event.is_set() is True
    assert mock_kill.called


def test_subprocess_crash_emits_failed_session_end(mock_server: MockHnsXServer) -> None:
    """If the subprocess exits non-zero (crash), worker emits session_end failed."""
    # Push a session whose spec is invalid enough to make session_runtime exit 1.
    mock_server.session_complete(domain_spec_json="not valid json")

    svc, t = _start_worker_in_thread(_make_config(mock_server.addr))
    try:
        assert wait_for(lambda: len(mock_server.acks) >= 1, timeout=10.0)
        assert wait_for(
            lambda: "status:failed" in mock_server.snapshot_stream_kinds(),
            timeout=10.0,
        ), f"kinds={mock_server.snapshot_stream_kinds()}"
    finally:
        svc.shutdown()
        t.join(timeout=5.0)
