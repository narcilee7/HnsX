"""SessionRuntime — the subprocess entry.

Each session runs in its own Python subprocess (Ray-style actor). The
parent process (``worker_service.py``) spawns one of these per session and
exchanges messages via:

  - stdin:  one JSON document with the session config (read once at start).
  - stdout: one JSON observation per line (JSONL).
  - SIGTERM: graceful stop.

This module's ``main()`` is invoked as ``python -m hnsx_worker.session_runtime``.

Custom adapters can be registered before ``main()`` runs by setting
``HNSX_ADAPTER_MODULES`` to a comma-separated list of fully-qualified
module names. Each module is imported; if it defines ``register(registry)``
we call it with the AdapterRegistry instance. (Tests use this hook to
register a slow stub adapter without modifying the package.)
"""

from __future__ import annotations

import importlib
import json
import os
import signal
import sys
import threading
import time
from typing import Any

from hnsx_worker.session_executor import _Stopped, execute_session


def emit(obs: dict[str, Any]) -> None:
    """Write one observation as a JSON line on stdout, flushed."""
    obs.setdefault("created_at_ms", _now_ms())
    sys.stdout.write(json.dumps(obs, default=str) + "\n")
    sys.stdout.flush()


def _now_ms() -> int:
    return int(time.time() * 1000)


def _load_extra_adapters() -> None:
    """Import any modules listed in ``HNSX_ADAPTER_MODULES`` and let them register."""
    from hnsx_worker.adapters import AdapterRegistry

    spec = os.environ.get("HNSX_ADAPTER_MODULES", "")
    for name in (m.strip() for m in spec.split(",") if m.strip()):
        try:
            mod = importlib.import_module(name)
        except Exception as e:  # noqa: BLE001 — surface any plugin load failure
            sys.stderr.write(f"session_runtime: failed to import {name!r}: {e}\n")
            continue
        register = getattr(mod, "register", None)
        if callable(register):
            register(AdapterRegistry)


def main() -> int:
    """Read config from stdin, run the session, return exit code."""
    _load_extra_adapters()
    raw = sys.stdin.read()
    if not raw.strip():
        sys.stderr.write("session_runtime: empty stdin\n")
        return 2
    try:
        config = json.loads(raw)
    except json.JSONDecodeError as e:
        sys.stderr.write(f"session_runtime: invalid JSON on stdin: {e}\n")
        return 2

    stop_event = threading.Event()
    try:
        signal.signal(signal.SIGTERM, lambda *_: stop_event.set())
    except ValueError:
        # Not in main thread (shouldn't happen for `python -m ...` but be safe).
        pass

    session_id = config.get("session_id", "")
    domain_id = ""
    spec: dict[str, Any] = {}
    try:
        spec = json.loads(config.get("domain_spec_json") or "{}")
        domain_id = spec.get("id", "")
    except json.JSONDecodeError as e:
        sys.stderr.write(f"session_runtime: invalid domain_spec_json: {e}\n")
        return 2

    trigger_raw = config.get("trigger_payload_json") or "{}"
    try:
        trigger = json.loads(trigger_raw)
    except json.JSONDecodeError as e:
        sys.stderr.write(f"session_runtime: invalid trigger_payload_json: {e}\n")
        return 2

    emit(
        {
            "kind": "session_start",
            "session_id": session_id,
            "domain_id": domain_id,
            "payload": {"trigger_keys": sorted(trigger.keys()) if isinstance(trigger, dict) else []},
        }
    )

    rc = 0
    try:
        execute_session(spec, trigger, config, stop_event=stop_event, emit=emit)
        emit(
            {
                "kind": "session_end",
                "session_id": session_id,
                "domain_id": domain_id,
                "state": "completed",
            }
        )
    except _Stopped:
        emit(
            {
                "kind": "session_end",
                "session_id": session_id,
                "domain_id": domain_id,
                "state": "cancelled",
            }
        )
        rc = 130  # conventional exit code for SIGTERM
    except Exception as e:  # noqa: BLE001 — we want to surface everything
        emit(
            {
                "kind": "session_end",
                "session_id": session_id,
                "domain_id": domain_id,
                "state": "failed",
                "payload": {"error": str(e), "error_type": type(e).__name__},
            }
        )
        rc = 1
    return rc


if __name__ == "__main__":
    sys.exit(main())
