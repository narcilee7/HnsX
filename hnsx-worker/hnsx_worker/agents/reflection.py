"""Reflection helper — gives a running agent the ability to assess progress.

W12 ships a lightweight ``reflect(...)`` utility that calls the same
adapter the agent uses (or a cheaper "judge" model if configured) and asks
two questions:

  1. ``on_track`` — has the agent made meaningful progress toward the goal
     in the last few turns?
  2. ``revised_plan`` — if progress is off track, what should change?

The output is emitted as a ``reflection`` observation. Strategies
(:class:`ReActAgent`, :class:`PlanAndSolveAgent`) call this between turns
or after each step.

Reflection is opt-in. The :func:`reflect` helper returns ``None`` when
the agent is not configured to use it (``orchestration.reflection.enabled
== False``) so callers don't need to special-case the disabled path.
"""

from __future__ import annotations

import json
import logging
import re
from collections.abc import Callable
from dataclasses import dataclass
from typing import Any

from hnsx_worker.adapters.base import Adapter

log = logging.getLogger("hnsx_worker.agents.reflection")

EmitFn = Callable[[dict], None]


_REFLECTION_PROMPT = """\
You are checking whether an agent is making progress on its goal.

Original task:
{goal}

Recent activity (last few turns):
{history}

Reply with a JSON object of the form:
{{
  "on_track": true | false,
  "reason": "<one-sentence rationale>",
  "revised_plan": ["step 1", "step 2", "..."]
}}

Only include "revised_plan" when on_track is false. If the agent already
finished, set on_track=true and revised_plan=[].
"""

_DECISION_RE = re.compile(r"\{[\s\S]*?\}")


@dataclass
class ReflectionResult:
    """Structured output from :func:`reflect`."""

    on_track: bool
    reason: str = ""
    revised_plan: list[str] | None = None


def _strip_code_fence(text: str) -> str:
    text = (text or "").strip()
    if text.startswith("```"):
        # Remove the first and last fence lines.
        parts = text.split("```")
        if len(parts) >= 3:
            inner = "```".join(parts[1:-1])
            text = inner.strip()
            if text.startswith("json"):
                text = text[4:].strip()
    return text


def _parse_reflection(text: str) -> ReflectionResult:
    """Best-effort JSON parse; falls back to a permissive on_track default."""
    cleaned = _strip_code_fence(text)
    m = _DECISION_RE.search(cleaned)
    candidate = m.group(0) if m else cleaned
    try:
        parsed = json.loads(candidate)
        if isinstance(parsed, dict):
            on_track = bool(parsed.get("on_track", True))
            reason = str(parsed.get("reason", ""))
            revised = parsed.get("revised_plan")
            if isinstance(revised, list) and all(isinstance(x, str) for x in revised):
                return ReflectionResult(on_track=on_track, reason=reason, revised_plan=revised)
            return ReflectionResult(on_track=on_track, reason=reason, revised_plan=None)
    except json.JSONDecodeError:
        pass
    # Permissive fallback: if "stuck" / "off track" appears in the text, mark
    # off-track; otherwise assume on-track.
    lowered = cleaned.lower()
    if "off track" in lowered or "stuck" in lowered or "replan" in lowered:
        return ReflectionResult(on_track=False, reason=cleaned[:200], revised_plan=None)
    return ReflectionResult(on_track=True, reason=cleaned[:200], revised_plan=None)


def reflect(
    *,
    adapter: Adapter,
    agent_cfg: dict[str, Any],
    goal: str,
    history: list[dict[str, Any]],
    session_id: str,
    domain_id: str,
    emit: EmitFn,
    agent_id: str = "",
    enabled: bool = True,
) -> ReflectionResult | None:
    """Call the adapter to assess progress.

    Returns ``None`` when ``enabled=False`` (so callers can short-circuit
    cheaply without inspecting the spec).
    """
    if not enabled:
        return None

    if not history:
        return ReflectionResult(on_track=True, reason="no history yet", revised_plan=None)

    history_text = _format_history(history)
    prompt = _REFLECTION_PROMPT.format(goal=goal, history=history_text)

    try:
        result = adapter.invoke(agent_cfg, prompt, {"reflection": True})
    except Exception as e:  # noqa: BLE001
        log.warning("reflection adapter.invoke failed: %s", e)
        return ReflectionResult(on_track=True, reason=f"reflection error: {e!s}", revised_plan=None)

    parsed = _parse_reflection(result.text or "")
    emit(
        {
            "kind": "reflection",
            "session_id": session_id,
            "domain_id": domain_id,
            "agent_id": agent_id,
            "payload": {
                "on_track": parsed.on_track,
                "reason": parsed.reason,
                "revised_plan": parsed.revised_plan,
                "raw": (result.text or "")[:1000],
            },
        }
    )
    return parsed


def _format_history(history: list[dict[str, Any]]) -> str:
    """Format the recent turn summary for the reflection prompt."""
    lines: list[str] = []
    for entry in history[-6:]:
        kind = entry.get("kind", "")
        payload = entry.get("payload", {}) or {}
        if kind == "agent_text":
            content = payload.get("content", "")
            lines.append(f"assistant: {str(content)[:300]}")
        elif kind == "tool_call":
            name = payload.get("name", "")
            lines.append(f"tool_call[{name}]: {str(payload.get('input', {}))[:200]}")
        elif kind == "tool_result":
            out = payload.get("output", "")
            err = payload.get("error", "")
            lines.append(f"tool_result: output={str(out)[:200]} error={err[:200]}".strip())
    return "\n".join(lines) or "(empty)"


__all__ = ["ReflectionResult", "reflect"]
