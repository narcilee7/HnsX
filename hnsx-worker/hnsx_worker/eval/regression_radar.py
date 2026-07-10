"""W13 Regression Radar — track eval baselines and flag regressions.

W9 ships :func:`hnsx_worker.eval.runner.compare_eval_reports` which
compares two reports in memory. W13 wraps it with:

  - :class:`RegressionStore` — a tiny in-process key/value store for
    persisting eval reports by id (so we can compare ``latest`` against
    any historical baseline).
  - :func:`detect_regressions` — given a current report and a baseline,
    produce a list of :class:`RegressionAlert` objects (severity-tagged).
  - :func:`check_regressions` — CI-friendly wrapper that picks the right
    baseline and returns a non-zero exit code on regression.

CI integration::

    from hnsx_worker.eval import RegressionStore, check_regressions
    store = RegressionStore()  # load from disk in real deployments
    rc = check_regressions(current_report, store=store, baseline_label="main")
    sys.exit(rc)

The alert levels are:

  - ``ok``        — no regressions.
  - ``watch``    — score dropped but within tolerance.
  - ``regression`` — score dropped beyond the threshold; fail CI by default.
"""

from __future__ import annotations

import json
import logging
import os
import tempfile
import time
import uuid
from collections.abc import Callable
from dataclasses import asdict, dataclass, field
from typing import Any

log = logging.getLogger("hnsx_worker.eval.regression_radar")

EmitFn = Callable[[dict], None]


SEVERITY_ORDER = ("ok", "watch", "regression")
DEFAULT_SCORE_THRESHOLD = 0.05

# Where to persist the baseline store when neither ``path=`` nor
# ``HNSX_REGRESSION_STORE`` is set. The path is relative to CWD — for
# installed worker processes, callers should override via env var.
DEFAULT_PATH = ".hnsx/regression-radar.json"
ENV_PATH_KEY = "HNSX_REGRESSION_STORE"


def resolve_default_path(path: str | None = None) -> str:
    """Pick the baseline path with priority ``arg > env > default``."""
    if path:
        return path
    env_path = os.environ.get(ENV_PATH_KEY)
    if env_path:
        return env_path
    return DEFAULT_PATH


# ---------------------------------------------------------------------------
# Types
# ---------------------------------------------------------------------------


@dataclass
class RegressionAlert:
    """One scored-case alert."""

    case_id: str
    severity: str  # "ok" | "watch" | "regression"
    before: float
    after: float
    delta: float
    reason: str = ""


@dataclass
class RegressionCheck:
    """Output of :func:`detect_regressions`."""

    baseline_label: str
    baseline_eval_set_id: str
    severity: str  # worst severity across cases
    alerts: list[RegressionAlert] = field(default_factory=list)
    summary: dict[str, int] = field(default_factory=dict)

    @property
    def regressions(self) -> list[RegressionAlert]:
        return [a for a in self.alerts if a.severity == "regression"]

    @property
    def watches(self) -> list[RegressionAlert]:
        return [a for a in self.alerts if a.severity == "watch"]

    def to_dict(self) -> dict[str, Any]:
        return {
            "baseline_label": self.baseline_label,
            "baseline_eval_set_id": self.baseline_eval_set_id,
            "severity": self.severity,
            "alerts": [asdict(a) for a in self.alerts],
            "summary": self.summary,
        }


# ---------------------------------------------------------------------------
# Persistent store
# ---------------------------------------------------------------------------


class RegressionStore:
    """Tiny baseline-store backed by a JSON file.

    Not a database — designed for ``main`` / ``PR-head`` style baselines
    where one commit produces one report. Keys are arbitrary labels (CI
    job names, branch names, ``"main"``).
    """

    def __init__(self, path: str | None = None) -> None:
        self._path = resolve_default_path(path)
        self._data: dict[str, dict[str, Any]] = {}
        self._load()

    # ---- I/O --------------------------------------------------------

    def _load(self) -> None:
        if not os.path.exists(self._path):
            return
        try:
            with open(self._path, "r", encoding="utf-8") as fh:
                raw = json.load(fh)
            if isinstance(raw, dict):
                self._data = raw
        except (OSError, json.JSONDecodeError) as e:
            log.warning("RegressionStore %s: failed to load: %s", self._path, e)
            self._data = {}

    def save(self) -> None:
        try:
            os.makedirs(os.path.dirname(self._path) or ".", exist_ok=True)
            with open(self._path, "w", encoding="utf-8") as fh:
                json.dump(self._data, fh, ensure_ascii=False, default=str)
        except OSError as e:
            log.warning("RegressionStore %s: failed to save: %s", self._path, e)

    @property
    def path(self) -> str:
        return self._path

    # ---- API --------------------------------------------------------

    def put(self, label: str, report: dict[str, Any]) -> None:
        self._data[label] = {
            "stored_at_ms": int(time.time() * 1000),
            "report": dict(report),
        }
        self.save()

    def get(self, label: str) -> dict[str, Any] | None:
        entry = self._data.get(label)
        if entry is None:
            return None
        return entry.get("report")

    def labels(self) -> list[str]:
        return sorted(self._data.keys())

    def clear(self) -> None:
        self._data = {}
        self.save()


