"""Python execution tool — runs a snippet in a restricted namespace.

W3.x ships a *minimal* in-process Python tool. The execution surface is
limited by:

  1. **Restricted builtins.** A curated :data:`_SAFE_BUILTINS` mapping is
     exposed instead of Python's full ``builtins``. ``open`` / ``__import__``
     / ``exec`` / ``eval`` are gone; ``print`` / ``len`` / ``range`` /
     ``json`` / ``math`` / ``datetime`` etc. are available.
  2. **Timeout.** A wall-clock watchdog kills the execution when it
     runs past ``timeout_seconds`` (Unix-only; uses SIGALRM in the main
     thread).
  3. **Output cap.** Both ``stdout`` and ``stderr`` are truncated to
     ``max_output_bytes`` so a runaway ``print()`` doesn't blow up the
     observation stream.
  4. **No fs / network.** No module in the safe namespace can read the
     filesystem or open sockets without explicit opt-in via
     ``config.allow_network: true`` (still gated by the policy engine
     in W6 — don't flip on in production without a real sandbox).

This is intentionally **not** a hardened sandbox. W6 will plug in a real
backend (subprocess + namespace / container) and replace this stub. The
W3 tool is here so the Tool Registry + executor wiring has a real
python-execution surface to dispatch to.

Spec entry shape::

    - name: calc
      type: python
      config:
        timeout_seconds: 5
        max_output_bytes: 65536
        allow_network: false   # default; W6 policy overrides

LLM-facing schema::

    {
      "type": "object",
      "properties": {
        "code":   {"type": "string"},
        "locals": {"type": "object"}
      },
      "required": ["code"]
    }
"""

from __future__ import annotations

import contextlib
import io
import json
import logging
import signal
import sys
import threading
import time
from collections.abc import Mapping
from dataclasses import dataclass
from typing import Any

from .base import Tool, ToolContext, ToolResult

log = logging.getLogger("hnsx_worker.tools.python")

_DEFAULT_TIMEOUT_SECONDS = 5.0
_DEFAULT_MAX_OUTPUT_BYTES = 65536

# Safe modules that the Python tool pre-imports into the exec namespace.
# W6 will gate these through the policy engine.
_SAFE_MODULES: dict[str, Any] = {
    "json": json,
    "math": __import__("math"),
    "datetime": __import__("datetime"),
    "re": __import__("re"),
    "sys": __import__("sys"),
    "statistics": __import__("statistics"),
    "collections": __import__("collections"),
    "itertools": __import__("itertools"),
}

# Builtins exposed to user code. Stripped:
#   open, exec, eval, compile, input, breakpoint, exit, quit,
#   globals, locals, vars, delattr, setattr — anything that touches
#   fs / process / outer scope.
#
# ``__import__`` is replaced with a restricted variant that only allows
# importing from the safe-module whitelist (see :func:`_safe_import`).
_SAFE_BUILTINS: dict[str, Any] = {
    name: getattr(__import__("builtins"), name)
    for name in (
        "abs", "all", "any", "ascii", "bin", "bool", "bytearray", "bytes",
        "callable", "chr", "complex", "dict", "divmod", "enumerate",
        "filter", "float", "format", "frozenset", "hash", "hex", "id",
        "int", "isinstance", "issubclass", "iter", "len", "list", "map",
        "max", "min", "next", "object", "oct", "ord", "pow", "print",
        "range", "repr", "reversed", "round", "set", "slice", "sorted",
        "str", "sum", "tuple", "type", "zip", "True", "False", "None",
        "NotImplemented", "Ellipsis", "Exception", "ValueError", "TypeError",
        "KeyError", "IndexError", "RuntimeError", "StopIteration",
        "ZeroDivisionError", "ArithmeticError",
    )
}


@dataclass
class PythonToolConfig:
    """Static configuration for one PythonTool instance."""

    timeout_seconds: float = _DEFAULT_TIMEOUT_SECONDS
    max_output_bytes: int = _DEFAULT_MAX_OUTPUT_BYTES
    allow_network: bool = False
    echo_code: bool = False  # for debugging; default off

    @classmethod
    def from_spec(cls, raw: Mapping[str, Any]) -> PythonToolConfig:
        timeout = float(raw.get("timeout_seconds", _DEFAULT_TIMEOUT_SECONDS))
        if timeout <= 0:
            raise ValueError("python tool: timeout_seconds must be > 0")
        max_bytes = int(raw.get("max_output_bytes", _DEFAULT_MAX_OUTPUT_BYTES))
        if max_bytes < 0:
            raise ValueError("python tool: max_output_bytes must be >= 0")
        return cls(
            timeout_seconds=timeout,
            max_output_bytes=max_bytes,
            allow_network=bool(raw.get("allow_network", False)),
            echo_code=bool(raw.get("echo_code", False)),
        )


