"""Tests for the Python tool (W3.x).

Coverage:

  - Config parsing + validation.
  - Schema exposure.
  - Executes simple expressions and returns the value.
  - Captures stdout / stderr.
  - Truncates long output.
  - Restricts builtins (open / __import__ blocked).
  - Blocks network modules when allow_network=False.
  - Surfaces exceptions as structured errors (no crash).
  - Timeout fires on long-running code (Unix + thread fallback).
  - locals are injected into the namespace.
"""

from __future__ import annotations

import os
import sys

import pytest

from hnsx_worker.tools import ToolContext
from hnsx_worker.tools.python import (
    PythonTool,
    PythonToolConfig,
    _BlockedModule,
    _build_namespace,
    _truncate,
)


def _make_tool(
    *,
    name: str = "py",
    config: dict | None = None,
) -> PythonTool:
    base = {"timeout_seconds": 5, "max_output_bytes": 1024}
    return PythonTool(name, PythonToolConfig.from_spec({**base, **(config or {})}))


# ---------------------------------------------------------------------------
# Config
# ---------------------------------------------------------------------------


def test_config_defaults() -> None:
    cfg = PythonToolConfig.from_spec({})
    assert cfg.timeout_seconds == 5.0
    assert cfg.max_output_bytes == 65536
    assert cfg.allow_network is False
    assert cfg.echo_code is False


def test_config_rejects_non_positive_timeout() -> None:
    with pytest.raises(ValueError, match="timeout_seconds"):
        PythonToolConfig.from_spec({"timeout_seconds": 0})


def test_config_rejects_negative_max_output_bytes() -> None:
    with pytest.raises(ValueError, match="max_output_bytes"):
        PythonToolConfig.from_spec({"max_output_bytes": -1})


# ---------------------------------------------------------------------------
# Schema
# ---------------------------------------------------------------------------


def test_schema_requires_code() -> None:
    tool = _make_tool()
    schema = tool.schema
    assert schema["required"] == ["code"]
    assert "code" in schema["properties"]
    assert "locals" in schema["properties"]


# ---------------------------------------------------------------------------
# Execution
# ---------------------------------------------------------------------------


def test_simple_expression_returns_value() -> None:
    tool = _make_tool()
    result = tool.invoke(ToolContext(), {"code": "1 + 2"})
    assert result.ok, result.error
    assert result.output["result"] == "3"


def test_print_output_is_captured() -> None:
    tool = _make_tool()
    result = tool.invoke(ToolContext(), {"code": "print('hello')"})
    assert result.ok
    assert "hello" in result.output["stdout"]


def test_stderr_is_captured() -> None:
    tool = _make_tool()
    result = tool.invoke(
        ToolContext(),
        {"code": "import sys; sys.stderr.write('warn\\n')"},
    )
    assert result.ok
    assert "warn" in result.output["stderr"]


def test_statements_plus_expression_at_end() -> None:
    tool = _make_tool()
    code = "x = 10\nx * x"
    result = tool.invoke(ToolContext(), {"code": code})
    assert result.ok, result.error
    assert result.output["result"] == "100"


def test_locals_are_injected() -> None:
    tool = _make_tool()
    result = tool.invoke(ToolContext(), {"code": "a + b", "locals": {"a": 2, "b": 3}})
    assert result.ok
    assert result.output["result"] == "5"


def test_safe_modules_available() -> None:
    tool = _make_tool()
    result = tool.invoke(ToolContext(), {"code": "json.dumps({'k': 1})"})
    assert result.ok
    # json.dumps uses double quotes; ``repr`` of that string wraps in
    # single quotes.
    assert result.output["result"] == '\'{"k": 1}\''


# ---------------------------------------------------------------------------
# Restrictions
# ---------------------------------------------------------------------------


def test_open_is_blocked() -> None:
    tool = _make_tool()
    result = tool.invoke(ToolContext(), {"code": "open('/etc/passwd').read()"})
    assert not result.ok
    assert "NameError" in (result.error or "")


def test_import_os_system_blocked() -> None:
    tool = _make_tool()
    # __import__ is stripped; ``import os`` itself fails because the
    # builtins namespace doesn't expose __import__.
    result = tool.invoke(ToolContext(), {"code": "import os; os.system('echo hi')"})
    assert not result.ok


def test_network_blocked_when_allow_network_false() -> None:
    tool = _make_tool()
    result = tool.invoke(ToolContext(), {"code": "socket.socket()"})
    assert not result.ok
    assert "blocked" in (result.error or "").lower()


def test_network_allowed_when_allow_network_true() -> None:
    """With allow_network=True the network stubs are not injected; the
    tool relies on the user to handle their own imports. We don't try to
    actually open a socket in a test."""
    tool = _make_tool(config={"allow_network": True})
    # The ``socket`` placeholder is not present, so the name is not bound.
    # This raises NameError rather than a "blocked" error.
    result = tool.invoke(ToolContext(), {"code": "socket.socket()"})
    assert not result.ok
    assert "NameError" in (result.error or "") or "not defined" in (result.error or "")


def test_blocked_module_attribute_access_raises() -> None:
    mod = _BlockedModule("socket")
    with pytest.raises(RuntimeError, match="blocked"):
        mod.connect(("example.com", 80))


# ---------------------------------------------------------------------------
# Output truncation
# ---------------------------------------------------------------------------


