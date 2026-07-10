"""Eval runner for W8/W9.

Runs a configured set of scorers against the session output and emits
``eval_score`` observations. W9 adds EvalSet batch running and baseline
comparison.
"""

from __future__ import annotations

import logging
import threading
import time
import uuid
from collections.abc import Callable
from typing import Any

from .scorers import Score, score

log = logging.getLogger("hnsx_worker.eval.runner")

EmitFn = Callable[[dict], None]
SessionFn = Callable[..., dict[str, Any]]


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


def run_eval_set(
    eval_set: dict[str, Any],
    session_fn: SessionFn,
    spec: dict[str, Any],
    config: dict[str, Any],
    *,
    stop_event: threading.Event,
    emit: EmitFn | None = None,
    baseline_report: dict[str, Any] | None = None,
) -> dict[str, Any]:
    """Run every case in ``eval_set`` through ``session_fn`` and score outputs.

    Args:
        eval_set: Dict with ``eval_set_id`` and ``cases`` list. Each case has
            ``case_id``, ``input``, ``expected``, and ``scorers``.
        session_fn: Callable that runs one session. Signature must accept
            ``(spec, trigger, config, *, stop_event, emit)`` and return a dict.
        spec: Parsed DomainSpec.
        config: Base session config; ``session_id`` and ``eval_case`` are
            derived per case.
        stop_event: Set to cancel the run early.
        emit: Optional observation sink.
        baseline_report: Optional previous report for regression comparison.

    Returns:
        JSON-safe eval report dict.
    """
    eval_set_id = str(eval_set.get("eval_set_id", "") or uuid.uuid4().hex[:8])
    cases = list(eval_set.get("cases") or [])
    base_session_id = str(config.get("session_id", f"eval-{eval_set_id}"))

    def _noop_emit(_obs: dict) -> None:
        return None

    if emit is None:
        emit = _noop_emit

    report: dict[str, Any] = {
        "eval_set_id": eval_set_id,
        "total_cases": len(cases),
        "passed_cases": 0,
        "avg_score": 0.0,
        "total_duration_ms": 0,
        "total_cost_usd": 0.0,
        "cases": [],
        "baseline": None,
    }

    if stop_event.is_set():
        return report

    started_at = time.monotonic()
    case_reports: list[dict[str, Any]] = []
    all_scores: list[float] = []
    passed_count = 0

    for idx, case in enumerate(cases):
        if stop_event.is_set():
            log.info("eval_set %s cancelled at case %s", eval_set_id, idx)
            break

        if not isinstance(case, dict):
            log.warning("eval_set case %s is not a dict", idx)
            continue

        case_id = str(case.get("case_id") or f"case-{idx}")
        trigger = case.get("input") or {}
        eval_case = {
            "case_id": case_id,
            "expected": case.get("expected"),
            "scorers": list(case.get("scorers") or []),
        }
        case_config = dict(config)
        case_config["session_id"] = f"{base_session_id}-{case_id}"
        case_config["eval_case"] = eval_case
        # Ensure a single case does not inherit a previous eval_report.
        case_config.pop("eval_set", None)

        if emit is not None:
            emit(
                {
                    "kind": "eval_case_start",
                    "session_id": case_config["session_id"],
                    "payload": {"eval_set_id": eval_set_id, "case_id": case_id},
                }
            )

        try:
            result = session_fn(
                spec,
                trigger,
                case_config,
                stop_event=stop_event,
                emit=emit,
            )
        except Exception as e:  # noqa: BLE001
            log.warning("eval_set case %s failed: %s", case_id, e)
            case_reports.append(
                {
                    "case_id": case_id,
                    "input": trigger,
                    "error": str(e),
                    "error_type": type(e).__name__,
                    "eval_scores": aggregate_scores({}),
                }
            )
            continue

        eval_scores = result.get("eval_scores") or aggregate_scores({})
        case_report: dict[str, Any] = {
            "case_id": case_id,
            "input": trigger,
            "output": result.get("output", ""),
            "eval_scores": eval_scores,
            "duration_ms": result.get("duration_ms", 0),
            "cost_usd": result.get("cost_usd", 0.0),
        }
        case_reports.append(case_report)
        all_scores.append(float(eval_scores.get("score", 0.0)))
        if (
            eval_scores.get("passed", 0) == eval_scores.get("total", 0)
            and eval_scores.get("total", 0) > 0
        ):
            passed_count += 1

        if emit is not None:
            emit(
                {
                    "kind": "eval_case_end",
                    "session_id": case_config["session_id"],
                    "payload": {
                        "eval_set_id": eval_set_id,
                        "case_id": case_id,
                        "eval_scores": eval_scores,
                    },
                }
            )

    report["cases"] = case_reports
    report["total_duration_ms"] = int((time.monotonic() - started_at) * 1000)
    report["total_cost_usd"] = sum(c.get("cost_usd", 0.0) for c in case_reports)
    report["avg_score"] = round(sum(all_scores) / len(all_scores), 4) if all_scores else 0.0
    report["passed_cases"] = passed_count

    if baseline_report is not None:
        report["baseline"] = compare_eval_reports(report, baseline_report)

    return report


def compare_eval_reports(
    current: dict[str, Any],
    baseline: dict[str, Any],
    *,
    score_threshold: float = 0.05,
) -> dict[str, Any]:
    """Compare two eval reports and flag regressions / improvements."""
    baseline_by_id = {c["case_id"]: c for c in baseline.get("cases", [])}
    changes: list[dict[str, Any]] = []

    for case in current.get("cases", []):
        case_id = case["case_id"]
        base = baseline_by_id.get(case_id)
        if base is None:
            changes.append({"case_id": case_id, "change": "new"})
            continue

        cur_score = float(case.get("eval_scores", {}).get("score", 0.0))
        base_score = float(base.get("eval_scores", {}).get("score", 0.0))
        delta = round(cur_score - base_score, 4)

        if delta < -score_threshold:
            change = "regression"
        elif delta > score_threshold:
            change = "improvement"
        else:
            change = "unchanged"

        changes.append(
            {
                "case_id": case_id,
                "change": change,
                "before": base_score,
                "after": cur_score,
                "delta": delta,
            }
        )

    return {
        "baseline_eval_set_id": baseline.get("eval_set_id"),
        "changes": changes,
        "regressions": sum(1 for c in changes if c["change"] == "regression"),
        "improvements": sum(1 for c in changes if c["change"] == "improvement"),
        "unchanged": sum(1 for c in changes if c["change"] == "unchanged"),
        "new": sum(1 for c in changes if c["change"] == "new"),
    }


__all__ = ["aggregate_scores", "compare_eval_reports", "run_eval", "run_eval_set"]
