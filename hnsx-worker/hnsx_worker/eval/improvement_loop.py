"""W13 Eval-driven improvement loop.

The W8/W9 runner produces an :func:`run_eval_set` report. W13 adds the
post-hoc step: look at the failed cases, classify them into failure
modes, and propose concrete prompt / policy / tool fixes.

The loop is intentionally offline:

  1. Aggregate failures from the latest ``eval_report``.
  2. Optionally ask an LLM (configured judge) to cluster them by mode.
  3. For each cluster, generate a fix suggestion.
  4. Emit a single ``improvement_report`` observation so Eval / Trace
     pick it up.

No prompt is applied automatically — :class:`ImprovementReport` exposes
``apply()`` as a no-op until a human approves (or :class:`PromptEvolution`
selects the best variant — see :mod:`prompt_evolution`).
"""

from __future__ import annotations

import json
import logging
import re
import uuid
from collections.abc import Callable
from dataclasses import dataclass, field
from typing import Any

from hnsx_worker.adapters import AdapterRegistry

log = logging.getLogger("hnsx_worker.eval.improvement_loop")

EmitFn = Callable[[dict], None]


# ---------------------------------------------------------------------------
# Domain types
# ---------------------------------------------------------------------------


@dataclass
class FailureCase:
    """One failed EvalCase with enough context to diagnose it."""

    case_id: str
    input: Any
    output: Any
    expected: Any
    scorer: str
    score: float
    reason: str = ""
    trace_excerpt: list[dict[str, Any]] = field(default_factory=list)


@dataclass
class FailureCluster:
    """A cluster of failures sharing one root cause."""

    cluster_id: str
    label: str
    description: str
    cases: list[FailureCase] = field(default_factory=list)
    suggested_fixes: list[dict[str, Any]] = field(default_factory=list)


@dataclass
class ImprovementReport:
    """Output of :func:`run_improvement_loop`."""

    report_id: str
    eval_set_id: str
    generated_at_ms: int
    total_cases: int
    failed_cases: int
    passed_rate: float
    clusters: list[FailureCluster] = field(default_factory=list)
    raw_suggestions: list[dict[str, Any]] = field(default_factory=list)

    def to_dict(self) -> dict[str, Any]:
        return {
            "report_id": self.report_id,
            "eval_set_id": self.eval_set_id,
            "generated_at_ms": self.generated_at_ms,
            "total_cases": self.total_cases,
            "failed_cases": self.failed_cases,
            "passed_rate": round(self.passed_rate, 4),
            "clusters": [
                {
                    "cluster_id": c.cluster_id,
                    "label": c.label,
                    "description": c.description,
                    "case_ids": [cc.case_id for cc in c.cases],
                    "suggested_fixes": c.suggested_fixes,
                }
                for c in self.clusters
            ],
            "raw_suggestions": self.raw_suggestions,
        }

    def apply(self) -> None:
        """No-op until a human / PromptEvolution approves.

        Kept as a method so callers can ``report.apply()`` once they decide
        on a fix path. Future phases can wire this into a control-plane
        endpoint that patches the DomainSpec and emits a ``prompt_updated``
        observation.
        """
        log.info(
            "improvement_report %s: apply() called but no automatic "
            "prompt rewrite is configured; review clusters manually.",
            self.report_id,
        )


# ---------------------------------------------------------------------------
# Failure extraction
# ---------------------------------------------------------------------------


def _extract_failures(eval_report: dict[str, Any]) -> list[FailureCase]:
    """Pull failed cases out of an ``eval_report``."""
    failures: list[FailureCase] = []
    for case in eval_report.get("cases") or []:
        scores = case.get("eval_scores") or {}
        details = scores.get("details") or {}
        for scorer_name, scorer_details in details.items():
            if not isinstance(scorer_details, dict):
                continue
            if scorer_details.get("passed") is False:
                failures.append(
                    FailureCase(
                        case_id=case.get("case_id", ""),
                        input=case.get("input"),
                        output=case.get("output"),
                        expected=case.get("expected"),
                        scorer=scorer_name,
                        score=float(scorer_details.get("score", 0.0) or 0.0),
                        reason=str(scorer_details.get("error") or scorer_details.get("reason") or ""),
                    )
                )
    return failures