def test_truncate_returns_unchanged_when_within_limit() -> None:
    out, truncated = _truncate("abc", 100)
    assert out == "abc"
    assert truncated is False


def test_truncate_caps_long_strings() -> None:
    long = "x" * 1000
    out, truncated = _truncate(long, 100)
    assert len(out) == 100
    assert truncated is True


def test_stdout_truncation_flagged() -> None:
    tool = _make_tool(config={"max_output_bytes": 16})
    result = tool.invoke(ToolContext(), {"code": "print('x' * 500)"})
    assert result.ok
    assert result.metadata["stdout_truncated"] is True
    assert len(result.output["stdout"]) == 16


# ---------------------------------------------------------------------------
# Errors
# ---------------------------------------------------------------------------


def test_syntax_error_returns_structured_error() -> None:
    tool = _make_tool()
    result = tool.invoke(ToolContext(), {"code": "this is not python !@#"})
    assert not result.ok
    assert result.metadata.get("exception_type") == "SyntaxError"


def test_runtime_error_returns_structured_error() -> None:
    tool = _make_tool()
    result = tool.invoke(ToolContext(), {"code": "1/0"})
    assert not result.ok
    assert "ZeroDivisionError" in (result.error or "")


def test_undefined_name_returns_error() -> None:
    tool = _make_tool()
    result = tool.invoke(ToolContext(), {"code": "undefined_thing"})
    assert not result.ok
    assert "NameError" in (result.error or "")


def test_empty_code_returns_error() -> None:
    tool = _make_tool()
    result = tool.invoke(ToolContext(), {"code": "   "})
    assert not result.ok
    assert "non-empty" in (result.error or "")


def test_input_must_be_dict() -> None:
    tool = _make_tool()
    result = tool.invoke(ToolContext(), "not a dict")  # type: ignore[arg-type]
    assert not result.ok
    assert "JSON object" in (result.error or "")


def test_locals_must_be_object() -> None:
    tool = _make_tool()
    result = tool.invoke(ToolContext(), {"code": "1", "locals": "bad"})
    assert not result.ok
    assert "locals" in (result.error or "")


def test_locals_non_serializable_returns_error() -> None:
    tool = _make_tool()
    # ``object()`` isn't JSON-serializable; the tool should reject.
    result = tool.invoke(
        ToolContext(),
        {"code": "1", "locals": {"x": object()}},
    )
    assert not result.ok
    assert "locals" in (result.error or "").lower()


# ---------------------------------------------------------------------------
# Timeout
# ---------------------------------------------------------------------------


@pytest.mark.skipif(
    sys.platform.startswith("win"),
    reason="SIGALRM watchdog is unix-only",
)
def test_sigarm_timeout_on_long_loop() -> None:
    """Signal-based watchdog: tight timeout on a CPU-bound loop.

    SIGALRM only fires reliably on the main thread. ``pytest`` runs tests
    on the main thread, so this test exercises the signal path on POSIX.
    """
    import threading

    if threading.current_thread() is not threading.main_thread():
        pytest.skip("SIGALRM only fires on the main thread")
    tool = _make_tool(config={"timeout_seconds": 0.5})
    result = tool.invoke(ToolContext(), {"code": "while True: pass"})
    assert not result.ok
    assert "timeout" in (result.error or "").lower()
    assert result.metadata.get("timed_out") is True


def test_thread_timeout_on_long_loop() -> None:
    """Thread-based fallback: another thread blocks on a CPU loop, the
    main thread times out via join()."""
    import threading

    # Skip if we're on the main thread (where SIGALRM would actually fire
    # reliably and the test would test that path, not the thread path).
    if threading.current_thread() is threading.main_thread() and os.name == "posix":
        pytest.skip("would exercise the SIGALRM path on POSIX main thread")

    tool = _make_tool(config={"timeout_seconds": 0.2})
    result = tool.invoke(ToolContext(), {"code": "while True: pass"})
    assert not result.ok
    assert "timeout" in (result.error or "").lower()


# ---------------------------------------------------------------------------
# Builtins smoke
# ---------------------------------------------------------------------------


def test_safe_builtins_include_print_and_len() -> None:
    ns = _build_namespace({}, allow_network=False)
    assert callable(ns["__builtins__"]["print"])
    assert ns["__builtins__"]["len"]([1, 2, 3]) == 3


def test_safe_builtins_exclude_open() -> None:
    ns = _build_namespace({}, allow_network=False)
    assert "open" not in ns["__builtins__"]


# ---------------------------------------------------------------------------
# ToolResult integration with registry
# ---------------------------------------------------------------------------


def test_python_tool_wires_through_registry() -> None:
    from hnsx_worker.tools import ToolRegistry

    reg = ToolRegistry()
    reg.register(_make_tool(name="calc"))
    result = reg.call("calc", ToolContext(), {"code": "sum([1, 2, 3])"})
    assert result.ok
    assert result.output["result"] == "6"


# ---------------------------------------------------------------------------
# Defensive: _exec_last_expression handles eval-only snippets
# ---------------------------------------------------------------------------


def test_single_expression_snippet_returns_value() -> None:
    """A snippet that's just one expression (no statements) should still
    produce a result. The exec-after-syntax-error fallback path."""
    tool = _make_tool()
    result = tool.invoke(ToolContext(), {"code": "max(1, 2, 3)"})
    assert result.ok
    assert result.output["result"] == "3"
