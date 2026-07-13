"""Tests for W5 — multi-agent orchestration (supervisor / hierarchical / autonomous).

We use scripted adapters so no real LLM credentials are required.
"""

from __future__ import annotations

import threading
from typing import Any

import pytest

from hnsx_worker.adapters import AdapterRegistry
from hnsx_worker.adapters.base import Adapter, AdapterResult, Cost, StreamChunk
from hnsx_worker.harness import (
    HarnessValidationError,
    build_context,
    evaluate_condition,
    load,
    run,
)
from hnsx_worker.session_executor import execute_session

# ---------------------------------------------------------------------------
# Scripted adapter fixture
# ---------------------------------------------------------------------------


@pytest.fixture(autouse=True)
def _register_scripted_orchestration_adapter() -> Any:
    class _ScriptedAdapter(Adapter):
        def name(self) -> str:
            return "scripted_orchestration"

        def invoke(self, agent: dict, prompt: str, input: dict) -> AdapterResult:
            script = agent.get("_scripted_response", {})
            return AdapterResult(
                text=script.get("text", ""),
                cost=Cost(),
            )

        def invoke_stream(self, agent: dict, prompt: str, input: dict):
            script = agent.get("_scripted_response", {})
            yield StreamChunk(text_delta=script.get("text", ""))
            yield StreamChunk(cost=Cost())

    AdapterRegistry.register("scripted_orchestration", _ScriptedAdapter)
    yield
    AdapterRegistry.reset()


# ---------------------------------------------------------------------------
# loader
# ---------------------------------------------------------------------------


def test_load_validates_supervisor_references() -> None:
    spec = {
        "id": "s",
        "harness": {
            "agents": {
                "supervisor": {"id": "supervisor", "adapter": {"kind": "noop"}},
                "billing": {"id": "billing", "adapter": {"kind": "noop"}},
            },
            "session": {
                "mode": "supervisor",
                "supervisor": {
                    "agent": "supervisor",
                    "transitions": [
                        {"condition": "", "to": "billing"},
                    ],
                },
            },
        },
    }
    harness = load(spec)
    assert harness.mode == "supervisor"


# W16: supervisor cross-reference validation moved to the Go server's
# ValidateDomain RPC. The Python loader is now a data view; runtime
# prerequisites are enforced by harness.runner.run.


# ---------------------------------------------------------------------------
# transition
# ---------------------------------------------------------------------------


def test_evaluate_condition_matches_output_field() -> None:
    ctx = build_context(output={"intent": "billing"})
    assert evaluate_condition("output.intent == 'billing'", ctx) is True
    assert evaluate_condition("output.intent == 'technical'", ctx) is False


def test_evaluate_condition_with_observations() -> None:
    ctx = build_context(
        output={"intent": "billing"},
        observations=[
            {"kind": "agent_text", "payload": {"content": "hi"}},
            {"kind": "agent_text", "payload": {"content": "billing"}},
        ],
    )
    assert (
        evaluate_condition(
            "observations[-1].payload.content == 'billing'",
            ctx,
        )
        is True
    )


def test_evaluate_condition_empty_is_true() -> None:
    ctx = build_context()
    assert evaluate_condition("", ctx) is True


def test_evaluate_condition_syntax_error_is_false() -> None:
    ctx = build_context(output={"intent": "billing"})
    assert evaluate_condition("this is not jmespath", ctx) is False


# ---------------------------------------------------------------------------
# runner
# ---------------------------------------------------------------------------


def test_supervisor_routes_to_billing_and_exits() -> None:
    spec = {
        "id": "triage-test",
        "harness": {
            "agents": {
                "supervisor": {
                    "id": "supervisor",
                    "adapter": {"kind": "scripted_orchestration"},
                    "system_prompt": "route",
                    "_scripted_response": {
                        "text": '{"to": "billing", "reason": "billing question"}'
                    },
                },
                "billing": {
                    "id": "billing",
                    "adapter": {"kind": "scripted_orchestration"},
                    "system_prompt": "billing",
                    "_scripted_response": {"text": "billing reply"},
                },
            },
            "session": {
                "mode": "supervisor",
                "supervisor": {
                    "agent": "supervisor",
                    "transitions": [
                        {"condition": "output.to == 'billing'", "to": "billing"},
                    ],
                    "exit": [
                        {"condition": "output.text == 'billing reply'", "state": "completed"},
                    ],
                },
            },
        },
    }
    captured: list[dict] = []
    run(
        spec,
        trigger={"q": "refund"},
        config={"session_id": "s1"},
        stop_event=threading.Event(),
        emit=captured.append,
    )

    kinds = [o["kind"] for o in captured]
    assert "routing_decision" in kinds
    assert "specialist_start" in kinds
    assert "specialist_end" in kinds
    assert "session_end" in kinds

    decision = [o for o in captured if o["kind"] == "routing_decision"][0]
    assert decision["payload"]["to"] == "billing"
    assert decision["payload"]["reason"] == "billing question"

    end = [o for o in captured if o["kind"] == "session_end"][0]
    assert end["state"] == "completed"


