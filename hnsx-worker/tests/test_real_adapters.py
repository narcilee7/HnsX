"""Mocked-HTTP tests for the real provider adapters (anthropic / openai / ollama).

Each test monkeypatches ``httpx.Client`` so the adapter talks to an
in-process ``httpx.MockTransport``, not the real provider. This lets us
verify the wire-level contract (request body, headers, response parsing)
without needing API keys or network access.
"""

from __future__ import annotations

import json
from typing import Any

import httpx
import pytest

from hnsx_worker.adapters import AdapterRegistry
from hnsx_worker.adapters.anthropic import AnthropicAdapter
from hnsx_worker.adapters.ollama import OllamaAdapter
from hnsx_worker.adapters.openai import OpenAIAdapter


# ---------------------------------------------------------------------------
# helpers
# ---------------------------------------------------------------------------


class _ClientRecorder:
    """Records every request received by a MockTransport and returns canned responses."""

    def __init__(self, response_payload: dict, status_code: int = 200) -> None:
        self.response_payload = response_payload
        self.status_code = status_code
        self.requests: list[httpx.Request] = []

    def __call__(self, request: httpx.Request) -> httpx.Response:
        self.requests.append(request)
        return httpx.Response(self.status_code, json=self.response_payload)


@pytest.fixture
def patch_httpx_client(monkeypatch):
    """Replace ``httpx.Client`` with a factory that builds a MockTransport client.

    Returns a list-of-_ClientRecorder; one per Client() call.
    """
    recorders: list[_ClientRecorder] = []
    real_client = httpx.Client  # bound BEFORE we patch the name

    def _factory(*args: Any, **kwargs: Any) -> httpx.Client:
        # Default to the most recently registered recorder's handler if the
        # caller didn't supply a transport.
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


# ---------------------------------------------------------------------------
# Anthropic
# ---------------------------------------------------------------------------


def test_anthropic_adapter_sends_expected_request(monkeypatch, patch_httpx_client) -> None:
    rec = _ClientRecorder(
        {
            "id": "msg_test",
            "type": "message",
            "role": "assistant",
            "content": [{"type": "text", "text": "hello from claude"}],
            "usage": {"input_tokens": 12, "output_tokens": 7},
        }
    )
    patch_httpx_client.append(rec)  # type: ignore[attr-defined]
    monkeypatch.setenv("ANTHROPIC_API_KEY", "sk-test-anthropic")

    adapter = AnthropicAdapter()
    agent = {
        "id": "triage",
        "provider": "anthropic",
        "model": "claude-haiku-4-5",
        "adapter": {"kind": "anthropic", "timeout_seconds": 30},
        "api_key_env": "ANTHROPIC_API_KEY",
    }
    result = adapter.invoke(agent, "you are a triage agent", {"question": "hi"})

    assert result.text == "hello from claude"
    assert result.cost is not None
    assert result.cost.prompt_tokens == 12
    assert result.cost.completion_tokens == 7

    # Wire-level contract.
    assert len(rec.requests) == 1
    req = rec.requests[0]
    assert str(req.url) == "https://api.anthropic.com/v1/messages"
    assert req.headers["x-api-key"] == "sk-test-anthropic"
    assert req.headers["anthropic-version"] == "2023-06-01"
    body = json.loads(req.content)
    assert body["model"] == "claude-haiku-4-5"
    assert body["system"] == "you are a triage agent"
    assert body["messages"] == [{"role": "user", "content": '{"question": "hi"}'}]


def test_anthropic_adapter_raises_when_api_key_missing(monkeypatch) -> None:
    monkeypatch.delenv("ANTHROPIC_API_KEY", raising=False)
    adapter = AnthropicAdapter()
    agent = {
        "id": "x",
        "provider": "anthropic",
        "model": "x",
        "api_key_env": "DEFINITELY_NOT_SET",
    }
    with pytest.raises(RuntimeError, match="API key env var"):
        adapter.invoke(agent, "sys", {"q": "1"})


# ---------------------------------------------------------------------------
# OpenAI
# ---------------------------------------------------------------------------


