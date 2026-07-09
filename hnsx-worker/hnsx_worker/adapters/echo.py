"""EchoAdapter — echoes the input map back as JSON.

Useful for UI demos and for verifying that the wire-up between
session_executor → adapter → observation pipeline is correct.
"""

from __future__ import annotations

import json

from hnsx_worker.adapters.base import Adapter, AdapterResult


class EchoAdapter(Adapter):
    """Echoes the input map back as a JSON string with an agent-id header."""

    def name(self) -> str:
        return "echo"

    def invoke(self, agent: dict, prompt: str, input: dict) -> AdapterResult:
        body = json.dumps(input, sort_keys=True, default=str)
        text = f"[echo] agent={agent.get('id', '')} input={body}"
        return AdapterResult(text=text)
