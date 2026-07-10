"""Transition expression evaluation for multi-agent orchestration.

W5 introduces dynamic routing: after an agent produces output, the Harness
evaluates one or more transition rules to decide which agent runs next.
Expressions are JMESPath-like and evaluated against a context object that
contains the latest observations and the current agent's output.

Example DomainSpec::

    session:
      mode: supervisor
      supervisor:
        agent: triage
        transitions:
          - condition: $.output.intent == 'billing'
            to: billing
          - condition: $.output.intent == 'technical'
            to: technical
        exit:
          - condition: $.output.escalate == true
            state: failed
            reason: escalation required

The ``condition`` string is passed to :func:`evaluate_condition`. It returns a
boolean. The caller (``harness.runner``) picks the first matching transition.
"""

from __future__ import annotations

import logging
from typing import Any

try:
    import jmespath
except ImportError as _jmespath_import_error:  # pragma: no cover
    jmespath = None  # type: ignore[assignment]

log = logging.getLogger("hnsx_worker.harness.transition")


def evaluate_condition(condition: str, context: dict[str, Any]) -> bool:
    """Evaluate a JMESPath boolean condition against ``context``.

    Args:
        condition: A JMESPath expression that should return a truthy/falsy
            value, e.g. ``"$.output.intent == 'billing'"``.
        context: The evaluation context (see :func:`build_context`).

    Returns:
        ``True`` if the expression evaluates to a truthy value, else
        ``False``. Syntax errors and type errors are logged and treated as
        ``False`` so a bad transition rule doesn't crash the session.
    """
    if not condition:
        return True

    if jmespath is None:
        log.warning("jmespath not installed; falling back to literal false")
        return False

    # jmespath.search expects a dict root; our context is already that.
    try:
        result = jmespath.search(condition, context)
    except Exception as e:  # noqa: BLE001
        log.warning("transition condition error: %s: %s", condition, e)
        return False

    if isinstance(result, bool):
        return result
    # Treat non-boolean results by truthiness.
    return bool(result)


def build_context(
    *,
    output: Any = None,
    observations: list[dict[str, Any]] | None = None,
    vars_: dict[str, Any] | None = None,
    agent_id: str = "",
    turn: int = 0,
) -> dict[str, Any]:
    """Build the evaluation context used by transition conditions.

    The context exposes:

      - ``output`` — the last agent's structured output (or raw text).
      - ``observations`` — the observation stream emitted so far.
      - ``vars`` — workflow variables accumulated across steps/turns.
      - ``agent_id`` / ``turn`` — metadata for debugging.
    """
    return {
        "output": output,
        "observations": observations or [],
        "vars": vars_ or {},
        "agent_id": agent_id,
        "turn": turn,
    }


__all__ = ["evaluate_condition", "build_context"]
