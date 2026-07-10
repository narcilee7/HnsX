"""Tool abstraction for API Agents (M3 / W3.x).

Public API:

  - :class:`Tool`       — base class for tools agents can call.
  - :class:`ToolContext` — per-invocation context (session / turn / secrets / emit).
  - :class:`ToolResult` — return type with ``ok`` / ``error`` / ``output``.
  - :class:`ToolDecision` — policy hook return value (W6 wires a real engine here).
  - :class:`ToolRegistry` — name → Tool map with optional policy gating.
  - :func:`build_delegate_tool` — W12 helper that wires ``delegate_to``.

W3.1: this module + ``registry.py`` provide the foundation.
W3.2: ``http.py`` adds the first real tool (HTTP with secret injection).
W3.3: ``session_executor._run_multi_turn`` calls through ``ToolRegistry``.
W12: ``delegate`` tool type + ``build_delegate_tool`` for multi-agent strategies.
"""

from .base import (
    EmitFn,
    PolicyHook,
    Tool,
    ToolContext,
    ToolDecision,
    ToolResult,
)
from .delegate import DelegateTool, DelegateToolConfig
from .factory import build_delegate_tool, build_tool, tool_schemas_for_adapter
from .registry import ToolRegistry
from .self_check import SelfCheckTool, SelfCheckToolConfig, build_self_check_tool
from .human_approval import HumanApprovalTool, HumanApprovalToolConfig, build_human_approval_tool

__all__ = [
    "DelegateTool",
    "DelegateToolConfig",
    "EmitFn",
    "HumanApprovalTool",
    "HumanApprovalToolConfig",
    "PolicyHook",
    "SelfCheckTool",
    "SelfCheckToolConfig",
    "Tool",
    "ToolContext",
    "ToolDecision",
    "ToolRegistry",
    "ToolResult",
    "build_delegate_tool",
    "build_human_approval_tool",
    "build_self_check_tool",
    "build_tool",
    "tool_schemas_for_adapter",
]
