"""W14 Human-in-the-loop approval.

The approval module defines a small async protocol that lets a running
session pause to ask a human for permission before continuing.

Two halves:

  - :class:`ApprovalBus` — the transport. Implementations can be in-memory
    (default, used by tests + single-worker deployments), a long-poll/SSE
    against the control plane, or a Slack/Email adapter.
  - :func:`request_approval` — the high-level "ask and wait" call that the
    Agent / Policy layer uses to gate dangerous operations.

A request goes through this lifecycle::

    submitted → pending → responded
                     ↘ timeout / denied → failed

Both legs emit observations so the operator can audit the decision later.
"""

from __future__ import annotations

from .bus import (
    ApprovalBus,
    InMemoryApprovalBus,
    PendingRequest,
    request_approval,
)
from .protocol import (
    ApprovalDecision,
    ApprovalRequest,
    ApprovalResponse,
    ApprovalState,
)

__all__ = [
    "ApprovalBus",
    "ApprovalDecision",
    "ApprovalRequest",
    "ApprovalResponse",
    "ApprovalState",
    "InMemoryApprovalBus",
    "PendingRequest",
    "request_approval",
]