"""Tests for session_executor streaming + multi-turn behavior.

Uses stub adapters that produce deterministic text / tool_call streams so we
can verify the executor wires chunks into observations correctly and the
multi-turn loop respects ``max_turns``.
"""

from __future__ import annotations

import threading
from typing import Any

import pytest

from hnsx_worker.adapters import AdapterRegistry
from hnsx_worker.adapters.base import Adapter, AdapterResult, Cost, StreamChunk, ToolCall
from hnsx_worker.session_executor import (
    _stream_turn,
    execute_session,
)


# ---------------------------------------------------------------------------
# stub adapter registry
# ---------------------------------------------------------------------------


class _StreamingScriptedAdapter(Adapter):
    """Plays back a script of (text_chunks, tool_calls) per invocation.

    Each call to ``invoke_stream`` returns the next script entry. When the
    script is exhausted, the adapter loops the last entry (so multi-turn
    loops that exceed the script still terminate naturally with a no-tool
    text reply).
    """

    def __init__(self, script: list[dict]) -> None:
        self.script = script
        self.calls = 0

    def name(self) -> str:
        return "scripted"

    def invoke(self, agent: dict, prompt: str, input: dict) -> AdapterResult:
        text = "".join(step.get("text", "") for step in self.script)
        tool_calls = [
            ToolCall(id=tc["id"], name=tc["name"], input=tc.get("input", {}))
            for tc in self.script[0].get("tool_calls", []) if self.calls == 0
        ]
        return AdapterResult(text=text, tool_calls=tool_calls)

    def invoke_stream(self, agent: dict, prompt: str, input: dict) -> Any:
        # Re-derive scripts from the *messages* history so we can vary by turn.
        messages = input.get("_messages") or []
        turn_idx = sum(1 for m in messages if m.get("role") == "assistant")
        if turn_idx < len(self.script):
            step = self.script[turn_idx]
        else:
            step = self.script[-1]
        self.calls += 1
        for chunk in step.get("text_chunks", []):
            yield StreamChunk(text_delta=chunk)
        for tc in step.get("tool_calls", []):
            yield StreamChunk(
                tool_call=ToolCall(
                    id=tc["id"], name=tc["name"], input=tc.get("input", {}),
                    raw_input=str(tc.get("input", {})),
                )
            )
        yield StreamChunk(cost=Cost(prompt_tokens=10, completion_tokens=2, latency_ms=42))


def _register_scripted(name: str, script: list[dict]) -> type:
    cls = type(
        f"_Scripted_{name}",
        (_StreamingScriptedAdapter,),
        {"__init__": lambda self: _StreamingScriptedAdapter.__init__(self, script)},
    )
    AdapterRegistry.register(name, cls)
    return cls


# ---------------------------------------------------------------------------
# streaming observation emission
# ---------------------------------------------------------------------------


def _noop_single_spec(adapter_kind: str = "scripted") -> dict:
    return {
        "id": "test-domain",
        "version": "0.1.0",
        "harness": {
            "agents": {
                "primary": {
                    "id": "primary",
                    "provider": adapter_kind,
                    "model": "test-model",
                    "adapter": {"kind": adapter_kind},
                    "system_prompt": "you are primary",
                }
            },
            "session": {"mode": "single-task", "agent": "primary"},
        },
    }


def _collect_observations(spec: dict, *, trigger: dict | None = None) -> list[dict]:
    obs: list[dict] = []
    execute_session(
        spec,
        trigger or {"q": "hi"},
        {"session_id": "s-x"},
        stop_event=threading.Event(),
        emit=lambda o: obs.append(o),
    )
    return obs


def test_single_mode_emits_text_deltas_and_final_agent_text() -> None:
    _register_scripted(
        "scripted_stream",
        [
            {
                "text_chunks": ["hello ", "world"],
                "tool_calls": [],
            }
        ],
    )
    spec = _noop_single_spec("scripted_stream")
    obs = _collect_observations(spec)

    kinds = [o["kind"] for o in obs]
    assert "turn_start" in kinds
    assert "agent_invoke" in kinds
    assert "agent_text_delta" in kinds
    assert "agent_text" in kinds
    assert "turn_end" in kinds

    deltas = [o["payload"]["content"] for o in obs if o["kind"] == "agent_text_delta"]
    assert "".join(deltas) == "hello world"

    final = [o for o in obs if o["kind"] == "agent_text"][-1]["payload"]
    assert final["content"] == "hello world"
    assert final["final"] is True


def test_single_mode_emits_cost_observation() -> None:
    _register_scripted(
        "scripted_cost",
        [{"text_chunks": ["hi"], "tool_calls": []}],
    )
    spec = _noop_single_spec("scripted_cost")
    obs = _collect_observations(spec)
    costs = [o for o in obs if o["kind"] == "agent_cost"]
    assert costs
    assert costs[-1]["payload"]["prompt_tokens"] == 10
    assert costs[-1]["payload"]["completion_tokens"] == 2
    assert costs[-1]["payload"]["latency_ms"] == 42


