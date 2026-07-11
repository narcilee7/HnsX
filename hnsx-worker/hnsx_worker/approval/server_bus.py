"""Server-backed approval bus for worker mode.

The worker subprocess has no local human operator, so it registers approval
requests with the Go control plane over HTTP and polls until the operator
(Console/CLI) resolves them.
"""

from __future__ import annotations

import json
import logging
import threading
import time
import urllib.error
import urllib.request
from typing import Any

from .bus import ApprovalBus, EmitFn, PendingRequest
from .protocol import ApprovalDecision, ApprovalRequest, ApprovalResponse, ApprovalState

log = logging.getLogger("hnsx_worker.approval.server_bus")


class ServerApprovalBus(ApprovalBus):
    """Approval bus that delegates to the HnsX server REST API.

    - ``submit`` creates the approval record on the server (POST /api/v1/approvals).
    - ``get`` polls the server for the current status (GET /api/v1/approvals/:id).
    - ``respond`` is a no-op: the server/Console owns the resolution.
    """

    def __init__(
        self,
        base_url: str,
        *,
        auth_token: str | None = None,
        emit: EmitFn | None = None,
        poll_interval: float = 0.5,
    ) -> None:
        self._base_url = base_url.rstrip("/")
        self._auth_token = auth_token
        self._emit = emit or (lambda _o: None)
        self._poll_interval = poll_interval
        self._pending: dict[str, PendingRequest] = {}
        self._lock = threading.RLock()

    def _request(
        self, method: str, path: str, payload: dict[str, Any] | None = None
    ) -> dict[str, Any] | None:
        url = f"{self._base_url}{path}"
        data: bytes | None = None
        headers: dict[str, str] = {"Accept": "application/json"}
        if payload is not None:
            data = json.dumps(payload).encode("utf-8")
            headers["Content-Type"] = "application/json"
        if self._auth_token:
            headers["Authorization"] = f"Bearer {self._auth_token}"
        req = urllib.request.Request(url, data=data, method=method, headers=headers)
        try:
            with urllib.request.urlopen(req, timeout=10) as resp:
                body = resp.read()
                if not body:
                    return None
                return json.loads(body)
        except urllib.error.HTTPError as e:
            log.warning("approval server %s %s failed: %s %s", method, path, e.code, e.reason)
            try:
                return json.loads(e.read())
            except Exception:  # noqa: BLE001
                return None
        except Exception as e:  # noqa: BLE001
            log.warning("approval server %s %s error: %s", method, path, e)
            return None

    def submit(self, request: ApprovalRequest) -> PendingRequest:
        with self._lock:
            pending = PendingRequest(request=request)
            self._pending[request.request_id] = pending
        payload = {
            "id": request.request_id,
            "session_id": request.session_id,
            "domain_id": request.domain_id,
            "action": f"tool_call:{request.tool_name}" if request.tool_name else "human_approval",
            "resource": request.tool_name or "human_approval",
            "risk_level": request.severity or "high",
            "context": {
                "reason": request.reason,
                "options": request.options,
                "draft": request.draft,
                "tool_input": request.tool_input,
                "agent_id": request.agent_id,
            },
            "requested_by": request.agent_id or "agent",
        }
        resp = self._request("POST", "/api/v1/approvals", payload)
        if resp is None:
            log.error("failed to submit approval request %s", request.request_id)
        else:
            log.info("submitted approval request %s", request.request_id)
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

    def get(self, request_id: str) -> PendingRequest | None:
        with self._lock:
            cached = self._pending.get(request_id)
            if cached is not None and cached.state is not ApprovalState.PENDING:
                return cached
        resp = self._request("GET", f"/api/v1/approvals/{request_id}")
        if resp is None:
            return None
        status = str(resp.get("status", "pending"))
        state = self._state_from_server(status)
        with self._lock:
            pending = self._pending.get(request_id)
            if pending is None:
                return None
            if state is not ApprovalState.PENDING and pending.state is ApprovalState.PENDING:
                decision = ApprovalDecision.APPROVE if state is ApprovalState.APPROVED else ApprovalDecision.DENY
                pending.response = ApprovalResponse(
                    request_id=request_id,
                    decision=decision,
                    chosen_option=decision.value,
                    decided_by=str(resp.get("reviewed_by") or ""),
                    comment=str(resp.get("comment") or ""),
                    decided_at_ms=int(time.time() * 1000),
                )
                pending.state = state
                pending.event.set()
                self._emit(
                    {
                        "kind": "approval_received",
                        "session_id": pending.request.session_id,
                        "domain_id": pending.request.domain_id,
                        "agent_id": pending.request.agent_id,
                        "payload": pending.response.to_dict(),
                    }
                )
            return pending

    def respond(self, response: ApprovalResponse) -> bool:
        # Resolutions come from the server/Console, not the worker.
        return False

    def cancel(self, request_id: str) -> bool:
        with self._lock:
            pending = self._pending.get(request_id)
            if pending is None:
                return False
            if pending.state is not ApprovalState.PENDING:
                return False
            pending.state = ApprovalState.CANCELLED
            pending.event.set()
        return True

    @staticmethod
    def _state_from_server(status: str) -> ApprovalState:
        mapping = {
            "pending": ApprovalState.PENDING,
            "approved": ApprovalState.APPROVED,
            "rejected": ApprovalState.DENIED,
            "expired": ApprovalState.TIMEOUT,
        }
        return mapping.get(status, ApprovalState.PENDING)


__all__ = ["ServerApprovalBus"]
