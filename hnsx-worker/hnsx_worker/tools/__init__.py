"""Tool abstraction for API Agents (M3 / W3.x).

Public API:

  - :class:`Tool`       — base class for tools agents can call.
  - :class:`ToolContext` — per-invocation context (session / turn / secrets / emit).
  - :class:`ToolResult` — return type with ``ok`` / ``error`` / ``output``.
  - :class:`ToolDecision` — policy hook return value (W6 wires a real engine here).
  - :class:`ToolRegistry` — name → Tool map with optional policy gating.

W3.1: this module + ``registry.py`` provide the foundation.
W3.2: ``http.py`` adds the first real tool (HTTP with secret injection).
W3.3: ``session_executor._run_multi_turn`` calls through ``ToolRegistry``.
"""

from .base import (
    EmitFn,
    PolicyHook,
    Tool,
    ToolContext,
    ToolDecision,
    ToolResult,
)
from .registry import ToolRegistry

__all__ = [
    "EmitFn",
    "PolicyHook",
    "Tool",
    "ToolContext",
    "ToolDecision",
    "ToolRegistry",
    "ToolResult",
]