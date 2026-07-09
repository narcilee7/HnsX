"""Tests for the Tool / ToolRegistry foundation (W3.1).

Covers:

  - Basic register / get / contains / names.
  - Dispatch with policy allow.
  - Dispatch with policy deny → emits ``policy_violation`` + returns error.
  - Unknown tool returns a structured error (does not raise).
  - ToolResult serialization shape.
  - ToolContext wires session / turn / secrets / emit to the tool.
"""

from __future__ import annotations

from typing import Any

import pytest

from hnsx_worker.tools import (
    Tool,
    ToolContext,
    ToolDecision,
    ToolRegistry,
    ToolResult,
)


# ---------------------------------------------------------------------------
# test doubles
# ---------------------------------------------------------------------------


class _EchoTool(Tool):
    """Returns its input dict wrapped in ``{"echo": ...}``."""

    @property
    def name(self) -> str:
        return "echo"

    @property
    def schema(self) -> dict[str, Any]:
        return {
            "type": "object",
            "properties": {"text": {"type": "string"}},
            "required": ["text"],
        }

    def invoke(self, ctx: ToolContext, input: dict[str, Any]) -> ToolResult:
        return ToolResult(output={"echo": input, "turn": ctx.turn})


class _SecretReadTool(Tool):
    """Reads ``ctx.secrets[input["name"]]`` and returns the value."""

    @property
    def name(self) -> str:
        return "secret_read"

    def invoke(self, ctx: ToolContext, input: dict[str, Any]) -> ToolResult:
        name = input.get("name", "")
        if name not in ctx.secrets:
            return ToolResult(error=f"no secret named {name!r}")
        return ToolResult(output={"name": name, "value": ctx.secrets[name]})


class _FailingTool(Tool):
    """Returns a structured failure (does not raise)."""

    @property
    def name(self) -> str:
        return "failing"

    def invoke(self, ctx: ToolContext, input: dict[str, Any]) -> ToolResult:
        return ToolResult(error="intentional failure", metadata={"input": input})


# ---------------------------------------------------------------------------
# registry basics
# ---------------------------------------------------------------------------


def test_register_and_get() -> None:
    reg = ToolRegistry()
    reg.register(_EchoTool())
    assert "echo" in reg
    assert reg.get("echo") is not None
    assert reg.get("nope") is None
    assert reg.names() == ["echo"]


def test_register_rejects_non_tool() -> None:
    reg = ToolRegistry()
    with pytest.raises(TypeError):
        reg.register("not a tool")  # type: ignore[arg-type]


def test_unregister() -> None:
    reg = ToolRegistry()
    tool = _EchoTool()
    reg.register(tool)
    assert "echo" in reg
    reg.unregister("echo")
    assert "echo" not in reg
    # Re-register same instance is fine.
    reg.register(tool)
    assert "echo" in reg


# ---------------------------------------------------------------------------
# dispatch
# ---------------------------------------------------------------------------


def test_call_invokes_tool_with_context_and_input() -> None:
    reg = ToolRegistry()
    reg.register(_EchoTool())
    ctx = ToolContext(session_id="s-1", domain_id="d-1", agent_id="a", turn=3)
    result = reg.call("echo", ctx, {"text": "hi"})

    assert result.ok
    assert result.error is None
    assert result.output == {"echo": {"text": "hi"}, "turn": 3}


def test_call_unknown_tool_returns_structured_error() -> None:
    reg = ToolRegistry()
    reg.register(_EchoTool())
    result = reg.call("nope", ToolContext(), {})

    assert not result.ok
    assert "unknown tool" in (result.error or "")
    assert result.metadata.get("available_tools") == ["echo"]


def test_call_returns_structured_failure() -> None:
    reg = ToolRegistry()
    reg.register(_FailingTool())
    result = reg.call("failing", ToolContext(), {"x": 1})

    assert not result.ok
    assert result.error == "intentional failure"
    assert result.metadata == {"input": {"x": 1}}


