"""Adapter base class and result type.

An Adapter is the bridge between the HnsX runtime and an external Agent
provider (Anthropic / OpenAI / ClaudeCode / etc.). Adapters wrap the
provider-specific wire format and surface a uniform ``invoke`` contract.
"""

from __future__ import annotations

from abc import ABC, abstractmethod
from dataclasses import dataclass


@dataclass
class AdapterResult:
    """The result of one Agent invocation.

    Attributes:
        text: The agent's text reply (always present; may be empty if the
            agent returned only tool calls — V1.1 doesn't model tool calls yet).
        cost: Optional cost/tokens/latency for this invocation. The runtime
            propagates this into the Session's final result.
    """

    text: str
    cost: "Cost | None" = None  # type: ignore[name-defined]


@dataclass
class Cost:
    """Per-invocation cost snapshot."""

    prompt_tokens: int = 0
    completion_tokens: int = 0
    cost_usd: float = 0.0
    latency_ms: int = 0


class Adapter(ABC):
    """Abstract base class for Agent adapters.

    Implementations MUST be safe for concurrent use from multiple threads
    (the runtime shares one adapter instance across all turns of one session
    and may dispatch concurrent sessions to the same adapter).
    """

    @abstractmethod
    def name(self) -> str:
        """Return the adapter kind (e.g. "noop", "anthropic")."""

    @abstractmethod
    def invoke(
        self,
        agent: dict,
        prompt: str,
        input: dict,
    ) -> AdapterResult:
        """Invoke the agent with the given prompt and input.

        Args:
            agent: The agent's spec dict (subset of DomainSpec.Harness.Agents).
            prompt: The fully-resolved system prompt string.
            input: The current turn's input dict (trigger + step context).

        Returns:
            The agent's reply. May raise on transport errors — the runtime
            will surface those as ``error`` observations.
        """
