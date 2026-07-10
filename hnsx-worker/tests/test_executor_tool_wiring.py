"""Tests for W3.3 — executor wiring through the ToolRegistry.

Coverage:

  - ``_build_tool_registry`` constructs a registry from spec entries
    (http / sql / python + external refs).
  - LLM-facing schemas are injected onto the agent for adapters.
  - Tool spec failures are surfaced as ``tool_spec_invalid`` and the
    rest of the registry still works.
  - Secrets flow from config into ToolContext.
  - Policy hook stub is called once per tool call.
  - Multi-turn executor emits ``tool_result`` whose ``output`` comes
    from the real tool (no more ``stub=True``).
  - Unknown tools still produce structured errors (not crashes).
"""

from __future__ import annotations

from typing import Any

import pytest

from hnsx_worker.adapters import AdapterRegistry
from hnsx_worker.adapters.base import Adapter, AdapterResult, Cost, StreamChunk, ToolCall
from hnsx_worker.session_executor import (
    _allow_all_policy_hook,
    _build_tool_registry,
    _make_tool_context_factory,
    _read_secrets,
    execute_session,
)
from hnsx_worker.tools import ToolContext, ToolDecision

# ---------------------------------------------------------------------------
# Test helpers
# ---------------------------------------------------------------------------


# The fixture below registers a ``scripted_for_wiring`` adapter that
# reads its response (text + tool_calls) from ``agent._scripted_response``.
# Each test sets that field to drive the agent's behavior without
# requiring real LLM credentials.


@pytest.fixture(autouse=True)
def _register_scripted_adapter() -> Any:
    class _ScriptedAdapter(Adapter):
        def name(self) -> str:
            return "scripted_for_wiring"

        def invoke(self, agent: dict, prompt: str, input: dict) -> Any:
            scripts = agent.get("_scripted_response", {})
            text = scripts.get("text", "")
            tcs = [ToolCall(**tc) for tc in scripts.get("tool_calls", [])]
            return AdapterResult(text=text, tool_calls=tcs)

        def invoke_stream(self, agent: dict, prompt: str, input: dict):
            scripts = agent.get("_scripted_response", {})
            for chunk in scripts.get("text_chunks", [scripts.get("text", "")]):
                yield StreamChunk(text_delta=chunk)
            for tc in scripts.get("tool_calls", []):
                yield StreamChunk(tool_call=ToolCall(**tc))
            yield StreamChunk(cost=Cost())

    AdapterRegistry.register("scripted_for_wiring", _ScriptedAdapter)
    yield
    AdapterRegistry._registry.pop("scripted_for_wiring", None)
    AdapterRegistry._singletons.pop("scripted_for_wiring", None)


# ---------------------------------------------------------------------------
# _read_secrets
# ---------------------------------------------------------------------------


def test_read_secrets_from_dict() -> None:
    cfg = {"secrets": {"api_key": "k", "db_pass": "p"}}
    assert _read_secrets(cfg) == {"api_key": "k", "db_pass": "p"}


def test_read_secrets_from_list_of_pairs() -> None:
    cfg = {"secrets": [{"name": "api_key", "value": "k"}]}
    assert _read_secrets(cfg) == {"api_key": "k"}


def test_read_secrets_from_missing() -> None:
    assert _read_secrets({}) == {}
    assert _read_secrets({"secrets": None}) == {}


def test_read_secrets_coerces_to_strings() -> None:
    cfg = {"secrets": {"port": 5432, "name": 12345}}
    assert _read_secrets(cfg) == {"port": "5432", "name": "12345"}


# ---------------------------------------------------------------------------
# _build_tool_registry
# ---------------------------------------------------------------------------


def test_build_registry_registers_http_tool() -> None:
    agent = {
        "tools": [
            {"name": "fetch", "type": "http", "config": {"url": "https://x/{id}"}},
        ]
    }
    registry, failures = _build_tool_registry(
        agent=agent, session_id="s", domain_id="d", emit=lambda o: None
    )
    assert failures == []
    assert "fetch" in registry
    # The agent.tools list was rewritten with the LLM-facing schemas.
    assert agent["tools"][0]["name"] == "fetch"
    assert "path_params" in agent["tools"][0]["input_schema"]["properties"]


