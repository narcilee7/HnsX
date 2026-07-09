"""Adapter registry.

The registry maps ``AdapterConfig.Kind`` strings to adapter *classes*. New
provider adapters (#12) register themselves here at import time:

    from hnsx_worker.adapters import AdapterRegistry
    from hnsx_worker.adapters.anthropic import AnthropicAdapter
    AdapterRegistry.register("anthropic", AnthropicAdapter)
"""

from __future__ import annotations

from hnsx_worker.adapters.base import Adapter, AdapterResult, Cost
from hnsx_worker.adapters.echo import EchoAdapter
from hnsx_worker.adapters.noop import NoopAdapter

__all__ = ["Adapter", "AdapterResult", "Cost", "AdapterRegistry"]


class AdapterRegistry:
    """Maps adapter ``kind`` strings to their ``Adapter`` subclass.

    Instances are shared (worker-level). Adapters are instantiated lazily on
    first ``get()`` so that import order and test setup stay simple.
    """

    _registry: dict[str, type[Adapter]] = {}
    _singletons: dict[str, Adapter] = {}
    _initialized: bool = False

    @classmethod
    def register(cls, kind: str, adapter_cls: type[Adapter]) -> None:
        """Register an adapter class under ``kind`` (overwrites if present)."""
        cls._registry[kind] = adapter_cls
        cls._singletons.pop(kind, None)

    @classmethod
    def get(cls, kind: str) -> Adapter:
        """Return the singleton adapter instance for ``kind``.

        Raises:
            KeyError: if ``kind`` is not registered.
        """
        cls._ensure_builtins()
        if kind in cls._singletons:
            return cls._singletons[kind]
        adapter = cls._registry[kind]()
        cls._singletons[kind] = adapter
        return adapter

    @classmethod
    def kinds(cls) -> list[str]:
        """Return all registered kinds (for diagnostics)."""
        cls._ensure_builtins()
        return sorted(cls._registry.keys())

    @classmethod
    def _ensure_builtins(cls) -> None:
        if cls._initialized:
            return
        cls._registry.setdefault("noop", NoopAdapter)
        cls._registry.setdefault("echo", EchoAdapter)
        cls._initialized = True
