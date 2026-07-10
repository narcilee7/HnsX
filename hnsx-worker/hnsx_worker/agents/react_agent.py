"""W12 ReAct agent — Thought → Action → Observation loop.

Reuses :func:`run_multi_turn_loop` so tool dispatch, policy gating, store
persistence and message history all behave identically to plain
multi-turn. The ReAct-specific bits are:

  - A prompt suffix that nudges the LLM to reason between tool calls.
  - Loop detection: stop when the same ``(tool, input_fingerprint)`` repeats
    more than ``loop_threshold`` times in a row.
  - Optional reflection between turns via :mod:`reflection`.

Observations emitted:

  - ``plan_start`` / ``plan_end`` (always, even if the plan is empty).
  - ``react_step`` — one per turn, summarising the action + observation.
  - ``react_loop`` — emitted once when the loop detector triggers.
  - ``reflection`` — when ``orchestration.reflection.enabled`` is true.
"""

from __future__ import annotations

import hashlib
import json
import logging
import threading
from collections.abc import Callable
from typing import Any

from hnsx_worker.adapters import AdapterRegistry
from hnsx_worker.adapters.base import Adapter

from .base import Agent, AgentError, EmitFn
from .reflection import reflect

log = logging.getLogger("hnsx_worker.agents.react")

_REACT_SUFFIX = """

You are operating in ReAct mode. On every turn you must:
  1. Briefly explain the next action you will take and why.
  2. Then call at most one tool that advances the goal.
When you have enough information to answer, respond with plain text
(no tool call) and the loop will end.
"""

EmitFn = Callable[[dict], None]


def _fingerprint(tool_call: Any) -> str:
    """Stable hash for loop detection."""
    payload = {
        "name": tool_call.name,
        "input": tool_call.input,
    }
    raw = json.dumps(payload, sort_keys=True, default=str)
    return hashlib.sha256(raw.encode("utf-8")).hexdigest()


