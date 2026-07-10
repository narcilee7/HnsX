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
from collections.abc import Callable
from typing import Any

from hnsx_worker.adapters import AdapterRegistry
from hnsx_worker.adapters.base import Adapter
from hnsx_worker.harness import run as run_orchestration
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

log = logging.getLogger("hnsx_worker.session_executor")

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
        run_orchestration(
            spec, trigger, config, stop_event=stop_event, emit=emit
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
        agent=agent,
        session_id=session_id,
        domain_id=domain_id,
        emit=emit,
        policy_decision=policy.check_tool,
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


# ---------------------------------------------------------------------------
# Tool registry / context wiring
# ---------------------------------------------------------------------------


def _build_tool_registry(
    *,
    agent: dict,
    session_id: str,
    domain_id: str,
    emit: EmitFn,
    policy_decision: Any = None,
) -> tuple[ToolRegistry, list[str]]:
    """Build a ToolRegistry from ``agent.tools``.

    Returns ``(registry, failures)`` where ``failures`` is a list of
    human-readable error strings for entries that couldn't be built.
    The executor emits a ``tool_spec_invalid`` observation so the user
    can see what was dropped.
    """
    raw_tools = agent.get("tools") or []
    registry = ToolRegistry(policy_decision=policy_decision)
    failures: list[str] = []
    for entry in raw_tools:
        if not isinstance(entry, dict):
            failures.append(f"non-dict tool entry: {entry!r}")
            continue
        # Pass through name-only references unchanged (CLI-agent / W4).
        if "type" not in entry:
            continue
        try:
            tool = build_tool(entry)
        except ValueError as e:
            failures.append(str(e))
            continue
        registry.register(tool)
    # Inject the LLM-facing tool schemas so adapters see the right
    # definitions when they call the provider API.
    agent["tools"] = tool_schemas_for_adapter({"tools": raw_tools})
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
) -> Callable[..., ToolContext]:
    """Return a callable that builds a per-call ToolContext.

    The factory closes over session-scoped fields (session_id / domain_id
    / agent_id / secrets / emit) and accepts per-call fields (turn /
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
