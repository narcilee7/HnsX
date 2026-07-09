"""NoopAdapter — deterministic, offline-friendly response.

Used as the smoke-test adapter ("verify the harness pipeline without any LLM")
and as the default during Step 2/3 development.
"""

from __future__ import annotations

from hnsx_worker.adapters.base import Adapter, AdapterResult


class NoopAdapter(Adapter):
    """Returns a deterministic string that identifies the agent + prompt + input keys."""

    def name(self) -> str:
        return "noop"

    def invoke(self, agent: dict, prompt: str, input: dict) -> AdapterResult:
        keys = sorted(input.keys())
        text = (
            f"[noop] agent={agent.get('id', '')} "
            f"provider={agent.get('provider', '')} "
            f"model={agent.get('model', '')} "
            f"prompt_len={len(prompt)} "
            f"input_keys={keys}"
        )
        return AdapterResult(text=text)
