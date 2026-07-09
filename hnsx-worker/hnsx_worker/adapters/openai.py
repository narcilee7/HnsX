"""OpenAI Chat Completions API adapter.

Calls https://api.openai.com/v1/chat/completions via httpx. The API key is
read from the environment variable named in ``agent.api_key_env``. A custom
``agent.adapter.endpoint`` overrides the default base URL for testing,
self-hosted OpenAI-compatible servers (vLLM, llama.cpp, etc.), or Azure-style
frontends.

Streaming support:
  - ``invoke`` returns a single :class:`AdapterResult` after the full response
    has been received.
  - ``invoke_stream`` yields :class:`StreamChunk` objects as SSE events arrive.
    The SSE wire format is ``data: <json>\n\n`` terminated by ``data: [DONE]``.

Tool use:
  - ``agent.tools`` may contain a list of tool definitions; these are passed as
    the ``tools`` parameter on the Chat Completions API. In streaming mode
    ``delta.tool_calls`` arrives incrementally (name first, then arguments).
  - Response ``tool_calls`` content blocks are returned as :class:`ToolCall`.
"""

from __future__ import annotations

import json
import logging
import os
import time
from collections.abc import Iterator
from typing import Any

import httpx

from hnsx_worker.adapters.base import (
    Adapter,
    AdapterResult,
    Cost,
    StreamChunk,
    ToolCall,
)

log = logging.getLogger("hnsx_worker.adapters.openai")

_DEFAULT_BASE_URL = "https://api.openai.com"
_DEFAULT_MODEL = "gpt-4o-mini"
_DEFAULT_MAX_TOKENS = 1024


class OpenAIAdapter(Adapter):
    """Adapter for the OpenAI Chat Completions API."""

    def name(self) -> str:
        return "openai"

    def invoke(self, agent: dict, prompt: str, input: dict) -> AdapterResult:
        api_key = _resolve_api_key(agent)
        if not api_key:
            raise RuntimeError(
                f"openai adapter: API key env var {agent.get('api_key_env', 'OPENAI_API_KEY')!r} "
                "is not set"
            )

        base_url = (agent.get("adapter", {}) or {}).get("endpoint") or _DEFAULT_BASE_URL
        timeout = (agent.get("adapter", {}) or {}).get("timeout_seconds", 60)
        model = agent.get("model") or _DEFAULT_MODEL
        max_tokens = int((agent.get("adapter", {}) or {}).get("max_tokens", _DEFAULT_MAX_TOKENS))
        tools = _build_tools(agent)

        headers = {
            "authorization": f"Bearer {api_key}",
            "content-type": "application/json",
        }
        body = {
            "model": model,
            "max_tokens": max_tokens,
            "messages": _build_messages(input),
        }
        if prompt and not any(m.get("role") == "system" for m in body["messages"]):
            body["messages"].insert(0, {"role": "system", "content": prompt})
        if tools:
            body["tools"] = tools
            body["tool_choice"] = (agent.get("adapter", {}) or {}).get("tool_choice", "auto")

        start = time.monotonic()
        with httpx.Client(base_url=base_url, timeout=timeout) as client:
            resp = client.post("/v1/chat/completions", headers=headers, json=body)
            resp.raise_for_status()
            data = resp.json()
        latency_ms = int((time.monotonic() - start) * 1000)

        text, tool_calls = _extract_content(data)
        usage = data.get("usage", {}) or {}
        cost = Cost(
            prompt_tokens=int(usage.get("prompt_tokens", 0) or 0),
            completion_tokens=int(usage.get("completion_tokens", 0) or 0),
            latency_ms=latency_ms,
        )
        return AdapterResult(text=text, tool_calls=tool_calls, cost=cost)

    def invoke_stream(
        self,
        agent: dict,
        prompt: str,
        input: dict,
    ) -> Iterator[StreamChunk]:
        api_key = _resolve_api_key(agent)
        if not api_key:
            raise RuntimeError(
                f"openai adapter: API key env var {agent.get('api_key_env', 'OPENAI_API_KEY')!r} "
                "is not set"
            )

        base_url = (agent.get("adapter", {}) or {}).get("endpoint") or _DEFAULT_BASE_URL
        timeout = (agent.get("adapter", {}) or {}).get("timeout_seconds", 60)
        model = agent.get("model") or _DEFAULT_MODEL
        max_tokens = int((agent.get("adapter", {}) or {}).get("max_tokens", _DEFAULT_MAX_TOKENS))
        tools = _build_tools(agent)

        headers = {
            "authorization": f"Bearer {api_key}",
            "content-type": "application/json",
            "accept": "text/event-stream",
        }
        body = {
            "model": model,
            "max_tokens": max_tokens,
            "stream": True,
            "stream_options": {"include_usage": True},
            "messages": _build_messages(input),
        }
        if prompt and not any(m.get("role") == "system" for m in body["messages"]):
            body["messages"].insert(0, {"role": "system", "content": prompt})
        if tools:
            body["tools"] = tools
            body["tool_choice"] = (agent.get("adapter", {}) or {}).get("tool_choice", "auto")

        start = time.monotonic()
        prompt_tokens = 0
        completion_tokens = 0
        text_buffer = ""
        # tool index -> buffer of partials
        tool_buffers: dict[int, _ToolBuffer] = {}

        with httpx.Client(base_url=base_url, timeout=timeout) as client:
            with client.stream(
                "POST", "/v1/chat/completions", headers=headers, json=body
            ) as stream:
                stream.raise_for_status()
                for line in stream.iter_lines():
                    if not line:
                        continue
                    if line.startswith("data:"):
                        payload = line[len("data:"):].strip()
                    else:
                        continue
                    if payload == "[DONE]":
                        break
                    try:
                        event = json.loads(payload)
                    except json.JSONDecodeError:
                        continue

                    # Usage chunk (when include_usage is set).
                    usage = event.get("usage")
                    if usage:
                        prompt_tokens = int(usage.get("prompt_tokens", 0) or 0)
                        completion_tokens = int(usage.get("completion_tokens", 0) or 0)

                    for choice in event.get("choices", []) or []:
                        delta = choice.get("delta") or {}
                        # Text delta.
                        delta_text = delta.get("content")
                        if delta_text:
                            text_buffer += delta_text
                            yield StreamChunk(text_delta=delta_text)

                        # Tool call deltas.
                        for tc_delta in delta.get("tool_calls") or []:
                            idx = int(tc_delta.get("index", 0) or 0)
                            buf = tool_buffers.setdefault(
                                idx, _ToolBuffer(id="", name="", args_json="")
                            )
                            if tc_delta.get("id"):
                                buf.id = tc_delta["id"]
                            func = tc_delta.get("function") or {}
                            if func.get("name"):
                                buf.name = func["name"]
                            if "arguments" in func:
                                buf.args_json += func["arguments"] or ""
                            # Finish reason may ride along with the last delta.
                            if choice.get("finish_reason"):
                                buf.finish_reason = choice["finish_reason"]

        # Flush any pending tool buffers as completed ToolCalls.
        for buf in sorted(tool_buffers.values(), key=lambda b: b.id):
            if not buf.id:
                # OpenAI always assigns an id; skip incomplete buffers defensively.
                continue
            try:
                parsed = json.loads(buf.args_json) if buf.args_json else {}
            except json.JSONDecodeError:
                parsed = {}
            yield StreamChunk(
                tool_call=ToolCall(
                    id=buf.id,
                    name=buf.name,
                    input=parsed,
                    raw_input=buf.args_json,
                )
            )

        latency_ms = int((time.monotonic() - start) * 1000)
        yield StreamChunk(
            text_delta="",
            cost=Cost(
                prompt_tokens=prompt_tokens,
                completion_tokens=completion_tokens,
                latency_ms=latency_ms,
            ),
        )