# ---------------------------------------------------------------------------
# Detection
# ---------------------------------------------------------------------------


def _score_delta(current: dict, baseline: dict) -> float:
    return float(current.get("eval_scores", {}).get("score", 0.0)) - float(
        baseline.get("eval_scores", {}).get("score", 0.0)
    )


def detect_regressions(
    current_report: dict[str, Any],
    baseline_report: dict[str, Any],
    *,
    score_threshold: float = DEFAULT_SCORE_THRESHOLD,
    watch_threshold_ratio: float = 0.5,
    baseline_label: str = "baseline",
) -> RegressionCheck:
    """Compare two reports and emit a :class:`RegressionCheck`.

    Args:
        current_report: Latest eval report.
        baseline_report: Older eval report to compare against.
        score_threshold: Cases whose score drops by more than this
            absolute delta are flagged as ``regression``.
        watch_threshold_ratio: Cases whose drop is between
            ``watch_threshold_ratio * score_threshold`` and ``score_threshold``
            are flagged as ``watch``.
        baseline_label: Free-form label echoed in the result.
    """
    baseline_cases = {c["case_id"]: c for c in baseline_report.get("cases", [])}
    alerts: list[RegressionAlert] = []
    worst = "ok"

    for case in current_report.get("cases", []):
        case_id = case.get("case_id", "")
        base = baseline_cases.get(case_id)
        if base is None:
            alerts.append(
                RegressionAlert(
                    case_id=case_id,
                    severity="ok",
                    before=0.0,
                    after=float(case.get("eval_scores", {}).get("score", 0.0)),
                    delta=0.0,
                    reason="new case (no baseline)",
                )
            )
            continue

        delta = _score_delta(case, base)
        if delta < -score_threshold:
            severity = "regression"
        elif delta < -(score_threshold * watch_threshold_ratio):
            severity = "watch"
        else:
            severity = "ok"

        alerts.append(
            RegressionAlert(
                case_id=case_id,
                severity=severity,
                before=float(base.get("eval_scores", {}).get("score", 0.0)),
                after=float(case.get("eval_scores", {}).get("score", 0.0)),
                delta=round(delta, 4),
                reason=(
                    f"score dropped by {abs(delta):.3f}"
                    if severity != "ok"
                    else ""
                ),
            )
        )
        if SEVERITY_ORDER.index(severity) > SEVERITY_ORDER.index(worst):
            worst = severity

    summary = {
        "total": len(alerts),
        "regression": sum(1 for a in alerts if a.severity == "regression"),
        "watch": sum(1 for a in alerts if a.severity == "watch"),
        "ok": sum(1 for a in alerts if a.severity == "ok"),
    }
    return RegressionCheck(
        baseline_label=baseline_label,
        baseline_eval_set_id=str(baseline_report.get("eval_set_id", "")),
        severity=worst,
        alerts=alerts,
        summary=summary,
    )


# ---------------------------------------------------------------------------
# CI-friendly wrapper
# ---------------------------------------------------------------------------


def check_regressions(
    current_report: dict[str, Any],
    *,
    store: RegressionStore | None = None,
    baseline_label: str = "main",
    score_threshold: float = DEFAULT_SCORE_THRESHOLD,
    fail_on: str = "regression",
    emit: EmitFn | None = None,
) -> tuple[RegressionCheck, int]:
    """Load ``baseline_label`` from ``store`` and compare against ``current_report``.

    Returns ``(check, exit_code)``. The exit code is 0 when severity is
    below ``fail_on``, 1 otherwise. Missing baseline → exit 0 with a
    "first run" check.
    """
    emit = emit or (lambda _o: None)
    if store is None:
        store = RegressionStore()
    baseline = store.get(baseline_label)
    if baseline is None:
        check = RegressionCheck(
            baseline_label=baseline_label,
            baseline_eval_set_id="",
            severity="ok",
            alerts=[],
            summary={"total": 0, "regression": 0, "watch": 0, "ok": 0},
        )
        emit(
            {
                "kind": "regression_check",
                "payload": {
                    **check.to_dict(),
                    "first_run": True,
                },
            }
        )
        return check, 0

    check = detect_regressions(
        current_report,
        baseline,
        score_threshold=score_threshold,
        baseline_label=baseline_label,
    )
    rc = 1 if SEVERITY_ORDER.index(check.severity) >= SEVERITY_ORDER.index(fail_on) else 0
    emit({"kind": "regression_check", "payload": {**check.to_dict(), "exit_code": rc}})
    return check, rc


__all__ = [
    "DEFAULT_PATH",
    "DEFAULT_SCORE_THRESHOLD",
    "ENV_PATH_KEY",
    "RegressionAlert",
    "RegressionCheck",
    "RegressionStore",
    "check_regressions",
    "detect_regressions",
    "resolve_default_path",
]