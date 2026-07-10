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
import logging
import threading
import time
from collections.abc import Callable
from dataclasses import dataclass
from typing import Any

from hnsx_worker.adapters import AdapterRegistry
from hnsx_worker.adapters.base import Adapter
from hnsx_worker.harness import run as run_orchestration
from hnsx_worker.memory import MemoryStore
from hnsx_worker.memory import build_backend as build_memory_backend
from hnsx_worker.policy import PolicyEngine
from hnsx_worker.sandbox import build_backend as build_sandbox_backend
from hnsx_worker.store import build_backend as build_store_backend
from hnsx_worker.tools import (
    ToolContext,
    ToolDecision,
    ToolRegistry,
    ToolResult,
    build_tool,
    tool_schemas_for_adapter,
)
from hnsx_worker.tools.mcp_client import (
    build_mcp_server_map,
    discover_mcp_tools,
)
from hnsx_worker.tools.memory import (
    MemoryForgetTool,
    MemorySearchTool,
    MemoryStoreTool,
)

log = logging.getLogger("hnsx_worker.session_executor")

EmitFn = Callable[[dict], None]

_DEFAULT_MAX_TURNS = 10


def _accumulate_cost(cost: Any, totals: dict[str, Any]) -> None:
    """Add a per-turn Cost snapshot into running totals."""
    if cost is None:
        return
    totals["prompt_tokens"] += int(getattr(cost, "prompt_tokens", 0) or 0)
    totals["completion_tokens"] += int(getattr(cost, "completion_tokens", 0) or 0)
    totals["cost_usd"] += float(getattr(cost, "cost_usd", 0.0) or 0.0)
    totals["latency_ms"] += int(getattr(cost, "latency_ms", 0) or 0)


def execute_session(
    spec: dict[str, Any],
    trigger: dict[str, Any],
    config: dict[str, Any],
    *,
    stop_event: threading.Event,
    emit: EmitFn,
) -> dict[str, Any]:
    """Dispatch to the right mode-specific runner."""
    mode = _read_mode(spec)
    strategy = _read_strategy(spec)
    result: dict[str, Any] = {
        "output": "",
        "prompt_tokens": 0,
        "completion_tokens": 0,
        "cost_usd": 0.0,
        "latency_ms": 0,
    }

    memory_cfg = (spec.get("harness") or {}).get("memory")
    memory: MemoryStore | None = build_memory_backend(memory_cfg) if memory_cfg else None

    # W12: orchestration.strategy overrides session.mode when set to one of
    # the advanced strategies. ``direct`` keeps existing mode semantics.
    used_strategy = False
    try:
        if strategy == "react":
            result["output"] = _run_strategy_agent(
                spec, trigger, config,
                stop_event=stop_event,
                emit=emit,
                cost_totals=result,
                memory=memory,
                builder="react",
            )
            used_strategy = True
        elif strategy == "plan_and_solve":
            result["output"] = _run_strategy_agent(
                spec, trigger, config,
                stop_event=stop_event,
                emit=emit,
                cost_totals=result,
                memory=memory,
                builder="plan_and_solve",
            )
            used_strategy = True
        elif strategy == "multi_agent":
            result["output"] = _run_strategy_agent(
                spec, trigger, config,
                stop_event=stop_event,
                emit=emit,
                cost_totals=result,
                memory=memory,
                builder="multi_agent",
            )
            used_strategy = True

        if not used_strategy:
            if mode in ("single", "single-task"):
                result["output"] = _run_single(
                    spec, trigger, config, stop_event=stop_event, emit=emit, cost_totals=result
                )
            elif mode == "multi-turn":
                result["output"] = _run_multi_turn(
                    spec,
                    trigger,
                    config,
                    stop_event=stop_event,
                    emit=emit,
                    cost_totals=result,
                    memory=memory,
                )
            elif mode == "workflow":
                result["output"] = _run_workflow(
                    spec, trigger, config, stop_event=stop_event, emit=emit, cost_totals=result
                )
            elif mode in ("supervisor", "hierarchical", "autonomous"):
                run_orchestration(
                    spec, trigger, config, stop_event=stop_event, emit=emit
                )
            else:
                raise ValueError(f"unknown session.mode: {mode!r}")

        # W11: persist a session summary into long-term memory when configured.
        if memory is not None:
            _store_session_summary(
                memory,
                config.get("session_id", ""),
                spec,
                trigger,
                result.get("output", ""),
                emit=emit,
            )
    finally:
        if memory is not None:
            memory.close()

    # W9: output guardrails on the final assistant text (for modes that return
    # a final text before session_end).
    if mode not in ("supervisor", "hierarchical", "autonomous"):
        policy_for_output = PolicyEngine(
            spec,
            session_id=config.get("session_id", ""),
            domain_id=spec.get("id", ""),
            emit=emit,
        )
        guard = policy_for_output.check_output(result.get("output", ""))
        if not guard.allow:
            result["guardrail_violation"] = {
                "rule": guard.rule,
                "reason": guard.reason,
            }

    # W9: if an EvalSet was injected, run every case and produce a report.
    eval_set = config.get("eval_set")
    if eval_set:
        from hnsx_worker.eval import run_eval_set

        report = run_eval_set(
            eval_set,
            execute_session,
            spec,
            config,
            stop_event=stop_event,
            emit=emit,
            baseline_report=config.get("baseline_report"),
        )
        result["output"] = json.dumps(report, ensure_ascii=False, default=str)
        result["eval_report"] = report
        return result

    # W8: if an EvalCase was injected, run scorers against the final output.
    eval_case = config.get("eval_case")
    if eval_case:
        from hnsx_worker.eval import aggregate_scores, run_eval

        scores = run_eval(eval_case, result.get("output"), emit=emit)
        result["eval_scores"] = aggregate_scores(scores)

    return result


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
    cost_totals: dict[str, Any],
) -> str:
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
    _accumulate_cost(cost, cost_totals)
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
    return final_text


