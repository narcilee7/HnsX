"""Session executor — routes by ``session.mode`` and dispatches to adapters.

Step 2 supports four modes:

  - ``single`` / ``single-task`` — invoke the named primary agent once.
  - ``multi-turn`` — invoke once (full multi-turn semantics land in #5).
  - ``workflow`` — walk the static DAG (entry step → next step → ...).
  - ``supervisor`` / ``hierarchical`` / ``autonomous`` — NOT YET; raises.

Observations are emitted as Python dicts via the ``emit`` callable. The
subprocess entry (``session_runtime.py``) serializes them to JSONL on stdout.
"""

from __future__ import annotations

import threading
from collections.abc import Callable
from typing import Any

from hnsx_worker.adapters import AdapterRegistry

EmitFn = Callable[[dict], None]


def execute_session(
    spec: dict[str, Any],
    trigger: dict[str, Any],
    config: dict[str, Any],
    *,
    stop_event: threading.Event,
    emit: EmitFn,
) -> None:
    """Dispatch to the right mode-specific runner.

    Args:
        spec: Parsed DomainSpec (Python dict).
        trigger: Parsed trigger payload.
        config: The whole config dict read from stdin (contains
            ``session_id`` / ``correlation_id`` / etc.).
        stop_event: Set by the SIGTERM handler to ask for graceful exit.
        emit: Sink for observations (one Python dict per call).
    """
    mode = _read_mode(spec)
    if mode in ("single", "single-task"):
        _run_single(spec, trigger, config, stop_event=stop_event, emit=emit)
    elif mode == "multi-turn":
        # Step 2: behave as single-task. Full multi-turn loop with budget
        # enforcement + memory handover is #5.
        _run_single(spec, trigger, config, stop_event=stop_event, emit=emit)
    elif mode == "workflow":
        _run_workflow(spec, trigger, config, stop_event=stop_event, emit=emit)
    elif mode in ("supervisor", "hierarchical", "autonomous"):
        raise NotImplementedError(
            f"session.mode={mode} is scheduled for #5; Step 2 only supports "
            f"single / single-task / multi-turn / workflow"
        )
    else:
        raise ValueError(f"unknown session.mode: {mode!r}")


# ---------------------------------------------------------------------------
# mode runners
# ---------------------------------------------------------------------------


def _run_single(
    spec: dict,
    trigger: dict,
    config: dict,
    *,
    stop_event: threading.Event,
    emit: EmitFn,
) -> None:
    harness = spec.get("harness", {})
    agents: dict = harness.get("agents", {})
    session = harness.get("session", {})

    agent_name = session.get("agent") or _first_key(agents)
    if not agent_name:
        raise ValueError("single mode requires a non-empty agents map")
    if agent_name not in agents:
        raise KeyError(f"session.agent {agent_name!r} not in harness.agents")
    agent = agents[agent_name]
    prompt = _resolve_prompt(spec, agent)
    adapter = AdapterRegistry.get(agent.get("adapter", {}).get("kind", "noop"))

    _maybe_stop(stop_event, emit, config)

    emit(
        {
            "kind": "agent_invoke",
            "session_id": config.get("session_id", ""),
            "domain_id": spec.get("id", ""),
            "agent_id": agent_name,
            "payload": {"adapter": adapter.name()},
        }
    )
    result = adapter.invoke(agent, prompt, trigger)
    emit(
        {
            "kind": "agent_text",
            "session_id": config.get("session_id", ""),
            "domain_id": spec.get("id", ""),
            "agent_id": agent_name,
            "payload": {"content": result.text},
        }
    )


