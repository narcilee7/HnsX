"""Tests for the W13 eval_self_check tool."""

from __future__ import annotations

import json

from hnsx_worker.adapters import AdapterRegistry
from hnsx_worker.adapters.base import Adapter, AdapterResult, Cost, StreamChunk
from hnsx_worker.tools import ToolContext, build_tool


class _StubJudgeAdapter(Adapter):
    def __init__(self, text: str = "") -> None:
        self._text = text

    def name(self) -> str:
        return "stub_judge"

    def invoke(self, agent: dict, prompt: str, input: dict) -> AdapterResult:
        return AdapterResult(text=self._text, cost=Cost())

    def invoke_stream(self, agent: dict, prompt: str, input: dict):
        yield StreamChunk(text_delta=self._text)


def _ctx() -> ToolContext:
    return ToolContext(
        session_id="s",
        domain_id="d",
        agent_id="a",
        turn=1,
        tool_call_id="t",
        secrets={},
        emit=lambda _o: None,
    )


def test_self_check_tool_passes_when_score_meets_threshold() -> None:
    class _PassJudge(Adapter):
        def name(self) -> str:
            return "stub_judge_passing"

        def invoke(self, agent: dict, prompt: str, input: dict) -> AdapterResult:
            return AdapterResult(
                text=json.dumps({"score": 0.9, "passed": True, "rationale": "ok"}),
                cost=Cost(),
            )

        def invoke_stream(self, agent: dict, prompt: str, input: dict):
            yield StreamChunk(text_delta="")

    AdapterRegistry.register("stub_judge_passing", _PassJudge)
    try:
        tool = build_tool(
            {
                "name": "eval_self_check",
                "type": "self_check",
                "config": {"rubric": "x", "judge_adapter": "stub_judge_passing", "threshold": 0.7},
            }
        )
        result = tool.invoke(
            _ctx(),
            {"task": "answer X", "candidate_output": "X is Y"},
        )
        assert result.ok
        assert result.output["passed"] is True
        assert result.output["score"] == 0.9
    finally:
        AdapterRegistry.reset()


def test_self_check_tool_fails_when_score_below_threshold() -> None:
    class _FailJudge(Adapter):
        def name(self) -> str:
            return "stub_judge_failing"

        def invoke(self, agent: dict, prompt: str, input: dict) -> AdapterResult:
            return AdapterResult(
                text=json.dumps({"score": 0.3, "passed": False, "rationale": "bad"}),
                cost=Cost(),
            )

        def invoke_stream(self, agent: dict, prompt: str, input: dict):
            yield StreamChunk(text_delta="")

    AdapterRegistry.register("stub_judge_failing", _FailJudge)
    try:
        tool = build_tool(
            {
                "name": "eval_self_check",
                "type": "self_check",
                "config": {"rubric": "x", "judge_adapter": "stub_judge_failing", "threshold": 0.7},
            }
        )
        result = tool.invoke(_ctx(), {"task": "x", "candidate_output": "y"})
        assert result.ok
        assert result.output["passed"] is False
        assert result.output["score"] == 0.3
    finally:
        AdapterRegistry.reset()


def test_self_check_tool_handles_unparseable_judge_output() -> None:
    class _BadJudge(Adapter):
        def name(self) -> str:
            return "stub_judge_bad"

        def invoke(self, agent: dict, prompt: str, input: dict) -> AdapterResult:
            return AdapterResult(text="not json", cost=Cost())

        def invoke_stream(self, agent: dict, prompt: str, input: dict):
            yield StreamChunk(text_delta="")

    AdapterRegistry.register("stub_judge_bad", _BadJudge)
    try:
        tool = build_tool(
            {
                "name": "eval_self_check",
                "type": "self_check",
                "config": {"rubric": "x", "judge_adapter": "stub_judge_bad"},
            }
        )
        result = tool.invoke(_ctx(), {"task": "x", "candidate_output": "y"})
        assert result.ok
        # Falls back to score=0 / passed=False; rationale includes raw output.
        assert result.output["score"] == 0.0
        assert "unparseable" in result.output["details"]["rationale"]
    finally:
        AdapterRegistry.reset()


def test_self_check_tool_validates_input() -> None:
    class _EmptyJudge(Adapter):
        def name(self) -> str:
            return "stub_judge_v"

        def invoke(self, agent: dict, prompt: str, input: dict) -> AdapterResult:
            return AdapterResult(text="{}", cost=Cost())

        def invoke_stream(self, agent: dict, prompt: str, input: dict):
            yield StreamChunk(text_delta="")

    AdapterRegistry.register("stub_judge_v", _EmptyJudge)
    try:
        tool = build_tool(
            {
                "name": "eval_self_check",
                "type": "self_check",
                "config": {"rubric": "x", "judge_adapter": "stub_judge_v"},
            }
        )
        result = tool.invoke(_ctx(), {"candidate_output": "y"})
        assert result.error is not None
        assert "task" in result.error
    finally:
        AdapterRegistry.reset()