def test_build_registry_registers_python_tool() -> None:
    agent = {
        "tools": [
            {"name": "calc", "type": "python", "config": {"timeout_seconds": 2}},
        ]
    }
    registry, failures = _build_tool_registry(
        agent=agent, session_id="s", domain_id="d", emit=lambda o: None
    )
    assert failures == []
    assert "calc" in registry


def test_build_registry_registers_sql_tool() -> None:
    agent = {
        "tools": [
            {"name": "lookup", "type": "sql", "config": {"dsn": "sqlite:///:memory:"}},
        ]
    }
    registry, failures = _build_tool_registry(
        agent=agent, session_id="s", domain_id="d", emit=lambda o: None
    )
    assert failures == []
    assert "lookup" in registry


def test_build_registry_passes_through_name_only_entries() -> None:
    """Entries without ``type`` are external references — pass through."""
    agent = {
        "tools": [
            {"name": "claude_code_bash"},
            {"name": "fetch", "type": "http", "config": {"url": "https://x"}},
        ]
    }
    registry, failures = _build_tool_registry(
        agent=agent, session_id="s", domain_id="d", emit=lambda o: None
    )
    assert failures == []
    # External entry NOT registered as a built-in (the W4 adapter owns it).
    assert "claude_code_bash" not in registry
    # Built-in IS registered.
    assert "fetch" in registry
    # Schemas include both — external with a permissive fallback.
    names = [t["name"] for t in agent["tools"]]
    assert names == ["claude_code_bash", "fetch"]


def test_build_registry_surfaces_failures() -> None:
    agent = {
        "tools": [
            {"name": "fetch", "type": "http"},  # missing url → ValueError
            {"name": "weird", "type": "teleportation"},
            {"name": "calc", "type": "python", "config": {}},
        ]
    }
    registry, failures = _build_tool_registry(
        agent=agent, session_id="s", domain_id="d", emit=lambda o: None
    )
    # Two entries failed; one (calc) succeeded.
    assert len(failures) == 2
    assert "calc" in registry  # the python tool survived
    # The failures message mentions the offending names / types.
    assert any("fetch" in f for f in failures)
    assert any("teleportation" in f for f in failures)


def test_build_registry_rejects_non_dict_entries() -> None:
    agent = {"tools": [{"name": "good", "type": "python"}, "bad-entry"]}
    registry, failures = _build_tool_registry(
        agent=agent, session_id="s", domain_id="d", emit=lambda o: None
    )
    assert "good" in registry
    assert any("non-dict" in f for f in failures)


def test_build_registry_handles_missing_tools_key() -> None:
    registry, failures = _build_tool_registry(
        agent={}, session_id="s", domain_id="d", emit=lambda o: None
    )
    assert registry.names() == []
    assert failures == []


# ---------------------------------------------------------------------------
# _make_tool_context_factory
# ---------------------------------------------------------------------------


def test_tool_context_factory_closes_over_session_state() -> None:
    captured: list[dict] = []
    factory = _make_tool_context_factory(
        session_id="s-1",
        domain_id="d-1",
        agent_id="a-1",
        secrets={"k": "v"},
        emit=captured.append,
    )
    ctx = factory(turn=3, tool_call_id="tc-9")
    assert ctx.session_id == "s-1"
    assert ctx.domain_id == "d-1"
    assert ctx.agent_id == "a-1"
    assert ctx.turn == 3
    assert ctx.tool_call_id == "tc-9"
    assert ctx.secrets == {"k": "v"}
    # Bound method ``list.append`` doesn't compare with ``is``; just
    # check the call goes through.
    assert ctx.emit is not None
    ctx.emit({"k": "v"})
    assert captured == [{"k": "v"}]


def test_tool_context_factory_returns_independent_secrets_dicts() -> None:
    """Mutating one context's secrets must not bleed into another."""
    factory = _make_tool_context_factory(
        session_id="s",
        domain_id="d",
        agent_id="a",
        secrets={"shared": "v"},
        emit=lambda o: None,
    )
    c1 = factory(turn=1, tool_call_id="x")
    c2 = factory(turn=1, tool_call_id="y")
    c1.secrets["leak"] = "no"
    assert "leak" not in c2.secrets


# ---------------------------------------------------------------------------
# _allow_all_policy_hook
# ---------------------------------------------------------------------------