def _run_multi_turn(
    spec: dict,
    trigger: dict,
    config: dict,
    *,
    stop_event: threading.Event,
    emit: EmitFn,
    cost_totals: dict[str, Any],
    memory: MemoryStore | None = None,
) -> str:
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

    session_id = config.get("session_id", "")
    domain_id = spec.get("id", "")

    # W6 runtime services: policy, sandbox, store.
    policy = PolicyEngine(
        spec,
        session_id=session_id,
        domain_id=domain_id,
        agent_id=agent_name,
        emit=emit,
    )
    # W6 runtime services: policy, sandbox, store. Sandbox is built but not
    # yet wired into every tool — W6 exposes the backend; later work can pass
    # it to tools that need process/container isolation.
    _sandbox = build_sandbox_backend(harness.get("sandbox"))
    store = build_store_backend(harness.get("store"))

    # Restore prior messages from the store (no-op for in_memory on first turn).
    messages: list[dict] = store.get(session_id, "messages") or []
    if not messages:
        messages = _build_initial_messages(prompt, trigger)

    final_text = ""
    stop_reason = "natural"

    # Build the ToolRegistry from the agent's ``tools`` spec entries.
    registry, schema_failures = _build_tool_registry(
        spec=spec,
        agent=agent,
        session_id=session_id,
        domain_id=domain_id,
        emit=emit,
        policy_decision=policy.check_tool,
        memory=memory,
    )
    if schema_failures:
        for failure in schema_failures:
            log.warning("tool spec dropped for agent %s: %s", agent_name, failure)
        emit(
            {
                "kind": "tool_spec_invalid",
                "session_id": session_id,
                "domain_id": domain_id,
                "agent_id": agent_name,
                "payload": {"failures": schema_failures},
            }
        )

    secrets = _read_secrets(config)
    tool_ctx_factory = _make_tool_context_factory(
        session_id=session_id,
        domain_id=domain_id,
        agent_id=agent_name,
        secrets=secrets,
        emit=emit,
        sandbox=_sandbox,
    )

    for turn in range(1, max_turns + 1):
        if stop_event.is_set():
            stop_reason = "cancelled"
            break

        # W6 budget check at the start of each turn.
        budget_decision = policy.check_budget()
        if not budget_decision.allow:
            emit(
                {
                    "kind": "session_end",
                    "session_id": session_id,
                    "domain_id": domain_id,
                    "state": "failed",
                    "payload": {"reason": budget_decision.reason, "turn": turn},
                }
            )
            return

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
        _accumulate_cost(cost, cost_totals)

        if cost is not None:
            policy.add_cost(cost.cost_usd)
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

        # Persist messages after the assistant response so a restart can resume.
        store.set(session_id, "messages", messages)

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

        # Tool calls: emit terminal text (may be empty) and each tool_call.
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

        # Call each tool. API-agent tool_calls go through the ToolRegistry
        # with policy gating; CLI-agent tool_calls are delegated.
        for tc in tool_calls:
            ctx = tool_ctx_factory(turn=turn, tool_call_id=tc.id)
            if _is_cli_adapter(adapter):
                tool_decision = policy.check_tool(tc.name, ctx)
                result = _delegate_cli_tool_result(tc, decision=tool_decision)
            else:
                result = registry.call(tc.name, ctx, dict(tc.input))
            payload = {
                "tool_call_id": tc.id,
                "name": tc.name,
                "output": result.to_observation_payload(),
                "ok": result.ok,
                "turn": turn,
            }
            emit(
                {
                    "kind": "tool_result",
                    "session_id": session_id,
                    "domain_id": domain_id,
                    "agent_id": agent_name,
                    "payload": payload,
                }
            )
            serialized = result.output if result.ok else {"error": result.error}
            messages = _append_tool_result(
                messages, tc, serialized, adapter_kind=adapter.name()
            )

        # Persist messages after tool results so the next turn resumes cleanly.
        store.set(session_id, "messages", messages)

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
    return final_text


