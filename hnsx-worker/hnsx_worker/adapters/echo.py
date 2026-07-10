"""EchoAdapter — echoes the input map back as JSON.

Useful for UI demos and for verifying that the wire-up between
session_executor → adapter → observation pipeline is correct.

Environment-driven fixture:
  Set ``HNSX_ECHO_TOOL_CALL`` to a JSON object to make every echo invocation
  return a deterministic tool call instead of text. Used by the MCP e2e test
  so it can exercise the tool registry without an external LLM.
"""

from __future__ import annotations

import json
import os

from hnsx_worker.adapters.base import Adapter, AdapterResult, ToolCall


class EchoAdapter(Adapter):
    """Echoes the input map back as a JSON string with an agent-id header."""

    def name(self) -> str:
        return "echo"

    def invoke(self, agent: dict, prompt: str, input: dict) -> AdapterResult:
        body = json.dumps(input, sort_keys=True, default=str)
        text = f"[echo] agent={agent.get('id', '')} input={body}"

        # Optional deterministic tool-call fixture for offline e2e tests.
        fixture = self._fixture(agent)
        if fixture:
            tool_call = ToolCall(
                id=str(fixture.get("tool_call_id") or "echo-call-1"),
                name=str(fixture.get("name") or ""),
                input=dict(fixture.get("input") or {}),
                raw_input=str(fixture.get("raw_input") or ""),
            )
            return AdapterResult(text=str(fixture.get("text") or ""), tool_calls=[tool_call])

        return AdapterResult(text=text)

    def _fixture(self, agent: dict) -> dict | None:
        # Prefer agent-level config (ignored by the Go control plane today,
        # but kept for local worker tests that pass a full dict).
        cfg = agent.get("adapter") or {}
        if "echo_tool_call" in cfg:
            return cfg["echo_tool_call"]  # type: ignore[return-value]
        # Fall back to the environment fixture used by docker-compose e2e.
        raw = os.environ.get("HNSX_ECHO_TOOL_CALL", "")
        if raw:
            try:
                return json.loads(raw)  # type: ignore[return-value]
            except json.JSONDecodeError:
                return None
        return None
