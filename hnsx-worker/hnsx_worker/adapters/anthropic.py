"""Anthropic Messages API adapter.

Calls https://docs.anthropic.com/en/api/messages via httpx. The API key is
read from the environment variable named in ``agent.api_key_env`` (per
the DomainSpec convention). A custom ``agent.adapter.endpoint`` overrides
the default base URL for testing or self-hosted Anthropic-compatible
deployments.
"""

from __future__ import annotations

import json
import logging
import os

import httpx

from hnsx_worker.adapters.base import Adapter, AdapterResult, Cost

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
            raise RuntimeError(
                f"anthropic adapter: API key env var {agent.get('api_key_env', 'ANTHROPIC_API_KEY')!r} "
                "is not set"
            )

        base_url = (agent.get("adapter", {}) or {}).get("endpoint") or _DEFAULT_BASE_URL
        timeout = (agent.get("adapter", {}) or {}).get("timeout_seconds", 60)
        model = agent.get("model") or _DEFAULT_MODEL
        max_tokens = int((agent.get("adapter", {}) or {}).get("max_tokens", _DEFAULT_MAX_TOKENS))

        headers = {
            "x-api-key": api_key,
            "anthropic-version": _API_VERSION,
            "content-type": "application/json",
        }
        body = {
            "model": model,
            "max_tokens": max_tokens,
            "system": prompt,
            "messages": [
                {"role": "user", "content": _input_to_user_content(input)},
            ],
        }

        with httpx.Client(base_url=base_url, timeout=timeout) as client:
            resp = client.post("/v1/messages", headers=headers, json=body)
            resp.raise_for_status()
            data = resp.json()

        text = _extract_text(data)
        usage = data.get("usage", {}) or {}
        cost = Cost(
            prompt_tokens=int(usage.get("input_tokens", 0) or 0),
            completion_tokens=int(usage.get("output_tokens", 0) or 0),
        )
        return AdapterResult(text=text, cost=cost)


# ---------------------------------------------------------------------------
# helpers
# ---------------------------------------------------------------------------


def _resolve_api_key(agent: dict) -> str:
    env_name = agent.get("api_key_env") or "ANTHROPIC_API_KEY"
    return os.environ.get(env_name, "")


def _input_to_user_content(input: dict) -> str:
    """Serialize the turn's input dict as the user-role message.

    We send the dict as pretty JSON so the model sees a clean structured
    payload (rather than Python ``repr``). For V1.1 we keep it dead simple;
    later this hook will turn ``input`` into multi-turn message lists.
    """
    if isinstance(input, dict) and len(input) == 1 and "content" in input:
        # Already shaped: {"content": "..."}
        value = input["content"]
        return str(value) if not isinstance(value, str) else value
    return json.dumps(input, ensure_ascii=False, default=str)


def _extract_text(data: dict) -> str:
    """Pull the text out of an Anthropic Messages response.

    Response shape::

        {"content": [{"type": "text", "text": "..."}, ...]}
    """
    parts: list[str] = []
    for block in data.get("content", []) or []:
        if block.get("type") == "text":
            parts.append(block.get("text", ""))
    return "".join(parts)
