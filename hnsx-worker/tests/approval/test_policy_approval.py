"""Tests for the W14 ``policy.approval.required_for`` DSL."""

from __future__ import annotations

import threading

import pytest

from hnsx_worker.approval import ApprovalDecision, ApprovalResponse, InMemoryApprovalBus
from hnsx_worker.policy.engine import (
    ApprovalPolicy,
    PolicyEngine,
    _glob_match,
)
from hnsx_worker.session_executor import _maybe_gate_for_approval
from hnsx_worker.tools import ToolResult


# ---------------------------------------------------------------------------
# Glob matching
# ---------------------------------------------------------------------------


@pytest.mark.parametrize(
    "pattern,value,expected",
    [
        ("user:*", "user:42", True),
        ("user:*", "billing:42", False),
        ("user:42", "user:42", True),
        ("user:42", "user:99", False),
        ("user:*:write", "user:42:write", True),
        ("user:*:write", "user:42:read", False),
        ("", "", True),
        ("*", "anything", True),
        ("user:", "user:", True),
        ("user:", "user", False),
    ],
)
def test_glob_match(pattern: str, value: str, expected: bool) -> None:
    assert _glob_match(pattern, value) is expected


# ---------------------------------------------------------------------------
# ApprovalPolicy
# ---------------------------------------------------------------------------


def test_approval_policy_tools_match() -> None:
    p = ApprovalPolicy(tools={"refund"})
    assert p.requires_approval("refund") == (True, "policy.approval.required_for.tools")
    assert p.requires_approval("http") == (False, "")


def test_approval_policy_resources_match_any_string_input() -> None:
    p = ApprovalPolicy(resources=["user:*"])
    assert p.requires_approval("http", {"path": "user:42"})[0] is True
    assert p.requires_approval("http", {"path": "billing:42"})[0] is False


def test_approval_policy_cost_threshold() -> None:
    p = ApprovalPolicy(cost_threshold_usd=0.5)
    assert p.requires_approval("http", {}, projected_cost_usd=1.0)[0] is True
    assert p.requires_approval("http", {}, projected_cost_usd=0.1)[0] is False


def test_approval_policy_no_match_when_disabled() -> None:
    p = ApprovalPolicy()
    assert p.requires_approval("refund") == (False, "")
    assert p.requires_approval("http", {"x": "y"}) == (False, "")


# ---------------------------------------------------------------------------
# PolicyEngine integration
# ---------------------------------------------------------------------------


def _spec(policy_block: dict) -> dict:
    return {
        "id": "demo",
        "harness": {
            "policy": policy_block,
        },
    }


def test_policy_engine_loads_approval_block() -> None:
    spec = _spec(
        {
            "approval": {
                "required_for": {
                    "tools": ["refund"],
                    "resources": ["user:*"],
                    "cost_threshold_usd": 0.5,
                },
                "default_timeout_seconds": 60,
            }
        }
    )
    engine = PolicyEngine(spec, session_id="s", domain_id="d")
    assert engine.approval_policy.tools == {"refund"}
    assert engine.approval_policy.resources == ["user:*"]
    assert engine.approval_policy.cost_threshold_usd == 0.5
    assert engine.approval_policy.default_timeout_seconds == 60


# ---------------------------------------------------------------------------
# _maybe_gate_for_approval
# ---------------------------------------------------------------------------


def test_gate_returns_none_when_no_policy() -> None:
    result = _maybe_gate_for_approval(
        approval_policy=None,
        approval_bus=None,
        session_id="s",
        domain_id="d",
        agent_id="a",
        tool_name="http",
        tool_input={},
        stop_event=threading.Event(),
    )
    assert result is None


def test_gate_returns_none_when_policy_says_no() -> None:
    p = ApprovalPolicy()
    result = _maybe_gate_for_approval(
        approval_policy=p,
        approval_bus=None,
        session_id="s",
        domain_id="d",
        agent_id="a",
        tool_name="http",
        tool_input={},
        stop_event=threading.Event(),
    )
    assert result is None


def test_gate_returns_error_when_policy_says_yes_but_no_bus() -> None:
    p = ApprovalPolicy(tools={"refund"})
    result = _maybe_gate_for_approval(
        approval_policy=p,
        approval_bus=None,
        session_id="s",
        domain_id="d",
        agent_id="a",
        tool_name="refund",
        tool_input={"amount": 100},
        stop_event=threading.Event(),
    )
    assert isinstance(result, ToolResult)
    assert result.ok is False
    assert "approval_bus" in result.error
    assert result.metadata["approval_required"] is True


def test_gate_blocks_until_approval_then_proceeds() -> None:
    bus = InMemoryApprovalBus()
    p = ApprovalPolicy(tools={"refund"}, default_timeout_seconds=0)

    def respond_after() -> None:
        import time

        time.sleep(0.05)
        # We need the request_id, but we can't know it ahead of time. So we
        # respond to whichever is pending.
        pending = bus.pending()
        assert pending, "expected a pending request"
        bus.respond(
            ApprovalResponse(
                request_id=pending[0].request.request_id,
                decision=ApprovalDecision.APPROVE,
            )
        )

    threading.Thread(target=respond_after, daemon=True).start()
    result = _maybe_gate_for_approval(
        approval_policy=p,
        approval_bus=bus,
        session_id="s",
        domain_id="d",
        agent_id="a",
        tool_name="refund",
        tool_input={"amount": 100},
        stop_event=threading.Event(),
    )
    assert result is None  # approval granted → tool may proceed


def test_gate_returns_error_when_denied() -> None:
    bus = InMemoryApprovalBus()
    p = ApprovalPolicy(tools={"refund"})

    def respond_now() -> None:
        pending = bus.pending()
        bus.respond(
            ApprovalResponse(
                request_id=pending[0].request.request_id,
                decision=ApprovalDecision.DENY,
                comment="too risky",
            )
        )

    # respond_after submit (we submit inside the gate).
    def caller() -> None:
        import time

        time.sleep(0.05)
        respond_now()

    threading.Thread(target=caller, daemon=True).start()
    result = _maybe_gate_for_approval(
        approval_policy=p,
        approval_bus=bus,
        session_id="s",
        domain_id="d",
        agent_id="a",
        tool_name="refund",
        tool_input={},
        stop_event=threading.Event(),
    )
    assert isinstance(result, ToolResult)
    assert result.ok is False
    assert result.metadata["approval_decision"] == "deny"