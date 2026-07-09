"""Tool abstraction — base classes for API Agent capabilities.

A ``Tool`` is a callable capability exposed to LLM agents via tool_use /
function_calling. The Tool layer is the **governance boundary** for API
agents: every tool call goes through policy gating and emits an audit
observation.

Distinct from CLI agents (Claude Code / Codex): those adapters bring their
own shell/file/edit primitives — the Tool layer only constrains and audits,
it does not re-implement them.

W3.1 scope: this module + ``registry.py`` provide the registry foundation.
Real tools (``http`` / ``sql`` / ``python``) land in W3.2 / later phases.
"""

from __future__ import annotations

from collections.abc import Callable
from dataclasses import dataclass, field
from typing import Any


# ---------------------------------------------------------------------------
# Result / decision types
# ---------------------------------------------------------------------------


@dataclass
class ToolResult:
    """The outcome of one tool invocation.

    Tools should return a ToolResult rather than raising — the executor
    captures ``error`` as a ``tool_result`` observation with ``ok: false``
    and continues the loop. Raising is reserved for programmer bugs.
    """

    output: Any = None
    error: str | None = None
    metadata: dict[str, Any] = field(default_factory=dict)

    @property
    def ok(self) -> bool:
        return self.error is None

    def to_observation_payload(self) -> dict[str, Any]:
        """Serialize for the ``tool_result`` observation payload."""
        if self.error is not None:
            return {
                "ok": False,
                "error": self.error,
                "metadata": dict(self.metadata),
            }
        return {
            "ok": True,
            "output": self.output,
            "metadata": dict(self.metadata),
        }


@dataclass
class ToolDecision:
    """Result of a policy check on a tool call.

    W6 will introduce a full ``PolicyEngine`` that produces these. For now
    ``ToolRegistry`` accepts a plain ``Callable[[name, input, ctx], ToolDecision]``
    so W3.1 doesn't need to import the policy package.
    """

    allow: bool = True
    decision: str = "allow"  # 'allow' | 'deny' | 'require_approval'
    reason: str = ""


# ---------------------------------------------------------------------------
# Per-call context
# ---------------------------------------------------------------------------


# Signature of the policy hook passed to ``ToolRegistry``. Returns a decision.
PolicyHook = Callable[[str, dict[str, Any], "ToolContext"], ToolDecision]


# Signature of the observation emitter passed via ``ToolContext.emit``.
# Matches the ``emit`` callable used by ``session_executor``.
EmitFn = Callable[[dict[str, Any]], None]


@dataclass
class ToolContext:
    """Per-invocation context for a tool call.

    Bundles everything a tool needs about the current session / turn:

      - **Identity**: ``session_id`` / ``domain_id`` / ``agent_id`` / ``turn``.
      - **Secrets**: ``{secret.XXX}`` placeholders resolved by Control Plane
        before reaching the worker. Tools read these via
        ``ctx.secrets["XXX"]``. Audit logs record the secret *name* only,
        never the value.
      - **Emit**: a hook that lets the tool push its own observations
        (rate limits, cache hits, retries, etc.) onto the session stream.
    """

    session_id: str = ""
    domain_id: str = ""
    agent_id: str = ""
    turn: int = 0
    tool_call_id: str = ""
    secrets: dict[str, str] = field(default_factory=dict)
    emit: EmitFn | None = None


# ---------------------------------------------------------------------------
# Tool base class
# ---------------------------------------------------------------------------


class Tool:
    """Base class for API Agent tools.

    Subclasses implement:
      - ``name``   — the registered name (matches the agent's tool def).
      - ``schema`` — the JSON schema passed to the LLM as the tool definition.
      - ``invoke`` — the actual implementation.
    """

    @property
    def name(self) -> str:
        """The tool's registered name (matches ``agent.tools[].name``)."""
        raise NotImplementedError

    @property
    def schema(self) -> dict[str, Any]:
        """JSON schema describing the tool's input arguments.

        Default is an empty object schema, which makes the tool callable
        with any input. Subclasses should return a proper schema so the
        LLM can fill in arguments correctly.
        """
        return {"type": "object", "properties": {}, "additionalProperties": True}

    def invoke(self, ctx: ToolContext, input: dict[str, Any]) -> ToolResult:
        """Run the tool and return a result.

        Implementations should NOT raise for expected failures — return
        ``ToolResult(error=...)`` instead so the agent sees the failure
        and can adapt. Raising signals a programmer bug; the executor
        surfaces it as a ``session_end{state: failed}``.
        """
        raise NotImplementedError