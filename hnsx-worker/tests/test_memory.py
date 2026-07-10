"""Tests for long-term memory (W11)."""

from __future__ import annotations

from typing import Any

import pytest

from hnsx_worker.memory import MemoryItem, MemoryStore, build_backend
from hnsx_worker.memory.in_memory import InMemoryMemoryStore
from hnsx_worker.memory.sqlite import SqliteMemoryStore
from hnsx_worker.tools import ToolContext
from hnsx_worker.tools.memory import (
    MemoryForgetTool,
    MemorySearchTool,
    MemoryStoreTool,
)

# ---------------------------------------------------------------------------
# Backend tests
# ---------------------------------------------------------------------------


@pytest.fixture(params=["in_memory", "sqlite"])
def store(request: pytest.FixtureRequest) -> MemoryStore:
    if request.param == "in_memory":
        return InMemoryMemoryStore()
    return SqliteMemoryStore(":memory:")


def test_memory_add_and_search(store: MemoryStore) -> None:
    item = MemoryItem(
        session_id="s1",
        key="color",
        kind="preference",
        content="user likes blue",
    )
    store.add(item)
    results = store.search("blue", session_id="s1")
    assert len(results) == 1
    assert results[0].content == "user likes blue"


def test_memory_search_filters_by_session(store: MemoryStore) -> None:
    store.add(MemoryItem(session_id="s1", content="alpha"))
    store.add(MemoryItem(session_id="s2", content="beta"))
    assert len(store.search("alpha", session_id="s1")) == 1
    assert len(store.search("alpha", session_id="s2")) == 0


def test_memory_key_overwrite(store: MemoryStore) -> None:
    store.add(MemoryItem(session_id="s1", key="color", content="blue"))
    store.add(MemoryItem(session_id="s1", key="color", content="green"))
    results = store.search("color", session_id="s1")
    assert len(results) == 1
    assert results[0].content == "green"


def test_memory_delete_by_key(store: MemoryStore) -> None:
    store.add(MemoryItem(session_id="s1", key="tmp", content="x"))
    assert store.delete_by_key("tmp", session_id="s1")
    assert not store.delete_by_key("tmp", session_id="s1")
    assert len(store.search("x", session_id="s1")) == 0


def test_memory_get_recent(store: MemoryStore) -> None:
    store.add(MemoryItem(session_id="s1", content="a"))
    store.add(MemoryItem(session_id="s1", content="b"))
    recent = store.get_recent(2, session_id="s1")
    assert [r.content for r in recent] == ["b", "a"]


# ---------------------------------------------------------------------------
# Tool tests
# ---------------------------------------------------------------------------


def test_memory_store_tool() -> None:
    store = InMemoryMemoryStore()
    tool = MemoryStoreTool("memory_store", store)
    ctx = ToolContext(session_id="s1")
    result = tool.invoke(ctx, {"value": "important fact"})
    assert result.ok
    assert len(store.search("important", session_id="s1")) == 1


def test_memory_search_tool() -> None:
    store = InMemoryMemoryStore()
    store.add(MemoryItem(session_id="s1", content="the user prefers email"))
    tool = MemorySearchTool("memory_search", store)
    ctx = ToolContext(session_id="s1")
    result = tool.invoke(ctx, {"query": "email"})
    assert result.ok
    assert len(result.output["results"]) == 1


def test_memory_forget_tool() -> None:
    store = InMemoryMemoryStore()
    store.add(MemoryItem(session_id="s1", key="pref", content="x"))
    tool = MemoryForgetTool("memory_forget", store)
    ctx = ToolContext(session_id="s1")
    result = tool.invoke(ctx, {"key": "pref"})
    assert result.ok
    assert result.output["deleted"]
    assert len(store.search("x", session_id="s1")) == 0


def test_memory_tools_emit_observations() -> None:
    store = InMemoryMemoryStore()
    observations: list[dict[str, Any]] = []
    ctx = ToolContext(
        session_id="s1",
        domain_id="d1",
        agent_id="a1",
        tool_call_id="tc1",
        emit=lambda o: observations.append(o),
    )
    MemoryStoreTool("memory_store", store).invoke(ctx, {"value": "v"})
    MemorySearchTool("memory_search", store).invoke(ctx, {"query": "v"})
    MemoryForgetTool("memory_forget", store).invoke(ctx, {"key": "k"})

    kinds = [o["kind"] for o in observations]
    assert kinds.count("memory_write") == 2  # store + forget
    assert kinds.count("memory_read") == 1


# ---------------------------------------------------------------------------
# Registry / executor integration
# ---------------------------------------------------------------------------


def test_tool_registry_auto_injects_memory_tools() -> None:
    from hnsx_worker.session_executor import _build_tool_registry

    spec = {
        "id": "mem-test",
        "harness": {
            "memory": {"backend": "in_memory"},
            "agents": {
                "agent": {
                    "id": "agent",
                    "tools": [
                        {"name": "fetch", "type": "http", "config": {"url": "https://x"}}
                    ],
                }
            },
        },
    }
    agent = spec["harness"]["agents"]["agent"]
    registry, failures = _build_tool_registry(
        spec=spec,
        agent=agent,
        session_id="s1",
        domain_id="mem-test",
        emit=lambda o: None,
        memory=InMemoryMemoryStore(),
    )
    assert failures == []
    assert "memory_store" in registry
    assert "memory_search" in registry
    assert "memory_forget" in registry
    tool_names = {t["name"] for t in agent["tools"]}
    assert {"fetch", "memory_store", "memory_search", "memory_forget"} <= tool_names


# ---------------------------------------------------------------------------
# Domain config
# ---------------------------------------------------------------------------


def test_build_backend_sqlite() -> None:
    backend = build_backend({"backend": "sqlite", "path": ":memory:"})
    assert isinstance(backend, SqliteMemoryStore)


def test_build_backend_unknown_falls_back_to_sqlite() -> None:
    backend = build_backend({"backend": "weaviate"})
    assert isinstance(backend, SqliteMemoryStore)


def test_build_backend_rejects_bad_kind() -> None:
    with pytest.raises(ValueError, match="unknown memory backend"):
        build_backend({"backend": "bad"})
