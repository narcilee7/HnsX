"""Multi-agent orchestration runner (supervisor / hierarchical / autonomous).

The runner implements the W5 orchestration loop:

  1. Load and validate the DomainSpec.
  2. Run the supervisor agent; parse its output as a routing decision.
  3. Evaluate transition rules against the decision; pick the target agent.
  4. Run the target (specialist) agent.
  5. Evaluate exit rules; either finish or return to step 2.

Observations emitted:

  - ``routing_decision`` — the supervisor's parsed decision (to / reason /
    confidence).
  - ``specialist_start`` / ``specialist_end`` — lifecycle of a specialist run.
  - ``supervisor_decision`` — alias for routing_decision (kept for backward
    compatibility with early W5 docs).
"""

from __future__ import annotations

import json
import logging
import threading
from collections.abc import Callable
from typing import Any

from hnsx_worker.adapters import AdapterRegistry
from hnsx_worker.adapters.base import Adapter

from .loader import HarnessSpec, load
from .transition import build_context, evaluate_condition

log = logging.getLogger("hnsx_worker.harness.runner")

EmitFn = Callable[[dict], None]
_DEFAULT_MAX_TURNS = 10


class OrchestrationError(Exception):
    """Raised when the orchestration loop cannot make progress."""


def run(
    spec: dict[str, Any],
    trigger: dict[str, Any],
    config: dict[str, Any],
    *,
    stop_event: threading.Event,
    emit: EmitFn,
) -> None:
    """Entry point for supervisor / hierarchical / autonomous modes.

    Args:
        spec: Parsed DomainSpec.
        trigger: User trigger payload.
        config: Runtime config (session_id, etc.).
        stop_event: Cancellation signal.
        emit: Observation sink.
    """
    harness = load(spec)
    mode = harness.mode
    session_id = config.get("session_id", "")
    domain_id = spec.get("id", "")

    supervisor_cfg = harness.supervisor_cfg
    supervisor_name = str(supervisor_cfg.get("agent"))
    transitions = list(supervisor_cfg.get("transitions") or [])
    exit_rules = list(supervisor_cfg.get("exit") or [])
    max_turns = int(
        (harness.harness.get("policy", {}) or {}).get("budget", {}).get("max_turns")
        or harness.session.get("max_turns")
        or _DEFAULT_MAX_TURNS
    )

    current_input = dict(trigger)
    vars_: dict[str, Any] = {}
    observations: list[dict[str, Any]] = []

    def _emit(kind: str, payload: dict, **extra: Any) -> None:
        obs = {
            "kind": kind,
            "session_id": session_id,
            "domain_id": domain_id,
            "payload": payload,
        }
        obs.update(extra)
        observations.append(obs)
        emit(obs)

    for turn in range(1, max_turns + 1):
        if stop_event.is_set():
            _emit(
                "session_end",
                {"reason": "cancelled", "turn": turn},
                state="cancelled",
            )
            return

        # ------------------------------------------------------------------ supervisor
        supervisor_agent = harness.get_agent(supervisor_name)
        supervisor_adapter = AdapterRegistry.get(
            supervisor_agent.get("adapter", {}).get("kind", "noop")
        )
        _emit(
            "turn_start",
            {"turn": turn, "agent": supervisor_name, "mode": mode},
            agent_id=supervisor_name,
        )
        supervisor_text = _run_agent(
            supervisor_adapter,
            supervisor_agent,
            harness,
            current_input,
            session_id=session_id,
            domain_id=domain_id,
            agent_id=supervisor_name,
            stop_event=stop_event,
            emit=emit,
            turn=turn,
        )
        _emit(
            "turn_end",
            {"turn": turn, "agent": supervisor_name, "stop_reason": "natural"},
            agent_id=supervisor_name,
        )

        decision = _parse_routing_decision(supervisor_text)
        _emit(
            "routing_decision",
            {
                "turn": turn,
                "to": decision.get("to", ""),
                "reason": decision.get("reason", ""),
                "confidence": decision.get("confidence"),
                "raw": supervisor_text,
            },
            agent_id=supervisor_name,
        )
        # Backward-compatible alias.
        _emit(
            "supervisor_decision",
            {
                "turn": turn,
                "to": decision.get("to", ""),
                "reason": decision.get("reason", ""),
                "confidence": decision.get("confidence"),
            },
            agent_id=supervisor_name,
        )

        # ------------------------------------------------------------------ transitions
        transition_ctx = build_context(
            output=decision,
            observations=observations,
            vars_=vars_,
            agent_id=supervisor_name,
            turn=turn,
        )

        # Find a matching transition.
        target_name: str | None = None
        for rule in transitions:
            if evaluate_condition(rule.get("condition", ""), transition_ctx):
                target_name = str(rule.get("to"))
                break

        if target_name is None:
            # No transition matched. For supervisor mode, loop back to the
            # supervisor with the same input. For autonomous mode this is a
            # failure.
            if mode == "autonomous":
                raise OrchestrationError(
                    "no transition matched and mode=autonomous does not allow fallback"
                )
            log.info("no transition matched at turn %s; returning to supervisor", turn)
            continue

        # ------------------------------------------------------------------ specialist
        specialist_agent = harness.get_agent(target_name)
        specialist_adapter = AdapterRegistry.get(
            specialist_agent.get("adapter", {}).get("kind", "noop")
        )
        _emit(
            "specialist_start",
            {"turn": turn, "from": supervisor_name, "to": target_name},
            agent_id=target_name,
        )
        specialist_input = _build_specialist_input(
            specialist_agent, current_input, decision, vars_
        )
        specialist_text = _run_agent(
            specialist_adapter,
            specialist_agent,
            harness,
            specialist_input,
            session_id=session_id,
            domain_id=domain_id,
            agent_id=target_name,
            stop_event=stop_event,
            emit=emit,
            turn=turn,
        )
        _emit(
            "specialist_end",
            {
                "turn": turn,
                "from": supervisor_name,
                "to": target_name,
                "output": specialist_text,
            },
            agent_id=target_name,
        )

        # ------------------------------------------------------------------ exit
        exit_ctx = build_context(
            output={"text": specialist_text},
            observations=observations,
            vars_=vars_,
            agent_id=target_name,
            turn=turn,
        )
        for rule in exit_rules:
            if evaluate_condition(rule.get("condition", ""), exit_ctx):
                state = rule.get("state", "completed")
                reason = rule.get("reason", "exit rule matched")
                _emit(
                    "session_end",
                    {"reason": reason, "turn": turn, "matched_exit": rule},
                    state=state,
                )
                return

        # Prepare input for the next supervisor turn.
        current_input = {"previous_output": specialist_text, **dict(trigger)}
        vars_["last_specialist"] = target_name
        vars_["last_output"] = specialist_text

    # Max turns exhausted.
    _emit(
        "session_end",
        {"reason": "max_turns reached", "turn": max_turns},
        state="failed",
    )


