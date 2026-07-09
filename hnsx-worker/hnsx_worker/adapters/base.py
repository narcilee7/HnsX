"""Adapter base class and result type.

An Adapter is the bridge between the HnsX runtime and an external Agent
provider (Anthropic / OpenAI / ClaudeCode / Codex / Ollama / etc.). Adapters wrap
the provider-specific wire format and surface a uniform ``invoke`` contract.
"""

from __future__ import annotations

from abc import ABC, abstractmethod
from dataclasses import dataclass, field
from typing import Any


@dataclass
class Cost:
    """Per-invocation cost snapshot."""

    prompt_tokens: int = 0
    completion_tokens: int = 0
    cost_usd: float = 0.0
    latency_ms: int = 0


@dataclass
class ToolCall:
    """A tool invocation requested by the agent.

    Attributes:
        id: Provider-scoped tool call id (used to match results).
        name: Tool name as registered in the ToolRegistry.
        input: Parsed tool arguments (dict).
        raw_input: Raw input JSON string, if already serialized by the provider.
    """

    id: str
    name: str
    input: dict[str, Any] = field(default_factory=dict)
    raw_input: str = ""


@dataclass
class StreamChunk:
    """One piece of a streaming adapter response.

    A stream is a sequence of chunks. Text deltas should be concatenated to
    form the final assistant message. Tool calls are emitted once, when the
    full input JSON has been assembled.
    """

    text_delta: str = ""
    tool_call: ToolCall | None = None
    finish_reason: str | None = None
    cost: Cost | None = None


@dataclass
class AdapterResult:
    """The result of one Agent invocation.

    Attributes:
        text: The agent's text reply (may be empty if the response only
            contained tool calls).
        tool_calls: Tool invocations requested by the agent.
        cost: Optional cost/tokens/latency for this invocation.
    """

    text: str
    tool_calls: list[ToolCall] = field(default_factory=list)
    cost: Cost | None = None


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

    def invoke_stream(
        self,
        agent: dict,
        prompt: str,
        input: dict,
    ) -> Any:
        """Stream the agent response as a sequence of :class:`StreamChunk`.

        The default implementation delegates to :meth:`invoke` and emits a
        single chunk containing the full text. Adapters that support native
        streaming (Anthropic, OpenAI) should override this method.

        Yields:
            StreamChunk objects.
        """
        result = self.invoke(agent, prompt, input)
        if result.tool_calls:
            for tc in result.tool_calls:
                yield StreamChunk(tool_call=tc)
        yield StreamChunk(text_delta=result.text, cost=result.cost)
