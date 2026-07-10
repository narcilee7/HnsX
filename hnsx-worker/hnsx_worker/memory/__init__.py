"""Long-term memory for agents (W11).

Public API:

  - :class:`MemoryItem` — one stored memory.
  - :class:`MemoryStore` — abstract store.
  - :func:`build_backend` — construct a store from DomainSpec config.
"""

from __future__ import annotations

from .backend import MemoryStore, build_backend
from .base import MemoryItem

__all__ = ["MemoryItem", "MemoryStore", "build_backend"]
