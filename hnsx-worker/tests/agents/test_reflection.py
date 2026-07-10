"""Tests for the W12 reflection helper."""

from __future__ import annotations

from typing import Any

from hnsx_worker.adapters.base import AdapterResult, Cost
from hnsx_worker.agents.reflection import ReflectionResult, _parse_reflection, reflect


class _StubAdapter:
    def __init__(self, text: str = "") -> None:
        self._text = text
        self.last_prompt: str = ""

    def name(self) -> str:
        return "stub"

    def invoke(self, agent: dict, prompt: str, input: dict) -> AdapterResult:
        self.last_prompt = prompt
        return AdapterResult(text=self._text, cost=Cost())


def test_parse_reflection_accepts_well_formed_json() -> None:
    text = '```json\n{"on_track": false, "reason": "stuck", "revised_plan": ["x"]}\n```'
    r = _parse_reflection(text)
    assert isinstance(r, ReflectionResult)
    assert r.on_track is False
    assert r.revised_plan == ["x"]


def test_parse_reflection_falls_back_to_keyword() -> None:
    r = _parse_reflection("I think we are off track and need to replan")
    assert r.on_track is False


def test_parse_reflection_defaults_to_on_track() -> None:
    r = _parse_reflection("looks good, moving on")
    assert r.on_track is True


def test_reflect_returns_none_when_disabled() -> None:
    adapter = _StubAdapter()
    result = reflect(
        adapter=adapter,  # type: ignore[arg-type]
        agent_cfg={},
        goal="g",
        history=[{"kind": "agent_text", "payload": {"content": "hi"}}],
        session_id="",
        domain_id="",
        emit=lambda _o: None,
        enabled=False,
    )
    assert result is None


def test_reflect_emits_observation_when_enabled() -> None:
    adapter = _StubAdapter(
        text='{"on_track": true, "reason": "all good"}'
    )
    observations: list[dict] = []
    result = reflect(
        adapter=adapter,  # type: ignore[arg-type]
        agent_cfg={"id": "x"},
        goal="explain X",
        history=[{"kind": "agent_text", "payload": {"content": "step1"}}],
        session_id="sess",
        domain_id="dom",
        emit=observations.append,
        agent_id="a1",
        enabled=True,
    )
    assert isinstance(result, ReflectionResult)
    assert result.on_track is True
    assert any(o["kind"] == "reflection" for o in observations)
    assert "Goal:" in adapter.last_prompt or "Original task" in adapter.last_prompt


def test_reflect_skips_empty_history() -> None:
    adapter = _StubAdapter()
    result = reflect(
        adapter=adapter,  # type: ignore[arg-type]
        agent_cfg={},
        goal="g",
        history=[],
        session_id="",
        domain_id="",
        emit=lambda _o: None,
        enabled=True,
    )
    assert result is not None
    assert result.on_track is True