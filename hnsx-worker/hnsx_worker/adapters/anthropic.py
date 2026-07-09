"""Anthropic Messages API adapter.

Calls https://docs.anthropic.com/en/api/messages via httpx. The API key is
read from the environment variable named in ``agent.api_key_env`` (per
the DomainSpec convention). A custom ``agent.adapter.endpoint`` overrides
the default base URL for testing or self-hosted Anthropic-compatible
deployments.

Streaming support:
  - ``invoke`` returns a single :class:`AdapterResult` after the full response
    has been received.
  - ``invoke_stream`` yields :class:`StreamChunk` objects as SSE events arrive.

Tool use:
  - ``agent.tools`` may contain a list of tool definitions; these are passed as
    the ``tools`` parameter on the Messages API.
  - Response ``tool_use`` content blocks are returned as :class:`ToolCall`.
"""

from __future__ import annotations

import json
import logging
import os
from collections.abc import Iterator
from typing import Any

import httpx

from hnsx_worker.adapters.base import Adapter, AdapterResult, Cost, StreamChunk, ToolCall

log = logging.getLogger("hnsx_worker.adapters.anthropic")

_DEFAULT_BASE_URL = "https://api.anthropic.com"
_API_VERSION = "2023-06-01"
_DEFAULT_MODEL = "claude-haiku-4-5"
_DEFAULT_MAX_TOKENS = 1024