def test_openai_adapter_sends_expected_request(monkeypatch, patch_httpx_client) -> None:
    rec = _ClientRecorder(
        {
            "id": "chatcmpl-test",
            "object": "chat.completion",
            "choices": [{"index": 0, "message": {"role": "assistant", "content": "hello from gpt"}}],
            "usage": {"prompt_tokens": 5, "completion_tokens": 9, "total_tokens": 14},
        }
    )
    patch_httpx_client.append(rec)  # type: ignore[attr-defined]
    monkeypatch.setenv("OPENAI_API_KEY", "sk-test-openai")

    adapter = OpenAIAdapter()
    agent = {
        "id": "billing",
        "provider": "openai",
        "model": "gpt-4o-mini",
        "adapter": {"kind": "openai", "timeout_seconds": 60},
        "api_key_env": "OPENAI_API_KEY",
    }
    result = adapter.invoke(agent, "you are the billing agent", {"question": "refund please"})

    assert result.text == "hello from gpt"
    assert result.cost is not None
    assert result.cost.prompt_tokens == 5
    assert result.cost.completion_tokens == 9

    req = rec.requests[0]
    assert str(req.url) == "https://api.openai.com/v1/chat/completions"
    assert req.headers["authorization"] == "Bearer sk-test-openai"
    body = json.loads(req.content)
    assert body["model"] == "gpt-4o-mini"
    assert body["messages"][0] == {"role": "system", "content": "you are the billing agent"}
    assert body["messages"][1] == {"role": "user", "content": '{"question": "refund please"}'}


def test_openai_adapter_uses_custom_endpoint(monkeypatch, patch_httpx_client) -> None:
    """``agent.adapter.endpoint`` overrides the base URL (for Azure / vLLM / mocks)."""
    rec = _ClientRecorder({"choices": [{"message": {"content": "ok"}}], "usage": {}})
    patch_httpx_client.append(rec)  # type: ignore[attr-defined]
    monkeypatch.setenv("OPENAI_API_KEY", "sk-x")

    adapter = OpenAIAdapter()
    agent = {
        "id": "a",
        "provider": "openai",
        "model": "custom",
        "adapter": {"kind": "openai", "endpoint": "https://my-vllm.example.com"},
    }
    result = adapter.invoke(agent, "sys", {"q": "1"})
    assert result.text == "ok"
    assert rec.requests[0].url.host == "my-vllm.example.com"


# ---------------------------------------------------------------------------
# Ollama
# ---------------------------------------------------------------------------


def test_ollama_adapter_sends_expected_request(monkeypatch, patch_httpx_client) -> None:
    rec = _ClientRecorder(
        {
            "model": "llama3.1",
            "message": {"role": "assistant", "content": "hi from llama"},
            "prompt_eval_count": 11,
            "eval_count": 4,
        }
    )
    patch_httpx_client.append(rec)  # type: ignore[attr-defined]

    adapter = OllamaAdapter()
    agent = {
        "id": "local",
        "provider": "ollama",
        "model": "llama3.1",
        "adapter": {"kind": "ollama", "timeout_seconds": 120},
    }
    result = adapter.invoke(agent, "sys", {"q": "1"})

    assert result.text == "hi from llama"
    assert result.cost is not None
    assert result.cost.prompt_tokens == 11
    assert result.cost.completion_tokens == 4

    req = rec.requests[0]
    assert str(req.url) == "http://localhost:11434/api/chat"
    body = json.loads(req.content)
    assert body["model"] == "llama3.1"
    assert body["messages"][0] == {"role": "system", "content": "sys"}


# ---------------------------------------------------------------------------
# Registry
# ---------------------------------------------------------------------------


def test_registry_includes_real_adapters() -> None:
    assert "anthropic" in AdapterRegistry.kinds()
    assert "openai" in AdapterRegistry.kinds()
    assert "ollama" in AdapterRegistry.kinds()
    assert "noop" in AdapterRegistry.kinds()
    assert "echo" in AdapterRegistry.kinds()


def test_registry_returns_singletons() -> None:
    a1 = AdapterRegistry.get("noop")
    a2 = AdapterRegistry.get("noop")
    assert a1 is a2