class PythonTool(Tool):
    """One configured Python execution tool, callable by the LLM by ``name``.

    The instance is constructed by
    :func:`hnsx_worker.tools.factory.build_tool` from a spec entry like
    ``{name, type: 'python', config: {...}}``.
    """

    def __init__(self, name: str, config: PythonToolConfig) -> None:
        self._name = name
        self._config = config

    @property
    def name(self) -> str:
        return self._name

    @property
    def config(self) -> PythonToolConfig:
        return self._config

    @property
    def schema(self) -> dict[str, Any]:
        return {
            "type": "object",
            "properties": {
                "code": {
                    "type": "string",
                    "description": (
                        "Python code to execute. Statements run; the last "
                        "expression's value (if any) is returned."
                    ),
                },
                "locals": {
                    "type": "object",
                    "description": "Variables injected into the exec namespace.",
                },
            },
            "required": ["code"],
            "additionalProperties": False,
        }

    # ------------------------------------------------------------------ invoke

    def invoke(self, ctx: ToolContext, input: dict[str, Any]) -> ToolResult:
        if not isinstance(input, dict):
            return ToolResult(error="input must be a JSON object")
        code = input.get("code")
        if not isinstance(code, str) or not code.strip():
            return ToolResult(error="input.code must be a non-empty string")
        locals_in = input.get("locals") or {}
        if not isinstance(locals_in, dict):
            return ToolResult(error="input.locals must be an object")

        if self._config.echo_code:
            log.debug("python tool %s: %s", self._name, code[:512])

        sandbox = getattr(ctx, "sandbox", None)
        if sandbox is not None and getattr(sandbox, "name", "none") != "none":
            return self._invoke_in_sandbox(ctx, code, locals_in)

        return self._invoke_in_process(code, locals_in)

    def _invoke_in_sandbox(
        self, ctx: ToolContext, code: str, locals_in: dict[str, Any]
    ) -> ToolResult:
        """Delegate execution to the configured sandbox backend."""
        sandbox = ctx.sandbox
        assert sandbox is not None
        payload = json.dumps(
            {
                "code": code,
                "locals": locals_in,
                "config": {
                    "timeout_seconds": self._config.timeout_seconds,
                    "max_output_bytes": self._config.max_output_bytes,
                    "allow_network": self._config.allow_network,
                    "echo_code": self._config.echo_code,
                },
            },
            ensure_ascii=False,
            default=str,
        )
        command = [sys.executable, "-m", "hnsx_worker.tools.python_runner"]
        sandbox_result = sandbox.run(
            command,
            input=payload,
            timeout_seconds=self._config.timeout_seconds,
        )
        if not sandbox_result.ok:
            self._emit_sandbox_violation(ctx, sandbox_result.error or "sandbox failed")
            return ToolResult(
                error=f"python tool: {sandbox_result.error}",
                metadata={
                    "stdout": sandbox_result.stdout,
                    "stderr": sandbox_result.stderr,
                    "sandbox": sandbox.name,
                },
            )
        try:
            data = json.loads(sandbox_result.stdout)
        except json.JSONDecodeError as e:
            return ToolResult(
                error=f"python tool: sandbox output is not valid JSON: {e}",
                metadata={
                    "stdout": sandbox_result.stdout,
                    "stderr": sandbox_result.stderr,
                    "sandbox": sandbox.name,
                },
            )
        if data.get("ok"):
            return ToolResult(
                output=data.get("output"),
                metadata={**data.get("metadata", {}), "sandbox": sandbox.name},
            )
        return ToolResult(
            error=data.get("error", "unknown sandbox error"),
            metadata={**data.get("metadata", {}), "sandbox": sandbox.name},
        )

    def _emit_sandbox_violation(self, ctx: ToolContext, reason: str) -> None:
        if ctx.emit is None:
            return
        ctx.emit(
            {
                "kind": "sandbox_violation",
                "session_id": ctx.session_id,
                "domain_id": ctx.domain_id,
                "agent_id": ctx.agent_id,
                "payload": {"tool": self._name, "reason": reason},
            }
        )

    def _invoke_in_process(self, code: str, locals_in: dict[str, Any]) -> ToolResult:
        """Run the code in the current process (legacy fast path)."""
        try:
            namespace = _build_namespace(
                locals_in, allow_network=self._config.allow_network
            )
        except ValueError as e:
            return ToolResult(error=str(e))
        stdout_buf = io.StringIO()
        stderr_buf = io.StringIO()

        started = time.monotonic()
        timed_out = False
        exc: BaseException | None = None
        result_repr: str | None = None
        try:
            with contextlib.redirect_stdout(stdout_buf), contextlib.redirect_stderr(stderr_buf):
                self._run_with_timeout(code, namespace, self._config.timeout_seconds)
            # Pull the implicit result if the code's last line was an
            # expression (compiled separately as a single-expression eval).
            value = namespace.pop("__hnsx_result__", None)
            if value is not None:
                try:
                    result_repr = repr(value)
                except Exception:  # noqa: BLE001
                    result_repr = "<unrepr-able>"
        except _TimeoutError:
            timed_out = True
        except SyntaxError as e:
            exc = e
        except Exception as e:  # noqa: BLE001 — surface as structured error
            exc = e

        elapsed_ms = int((time.monotonic() - started) * 1000)
        stdout_str, stdout_truncated = _truncate(
            stdout_buf.getvalue(), self._config.max_output_bytes
        )
        stderr_str, stderr_truncated = _truncate(
            stderr_buf.getvalue(), self._config.max_output_bytes
        )

        if timed_out:
            return ToolResult(
                error=f"python tool: execution exceeded {self._config.timeout_seconds}s timeout",
                metadata={
                    "stdout": stdout_str,
                    "stderr": stderr_str,
                    "stdout_truncated": stdout_truncated,
                    "stderr_truncated": stderr_truncated,
                    "elapsed_ms": elapsed_ms,
                    "timed_out": True,
                    "allow_network": self._config.allow_network,
                },
            )
        if exc is not None:
            return ToolResult(
                error=f"python tool: {type(exc).__name__}: {exc}",
                metadata={
                    "stdout": stdout_str,
                    "stderr": stderr_str,
                    "stdout_truncated": stdout_truncated,
                    "stderr_truncated": stderr_truncated,
                    "elapsed_ms": elapsed_ms,
                    "exception_type": type(exc).__name__,
                    "allow_network": self._config.allow_network,
                },
            )
        return ToolResult(
            output={
                "result": result_repr,
                "stdout": stdout_str,
                "stderr": stderr_str,
            },
            metadata={
                "stdout_truncated": stdout_truncated,
                "stderr_truncated": stderr_truncated,
                "elapsed_ms": elapsed_ms,
                "allow_network": self._config.allow_network,
            },
        )

    # ------------------------------------------------------------------ runner

    def _run_with_timeout(self, code: str, namespace: dict, timeout_seconds: float) -> None:
        """Execute ``code`` with a wall-clock timeout.

        Strategy:

          - On POSIX + main thread: ``signal.SIGALRM`` based watchdog.
            Reliable for blocking C extensions / infinite loops.
          - Elsewhere (threads, Windows): fall back to running in a
            worker thread + ``join(timeout)``. CPU-bound code may
            overshoot the deadline; we accept that for W3 and let
            W6 replace with a real subprocess sandbox.
        """
        if threading.current_thread() is threading.main_thread():
            try:
                self._run_with_sigarm(code, namespace, timeout_seconds)
                return
            except (ValueError, OSError):
                # Signal-based watchdog not available here; fall through.
                pass
        self._run_with_thread(code, namespace, timeout_seconds)

    def _run_with_sigarm(self, code: str, namespace: dict, timeout_seconds: float) -> None:
        def _on_alarm(signum: int, frame: Any) -> None:
            raise _TimeoutError(f"execution exceeded {timeout_seconds}s")

        old_handler = signal.signal(signal.SIGALRM, _on_alarm)
        # signal.setitimer is more precise than alarm(); prefer it when
        # available. ``alarm()`` only accepts integer seconds.
        if hasattr(signal, "setitimer"):
            signal.setitimer(signal.ITIMER_REAL, timeout_seconds)
        else:
            signal.alarm(max(1, int(timeout_seconds)))
        try:
            _execute_code(code, namespace)
        finally:
            if hasattr(signal, "setitimer"):
                signal.setitimer(signal.ITIMER_REAL, 0)
            else:
                signal.alarm(0)
            signal.signal(signal.SIGALRM, old_handler)

    def _run_with_thread(self, code: str, namespace: dict, timeout_seconds: float) -> None:
        err: list[BaseException] = []

        def _target() -> None:
            try:
                _execute_code(code, namespace)
            except BaseException as e:  # noqa: BLE001
                err.append(e)

        t = threading.Thread(target=_target, daemon=True)
        t.start()
        t.join(timeout_seconds)
        if t.is_alive():
            raise _TimeoutError(f"execution exceeded {timeout_seconds}s")
        if err:
            raise err[0]


# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------


class _TimeoutError(Exception):
    """Raised internally when the watchdog trips."""


def _build_namespace(locals_in: Mapping[str, Any], *, allow_network: bool) -> dict:
    """Construct the restricted exec namespace for one invocation."""
    namespace: dict[str, Any] = {
        "__builtins__": _SAFE_BUILTINS,
        "__name__": "__hnsx_python__",
    }
    # Wire up a restricted __import__ that only allows the safe modules.
    builtins_dict = dict(_SAFE_BUILTINS)
    builtins_dict["__import__"] = _make_safe_importer()
    namespace["__builtins__"] = builtins_dict
    namespace.update(_SAFE_MODULES)
    # Locals are filtered through ``json.dumps`` so non-serializable
    # values (sets, custom objects, functions) are rejected before they
    # reach the exec namespace.
    try:
        sanitized = json.loads(json.dumps(locals_in))
    except (TypeError, ValueError) as e:
        raise ValueError(
            f"locals contains non-JSON-serializable values: {e!s}"
        ) from e
    if not isinstance(sanitized, dict):
        raise ValueError("locals must be an object")
    namespace.update(sanitized)
    if not allow_network:
        # Pre-bind ``socket`` / ``urllib`` to raise on access. This is
        # advisory only — a determined agent can re-import. W6 swaps in
        # a real sandbox.
        namespace["socket"] = _BlockedModule("socket")
        namespace["urllib"] = _BlockedModule("urllib")
        namespace["http"] = _BlockedModule("http")
        namespace["requests"] = _BlockedModule("requests")
    return namespace


