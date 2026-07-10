"""Tests for OpenAI adapter streaming + tool use + multi-turn history.

Builds on ``test_real_adapters.py`` (which covers single-shot invocation).
Here we focus on:

  - Streaming text deltas
  - Streaming tool call assembly
  - Tool use in the non-streaming ``invoke`` path
  - Honoring ``input._messages`` for multi-turn
"""

from __future__ import annotations

import json
from typing import Any

import httpx
import pytest

from hnsx_worker.adapters.openai import OpenAIAdapter


# ---------------------------------------------------------------------------
# helpers
# ---------------------------------------------------------------------------


class _ClientRecorder:
    def __init__(self, response_payload: dict, status_code: int = 200) -> None:
        self.response_payload = response_payload
        self.status_code = status_code
        self.requests: list[httpx.Request] = []

    def __call__(self, request: httpx.Request) -> httpx.Response:
        self.requests.append(request)
        return httpx.Response(self.status_code, json=self.response_payload)


@pytest.fixture
def patch_httpx_client(monkeypatch):
    recorders: list[_ClientRecorder] = []
    real_client = httpx.Client

    def _factory(*args: Any, **kwargs: Any) -> httpx.Client:
        if "transport" not in kwargs:
            if recorders:
                kwargs["transport"] = httpx.MockTransport(recorders[-1])
            else:
                kwargs["transport"] = httpx.MockTransport(
                    lambda r: httpx.Response(500, json={"err": "no mock registered"})
                )
        return real_client(*args, **kwargs)

    monkeypatch.setattr(httpx, "Client", _factory)
    return recorders


class _FakeStream:
    def __init__(self, lines: list[str]) -> None:
        self._lines = lines

    def __enter__(self) -> _FakeStream:
        return self

    def __exit__(self, *exc: object) -> None:
        return None

    def iter_lines(self) -> Any:
        yield from self._lines

    def raise_for_status(self) -> None:
        pass


class _StreamingClient:
    def __init__(self, stream_lines: list[str]) -> None:
        self.requests: list[httpx.Request] = []
        self._stream_lines = stream_lines

    def __enter__(self) -> _StreamingClient:
        return self

    def __exit__(self, *exc: object) -> None:
        return None

    def stream(
        self,
        method: str,
        url: Any,
        *,
        headers: dict[str, str] | None = None,
        json: dict[str, Any] | None = None,
        **kwargs: Any,
    ) -> _FakeStream:
        req = httpx.Request(method, url, headers=headers, json=json)
        self.requests.append(req)
        return _FakeStream(self._stream_lines)


def _sse(*events: dict) -> list[str]:
    return ["data: " + json.dumps(e) for e in events] + ["data: [DONE]"]


# ---------------------------------------------------------------------------
# Streaming: text
# ---------------------------------------------------------------------------


def test_openai_streaming_text(monkeypatch) -> None:
    events = [
        {
            "id": "chatcmpl-x",
            "object": "chat.completion.chunk",
            "choices": [{"index": 0, "delta": {"role": "assistant", "content": "hello "}}],
        },
        {
            "choices": [{"index": 0, "delta": {"content": "from "}}],
        },
        {
            "choices": [{"index": 0, "delta": {"content": "gpt"}}],
        },
        {
            "choices": [{"index": 0, "delta": {}, "finish_reason": "stop"}],
        },
        {"usage": {"prompt_tokens": 8, "completion_tokens": 3}},
    ]
    lines = _sse(*events)
    fake = _StreamingClient(lines)
    monkeypatch.setattr(httpx, "Client", lambda *args, **kwargs: fake)
    monkeypatch.setenv("OPENAI_API_KEY", "sk-stream")

    adapter = OpenAIAdapter()
    chunks = list(adapter.invoke_stream({"id": "a", "model": "gpt-test"}, "sys", {"q": "hi"}))

    text_deltas = [c.text_delta for c in chunks if c.text_delta]
    assert "".join(text_deltas) == "hello from gpt"

    cost_chunks = [c for c in chunks if c.cost is not None]
    assert cost_chunks, "expected a final cost chunk"
    final_cost = cost_chunks[-1].cost
    assert final_cost.prompt_tokens == 8
    assert final_cost.completion_tokens == 3

    body = json.loads(fake.requests[0].content)
    assert body["stream"] is True
    assert body["messages"][0] == {"role": "system", "content": "sys"}
    assert body["messages"][1]["role"] == "user"


# ---------------------------------------------------------------------------
# Tool use (non-streaming)
# ---------------------------------------------------------------------------


