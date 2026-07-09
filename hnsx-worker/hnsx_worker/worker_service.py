"""WorkerService — the parent process that talks to the Go control plane.

Threading model:

  - main thread:    ``_pull_loop`` — long-polls the server and forks
                    one subprocess per assigned session.
  - heartbeat:      ``_heartbeat_loop`` — every ``heartbeat_interval_seconds``
                    sends a Heartbeat RPC.
  - stream producer:``_stream_producer_loop`` — opens the bidi
                    StreamChannel and feeds outbound messages from
                    ``_obs_queue`` to the server.
  - per-subprocess: ``_pump_subprocess_stdout`` — one daemon thread per
                    session subprocess, parsing JSONL observations and
                    pushing them into ``_obs_queue``.

The parent process NEVER executes the session itself — that's the subprocess's
job. This keeps the harness "session = subprocess" invariant.

This module is wire-format agnostic: all gRPC types live behind
``ControlPlaneClient`` in :mod:`hnsx_worker.proto_client`.
"""

from __future__ import annotations

import json
import logging
import os
import queue
import signal
import socket
import subprocess
import sys
import threading
import time
import uuid
from collections.abc import Iterator
from typing import Any

from hnsx_worker.config import WorkerConfig
from hnsx_worker.proto_client import (
    AckRequest,
    ControlPlaneClient,
    HeartbeatRequest,
    NackRequest,
    Observation,
    OutboundMessage,
    ResourceCapacity,
    ResourceUsage,
    ServerEvent,
    SessionStatusUpdate,
    WorkerHealth,
    WorkerHealthStatus,
    WorkerInfo,
)

log = logging.getLogger("hnsx_worker.worker_service")

# how long to wait for an in-flight subprocess when the worker is shutting down
_SHUTDOWN_GRACE_SECONDS = 5.0


