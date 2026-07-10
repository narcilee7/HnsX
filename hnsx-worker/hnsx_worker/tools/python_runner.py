"""Sandboxed Python tool runner.

Reads a JSON payload from stdin, executes the code with the same restricted
namespace used by :class:`hnsx_worker.tools.python.PythonTool`, and writes the
result as JSON to stdout.

Used by ``PythonTool`` when a real sandbox backend (process / container) is
configured, so the user code runs in a separate process instead of the session
runtime.
"""

from __future__ import annotations

import json
import sys

from hnsx_worker.tools.base import ToolContext
from hnsx_worker.tools.python import PythonTool, PythonToolConfig


def main() -> int:
    raw = sys.stdin.read()
    if not raw.strip():
        sys.stderr.write("python_runner: empty stdin\n")
        return 2
    try:
        payload = json.loads(raw)
    except json.JSONDecodeError as e:
        sys.stderr.write(f"python_runner: invalid JSON: {e}\n")
        return 2

    code = payload.get("code", "")
    locals_in = payload.get("locals") or {}
    config = PythonToolConfig.from_spec(payload.get("config") or {})
    tool = PythonTool("sandbox-python", config)
    result = tool.invoke(ToolContext(), {"code": code, "locals": locals_in})
    sys.stdout.write(json.dumps(result.to_observation_payload(), default=str) + "\n")
    return 0


if __name__ == "__main__":
    sys.exit(main())
