"""Tests for the W12 ReAct agent.

These tests use a scripted adapter (registered per-test) to avoid real
LLM calls. The adapter answers ReAct-style prompts deterministically so
we can assert on tool-call sequences, loop detection, and reflection
emissions.
"""

from __future__ import annotations

import threading
from typing import Any

import pytest

from hnsx_worker.adapters import AdapterRegistry
from hnsx_worker.adapters.base import Adapter, AdapterResult, Cost, StreamChunk
from hnsx_worker.agents import build_react_agent


# ---------------------------------------------------------------------------
# Scripted adapter fixture
# ---------------------------------------------------------------------------


@pytest.fixture
def scripted_react_adapter() -> Any:
    """Adapter whose behaviour is driven by ``agent['_react_script']``."""

    class _ScriptedReactAdapter(Adapter):
        def name(self) -> str:
            return "scripted_react"

        def invoke(self, agent: dict, prompt: str, input: dict) -> AdapterResult:
            script = agent.get("_react_script", [])
            idx = agent.setdefault("_react_idx", 0)
            entry: Any = script[idx] if idx < len(script) else {"text": "ok"}
            agent["_react_idx"] = idx + 1
            if isinstance(entry, str):
                return AdapterResult(text=entry, cost=Cost())
            return AdapterResult(
                text=entry.get("text", ""),
                tool_calls=list(entry.get("tool_calls", [])),
                cost=Cost(),
            )

        def invoke_stream(self, agent: dict, prompt: str, input: dict):
            result = self.invoke(agent, prompt, input)
            yield StreamChunk(text_delta=result.text)
            if result.tool_calls:
                for tc in result.tool_calls:
                    yield StreamChunk(tool_call=tc)
            yield StreamChunk(cost=result.cost or Cost())

    AdapterRegistry.register("scripted_react", _ScriptedReactAdapter)
    yield _ScriptedReactAdapter
    AdapterRegistry.reset()


def _base_spec(orchestration: dict | None = None) -> dict:
    return {
        "id": "react-demo",
        "harness": {
            "agents": {
                "primary": {
                    "id": "primary",
                    "adapter": {"kind": "scripted_react"},
                    "system_prompt": "you are the agent",
                }
            },
            "session": {"mode": "multi-turn", "agent": "primary"},
            "orchestration": orchestration or {"strategy": "react"},
        },
    }


def _make_tool_call(name: str, input: dict, tc_id: str = "tc-1") -> Any:
    from hnsx_worker.adapters.base import ToolCall

    return ToolCall(id=tc_id, name=name, input=input, raw_input="")


# ---------------------------------------------------------------------------
# ReAct loop basics
# ---------------------------------------------------------------------------


def test_react_runs_until_no_tool_calls(scripted_react_adapter: Any) -> None:
    spec = _base_spec()
    agent_cfg = spec["harness"]["agents"]["primary"]
    agent_cfg["_react_script"] = [
        {"text": "first thought", "tool_calls": [_make_tool_call("search", {"q": "a"})]},
        {"text": "second thought", "tool_calls": [_make_tool_call("search", {"q": "b"})]},
        {"text": "final answer"},
    ]
    config = {"session_id": "s1"}
    observations: list[dict] = []

    def emit(obs: dict) -> None:
        observations.append(obs)

    agent = build_react_agent(spec=spec, config=config, emit=emit)
    out = agent.run({"content": "what is foo?"}, stop_event=threading.Event())
    assert out == "final answer"
    kinds = [o["kind"] for o in observations]
    assert "plan_start" in kinds
    assert "plan_end" in kinds
    assert "react_step" in kinds
    assert observations.count({"kind": "tool_result"}) or any(
        o["kind"] == "tool_result" for o in observations
    )


def test_react_loop_detection_emits_react_loop(scripted_react_adapter: Any) -> None:
    spec = _base_spec(
        {"strategy": "react", "react": {"max_steps": 6, "loop_threshold": 3}}
    )
    agent_cfg = spec["harness"]["agents"]["primary"]
    # Same tool call repeated → should fire loop detector after 3 in a row.
    same = _make_tool_call("noop_tool", {"x": 1}, tc_id="tc-loop")
    agent_cfg["_react_script"] = [
        {"text": "trying", "tool_calls": [same]},
        {"text": "trying", "tool_calls": [same]},
        {"text": "trying", "tool_calls": [same]},
        {"text": "shouldn't reach"},
    ]
    config = {"session_id": "s2"}
    observations: list[dict] = []
    agent = build_react_agent(
        spec=spec, config=config, emit=observations.append
    )
    out = agent.run({"content": "loop?"}, stop_event=threading.Event())
    assert out == ""
    assert any(o["kind"] == "react_loop" for o in observations)


def test_react_reflection_when_enabled(scripted_react_adapter: Any) -> None:
    spec = _base_spec(
        {
            "strategy": "react",
            "react": {"max_steps": 3, "reflection": True},
        }
    )
    agent_cfg = spec["harness"]["agents"]["primary"]
    # Reflection calls invoke(prompt, {"reflection": True}) — answer with
    # an off-track JSON so we can verify the observation.
    agent_cfg["_react_script"] = [
        {"text": "trying", "tool_calls": [_make_tool_call("tool_a", {"k": 1})]},
        {"text": "off-track signal"},
        {"text": "ok", "tool_calls": []},
        {"text": '`{"on_track": false, "reason": "stuck", "revised_plan": ["retry"]}`'},
    ]
    config = {"session_id": "s3"}
    observations: list[dict] = []
    agent = build_react_agent(
        spec=spec, config=config, emit=observations.append
    )
    agent.run({"content": "do thing"}, stop_event=threading.Event())
    assert any(o["kind"] == "reflection" for o in observations)


def test_react_builder_requires_agent(scripted_react_adapter: Any) -> None:
    spec = {
        "id": "no-agent",
        "harness": {
            "agents": {},
            "session": {"mode": "multi-turn"},
            "orchestration": {"strategy": "react"},
        },
    }
    from hnsx_worker.agents.base import AgentError

    with pytest.raises(AgentError):
        build_react_agent(spec=spec, config={}, emit=lambda _o: None)