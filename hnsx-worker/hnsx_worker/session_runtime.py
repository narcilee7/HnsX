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
from typing import Any, Callable

from hnsx_worker.logging import (
    correlation_id_var,
    session_id_var,
    trace_id_var,
)
from hnsx_worker.proto_client.client import ControlPlaneClient
from hnsx_worker.session_executor import _Stopped, execute_session


def _build_approval_bus(emit_fn: Callable[[dict[str, Any]], None]) -> Any | None:
    """Create a server-backed approval bus when the worker knows the server URL.

    Priority:
      1. ``HNSX_SERVER_HTTP_URL`` — explicit HTTP base URL.
      2. ``HNSX_SERVER`` — gRPC address; derive host and assume HTTP port 50051.
    """
    from hnsx_worker.approval import ServerApprovalBus

    base_url = os.environ.get("HNSX_SERVER_HTTP_URL")
    if not base_url:
        grpc_addr = os.environ.get("HNSX_SERVER")
        if grpc_addr:
            host = grpc_addr.rsplit(":", 1)[0]
            base_url = f"http://{host}:50051"
    if not base_url:
        return None
    return ServerApprovalBus(base_url=base_url, emit=emit_fn)


def emit(obs: dict[str, Any]) -> None:
    """Write one observation as a JSON line on stdout, flushed."""
    obs.setdefault("trace_id", trace_id_var.get())
    obs.setdefault("created_at_ms", _now_ms())
    sys.stdout.write(json.dumps(obs, default=str) + "\n")
    sys.stdout.flush()


def _now_ms() -> int:
    return int(time.time() * 1000)


def _load_secrets_from_env() -> dict[str, str]:
    """Read secrets forwarded as ``HNSX_SECRET_*`` environment variables.

    The key name is derived from the env var by stripping the prefix and
    lower-casing: ``HNSX_SECRET_API_KEY`` → ``api_key``.
    """
    prefix = "HNSX_SECRET_"
    out: dict[str, str] = {}
    for key, value in os.environ.items():
        if key.startswith(prefix):
            name = key[len(prefix) :].lower()
            if name:
                out[name] = value
    return out


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


def _merge_secrets_into_config(config: dict[str, Any]) -> None:
    """Merge env-var secrets into ``config['secrets']`` without overwriting."""
    env_secrets = _load_secrets_from_env()
    if not env_secrets:
        return
    existing = config.get("secrets") or {}
    if isinstance(existing, dict):
        merged = dict(env_secrets)
        merged.update(existing)
        config["secrets"] = merged
    elif isinstance(existing, list):
        # List-of-pairs form: append env secrets that aren't already present.
        names = {str(item.get("name")) for item in existing if isinstance(item, dict)}
        for name, value in env_secrets.items():
            if name not in names:
                existing.append({"name": name, "value": value})
    else:
        config["secrets"] = env_secrets


def _validate_spec_via_server(spec: dict[str, Any]) -> tuple[bool, str]:
    """Ask the Go server to authoritatively validate ``spec``.

    Returns ``(True, "")`` when the server says the spec is valid, or
    ``(False, error_message)`` when it is invalid or unreachable.

    If ``HNSX_SERVER`` is not set we skip the RPC and print a warning on
    stderr. This keeps standalone unit tests working; production workers
    are always launched with the server address.
    """
    server_addr = os.environ.get("HNSX_SERVER")
    if not server_addr:
        sys.stderr.write(
            "session_runtime: HNSX_SERVER not set; skipping server-side "
            "DomainSpec validation\n"
        )
        return True, ""

    try:
        spec_json = json.dumps(spec)
    except TypeError as exc:
        return False, f"failed to serialize spec to JSON: {exc}"

    client = ControlPlaneClient(server_addr)
    try:
        valid, errors = client.validate_domain(spec_json)
    finally:
        client.close()

    if valid:
        return True, ""
    return False, "DomainSpec validation failed:\n" + "\n".join(errors)


def main() -> int:
    """Read config from stdin, run the session, return exit code."""
    _load_extra_adapters()
    raw = sys.stdin.read()
    if not raw.strip():
        sys.stderr.write("session_runtime: empty stdin\n")
        return 2
    try:
        config = json.loads(raw)
        _merge_secrets_into_config(config)
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
    trace_id_var.set(config.get("trace_id", ""))
    correlation_id_var.set(config.get("correlation_id", ""))
    session_id_var.set(session_id)
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

    valid, err = _validate_spec_via_server(spec)
    if not valid:
        sys.stderr.write(f"session_runtime: {err}\n")
        emit(
            {
                "kind": "session_end",
                "session_id": session_id,
                "domain_id": domain_id,
                "state": "failed",
                "payload": {"error": err, "error_type": "DomainValidationError"},
            }
        )
        return 1

    emit(
        {
            "kind": "session_start",
            "session_id": session_id,
            "domain_id": domain_id,
            "payload": {
                "trigger_keys": sorted(trigger.keys()) if isinstance(trigger, dict) else []
            },
        }
    )

    rc = 0
    try:
        # W7: schedule a self-termination if the server gave us a hard cap.
        timeout_seconds = config.get("session_timeout_seconds")
        timer: threading.Timer | None = None
        if isinstance(timeout_seconds, (int, float)) and timeout_seconds > 0:
            timer = threading.Timer(
                float(timeout_seconds), _timeout_self, args=(session_id, stop_event)
            )
            timer.daemon = True
            timer.start()

        start = time.monotonic()
        approval_bus = _build_approval_bus(emit)
        result = execute_session(
            spec,
            trigger,
            config,
            stop_event=stop_event,
            emit=emit,
            approval_bus=approval_bus,
        )
        end_payload: dict[str, Any] = {}
        if isinstance(result, dict):
            result["duration_ms"] = int((time.monotonic() - start) * 1000)
            end_payload["result"] = result
        emit(
            {
                "kind": "session_end",
                "session_id": session_id,
                "domain_id": domain_id,
                "state": "completed",
                "payload": end_payload,
            }
        )
        if timer is not None:
            timer.cancel()
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


def _timeout_self(session_id: str, stop_event: threading.Event) -> None:
    """W7: ask the session to stop because it hit session_timeout_seconds."""
    sys.stderr.write(f"session_runtime: session {session_id} timed out\n")
    stop_event.set()
    try:
        os.kill(os.getpid(), signal.SIGTERM)
    except OSError:
        pass


if __name__ == "__main__":
    sys.exit(main())
