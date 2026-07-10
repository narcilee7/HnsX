"""W12 Multi-Agent runner — the agent can hand work to peer agents in the Domain.

Strategy: same as ``direct`` (a multi-turn loop), but every peer agent
declared in the Domain is exposed via the ``delegate_to`` tool. The
primary agent decides when to delegate; the runner executes the
sub-agent synchronously and returns its text as the tool result.

Implementation:

  - Calls :func:`run_multi_turn_loop` from :mod:`session_executor` exactly
    like ``direct`` mode.
  - Builds a single ``DelegateTool`` whose ``allowed_agents`` defaults to
    every agent in the Domain except the primary one.
  - Injects the tool into the loop's :class:`AgentLoopContext` via
    ``build_agent_loop_context(extra_tools=...)``.

The ``multi_agent`` strategy complements W5's ``supervisor`` mode:

  - ``supervisor`` mode — the **Harness** drives routing (supervisor →
    specialist via JMESPath transitions).
  - ``multi_agent`` strategy — the **Agent** drives routing (the LLM
    decides which peer to call).
"""

from __future__ import annotations

import logging
import threading
from typing import Any

from hnsx_worker.adapters import AdapterRegistry
from hnsx_worker.tools import build_delegate_tool

from .base import Agent, AgentError, EmitFn

log = logging.getLogger("hnsx_worker.agents.multi_agent")

EmitFn = Any


class MultiAgentRunner(Agent):
    """Strategy: multi_agent."""

    name = "multi_agent"

    def __init__(
        self,
        *,
        agent_cfg: dict[str, Any],
        adapter: Any,
        session_id: str,
        domain_id: str,
        emit: EmitFn,
        config: dict[str, Any] | None = None,
        delegate_tool_name: str = "delegate_to",
        allowed_agents: list[str] | None = None,
        agent_id: str = "",
    ) -> None:
        super().__init__(
            agent_cfg=agent_cfg,
            adapter=adapter,
            session_id=session_id,
            domain_id=domain_id,
            emit=emit,
            config=config,
            agent_id=agent_id,
        )
        self.delegate_tool_name = delegate_tool_name
        self.allowed_agents = list(allowed_agents or [])

    def run(self, trigger: dict[str, Any], *, stop_event: threading.Event) -> str:
        from hnsx_worker.session_executor import (
            AgentLoopContext,
            build_agent_loop_context,
            run_multi_turn_loop,
        )

        config = self.config or {}
        spec = config.get("spec") or {}
        session_cfg = config.get("session_config") or {}
        memory = config.get("memory")

        # Build a DelegateTool wired to the same DomainSpec.
        delegate_tool = build_delegate_tool(
            {"name": self.delegate_tool_name, "config": {}},
            harness_spec=spec,
            session_config=session_cfg,
            stop_event=stop_event,
        )

        ctx: AgentLoopContext = build_agent_loop_context(
            spec=spec,
            agent_cfg=self.agent_cfg,
            config=session_cfg,
            memory=memory,
            extra_tools=[delegate_tool],
        )

        cost_totals: dict[str, Any] = {
            "prompt_tokens": 0,
            "completion_tokens": 0,
            "cost_usd": 0.0,
            "latency_ms": 0,
        }

        self.emit(
            {
                "kind": "multi_agent_start",
                "session_id": self.session_id,
                "domain_id": self.domain_id,
                "agent_id": self.agent_id,
                "payload": {
                    "strategy": "multi_agent",
                    "delegate_tool": self.delegate_tool_name,
                    "allowed_agents": self.allowed_agents,
                },
            }
        )

        result = run_multi_turn_loop(
            ctx,
            user_input=trigger,
            stop_event=stop_event,
            emit=self.emit,
            cost_totals=cost_totals,
        )

        self.emit(
            {
                "kind": "multi_agent_end",
                "session_id": self.session_id,
                "domain_id": self.domain_id,
                "agent_id": self.agent_id,
                "payload": {
                    "tool_call_count": result["tool_call_count"],
                    "turn_count": result["turn_count"],
                    "stop_reason": result["stop_reason"],
                },
            }
        )
        return result["final_text"]


def build_multi_agent_runner(
    *,
    spec: dict[str, Any],
    config: dict[str, Any],
    agent_cfg: dict[str, Any] | None = None,
    emit: EmitFn,
    memory: Any = None,
) -> MultiAgentRunner:
    """Construct a :class:`MultiAgentRunner` from a DomainSpec."""
    harness = spec.get("harness", {}) or {}
    agents = harness.get("agents", {}) or {}
    session = harness.get("session", {}) or {}
    agent_name = session.get("agent") or next(iter(agents), "")
    if agent_name not in agents:
        raise AgentError("multi_agent strategy requires session.agent or a default agent")
    cfg = agent_cfg or agents[agent_name]
    adapter_kind = cfg.get("adapter", {}).get("kind", "noop")
    adapter = AdapterRegistry.get(adapter_kind)
    orchestration = harness.get("orchestration", {}) or {}
    multi_cfg = orchestration.get("multi_agent") or {}

    allowed = list(multi_cfg.get("allowed_agents") or [])
    if not allowed:
        # Default: every other agent in the Domain is allowed.
        allowed = [name for name in agents.keys() if name != agent_name]

    return MultiAgentRunner(
        agent_cfg=cfg,
        adapter=adapter,
        session_id=config.get("session_id", ""),
        domain_id=spec.get("id", ""),
        emit=emit,
        config={
            "spec": spec,
            "session_config": config,
            "memory": memory,
        },
        delegate_tool_name=multi_cfg.get("tool_name", "delegate_to"),
        allowed_agents=allowed,
    )


__all__ = ["MultiAgentRunner", "build_multi_agent_runner"]
