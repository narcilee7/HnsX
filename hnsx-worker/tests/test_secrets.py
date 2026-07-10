"""Tests for W6 — secret injection via environment variables.
"""

from __future__ import annotations

import json
import os
import threading
from unittest import mock

from hnsx_worker.session_executor import execute_session
from hnsx_worker.session_runtime import _load_secrets_from_env, _merge_secrets_into_config


def test_load_secrets_from_env() -> None:
    env = {
        "HNSX_SECRET_API_KEY": "secret-key",
        "HNSX_SECRET_DB_PASS": "secret-pass",
        "UNRELATED": "x",
    }
    with mock.patch.dict(os.environ, env, clear=False):
        secrets = _load_secrets_from_env()
    assert secrets == {"api_key": "secret-key", "db_pass": "secret-pass"}


def test_merge_secrets_into_config_dict() -> None:
    config: dict = {"secrets": {"api_key": "from-payload"}}
    env = {"HNSX_SECRET_API_KEY": "from-env", "HNSX_SECRET_DB_PASS": "from-env"}
    with mock.patch.dict(os.environ, env, clear=False):
        _merge_secrets_into_config(config)
    # Payload secrets win over env secrets.
    assert config["secrets"]["api_key"] == "from-payload"
    assert config["secrets"]["db_pass"] == "from-env"


def test_merge_secrets_into_config_list() -> None:
    config: dict = {"secrets": [{"name": "api_key", "value": "from-payload"}]}
    with mock.patch.dict(os.environ, {"HNSX_SECRET_DB_PASS": "from-env"}, clear=False):
        _merge_secrets_into_config(config)
    assert config["secrets"][-1] == {"name": "db_pass", "value": "from-env"}


def test_executor_resolves_secret_in_tool() -> None:
    """A tool config with {secret.api_key} resolves from config secrets."""
    spec = {
        "id": "secret-test",
        "harness": {
            "agents": {
                "primary": {
                    "id": "primary",
                    "adapter": {"kind": "scripted_secret"},
                    "system_prompt": "x",
                    "tools": [
                        {
                            "name": "fetch",
                            "type": "http",
                            "config": {
                                "url": "https://api.example.com/x",
                                "headers": {"Authorization": "Bearer {secret.api_key}"},
                            },
                        }
                    ],
                    "_scripted_response": {
                        "text_chunks": ["calling tool"],
                        "tool_calls": [
                            {"id": "tc-1", "name": "fetch", "input": {"path_params": {}}}
                        ],
                    },
                }
            },
            "session": {"mode": "multi-turn", "agent": "primary"},
            "policy": {"budget": {"max_turns": 1}},
        },
    }

    from hnsx_worker.adapters import AdapterRegistry
    from hnsx_worker.adapters.base import Adapter, AdapterResult, Cost, StreamChunk, ToolCall

    class _ScriptedSecret(Adapter):
        def name(self) -> str:
            return "scripted_secret"

        def invoke(self, agent: dict, prompt: str, input: dict) -> AdapterResult:
            return AdapterResult(text="ok")

        def invoke_stream(self, agent: dict, prompt: str, input: dict):
            scripts = agent.get("_scripted_response", {})
            for chunk in scripts.get("text_chunks", []):
                yield StreamChunk(text_delta=chunk)
            for tc in scripts.get("tool_calls", []):
                yield StreamChunk(tool_call=ToolCall(**tc))
            yield StreamChunk(cost=Cost())

    AdapterRegistry.register("scripted_secret", _ScriptedSecret)
    captured: list[dict] = []
    try:
        execute_session(
            spec,
            trigger={"q": "x"},
            config={"session_id": "s-secret", "secrets": {"api_key": "resolved-key"}},
            stop_event=threading.Event(),
            emit=captured.append,
        )
    finally:
        AdapterRegistry._registry.pop("scripted_secret", None)
        AdapterRegistry._singletons.pop("scripted_secret", None)

    tool_results = [o for o in captured if o["kind"] == "tool_result"]
    assert len(tool_results) == 1
    # The resolved Authorization header should NOT appear in observations.
    assert "resolved-key" not in json.dumps(captured)
    # The tool call was attempted (network error is expected against example.com).
    assert tool_results[0]["payload"]["name"] == "fetch"
