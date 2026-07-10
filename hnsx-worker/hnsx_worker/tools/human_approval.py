"""W14 ``request_human_approval`` tool — the agent pauses to ask a human.

The tool's contract::

    request_human_approval(
        reason="approve refund of $42",
        options=["approve", "deny", "edit"],
        draft="refund has been processed; check your email",
        severity="high",
        timeout_seconds=900,
    )

Returns ``ToolResult(output={decision, chosen_option, edited_draft, comment})``
where ``decision`` is one of ``approve`` / ``deny`` / ``edit``.

Wiring:

  - The tool is built by ``build_human_approval_tool(...)`` so the factory
    can hand it the surrounding :class:`ApprovalBus` and ``stop_event``.
  - On ``deny`` the tool returns an error result so the agent's loop sees
    ``ok=False`` and can branch accordingly.
  - On ``edit`` the ``edited_draft`` is returned so the next turn can
    use the human's edited version.
"""

from __future__ import annotations

import logging
import threading
from dataclasses import dataclass, field
from typing import Any

from hnsx_worker.approval import (
    ApprovalBus,
    ApprovalDecision,
    ApprovalRequest,
    request_approval,
)

from .base import Tool, ToolContext, ToolResult

log = logging.getLogger("hnsx_worker.tools.human_approval")


@dataclass
class HumanApprovalToolConfig:
    """Per-instance wiring for the ``request_human_approval`` tool."""

    bus: ApprovalBus
    stop_event: threading.Event = field(default_factory=threading.Event)
    default_timeout_seconds: float = 0.0  # 0 = wait forever
    default_options: list[str] = field(default_factory=lambda: ["approve", "deny"])

    @classmethod
    def from_spec(
        cls,
        raw: dict[str, Any],
        *,
        bus: ApprovalBus,
        stop_event: threading.Event,
    ) -> "HumanApprovalToolConfig":
        options = raw.get("default_options") or ["approve", "deny"]
        if not isinstance(options, list):
            raise ValueError("human_approval.default_options must be a list")
        return cls(
            bus=bus,
            stop_event=stop_event,
            default_timeout_seconds=float(raw.get("default_timeout_seconds") or 0.0),
            default_options=[str(o) for o in options],
        )


class HumanApprovalTool(Tool):
    """Tool: ``request_human_approval(...)``."""

    def __init__(self, name: str, config: HumanApprovalToolConfig) -> None:
        self._name = name
        self._config = config

    @property
    def name(self) -> str:
        return self._name

    @property
    def schema(self) -> dict[str, Any]:
        return {
            "type": "object",
            "properties": {
                "reason": {
                    "type": "string",
                    "description": "Why this approval is being requested.",
                },
                "options": {
                    "type": "array",
                    "items": {"type": "string"},
                    "description": "Choices offered to the human.",
                },
                "draft": {
                    "description": "Optional draft the human may edit before approval.",
                },
                "severity": {
                    "type": "string",
                    "enum": ["low", "medium", "high"],
                    "description": "Operator-visible urgency hint.",
                },
                "timeout_seconds": {
                    "type": "number",
                    "description": "How long to wait. 0 means wait forever.",
                },
            },
            "required": ["reason"],
            "additionalProperties": False,
        }

    def invoke(self, ctx: ToolContext, input: dict[str, Any]) -> ToolResult:
        if not isinstance(input, dict):
            return ToolResult(error="request_human_approval input must be a JSON object")

        reason = str(input.get("reason", "")).strip()
        if not reason:
            return ToolResult(error="request_human_approval requires a non-empty 'reason'")

        options = input.get("options") or self._config.default_options
        if not isinstance(options, list) or not options:
            return ToolResult(error="options must be a non-empty list of strings")
        options = [str(o) for o in options]

        timeout = float(
            input.get("timeout_seconds")
            if input.get("timeout_seconds") is not None
            else self._config.default_timeout_seconds
        )

        request = ApprovalRequest(
            session_id=ctx.session_id,
            domain_id=ctx.domain_id,
            agent_id=ctx.agent_id,
            reason=reason,
            options=options,
            severity=str(input.get("severity") or "medium"),
            draft=input.get("draft"),
            tool_name=ctx.tool_call_id and "request_human_approval" or "",
            tool_input={"reason": reason, "options": options},
            timeout_seconds=timeout,
            metadata={"turn": ctx.turn},
        )

        # Block (in-process) until the human responds, the timeout fires,
        # or the harness signals a stop.
        response = request_approval(
            self._config.bus,
            request,
            stop_event=self._config.stop_event,
        )

        output = {
            "decision": response.decision.value,
            "chosen_option": response.chosen_option,
            "edited_draft": response.edited_draft,
            "comment": response.comment,
            "decided_by": response.decided_by,
        }
        # ``deny`` and "timed out" both surface as errors so the multi-turn
        # loop sees a failure and the agent can react.
        if not response.allowed:
            return ToolResult(
                error=(
                    f"human approval {response.decision.value} "
                    f"({response.comment or 'no reason given'})"
                ),
                output=output,
                metadata={"decided_by": response.decided_by},
            )
        return ToolResult(
            output=output,
            metadata={
                "decided_by": response.decided_by,
                "was_edit": response.decision is ApprovalDecision.EDIT,
            },
        )


def build_human_approval_tool(
    name: str,
    raw_config: dict[str, Any],
    *,
    bus: ApprovalBus,
    stop_event: threading.Event,
) -> Tool:
    """Helper for the factory."""
    config = HumanApprovalToolConfig.from_spec(raw_config, bus=bus, stop_event=stop_event)
    return HumanApprovalTool(name, config)


__all__ = [
    "HumanApprovalTool",
    "HumanApprovalToolConfig",
    "build_human_approval_tool",
]
