"""Tests for the W12 Plan-and-Solve agent."""

from __future__ import annotations

import json
import threading
from typing import Any

import pytest

from hnsx_worker.adapters import AdapterRegistry
from hnsx_worker.adapters.base import Adapter, AdapterResult, Cost, StreamChunk
from hnsx_worker.agents import build_plan_and_solve_agent
from hnsx_worker.agents.base import parse_plan


@pytest.fixture
def scripted_plan_adapter() -> Any:
    class _ScriptedPlanAdapter(Adapter):
        def name(self) -> str:
            return "scripted_plan"

        def invoke(self, agent: dict, prompt: str, input: dict) -> AdapterResult:
            # Plan-generation call.
            if input.get("planning"):
                plan = agent.get("_plan", [])
                return AdapterResult(
                    text=json.dumps(plan),
                    cost=Cost(),
                )
            if input.get("replan"):
                revised = agent.get("_revised_plan", [])
                return AdapterResult(
                    text=json.dumps(revised),
                    cost=Cost(),
                )
            # Step execution: just echo back the step content.
            text = (
                input.get("content", "")
                if isinstance(input, dict)
                else ""
            )
            return AdapterResult(text=f"done: {text}", cost=Cost())

        def invoke_stream(self, agent: dict, prompt: str, input: dict):
            result = self.invoke(agent, prompt, input)
            yield StreamChunk(text_delta=result.text)
            yield StreamChunk(cost=result.cost or Cost())

    AdapterRegistry.register("scripted_plan", _ScriptedPlanAdapter)
    yield _ScriptedPlanAdapter
    AdapterRegistry.reset()


def _spec(orchestration: dict | None = None) -> dict:
    return {
        "id": "plan-demo",
        "harness": {
            "agents": {
                "primary": {
                    "id": "primary",
                    "adapter": {"kind": "scripted_plan"},
                    "system_prompt": "you are the planner",
                }
            },
            "session": {"mode": "multi-turn", "agent": "primary"},
            "orchestration": orchestration or {"strategy": "plan_and_solve"},
        },
    }


def test_parse_plan_accepts_bare_json_array() -> None:
    assert parse_plan('["a", "b", "c"]') == ["a", "b", "c"]


def test_parse_plan_accepts_object_with_plan_key() -> None:
    assert parse_plan('plan: {"plan": ["x", "y"]}') == ["x", "y"]


def test_parse_plan_accepts_markdown_bullets() -> None:
    md = "Here is the plan:\n- first\n- second\n1. third\n"
    assert parse_plan(md) == ["first", "second", "third"]


def test_parse_plan_empty_when_nothing_parsed() -> None:
    assert parse_plan("") == []
    assert parse_plan("not a plan") == []


def test_plan_and_solve_emits_step_observations(scripted_plan_adapter: Any) -> None:
    spec = _spec()
    spec["harness"]["agents"]["primary"]["_plan"] = [
        "research the topic",
        "summarize findings",
    ]
    config = {"session_id": "s-p1"}
    observations: list[dict] = []

    def emit(obs: dict) -> None:
        observations.append(obs)

    agent = build_plan_and_solve_agent(
        spec=spec, config=config, emit=emit
    )
    out = agent.run({"content": "analyze thing"}, stop_event=threading.Event())
    assert "research the topic" in out
    assert "summarize findings" in out
    kinds = [o["kind"] for o in observations]
    assert "plan_start" in kinds
    assert "plan_complete" in kinds
    assert kinds.count("plan_step_start") == 2
    assert kinds.count("plan_step_end") == 2


def test_plan_and_solve_handles_empty_plan(scripted_plan_adapter: Any) -> None:
    from hnsx_worker.agents.base import AgentError

    spec = _spec()
    spec["harness"]["agents"]["primary"]["_plan"] = []
    config = {"session_id": "s-p2"}
    agent = build_plan_and_solve_agent(
        spec=spec, config=config, emit=lambda _o: None
    )
    with pytest.raises(AgentError):
        agent.run({"content": "nothing"}, stop_event=threading.Event())