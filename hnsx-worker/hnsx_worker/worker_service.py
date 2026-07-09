"""WorkerService — the parent process that talks to the Go control plane.

Threading model (Step 2):

  - main thread:    ``_pull_loop`` — long-polls ``PullSession`` and forks
                    one subprocess per assigned session.
  - heartbeat:      ``_heartbeat_loop`` — every ``heartbeat_interval_seconds``
                    sends a Heartbeat RPC.
  - stream producer:``_stream_producer_loop`` — opens the bidi
                    ``StreamChannel`` and feeds observations from
                    ``_obs_queue`` to the server.
  - per-subprocess: ``_pump_subprocess_stdout`` — one daemon thread per
                    session subprocess, parsing JSONL observations and
                    pushing them into ``_obs_queue``.

The parent process NEVER executes the session itself — that's the subprocess's
job. This keeps the harness "session = subprocess" invariant from
``design/Tech/V1/Architecture.md`` §10.3.
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

import grpc

from hnsx_worker.config import WorkerConfig
from hnsx_worker.proto.gen.hnsx.v1 import observation_pb2, worker_pb2, worker_pb2_grpc

log = logging.getLogger("hnsx_worker.worker_service")

# how long to wait for an in-flight subprocess when the worker is shutting down
_SHUTDOWN_GRACE_SECONDS = 5.0


class WorkerService:
    def __init__(self, config: WorkerConfig) -> None:
        self.config = config
        self.channel = grpc.insecure_channel(config.server_addr)
        self.worker_stub = worker_pb2_grpc.WorkerServiceStub(self.channel)
        self.scheduler_stub = worker_pb2_grpc.SchedulerServiceStub(self.channel)
        self.worker_id: str = config.worker_id  # server may overwrite on Register
        self._obs_queue: queue.Queue[worker_pb2.StreamChannelRequest] = queue.Queue()
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
        try:
            self.channel.close()
        except Exception:  # noqa: BLE001
            pass

    # ------------------------------------------------------------------ lifecycle

    def _register(self) -> None:
        info = worker_pb2.WorkerInfo(
            worker_id=self.config.worker_id,
            tenant_id="local",  # V1.1: single-tenant local dev
            version="0.1.0",
            region=self.config.region,
            hostname=socket.gethostname(),
            pid=str(os.getpid()),
            capacity=self.config.capacity,
        )
        req = worker_pb2.RegisterRequest(info=info, auth_token=self.config.auth_token)
        resp = self.worker_stub.Register(req, timeout=10.0)
        self.worker_id = resp.worker_id or self.worker_id or f"w-{uuid.uuid4().hex[:8]}"
        log.info("registered as %s (heartbeat every %ds)", self.worker_id, resp.heartbeat_interval_seconds or self.config.heartbeat_interval_seconds)

    def _heartbeat_loop(self) -> None:
        interval = self.config.heartbeat_interval_seconds
        while not self._stop_event.is_set():
            try:
                with self._lock:
                    running_ids = list(self._running.keys())
                usage = worker_pb2.ResourceUsage(
                    running_sessions=len(running_ids),
                    free_slots=max(0, self.config.capacity.max_concurrent_sessions - len(running_ids)),
                )
                req = worker_pb2.HeartbeatRequest(
                    worker_id=self.worker_id,
                    timestamp_ms=int(time.time() * 1000),
                    usage=usage,
                    running_session_ids=running_ids,
                    health=worker_pb2.WorkerHealth(
                        status=worker_pb2.WorkerHealth.STATUS_HEALTHY,
                    ),
                )
                self.worker_stub.Heartbeat(req, timeout=5.0)
            except grpc.RpcError as e:
                log.warning("heartbeat failed: %s", e.code().name)
            self._stop_event.wait(interval)

    # ------------------------------------------------------------------ pull

    def _pull_loop(self) -> None:
        max_wait = 30  # seconds; server may hold the call up to this long
        while not self._stop_event.is_set():
            try:
                req = worker_pb2.PullSessionRequest(
                    worker_id=self.worker_id,
                    max_wait_seconds=max_wait,
                )
                resp = self.scheduler_stub.PullSession(req, timeout=max_wait + 10.0)
            except grpc.RpcError as e:
                if e.code() == grpc.StatusCode.CANCELLED:
                    return
                log.warning("pull failed: %s; retrying", e.code().name)
                self._stop_event.wait(2.0)
                continue
            if not resp.session_id:
                continue  # empty result == no work; loop again
            self._on_session(resp)

    def _on_session(self, resp: worker_pb2.PullSessionResponse) -> None:
        with self._lock:
            if len(self._running) >= self.config.capacity.max_concurrent_sessions:
                # no slot; requeue
                self._nack(resp, reason="no_free_slots", requeue=True, error_code="CAPACITY")
                return
        log.info("assigned session %s (domain=%s)", resp.session_id, resp.domain_id)
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
            self._nack(resp, reason=f"spawn_failed: {e}", requeue=False, error_code="SPAWN")
            return

        payload = json.dumps(
            {
                "session_id": resp.session_id,
                "correlation_id": resp.correlation_id,
                "domain_id": resp.domain_id,
                "domain_version": resp.domain_version,
                "trace_id": resp.trace_id,
                "domain_spec_json": resp.domain_spec_json,
                "trigger_payload_json": resp.trigger_payload_json,
                "session_timeout_seconds": resp.session_timeout_seconds,
            }
        )
        assert proc.stdin is not None
        try:
            proc.stdin.write(payload)
            proc.stdin.close()
        except BrokenPipeError:
            pass

        handle = _SessionHandle(proc=proc, session_id=resp.session_id, correlation_id=resp.correlation_id)
        with self._lock:
            self._running[resp.session_id] = handle

        threading.Thread(
            target=self._pump_subprocess_stdout,
            args=(handle,),
            name=f"pump-{resp.session_id[:8]}",
            daemon=True,
        ).start()

        try:
            self.scheduler_stub.AckSession(
                worker_pb2.AckSessionRequest(
                    worker_id=self.worker_id,
                    session_id=resp.session_id,
                    acknowledged_at_ms=int(time.time() * 1000),
                    correlation_id=resp.correlation_id,
                ),
                timeout=5.0,
            )
        except grpc.RpcError as e:
            log.warning("ack failed for %s: %s", resp.session_id, e.code().name)

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
        proto_obs = worker_pb2.StreamChannelRequest(worker_id=self.worker_id)
        # All observations from the subprocess are batched through the
        # ``observations`` oneof arm; structured session_end events
        # are emitted as a separate status event so the server's fan-out
        # can do a clean session_end signal.
        if obs.get("kind") == "session_end":
            proto_obs.status.CopyFrom(
                worker_pb2.SessionStatusUpdate(
                    session_id=handle.session_id,
                    state=obs.get("state", "completed"),
                    message=json.dumps(obs.get("payload", {})),
                    timestamp_ms=int(obs.get("created_at_ms") or time.time() * 1000),
                )
            )
        else:
            batch = worker_pb2.ObservationBatch()
            batch.observations.append(
                observation_pb2.Observation(
                    session_id=handle.session_id,
                    domain_id=obs.get("domain_id", ""),
                    step_id=obs.get("step_id", ""),
                    agent_id=obs.get("agent_id", ""),
                    kind=obs.get("kind", ""),
                    payload=json.dumps(obs.get("payload", {}), default=str),
                    created_at_ms=int(obs.get("created_at_ms") or time.time() * 1000),
                )
            )
            proto_obs.observations.CopyFrom(batch)
        self._obs_queue.put(proto_obs)

    def _nack(
        self,
        resp: worker_pb2.PullSessionResponse,
        *,
        reason: str,
        requeue: bool,
        error_code: str,
    ) -> None:
        try:
            self.scheduler_stub.NackSession(
                worker_pb2.NackSessionRequest(
                    worker_id=self.worker_id,
                    session_id=resp.session_id,
                    reason=reason,
                    error_code=error_code,
                    requeue=requeue,
                ),
                timeout=5.0,
            )
        except grpc.RpcError as e:
            log.warning("nack failed for %s: %s", resp.session_id, e.code().name)

    # ------------------------------------------------------------------ stream

    def _stream_producer_loop(self) -> None:
        """Open the bidi StreamChannel and forward observations to the server.

        Server-pushed messages (Cancel / Drain / DomainInvalidation) are
        received here too. Step 2 just logs them; full handling is #9+#11.
        """
        backoff = 0.5
        while not self._stop_event.is_set():
            try:
                stream = self.scheduler_stub.StreamChannel(_outbound_iter(self._obs_queue))
                backoff = 0.5
                for server_event in stream:
                    if self._stop_event.is_set():
                        break
                    self._handle_server_event(server_event)
            except grpc.RpcError as e:
                if self._stop_event.is_set():
                    return
                log.warning("stream channel error: %s; reconnecting in %.1fs", e.code().name, backoff)
                self._stop_event.wait(backoff)
                backoff = min(backoff * 2, 10.0)

    def _handle_server_event(self, event: worker_pb2.StreamChannelResponse) -> None:
        kind = event.WhichOneof("payload")
        if kind == "cancel":
            self._cancel_session(event.cancel.session_id, reason=event.cancel.reason)
        elif kind == "drain":
            log.info("server requested drain: %s", event.drain.reason)
            # Step 2: log only; full drain handling is #9.
        elif kind == "invalidate":
            log.info("server invalidated domain %s/%s", event.invalidate.domain_id, event.invalidate.version)
        elif kind == "ping":
            log.debug("server ping")
        else:
            log.debug("server event: %r", event)

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


def _outbound_iter(q: "queue.Queue[worker_pb2.StreamChannelRequest]") -> Iterator[worker_pb2.StreamChannelRequest]:
    """Bridge a queue.Queue into a generator for gRPC client-streaming."""
    while True:
        try:
            item = q.get(timeout=0.5)
        except queue.Empty:
            # yield a heartbeat-shaped ping-style no-op periodically
            # (the server can interpret silence as liveness; we keep the
            #  stream alive by yielding nothing here, but gRPC will close
            #  on idle. To stay alive we yield a periodic empty Status.)
            continue
        yield item