def _run_workflow(
    spec: dict,
    trigger: dict,
    config: dict,
    *,
    stop_event: threading.Event,
    emit: EmitFn,
    cost_totals: dict[str, Any],
) -> str:
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
    final_text = ""

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
        final_text = text
        _accumulate_cost(cost, cost_totals)
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
            return final_text
        if next_id not in steps_by_id:
            raise KeyError(f"step {step['id']!r} next {next_id!r} not in steps")
        step = steps_by_id[next_id]

    return final_text


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


# ---------------------------------------------------------------------------
# Tool registry / context wiring
# ---------------------------------------------------------------------------


def _build_tool_registry(
    *,
    spec: dict[str, Any],
    agent: dict,
    session_id: str,
    domain_id: str,
    emit: EmitFn,
    policy_decision: Any = None,
    memory: MemoryStore | None = None,
) -> tuple[ToolRegistry, list[str]]:
    """Build a ToolRegistry from ``agent.tools``.

    Returns ``(registry, failures)`` where ``failures`` is a list of
    human-readable error strings for entries that couldn't be built.
    The executor emits a ``tool_spec_invalid`` observation so the user
    can see what was dropped.
    """
    raw_tools = agent.get("tools") or []
    mcp_server_map = build_mcp_server_map(spec)

    # Discover remote tool schemas for any referenced MCP servers so the
    # LLM-facing schema is accurate and we don't reconnect per tool.
    mcp_schemas: dict[str, dict[str, Any]] = {}
    failures: list[str] = []
    referenced_servers: set[str] = set()
    for entry in raw_tools:
        if not isinstance(entry, dict):
            continue
        if entry.get("type") != "mcp_client":
            continue
        cfg = entry.get("config") or {}
        server_name = str(cfg.get("server", ""))
        if server_name and server_name in mcp_server_map:
            referenced_servers.add(server_name)

    for server_name in sorted(referenced_servers):
        try:
            schemas = discover_mcp_tools(mcp_server_map[server_name])
        except Exception as e:  # noqa: BLE001
            failures.append(f"mcp server {server_name!r} discovery failed: {e!s}")
            continue
        for remote_name, schema in schemas.items():
            mcp_schemas[f"{server_name}::{remote_name}"] = schema

    registry = ToolRegistry(policy_decision=policy_decision)
    for entry in raw_tools:
        if not isinstance(entry, dict):
            failures.append(f"non-dict tool entry: {entry!r}")
            continue
        # Pass through name-only references unchanged (CLI-agent / W4).
        if "type" not in entry:
            continue
        try:
            tool = build_tool(
                entry,
                mcp_servers=mcp_server_map,
                mcp_schemas=mcp_schemas,
            )
        except ValueError as e:
            failures.append(str(e))
            continue
        registry.register(tool)

    # W11: auto-inject memory tools when a memory backend is configured.
    if memory is not None:
        for tool in (
            MemoryStoreTool("memory_store", memory),
            MemorySearchTool("memory_search", memory),
            MemoryForgetTool("memory_forget", memory),
        ):
            if tool.name not in registry.names():
                registry.register(tool)

    # Inject the LLM-facing tool schemas so adapters see the right
    # definitions when they call the provider API.
    schemas = tool_schemas_for_adapter(
        {"tools": raw_tools},
        mcp_servers=mcp_server_map,
        mcp_schemas=mcp_schemas,
    )
    # W11: include auto-injected memory tools in the adapter schema list.
    if memory is not None:
        for mem_tool in (
            MemoryStoreTool("memory_store", memory),
            MemorySearchTool("memory_search", memory),
            MemoryForgetTool("memory_forget", memory),
        ):
            if mem_tool.name not in {s["name"] for s in schemas}:
                schemas.append(
                    {
                        "name": mem_tool.name,
                        "description": {
                            "memory_store": "Save a fact or preference to long-term memory.",
                            "memory_search": "Search long-term memory for relevant context.",
                            "memory_forget": "Delete a memory by key.",
                        }[mem_tool.name],
                        "input_schema": mem_tool.schema,
                    }
                )
    agent["tools"] = schemas
    return registry, failures


