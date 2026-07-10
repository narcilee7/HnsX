"""W12 Plan-and-Solve agent — generate a plan, then execute it step by step.

Flow:

  1. Ask the LLM to produce a numbered plan as a JSON array of step
     strings. Emitted as ``plan_start`` / ``plan_end`` observations.
  2. For each step, run a multi-turn loop with the step text injected as
     context. The agent uses tools to make progress on the step.
  3. After each step, optionally reflect; if reflection says we're off
     track and ``orchestration.plan_and_solve.replan_after_step`` is
     true, ask the LLM for a revised plan and re-queue remaining steps.
  4. Concatenate the step outputs and emit ``plan_complete``.

The Plan-and-Solve strategy is heavier than ReAct (two LLM calls per step
in the worst case) but gives the operator a clear audit trail of what
the agent intended vs what it actually did.
"""

from __future__ import annotations

import json
import logging
import threading
from collections.abc import Callable
from typing import Any

from hnsx_worker.adapters import AdapterRegistry

from .base import Agent, AgentError, EmitFn, parse_plan
from .reflection import reflect

log = logging.getLogger("hnsx_worker.agents.plan_solve")

EmitFn = Callable[[dict], None]

_PLAN_PROMPT = """\
You are a planning module. Given the task below, output a JSON array of
concrete, ordered steps. Each step should be one short sentence that an
agent can execute in a single multi-turn loop.

Task:
{task}

Output format: a JSON array of strings. Example: ["step 1", "step 2"]
"""


