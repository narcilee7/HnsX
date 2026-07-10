"""Tests for the W14 human-in-the-loop approval module."""

from __future__ import annotations

import threading
import time
from typing import Any

import pytest

from hnsx_worker.approval import (
    ApprovalBus,
    ApprovalDecision,
    ApprovalRequest,
    ApprovalResponse,
    ApprovalState,
    InMemoryApprovalBus,
    PendingRequest,
    request_approval,
)
from hnsx_worker.tools import (
    HumanApprovalTool,
    HumanApprovalToolConfig,
    ToolContext,
    ToolResult,
    build_human_approval_tool,
)


# ---------------------------------------------------------------------------
# Protocol basics
# ---------------------------------------------------------------------------


def _sample_request(**overrides: Any) -> ApprovalRequest:
    payload: dict[str, Any] = {
        "session_id": "s1",
        "domain_id": "d1",
        "agent_id": "a1",
        "reason": "approve refund",
        "options": ["approve", "deny"],
    }
    payload.update(overrides)
    return ApprovalRequest(**payload)


def test_request_to_dict_round_trip() -> None:
    r = _sample_request(severity="high", draft="refund email")
    raw = r.to_dict()
    rebuilt = ApprovalRequest.from_dict(raw)
    assert rebuilt.session_id == r.session_id
    assert rebuilt.reason == r.reason
    assert rebuilt.severity == "high"
    assert rebuilt.draft == "refund email"


def test_response_allowed_when_approve() -> None:
    r = ApprovalResponse(
        request_id="x", decision=ApprovalDecision.APPROVE, decided_by="alice"
    )
    assert r.allowed is True


def test_response_not_allowed_when_deny_or_timeout() -> None:
    for decision in (ApprovalDecision.DENY,):
        r = ApprovalResponse(request_id="x", decision=decision)
        assert r.allowed is False


# ---------------------------------------------------------------------------
# In-memory bus
# ---------------------------------------------------------------------------


def test_bus_submit_responds_match() -> None:
    bus = InMemoryApprovalBus()
    req = _sample_request()
    pending = bus.submit(req)
    bus.respond(ApprovalResponse(request_id=req.request_id, decision=ApprovalDecision.APPROVE))
    pending.event.wait(timeout=1)
    assert pending.state == ApprovalState.APPROVED
    assert pending.response is not None
    assert pending.response.decision is ApprovalDecision.APPROVE


def test_bus_respond_unknown_request_returns_false() -> None:
    bus = InMemoryApprovalBus()
    assert (
        bus.respond(
            ApprovalResponse(request_id="nope", decision=ApprovalDecision.APPROVE)
        )
        is False
    )


def test_bus_cancel_sets_state() -> None:
    bus = InMemoryApprovalBus()
    req = _sample_request()
    pending = bus.submit(req)
    assert bus.cancel(req.request_id) is True
    pending.event.wait(timeout=1)
    assert pending.state == ApprovalState.CANCELLED
    assert bus.cancel(req.request_id) is False  # already cancelled


def test_bus_emits_observations() -> None:
    observations: list[dict] = []
    bus = InMemoryApprovalBus(emit=observations.append)
    req = _sample_request()
    bus.submit(req)
    bus.respond(ApprovalResponse(request_id=req.request_id, decision=ApprovalDecision.APPROVE))
    kinds = [o["kind"] for o in observations]
    assert "approval_requested" in kinds
    assert "approval_received" in kinds


# ---------------------------------------------------------------------------
# request_approval helper
# ---------------------------------------------------------------------------


def test_request_approval_blocks_until_response() -> None:
    bus = InMemoryApprovalBus()
    req = _sample_request()

    def respond_later() -> None:
        time.sleep(0.05)
        bus.respond(
            ApprovalResponse(
                request_id=req.request_id, decision=ApprovalDecision.APPROVE
            )
        )

    threading.Thread(target=respond_later, daemon=True).start()
    response = request_approval(bus, req)
    assert response.allowed is True


def test_request_approval_respects_stop_event() -> None:
    bus = InMemoryApprovalBus()
    stop = threading.Event()
    req = _sample_request()
    stop.set()
    response = request_approval(bus, req, stop_event=stop)
    assert response.decision is ApprovalDecision.DENY
    assert "cancelled" in response.comment.lower()