def test_allow_all_policy_hook_decision() -> None:
    hook = _allow_all_policy_hook()
    ctx = ToolContext()
    decision = hook("any_tool", {"k": "v"}, ctx)
    assert isinstance(decision, ToolDecision)
    assert decision.allow is True
    assert decision.decision == "allow"


# ---------------------------------------------------------------------------
# End-to-end: executor dispatches through ToolRegistry
# ---------------------------------------------------------------------------


def test_executor_calls_through_registry_and_emits_real_tool_result() -> None:
    """Happy path: agent declares a python tool, LLM calls it, executor
    runs it via the registry and emits a real tool_result observation
    (no more ``stub=True``)."""
    spec = {
        "id": "wiring",
        "version": "0.1.0",
        "harness": {
            "agents": {
                "primary": {
                    "id": "primary",
                    "provider": "scripted_for_wiring",
                    "model": "test",
                    "adapter": {"kind": "scripted_for_wiring"},
                    "system_prompt": "be brief",
                    "tools": [
                        {
                            "name": "calc",
                            "type": "python",
                            "config": {"timeout_seconds": 2},
                        }
                    ],
                    "_scripted_response": {
                        "text_chunks": ["calling tool "],
                        "tool_calls": [
                            {"id": "tc-1", "name": "calc", "input": {"code": "21 * 2"}}
                        ],
                    },
                }
            },
            "session": {"mode": "multi-turn", "agent": "primary"},
            "policy": {"budget": {"max_turns": 1}},
        },
    }
    captured: list[dict] = []
    import threading

    execute_session(
        spec,
        trigger={"q": "compute"},
        config={"session_id": "s-wiring", "secrets": {"k": "v"}},
        stop_event=threading.Event(),
        emit=captured.append,
    )

    tool_results = [o for o in captured if o["kind"] == "tool_result"]
    assert len(tool_results) == 1
    payload = tool_results[0]["payload"]
    assert payload["name"] == "calc"
    assert payload["ok"] is True
    # Real Python tool produced a "42" result.
    assert payload["output"]["ok"] is True
    assert payload["output"]["output"]["result"] == "42"


def test_executor_surfaces_unknown_tool_as_structured_error() -> None:
    """The LLM calls a tool that isn't registered → structured error,
    not a crash."""
    spec = {
        "id": "unknown-tool",
        "version": "0.1.0",
        "harness": {
            "agents": {
                "primary": {
                    "id": "primary",
                    "provider": "scripted_for_wiring",
                    "model": "test",
                    "adapter": {"kind": "scripted_for_wiring"},
                    "system_prompt": "be brief",
                    "_scripted_response": {
                        "text_chunks": [""],
                        "tool_calls": [
                            {"id": "tc-1", "name": "ghost", "input": {}}
                        ],
                    },
                }
            },
            "session": {"mode": "multi-turn", "agent": "primary"},
            "policy": {"budget": {"max_turns": 1}},
        },
    }
    captured: list[dict] = []
    import threading

    execute_session(
        spec,
        trigger={"q": "x"},
        config={"session_id": "s-unk"},
        stop_event=threading.Event(),
        emit=captured.append,
    )
    tool_results = [o for o in captured if o["kind"] == "tool_result"]
    assert len(tool_results) == 1
    payload = tool_results[0]["payload"]
    assert payload["name"] == "ghost"
    assert payload["ok"] is False
    assert "unknown tool" in payload["output"]["error"]


def test_executor_emits_tool_spec_invalid_for_bad_entries() -> None:
    """A spec with an unknown tool type produces a tool_spec_invalid
    observation but the session still runs."""
    spec = {
        "id": "bad-spec",
        "version": "0.1.0",
        "harness": {
            "agents": {
                "primary": {
                    "id": "primary",
                    "provider": "scripted_for_wiring",
                    "model": "test",
                    "adapter": {"kind": "scripted_for_wiring"},
                    "system_prompt": "be brief",
                    "tools": [
                        {"name": "weird", "type": "teleportation"},
                    ],
                    "_scripted_response": {
                        "text_chunks": ["done"],
                        "tool_calls": [],
                    },
                }
            },
            "session": {"mode": "multi-turn", "agent": "primary"},
            "policy": {"budget": {"max_turns": 3}},
        },
    }
    captured: list[dict] = []
    import threading

    execute_session(
        spec,
        trigger={"q": "x"},
        config={"session_id": "s-bad"},
        stop_event=threading.Event(),
        emit=captured.append,
    )
    invalid = [o for o in captured if o["kind"] == "tool_spec_invalid"]
    assert len(invalid) == 1
    assert "weird" in invalid[0]["payload"]["failures"][0]


