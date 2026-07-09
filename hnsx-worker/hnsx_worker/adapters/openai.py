"""OpenAI Chat Completions API adapter.

Calls https://api.openai.com/v1/chat/completions via httpx. The API key is
read from the environment variable named in ``agent.api_key_env``. A custom
``agent.adapter.endpoint`` overrides the default base URL for testing,
self-hosted OpenAI-compatible servers (vLLM, llama.cpp, etc.), or Azure-style
frontends.
"""

from __future__ import annotations

import json
import logging
import os

import httpx

from hnsx_worker.adapters.base import Adapter, AdapterResult, Cost

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

        headers = {
            "authorization": f"Bearer {api_key}",
            "content-type": "application/json",
        }
        body = {
            "model": model,
            "max_tokens": max_tokens,
            "messages": [
                {"role": "system", "content": prompt},
                {"role": "user", "content": _input_to_user_content(input)},
            ],
        }

        with httpx.Client(base_url=base_url, timeout=timeout) as client:
            resp = client.post("/v1/chat/completions", headers=headers, json=body)
            resp.raise_for_status()
            data = resp.json()

        text = _extract_text(data)
        usage = data.get("usage", {}) or {}
        cost = Cost(
            prompt_tokens=int(usage.get("prompt_tokens", 0) or 0),
            completion_tokens=int(usage.get("completion_tokens", 0) or 0),
        )
        return AdapterResult(text=text, cost=cost)


# ---------------------------------------------------------------------------
# helpers
# ---------------------------------------------------------------------------


def _resolve_api_key(agent: dict) -> str:
    env_name = agent.get("api_key_env") or "OPENAI_API_KEY"
    return os.environ.get(env_name, "")


def _input_to_user_content(input: dict) -> str:
    if isinstance(input, dict) and len(input) == 1 and "content" in input:
        value = input["content"]
        return str(value) if not isinstance(value, str) else value
    return json.dumps(input, ensure_ascii=False, default=str)


def _extract_text(data: dict) -> str:
    """Pull the text out of a Chat Completions response.

    Response shape::

        {"choices": [{"message": {"role": "assistant", "content": "..."}, ...}]}
    """
    parts: list[str] = []
    for choice in data.get("choices", []) or []:
        msg = choice.get("message") or {}
        parts.append(msg.get("content", "") or "")
    return "".join(parts)