def _read_secrets(config: dict) -> dict[str, str]:
    """Pull resolved secrets from the session config.

    The Control Plane resolves ``{secret.X}`` placeholders in the
    DomainSpec at submission time and forwards the values here. We
    accept either a flat dict or a structured list-of-pairs shape so
    older call sites can stay compatible.
    """
    raw = config.get("secrets") or {}
    if isinstance(raw, dict):
        return {str(k): str(v) for k, v in raw.items()}
    if isinstance(raw, list):
        out: dict[str, str] = {}
        for item in raw:
            if isinstance(item, dict) and "name" in item:
                out[str(item["name"])] = str(item.get("value", ""))
        return out
    return {}


def _make_tool_context_factory(
    *,
    session_id: str,
    domain_id: str,
    agent_id: str,
    secrets: dict[str, str],
    emit: EmitFn,
    sandbox: Any = None,
) -> Callable[..., ToolContext]:
    """Return a callable that builds a per-call ToolContext.

    The factory closes over session-scoped fields (session_id / domain_id
    / agent_id / secrets / emit / sandbox) and accepts per-call fields (turn /
    tool_call_id). Using a factory keeps the per-turn loop body tidy.
    """
    def _factory(*, turn: int, tool_call_id: str) -> ToolContext:
        return ToolContext(
            session_id=session_id,
            domain_id=domain_id,
            agent_id=agent_id,
            turn=turn,
            tool_call_id=tool_call_id,
            secrets=dict(secrets),
            emit=emit,
            sandbox=sandbox,
        )

    return _factory


def _allow_all_policy_hook() -> Callable[[str, dict, ToolContext], ToolDecision]:
    """Default policy hook: allow everything.

    W6 will replace this with a real :class:`PolicyEngine`. The hook
    still runs (and the registry emits ``policy_check`` once W6 wires
    that), so existing audit infrastructure keeps working.
    """
    def _hook(name: str, input: dict, ctx: ToolContext) -> ToolDecision:
        return ToolDecision(allow=True, decision="allow", reason="w3-stub")

    return _hook


# ---------------------------------------------------------------------------
# CLI-agent helpers
# ---------------------------------------------------------------------------


_CLI_ADAPTER_KINDS = frozenset({"claudecode", "codex"})


def _is_cli_adapter(adapter: Any) -> bool:
    """Return True if the adapter delegates to an external CLI agent."""
    return adapter.name() in _CLI_ADAPTER_KINDS


def _delegate_cli_tool_result(
    tool_call: Any,
    *,
    decision: ToolDecision,
) -> ToolResult:
    """Synthesize a ToolResult for a tool that the CLI agent already executed.

    CLI agents (Claude Code / Codex) bring their own shell/file/edit
    primitives. The Harness does not re-execute them; it only audits the
    operation. The synthesized result lets the multi-turn loop continue.
    """
    if not decision.allow:
        return ToolResult(
            error=f"policy denied: {decision.reason or decision.decision}",
            metadata={
                "delegated_to_cli": True,
                "observed_only": True,
                "policy_decision": decision.decision,
            },
        )
    return ToolResult(
        output={
            "delegated_to_cli": True,
            "raw": tool_call.raw_input or tool_call.input,
            "message": "operation executed by CLI agent (audited only)",
        },
        metadata={
            "delegated_to_cli": True,
            "observed_only": True,
        },
    )


