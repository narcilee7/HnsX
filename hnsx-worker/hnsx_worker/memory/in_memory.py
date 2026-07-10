"""In-memory memory backend — fast, subprocess-local, useful for tests."""

from __future__ import annotations

import time

from .base import MemoryItem, MemoryStore


class InMemoryMemoryStore(MemoryStore):
    """Subprocess-local memory store."""

    def __init__(self) -> None:
        self._items: list[MemoryItem] = []

    @property
    def name(self) -> str:
        return "in_memory"

    def add(self, item: MemoryItem) -> None:
        self._cleanup_expired()
        # Overwrite by key if one exists.
        if item.key:
            self._items = [
                i
                for i in self._items
                if not self._key_match(i, item.key, item.session_id)
            ]
        item._created_at = time.monotonic()  # type: ignore[attr-defined]
        self._items.append(item)

    def search(
        self,
        query: str,
        *,
        session_id: str = "",
        top_k: int = 5,
    ) -> list[MemoryItem]:
        self._cleanup_expired()
        q = str(query).lower()
        scored: list[tuple[float, MemoryItem]] = []
        for item in self._items:
            if session_id and item.session_id != session_id:
                continue
            text = f"{item.key} {self._content_text(item)}".lower()
            if q in text:
                scored.append((text.count(q), item))
        scored.sort(key=lambda x: x[0], reverse=True)
        return [item for _, item in scored[:top_k]]

    def get_recent(self, n: int = 5, *, session_id: str = "") -> list[MemoryItem]:
        self._cleanup_expired()
        items = self._items
        if session_id:
            items = [i for i in items if i.session_id == session_id]
        return list(reversed(items[-n:]))

    def delete(self, item_id: str) -> bool:
        before = len(self._items)
        self._items = [i for i in self._items if i.id != item_id]
        return len(self._items) < before

    def delete_by_key(self, key: str, *, session_id: str = "") -> bool:
        before = len(self._items)
        self._items = [i for i in self._items if not self._key_match(i, key, session_id)]
        return len(self._items) < before

    def close(self) -> None:
        self._items.clear()

    def _key_match(self, item: MemoryItem, key: str, session_id: str) -> bool:
        if item.key != key:
            return False
        if session_id and item.session_id != session_id:
            return False
        return True

    def _content_text(self, item: MemoryItem) -> str:
        import json

        try:
            return json.dumps(item.content, ensure_ascii=False, default=str)
        except Exception:  # noqa: BLE001
            return str(item.content)

    def _cleanup_expired(self) -> None:
        now = time.monotonic()
        self._items = [
            i
            for i in self._items
            if i.ttl_seconds <= 0 or (now - getattr(i, "_created_at", now)) < i.ttl_seconds
        ]


__all__ = ["InMemoryMemoryStore"]
