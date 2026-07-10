"""Tests for W6 — PolicyEngine.
"""

from __future__ import annotations

from hnsx_worker.policy import PolicyEngine
from hnsx_worker.tools import ToolContext


def _capturing_engine(spec: dict) -> tuple[PolicyEngine, list[dict]]:
    captured: list[dict] = []
    engine = PolicyEngine(
        spec,
        session_id="s",
        domain_id="d",
        agent_id="a",
        emit=captured.append,
    )
    return engine, captured


def test_budget_allow_when_unset() -> None:
    engine, captured = _capturing_engine({"harness": {"policy": {}}})
    decision = engine.check_budget()
    assert decision.allow is True
    assert any(o["kind"] == "policy_check" for o in captured)


def test_budget_deny_when_cost_exceeded() -> None:
    spec = {"harness": {"policy": {"budget": {"max_cost_usd": 1.0}}}}
    engine, captured = _capturing_engine(spec)
    engine.add_cost(1.5)
    decision = engine.check_budget()
    assert decision.allow is False
    assert decision.rule == "budget.max_cost_usd"
    assert any(o["kind"] == "policy_violation" for o in captured)


def test_budget_deny_when_turns_exceeded() -> None:
    spec = {"harness": {"policy": {"budget": {"max_turns": 2}}}}
    engine, captured = _capturing_engine(spec)
    engine.budget.turns_used = 3
    decision = engine.check_budget()
    assert decision.allow is False
    assert decision.rule == "budget.max_turns"


def test_tool_allow_by_default() -> None:
    engine, captured = _capturing_engine({"harness": {"policy": {}}})
    decision = engine.check_tool("bash", {})
    assert decision.allow is True


def test_tool_deny_by_denied_list() -> None:
    spec = {
        "harness": {
            "policy": {"permissions": {"denied_tools": ["bash", "file_delete"]}}
        }
    }
    engine, captured = _capturing_engine(spec)
    decision = engine.check_tool("bash", {})
    assert decision.allow is False
    assert decision.rule == "permissions.denied_tools"
    assert any(o["kind"] == "policy_violation" for o in captured)


def test_tool_deny_by_allowed_list() -> None:
    spec = {
        "harness": {
            "policy": {"permissions": {"allowed_tools": ["http_get", "sql_read"]}}
        }
    }
    engine, captured = _capturing_engine(spec)
    assert engine.check_tool("http_get", {}).allow is True
    decision = engine.check_tool("bash", {})
    assert decision.allow is False
    assert decision.rule == "permissions.allowed_tools"


def test_tool_require_approval() -> None:
    spec = {
        "harness": {
            "policy": {
                "permissions": {"require_human_approval": ["file_write", "shell"]}
            }
        }
    }
    engine, captured = _capturing_engine(spec)
    decision = engine.check_tool("file_write", {})
    assert decision.allow is False
    assert decision.decision == "require_approval"


def test_check_tool_accepts_registry_signature() -> None:
    """ToolRegistry calls policy_decision(name, input, ctx)."""
    engine, _ = _capturing_engine({"harness": {"policy": {}}})
    ctx = ToolContext()
    decision = engine.check_tool("bash", {"q": "x"}, ctx)
    assert decision.allow is True


def test_add_cost_updates_budget() -> None:
    spec = {"harness": {"policy": {"budget": {"max_cost_usd": 2.0}}}}
    engine, _ = _capturing_engine(spec)
    engine.add_cost(0.5)
    engine.add_cost(0.3)
    assert engine.budget.cumulative_cost_usd == 0.8
    assert engine.budget.turns_used == 2


def test_output_guardrail_blocks_keyword() -> None:
    spec = {
        "harness": {
            "policy": {"output_guardrails": {"blocked_keywords": ["secret_key"]}}
        }
    }
    engine, captured = _capturing_engine(spec)
    decision = engine.check_output("the secret_key is exposed")
    assert decision.allow is False
    assert decision.rule == "output_guardrails.blocked_keywords"
    assert any(o["kind"] == "policy_violation" for o in captured)


def test_output_guardrail_blocks_pattern() -> None:
    spec = {
        "harness": {
            "policy": {"output_guardrails": {"blocked_patterns": [r"\b\d{4}-\d{4}-\d{4}-\d{4}\b"]}}
        }
    }
    engine, captured = _capturing_engine(spec)
    decision = engine.check_output("my card is 1234-5678-9012-3456")
    assert decision.allow is False
    assert decision.rule == "output_guardrails.blocked_patterns"


def test_output_guardrail_allows_clean_text() -> None:
    spec = {"harness": {"policy": {"output_guardrails": {"blocked_keywords": ["bad"]}}}}
    engine, captured = _capturing_engine(spec)
    decision = engine.check_output("this is fine")
    assert decision.allow is True
    assert any(o["kind"] == "policy_check" for o in captured)