# ---------------------------------------------------------------------------
# helpers
# ---------------------------------------------------------------------------


def _read_mode(spec: dict) -> str:
    return (spec.get("harness", {}).get("session", {}) or {}).get("mode", "")


def _read_strategy(spec: dict) -> str:
    """Read ``harness.orchestration.strategy`` (defaults to ``direct``)."""
    return (
        (spec.get("harness", {}) or {})
        .get("orchestration", {})
        .get("strategy", "direct")
        or "direct"
    )


def _run_strategy_agent(
    spec: dict,
    trigger: dict,
    config: dict,
    *,
    stop_event: threading.Event,
    emit: EmitFn,
    cost_totals: dict[str, Any],
    memory: MemoryStore | None,
    builder: str,
) -> str:
    """Dispatch to the W12 strategy builder and accumulate cost."""
    from hnsx_worker.agents import (
        build_multi_agent_runner,
        build_plan_and_solve_agent,
        build_react_agent,
    )

    if builder == "react":
        agent = build_react_agent(
            spec=spec, config=config, emit=emit, memory=memory
        )
    elif builder == "plan_and_solve":
        agent = build_plan_and_solve_agent(
            spec=spec, config=config, emit=emit, memory=memory
        )
    elif builder == "multi_agent":
        agent = build_multi_agent_runner(
            spec=spec, config=config, emit=emit, memory=memory
        )
    else:
        raise ValueError(f"unknown W12 strategy: {builder!r}")

    text = agent.run(trigger, stop_event=stop_event)
    # Cost attribution for strategy runs that go through run_multi_turn_loop
    # already accumulated via cost_totals; nothing more to do here.
    return text


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


def _store_session_summary(
    memory: MemoryStore,
    session_id: str,
    spec: dict[str, Any],
    trigger: dict[str, Any],
    output: str,
    *,
    emit: EmitFn,
) -> None:
    """Persist a lightweight session summary into long-term memory.

    W11 uses a deterministic summary (trigger keys + final output). A future
    phase can swap this for an LLM-generated summary while keeping the same
    MemoryStore contract.
    """
    from hnsx_worker.memory import MemoryItem

    summary = {
        "domain_id": spec.get("id", ""),
        "trigger_keys": sorted(trigger.keys()) if isinstance(trigger, dict) else [],
        "output_preview": output[:1000],
        "stored_at": int(time.time() * 1000),
    }
    item = MemoryItem(
        session_id=session_id,
        kind="summary",
        content=summary,
    )
    memory.add(item)
    emit(
        {
            "kind": "memory_write",
            "session_id": session_id,
            "domain_id": spec.get("id", ""),
            "agent_id": "",
            "payload": {
                "operation": "session_summary",
                "id": item.id,
                "kind": item.kind,
            },
        }
    )


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


# ---------------------------------------------------------------------------
# W12 public API — reusable multi-turn loop for advanced agent strategies
# ---------------------------------------------------------------------------


@dataclass
class AgentLoopContext:
    """Everything an advanced agent (W12) needs to drive the multi-turn loop.

    The executor builds this once per session, then hands it to whichever
    :class:`Agent` strategy runs. Strategies can decorate the loop with
    their own pre/post hooks without re-implementing the tool dispatch.
    """

    spec: dict[str, Any]
    agent_cfg: dict[str, Any]
    adapter: Any
    prompt: str
    registry: ToolRegistry
    policy: PolicyEngine
    store: Any
    sandbox: Any
    messages: list[dict]
    secrets: dict[str, str]
    session_id: str
    domain_id: str
    agent_id: str
    max_turns: int


HookFn = Callable[[int, "AgentLoopTurnInfo"], None]


@dataclass
class AgentLoopTurnInfo:
    """Snapshot passed to strategy hooks after each turn."""

    turn: int
    final_text: str
    tool_calls: list[Any]
    messages: list[dict]
    cost: Any