# ---------------------------------------------------------------------------
# helpers
# ---------------------------------------------------------------------------


class _ToolBuffer:
    __slots__ = ("id", "name", "args_json", "finish_reason")

    def __init__(self, id: str, name: str, args_json: str) -> None:
        self.id = id
        self.name = name
        self.args_json = args_json
        self.finish_reason: str | None = None


def _resolve_api_key(agent: dict) -> str:
    env_name = agent.get("api_key_env") or "OPENAI_API_KEY"
    return os.environ.get(env_name, "")


def _build_messages(input: dict) -> list[dict]:
    """Extract the messages list for the API call.

    The multi-turn executor embeds the full conversation history in
    ``input["_messages"]``. We honor that when present; otherwise we fall
    back to a system + user pair derived from ``prompt`` + ``input``.
    """
    history = input.get("_messages")
    if isinstance(history, list) and history:
        return history
    if isinstance(input, dict) and len(input) == 1 and "content" in input:
        value = input["content"]
        return [{"role": "user", "content": str(value) if not isinstance(value, str) else value}]
    return [{"role": "user", "content": json.dumps(input, ensure_ascii=False, default=str)}]


def _build_tools(agent: dict) -> list[dict[str, Any]]:
    """Convert ``agent.tools`` (list of tool names or dicts) to OpenAI tool defs.

    OpenAI's tool schema uses ``parameters`` instead of Anthropic's
    ``input_schema``. We accept either field name for ergonomics.
    """
    raw_tools = agent.get("tools") or []
    if not raw_tools:
        return []
    out: list[dict[str, Any]] = []
    for tool in raw_tools:
        if isinstance(tool, dict):
            params = tool.get("parameters") or tool.get("input_schema") or {"type": "object"}
            out.append(
                {
                    "type": "function",
                    "function": {
                        "name": tool.get("name", ""),
                        "description": tool.get("description", ""),
                        "parameters": params,
                    },
                }
            )
        elif isinstance(tool, str):
            out.append(
                {
                    "type": "function",
                    "function": {
                        "name": tool,
                        "description": "",
                        "parameters": {"type": "object"},
                    },
                }
            )
    return out


def _extract_text(data: dict) -> str:
    """Pull the text out of a Chat Completions response."""
    parts: list[str] = []
    for choice in data.get("choices", []) or []:
        msg = choice.get("message") or {}
        parts.append(msg.get("content", "") or "")
    return "".join(parts)


def _extract_content(data: dict) -> tuple[str, list[ToolCall]]:
    """Extract text and tool calls from a Chat Completions response."""
    text_parts: list[str] = []
    tool_calls: list[ToolCall] = []
    for choice in data.get("choices", []) or []:
        msg = choice.get("message") or {}
        text_parts.append(msg.get("content", "") or "")
        for tc in msg.get("tool_calls") or []:
            raw_args = tc.get("function", {}).get("arguments", "")
            if isinstance(raw_args, dict):
                parsed = raw_args
                raw_args_str = json.dumps(raw_args, ensure_ascii=False, default=str)
            elif isinstance(raw_args, str):
                try:
                    parsed = json.loads(raw_args) if raw_args else {}
                except json.JSONDecodeError:
                    parsed = {}
                raw_args_str = raw_args
            else:
                parsed = {}
                raw_args_str = ""
            tool_calls.append(
                ToolCall(
                    id=tc.get("id", ""),
                    name=tc.get("function", {}).get("name", ""),
                    input=parsed,
                    raw_input=raw_args_str,
                )
            )
    return "".join(text_parts), tool_calls