class ReActAgent(Agent):
    """Strategy: react."""

    name = "react"

    def __init__(
        self,
        *,
        agent_cfg: dict[str, Any],
        adapter: Adapter,
        session_id: str,
        domain_id: str,
        emit: EmitFn,
        config: dict[str, Any] | None = None,
        max_steps: int | None = None,
        loop_threshold: int = 3,
        reflection_enabled: bool = False,
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
        react_cfg = (config or {}).get("react") or {}
        self.max_steps = int(
            max_steps or react_cfg.get("max_steps") or self.agent_cfg.get("max_steps") or 8
        )
        self.loop_threshold = int(loop_threshold or react_cfg.get("loop_threshold") or 3)
        self.reflection_enabled = bool(reflection_enabled or react_cfg.get("reflection", False))

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

        # Build shared loop deps.
        ctx: AgentLoopContext = build_agent_loop_context(
            spec=spec,
            agent_cfg=self.agent_cfg,
            config=session_cfg,
            memory=memory,
        )

        cost_totals: dict[str, Any] = {
            "prompt_tokens": 0,
            "completion_tokens": 0,
            "cost_usd": 0.0,
            "latency_ms": 0,
        }

        recent_actions: list[str] = []
        observations_window: list[dict[str, Any]] = []
        goal_text = str(trigger.get("content") or json.dumps(trigger, ensure=str))

        self.emit(
            {
                "kind": "plan_start",
                "session_id": self.session_id,
                "domain_id": self.domain_id,
                "agent_id": self.agent_id,
                "payload": {"strategy": "react", "max_steps": self.max_steps},
            }
        )

        def _post_turn(turn: int, info: Any) -> None:
            # Detect loop: same fingerprint too many times.
            if info.tool_calls:
                fp = _fingerprint(info.tool_calls[-1])
                recent_actions.append(fp)
            else:
                recent_actions.clear()
            if (
                len(recent_actions) >= self.loop_threshold
                and len(set(recent_actions[-self.loop_threshold :])) == 1
            ):
                self.emit(
                    {
                        "kind": "react_loop",
                        "session_id": self.session_id,
                        "domain_id": self.domain_id,
                        "agent_id": self.agent_id,
                        "payload": {
                            "turn": turn,
                            "tool": info.tool_calls[-1].name if info.tool_calls else "",
                            "threshold": self.loop_threshold,
                        },
                    }
                )
                raise _ReactLoopBreak()

            # Emit a per-turn summary so Eval / Trace can see what happened.
            self.emit(
                {
                    "kind": "react_step",
                    "session_id": self.session_id,
                    "domain_id": self.domain_id,
                    "agent_id": self.agent_id,
                    "payload": {
                        "turn": turn,
                        "thought": (info.final_text or "")[:500],
                        "actions": [{"name": tc.name, "input": tc.input} for tc in info.tool_calls],
                    },
                }
            )

            # Optional reflection between turns.
            if self.reflection_enabled and not stop_event.is_set():
                result = reflect(
                    adapter=self.adapter,
                    agent_cfg=self.agent_cfg,
                    goal=goal_text,
                    history=observations_window[-8:]
                    + [
                        {"kind": "agent_text", "payload": {"content": info.final_text}},
                        *(
                            {
                                "kind": "tool_call",
                                "payload": {
                                    "name": tc.name,
                                    "input": tc.input,
                                },
                            }
                            for tc in info.tool_calls
                        ),
                    ],
                    session_id=self.session_id,
                    domain_id=self.domain_id,
                    emit=self.emit,
                    agent_id=self.agent_id,
                    enabled=True,
                )
                if result is not None and not result.on_track:
                    log.info(
                        "react: off-track at turn %s; reason=%s",
                        turn,
                        result.reason,
                    )

        try:
            result = run_multi_turn_loop(
                ctx,
                user_input=trigger,
                stop_event=stop_event,
                emit=self.emit,
                cost_totals=cost_totals,
                max_turns_override=self.max_steps,
                prompt_suffix=_REACT_SUFFIX,
                on_turn_end=_post_turn,
            )
        except _ReactLoopBreak:
            self.emit(
                {
                    "kind": "plan_end",
                    "session_id": self.session_id,
                    "domain_id": self.domain_id,
                    "agent_id": self.agent_id,
                    "payload": {"reason": "loop_detected"},
                }
            )
            return ""

        self.emit(
            {
                "kind": "plan_end",
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


class _ReactLoopBreak(Exception):
    """Internal sentinel raised when the loop detector fires."""


def build_react_agent(
    *,
    spec: dict[str, Any],
    config: dict[str, Any],
    agent_cfg: dict[str, Any] | None = None,
    emit: EmitFn,
    memory: Any = None,
) -> ReActAgent:
    """Construct a :class:`ReActAgent` from a DomainSpec.

    Picks the agent declared by ``harness.session.agent`` (or the first
    agent if unset). The orchestration config block is forwarded so
    ``max_steps`` / ``reflection`` / ``loop_threshold`` can be tuned per
    Domain.
    """
    harness = spec.get("harness", {}) or {}
    agents = harness.get("agents", {}) or {}
    session = harness.get("session", {}) or {}
    agent_name = session.get("agent") or next(iter(agents), "")
    if agent_name not in agents:
        raise AgentError(f"react strategy requires session.agent or a default agent")
    cfg = agent_cfg or agents[agent_name]
    adapter_kind = cfg.get("adapter", {}).get("kind", "noop")
    adapter = AdapterRegistry.get(adapter_kind)
    orchestration = harness.get("orchestration", {}) or {}
    react_cfg = orchestration.get("react") or {}

    agent = ReActAgent(
        agent_cfg=cfg,
        adapter=adapter,
        session_id=config.get("session_id", ""),
        domain_id=spec.get("id", ""),
        emit=emit,
        config={
            "spec": spec,
            "session_config": config,
            "memory": memory,
            "react": react_cfg,
        },
        max_steps=react_cfg.get("max_steps"),
        loop_threshold=react_cfg.get("loop_threshold", 3),
        reflection_enabled=react_cfg.get("reflection", False),
    )
    return agent


__all__ = ["ReActAgent", "build_react_agent"]
