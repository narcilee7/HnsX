"""Tests for W4 — CLI Agent adapters (Claude Code / Codex).

We test with *fake* CLI binaries written to a temporary directory so the suite
doesn't require the real ``claude`` or ``codex`` CLIs to be installed.
"""

from __future__ import annotations

import os
import stat
from pathlib import Path

import pytest

from hnsx_worker.adapters import AdapterRegistry
from hnsx_worker.adapters.claude_code import ClaudeCodeAdapter
from hnsx_worker.adapters.cli_base import CliAdapter, CliToolPattern
from hnsx_worker.adapters.codex import CodexAdapter


@pytest.fixture
def fake_claude(tmp_path: Path) -> str:
    """Create a fake ``claude`` executable that echoes its prompt."""
    script = tmp_path / "claude"
    script.write_text(
        "#!/bin/sh\n"
        'echo "Thinking..."\n'
        'echo "Running: ls -la"\n'
        'echo "Done."\n'
    )
    script.chmod(script.stat().st_mode | stat.S_IEXEC)
    return str(tmp_path)


@pytest.fixture
def fake_codex(tmp_path: Path) -> str:
    """Create a fake ``codex`` executable that writes a file and reports it."""
    script = tmp_path / "codex"
    script.write_text(
        "#!/bin/sh\n"
        'echo "Reading file: src/main.py"\n'
        'echo "Writing file: src/main.py"\n'
        'echo "Done."\n'
    )
    script.chmod(script.stat().st_mode | stat.S_IEXEC)
    return str(tmp_path)


def _collect_chunks(adapter: CliAdapter, agent: dict, prompt: str, input: dict):
    """Helper to materialize the stream into a list."""
    return list(adapter.invoke_stream(agent, prompt, input))


def test_claude_code_adapter_streams_text_and_detects_bash(fake_claude: str) -> None:
    adapter = ClaudeCodeAdapter()
    agent = {
        "id": "claude-agent",
        "adapter": {
            "kind": "claudecode",
            "config": {"workdir": "."},
        },
        "workdir": ".",
    }
    # Point the adapter at the fake binary directory.
    os.environ["PATH"] = fake_claude + os.pathsep + os.environ.get("PATH", "")
    chunks = _collect_chunks(adapter, agent, "list files", {})

    text = "".join(c.text_delta for c in chunks if c.text_delta)
    assert "Thinking..." in text
    assert "Running: ls -la" in text

    tool_calls = [c.tool_call for c in chunks if c.tool_call is not None]
    assert len(tool_calls) == 1
    assert tool_calls[0].name == "bash"
    assert "ls -la" in tool_calls[0].input.get("command", "")


def test_codex_adapter_streams_text_and_detects_file_ops(fake_codex: str) -> None:
    adapter = CodexAdapter()
    agent = {
        "id": "codex-agent",
        "adapter": {
            "kind": "codex",
            "config": {"workdir": "."},
        },
    }
    os.environ["PATH"] = fake_codex + os.pathsep + os.environ.get("PATH", "")
    chunks = _collect_chunks(adapter, agent, "review code", {})

    text = "".join(c.text_delta for c in chunks if c.text_delta)
    assert "Reading file: src/main.py" in text

    names = [c.tool_call.name for c in chunks if c.tool_call is not None]
    assert "file_read" in names
    assert "file_write" in names


def test_claude_code_invoke_fallback(fake_claude: str) -> None:
    adapter = ClaudeCodeAdapter()
    agent = {
        "id": "claude-agent",
        "adapter": {"kind": "claudecode", "config": {"workdir": "."}},
    }
    os.environ["PATH"] = fake_claude + os.pathsep + os.environ.get("PATH", "")
    result = adapter.invoke(agent, "hello", {})
    assert "Running: ls -la" in result.text
    assert len(result.tool_calls) == 1


def test_claude_code_missing_binary_raises() -> None:
    adapter = ClaudeCodeAdapter()
    agent = {
        "id": "claude-agent",
        "adapter": {
            "kind": "claudecode",
            "config": {"binary": "definitely_not_claude_12345"},
        },
    }
    with pytest.raises(RuntimeError, match="not found"):
        list(adapter.invoke_stream(agent, "hello", {}))


def test_claude_code_tool_pattern_detection() -> None:
    adapter = ClaudeCodeAdapter()
    patterns = adapter._tool_patterns()
    names = {p.tool_name for p in patterns}
    assert {"bash", "file_read", "file_write", "file_delete", "edit"}.issubset(names)


def test_registry_lazy_registers_cli_adapters() -> None:
    kinds = AdapterRegistry.kinds()
    assert "claudecode" in kinds
    assert "codex" in kinds


def test_custom_pattern_groupdict_becomes_input() -> None:
    class _FakeAdapter(CliAdapter):
        def name(self) -> str:
            return "fake"

        def _cli_binary(self) -> str:
            return "echo"

        def _tool_patterns(self) -> list[CliToolPattern]:
            return [CliToolPattern("echo", __import__("re").compile(r"path=(?P<path>\S+)"))]

    adapter = _FakeAdapter()
    agent = {"id": "fake", "adapter": {"kind": "fake"}}
    chunks = _collect_chunks(adapter, agent, "path=/tmp/x", {})
    tool_calls = [c.tool_call for c in chunks if c.tool_call is not None]
    assert len(tool_calls) == 1
    assert tool_calls[0].input["path"] == "/tmp/x"


@pytest.fixture(autouse=True)
def _reset_registry():
    """Drop cached singletons after each test to avoid cross-test bleed."""
    yield
    AdapterRegistry.reset()
