"""Eval runner for W8.

Runs a configured set of scorers against the session output and emits
``eval_score`` observations.
"""

from __future__ import annotations

import logging
from collections.abc import Callable
from typing import Any

from .scorers import Score, score

log = logging.getLogger("hnsx_worker.eval.runner")

EmitFn = Callable[[dict], None]


def run_eval(
    eval_case: dict[str, Any],
    session_output: Any,
    *,
    emit: EmitFn | None = None,
) -> dict[str, Score]:
    """Run all scorers defined in ``eval_case`` against ``session_output``.

    Args:
        eval_case: Dict with ``case_id``, ``expected``, and ``scorers`` list.
            Each scorer entry is ``{"name": "exact", ...kwargs}``.
        session_output: The value to score (typically the final assistant text).
        emit: Optional observation sink; each scorer emits an ``eval_score``.

    Returns:
        Mapping from scorer name to :class:`Score`.
    """
    case_id = str(eval_case.get("case_id", ""))
    expected = eval_case.get("expected")
    scorers = list(eval_case.get("scorers") or [])
    results: dict[str, Score] = {}

    for idx, entry in enumerate(scorers):
        if not isinstance(entry, dict):
            log.warning("eval_case scorer entry %s is not a dict", idx)
            continue
        name = str(entry.get("name", ""))
        kwargs = {k: v for k, v in entry.items() if k != "name"}
        results[name] = score(
            name,
            expected,
            session_output,
            case_id=case_id,
            emit=emit,
            **kwargs,
        )

    return results


def aggregate_scores(results: dict[str, Score]) -> dict[str, Any]:
    """Return a JSON-safe summary of scorer results."""
    if not results:
        return {"total": 0, "passed": 0, "score": 0.0}
    total = len(results)
    passed = sum(1 for s in results.values() if s.passed)
    avg_score = sum(s.score for s in results.values()) / total
    return {
        "total": total,
        "passed": passed,
        "score": round(avg_score, 4),
        "details": {
            name: {
                "score": s.score,
                "passed": s.passed,
                "details": s.details,
            }
            for name, s in results.items()
        },
    }


__all__ = ["aggregate_scores", "run_eval"]
