"""``ToolRegistry`` — name → Tool map with optional policy gating.

The registry is the single entry point used by ``session_executor``. It:

  1. Resolves the tool by name. Unknown tools return a ``ToolResult(error=...)``
     — never raise, so the agent can react.
  2. Calls the optional ``policy_decision`` hook. If the hook denies, the
     registry emits a ``policy_violation`` observation and returns an error
     result without invoking the tool.
  3. Dispatches to ``Tool.invoke(ctx, input)`` and returns its result.

The W6 ``PolicyEngine`` plugs in as a ``policy_decision`` callback here, so
the registry has no compile-time dependency on the policy package.
"""

from __future__ import annotations

import logging
from typing import Any

from .base import EmitFn, PolicyHook, Tool, ToolContext, ToolDecision, ToolResult

log = logging.getLogger("hnsx_worker.tools.registry")


class ToolRegistry:
    """In-memory name → Tool registry with optional policy gating.

    Usage::

        registry = ToolRegistry(policy_decision=my_decision_fn)
        registry.register(MyTool())
        result = registry.call("my_tool", ctx, {"foo": "bar"})

    The ``policy_decision`` argument is a ``Callable[[str, dict, ToolContext],
    ToolDecision]``. If it returns ``allow=False`` the registry emits a
    ``policy_violation`` observation (when ``ctx.emit`` is set) and returns
    ``ToolResult(error="denied by policy: ...")`` without calling the tool.
    """

    def __init__(
        self,
        *,
        policy_decision: PolicyHook | None = None,
    ) -> None:
        self._tools: dict[str, Tool] = {}
        self._policy_decision = policy_decision

    # ------------------------------------------------------------------ register

    def register(self, tool: Tool) -> None:
        """Register a tool instance under its ``name``."""
        if not isinstance(tool, Tool):
            raise TypeError(f"registry.register expects a Tool subclass, got {type(tool).__name__}")
        self._tools[tool.name] = tool
        log.debug("registered tool %r", tool.name)

    def unregister(self, name: str) -> None:
        self._tools.pop(name, None)

    def get(self, name: str) -> Tool | None:
        return self._tools.get(name)

    def names(self) -> list[str]:
        return sorted(self._tools.keys())

    def __contains__(self, name: str) -> bool:
        return name in self._tools

    def __len__(self) -> int:
        return len(self._tools)

    # ------------------------------------------------------------------ dispatch

    def call(self, name: str, ctx: ToolContext, input: dict[str, Any]) -> ToolResult:
        """Resolve + policy-check + invoke one tool.

        Never raises for expected failures. Programmer bugs (TypeError,
        AttributeError from a broken Tool subclass) bubble up so the
        executor can mark the session as failed.
        """
        tool = self._tools.get(name)
        if tool is None:
            return ToolResult(
                error=f"unknown tool: {name!r}",
                metadata={"available_tools": self.names()},
            )

        if self._policy_decision is not None:
            decision = self._policy_decision(name, dict(input), ctx)
            if isinstance(decision, ToolDecision):
                if not decision.allow:
                    self._emit_violation(name, input, ctx, decision)
                    return ToolResult(
                        error=f"denied by policy: {decision.reason or decision.decision}",
                        metadata={
                            "decision": decision.decision,
                            "policy_reason": decision.reason,
                        },
                    )
            else:
                # Defensive: a misbehaving hook returned something else.
                log.warning(
                    "policy_decision hook for tool %r returned %r (expected ToolDecision)",
                    name,
                    type(decision).__name__,
                )

        return tool.invoke(ctx, input)

    # ------------------------------------------------------------------ audit

    def _emit_violation(
        self,
        name: str,
        input: dict[str, Any],
        ctx: ToolContext,
        decision: ToolDecision,
    ) -> None:
        emit: EmitFn | None = ctx.emit
        if emit is None:
            return
        emit(
            {
                "kind": "policy_violation",
                "session_id": ctx.session_id,
                "domain_id": ctx.domain_id,
                "agent_id": ctx.agent_id,
                "payload": {
                    "tool": name,
                    "tool_call_id": ctx.tool_call_id,
                    "turn": ctx.turn,
                    "decision": decision.decision,
                    "reason": decision.reason,
                    # We intentionally do NOT include the raw input here if
                    # it might contain secret refs / PII. Audit logs should
                    # be safe to ship.
                    "input_keys": sorted(input.keys()) if isinstance(input, dict) else [],
                },
            }
        )


__all__ = ["ToolRegistry"]