class AnthropicAdapter(Adapter):
    """Adapter for the Anthropic Messages API."""

    def name(self) -> str:
        return "anthropic"

    def invoke(self, agent: dict, prompt: str, input: dict) -> AdapterResult:
        api_key = _resolve_api_key(agent)
        if not api_key:
            env_name = agent.get("api_key_env", "ANTHROPIC_API_KEY")
            raise RuntimeError(f"anthropic adapter: API key env var {env_name!r} is not set")

        base_url = (agent.get("adapter", {}) or {}).get("endpoint") or _DEFAULT_BASE_URL
        timeout = (agent.get("adapter", {}) or {}).get("timeout_seconds", 60)
        model = agent.get("model") or _DEFAULT_MODEL
        max_tokens = int((agent.get("adapter", {}) or {}).get("max_tokens", _DEFAULT_MAX_TOKENS))
        tools = _build_tools(agent)

        headers = {
            "x-api-key": api_key,
            "anthropic-version": _API_VERSION,
            "content-type": "application/json",
        }
        body: dict[str, Any] = {
            "model": model,
            "max_tokens": max_tokens,
            "system": prompt,
            "messages": _build_messages(input),
        }
        if tools:
            body["tools"] = tools

        with httpx.Client(base_url=base_url, timeout=timeout) as client:
            resp = client.post("/v1/messages", headers=headers, json=body)
            resp.raise_for_status()
            data = resp.json()

        text, tool_calls = _extract_content(data)
        usage = data.get("usage", {}) or {}
        cost = Cost(
            prompt_tokens=int(usage.get("input_tokens", 0) or 0),
            completion_tokens=int(usage.get("output_tokens", 0) or 0),
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
            env_name = agent.get("api_key_env", "ANTHROPIC_API_KEY")
            raise RuntimeError(f"anthropic adapter: API key env var {env_name!r} is not set")

        base_url = (agent.get("adapter", {}) or {}).get("endpoint") or _DEFAULT_BASE_URL
        timeout = (agent.get("adapter", {}) or {}).get("timeout_seconds", 60)
        model = agent.get("model") or _DEFAULT_MODEL
        max_tokens = int((agent.get("adapter", {}) or {}).get("max_tokens", _DEFAULT_MAX_TOKENS))
        tools = _build_tools(agent)

        headers = {
            "x-api-key": api_key,
            "anthropic-version": _API_VERSION,
            "content-type": "application/json",
            "accept": "text/event-stream",
        }
        body: dict[str, Any] = {
            "model": model,
            "max_tokens": max_tokens,
            "system": prompt,
            "messages": _build_messages(input),
            "stream": True,
        }
        if tools:
            body["tools"] = tools

        text_buffer = ""
        tool_buffers: dict[str, _ToolBuffer] = {}
        usage: dict[str, int] = {}

        with httpx.Client(base_url=base_url, timeout=timeout) as client:
            with client.stream(
                "POST", "/v1/messages", headers=headers, json=body
            ) as stream:
                stream.raise_for_status()
                for line in stream.iter_lines():
                    if not line.startswith("data: "):
                        continue
                    payload = line[6:]
                    if payload == "[DONE]":
                        break
                    try:
                        event = json.loads(payload)
                    except json.JSONDecodeError:
                        continue

                    kind = event.get("type")
                    if kind == "content_block_start":
                        block = event.get("content_block") or {}
                        if block.get("type") == "tool_use":
                            tid = block.get("id", "")
                            tool_buffers[tid] = _ToolBuffer(
                                id=tid,
                                name=block.get("name", ""),
                                input_json="",
                            )
                    elif kind == "content_block_delta":
                        delta = event.get("delta") or {}
                        if "text" in delta:
                            delta_text = delta["text"] or ""
                            text_buffer += delta_text
                            yield StreamChunk(text_delta=delta_text)
                        elif delta.get("type") == "input_json_delta":
                            idx = event.get("index", "")
                            # Map index to the most recent tool buffer if id is absent.
                            buf = _find_buffer_by_index(tool_buffers, idx)
                            if buf is not None:
                                partial = delta.get("partial_json", "")
                                buf.input_json += partial
                    elif kind == "content_block_stop":
                        idx = event.get("index", "")
                        buf = _find_buffer_by_index(tool_buffers, idx)
                        if buf is not None:
                            tool_input = _parse_tool_input(buf.input_json)
                            yield StreamChunk(
                                tool_call=ToolCall(
                                    id=buf.id,
                                    name=buf.name,
                                    input=tool_input,
                                    raw_input=buf.input_json,
                                )
                            )
                            tool_buffers.pop(buf.id, None)
                    elif kind == "message_delta":
                        _add_usage(usage, event.get("usage") or {})
                    elif kind == "message_start":
                        msg_usage = (event.get("message") or {}).get("usage") or {}
                        _add_usage(usage, msg_usage)

        cost = Cost(
            prompt_tokens=int(usage.get("input_tokens", 0) or 0),
            completion_tokens=int(usage.get("output_tokens", 0) or 0),
        )
        yield StreamChunk(text_delta="", cost=cost)


def _parse_tool_input(raw: str) -> dict[str, Any]:
    if not raw:
        return {}
    try:
        return json.loads(raw)
    except json.JSONDecodeError:
        return {}


def _add_usage(acc: dict[str, int], delta: dict[str, int]) -> None:
    acc["input_tokens"] = acc.get("input_tokens", 0) + delta.get("input_tokens", 0)
    acc["output_tokens"] = acc.get("output_tokens", 0) + delta.get("output_tokens", 0)


# ---------------------------------------------------------------------------
# helpers
# ---------------------------------------------------------------------------


class _ToolBuffer:
    def __init__(self, id: str, name: str, input_json: str) -> None:
        self.id = id
        self.name = name
        self.input_json = input_json


def _resolve_api_key(agent: dict) -> str:
    env_name = agent.get("api_key_env") or "ANTHROPIC_API_KEY"
    return os.environ.get(env_name, "")


def _build_messages(input: dict) -> list[dict]:
    """Extract the messages list for the API call.

    The multi-turn executor embeds the full conversation history in
    ``input["_messages"]`` as a list of ``{role, content}`` (or content blocks
    for Anthropic). We honor that when present; otherwise we fall back to a
    single-turn user message derived from ``input``.
    """
    history = input.get("_messages")
    if isinstance(history, list) and history:
        return history
    if isinstance(input, dict) and len(input) == 1 and "content" in input:
        value = input["content"]
        return [{"role": "user", "content": str(value) if not isinstance(value, str) else value}]
    return [{"role": "user", "content": json.dumps(input, ensure_ascii=False, default=str)}]


def _build_tools(agent: dict) -> list[dict[str, Any]]:
    """Convert ``agent.tools`` (list of tool names or dicts) to Anthropic tool defs."""
    raw_tools = agent.get("tools") or []
    if not raw_tools:
        return []
    out: list[dict[str, Any]] = []
    for tool in raw_tools:
        if isinstance(tool, dict):
            schema = tool.get("input_schema") or tool.get("schema") or {"type": "object"}
            out.append(
                {
                    "name": tool.get("name", ""),
                    "description": tool.get("description", ""),
                    "input_schema": schema,
                }
            )
        elif isinstance(tool, str):
            # Name-only tool reference; the runtime will resolve the schema later.
            out.append({"name": tool, "description": "", "input_schema": {"type": "object"}})
    return out


def _extract_text(data: dict) -> str:
    """Pull the text out of an Anthropic Messages response."""
    parts: list[str] = []
    for block in data.get("content", []) or []:
        if block.get("type") == "text":
            parts.append(block.get("text", ""))
    return "".join(parts)


def _extract_content(data: dict) -> tuple[str, list[ToolCall]]:
    """Extract text and tool calls from an Anthropic Messages response."""
    text_parts: list[str] = []
    tool_calls: list[ToolCall] = []
    for block in data.get("content", []) or []:
        btype = block.get("type")
        if btype == "text":
            text_parts.append(block.get("text", ""))
        elif btype == "tool_use":
            raw_input = block.get("input", "")
            if isinstance(raw_input, dict):
                tool_input = raw_input
                raw_input_str = json.dumps(raw_input, ensure_ascii=False, default=str)
            else:
                try:
                    tool_input = json.loads(raw_input) if raw_input else {}
                except json.JSONDecodeError:
                    tool_input = {}
                raw_input_str = str(raw_input)
            tool_calls.append(
                ToolCall(
                    id=block.get("id", ""),
                    name=block.get("name", ""),
                    input=tool_input,
                    raw_input=raw_input_str,
                )
            )
    return "".join(text_parts), tool_calls


def _find_buffer_by_index(buffers: dict[str, _ToolBuffer], index: Any) -> _ToolBuffer | None:
    """Best-effort mapping from SSE index to a tool buffer.

    Anthropic's SSE events reference content blocks by index. We don't always
    know the id at that point, so we return the most recently created buffer
    as a heuristic.
    """
    if not buffers:
        return None
    # If we only have one buffer, it must be the one.
    if len(buffers) == 1:
        return next(iter(buffers.values()))
    # Fallback: index is unreliable as a key, but is often 1-based after the
    # text block. Just return the first pending buffer.
    return next(iter(buffers.values()))
