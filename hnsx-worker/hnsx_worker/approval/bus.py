"""Approval bus — transport for ApprovalRequest ↔ ApprovalResponse.

The default :class:`InMemoryApprovalBus` is process-local and intended
for tests + single-worker deployments. Long-poll, SSE, Slack, or Email
back-ends are expected to subclass :class:`ApprovalBus`.

The high-level :func:`request_approval` helper does the
submit-and-block dance, including timeout handling.
"""

from __future__ import annotations

import logging
import threading
import time
from abc import ABC, abstractmethod
from collections.abc import Callable
from dataclasses import dataclass, field

from .protocol import (
    ApprovalDecision,
    ApprovalRequest,
    ApprovalResponse,
    ApprovalState,
)

log = logging.getLogger("hnsx_worker.approval.bus")


EmitFn = Callable[[dict], None]


@dataclass
class PendingRequest:
    """In-flight request tracked by the bus."""

    request: ApprovalRequest
    state: ApprovalState = ApprovalState.PENDING
    response: ApprovalResponse | None = None
    event: threading.Event = field(default_factory=threading.Event)


class ApprovalBus(ABC):
    """Abstract approval transport.

    Implementations are responsible for surfacing requests to the human
    (console / Slack / Email) and delivering the response back into the
    session. The base ABC doesn't mandate the notification side; tests
    call :meth:`respond` directly.
    """

    @abstractmethod
    def submit(self, request: ApprovalRequest) -> PendingRequest:
        """Register a new request and return a handle the caller can wait on."""

    @abstractmethod
    def respond(self, response: ApprovalResponse) -> bool:
        """Deliver a response. Returns True if it matched a pending request."""

    @abstractmethod
    def get(self, request_id: str) -> PendingRequest | None:
        """Look up an in-flight request."""

    @abstractmethod
    def cancel(self, request_id: str) -> bool:
        """Cancel a pending request (e.g. on session stop)."""


class InMemoryApprovalBus(ApprovalBus):
    """Thread-safe in-process bus.

    Designed for tests + single-worker mode. The bus exposes the pending
    requests so the harness can render them in the Console or unit-test
    fixtures.
    """

    def __init__(self, *, emit: EmitFn | None = None) -> None:
        self._pending: dict[str, PendingRequest] = {}
        self._lock = threading.RLock()
        self._emit = emit or (lambda _o: None)

    # ------------------------------------------------------------------ API

    def submit(self, request: ApprovalRequest) -> PendingRequest:
        with self._lock:
            pending = PendingRequest(request=request)
            self._pending[request.request_id] = pending
        self._emit(
            {
                "kind": "approval_requested",
                "session_id": request.session_id,
                "domain_id": request.domain_id,
                "agent_id": request.agent_id,
                "payload": request.to_dict(),
            }
        )
        return pending

    def respond(self, response: ApprovalResponse) -> bool:
        with self._lock:
            pending = self._pending.get(response.request_id)
            if pending is None:
                return False
            pending.response = response
            pending.state = self._state_from_decision(response)
            pending.event.set()
        self._emit(
            {
                "kind": "approval_received",
                "session_id": pending.request.session_id,
                "domain_id": pending.request.domain_id,
                "agent_id": pending.request.agent_id,
                "payload": response.to_dict(),
            }
        )
        return True

    def get(self, request_id: str) -> PendingRequest | None:
        with self._lock:
            return self._pending.get(request_id)

    def cancel(self, request_id: str) -> bool:
        """Cancel a pending request. Idempotent: only the first call wins."""
        with self._lock:
            pending = self._pending.get(request_id)
            if pending is None:
                return False
            if pending.state is not ApprovalState.PENDING:
                return False
            pending.state = ApprovalState.CANCELLED
            pending.event.set()
        return True

    # ------------------------------------------------------------------ introspection

    def pending(self) -> list[PendingRequest]:
        with self._lock:
            return list(self._pending.values())

    def clear(self) -> None:
        with self._lock:
            self._pending.clear()

    @staticmethod
    def _state_from_decision(response: ApprovalResponse) -> ApprovalState:
        if response.decision is ApprovalDecision.APPROVE:
            return ApprovalState.APPROVED
        if response.decision is ApprovalDecision.DENY:
            return ApprovalState.DENIED
        if response.decision is ApprovalDecision.EDIT:
            return ApprovalState.EDITED
        return ApprovalState.DENIED


# ---------------------------------------------------------------------------
# High-level helper
# ---------------------------------------------------------------------------


def request_approval(
    bus: ApprovalBus,
    request: ApprovalRequest,
    *,
    stop_event: threading.Event | None = None,
    poll_interval: float = 0.05,
) -> ApprovalResponse:
    """Submit ``request`` and block until the human responds or it times out.

    The function respects:

      - ``request.timeout_seconds``: how long to wait. ``0`` = forever.
      - ``stop_event``: external cancellation (e.g. session shutdown).

    On timeout the returned :class:`ApprovalResponse` has
    ``decision = DENY`` and a synthetic ``comment`` so the caller can
    distinguish "no answer" from "explicit deny".
    """
    pending = bus.submit(request)
    deadline = time.monotonic() + request.timeout_seconds if request.timeout_seconds > 0 else None
    while True:
        if stop_event is not None and stop_event.is_set():
            bus.cancel(request.request_id)
            return ApprovalResponse(
                request_id=request.request_id,
                decision=ApprovalDecision.DENY,
                comment="cancelled by stop_event",
            )
        if deadline is not None and time.monotonic() >= deadline:
            bus.cancel(request.request_id)
            return ApprovalResponse(
                request_id=request.request_id,
                decision=ApprovalDecision.DENY,
                comment="timed out waiting for human approval",
            )
        if pending.event.wait(timeout=poll_interval):
            break
    response = pending.response
    if response is None:
        return ApprovalResponse(
            request_id=request.request_id,
            decision=ApprovalDecision.DENY,
            comment="cancelled (no response)",
        )
    return response


__all__ = [
    "ApprovalBus",
    "InMemoryApprovalBus",
    "PendingRequest",
    "request_approval",
]
