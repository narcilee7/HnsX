"""Tests for the W12 Multi-Agent runner (delegate_to wiring)."""

from __future__ import annotations

import threading
from typing import Any

import pytest

from hnsx_worker.adapters import AdapterRegistry
from hnsx_worker.adapters.base import Adapter, AdapterResult, Cost, StreamChunk
from hnsx_worker.agents import build_multi_agent_runner
from hnsx_worker.adapters.base import ToolCall


@pytest.fixture
def scripted_multi_adapter() -> Any:
    """Primary agent decides to delegate; sub-agent answers directly."""

    class _ScriptedMultiAdapter(Adapter):
        def name(self) -> str:
            return "scripted_multi"

        def invoke(self, agent: dict, prompt: str, input: dict) -> AdapterResult:
            script = agent.get("_script", [])
            idx = agent.setdefault("_idx", 0)
            entry = script[idx] if idx < len(script) else {"text": "ok"}
            agent["_idx"] = idx + 1
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
            for tc in result.tool_calls:
                yield StreamChunk(tool_call=tc)
            yield StreamChunk(cost=result.cost or Cost())

    AdapterRegistry.register("scripted_multi", _ScriptedMultiAdapter)
    yield _ScriptedMultiAdapter
    AdapterRegistry.reset()


def _two_agent_spec() -> dict:
    return {
        "id": "multi-demo",
        "harness": {
            "agents": {
                "primary": {
                    "id": "primary",
                    "adapter": {"kind": "scripted_multi"},
                    "system_prompt": "you orchestrate",
                    "_script": [
                        {
                            "text": "I'll delegate.",
                            "tool_calls": [
                                ToolCall(
                                    id="tc-d",
                                    name="delegate_to",
                                    input={"agent": "helper", "task": "answer this"},
                                    raw_input="",
                                )
                            ],
                        },
                        {"text": "got the answer: yes"},
                    ],
                },
                "helper": {
                    "id": "helper",
                    "adapter": {"kind": "scripted_multi"},
                    "system_prompt": "you help",
                    "_script": [{"text": "yes"}],
                },
            },
            "session": {"mode": "multi-turn", "agent": "primary"},
            "orchestration": {
                "strategy": "multi_agent",
                "multi_agent": {"allowed_agents": ["helper"]},
            },
        },
    }


def test_multi_agent_runs_delegate_to(scripted_multi_adapter: Any) -> None:
    spec = _two_agent_spec()
    config = {"session_id": "s-m1"}
    observations: list[dict] = []
    agent = build_multi_agent_runner(
        spec=spec, config=config, emit=observations.append
    )
    out = agent.run({"content": "delegate test"}, stop_event=threading.Event())
    assert "got the answer" in out
    kinds = [o["kind"] for o in observations]
    assert "sub_agent_start" in kinds
    assert "sub_agent_end" in kinds
    assert any(
        o["kind"] == "tool_result" and o["payload"]["name"] == "delegate_to"
        for o in observations
    )


def test_multi_agent_default_allowed_agents(scripted_multi_adapter: Any) -> None:
    spec = _two_agent_spec()
    # Drop explicit allowed_agents; should default to all other agents.
    spec["harness"]["orchestration"]["multi_agent"].pop("allowed_agents", None)
    config = {"session_id": "s-m2"}
    observations: list[dict] = []
    agent = build_multi_agent_runner(
        spec=spec, config=config, emit=observations.append
    )
    out = agent.run({"content": "go"}, stop_event=threading.Event())
    assert "got the answer" in out


def test_multi_agent_builder_validates_agents(scripted_multi_adapter: Any) -> None:
    from hnsx_worker.harness.loader import HarnessValidationError, load

    # Single-agent spec + multi_agent strategy must fail at loader level.
    spec = {
        "id": "x",
        "harness": {
            "agents": {"only": {"id": "only", "adapter": {"kind": "noop"}}},
            "orchestration": {"strategy": "multi_agent"},
        },
    }
    with pytest.raises(HarnessValidationError):
        load(spec)


def test_multi_agent_loader_rejects_unknown_strategy() -> None:
    from hnsx_worker.harness.loader import HarnessValidationError, load

    spec = {
        "id": "bad",
        "harness": {
            "agents": {"a": {"id": "a"}, "b": {"id": "b"}},
            "orchestration": {"strategy": "totally_made_up"},
        },
    }
    with pytest.raises(HarnessValidationError):
        load(spec)