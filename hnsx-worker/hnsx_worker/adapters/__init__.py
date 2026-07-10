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
    def reset(cls) -> None:
        """Drop cached singletons and the 'initialized' flag (test-only helper)."""
        cls._singletons.clear()
        cls._initialized = False

    @classmethod
    def _ensure_builtins(cls) -> None:
        if cls._initialized:
            return
        # Built-in / no-network adapters (always available).
        cls._registry.setdefault("noop", NoopAdapter)
        cls._registry.setdefault("echo", EchoAdapter)
        # Real provider adapters (lazy-imported so missing SDK / httpx
        # doesn't break the worker if a user only needs noop/echo).
        _try_register(cls, "anthropic", "hnsx_worker.adapters.anthropic", "AnthropicAdapter")
        _try_register(cls, "openai", "hnsx_worker.adapters.openai", "OpenAIAdapter")
        _try_register(cls, "ollama", "hnsx_worker.adapters.ollama", "OllamaAdapter")
        _try_register(cls, "claudecode", "hnsx_worker.adapters.claude_code", "ClaudeCodeAdapter")
        _try_register(cls, "codex", "hnsx_worker.adapters.codex", "CodexAdapter")
        cls._initialized = True


def _try_register(reg: type[AdapterRegistry], kind: str, module: str, class_name: str) -> None:
    try:
        mod = __import__(module, fromlist=[class_name])
        cls = getattr(mod, class_name, None)
    except Exception:  # noqa: BLE001 — missing dep shouldn't break startup
        return
    if cls is not None:
        reg._registry[kind] = cls  # type: ignore[attr-defined]
        reg._singletons.pop(kind, None)
