"""W12 advanced agent orchestration strategies.

Layered on top of W3 (Tool Registry), W5 (multi-agent supervisor), and
W11 (long-term memory). Each concrete agent shares a common :class:`Agent`
shape so :func:`hnsx_worker.session_executor.execute_session` can dispatch
on ``harness.orchestration.strategy`` without special-casing.

Strategies shipped in W12:

  - ``direct`` — pass-through (default; falls back to existing multi-turn loop).
  - ``react`` — :class:`ReActAgent`. Reuses the multi-turn loop but with a
    ReAct-flavored prompt and explicit reflection between turns.
  - ``plan_and_solve`` — :class:`PlanAndSolveAgent`. Generates a numbered
    plan first, then executes each step with optional reflection + replan.
  - ``multi_agent`` — :class:`MultiAgentRunner`. Same as direct, but
    exposes the ``delegate_to`` tool so the agent can hand work to peer
    agents declared in the same Domain.

All strategies emit rich observations (``plan_*``, ``reflection``,
``delegate``, ``sub_agent_*``) so Eval + Trace can introspect what
happened.
"""

from __future__ import annotations

from .base import Agent, AgentError, EmitFn, parse_plan, parse_react_step
from .multi_agent import MultiAgentRunner, build_multi_agent_runner
from .plan_solve_agent import PlanAndSolveAgent, build_plan_and_solve_agent
from .react_agent import ReActAgent, build_react_agent

__all__ = [
    "Agent",
    "AgentError",
    "EmitFn",
    "MultiAgentRunner",
    "PlanAndSolveAgent",
    "ReActAgent",
    "build_multi_agent_runner",
    "build_plan_and_solve_agent",
    "build_react_agent",
    "parse_plan",
    "parse_react_step",
]