def test_request_approval_times_out() -> None:
    bus = InMemoryApprovalBus()
    req = _sample_request(timeout_seconds=0.1)
    started = time.monotonic()
    response = request_approval(bus, req, poll_interval=0.02)
    elapsed = time.monotonic() - started
    assert response.decision is ApprovalDecision.DENY
    assert "timed out" in response.comment
    assert elapsed < 1.0


def test_request_approval_returns_explicit_deny() -> None:
    bus = InMemoryApprovalBus()
    req = _sample_request()

    def respond_now() -> None:
        time.sleep(0.05)
        bus.respond(
            ApprovalResponse(
                request_id=req.request_id,
                decision=ApprovalDecision.DENY,
                decided_by="bob",
                comment="no",
            )
        )

    threading.Thread(target=respond_now, daemon=True).start()
    response = request_approval(bus, req, poll_interval=0.01)
    assert response.decision is ApprovalDecision.DENY
    assert response.decided_by == "bob"
    assert response.comment == "no"


# ---------------------------------------------------------------------------
# HumanApprovalTool
# ---------------------------------------------------------------------------


def _ctx() -> ToolContext:
    return ToolContext(
        session_id="s1",
        domain_id="d1",
        agent_id="a1",
        turn=1,
        tool_call_id="t1",
        secrets={},
        emit=lambda _o: None,
    )


def test_tool_returns_error_when_denied() -> None:
    bus = InMemoryApprovalBus()
    tool = build_human_approval_tool(
        "request_human_approval", {}, bus=bus, stop_event=threading.Event()
    )

    def respond() -> None:
        time.sleep(0.05)
        pending = bus.pending()
        bus.respond(
            ApprovalResponse(
                request_id=pending[0].request.request_id,
                decision=ApprovalDecision.DENY,
            )
        )

    threading.Thread(target=respond, daemon=True).start()
    result = tool.invoke(_ctx(), {"reason": "approve refund"})
    assert result.ok is False
    assert "deny" in result.error
    assert result.output["decision"] == "deny"


def test_tool_returns_output_when_approved() -> None:
    bus = InMemoryApprovalBus()
    tool = build_human_approval_tool(
        "request_human_approval", {}, bus=bus, stop_event=threading.Event()
    )

    def respond() -> None:
        time.sleep(0.05)
        pending = bus.pending()
        bus.respond(
            ApprovalResponse(
                request_id=pending[0].request.request_id,
                decision=ApprovalDecision.APPROVE,
                chosen_option="approve",
                decided_by="alice",
            )
        )

    threading.Thread(target=respond, daemon=True).start()
    result = tool.invoke(_ctx(), {"reason": "send email"})
    assert result.ok is True
    assert result.output["decision"] == "approve"
    assert result.output["chosen_option"] == "approve"
    assert result.metadata["decided_by"] == "alice"


def test_tool_returns_edited_draft() -> None:
    bus = InMemoryApprovalBus()
    tool = build_human_approval_tool(
        "request_human_approval", {}, bus=bus, stop_event=threading.Event()
    )

    def respond() -> None:
        time.sleep(0.05)
        pending = bus.pending()
        bus.respond(
            ApprovalResponse(
                request_id=pending[0].request.request_id,
                decision=ApprovalDecision.EDIT,
                edited_draft="rewritten message",
                decided_by="alice",
            )
        )

    threading.Thread(target=respond, daemon=True).start()
    result = tool.invoke(_ctx(), {"reason": "review", "draft": "original"})
    assert result.ok is True
    assert result.output["edited_draft"] == "rewritten message"
    assert result.metadata["was_edit"] is True


def test_tool_validates_input() -> None:
    bus = InMemoryApprovalBus()
    tool = build_human_approval_tool(
        "request_human_approval", {}, bus=bus, stop_event=threading.Event()
    )
    result = tool.invoke(_ctx(), {})
    assert result.error is not None
    assert "reason" in result.error


def test_tool_respects_timeout() -> None:
    bus = InMemoryApprovalBus()
    tool = build_human_approval_tool(
        "request_human_approval",
        {"default_timeout_seconds": 0.05},
        bus=bus,
        stop_event=threading.Event(),
    )
    started = time.monotonic()
    result = tool.invoke(_ctx(), {"reason": "no response"})
    elapsed = time.monotonic() - started
    assert result.ok is False
    assert "timed out" in (result.error or "")
    assert elapsed < 1.0