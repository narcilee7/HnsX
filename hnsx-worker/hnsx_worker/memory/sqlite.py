"""SQLite-backed persistent memory store.

Schema:

  - ``memory_items``: one row per MemoryItem.
  - search uses a simple substring match over key + content JSON; sufficient
    for W11. Vector/semantic search lands in a future phase with optional
    backends.
"""

from __future__ import annotations

import json
import sqlite3
import threading
import time
from typing import Any

from .base import MemoryItem, MemoryStore


class SqliteMemoryStore(MemoryStore):
    """File-backed memory store using SQLite.

    ``path`` follows normal sqlite3 semantics:

      - ``:memory:`` for an in-memory database.
      - A filesystem path for persistence across worker restarts.
    """

    def __init__(self, path: str = ":memory:") -> None:
        self._path = path
        self._lock = threading.Lock()
        self._conn = sqlite3.connect(path, check_same_thread=False)
        self._ensure_schema()

    @property
    def name(self) -> str:
        return "sqlite"

    def _ensure_schema(self) -> None:
        with self._conn:
            self._conn.execute(
                """
                CREATE TABLE IF NOT EXISTS memory_items (
                    id TEXT PRIMARY KEY,
                    session_id TEXT NOT NULL,
                    kind TEXT NOT NULL,
                    key TEXT,
                    content TEXT NOT NULL,
                    created_at REAL NOT NULL,
                    ttl_seconds INTEGER NOT NULL DEFAULT 0
                )
                """
            )
            self._conn.execute(
                "CREATE INDEX IF NOT EXISTS idx_memory_session ON memory_items(session_id)"
            )
            self._conn.execute(
                "CREATE INDEX IF NOT EXISTS idx_memory_created ON memory_items(created_at)"
            )
            self._conn.execute(
                "CREATE INDEX IF NOT EXISTS idx_memory_key ON memory_items(key)"
            )

    def add(self, item: MemoryItem) -> None:
        self._cleanup_expired()
        now = time.monotonic()
        with self._lock, self._conn:
            if item.key:
                self._conn.execute(
                    "DELETE FROM memory_items WHERE key = ? AND session_id = ?",
                    (item.key, item.session_id),
                )
            self._conn.execute(
                """
                INSERT INTO memory_items
                (id, session_id, kind, key, content, created_at, ttl_seconds)
                VALUES (?, ?, ?, ?, ?, ?, ?)
                """,
                (
                    item.id,
                    item.session_id,
                    item.kind,
                    item.key or None,
                    item.to_json(),
                    now,
                    item.ttl_seconds,
                ),
            )

    def search(
        self,
        query: str,
        *,
        session_id: str = "",
        top_k: int = 5,
    ) -> list[MemoryItem]:
        self._cleanup_expired()
        q = f"%{query}%"
        sql = """
            SELECT id, session_id, kind, key, content, ttl_seconds
            FROM memory_items
            WHERE (key LIKE ? OR content LIKE ?)
        """
        params: list[Any] = [q, q]
        if session_id:
            sql += " AND session_id = ?"
            params.append(session_id)
        sql += " ORDER BY created_at DESC LIMIT ?"
        params.append(top_k)
        with self._lock, self._conn:
            rows = self._conn.execute(sql, params).fetchall()
        return [self._row_to_item(row) for row in rows]

    def get_recent(self, n: int = 5, *, session_id: str = "") -> list[MemoryItem]:
        self._cleanup_expired()
        sql = "SELECT id, session_id, kind, key, content, ttl_seconds FROM memory_items"
        params: list[Any] = []
        if session_id:
            sql += " WHERE session_id = ?"
            params.append(session_id)
        sql += " ORDER BY created_at DESC LIMIT ?"
        params.append(n)
        with self._lock, self._conn:
            rows = self._conn.execute(sql, params).fetchall()
        return [self._row_to_item(row) for row in rows]

    def delete(self, item_id: str) -> bool:
        with self._lock, self._conn:
            cur = self._conn.execute(
                "DELETE FROM memory_items WHERE id = ?", (item_id,)
            )
            return cur.rowcount > 0

    def delete_by_key(self, key: str, *, session_id: str = "") -> bool:
        with self._lock, self._conn:
            if session_id:
                cur = self._conn.execute(
                    "DELETE FROM memory_items WHERE key = ? AND session_id = ?",
                    (key, session_id),
                )
            else:
                cur = self._conn.execute(
                    "DELETE FROM memory_items WHERE key = ?", (key,)
                )
            return cur.rowcount > 0

    def close(self) -> None:
        with self._lock:
            self._conn.close()

    def _row_to_item(self, row: sqlite3.Row) -> MemoryItem:
        id_, session_id, kind, key, content, ttl_seconds = row
        return MemoryItem(
            id=id_,
            session_id=session_id,
            kind=kind,
            key=key or "",
            content=json.loads(content),
            ttl_seconds=ttl_seconds,
        )

    def _cleanup_expired(self) -> None:
        now = time.monotonic()
        with self._lock, self._conn:
            self._conn.execute(
                "DELETE FROM memory_items WHERE ttl_seconds > 0 AND ? - created_at > ttl_seconds",
                (now,),
            )


__all__ = ["SqliteMemoryStore"]
