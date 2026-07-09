"""Session executor — routes by ``session.mode`` and dispatches to adapters.

Supported modes:

  - ``single`` / ``single-task`` — invoke the named primary agent once.
  - ``multi-turn`` — invoke up to ``max_turns`` times; if the agent returns
    tool calls, synthesize a tool result and loop back (real Tool Registry
    lands in M3).
  - ``workflow`` — walk the static DAG (entry step → next step → ...).
  - ``supervisor`` / ``hierarchical`` / ``autonomous`` — NOT YET; raises.

Observations are emitted as Python dicts via the ``emit`` callable. The
subprocess entry (``session_runtime.py``) serializes them to JSONL on stdout.

Streaming contract:

  Adapters that implement :meth:`Adapter.invoke_stream` are streamed
  chunk-by-chunk. Each text delta becomes an ``agent_text_delta`` observation;
  each completed tool call becomes an ``agent_tool_call`` observation; the
  cost snapshot becomes an ``agent_cost`` observation. After the stream ends
  the executor emits a terminal ``agent_text`` (the concatenated text) so
  downstream consumers that only watch for the final observation still see
  the reply.

Multi-turn contract:

  The executor maintains a ``messages`` list (system / user / assistant /
  tool). On each turn it asks the adapter to produce the next assistant
  message. If the assistant produced tool calls the executor emits a
  ``tool_result`` observation with a synthetic stub result (the Tool Registry
  that produces real results lands in M3) and loops. The loop terminates
  when the assistant replies with no tool calls, the configured ``max_turns``
  is reached, or the harness sends SIGTERM.
"""

from __future__ import annotations

import json
import threading
from collections.abc import Callable
from typing import Any

from hnsx_worker.adapters import AdapterRegistry
from hnsx_worker.adapters.base import Adapter

EmitFn = Callable[[dict], None]

_DEFAULT_MAX_TURNS = 10


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
        _run_multi_turn(spec, trigger, config, stop_event=stop_event, emit=emit)
    elif mode == "workflow":
        _run_workflow(spec, trigger, config, stop_event=stop_event, emit=emit)
    elif mode in ("supervisor", "hierarchical", "autonomous"):
        raise NotImplementedError(
            f"session.mode={mode} is scheduled for M4; M2 only supports "
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
            "kind": "turn_start",
            "session_id": config.get("session_id", ""),
            "domain_id": spec.get("id", ""),
            "agent_id": agent_name,
            "payload": {"turn": 1, "adapter": adapter.name()},
        }
    )
    emit(
        {
            "kind": "agent_invoke",
            "session_id": config.get("session_id", ""),
            "domain_id": spec.get("id", ""),
            "agent_id": agent_name,
            "payload": {"adapter": adapter.name()},
        }
    )
    messages = _build_initial_messages(prompt, trigger)
    final_text, _tool_calls, cost = _stream_turn(
        adapter,
        agent,
        prompt,
        trigger,
        messages=messages,
        session_id=config.get("session_id", ""),
        domain_id=spec.get("id", ""),
        agent_id=agent_name,
        stop_event=stop_event,
        emit=emit,
    )
    if cost is not None:
        emit(
            {
                "kind": "agent_cost",
                "session_id": config.get("session_id", ""),
                "domain_id": spec.get("id", ""),
                "agent_id": agent_name,
                "payload": {
                    "prompt_tokens": cost.prompt_tokens,
                    "completion_tokens": cost.completion_tokens,
                    "cost_usd": cost.cost_usd,
                    "latency_ms": cost.latency_ms,
                },
            }
        )
    emit(
        {
            "kind": "agent_text",
            "session_id": config.get("session_id", ""),
            "domain_id": spec.get("id", ""),
            "agent_id": agent_name,
            "payload": {"content": final_text, "final": True},
        }
    )
    emit(
        {
            "kind": "turn_end",
            "session_id": config.get("session_id", ""),
            "domain_id": spec.get("id", ""),
            "agent_id": agent_name,
            "payload": {"turn": 1, "stop_reason": "natural"},
        }
    )