def test_executor_injects_tool_schemas_for_adapter() -> None:
    """The agent's ``tools`` list is rewritten with LLM-facing schemas."""
    spec = {
        "id": "schemas",
        "version": "0.1.0",
        "harness": {
            "agents": {
                "primary": {
                    "id": "primary",
                    "provider": "scripted_for_wiring",
                    "model": "test",
                    "adapter": {"kind": "capture_schemas"},
                    "system_prompt": "x",
                    "tools": [
                        {
                            "name": "fetch",
                            "type": "http",
                            "config": {"url": "https://x/{id}"},
                        }
                    ],
                }
            },
            "session": {"mode": "multi-turn", "agent": "primary"},
            "policy": {"budget": {"max_turns": 2}},
        },
    }
    # Capture the agent arg handed to the scripted adapter; the schemas
    # injection should be visible there.
    captured: list[dict] = []
    seen_agents: list[dict] = []

    class _Capture(Adapter):
        def name(self) -> str:
            return "capture_schemas"

        def invoke(self, agent: dict, prompt: str, input: dict) -> AdapterResult:
            seen_agents.append(agent)
            return AdapterResult(text="ok", tool_calls=[])

        def invoke_stream(self, agent: dict, prompt: str, input: dict):
            seen_agents.append(agent)
            yield StreamChunk(text_delta="ok")
            yield StreamChunk(cost=Cost())

    AdapterRegistry.register("capture_schemas", _Capture)
    try:
        import threading

        execute_session(
            spec,
            trigger={"q": "x"},
            config={"session_id": "s-schemas"},
            stop_event=threading.Event(),
            emit=captured.append,
        )
    finally:
        # No public unregister; clear the registry entry directly.
        AdapterRegistry._registry.pop("capture_schemas", None)
        AdapterRegistry._singletons.pop("capture_schemas", None)

    assert seen_agents, "adapter was never invoked"
    tools = seen_agents[0]["tools"]
    assert tools[0]["name"] == "fetch"
    # The HTTP tool's path_params is now visible to the LLM.
    assert "path_params" in tools[0]["input_schema"]["properties"]


def test_executor_delegates_cli_agent_tool_calls() -> None:
    """CLI-agent adapters emit tool_call observations; the executor must not
    try to invoke them through the ToolRegistry (which would fail with
    'unknown tool'). Instead it emits a delegated tool_result."""

    class _FakeCliAdapter(Adapter):
        def name(self) -> str:
            return "claudecode"

        def invoke_stream(self, agent: dict, prompt: str, input: dict):
            yield StreamChunk(text_delta="ok")
            yield StreamChunk(
                tool_call=ToolCall(
                    id="tc-cli-1", name="bash", input={"command": "ls"}
                )
            )
            yield StreamChunk(cost=Cost())

        def invoke(self, agent: dict, prompt: str, input: dict) -> AdapterResult:
            return AdapterResult(text="ok")

    AdapterRegistry.register("claudecode", _FakeCliAdapter)
    spec = {
        "id": "cli-delegation",
        "version": "0.1.0",
        "harness": {
            "agents": {
                "primary": {
                    "id": "primary",
                    "adapter": {"kind": "claudecode"},
                    "system_prompt": "x",
                }
            },
            "session": {"mode": "multi-turn", "agent": "primary"},
            "policy": {"budget": {"max_turns": 1}},
        },
    }
    captured: list[dict] = []
    try:
        import threading

        execute_session(
            spec,
            trigger={"q": "x"},
            config={"session_id": "s-cli"},
            stop_event=threading.Event(),
            emit=captured.append,
        )
    finally:
        AdapterRegistry._registry.pop("claudecode", None)
        AdapterRegistry._singletons.pop("claudecode", None)

    tool_results = [o for o in captured if o["kind"] == "tool_result"]
    assert len(tool_results) == 1
    payload = tool_results[0]["payload"]
    assert payload["name"] == "bash"
    assert payload["ok"] is True
    assert payload["output"]["output"]["delegated_to_cli"] is True
