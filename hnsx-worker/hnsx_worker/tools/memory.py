"""Memory tools — let agents read/write long-term context.

Three built-in tools are auto-injected when ``harness.memory`` is configured:

  - ``memory_store`` — save a fact / preference / task_state.
  - ``memory_search`` — retrieve relevant memories by keyword.
  - ``memory_forget`` — delete a memory by key.

Every write emits a ``memory_write`` observation; every read emits a
``memory_read`` observation so the Harness UI can show what the agent
remembered.
"""

from __future__ import annotations

from typing import Any

from hnsx_worker.memory import MemoryItem, MemoryStore

from .base import Tool, ToolContext, ToolResult


class _MemoryTool(Tool):
    """Base class for memory tools."""

    def __init__(self, name: str, store: MemoryStore) -> None:
        self._name = name
        self._store = store

    @property
    def name(self) -> str:
        return self._name


class MemoryStoreTool(_MemoryTool):
    """Save a memory item."""

    @property
    def schema(self) -> dict[str, Any]:
        return {
            "type": "object",
            "properties": {
                "key": {
                    "type": "string",
                    "description": (
                        "Optional unique key. A new value overwrites any existing "
                        "item with the same key."
                    ),
                },
                "value": {
                    "type": "string",
                    "description": "The content to remember.",
                },
                "kind": {
                    "type": "string",
                    "enum": ["fact", "preference", "task_state", "summary"],
                    "description": "Kind of memory (default: fact).",
                },
                "ttl_seconds": {
                    "type": "integer",
                    "description": "Time-to-live in seconds. 0 means no expiration.",
                },
            },
            "required": ["value"],
            "additionalProperties": False,
        }

    def invoke(self, ctx: ToolContext, input: dict[str, Any]) -> ToolResult:
        if not isinstance(input, dict):
            return ToolResult(error="memory_store input must be a JSON object")
        value = input.get("value")
        if value is None or value == "":
            return ToolResult(error="memory_store requires a non-empty 'value'")
        item = MemoryItem(
            session_id=ctx.session_id,
            key=str(input.get("key", "")),
            kind=str(input.get("kind", "fact")),
            content=value,
            ttl_seconds=int(input.get("ttl_seconds", 0) or 0),
        )
        self._store.add(item)
        _emit_memory_write(ctx, item, operation="store")
        return ToolResult(
            output={"stored": True, "id": item.id},
            metadata={"kind": item.kind, "key": item.key},
        )


class MemorySearchTool(_MemoryTool):
    """Search stored memories."""

    @property
    def schema(self) -> dict[str, Any]:
        return {
            "type": "object",
            "properties": {
                "query": {
                    "type": "string",
                    "description": "Keywords to search for.",
                },
                "top_k": {
                    "type": "integer",
                    "description": "Maximum number of results (default: 5).",
                },
            },
            "required": ["query"],
            "additionalProperties": False,
        }

    def invoke(self, ctx: ToolContext, input: dict[str, Any]) -> ToolResult:
        if not isinstance(input, dict):
            return ToolResult(error="memory_search input must be a JSON object")
        query = input.get("query")
        if not query:
            return ToolResult(error="memory_search requires a non-empty 'query'")
        top_k = int(input.get("top_k", 5))
        items = self._store.search(
            str(query),
            session_id=ctx.session_id,
            top_k=max(1, top_k),
        )
        _emit_memory_read(ctx, query, items, operation="search")
        return ToolResult(
            output={
                "results": [
                    {
                        "id": item.id,
                        "kind": item.kind,
                        "key": item.key,
                        "content": item.content,
                    }
                    for item in items
                ]
            },
            metadata={"count": len(items)},
        )


class MemoryForgetTool(_MemoryTool):
    """Delete a memory by key."""

    @property
    def schema(self) -> dict[str, Any]:
        return {
            "type": "object",
            "properties": {
                "key": {
                    "type": "string",
                    "description": "Key of the memory to delete.",
                },
            },
            "required": ["key"],
            "additionalProperties": False,
        }

    def invoke(self, ctx: ToolContext, input: dict[str, Any]) -> ToolResult:
        if not isinstance(input, dict):
            return ToolResult(error="memory_forget input must be a JSON object")
        key = input.get("key")
        if not key:
            return ToolResult(error="memory_forget requires a non-empty 'key'")
        deleted = self._store.delete_by_key(str(key), session_id=ctx.session_id)
        _emit_memory_write(
            ctx,
            MemoryItem(session_id=ctx.session_id, key=str(key), content=""),
            operation="forget",
        )
        return ToolResult(
            output={"deleted": deleted},
            metadata={"key": str(key)},
        )


def _emit_memory_write(
    ctx: ToolContext,
    item: MemoryItem,
    *,
    operation: str,
) -> None:
    if ctx.emit is None:
        return
    ctx.emit(
        {
            "kind": "memory_write",
            "session_id": ctx.session_id,
            "domain_id": ctx.domain_id,
            "agent_id": ctx.agent_id,
            "payload": {
                "tool_call_id": ctx.tool_call_id,
                "operation": operation,
                "id": item.id,
                "kind": item.kind,
                "key": item.key,
                "turn": ctx.turn,
            },
        }
    )


def _emit_memory_read(
    ctx: ToolContext,
    query: str,
    items: list[MemoryItem],
    *,
    operation: str,
) -> None:
    if ctx.emit is None:
        return
    ctx.emit(
        {
            "kind": "memory_read",
            "session_id": ctx.session_id,
            "domain_id": ctx.domain_id,
            "agent_id": ctx.agent_id,
            "payload": {
                "tool_call_id": ctx.tool_call_id,
                "operation": operation,
                "query": query,
                "count": len(items),
                "ids": [item.id for item in items],
                "turn": ctx.turn,
            },
        }
    )


__all__ = [
    "MemoryForgetTool",
    "MemorySearchTool",
    "MemoryStoreTool",
]
