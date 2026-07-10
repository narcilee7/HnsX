"""Tests for W6 — store backends.
"""

from __future__ import annotations

import pytest

from hnsx_worker.store import InMemoryStore, build_backend


def test_in_memory_store_get_set_delete() -> None:
    store = InMemoryStore()
    store.set("s1", "messages", [{"role": "user"}])
    assert store.get("s1", "messages") == [{"role": "user"}]
    store.delete("s1", "messages")
    assert store.get("s1", "messages") is None


def test_in_memory_store_isolation() -> None:
    store = InMemoryStore()
    store.set("s1", "key", "a")
    store.set("s2", "key", "b")
    assert store.get("s1", "key") == "a"
    assert store.get("s2", "key") == "b"


def test_build_backend_defaults_to_in_memory() -> None:
    backend = build_backend(None)
    assert backend.name == "in_memory"


def test_build_backend_postgres() -> None:
    backend = build_backend({"backend": "postgres", "dsn": "postgresql://x"})
    assert backend.name == "postgres"


def test_build_backend_redis() -> None:
    backend = build_backend({"backend": "redis", "url": "redis://x"})
    assert backend.name == "redis"


def test_build_backend_unknown_raises() -> None:
    with pytest.raises(ValueError, match="unknown store backend"):
        build_backend({"backend": "s3"})