# ---------------------------------------------------------------------------
# Clustering
# ---------------------------------------------------------------------------


_CLUSTER_PROMPT = """\
You are clustering failed eval cases into failure modes. For each cluster
output a JSON object:

  {{ "label": "<short noun phrase>",
     "description": "<one-sentence cause>",
     "case_ids": ["id1", "id2", ...] }}

Cover every case exactly once. Output: a JSON array of clusters.
"""

_FIX_PROMPT = """\
You are an expert harness engineer. Given a cluster of failures, propose
1-3 concrete fixes. Each fix is a JSON object:

  {{ "kind": "prompt" | "policy" | "tool" | "memory",
     "target": "<prompt name / rule name / tool name / memory key>",
     "rationale": "<why>",
     "change": "<concrete change to apply>" }}

Output: a JSON array of fixes.
"""


_DECISION_RE = re.compile(r"\[[\s\S]*?\]")


def _strip_fence(text: str) -> str:
    text = (text or "").strip()
    if text.startswith("```"):
        parts = text.split("```")
        if len(parts) >= 3:
            text = "```".join(parts[1:-1]).strip()
            if text.startswith("json"):
                text = text[4:].strip()
    return text


def _safe_json_array(text: str) -> list[Any]:
    cleaned = _strip_fence(text)
    m = _DECISION_RE.search(cleaned)
    candidate = m.group(0) if m else cleaned
    try:
        parsed = json.loads(candidate)
        if isinstance(parsed, list):
            return parsed
    except json.JSONDecodeError:
        pass
    return []


def _adjective_clusters(failures: list[FailureCase]) -> list[FailureCluster]:
    """Fallback clustering when no LLM is configured: group by scorer."""
    by_scorer: dict[str, list[FailureCase]] = {}
    for f in failures:
        by_scorer.setdefault(f.scorer or "unknown", []).append(f)
    clusters: list[FailureCluster] = []
    for scorer, items in by_scorer.items():
        clusters.append(
            FailureCluster(
                cluster_id=f"cluster-{uuid.uuid4().hex[:8]}",
                label=f"{scorer} failures",
                description=f"All failures scored by '{scorer}'",
                cases=items,
            )
        )
    return clusters


