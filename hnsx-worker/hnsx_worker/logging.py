"""Structured JSON logging + request-context helpers for W9.

Provides a JSON formatter that carries ``worker_id``, ``session_id``,
``trace_id`` and ``correlation_id`` on every log record. Context is read
from :mod:`contextvars` so per-session subprocesses can bind their IDs once
at startup and have them appear on all subsequent logs.
"""

from __future__ import annotations

import contextvars
import json
import logging
from typing import Any

worker_id_var: contextvars.ContextVar[str] = contextvars.ContextVar(
    "worker_id", default=""
)
session_id_var: contextvars.ContextVar[str] = contextvars.ContextVar(
    "session_id", default=""
)
trace_id_var: contextvars.ContextVar[str] = contextvars.ContextVar(
    "trace_id", default=""
)
correlation_id_var: contextvars.ContextVar[str] = contextvars.ContextVar(
    "correlation_id", default=""
)


class ContextFilter(logging.Filter):
    """Copy current context IDs into every LogRecord."""

    def filter(self, record: logging.LogRecord) -> bool:
        record.worker_id = worker_id_var.get()
        record.session_id = session_id_var.get()
        record.trace_id = trace_id_var.get()
        record.correlation_id = correlation_id_var.get()
        return True


class JSONFormatter(logging.Formatter):
    """Emit log records as single-line JSON."""

    def format(self, record: logging.LogRecord) -> str:
        obj: dict[str, Any] = {
            "timestamp": self.formatTime(record),
            "level": record.levelname,
            "logger": record.name,
            "message": record.getMessage(),
            "worker_id": getattr(record, "worker_id", ""),
            "session_id": getattr(record, "session_id", ""),
            "trace_id": getattr(record, "trace_id", ""),
            "correlation_id": getattr(record, "correlation_id", ""),
        }
        if record.exc_info:
            obj["exception"] = self.formatException(record.exc_info)
        return json.dumps(obj, default=str)


def configure_logging(level: int = logging.INFO) -> None:
    """Configure the root logger with JSON output and context filtering."""
    handler = logging.StreamHandler()
    handler.setFormatter(JSONFormatter())
    handler.addFilter(ContextFilter())

    root = logging.getLogger()
    root.handlers[:] = []
    root.addHandler(handler)
    root.setLevel(level)
