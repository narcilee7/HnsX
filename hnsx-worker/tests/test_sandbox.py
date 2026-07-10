"""Tests for W6 — sandbox backends.
"""

from __future__ import annotations

import sys

import pytest

from hnsx_worker.sandbox import (
    ContainerSandbox,
    NoneSandbox,
    ProcessSandbox,
    build_backend,
)
from hnsx_worker.tools.base import ToolContext
from hnsx_worker.tools.python import PythonTool, PythonToolConfig


def test_none_sandbox_runs_echo() -> None:
    backend = NoneSandbox()
    result = backend.run([sys.executable, "-c", "print('hello')"])
    assert result.ok is True
    assert "hello" in result.stdout


def test_none_sandbox_captures_stderr() -> None:
    backend = NoneSandbox()
    result = backend.run(
        [sys.executable, "-c", "import sys; sys.stderr.write('err')"]
    )
    assert result.returncode == 0
    assert result.stderr == "err"


def test_none_sandbox_timeout() -> None:
    backend = NoneSandbox()
    result = backend.run(
        [sys.executable, "-c", "import time; time.sleep(10)"],
        timeout_seconds=0.1,
    )
    assert result.ok is False
    assert "timeout" in result.error.lower()


def test_process_sandbox_runs_command() -> None:
    backend = ProcessSandbox()
    result = backend.run([sys.executable, "-c", "print('ok')"])
    assert result.ok is True
    assert "ok" in result.stdout


def test_container_sandbox_missing_runtime() -> None:
    backend = ContainerSandbox(runtime="definitely_not_docker_12345")
    result = backend.run(["echo", "hi"])
    assert result.ok is False
    assert "container runtime" in result.error.lower()


def test_build_backend_defaults_to_none() -> None:
    backend = build_backend(None)
    assert backend.name == "none"


def test_build_backend_process() -> None:
    backend = build_backend({"backend": "process", "use_namespace": True})
    assert isinstance(backend, ProcessSandbox)
    assert backend.use_namespace is True


def test_build_backend_unknown_raises() -> None:
    with pytest.raises(ValueError, match="unknown sandbox backend"):
        build_backend({"backend": "vm"})


def test_python_tool_runs_in_process_sandbox() -> None:
    tool = PythonTool("calc", PythonToolConfig(timeout_seconds=2))
    ctx = ToolContext(sandbox=ProcessSandbox())
    result = tool.invoke(ctx, {"code": "print('hello from sandbox')"})
    assert result.ok is True
    assert "hello from sandbox" in result.output["stdout"]
    assert result.metadata.get("sandbox") == "process"


def test_python_tool_sandbox_timeout_emits_violation() -> None:
    tool = PythonTool("slow", PythonToolConfig(timeout_seconds=0.1))
    observations: list[dict] = []
    ctx = ToolContext(
        sandbox=ProcessSandbox(),
        emit=lambda o: observations.append(o),
    )
    result = tool.invoke(ctx, {"code": "import time; time.sleep(10)"})
    assert result.ok is False
    assert any(o["kind"] == "sandbox_violation" for o in observations)
    assert "timeout" in result.error.lower()