def _llm_clusters(
    failures: list[FailureCase],
    *,
    spec: dict[str, Any],
    judge_adapter_kind: str,
    emit: EmitFn,
) -> tuple[list[FailureCluster], list[dict[str, Any]]]:
    """Ask the configured LLM to cluster failures + suggest fixes."""
    raw_suggestions: list[dict[str, Any]] = []
    if not failures:
        return [], raw_suggestions

    adapter = AdapterRegistry.get(judge_adapter_kind)
    failures_payload = [
        {
            "case_id": f.case_id,
            "input": (str(f.input)[:500] if f.input is not None else ""),
            "output": (str(f.output)[:500] if f.output is not None else ""),
            "expected": (str(f.expected)[:200] if f.expected is not None else ""),
            "scorer": f.scorer,
            "reason": f.reason,
        }
        for f in failures
    ]
    prompt = _CLUSTER_PROMPT + "\n\nCases:\n" + json.dumps(
        failures_payload, ensure_ascii=False
    )
    try:
        result = adapter.invoke({}, prompt, {"improvement_clustering": True})
    except Exception as e:  # noqa: BLE001
        log.warning("improvement_loop clustering LLM call failed: %s", e)
        return _adjective_clusters(failures), raw_suggestions

    raw_clusters = _safe_json_array(result.text or "")
    case_lookup = {f.case_id: f for f in failures}
    clusters: list[FailureCluster] = []
    for idx, entry in enumerate(raw_clusters):
        if not isinstance(entry, dict):
            continue
        ids = [str(x) for x in entry.get("case_ids") or []]
        cases = [case_lookup[i] for i in ids if i in case_lookup]
        if not cases:
            continue
        cluster = FailureCluster(
            cluster_id=f"cluster-{uuid.uuid4().hex[:8]}",
            label=str(entry.get("label", f"cluster-{idx}")),
            description=str(entry.get("description", "")),
            cases=cases,
        )
        # Now ask for fixes per cluster.
        fix_prompt = _FIX_PROMPT + "\n\nCluster:\n" + json.dumps(
            {
                "label": cluster.label,
                "description": cluster.description,
                "cases": [
                    {"case_id": c.case_id, "reason": c.reason, "scorer": c.scorer}
                    for c in cases
                ],
            },
            ensure_ascii=False,
        )
        try:
            fix_result = adapter.invoke(
                {"spec_excerpt": json.dumps(spec)[:1000]},
                fix_prompt,
                {"improvement_fix": True},
            )
        except Exception as e:  # noqa: BLE001
            log.warning("improvement_loop fix-generation failed: %s", e)
            continue
        fixes = _safe_json_array(fix_result.text or "")
        for f in fixes:
            if not isinstance(f, dict):
                continue
            f_dict = {
                "kind": str(f.get("kind", "prompt")),
                "target": str(f.get("target", "")),
                "rationale": str(f.get("rationale", "")),
                "change": str(f.get("change", "")),
            }
            cluster.suggested_fixes.append(f_dict)
            raw_suggestions.append({"cluster_id": cluster.cluster_id, **f_dict})
        clusters.append(cluster)

    if not clusters:
        return _adjective_clusters(failures), raw_suggestions

    emit(
        {
            "kind": "improvement_clusters",
            "payload": {
                "report_eval_set_id": spec.get("id", ""),
                "clusters": [
                    {
                        "cluster_id": c.cluster_id,
                        "label": c.label,
                        "case_count": len(c.cases),
                    }
                    for c in clusters
                ],
            },
        }
    )
    return clusters, raw_suggestions


# ---------------------------------------------------------------------------
# Public entry point
# ---------------------------------------------------------------------------


def run_improvement_loop(
    eval_report: dict[str, Any],
    spec: dict[str, Any],
    *,
    judge_adapter_kind: str | None = None,
    emit: EmitFn | None = None,
    generated_at_ms: int | None = None,
) -> ImprovementReport:
    """Analyze an eval report and produce an :class:`ImprovementReport`.

    Args:
        eval_report: Output of :func:`run_eval_set`.
        spec: DomainSpec used for the run (passed to the judge so it knows
            what prompt / tools exist).
        judge_adapter_kind: Optional adapter kind for the clustering LLM.
            When ``None``, falls back to heuristic clustering by scorer.
        emit: Optional observation sink.
        generated_at_ms: Override for testability.
    """
    import time

    emit = emit or (lambda _o: None)
    failures = _extract_failures(eval_report)
    total_cases = int(eval_report.get("total_cases") or 0)
    passed_cases = int(eval_report.get("passed_cases") or 0)
    passed_rate = (passed_cases / total_cases) if total_cases else 0.0

    if judge_adapter_kind:
        clusters, raw_suggestions = _llm_clusters(
            failures, spec=spec, judge_adapter_kind=judge_adapter_kind, emit=emit
        )
    else:
        clusters = _adjective_clusters(failures)
        raw_suggestions = []

    report = ImprovementReport(
        report_id=f"improvement-{uuid.uuid4().hex[:8]}",
        eval_set_id=str(eval_report.get("eval_set_id", "")),
        generated_at_ms=generated_at_ms if generated_at_ms is not None else int(time.time() * 1000),
        total_cases=total_cases,
        failed_cases=len(failures),
        passed_rate=passed_rate,
        clusters=clusters,
        raw_suggestions=raw_suggestions,
    )
    emit(
        {
            "kind": "improvement_report",
            "payload": report.to_dict(),
        }
    )
    return report


__all__ = [
    "FailureCase",
    "FailureCluster",
    "ImprovementReport",
    "run_improvement_loop",
]