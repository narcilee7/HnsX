"""Store backends for cross-turn context persistence (W6).

The store keeps ``messages`` / ``state`` across turns and, in future versions,
across subprocess restarts. W6 ships three backends:

  - ``in_memory``: default, lives for the lifetime of the subprocess.
  - ``postgres``: persistent, cross-subprocess context + knowledge.
  - ``redis``: ephemeral, fast cross-turn cache.

Spec shape::

    harness:
      store:
        backend: in_memory   # or postgres / redis
        # postgres / redis-specific config
"""

from __future__ import annotations

import logging
from abc import ABC, abstractmethod
from typing import Any

log = logging.getLogger("hnsx_worker.store.backend")


class StoreBackend(ABC):
    """Abstract context store."""

    @property
    @abstractmethod
    def name(self) -> str:
        """Backend identifier."""

    @abstractmethod
    def get(self, session_id: str, key: str, default: Any = None) -> Any:
        """Read a value for ``session_id`` / ``key``."""

    @abstractmethod
    def set(
        self,
        session_id: str,
        key: str,
        value: Any,
        *,
        ttl_seconds: int = 0,
    ) -> None:
        """Write a value for ``session_id`` / ``key``."""

    @abstractmethod
    def delete(self, session_id: str, key: str) -> None:
        """Delete a key."""


class InMemoryStore(StoreBackend):
    """Default in-process store."""

    def __init__(self) -> None:
        self._data: dict[str, dict[str, Any]] = {}

    @property
    def name(self) -> str:
        return "in_memory"

    def get(self, session_id: str, key: str, default: Any = None) -> Any:
        return self._data.get(session_id, {}).get(key, default)

    def set(
        self,
        session_id: str,
        key: str,
        value: Any,
        *,
        ttl_seconds: int = 0,
    ) -> None:
        self._data.setdefault(session_id, {})[key] = value

    def delete(self, session_id: str, key: str) -> None:
        self._data.get(session_id, {}).pop(key, None)


class PostgresStore(StoreBackend):
    """Postgres-backed persistent store (W6 stub — schema to be finalized)."""

    def __init__(self, dsn: str) -> None:
        self.dsn = dsn

    @property
    def name(self) -> str:
        return "postgres"

    def get(self, session_id: str, key: str, default: Any = None) -> Any:
        log.debug("postgres store get %s/%s", session_id, key)
        return default

    def set(
        self,
        session_id: str,
        key: str,
        value: Any,
        *,
        ttl_seconds: int = 0,
    ) -> None:
        log.debug("postgres store set %s/%s", session_id, key)

    def delete(self, session_id: str, key: str) -> None:
        log.debug("postgres store delete %s/%s", session_id, key)


class RedisStore(StoreBackend):
    """Redis-backed ephemeral store (W6 stub — requires redis client)."""

    def __init__(self, url: str) -> None:
        self.url = url

    @property
    def name(self) -> str:
        return "redis"

    def get(self, session_id: str, key: str, default: Any = None) -> Any:
        log.debug("redis store get %s/%s", session_id, key)
        return default

    def set(
        self,
        session_id: str,
        key: str,
        value: Any,
        *,
        ttl_seconds: int = 0,
    ) -> None:
        log.debug("redis store set %s/%s", session_id, key)

    def delete(self, session_id: str, key: str) -> None:
        log.debug("redis store delete %s/%s", session_id, key)


def build_backend(config: dict[str, Any] | None) -> StoreBackend:
    """Build a store backend from the DomainSpec ``harness.store`` block."""
    cfg = config or {}
    kind = str(cfg.get("backend") or "in_memory")
    if kind == "in_memory":
        return InMemoryStore()
    if kind == "postgres":
        return PostgresStore(dsn=str(cfg.get("dsn") or ""))
    if kind == "redis":
        return RedisStore(url=str(cfg.get("url") or ""))
    raise ValueError(f"unknown store backend: {kind!r}")


__all__ = [
    "StoreBackend",
    "InMemoryStore",
    "PostgresStore",
    "RedisStore",
    "build_backend",
]