def build_agent_loop_context(
    *,
    spec: dict[str, Any],
    agent_cfg: dict[str, Any],
    config: dict[str, Any],
    memory: MemoryStore | None = None,
    extra_tools: list[Any] | None = None,
) -> AgentLoopContext:
    """Build an :class:`AgentLoopContext` for the given agent.

    Reused by:

      - the built-in ``multi-turn`` mode (via :func:`_run_multi_turn`)
      - :class:`ReActAgent` / :class:`PlanAndSolveAgent` / :class:`MultiAgentRunner`

    Pass ``extra_tools`` to inject additional tools (e.g. ``delegate_to``)
    on top of the agent's declared ``tools:`` entries.
    """
    harness = spec.get("harness", {})
    session_id = config.get("session_id", "")
    domain_id = spec.get("id", "")
    agent_id = (
        (harness.get("session", {}) or {}).get("agent")
        or next(iter(harness.get("agents", {}) or {}), "")
    )
    if not agent_id:
        raise ValueError("build_agent_loop_context requires a non-empty agents map")
    prompt = _resolve_prompt(spec, agent_cfg)
    adapter = AdapterRegistry.get(agent_cfg.get("adapter", {}).get("kind", "noop"))
    max_turns = int(
        (harness.get("policy", {}) or {}).get("budget", {}).get("max_turns")
        or (harness.get("session", {}) or {}).get("max_turns")
        or _DEFAULT_MAX_TURNS
    )

    policy = PolicyEngine(
        spec, session_id=session_id, domain_id=domain_id, agent_id=agent_id, emit=_noop_emit
    )
    sandbox = build_sandbox_backend(harness.get("sandbox"))
    store = build_store_backend(harness.get("store"))

    # Re-use _build_tool_registry so the agent sees the same schemas as the
    # built-in multi-turn mode. We pass through memory so memory_* tools are
    # auto-injected when configured.
    registry, _failures = _build_tool_registry(
        spec=spec,
        agent=agent_cfg,
        session_id=session_id,
        domain_id=domain_id,
        emit=_noop_emit,
        policy_decision=policy.check_tool,
        memory=memory,
    )
    if extra_tools:
        for tool in extra_tools:
            if tool.name in registry.names():
                continue
            registry.register(tool)

    return AgentLoopContext(
        spec=spec,
        agent_cfg=agent_cfg,
        adapter=adapter,
        prompt=prompt,
        registry=registry,
        policy=policy,
        store=store,
        sandbox=sandbox,
        messages=[],
        secrets=_read_secrets(config),
        session_id=session_id,
        domain_id=domain_id,
        agent_id=agent_id,
        max_turns=max_turns,
    )


