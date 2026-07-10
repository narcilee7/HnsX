"""Base types and helpers shared by all W12 agent strategies.

The :class:`Agent` ABC gives every strategy the same call shape::

    agent.run(trigger, *, stop_event) -> str

Each strategy decides how to drive the underlying adapter / tool registry
inside ``run``. Helpers here parse LLM text into structured signals
(``plan: [...]`` lists, ``Thought: ...`` / ``Action: ...`` markers) so
strategies can stay focused on loop semantics.
"""

from __future__ import annotations

import json
import logging
import re
from abc import ABC, abstractmethod
from collections.abc import Callable
from dataclasses import dataclass
from typing import Any

from hnsx_worker.adapters.base import Adapter

log = logging.getLogger("hnsx_worker.agents.base")


EmitFn = Callable[[dict], None]


class AgentError(Exception):
    """Raised when an agent cannot make progress."""


# ---------------------------------------------------------------------------
# Parsing helpers
# ---------------------------------------------------------------------------

# Match the first JSON array anywhere in the text.
_JSON_ARRAY_RE = re.compile(r"\[\s*[\s\S]*?\s*\]")


def parse_plan(text: str) -> list[str]:
    """Extract a numbered plan from an LLM reply.

    Accepts three shapes:

      1. A bare JSON array: ``["step 1", "step 2"]``.
      2. A JSON object containing a ``plan`` array.
      3. A markdown bullet / numbered list (``- foo`` / ``1. foo``).
    """
    text = (text or "").strip()
    if not text:
        return []

    # 1. Bare JSON array.
    m = _JSON_ARRAY_RE.search(text)
    if m:
        try:
            parsed = json.loads(m.group(0))
            if isinstance(parsed, list) and all(isinstance(x, str) for x in parsed):
                return [str(x).strip() for x in parsed if str(x).strip()]
        except json.JSONDecodeError:
            pass

    # 2. JSON object containing a "plan" / "steps" key.
    obj_start = text.find("{")
    obj_end = text.rfind("}")
    if obj_start >= 0 and obj_end > obj_start:
        try:
            parsed = json.loads(text[obj_start : obj_end + 1])
            if isinstance(parsed, dict):
                for key in ("plan", "steps", "todo"):
                    value = parsed.get(key)
                    if isinstance(value, list) and all(isinstance(x, str) for x in value):
                        return [str(x).strip() for x in value if str(x).strip()]
        except json.JSONDecodeError:
            pass

    # 3. Markdown bullet list.
    bullets: list[str] = []
    for line in text.splitlines():
        stripped = line.strip()
        if not stripped:
            continue
        # "- foo", "* foo", "1. foo", "1) foo"
        m = re.match(r"^(?:[-*]\s+|\d+[.)]\s+)(.+)$", stripped)
        if m:
            bullets.append(m.group(1).strip())
    return bullets


@dataclass
class ReactStep:
    """One parsed Thought / Action / Observation triple."""

    thought: str = ""
    action: str = ""  # raw action text (may be a tool name + JSON args)
    final_answer: str = ""


_REACT_THOUGHT_RE = re.compile(r"(?im)^\s*thought\s*:\s*(.+)$")
_REACT_ACTION_RE = re.compile(r"(?im)^\s*action\s*:\s*(.+)$")
_REACT_FINAL_RE = re.compile(r"(?im)^\s*final\s*answer\s*:\s*(.+)$")


def parse_react_step(text: str) -> ReactStep:
    """Best-effort extraction of a ReAct step from a free-form LLM reply.

    Modern tool-calling LLMs express ReAct through native tool calls rather
    than textual markers, so this is only used when the adapter falls back
    to plain text (e.g. Ollama or when ``react.style=text`` is forced).
    """
    text = (text or "").strip()
    step = ReactStep()
    if not text:
        return step

    tm = _REACT_THOUGHT_RE.search(text)
    if tm:
        step.thought = tm.group(1).strip()

    am = _REACT_ACTION_RE.search(text)
    if am:
        step.action = am.group(1).strip()

    fm = _REACT_FINAL_RE.search(text)
    if fm:
        step.final_answer = fm.group(1).strip()
    return step


# ---------------------------------------------------------------------------
# Base class
# ---------------------------------------------------------------------------


class Agent(ABC):
    """Strategy contract for W12.

    Subclasses own their loop. The executor pre-builds everything the
    strategy needs (adapter, tool registry, policy, secrets, memory) and
    hands it in via :meth:`bind`. ``run`` then does whatever the strategy
    says.
    """

    name: str = "agent"

    def __init__(
        self,
        *,
        agent_cfg: dict[str, Any],
        adapter: Adapter,
        session_id: str,
        domain_id: str,
        emit: EmitFn,
        config: dict[str, Any] | None = None,
        agent_id: str = "",
    ) -> None:
        self.agent_cfg = agent_cfg
        self.adapter = adapter
        self.session_id = session_id
        self.domain_id = domain_id
        self.emit = emit
        self.config = config or {}
        self.agent_id = agent_id or str(agent_cfg.get("id") or agent_cfg.get("name") or "")

    @abstractmethod
    def run(self, trigger: dict[str, Any], *, stop_event) -> str:
        """Execute the agent and return its final assistant text."""


__all__ = [
    "Agent",
    "AgentError",
    "EmitFn",
    "ReactStep",
    "parse_plan",
    "parse_react_step",
]
