"""Multi-agent orchestration support (W5).

Public API:

  - :func:`harness.loader.load` — validate a DomainSpec for orchestration.
  - :func:`harness.runner.run` — run supervisor / hierarchical / autonomous loops.
  - :func:`harness.transition.evaluate_condition` — evaluate a transition rule.
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