def run_multi_turn_loop(
    context: AgentLoopContext,
    *,
    user_input: dict[str, Any],
    stop_event: threading.Event,
    emit: EmitFn,
    cost_totals: dict[str, Any] | None = None,
    on_turn_start: HookFn | None = None,
    on_tool_result: HookFn | None = None,
    on_turn_end: HookFn | None = None,
    max_turns_override: int | None = None,
    prompt_suffix: str = "",
) -> dict[str, Any]:
    """Run the tool-using multi-turn loop.

    Strategies (ReAct / Plan-and-Solve / Multi-Agent) get:

      - the same stream-based agent invocation,
      - the same ToolRegistry + policy gating,
      - the same message-store persistence,

    while staying free to decorate the loop via the three hooks.

    Returns ``{final_text, tool_call_count, turn_count, cost}``.
    """
    if cost_totals is None:
        cost_totals = {
            "prompt_tokens": 0,
            "completion_tokens": 0,
            "cost_usd": 0.0,
            "latency_ms": 0,
        }

    max_turns = int(max_turns_override or context.max_turns)

    messages = list(context.messages)
    if not messages:
        messages = _build_initial_messages(context.prompt + prompt_suffix, user_input)

    final_text = ""
    stop_reason = "natural"
    tool_call_count = 0
    cost = None

    secrets = context.secrets
    ctx_factory = _make_tool_context_factory(
        session_id=context.session_id,
        domain_id=context.domain_id,
        agent_id=context.agent_id,
        secrets=secrets,
        emit=emit,
        sandbox=context.sandbox,
    )

    for turn in range(1, max_turns + 1):
        if stop_event.is_set():
            stop_reason = "cancelled"
            break

        budget_decision = context.policy.check_budget()
        if not budget_decision.allow:
            emit(
                {
                    "kind": "session_end",
                    "session_id": context.session_id,
                    "domain_id": context.domain_id,
                    "state": "failed",
                    "payload": {"reason": budget_decision.reason, "turn": turn},
                }
            )
            return {
                "final_text": "",
                "tool_call_count": tool_call_count,
                "turn_count": turn,
                "cost": cost,
                "stop_reason": "budget_exceeded",
            }

        emit(
            {
                "kind": "turn_start",
                "session_id": context.session_id,
                "domain_id": context.domain_id,
                "agent_id": context.agent_id,
                "payload": {"turn": turn, "adapter": context.adapter.name(), "max_turns": max_turns},
            }
        )

        text, tool_calls, cost = _stream_turn(
            context.adapter,
            context.agent_cfg,
            context.prompt + prompt_suffix,
            user_input,
            messages=messages,
            session_id=context.session_id,
            domain_id=context.domain_id,
            agent_id=context.agent_id,
            stop_event=stop_event,
            emit=emit,
            turn=turn,
        )
        final_text = text
        _accumulate_cost(cost, cost_totals)
        if cost is not None:
            context.policy.add_cost(cost.cost_usd)

        context.store.set(context.session_id, "messages", messages)

        info = AgentLoopTurnInfo(
            turn=turn,
            final_text=text,
            tool_calls=list(tool_calls),
            messages=messages,
            cost=cost,
        )

        if not tool_calls:
            emit(
                {
                    "kind": "agent_text",
                    "session_id": context.session_id,
                    "domain_id": context.domain_id,
                    "agent_id": context.agent_id,
                    "payload": {"content": text, "final": True, "turn": turn},
                }
            )
            emit(
                {
                    "kind": "turn_end",
                    "session_id": context.session_id,
                    "domain_id": context.domain_id,
                    "agent_id": context.agent_id,
                    "payload": {"turn": turn, "stop_reason": "natural"},
                }
            )
            if on_turn_end is not None:
                on_turn_end(turn, info)
            stop_reason = "natural"
            break

        emit(
            {
                "kind": "agent_text",
                "session_id": context.session_id,
                "domain_id": context.domain_id,
                "agent_id": context.agent_id,
                "payload": {"content": text, "final": False, "turn": turn},
            }
        )
        for tc in tool_calls:
            ctx = ctx_factory(turn=turn, tool_call_id=tc.id)
            if _is_cli_adapter(context.adapter):
                tool_decision = context.policy.check_tool(tc.name, ctx)
                result = _delegate_cli_tool_result(tc, decision=tool_decision)
            else:
                result = context.registry.call(tc.name, ctx, dict(tc.input))
            tool_call_count += 1
            payload = {
                "tool_call_id": tc.id,
                "name": tc.name,
                "output": result.to_observation_payload(),
                "ok": result.ok,
                "turn": turn,
            }
            emit(
                {
                    "kind": "tool_result",
                    "session_id": context.session_id,
                    "domain_id": context.domain_id,
                    "agent_id": context.agent_id,
                    "payload": payload,
                }
            )
            serialized = result.output if result.ok else {"error": result.error}
            messages = _append_tool_result(
                messages, tc, serialized, adapter_kind=context.adapter.name()
            )
            if on_tool_result is not None:
                on_tool_result(turn, AgentLoopTurnInfo(
                    turn=turn,
                    final_text=text,
                    tool_calls=[tc],
                    messages=messages,
                    cost=cost,
                ))

        context.store.set(context.session_id, "messages", messages)

        emit(
            {
                "kind": "turn_end",
                "session_id": context.session_id,
                "domain_id": context.domain_id,
                "agent_id": context.agent_id,
                "payload": {"turn": turn, "stop_reason": "tool_call"},
            }
        )
        if on_turn_end is not None:
            on_turn_end(turn, info)
        if on_turn_start is not None:
            # Defer to next iteration.
            pass

        if turn >= max_turns:
            stop_reason = "max_turns"
            break
    else:
        stop_reason = "max_turns"

    if stop_reason == "max_turns":
        emit(
            {
                "kind": "agent_text",
                "session_id": context.session_id,
                "domain_id": context.domain_id,
                "agent_id": context.agent_id,
                "payload": {"content": final_text, "final": True, "truncated": True},
            }
        )

    return {
        "final_text": final_text,
        "tool_call_count": tool_call_count,
        "turn_count": min(turn, max_turns) if "turn" in locals() else 0,
        "cost": cost,
        "stop_reason": stop_reason,
    }


def _noop_emit(_obs: dict) -> None:  # pragma: no cover - debug helper
    """Default ``emit`` used during context build (no observation sink yet)."""
