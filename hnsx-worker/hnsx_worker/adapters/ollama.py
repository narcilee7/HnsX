"""Ollama HTTP adapter.

Calls http://localhost:11434/api/chat via httpx. Ollama is a popular
local-model runtime; this adapter speaks its native ``/api/chat`` endpoint
(OpenAI-compatible is also available at ``/v1/chat/completions`` but the
native shape gives us token counts without an extra config knob).
"""

from __future__ import annotations

import json
import logging
import os

import httpx

from hnsx_worker.adapters.base import Adapter, AdapterResult, Cost

log = logging.getLogger("hnsx_worker.adapters.ollama")

_DEFAULT_BASE_URL = "http://localhost:11434"
_DEFAULT_MODEL = "llama3.1"


class OllamaAdapter(Adapter):
    """Adapter for a locally running Ollama server."""

    def name(self) -> str:
        return "ollama"

    def invoke(self, agent: dict, prompt: str, input: dict) -> AdapterResult:
        base_url = (agent.get("adapter", {}) or {}).get("endpoint") or os.environ.get(
            "OLLAMA_HOST", _DEFAULT_BASE_URL
        )
        if not base_url.startswith("http"):
            base_url = f"http://{base_url}"
        timeout = (agent.get("adapter", {}) or {}).get("timeout_seconds", 120)
        model = agent.get("model") or _DEFAULT_MODEL

        body = {
            "model": model,
            "stream": False,
            "messages": [
                {"role": "system", "content": prompt},
                {"role": "user", "content": _input_to_user_content(input)},
            ],
        }

        with httpx.Client(base_url=base_url, timeout=timeout) as client:
            resp = client.post("/api/chat", json=body)
            resp.raise_for_status()
            data = resp.json()

        text = (data.get("message") or {}).get("content", "") or ""
        cost = Cost(
            prompt_tokens=int((data.get("prompt_eval_count") or 0)),
            completion_tokens=int((data.get("eval_count") or 0)),
        )
        return AdapterResult(text=text, cost=cost)


def _input_to_user_content(input: dict) -> str:
    if isinstance(input, dict) and len(input) == 1 and "content" in input:
        value = input["content"]
        return str(value) if not isinstance(value, str) else value
    return json.dumps(input, ensure_ascii=False, default=str)