class WorkerService:
    def __init__(self, config: WorkerConfig) -> None:
        self.config = config
        self.client = ControlPlaneClient(
            config.server_addr, auth_token=config.auth_token
        )
        self.worker_id: str = config.worker_id  # server may overwrite on Register
        self._obs_queue: queue.Queue[OutboundMessage] = queue.Queue()
        self._running: dict[str, _SessionHandle] = {}
        self._stop_event = threading.Event()
        self._lock = threading.Lock()

    # ------------------------------------------------------------------ public

    def run(self) -> None:
        """Block until the worker is asked to stop."""
        self._install_signal_handlers()
        self._register()
        threading.Thread(target=self._heartbeat_loop, name="heartbeat", daemon=True).start()
        threading.Thread(
            target=self._stream_producer_loop, name="stream-producer", daemon=True
        ).start()
        log.info("worker %s ready; entering pull loop", self.worker_id)
        try:
            self._pull_loop()
        finally:
            self.shutdown()

    def shutdown(self) -> None:
        if self._stop_event.is_set():
            return
        log.info("worker %s shutting down", self.worker_id)
        self._stop_event.set()
        with self._lock:
            handles = list(self._running.values())
        deadline = time.monotonic() + _SHUTDOWN_GRACE_SECONDS
        for handle in handles:
            handle.proc.terminate()
        for handle in handles:
            remaining = max(0.0, deadline - time.monotonic())
            try:
                handle.proc.wait(timeout=remaining)
            except subprocess.TimeoutExpired:
                handle.proc.kill()
                handle.proc.wait()
        self.client.close()

    # ------------------------------------------------------------------ lifecycle

    def _register(self) -> None:
        info = WorkerInfo(
            worker_id=self.config.worker_id,
            tenant_id="local",  # V1.1: single-tenant local dev
            version="0.2.0",
            region=self.config.region,
            hostname=socket.gethostname(),
            pid=str(os.getpid()),
            capacity=ResourceCapacity(
                max_concurrent_sessions=self.config.capacity.max_concurrent_sessions,
                providers=list(self.config.capacity.providers),
                models=list(self.config.capacity.models),
            ),
        )
        resp = self.client.register(info)
        self.worker_id = resp.worker_id or self.worker_id or f"w-{uuid.uuid4().hex[:8]}"
        log.info(
            "registered as %s (heartbeat every %ds)",
            self.worker_id,
            resp.heartbeat_interval_seconds or self.config.heartbeat_interval_seconds,
        )

    def _heartbeat_loop(self) -> None:
        interval = self.config.heartbeat_interval_seconds
        while not self._stop_event.is_set():
            try:
                with self._lock:
                    running_ids = list(self._running.keys())
                req = HeartbeatRequest(
                    worker_id=self.worker_id,
                    timestamp_ms=int(time.time() * 1000),
                    usage=ResourceUsage(
                        running_sessions=len(running_ids),
                        free_slots=max(
                            0,
                            self.config.capacity.max_concurrent_sessions
                            - len(running_ids),
                        ),
                    ),
                    running_session_ids=running_ids,
                    health=WorkerHealth(status=WorkerHealthStatus.HEALTHY),
                )
                self.client.heartbeat(req)
            except Exception as e:  # noqa: BLE001 — network blips shouldn't kill the loop
                log.warning("heartbeat failed: %s", e)
            self._stop_event.wait(interval)

    # ------------------------------------------------------------------ pull

    def _pull_loop(self) -> None:
        max_wait = 30  # seconds; server may hold the call up to this long
        while not self._stop_event.is_set():
            try:
                assignment = self.client.pull_session(
                    worker_id=self.worker_id,
                    max_wait_seconds=max_wait,
                )
            except Exception as e:  # noqa: BLE001 — channel-level errors
                log.warning("pull failed: %s; retrying", e)
                self._stop_event.wait(2.0)
                continue
            if assignment.is_empty():
                continue  # empty result == no work; loop again
            self._on_session(assignment)

    def _on_session(self, assignment: Any) -> None:
        with self._lock:
            if len(self._running) >= self.config.capacity.max_concurrent_sessions:
                # no slot; requeue
                self._nack(assignment, reason="no_free_slots", requeue=True, error_code="CAPACITY")
                return
        log.info("assigned session %s (domain=%s)", assignment.session_id, assignment.domain_id)
        try:
            proc = subprocess.Popen(
                [sys.executable, "-m", "hnsx_worker.session_runtime"],
                stdin=subprocess.PIPE,
                stdout=subprocess.PIPE,
                stderr=subprocess.PIPE,
                text=True,
                bufsize=1,
            )
        except OSError as e:
            log.error("failed to spawn subprocess: %s", e)
            self._nack(assignment, reason=f"spawn_failed: {e}", requeue=False, error_code="SPAWN")
            return

        payload = json.dumps(
            {
                "session_id": assignment.session_id,
                "correlation_id": assignment.correlation_id,
                "domain_id": assignment.domain_id,
                "domain_version": assignment.domain_version,
                "trace_id": assignment.trace_id,
                "domain_spec_json": assignment.domain_spec_json,
                "trigger_payload_json": assignment.trigger_payload_json,
                "session_timeout_seconds": assignment.session_timeout_seconds,
            }
        )
        assert proc.stdin is not None
        try:
            proc.stdin.write(payload)
            proc.stdin.close()
        except BrokenPipeError:
            pass

        handle = _SessionHandle(
            proc=proc,
            session_id=assignment.session_id,
            correlation_id=assignment.correlation_id,
        )
        with self._lock:
            self._running[assignment.session_id] = handle

        threading.Thread(
            target=self._pump_subprocess_stdout,
            args=(handle,),
            name=f"pump-{assignment.session_id[:8]}",
            daemon=True,
        ).start()

        try:
            self.client.ack_session(
                AckRequest(
                    worker_id=self.worker_id,
                    session_id=assignment.session_id,
                    correlation_id=assignment.correlation_id,
                    acknowledged_at_ms=int(time.time() * 1000),
                )
            )
        except Exception as e:  # noqa: BLE001
            log.warning("ack failed for %s: %s", assignment.session_id, e)

    def _pump_subprocess_stdout(self, handle: "_SessionHandle") -> None:
        assert handle.proc.stdout is not None
        try:
            for line in handle.proc.stdout:
                line = line.strip()
                if not line:
                    continue
                try:
                    obs = json.loads(line)
                except json.JSONDecodeError:
                    log.warning("subprocess emitted non-JSON line: %r", line[:200])
                    continue
                self._enqueue_observation(handle, obs)
        except Exception as e:  # noqa: BLE001
            log.warning("pump for %s ended: %s", handle.session_id, e)
        finally:
            rc = handle.proc.wait()
            self._enqueue_observation(
                handle,
                {
                    "kind": "session_end",
                    "session_id": handle.session_id,
                    "state": "completed" if rc == 0 else ("cancelled" if rc == 130 else "failed"),
                    "payload": {"exit_code": rc},
                },
            )
            with self._lock:
                self._running.pop(handle.session_id, None)
            log.info("session %s exited rc=%d", handle.session_id, rc)

    def _enqueue_observation(self, handle: "_SessionHandle", obs: dict[str, Any]) -> None:
        # All observations from the subprocess are batched through the
        # ``observations`` envelope. Session end events are special: they are
        # also forwarded as a status update so the scheduler can update its
        # bookkeeping and the API session state.
        observation = Observation(
            session_id=handle.session_id,
            domain_id=obs.get("domain_id", ""),
            step_id=obs.get("step_id", ""),
            agent_id=obs.get("agent_id", ""),
            kind=obs.get("kind", ""),
            payload=dict(obs.get("payload", {}) or {}),
            created_at_ms=int(obs.get("created_at_ms") or time.time() * 1000),
        )
        self._obs_queue.put(
            OutboundMessage(kind="observations", observations=[observation])
        )

        if obs.get("kind") == "session_end":
            self._obs_queue.put(
                OutboundMessage(
                    kind="status",
                    status=SessionStatusUpdate(
                        session_id=handle.session_id,
                        state=obs.get("state", "completed"),
                        message=json.dumps(obs.get("payload", {}), default=str),
                        timestamp_ms=int(obs.get("created_at_ms") or time.time() * 1000),
                    ),
                )
            )

    def _nack(
        self,
        assignment: Any,
        *,
        reason: str,
        requeue: bool,
        error_code: str,
    ) -> None:
        try:
            self.client.nack_session(
                NackRequest(
                    worker_id=self.worker_id,
                    session_id=assignment.session_id,
                    correlation_id=assignment.correlation_id,
                    reason=reason,
                    error_code=error_code,
                    requeue=requeue,
                )
            )
        except Exception as e:  # noqa: BLE001
            log.warning("nack failed for %s: %s", assignment.session_id, e)

    # ------------------------------------------------------------------ stream

    def _stream_producer_loop(self) -> None:
        """Open the bidi StreamChannel and forward outbound messages to the server.

        Server-pushed events (Cancel / Drain / DomainInvalidation) are received
        here too. Full handling lands in #9+#11.
        """
        backoff = 0.5
        while not self._stop_event.is_set():
            try:
                events = self.client.open_stream(
                    _outbound_iter(self._obs_queue, self._stop_event),
                    timeout=None,
                )
                backoff = 0.5
                for server_event in events:
                    if self._stop_event.is_set():
                        break
                    self._handle_server_event(server_event)
            except Exception as e:  # noqa: BLE001 — channel-level errors
                if self._stop_event.is_set():
                    return
                log.warning(
                    "stream channel error: %s; reconnecting in %.1fs", e, backoff
                )
                self._stop_event.wait(backoff)
                backoff = min(backoff * 2, 10.0)

    def _handle_server_event(self, event: ServerEvent) -> None:
        if event.kind == "cancel" and event.cancel is not None:
            self._cancel_session(event.cancel.session_id, reason=event.cancel.reason)
        elif event.kind == "drain" and event.drain is not None:
            log.info(
                "server requested drain: %s (deadline=%ds)",
                event.drain.reason,
                event.drain.deadline_seconds,
            )
            # Full drain handling is #9.
        elif event.kind == "invalidate" and event.invalidate is not None:
            log.info(
                "server invalidated domain %s/%s",
                event.invalidate.domain_id,
                event.invalidate.version,
            )
        elif event.kind == "ping":
            log.debug("server ping")
        else:
            log.debug("server event: kind=%s", event.kind)

    def _cancel_session(self, session_id: str, *, reason: str) -> None:
        with self._lock:
            handle = self._running.get(session_id)
        if handle is None:
            return
        log.info("cancelling session %s (reason=%s)", session_id, reason)
        try:
            handle.proc.send_signal(signal.SIGTERM)
        except ProcessLookupError:
            pass

    # ------------------------------------------------------------------ signals

    def _install_signal_handlers(self) -> None:
        def _on_signal(signum: int, _frame: Any) -> None:
            log.info("received signal %d; shutting down", signum)
            self._stop_event.set()

        try:
            signal.signal(signal.SIGINT, _on_signal)
            signal.signal(signal.SIGTERM, _on_signal)
        except ValueError:
            # not main thread (e.g. tests)
            pass


class _SessionHandle:
    __slots__ = ("proc", "session_id", "correlation_id")

    def __init__(self, proc: subprocess.Popen, session_id: str, correlation_id: str) -> None:
        self.proc = proc
        self.session_id = session_id
        self.correlation_id = correlation_id


def _outbound_iter(
    q: "queue.Queue[OutboundMessage]", stop_event: threading.Event
) -> Iterator[OutboundMessage]:
    """Bridge a queue.Queue into a generator for the bidi stream."""
    while not stop_event.is_set():
        try:
            item = q.get(timeout=0.5)
        except queue.Empty:
            continue
        yield item