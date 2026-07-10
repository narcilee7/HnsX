"""Tests for workflow session mode.

Covers:

  - ``execute_session`` with ``mode: workflow`` walks the configured DAG.
  - Step output variables are interpolated into the next step's input.
  - Step-level ``prompt`` / ``prompt_ref`` override the agent's default prompt.
"""

from __future__ import annotations

import threading
from typing import Any

from hnsx_worker.adapters import AdapterRegistry
from hnsx_worker.adapters.base import Adapter, AdapterResult
from hnsx_worker.session_executor import execute_session


def test_workflow_runs_steps_and_interpolates_variables() -> None:
    """Two-step workflow: first step output feeds second step input."""
    captured: list[dict] = []
    spec = {
        "id": "wf",
        "version": "0.1.0",
        "harness": {
            "agents": {
                "classifier": {
                    "id": "classifier",
                    "provider": "echo",
                    "adapter": {"kind": "echo"},
                    "system_prompt": "default",
                },
                "responder": {
                    "id": "responder",
                    "provider": "echo",
                    "adapter": {"kind": "echo"},
                    "system_prompt": "default",
                },
            },
            "prompts": {
                "default": {
                    "type": "system",
                    "template": "You are a workflow assistant.",
                }
            },
            "session": {
                "mode": "workflow",
                "workflow": {
                    "entry": "classify",
                    "steps": [
                        {
                            "id": "classify",
                            "agent": "classifier",
                            "output": "classification",
                            "next": "respond",
                        },
                        {
                            "id": "respond",
                            "agent": "responder",
                            "input": {"classification": "${classification}"},
                            "output": "response",
                        },
                    ],
                },
            },
            "policy": {"budget": {"max_turns": 5}},
        },
    }

    execute_session(
        spec,
        trigger={"message": "I need a refund"},
        config={"session_id": "s-wf"},
        stop_event=threading.Event(),
        emit=captured.append,
    )

    step_starts = [o for o in captured if o["kind"] == "step_start"]
    step_ends = [o for o in captured if o["kind"] == "step_end"]
    assert len(step_starts) == 2
    assert len(step_ends) == 2
    assert step_starts[0]["step_id"] == "classify"
    assert step_starts[1]["step_id"] == "respond"
    # The second step's input contains the interpolated classification variable.
    assert "refund" in step_ends[1]["payload"]["output"].lower()


def test_workflow_step_prompt_overrides_agent_prompt() -> None:
    """A step with ``prompt`` / ``prompt_ref`` uses that prompt instead of the
    agent's ``system_prompt``.
    """
    seen_prompts: list[str] = []

    class _CapturePromptAdapter(Adapter):
        def name(self) -> str:
            return "capture_prompt"

        def invoke(self, agent: dict, prompt: str, input: dict) -> AdapterResult:
            seen_prompts.append(prompt)
            return AdapterResult(text="ok")

    AdapterRegistry.register("capture_prompt", _CapturePromptAdapter)
    try:
        spec = {
            "id": "wf-prompt",
            "version": "0.1.0",
            "harness": {
                "agents": {
                    "a": {
                        "id": "a",
                        "provider": "capture_prompt",
                        "adapter": {"kind": "capture_prompt"},
                        "system_prompt": "agent-prompt",
                    },
                },
                "prompts": {
                    "agent-prompt": {"type": "system", "template": "agent"},
                    "step-prompt": {"type": "system", "template": "step-by-ref"},
                },
                "session": {
                    "mode": "workflow",
                    "workflow": {
                        "entry": "s1",
                        "steps": [
                            {
                                "id": "s1",
                                "agent": "a",
                                "prompt": "literal-step-prompt",
                                "next": "s2",
                            },
                            {
                                "id": "s2",
                                "agent": "a",
                                "prompt_ref": "step-prompt",
                            },
                        ],
                    },
                },
                "policy": {"budget": {"max_turns": 5}},
            },
        }
        execute_session(
            spec,
            trigger={"message": "hi"},
            config={"session_id": "s-wf-prompt"},
            stop_event=threading.Event(),
            emit=lambda o: None,
        )
    finally:
        AdapterRegistry._registry.pop("capture_prompt", None)
        AdapterRegistry._singletons.pop("capture_prompt", None)

    assert seen_prompts == ["literal-step-prompt", "step-by-ref"]


def test_workflow_falls_back_to_agent_prompt() -> None:
    """A step without prompt config uses the agent's system prompt."""
    seen_prompts: list[str] = []

    class _CapturePromptAdapter(Adapter):
        def name(self) -> str:
            return "capture_prompt_fallback"

        def invoke(self, agent: dict, prompt: str, input: dict) -> AdapterResult:
            seen_prompts.append(prompt)
            return AdapterResult(text="ok")

    AdapterRegistry.register("capture_prompt_fallback", _CapturePromptAdapter)
    try:
        spec = {
            "id": "wf-fallback",
            "version": "0.1.0",
            "harness": {
                "agents": {
                    "a": {
                        "id": "a",
                        "provider": "capture_prompt_fallback",
                        "adapter": {"kind": "capture_prompt_fallback"},
                        "system_prompt": "fallback-prompt",
                    },
                },
                "prompts": {
                    "fallback-prompt": {
                        "type": "system",
                        "template": "fallback",
                    }
                },
                "session": {
                    "mode": "workflow",
                    "workflow": {
                        "entry": "s1",
                        "steps": [{"id": "s1", "agent": "a"}],
                    },
                },
                "policy": {"budget": {"max_turns": 5}},
            },
        }
        execute_session(
            spec,
            trigger={"message": "hi"},
            config={"session_id": "s-wf-fallback"},
            stop_event=threading.Event(),
            emit=lambda o: None,
        )
    finally:
        AdapterRegistry._registry.pop("capture_prompt_fallback", None)
        AdapterRegistry._singletons.pop("capture_prompt_fallback", None)

    assert seen_prompts == ["fallback"]