def _run_multi_turn(
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
        raise ValueError("multi-turn mode requires a non-empty agents map")
    if agent_name not in agents:
        raise KeyError(f"session.agent {agent_name!r} not in harness.agents")
    agent = agents[agent_name]
    prompt = _resolve_prompt(spec, agent)
    adapter = AdapterRegistry.get(agent.get("adapter", {}).get("kind", "noop"))
    max_turns = int(
        (harness.get("policy", {}) or {}).get("budget", {}).get("max_turns")
        or session.get("max_turns")
        or _DEFAULT_MAX_TURNS
    )

    _maybe_stop(stop_event, emit, config)

    messages = _build_initial_messages(prompt, trigger)
    session_id = config.get("session_id", "")
    domain_id = spec.get("id", "")
    final_text = ""
    stop_reason = "natural"

    for turn in range(1, max_turns + 1):
        if stop_event.is_set():
            stop_reason = "cancelled"
            break

        emit(
            {
                "kind": "turn_start",
                "session_id": session_id,
                "domain_id": domain_id,
                "agent_id": agent_name,
                "payload": {"turn": turn, "adapter": adapter.name(), "max_turns": max_turns},
            }
        )
        emit(
            {
                "kind": "agent_invoke",
                "session_id": session_id,
                "domain_id": domain_id,
                "agent_id": agent_name,
                "payload": {"adapter": adapter.name(), "turn": turn},
            }
        )

        text, tool_calls, cost = _stream_turn(
            adapter,
            agent,
            prompt,
            trigger,
            messages=messages,
            session_id=session_id,
            domain_id=domain_id,
            agent_id=agent_name,
            stop_event=stop_event,
            emit=emit,
            turn=turn,
        )
        final_text = text

        if cost is not None:
            emit(
                {
                    "kind": "agent_cost",
                    "session_id": session_id,
                    "domain_id": domain_id,
                    "agent_id": agent_name,
                    "payload": {
                        "turn": turn,
                        "prompt_tokens": cost.prompt_tokens,
                        "completion_tokens": cost.completion_tokens,
                        "cost_usd": cost.cost_usd,
                        "latency_ms": cost.latency_ms,
                    },
                }
            )

        # No tool calls: terminal assistant message; loop ends naturally.
        if not tool_calls:
            emit(
                {
                    "kind": "agent_text",
                    "session_id": session_id,
                    "domain_id": domain_id,
                    "agent_id": agent_name,
                    "payload": {"content": text, "final": True, "turn": turn},
                }
            )
            emit(
                {
                    "kind": "turn_end",
                    "session_id": session_id,
                    "domain_id": domain_id,
                    "agent_id": agent_name,
                    "payload": {"turn": turn, "stop_reason": "natural"},
                }
            )
            break

        # Tool calls: emit terminal text (may be empty) and each tool_call,
        # then synthesize tool results so the next turn has something to read.
        emit(
            {
                "kind": "agent_text",
                "session_id": session_id,
                "domain_id": domain_id,
                "agent_id": agent_name,
                "payload": {"content": text, "final": False, "turn": turn},
            }
        )
        for tc in tool_calls:
            emit(
                {
                    "kind": "tool_call",
                    "session_id": session_id,
                    "domain_id": domain_id,
                    "agent_id": agent_name,
                    "payload": {
                        "tool_call_id": tc.id,
                        "name": tc.name,
                        "input": tc.input,
                        "raw_input": tc.raw_input,
                        "turn": turn,
                    },
                }
            )
        emit(
            {
                "kind": "turn_end",
                "session_id": session_id,
                "domain_id": domain_id,
                "agent_id": agent_name,
                "payload": {"turn": turn, "stop_reason": "tool_call"},
            }
        )

        # Synthesize tool results + extend messages for the next turn.
        for tc in tool_calls:
            stub_result = _stub_tool_result(tc)
            emit(
                {
                    "kind": "tool_result",
                    "session_id": session_id,
                    "domain_id": domain_id,
                    "agent_id": agent_name,
                    "payload": {
                        "tool_call_id": tc.id,
                        "name": tc.name,
                        "output": stub_result,
                        "turn": turn,
                    },
                }
            )
            messages = _append_tool_result(messages, tc, stub_result, adapter_kind=adapter.name())

        if turn >= max_turns:
            stop_reason = "max_turns"
            break
    else:
        stop_reason = "max_turns"

    if stop_reason == "max_turns":
        emit(
            {
                "kind": "agent_text",
                "session_id": session_id,
                "domain_id": domain_id,
                "agent_id": agent_name,
                "payload": {"content": final_text, "final": True, "truncated": True},
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
                "payload": {"adapter": adapter.name()},
            }
        )
        messages = _build_initial_messages(prompt, step_input)
        text, _tool_calls, cost = _stream_turn(
            adapter,
            agent,
            prompt,
            step_input,
            messages=messages,
            session_id=config.get("session_id", ""),
            domain_id=spec.get("id", ""),
            agent_id=agent_name,
            step_id=step["id"],
            stop_event=stop_event,
            emit=emit,
        )
        if step.get("output"):
            vars_[step["output"]] = text
        if cost is not None:
            emit(
                {
                    "kind": "agent_cost",
                    "session_id": config.get("session_id", ""),
                    "domain_id": spec.get("id", ""),
                    "step_id": step["id"],
                    "agent_id": agent_name,
                    "payload": {
                        "prompt_tokens": cost.prompt_tokens,
                        "completion_tokens": cost.completion_tokens,
                        "cost_usd": cost.cost_usd,
                        "latency_ms": cost.latency_ms,
                    },
                }
            )
        emit(
            {
                "kind": "step_end",
                "session_id": config.get("session_id", ""),
                "domain_id": spec.get("id", ""),
                "step_id": step["id"],
                "agent_id": agent_name,
                "payload": {"output": text},
            }
        )
        next_id = step.get("next")
        if not next_id:
            return
        if next_id not in steps_by_id:
            raise KeyError(f"step {step['id']!r} next {next_id!r} not in steps")
        step = steps_by_id[next_id]


# ---------------------------------------------------------------------------
# streaming
# ---------------------------------------------------------------------------


def _stream_turn(
    adapter: Any,
    agent: dict,
    prompt: str,
    user_input: dict,
    *,
    messages: list[dict],
    session_id: str,
    domain_id: str,
    agent_id: str,
    step_id: str = "",
    stop_event: threading.Event,
    emit: EmitFn,
    turn: int = 1,
) -> tuple[str, list[Any], Any]:
    """Run one turn of the agent, preferring ``invoke_stream`` when available.

    Returns:
        (final_text, tool_calls, cost)
    """
    base_emit = {
        "session_id": session_id,
        "domain_id": domain_id,
        "agent_id": agent_id,
    }
    if step_id:
        base_emit["step_id"] = step_id

    # Pass messages history through the input dict so adapters can read it.
    # We use a hidden key so it never collides with trigger payloads.
    payload_input = dict(user_input)
    payload_input["_messages"] = messages

    text_parts: list[str] = []
    tool_calls: list[Any] = []
    cost: Any = None

    stream_fn = getattr(adapter, "invoke_stream", None)
    use_stream = callable(stream_fn) and stream_fn is not Adapter.invoke_stream

    if use_stream:
        try:
            for chunk in adapter.invoke_stream(agent, prompt, payload_input):
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
                if chunk.tool_call is not None:
                    tool_calls.append(chunk.tool_call)
                    emit(
                        {
                            **base_emit,
                            "kind": "agent_tool_call",
                            "payload": {
                                "tool_call_id": chunk.tool_call.id,
                                "name": chunk.tool_call.name,
                                "input": chunk.tool_call.input,
                                "raw_input": chunk.tool_call.raw_input,
                                "turn": turn,
                            },
                        }
                    )
                if chunk.cost is not None:
                    cost = chunk.cost
        except NotImplementedError:
            use_stream = False
        except Exception:
            # Stream failed mid-flight; fall through to non-stream retry.
            use_stream = False

    if not use_stream:
        result = adapter.invoke(agent, prompt, payload_input)
        text_parts.append(result.text)
        tool_calls = list(result.tool_calls)
        cost = result.cost

    return "".join(text_parts), tool_calls, cost


def _build_initial_messages(prompt: str, user_input: dict) -> list[dict]:
    """Build the initial messages list (system + user)."""
    messages: list[dict] = []
    if prompt:
        messages.append({"role": "system", "content": prompt})
    messages.append({"role": "user", "content": _input_to_user_content(user_input)})
    return messages


def _append_tool_result(
    messages: list[dict],
    tool_call: Any,
    result: Any,
    *,
    adapter_kind: str,
) -> list[dict]:
    """Append an assistant tool_call message + tool result to the history.

    Format follows the OpenAI / Anthropic "tool result" pattern so adapters
    can map them directly to provider-native message types.
    """
    assistant_msg: dict[str, Any] = {"role": "assistant", "content": ""}
    if adapter_kind == "openai":
        assistant_msg["tool_calls"] = [
            {
                "id": tool_call.id,
                "type": "function",
                "function": {
                    "name": tool_call.name,
                    "arguments": tool_call.raw_input or json.dumps(tool_call.input),
                },
            }
        ]
        messages.append(assistant_msg)
        messages.append(
            {
                "role": "tool",
                "tool_call_id": tool_call.id,
                "content": json.dumps(result, ensure_ascii=False, default=str),
            }
        )
    else:
        # Anthropic and generic providers: use the tool_use block shape.
        assistant_msg["content"] = [
            {
                "type": "tool_use",
                "id": tool_call.id,
                "name": tool_call.name,
                "input": tool_call.input,
            }
        ]
        messages.append(assistant_msg)
        messages.append(
            {
                "role": "user",
                "content": [
                    {
                        "type": "tool_result",
                        "tool_use_id": tool_call.id,
                        "content": json.dumps(result, ensure_ascii=False, default=str),
                    }
                ],
            }
        )
    return messages


def _stub_tool_result(tool_call: Any) -> dict:
    """Placeholder tool result.

    Real Tool Registry (http/shell/sql/python) lands in M3. For now the
    executor emits an empty / echo result so multi-turn loops can still
    terminate and the agent gets *something* to react to.
    """
    return {
        "ok": True,
        "stub": True,
        "name": tool_call.name,
        "echo_input": tool_call.input,
        "message": "tool registry not implemented yet (M3)",
    }


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


def _input_to_user_content(input: dict) -> str:
    """Serialize a turn input for the user-role message."""
    payload = input.get("content")
    if payload is not None and not isinstance(payload, (dict, list)):
        return str(payload)
    return json.dumps(input, ensure_ascii=False, default=str)


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