class PlanAndSolveAgent(Agent):
    """Strategy: plan_and_solve."""

    name = "plan_and_solve"

    def __init__(
        self,
        *,
        agent_cfg: dict[str, Any],
        adapter: Any,
        session_id: str,
        domain_id: str,
        emit: EmitFn,
        config: dict[str, Any] | None = None,
        max_replans: int = 2,
        reflection_enabled: bool = False,
        replan_after_step: bool = True,
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
        plan_cfg = (config or {}).get("plan_and_solve") or {}
        self.max_replans = int(
            max_replans if max_replans is not None else plan_cfg.get("max_replans", 2)
        )
        self.reflection_enabled = bool(reflection_enabled or plan_cfg.get("reflection", False))
        self.replan_after_step = bool(
            replan_after_step
            if replan_after_step is not None
            else plan_cfg.get("replan_after_step", True)
        )

    # ----------------------------------------------------------------- main loop

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

        ctx: AgentLoopContext = build_agent_loop_context(
            spec=spec,
            agent_cfg=self.agent_cfg,
            config=session_cfg,
            memory=memory,
        )

        goal_text = str(trigger.get("content") or json.dumps(trigger, ensure=str))

        # 1. Generate the initial plan.
        plan = self._generate_plan(goal_text, stop_event)
        if not plan:
            raise AgentError("plan_and_solve: failed to produce a plan")

        self.emit(
            {
                "kind": "plan_start",
                "session_id": self.session_id,
                "domain_id": self.domain_id,
                "agent_id": self.agent_id,
                "payload": {"strategy": "plan_and_solve", "steps": plan},
            }
        )

        step_outputs: list[str] = []
        replans_left = self.max_replans
        step_index = 0
        while step_index < len(plan):
            if stop_event.is_set():
                break
            step = plan[step_index]
            self.emit(
                {
                    "kind": "plan_step_start",
                    "session_id": self.session_id,
                    "domain_id": self.domain_id,
                    "agent_id": self.agent_id,
                    "payload": {"step": step_index + 1, "total": len(plan), "text": step},
                }
            )

            step_input = {
                "content": f"Step {step_index + 1}/{len(plan)}: {step}",
                "goal": goal_text,
                "plan": plan,
                "step_index": step_index,
            }
            cost_totals: dict[str, Any] = {
                "prompt_tokens": 0,
                "completion_tokens": 0,
                "cost_usd": 0.0,
                "latency_ms": 0,
            }
            observations_window: list[dict[str, Any]] = []
            try:
                result = run_multi_turn_loop(
                    ctx,
                    user_input=step_input,
                    stop_event=stop_event,
                    emit=self.emit,
                    cost_totals=cost_totals,
                    on_turn_end=lambda turn, info: observations_window.append(
                        {"turn": turn, "text": info.final_text[:300]}
                    ),
                )
            except Exception as e:  # noqa: BLE001
                log.warning("plan_and_solve step %s failed: %s", step_index + 1, e)
                result = {
                    "final_text": "",
                    "tool_call_count": 0,
                    "turn_count": 0,
                    "stop_reason": "error",
                }

            step_output = result["final_text"]
            step_outputs.append(step_output)
            self.emit(
                {
                    "kind": "plan_step_end",
                    "session_id": self.session_id,
                    "domain_id": self.domain_id,
                    "agent_id": self.agent_id,
                    "payload": {
                        "step": step_index + 1,
                        "output": step_output[:1000],
                        "tool_call_count": result["tool_call_count"],
                        "turn_count": result["turn_count"],
                        "stop_reason": result["stop_reason"],
                    },
                }
            )

            # 2. Optional reflection after the step.
            reflection_result = None
            if self.reflection_enabled and not stop_event.is_set():
                reflection_result = reflect(
                    adapter=self.adapter,
                    agent_cfg=self.agent_cfg,
                    goal=f"{goal_text} (current step: {step})",
                    history=observations_window,
                    session_id=self.session_id,
                    domain_id=self.domain_id,
                    emit=self.emit,
                    agent_id=self.agent_id,
                    enabled=True,
                )

            # 3. Optional replan.
            if (
                self.replan_after_step
                and replans_left > 0
                and reflection_result is not None
                and not reflection_result.on_track
            ):
                replans_left -= 1
                revised = self._revise_plan(goal_text, plan, step_index, step_output, stop_event)
                if revised:
                    remaining = plan[step_index + 1 :]
                    plan = plan[: step_index + 1] + revised
                    self.emit(
                        {
                            "kind": "plan_revised",
                            "session_id": self.session_id,
                            "domain_id": self.domain_id,
                            "agent_id": self.agent_id,
                            "payload": {
                                "after_step": step_index + 1,
                                "added_steps": revised,
                                "remaining_old_steps": remaining,
                            },
                        }
                    )

            step_index += 1

        final = "\n\n".join(step_outputs).strip()
        self.emit(
            {
                "kind": "plan_complete",
                "session_id": self.session_id,
                "domain_id": self.domain_id,
                "agent_id": self.agent_id,
                "payload": {"step_count": len(step_outputs), "replans_left": replans_left},
            }
        )
        return final

    # ----------------------------------------------------------------- helpers

    def _generate_plan(self, goal: str, stop_event: threading.Event) -> list[str]:
        prompt = _PLAN_PROMPT.format(task=goal)
        try:
            result = self.adapter.invoke(self.agent_cfg, prompt, {"planning": True})
        except Exception as e:  # noqa: BLE001
            log.warning("plan generation failed: %s", e)
            return []
        return parse_plan(result.text or "")

    def _revise_plan(
        self,
        goal: str,
        plan: list[str],
        step_index: int,
        last_output: str,
        stop_event: threading.Event,
    ) -> list[str]:
        """Ask the LLM to revise the remaining steps after an off-track result."""
        prompt = (
            "The agent went off-track after step "
            f"{step_index + 1} ('{plan[step_index]}').\n"
            f"Step output: {last_output[:500]}\n\n"
            "Original task: " + goal + "\n\n"
            "Output a JSON array of replacement steps for the remainder of "
            "the plan. Keep them concrete and short."
        )
        try:
            result = self.adapter.invoke(self.agent_cfg, prompt, {"replan": True})
        except Exception as e:  # noqa: BLE001
            log.warning("plan revision failed: %s", e)
            return []
        revised = parse_plan(result.text or "")
        return revised


def build_plan_and_solve_agent(
    *,
    spec: dict[str, Any],
    config: dict[str, Any],
    agent_cfg: dict[str, Any] | None = None,
    emit: EmitFn,
    memory: Any = None,
) -> PlanAndSolveAgent:
    """Construct a :class:`PlanAndSolveAgent` from a DomainSpec."""
    harness = spec.get("harness", {}) or {}
    agents = harness.get("agents", {}) or {}
    session = harness.get("session", {}) or {}
    agent_name = session.get("agent") or next(iter(agents), "")
    if agent_name not in agents:
        raise AgentError("plan_and_solve strategy requires session.agent or a default agent")
    cfg = agent_cfg or agents[agent_name]
    adapter_kind = cfg.get("adapter", {}).get("kind", "noop")
    adapter = AdapterRegistry.get(adapter_kind)
    orchestration = harness.get("orchestration", {}) or {}
    plan_cfg = orchestration.get("plan_and_solve") or {}

    agent = PlanAndSolveAgent(
        agent_cfg=cfg,
        adapter=adapter,
        session_id=config.get("session_id", ""),
        domain_id=spec.get("id", ""),
        emit=emit,
        config={
            "spec": spec,
            "session_config": config,
            "memory": memory,
            "plan_and_solve": plan_cfg,
        },
        max_replans=plan_cfg.get("max_replans", 2),
        reflection_enabled=plan_cfg.get("reflection", False),
        replan_after_step=plan_cfg.get("replan_after_step", True),
    )
    return agent


__all__ = ["PlanAndSolveAgent", "build_plan_and_solve_agent"]
