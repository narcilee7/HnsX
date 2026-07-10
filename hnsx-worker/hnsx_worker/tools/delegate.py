"""Delegation tool — let an agent hand work to another agent in the same Domain.

W12 takes a thin approach to advanced orchestration: the parent agent decides
*when* and *to whom* to delegate; the Worker only provides the runtime to
spawn the sub-agent, run it, and route the result back.

The sub-agent is executed as a single adapter invocation (no nested multi-turn
loop). It gets its own system prompt, adapter, and tool schema from the
DomainSpec. The parent agent receives the sub-agent's final text as the tool
result.

Observations emitted:

  - ``sub_agent_start`` / ``sub_agent_end`` — lifecycle of the delegated run.
  - ``delegate`` — compact record of the hand-off.
"""

from __future__ import annotations

import threading
from collections.abc import Mapping
from dataclasses import dataclass, field
from typing import Any

from hnsx_worker.adapters import AdapterRegistry
from hnsx_worker.harness.runner import _run_agent as _run_sub_agent
from hnsx_worker.harness.loader import load as _load_harness

from .base import Tool, ToolContext, ToolResult


@dataclass
class DelegateToolConfig:
    """Configuration for the ``delegate_to`` tool.

    ``allowed_agents`` restricts which agents the parent may target. An empty
    list means "any agent declared in the DomainSpec".
    """

    spec: dict[str, Any] = field(default_factory=dict)
    session_config: dict[str, Any] = field(default_factory=dict)
    stop_event: threading.Event = field(default_factory=threading.Event)
    allowed_agents: list[str] = field(default_factory=list)

    @classmethod
    def from_spec(
        cls,
        raw: Mapping[str, Any],
        *,
        spec: dict[str, Any],
        session_config: dict[str, Any],
        stop_event: threading.Event,
    ) -> "DelegateToolConfig":
        allowed = raw.get("allowed_agents") or []
        if not isinstance(allowed, list):
            raise ValueError("delegate_to.allowed_agents must be a list")
        return cls(
            spec=spec,
            session_config=session_config,
            stop_event=stop_event,
            allowed_agents=[str(a) for a in allowed],
        )


class DelegateTool(Tool):
    """Tool: ``delegate_to(agent, task)`` — run another Domain agent."""

    def __init__(self, name: str, config: DelegateToolConfig) -> None:
        self._name = name
        self._config = config
        self._harness = _load_harness(config.spec)

    @property
    def name(self) -> str:
        return self._name

    @property
    def schema(self) -> dict[str, Any]:
        agents = self._allowed_agent_names()
        return {
            "type": "object",
            "properties": {
                "agent": {
                    "type": "string",
                    "enum": agents,
                    "description": "Name of the agent to delegate the task to.",
                },
                "task": {
                    "type": "string",
                    "description": "The task or question to hand off.",
                },
                "context": {
                    "type": "object",
                    "description": "Optional extra context for the sub-agent.",
                },
            },
            "required": ["agent", "task"],
            "additionalProperties": False,
        }

    def invoke(self, ctx: ToolContext, input: dict[str, Any]) -> ToolResult:
        if not isinstance(input, dict):
            return ToolResult(error="delegate_to input must be a JSON object")

        target = str(input.get("agent", "")).strip()
        task = input.get("task")
        if not target:
            return ToolResult(error="delegate_to requires 'agent'")
        if task is None or task == "":
            return ToolResult(error="delegate_to requires a non-empty 'task'")

        allowed = self._allowed_agent_names()
        if allowed and target not in allowed:
            return ToolResult(
                error=f"delegation to {target!r} not allowed; allowed={sorted(allowed)}"
            )

        try:
            target_agent = self._harness.get_agent(target)
        except KeyError as e:
            return ToolResult(error=f"unknown agent: {target!r}")

        adapter = AdapterRegistry.get(
            target_agent.get("adapter", {}).get("kind", "noop")
        )

        sub_input = {
            "task": task,
            "context": input.get("context") or {},
            "from_agent": ctx.agent_id,
        }

        self._emit_lifecycle(ctx, target, "sub_agent_start")
        try:
            output = _run_sub_agent(
                adapter,
                target_agent,
                self._harness,
                sub_input,
                session_id=ctx.session_id,
                domain_id=ctx.domain_id,
                agent_id=target,
                stop_event=self._config.stop_event,
                emit=ctx.emit or (lambda _obs: None),
                turn=ctx.turn,
            )
        except Exception as e:  # noqa: BLE001
            self._emit_lifecycle(ctx, target, "sub_agent_end", error=str(e))
            return ToolResult(error=f"sub-agent {target!r} failed: {e!s}")

        self._emit_lifecycle(ctx, target, "sub_agent_end", output=output)
        return ToolResult(
            output={"agent": target, "output": output},
            metadata={"delegated_to": target, "task_preview": str(task)[:200]},
        )

    def _allowed_agent_names(self) -> list[str]:
        agents = list(self._harness.agents.keys())
        if self._config.allowed_agents:
            return [a for a in agents if a in self._config.allowed_agents]
        return agents

    def _emit_lifecycle(
        self,
        ctx: ToolContext,
        target: str,
        kind: str,
        *,
        output: str = "",
        error: str = "",
    ) -> None:
        if ctx.emit is None:
            return
        payload: dict[str, Any] = {
            "tool_call_id": ctx.tool_call_id,
            "from_agent": ctx.agent_id,
            "to_agent": target,
            "turn": ctx.turn,
        }
        if output:
            payload["output_preview"] = output[:500]
        if error:
            payload["error"] = error
        ctx.emit(
            {
                "kind": kind,
                "session_id": ctx.session_id,
                "domain_id": ctx.domain_id,
                "agent_id": target,
                "payload": payload,
            }
        )


__all__ = ["DelegateTool", "DelegateToolConfig"]
