"""Policy engine — runtime constraints for sessions and tool calls.

W6 plugs a real :class:`PolicyEngine` into ``session_executor``. The engine
is constructed from the DomainSpec's ``harness.policy`` block and enforces:

  - **budget**: cumulative cost vs ``max_cost_usd`` and turn count vs ``max_turns``.
  - **allowed/denied tools**: per-agent or per-domain tool lists.
  - **human approval**: pause the session and wait for operator approval.

Every check emits a ``policy_check`` or ``policy_violation`` observation via
the bound ``emit`` callable.

Spec shape::

    harness:
      policy:
        budget:
          max_cost_usd: 1.0
          max_turns: 10
        permissions:
          allowed_tools: [http_get, sql_read]
          denied_tools: [bash, file_delete]
          require_human_approval: [file_write, shell]
"""

from __future__ import annotations

import logging
from collections.abc import Callable
from dataclasses import dataclass, field
from typing import Any

from hnsx_worker.tools import ToolContext, ToolDecision

log = logging.getLogger("hnsx_worker.policy.engine")

EmitFn = Callable[[dict], None]


@dataclass
class Budget:
    """Running budget counters for a session."""

    max_cost_usd: float = 0.0
    max_turns: int = 0
    cumulative_cost_usd: float = 0.0
    turns_used: int = 0

    @property
    def cost_exceeded(self) -> bool:
        return self.max_cost_usd > 0 and self.cumulative_cost_usd >= self.max_cost_usd

    @property
    def turns_exceeded(self) -> bool:
        return self.max_turns > 0 and self.turns_used >= self.max_turns


@dataclass
class ToolPolicy:
    """Tool-level policy lists. Empty lists mean "not restricted by this list"."""

    allowed_tools: set[str] = field(default_factory=set)
    denied_tools: set[str] = field(default_factory=set)
    require_human_approval: set[str] = field(default_factory=set)


@dataclass
class Decision(ToolDecision):
    """Result of a policy check."""

    rule: str = ""

    def __post_init__(self) -> None:
        pass


class PolicyEngine:
    """Runtime policy evaluator.

    Usage::

        engine = PolicyEngine(spec, session_id="s", emit=emit)
        decision = engine.check_tool("bash", ctx)
        if not decision.allow:
            # session pause / fail
    """

    def __init__(
        self,
        spec: dict[str, Any],
        *,
        session_id: str,
        domain_id: str,
        agent_id: str = "",
        emit: EmitFn | None = None,
    ) -> None:
        self.spec = spec
        self.session_id = session_id
        self.domain_id = domain_id
        self.agent_id = agent_id
        self.emit = emit

        policy = (spec.get("harness", {}) or {}).get("policy", {}) or {}
        budget_cfg = policy.get("budget", {}) or {}
        self.budget = Budget(
            max_cost_usd=float(budget_cfg.get("max_cost_usd") or 0),
            max_turns=int(budget_cfg.get("max_turns") or 0),
        )

        perms = policy.get("permissions", {}) or {}
        self.tool_policy = ToolPolicy(
            allowed_tools=set(perms.get("allowed_tools") or []),
            denied_tools=set(perms.get("denied_tools") or []),
            require_human_approval=set(perms.get("require_human_approval") or []),
        )

    # ------------------------------------------------------------------ budget

    def check_budget(self, *, incremental_cost_usd: float = 0.0) -> Decision:
        """Check whether the session is still within budget.

        Call at the start of each turn. Pass the *estimated* cost of the turn
        if known (usually 0 until the adapter returns a cost observation).
        """
        projected = self.budget.cumulative_cost_usd + incremental_cost_usd
        if self.budget.cost_exceeded or (
            self.budget.max_cost_usd > 0 and projected > self.budget.max_cost_usd
        ):
            decision = Decision(
                allow=False,
                decision="deny",
                reason=(
                    f"budget exceeded: cost {projected:.6f} USD "
                    f">= max {self.budget.max_cost_usd:.6f} USD"
                ),
                rule="budget.max_cost_usd",
            )
            self._emit_violation("budget", decision)
            return decision

        if self.budget.turns_exceeded:
            decision = Decision(
                allow=False,
                decision="deny",
                reason=(
                    f"turn limit exceeded: {self.budget.turns_used} "
                    f">= {self.budget.max_turns}"
                ),
                rule="budget.max_turns",
            )
            self._emit_violation("budget", decision)
            return decision

        self._emit_check(
            "budget",
            Decision(
                allow=True,
                decision="allow",
                reason="within budget",
                rule="budget",
            ),
            payload={
                "cumulative_cost_usd": self.budget.cumulative_cost_usd,
                "max_cost_usd": self.budget.max_cost_usd,
                "turns_used": self.budget.turns_used,
                "max_turns": self.budget.max_turns,
            },
        )
        return Decision(allow=True, decision="allow", reason="within budget")

    def add_cost(self, cost_usd: float) -> None:
        """Add an adapter/tool cost to the running budget."""
        self.budget.cumulative_cost_usd += cost_usd
        self.budget.turns_used += 1

    # ------------------------------------------------------------------ tool

    def check_tool(
        self,
        tool_name: str,
        tool_input: dict[str, Any] | None = None,
        ctx: ToolContext | None = None,
    ) -> Decision:
        """Check whether a tool call is allowed by policy."""
        tp = self.tool_policy

        if tool_name in tp.denied_tools:
            decision = Decision(
                allow=False,
                decision="deny",
                reason=f"tool {tool_name!r} is in denied_tools",
                rule="permissions.denied_tools",
            )
            self._emit_violation(tool_name, decision)
            return decision

        if tp.allowed_tools and tool_name not in tp.allowed_tools:
            decision = Decision(
                allow=False,
                decision="deny",
                reason=(
                    f"tool {tool_name!r} not in allowed_tools "
                    f"{sorted(tp.allowed_tools)}"
                ),
                rule="permissions.allowed_tools",
            )
            self._emit_violation(tool_name, decision)
            return decision

        if tool_name in tp.require_human_approval:
            decision = Decision(
                allow=False,
                decision="require_approval",
                reason=f"tool {tool_name!r} requires human approval",
                rule="permissions.require_human_approval",
            )
            self._emit_check(tool_name, decision)
            return decision

        decision = Decision(
            allow=True,
            decision="allow",
            reason=f"tool {tool_name!r} allowed",
            rule="permissions",
        )
        self._emit_check(tool_name, decision)
        return decision

    # ------------------------------------------------------------------ audit

    def _emit_check(
        self,
        tool: str,
        decision: Decision,
        payload: dict[str, Any] | None = None,
    ) -> None:
        if self.emit is None:
            return
        self.emit(
            {
                "kind": "policy_check",
                "session_id": self.session_id,
                "domain_id": self.domain_id,
                "agent_id": self.agent_id,
                "payload": {
                    "tool": tool,
                    "decision": decision.decision,
                    "reason": decision.reason,
                    "rule": decision.rule,
                    **(payload or {}),
                },
            }
        )

    def _emit_violation(self, tool: str, decision: Decision) -> None:
        if self.emit is None:
            return
        self.emit(
            {
                "kind": "policy_violation",
                "session_id": self.session_id,
                "domain_id": self.domain_id,
                "agent_id": self.agent_id,
                "payload": {
                    "tool": tool,
                    "decision": decision.decision,
                    "reason": decision.reason,
                    "rule": decision.rule,
                },
            }
        )


__all__ = ["Budget", "Decision", "PolicyEngine", "ToolPolicy"]