# ---------------------------------------------------------------------------
# policy hook
# ---------------------------------------------------------------------------


def _deny_all(name: str, input: dict, ctx: ToolContext) -> ToolDecision:
    return ToolDecision(allow=False, decision="deny", reason=f"{name} is blocked")


def test_policy_deny_short_circuits_and_emits_violation() -> None:
    reg = ToolRegistry(policy_decision=_deny_all)
    reg.register(_EchoTool())

    observations: list[dict] = []
    ctx = ToolContext(
        session_id="s-1",
        domain_id="d-1",
        agent_id="a",
        turn=2,
        tool_call_id="tc-1",
        emit=lambda o: observations.append(o),
    )
    result = reg.call("echo", ctx, {"text": "hi"})

    assert not result.ok
    assert "denied by policy" in (result.error or "")
    assert result.metadata["decision"] == "deny"
    assert result.metadata["policy_reason"] == "echo is blocked"

    # Exactly one policy_violation observation was emitted.
    assert len(observations) == 1
    obs = observations[0]
    assert obs["kind"] == "policy_violation"
    assert obs["session_id"] == "s-1"
    assert obs["payload"]["tool"] == "echo"
    assert obs["payload"]["tool_call_id"] == "tc-1"
    assert obs["payload"]["decision"] == "deny"
    assert obs["payload"]["reason"] == "echo is blocked"
    assert obs["payload"]["turn"] == 2
    # Raw input keys are recorded (not the values) so audit stays safe.
    assert obs["payload"]["input_keys"] == ["text"]


def test_policy_allow_passes_through() -> None:
    def allow_all(name, input, ctx):
        return ToolDecision(allow=True)

    reg = ToolRegistry(policy_decision=allow_all)
    reg.register(_EchoTool())
    result = reg.call("echo", ToolContext(), {"text": "ok"})
    assert result.ok
    assert result.output == {"echo": {"text": "ok"}, "turn": 0}


def test_policy_hook_returning_non_decision_is_logged_but_not_fatal() -> None:
    """A buggy hook that returns a wrong type shouldn't crash — it should
    warn and let the tool run."""

    def buggy_hook(name, input, ctx):
        return "not a decision"

    reg = ToolRegistry(policy_decision=buggy_hook)
    reg.register(_EchoTool())
    result = reg.call("echo", ToolContext(), {"text": "ok"})
    assert result.ok


# ---------------------------------------------------------------------------
# ToolContext → tool integration
# ---------------------------------------------------------------------------


def test_tool_receives_secrets_via_context() -> None:
    reg = ToolRegistry()
    reg.register(_SecretReadTool())
    ctx = ToolContext(secrets={"api_key": "sk-1234"})
    result = reg.call("secret_read", ctx, {"name": "api_key"})
    assert result.ok
    assert result.output == {"name": "api_key", "value": "sk-1234"}


def test_tool_sees_missing_secret_as_error() -> None:
    reg = ToolRegistry()
    reg.register(_SecretReadTool())
    result = reg.call("secret_read", ToolContext(secrets={}), {"name": "missing"})
    assert not result.ok
    assert "no secret" in (result.error or "")


# ---------------------------------------------------------------------------
# ToolResult serialization
# ---------------------------------------------------------------------------


def test_tool_result_to_observation_payload_ok() -> None:
    r = ToolResult(output={"data": [1, 2, 3]}, metadata={"latency_ms": 12})
    assert r.to_observation_payload() == {
        "ok": True,
        "output": {"data": [1, 2, 3]},
        "metadata": {"latency_ms": 12},
    }


def test_tool_result_to_observation_payload_error() -> None:
    r = ToolResult(error="boom", metadata={"tool": "x"})
    payload = r.to_observation_payload()
    assert payload["ok"] is False
    assert payload["error"] == "boom"
    assert payload["metadata"] == {"tool": "x"}
    assert "output" not in payload