# ---------------------------------------------------------------------------
# multi-turn loop
# ---------------------------------------------------------------------------


def test_multi_turn_handles_tool_call_loop() -> None:
    # Turn 1: tool call. Turn 2: final text.
    _register_scripted(
        "scripted_multi",
        [
            {
                "text_chunks": ["Let me look that up. "],
                "tool_calls": [
                    {"id": "tc-1", "name": "search", "input": {"q": "orders"}},
                ],
            },
            {
                "text_chunks": ["Found 3 orders."],
                "tool_calls": [],
            },
        ],
    )
    spec = {
        "id": "multi-domain",
        "version": "0.1.0",
        "harness": {
            "agents": {
                "primary": {
                    "id": "primary",
                    "provider": "scripted_multi",
                    "model": "test",
                    "adapter": {"kind": "scripted_multi"},
                    "system_prompt": "be helpful",
                }
            },
            "session": {"mode": "multi-turn", "agent": "primary"},
            "policy": {"budget": {"max_turns": 5}},
        },
    }
    obs = _collect_observations(spec, trigger={"q": "list my orders"})

    turns_started = [o for o in obs if o["kind"] == "turn_start"]
    turns_ended = [o for o in obs if o["kind"] == "turn_end"]
    tool_calls = [o for o in obs if o["kind"] == "tool_call"]
    tool_results = [o for o in obs if o["kind"] == "tool_result"]
    final_texts = [o for o in obs if o["kind"] == "agent_text" and o["payload"].get("final")]

    assert len(turns_started) == 2, f"expected 2 turns, got {len(turns_started)}"
    assert len(turns_ended) == 2
    assert len(tool_calls) == 1
    assert tool_calls[0]["payload"]["tool_call_id"] == "tc-1"
    assert tool_calls[0]["payload"]["name"] == "search"
    assert tool_calls[0]["payload"]["input"] == {"q": "orders"}
    assert len(tool_results) == 1
    assert tool_results[0]["payload"]["tool_call_id"] == "tc-1"
    assert tool_results[0]["payload"]["output"]["stub"] is True
    assert len(final_texts) == 1
    assert final_texts[0]["payload"]["content"] == "Found 3 orders."


def test_multi_turn_respects_max_turns() -> None:
    # Infinite tool-call script so we only stop because of max_turns.
    infinite_tool_call = {"id": "tc-loop", "name": "loop", "input": {}}
    _register_scripted(
        "scripted_loop",
        [{"text_chunks": ["working..."], "tool_calls": [infinite_tool_call]}] * 10,
    )
    spec = {
        "id": "loop-domain",
        "version": "0.1.0",
        "harness": {
            "agents": {
                "primary": {
                    "id": "primary",
                    "provider": "scripted_loop",
                    "model": "test",
                    "adapter": {"kind": "scripted_loop"},
                    "system_prompt": "p",
                }
            },
            "session": {"mode": "multi-turn", "agent": "primary"},
            "policy": {"budget": {"max_turns": 3}},
        },
    }
    obs = _collect_observations(spec)

    turns_started = [o for o in obs if o["kind"] == "turn_start"]
    assert len(turns_started) == 3, f"expected 3 turns, got {len(turns_started)}"
    final_texts = [o for o in obs if o["kind"] == "agent_text"]
    last_final = [o for o in obs if o["kind"] == "agent_text" and o["payload"].get("truncated")]
    assert last_final, "expected a truncated final agent_text observation"


# ---------------------------------------------------------------------------
# _stream_turn fallback
# ---------------------------------------------------------------------------


def test_stream_turn_falls_back_when_adapter_raises() -> None:
    """If invoke_stream raises mid-flight, executor retries via invoke()."""

    class _BrokenStream(Adapter):
        def name(self) -> str:
            return "broken_stream"

        def invoke_stream(self, agent, prompt, input):
            raise RuntimeError("stream blew up")
            yield  # pragma: no cover — generator never reached

        def invoke(self, agent, prompt, input):
            return AdapterResult(text="fell back", cost=Cost(prompt_tokens=1, completion_tokens=1))

    AdapterRegistry.register("broken_stream", _BrokenStream)

    text, tool_calls, cost = _stream_turn(
        AdapterRegistry.get("broken_stream"),
        {"id": "a"},
        "sys",
        {"q": "hi"},
        messages=[{"role": "user", "content": "hi"}],
        session_id="s",
        domain_id="d",
        agent_id="a",
        stop_event=threading.Event(),
        emit=lambda o: None,
    )
    assert text == "fell back"
    assert tool_calls == []
    assert cost is not None
    assert cost.prompt_tokens == 1


# ---------------------------------------------------------------------------
# cleanup between tests
# ---------------------------------------------------------------------------


@pytest.fixture(autouse=True)
def _reset_registry():
    yield
    # Drop our scripted adapters so other tests start clean.
    for kind in ("scripted_stream", "scripted_cost", "scripted_multi", "scripted_loop"):
        AdapterRegistry._registry.pop(kind, None)  # type: ignore[attr-defined]
        AdapterRegistry._singletons.pop(kind, None)  # type: ignore[attr-defined]