def _run_workflow(
    spec: dict,
    trigger: dict,
    config: dict,
    *,
    stop_event: threading.Event,
    emit: EmitFn,
) -> None:
    harness = spec.get("harness", {})
    agents: dict = harness.get("agents", {})
    session = harness.get("session", {})
    workflow = session.get("workflow")
    if not workflow:
        raise ValueError("workflow mode requires harness.session.workflow")

    steps_by_id: dict[str, dict] = {s["id"]: s for s in workflow.get("steps", [])}
    entry = workflow.get("entry")
    if entry not in steps_by_id:
        raise KeyError(f"workflow.entry {entry!r} not found in steps")

    step = steps_by_id[entry]
    seen: set[str] = set()
    vars_: dict = dict(trigger)
    if isinstance(workflow.get("variables"), dict):
        for k, v in workflow["variables"].items():
            vars_.setdefault(k, v)

    while step:
        if step["id"] in seen:
            raise ValueError(f"workflow cycle at step {step['id']!r}")
        seen.add(step["id"])
        _maybe_stop(stop_event, emit, config)

        agent_name = step.get("agent")
        if agent_name not in agents:
            raise KeyError(f"step {step['id']!r} references unknown agent {agent_name!r}")
        agent = agents[agent_name]
        prompt = _resolve_prompt(spec, agent)
        adapter = AdapterRegistry.get(agent.get("adapter", {}).get("kind", "noop"))
        step_input = _build_step_input(step.get("input"), vars_)

        emit(
            {
                "kind": "step_start",
                "session_id": config.get("session_id", ""),
                "domain_id": spec.get("id", ""),
                "step_id": step["id"],
                "agent_id": agent_name,
            }
        )
        result = adapter.invoke(agent, prompt, step_input)
        if step.get("output"):
            vars_[step["output"]] = result.text
        emit(
            {
                "kind": "step_end",
                "session_id": config.get("session_id", ""),
                "domain_id": spec.get("id", ""),
                "step_id": step["id"],
                "agent_id": agent_name,
                "payload": {"output": result.text},
            }
        )
        next_id = step.get("next")
        if not next_id:
            return
        if next_id not in steps_by_id:
            raise KeyError(f"step {step['id']!r} next {next_id!r} not in steps")
        step = steps_by_id[next_id]


# ---------------------------------------------------------------------------
# helpers
# ---------------------------------------------------------------------------


def _read_mode(spec: dict) -> str:
    return (spec.get("harness", {}).get("session", {}) or {}).get("mode", "")


def _first_key(d: dict) -> str:
    return next(iter(d), "")


def _resolve_prompt(spec: dict, agent: dict) -> str:
    """Return the prompt string for the given agent.

    If ``agent.system_prompt`` references a named prompt in
    ``harness.prompts``, use that template; otherwise treat the value
    as a literal string.
    """
    sp = agent.get("system_prompt") or ""
    if not sp:
        return ""
    prompts = spec.get("harness", {}).get("prompts", {}) or {}
    if sp in prompts:
        return prompts[sp].get("template", "")
    return sp


def _build_step_input(raw: Any, vars_: dict) -> dict:
    out: dict = {}
    if isinstance(raw, dict):
        for k, v in raw.items():
            out[k] = _interpolate(v, vars_)
    for k, v in vars_.items():
        out.setdefault(k, v)
    return out


def _interpolate(value: Any, vars_: dict) -> Any:
    if isinstance(value, str):
        return _expand(value, vars_)
    if isinstance(value, dict):
        return {k: _interpolate(v, vars_) for k, v in value.items()}
    if isinstance(value, list):
        return [_interpolate(v, vars_) for v in value]
    return value


def _expand(s: str, vars_: dict) -> str:
    """Replace ``${var}`` placeholders with their JSON-encoded values."""
    out = s
    while True:
        start = out.find("${")
        if start < 0:
            return out
        end = out.find("}", start + 2)
        if end < 0:
            return out
        key = out[start + 2 : end]
        if key not in vars_:
            return out
        import json as _json

        replacement = _json.dumps(vars_[key])
        out = out[:start] + replacement + out[end + 1 :]


def _maybe_stop(stop_event: threading.Event, emit: EmitFn, config: dict) -> None:
    if stop_event.is_set():
        emit(
            {
                "kind": "session_end",
                "session_id": config.get("session_id", ""),
                "state": "cancelled",
                "payload": {"reason": "stop_event set"},
            }
        )
        raise _Stopped()


class _Stopped(Exception):
    """Sentinel raised by ``_maybe_stop`` to unwind the executor cleanly."""