def test_openai_invoke_with_tool_call(monkeypatch, patch_httpx_client) -> None:
    rec = _ClientRecorder(
        {
            "id": "chatcmpl-tool",
            "choices": [
                {
                    "index": 0,
                    "message": {
                        "role": "assistant",
                        "content": None,
                        "tool_calls": [
                            {
                                "id": "call_1",
                                "type": "function",
                                "function": {
                                    "name": "get_weather",
                                    "arguments": '{"city": "sf"}',
                                },
                            }
                        ],
                    },
                    "finish_reason": "tool_calls",
                }
            ],
            "usage": {"prompt_tokens": 6, "completion_tokens": 4},
        }
    )
    patch_httpx_client.append(rec)  # type: ignore[attr-defined]
    monkeypatch.setenv("OPENAI_API_KEY", "sk-tool")

    adapter = OpenAIAdapter()
    agent = {
        "id": "a",
        "provider": "openai",
        "model": "gpt-test",
        "tools": [
            {
                "name": "get_weather",
                "description": "Lookup weather",
                "parameters": {
                    "type": "object",
                    "properties": {"city": {"type": "string"}},
                    "required": ["city"],
                },
            }
        ],
    }
    result = adapter.invoke(agent, "sys", {"q": "weather?"})

    assert result.text == ""
    assert len(result.tool_calls) == 1
    tc = result.tool_calls[0]
    assert tc.id == "call_1"
    assert tc.name == "get_weather"
    assert tc.input == {"city": "sf"}

    body = json.loads(rec.requests[0].content)
    assert body["tools"][0]["function"]["name"] == "get_weather"
    assert "parameters" in body["tools"][0]["function"]


# ---------------------------------------------------------------------------
# Streaming: tool calls
# ---------------------------------------------------------------------------


def test_openai_streaming_tool_call(monkeypatch) -> None:
    events = [
        {
            "choices": [
                {
                    "index": 0,
                    "delta": {
                        "role": "assistant",
                        "tool_calls": [
                            {"index": 0, "id": "call_99", "type": "function",
                             "function": {"name": "lookup", "arguments": ""}},
                        ],
                    },
                }
            ],
        },
        {
            "choices": [
                {
                    "index": 0,
                    "delta": {
                        "tool_calls": [
                            {"index": 0, "function": {"arguments": '{"id": '}},
                        ],
                    },
                }
            ],
        },
        {
            "choices": [
                {
                    "index": 0,
                    "delta": {
                        "tool_calls": [
                            {"index": 0, "function": {"arguments": '"42"}'}},
                        ],
                    },
                    "finish_reason": "tool_calls",
                }
            ],
        },
        {"usage": {"prompt_tokens": 5, "completion_tokens": 8}},
    ]
    lines = _sse(*events)
    fake = _StreamingClient(lines)
    monkeypatch.setattr(httpx, "Client", lambda *args, **kwargs: fake)
    monkeypatch.setenv("OPENAI_API_KEY", "sk-stream-tool")

    adapter = OpenAIAdapter()
    chunks = list(adapter.invoke_stream({"id": "a", "model": "gpt-test"}, "sys", {"q": "lookup"}))

    tool_chunks = [c for c in chunks if c.tool_call is not None]
    assert len(tool_chunks) == 1
    tc = tool_chunks[0].tool_call
    assert tc.id == "call_99"
    assert tc.name == "lookup"
    assert tc.input == {"id": "42"}


# ---------------------------------------------------------------------------
# Multi-turn via _messages
# ---------------------------------------------------------------------------


def test_openai_honors_messages_history(monkeypatch, patch_httpx_client) -> None:
    rec = _ClientRecorder(
        {"choices": [{"message": {"role": "assistant", "content": "round 2"}}], "usage": {}}
    )
    patch_httpx_client.append(rec)  # type: ignore[attr-defined]
    monkeypatch.setenv("OPENAI_API_KEY", "sk-multi")

    adapter = OpenAIAdapter()
    history = [
        {"role": "system", "content": "be brief"},
        {"role": "user", "content": "first question"},
        {"role": "assistant", "content": "first answer"},
        {"role": "tool", "tool_call_id": "t1", "content": "tool output"},
        {"role": "user", "content": "follow up"},
    ]
    result = adapter.invoke({"id": "a", "model": "g"}, "be brief", {"_messages": history})

    assert result.text == "round 2"
    body = json.loads(rec.requests[0].content)
    # No duplicate system message — _messages already has the system message.
    assert body["messages"] == history


def test_anthropic_honors_messages_history(monkeypatch, patch_httpx_client) -> None:
    from hnsx_worker.adapters.anthropic import AnthropicAdapter

    rec = _ClientRecorder(
        {
            "id": "msg-mt",
            "type": "message",
            "role": "assistant",
            "content": [{"type": "text", "text": "got it"}],
            "usage": {},
        }
    )
    patch_httpx_client.append(rec)  # type: ignore[attr-defined]
    monkeypatch.setenv("ANTHROPIC_API_KEY", "sk-mt")

    adapter = AnthropicAdapter()
    history = [
        {"role": "user", "content": "first"},
        {"role": "assistant", "content": [{"type": "text", "text": "first reply"}]},
        {"role": "user", "content": "second"},
    ]
    result = adapter.invoke({"id": "a", "model": "c"}, "be brief", {"_messages": history})

    assert result.text == "got it"
    body = json.loads(rec.requests[0].content)
    # System prompt stays a top-level field.
    assert body["system"] == "be brief"
    assert body["messages"] == history