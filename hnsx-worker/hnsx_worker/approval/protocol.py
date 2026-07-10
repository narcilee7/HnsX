"""Approval protocol — request/response shapes and state machine."""

from __future__ import annotations

import enum
import time
import uuid
from collections.abc import Mapping
from dataclasses import dataclass, field
from typing import Any


class ApprovalState(str, enum.Enum):
    """Lifecycle states for an approval request."""

    PENDING = "pending"
    APPROVED = "approved"
    DENIED = "denied"
    EDITED = "edited"  # human approved AND modified the draft
    TIMEOUT = "timeout"
    CANCELLED = "cancelled"


class ApprovalDecision(str, enum.Enum):
    """High-level decision returned to the agent loop."""

    APPROVE = "approve"
    DENY = "deny"
    EDIT = "edit"  # human supplied an edited version of the input


@dataclass
class ApprovalRequest:
    """A single approval request sent to a human / operator.

    ``request_id`` is assigned at construction. ``severity`` is a free-form
    label (``low`` / ``medium`` / ``high``) that downstream transports can
    use to prioritise notifications.
    """

    session_id: str
    domain_id: str
    agent_id: str
    reason: str
    options: list[str] = field(default_factory=list)
    severity: str = "medium"
    draft: Any = None  # optional draft the human can edit
    tool_name: str = ""
    tool_input: dict[str, Any] = field(default_factory=dict)
    request_id: str = field(default_factory=lambda: f"approval-{uuid.uuid4().hex[:8]}")
    created_at_ms: int = field(default_factory=lambda: int(time.time() * 1000))
    timeout_seconds: float = 0.0  # 0 means "wait forever"
    metadata: dict[str, Any] = field(default_factory=dict)

    def to_dict(self) -> dict[str, Any]:
        return {
            "request_id": self.request_id,
            "session_id": self.session_id,
            "domain_id": self.domain_id,
            "agent_id": self.agent_id,
            "reason": self.reason,
            "options": list(self.options),
            "severity": self.severity,
            "draft": self.draft,
            "tool_name": self.tool_name,
            "tool_input": dict(self.tool_input),
            "created_at_ms": self.created_at_ms,
            "timeout_seconds": self.timeout_seconds,
            "metadata": dict(self.metadata),
        }

    @classmethod
    def from_dict(cls, raw: Mapping[str, Any]) -> "ApprovalRequest":
        return cls(
            session_id=str(raw.get("session_id", "")),
            domain_id=str(raw.get("domain_id", "")),
            agent_id=str(raw.get("agent_id", "")),
            reason=str(raw.get("reason", "")),
            options=list(raw.get("options") or []),
            severity=str(raw.get("severity", "medium")),
            draft=raw.get("draft"),
            tool_name=str(raw.get("tool_name", "")),
            tool_input=dict(raw.get("tool_input") or {}),
            request_id=str(raw.get("request_id") or f"approval-{uuid.uuid4().hex[:8]}"),
            created_at_ms=int(raw.get("created_at_ms") or int(time.time() * 1000)),
            timeout_seconds=float(raw.get("timeout_seconds") or 0.0),
            metadata=dict(raw.get("metadata") or {}),
        )


@dataclass
class ApprovalResponse:
    """What the human / operator decided."""

    request_id: str
    decision: ApprovalDecision
    chosen_option: str = ""
    edited_draft: Any = None
    decided_by: str = ""
    comment: str = ""
    decided_at_ms: int = field(default_factory=lambda: int(time.time() * 1000))
    metadata: dict[str, Any] = field(default_factory=dict)

    @property
    def allowed(self) -> bool:
        return self.decision in (ApprovalDecision.APPROVE, ApprovalDecision.EDIT)

    def to_dict(self) -> dict[str, Any]:
        return {
            "request_id": self.request_id,
            "decision": self.decision.value,
            "chosen_option": self.chosen_option,
            "edited_draft": self.edited_draft,
            "decided_by": self.decided_by,
            "comment": self.comment,
            "decided_at_ms": self.decided_at_ms,
            "metadata": dict(self.metadata),
        }

    @classmethod
    def from_dict(cls, raw: Mapping[str, Any]) -> "ApprovalResponse":
        decision_str = str(raw.get("decision") or ApprovalDecision.DENY.value)
        try:
            decision = ApprovalDecision(decision_str)
        except ValueError:
            decision = ApprovalDecision.DENY
        return cls(
            request_id=str(raw.get("request_id", "")),
            decision=decision,
            chosen_option=str(raw.get("chosen_option") or ""),
            edited_draft=raw.get("edited_draft"),
            decided_by=str(raw.get("decided_by") or ""),
            comment=str(raw.get("comment") or ""),
            decided_at_ms=int(raw.get("decided_at_ms") or int(time.time() * 1000)),
            metadata=dict(raw.get("metadata") or {}),
        )


__all__ = [
    "ApprovalDecision",
    "ApprovalRequest",
    "ApprovalResponse",
    "ApprovalState",
]