class _BlockedModule:
    """A placeholder that raises if any attribute is accessed."""

    def __init__(self, name: str) -> None:
        self._name = name

    def __getattr__(self, attr: str) -> Any:
        raise RuntimeError(
            f"python tool: {self._name}.{attr} is blocked "
            f"(network access disabled; set config.allow_network=true to enable)"
        )


def _make_safe_importer():
    """Return a callable suitable for ``__builtins__['__import__']``.

    Only the modules listed in ``_SAFE_MODULES`` (plus their submodules)
    are importable; anything else raises ``ImportError``. This is
    advisory — a determined user can still reach the real import via
    introspection. W6 swaps in a real sandbox.
    """

    def _safe_import(name, globals=None, locals=None, fromlist=(), level=0):
        top = name.split(".", 1)[0]
        if top not in _SAFE_MODULES and top != "__future__":
            raise ImportError(
                f"python tool: import of {name!r} is not allowed "
                f"(safe modules: {sorted(_SAFE_MODULES)})"
            )
        return __import__(name, globals, locals, fromlist, level)

    return _safe_import


def _execute_code(code: str, namespace: dict) -> None:
    """Run ``code`` in ``namespace``.

    If the *last line* is an expression, capture its value into
    ``__hnsx_result__`` so the tool can return it. This mirrors the
    behaviour of a REPL cell.
    """
    # Single-expression snippet: try eval first.
    try:
        eval_compiled = compile(code, "<hnsx_python>", "eval")
        namespace["__hnsx_result__"] = eval(eval_compiled, namespace)
        return
    except SyntaxError:
        pass

    try:
        compiled = compile(code, "<hnsx_python>", "exec")
    except SyntaxError:
        # Couldn't compile as exec — surface the syntax error.
        raise

    # Multi-line: if the last non-empty line is a plain expression,
    # split it off so we capture its value.
    stripped = code.rstrip()
    last_line = stripped.rsplit("\n", 1)[-1].strip()
    if last_line and not last_line.startswith(
        (
            "def ", "class ", "if ", "for ", "while ", "try", "with ", "@",
            "return", "import ", "from ", "raise", "pass", "break",
            "continue", "del ", "global ", "nonlocal ", "assert ", "#",
        )
    ):
        try:
            compile(last_line, "<hnsx_python_last>", "eval")
            head, _, tail = stripped.rpartition("\n")
            if tail.strip() == last_line and head.strip():
                exec(compile(head, "<hnsx_python>", "exec"), namespace)
                namespace["__hnsx_result__"] = eval(
                    compile(last_line, "<hnsx_python_last>", "eval"),
                    namespace,
                )
                return
        except SyntaxError:
            pass

    exec(compiled, namespace)


def _truncate(s: str, max_bytes: int) -> tuple[str, bool]:
    if max_bytes <= 0 or len(s) <= max_bytes:
        return s, False
    return s[:max_bytes], True


__all__ = ["PythonTool", "PythonToolConfig"]