def test_supervisor_no_match_falls_back_to_supervisor() -> None:
    spec = {
        "id": "triage-test",
        "harness": {
            "agents": {
                "supervisor": {
                    "id": "supervisor",
                    "adapter": {"kind": "scripted_orchestration"},
                    "_scripted_response": {"text": '{"to": "unknown"}'},
                },
                "billing": {
                    "id": "billing",
                    "adapter": {"kind": "scripted_orchestration"},
                    "_scripted_response": {"text": "unused"},
                },
            },
            "session": {
                "mode": "supervisor",
                "max_turns": 2,
                "supervisor": {
                    "agent": "supervisor",
                    "transitions": [
                        {"condition": "output.to == 'billing'", "to": "billing"},
                    ],
                },
            },
        },
    }
    captured: list[dict] = []
    run(
        spec,
        trigger={"q": "x"},
        config={"session_id": "s2"},
        stop_event=threading.Event(),
        emit=captured.append,
    )
    decisions = [o for o in captured if o["kind"] == "routing_decision"]
    # First turn no match, second turn max_turns ends the loop.
    assert len(decisions) == 2
    assert captured[-1]["kind"] == "session_end"


def test_executor_dispatches_supervisor_mode_to_runner() -> None:
    """execute_session delegates supervisor mode to harness.runner.run."""
    spec = {
        "id": "executor-supervisor",
        "harness": {
            "agents": {
                "supervisor": {
                    "id": "supervisor",
                    "adapter": {"kind": "scripted_orchestration"},
                    "_scripted_response": {"text": '{"to": "billing"}'},
                },
                "billing": {
                    "id": "billing",
                    "adapter": {"kind": "scripted_orchestration"},
                    "_scripted_response": {"text": "ok"},
                },
            },
            "session": {
                "mode": "supervisor",
                "supervisor": {
                    "agent": "supervisor",
                    "transitions": [
                        {"condition": "output.to == 'billing'", "to": "billing"},
                    ],
                    "exit": [
                        {"condition": "output.text == 'ok'", "state": "completed"},
                    ],
                },
            },
        },
    }
    captured: list[dict] = []
    execute_session(
        spec,
        trigger={"q": "x"},
        config={"session_id": "s-exec"},
        stop_event=threading.Event(),
        emit=captured.append,
    )
    assert any(o["kind"] == "routing_decision" for o in captured)
    assert captured[-1]["kind"] == "session_end"


def test_parse_routing_decision_extracts_json() -> None:
    from hnsx_worker.harness.runner import _parse_routing_decision

    text = (
        'Some words {\"to\": \"billing\", \"reason\": \"refund\", '
        '\"confidence\": 0.9} more words'
    )
    result = _parse_routing_decision(text)
    assert result == {"to": "billing", "reason": "refund", "confidence": 0.9}


def test_parse_routing_decision_falls_back_to_text() -> None:
    from hnsx_worker.harness.runner import _parse_routing_decision

    result = _parse_routing_decision("just a reason")
    assert result == {"to": "", "reason": "just a reason", "confidence": None}



# ---------------------------------------------------------------------------
# W15: harness loader — skill validation
# ---------------------------------------------------------------------------


def _loader_spec_with_skills() -> dict:
    return {
        "id": "skill-loader-demo",
        "version": "0.1.0",
        "harness": {
            "agents": {
                "triage": {
                    "id": "triage",
                    "provider": "noop",
                    "model": "noop-1",
                    "adapter": {"kind": "noop"},
                    "system_prompt": "triage-prompt",
                    "skill_refs": ["web-search"],
                }
            },
            "prompts": {"triage-prompt": {"type": "system", "template": "x"}},
            "skills": {
                "web-search": {
                    "id": "web-search",
                    "description": "fetch URLs",
                    "tools": [{"name": "fetch", "type": "http"}],
                }
            },
            "session": {"mode": "single"},
        },
    }


def test_load_populates_skill_registry() -> None:
    spec = _loader_spec_with_skills()
    harness = load(spec)
    assert "web-search" in harness.skills
    assert harness.skills["web-search"].id == "web-search"


def test_load_passes_when_all_skill_refs_resolve() -> None:
    # No exception means the skill registry was built successfully.
    load(_loader_spec_with_skills())


# W16: skill_ref cross-reference validation moved to the Go server's
# ValidateDomain RPC. Unknown or malformed skill_refs are silently ignored
# at load time; runtime resolution handles them gracefully.


def test_load_rejects_duplicate_skill_ids() -> None:
    spec = _loader_spec_with_skills()
    spec["harness"]["skills"] = [
        {"id": "web-search", "tools": [{"name": "fetch"}]},
        {"id": "web-search", "tools": [{"name": "fetch2"}]},
    ]
    with pytest.raises(HarnessValidationError):
        load(spec)


def test_load_rejects_skill_with_no_content() -> None:
    spec = _loader_spec_with_skills()
    spec["harness"]["skills"] = {
        "empty": {"id": "empty", "description": "nothing useful"}
    }
    spec["harness"]["agents"]["triage"]["skill_refs"] = ["empty"]
    with pytest.raises(HarnessValidationError) as exc:
        load(spec)
    assert "no prompts/tools/mcp_refs/examples" in str(exc.value)


def test_load_succeeds_when_agent_has_no_skill_refs() -> None:
    spec = _loader_spec_with_skills()
    spec["harness"]["agents"]["triage"].pop("skill_refs", None)
    load(spec)  # no exception