def _run_agent(
    adapter: Adapter,
    agent: dict,
    harness: HarnessSpec,
    user_input: dict,
    *,
    session_id: str,
    domain_id: str,
    agent_id: str,
    stop_event: threading.Event,
    emit: EmitFn,
    turn: int,
) -> str:
    """Run one agent and return its final text."""
    prompt = _resolve_prompt(harness, agent)
    text_parts: list[str] = []

    base_emit = {
        "session_id": session_id,
        "domain_id": domain_id,
        "agent_id": agent_id,
    }

    emit(
        {
            **base_emit,
            "kind": "agent_invoke",
            "payload": {"adapter": adapter.name(), "turn": turn},
        }
    )

    stream_fn = getattr(adapter, "invoke_stream", None)
    use_stream = callable(stream_fn)

    if use_stream:
        try:
            for chunk in adapter.invoke_stream(agent, prompt, user_input):
                if stop_event.is_set():
                    break
                if chunk.text_delta:
                    text_parts.append(chunk.text_delta)
                    emit(
                        {
                            **base_emit,
                            "kind": "agent_text_delta",
                            "payload": {"content": chunk.text_delta, "turn": turn},
                        }
                    )
                if chunk.cost is not None:
                    emit(
                        {
                            **base_emit,
                            "kind": "agent_cost",
                            "payload": {
                                "turn": turn,
                                "prompt_tokens": chunk.cost.prompt_tokens,
                                "completion_tokens": chunk.cost.completion_tokens,
                                "cost_usd": chunk.cost.cost_usd,
                                "latency_ms": chunk.cost.latency_ms,
                            },
                        }
                    )
        except NotImplementedError:
            use_stream = False

    if not use_stream:
        result = adapter.invoke(agent, prompt, user_input)
        text_parts.append(result.text)

    final_text = "".join(text_parts)
    emit(
        {
            **base_emit,
            "kind": "agent_text",
            "payload": {"content": final_text, "final": True, "turn": turn},
        }
    )
    return final_text


def _parse_routing_decision(text: str) -> dict[str, Any]:
    """Parse the supervisor's text output into a routing decision.

    Best effort: if the text contains a JSON object, extract it. Otherwise
    treat the whole text as the ``reason`` and leave ``to`` empty (the
    transition rules can match on reason text if they want).
    """
    text = text.strip()
    if not text:
        return {"to": "", "reason": "", "confidence": None}

    # Try to find a JSON object in the text.
    start = text.find("{")
    end = text.rfind("}")
    if start >= 0 and end > start:
        try:
            parsed = json.loads(text[start : end + 1])
            if isinstance(parsed, dict):
                return {
                    "to": str(parsed.get("to", "")),
                    "reason": str(parsed.get("reason", "")),
                    "confidence": parsed.get("confidence"),
                }
        except json.JSONDecodeError:
            pass

    # Fallback: treat the raw text as the reason.
    return {"to": "", "reason": text, "confidence": None}


def _build_specialist_input(
    specialist_agent: dict,
    trigger: dict,
    decision: dict,
    vars_: dict,
) -> dict:
    """Build the input handed to the specialist agent."""
    out: dict = {"trigger": trigger, "routing": decision}
    if vars_:
        out["vars"] = vars_
    return out


def _resolve_prompt(harness: HarnessSpec, agent: dict) -> str:
    """Resolve the agent's system prompt."""
    sp = agent.get("system_prompt") or ""
    if not sp:
        return ""
    if sp in harness.prompts:
        return harness.prompts[sp].get("template", "")
    return sp


__all__ = ["run", "OrchestrationError"]
