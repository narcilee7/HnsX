"""Memory abstraction — long-term context for agents.

A :class:`MemoryStore` persists :class:`MemoryItem` objects across turns and
sessions. W11 ships with:

  - ``sqlite`` (default): file-based persistent storage.
  - ``in_memory``: subprocess-local, useful for tests.

Future phases may add vector backends (chroma, mem0, weaviate) as optional
extras.
"""

from __future__ import annotations

import uuid
from abc import ABC, abstractmethod
from dataclasses import dataclass, field
from typing import Any


@dataclass
class MemoryItem:
    """One stored memory."""

    content: Any
    kind: str = "fact"  # fact | preference | task_state | summary
    key: str = ""
    session_id: str = ""
    id: str = field(default_factory=lambda: str(uuid.uuid4()))
    ttl_seconds: int = 0

    def to_json(self) -> str:
        import json

        return json.dumps(self.content, ensure_ascii=False, default=str)


class MemoryStore(ABC):
    """Abstract long-term memory store."""

    @property
    @abstractmethod
    def name(self) -> str:
        """Backend identifier."""

    @abstractmethod
    def add(self, item: MemoryItem) -> None:
        """Persist one memory item."""

    @abstractmethod
    def search(
        self,
        query: str,
        *,
        session_id: str = "",
        top_k: int = 5,
    ) -> list[MemoryItem]:
        """Return the most relevant items matching ``query``."""

    @abstractmethod
    def get_recent(self, n: int = 5, *, session_id: str = "") -> list[MemoryItem]:
        """Return the ``n`` most recent items, optionally scoped to a session."""

    @abstractmethod
    def delete(self, item_id: str) -> bool:
        """Delete one item by id. Returns True if it existed."""

    @abstractmethod
    def delete_by_key(self, key: str, *, session_id: str = "") -> bool:
        """Delete items matching ``key`` (and optionally ``session_id``)."""

    @abstractmethod
    def close(self) -> None:
        """Release any resources."""


__all__ = ["MemoryItem", "MemoryStore"]
