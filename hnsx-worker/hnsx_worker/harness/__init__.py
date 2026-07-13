"""Multi-agent orchestration support (W5+).

Public API:

  - :func:`harness.loader.load` — typed data view of a DomainSpec.
  - :func:`harness.runner.run` — run supervisor / hierarchical / autonomous loops.
  - :func:`harness.transition.evaluate_condition` — evaluate a transition rule.

Authoritative DomainSpec validation lives in the Go server and is invoked
from ``session_runtime`` via the ``ValidateDomain`` gRPC call.
"""

from __future__ import annotations

from .loader import HarnessSpec, HarnessValidationError, load
from .runner import OrchestrationError, run
from .transition import build_context, evaluate_condition

__all__ = [
    "HarnessSpec",
    "HarnessValidationError",
    "OrchestrationError",
    "build_context",
    "evaluate_condition",
    "load",
    "run",
]
