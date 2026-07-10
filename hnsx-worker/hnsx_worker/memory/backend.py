"""Memory backend factory."""

from __future__ import annotations

import logging
from typing import Any

from .base import MemoryStore
from .in_memory import InMemoryMemoryStore
from .sqlite import SqliteMemoryStore

log = logging.getLogger("hnsx_worker.memory.backend")


def build_backend(config: dict[str, Any] | None) -> MemoryStore:
    """Build a memory backend from the DomainSpec ``harness.memory`` block."""
    cfg = config or {}
    kind = str(cfg.get("backend") or "sqlite")
    if kind == "sqlite":
        path = str(cfg.get("path") or ":memory:")
        return SqliteMemoryStore(path=path)
    if kind == "in_memory":
        return InMemoryMemoryStore()
    if kind in ("chroma", "mem0", "weaviate"):
        log.warning(
            "memory backend %r is not bundled in W11; falling back to sqlite", kind
        )
        return SqliteMemoryStore(path=str(cfg.get("path") or ":memory:"))
    raise ValueError(f"unknown memory backend: {kind!r}")


__all__ = ["MemoryStore", "build_backend"]
