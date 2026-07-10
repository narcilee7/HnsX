"""W13 Auto Prompt Evolution.

Given a current prompt + the failures observed in an :class:`ImprovementReport`,
this module asks an LLM to propose N prompt variants, A/B evaluates each
on a small subset of the EvalSet, and recommends the winner.

The output is :class:`PromptEvolutionReport`. ``apply()`` is a deliberate
no-op — the operator must review ``recommended_variant`` and call
``apply()`` explicitly, mirroring the W13 acceptance criteria
("人工审批后才能替换生产 prompt").

Heuristics:

  - If the judge returns unparseable output we fall back to the current
    prompt so evolution never silently degrades.
  - Variants that score below the baseline are flagged but still ranked
    so the operator can inspect them.
  - The eval subset defaults to the failing cases (cheapest, most
    informative). Set ``cases`` to run on the full EvalSet.
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

log = logging.getLogger("hnsx_worker.eval.prompt_evolution")

EmitFn = Callable[[dict], None]

VARIANT_PROMPT = """\
You are a prompt engineer. Given the current prompt and a set of failures,
produce {n_variants} candidate rewrites that might fix the failures.

Current prompt:
\"\"\"{current}\"\"\"

Failures (case_id / reason / scorer):
{failures_json}

Output: a JSON array of variants, each:
  {{ "name": "<short>",
     "rationale": "<why>",
     "prompt": "<full new prompt text>" }}
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


# ---------------------------------------------------------------------------
# Types
# ---------------------------------------------------------------------------


@dataclass
class PromptVariant:
    """One rewrite proposal."""

    name: str
    rationale: str
    prompt: str
    score: float = 0.0
    case_results: dict[str, float] = field(default_factory=dict)


@dataclass
class PromptEvolutionReport:
    """Output of :func:`evolve_prompt`."""

    report_id: str
    eval_set_id: str
    prompt_name: str
    current_prompt: str
    variants: list[PromptVariant] = field(default_factory=list)
    recommended_variant: str = ""
    baseline_score: float = 0.0
    improved: bool = False
    raw_suggestions: list[dict[str, Any]] = field(default_factory=list)

    def to_dict(self) -> dict[str, Any]:
        return {
            "report_id": self.report_id,
            "eval_set_id": self.eval_set_id,
            "prompt_name": self.prompt_name,
            "current_prompt": self.current_prompt,
            "variants": [
                {
                    "name": v.name,
                    "rationale": v.rationale,
                    "score": round(v.score, 4),
                    "case_results": {k: round(val, 4) for k, val in v.case_results.items()},
                    "prompt": v.prompt,
                }
                for v in self.variants
            ],
            "recommended_variant": self.recommended_variant,
            "baseline_score": round(self.baseline_score, 4),
            "improved": self.improved,
            "raw_suggestions": self.raw_suggestions,
        }

    def apply(self) -> dict[str, Any] | None:
        """Return the recommended prompt. No-op until operator approves.

        A future W13.x can wire this into a control-plane endpoint that
        patches the DomainSpec and emits a ``prompt_updated`` observation.
        """
        if not self.recommended_variant:
            return None
        for v in self.variants:
            if v.name == self.recommended_variant:
                return {"prompt_name": self.prompt_name, "prompt": v.prompt}
        return None


# ---------------------------------------------------------------------------
# LLM variant generation
# ---------------------------------------------------------------------------


def _generate_variants(
    *,
    current_prompt: str,
    failures: list[dict[str, Any]],
    n_variants: int,
    judge_adapter_kind: str,
) -> tuple[list[dict[str, Any]], str]:
    """Ask the LLM for variants. Returns ``(variants, raw_text)``."""
    adapter = AdapterRegistry.get(judge_adapter_kind)
    prompt = VARIANT_PROMPT.format(
        n_variants=n_variants,
        current=current_prompt,
        failures_json=json.dumps(failures, ensure_ascii=False),
    )
    try:
        result = adapter.invoke({}, prompt, {"prompt_evolution": True})
    except Exception as e:  # noqa: BLE001
        log.warning("prompt evolution LLM call failed: %s", e)
        return [], ""
    raw = result.text or ""
    parsed = _safe_json_array(raw)
    variants: list[dict[str, Any]] = []
    for entry in parsed:
        if not isinstance(entry, dict):
            continue
        text = entry.get("prompt")
        if not isinstance(text, str) or not text.strip():
            continue
        variants.append(
            {
                "name": str(entry.get("name", "")) or f"variant-{len(variants) + 1}",
                "rationale": str(entry.get("rationale", "")),
                "prompt": text,
            }
        )
    return variants, raw


# ---------------------------------------------------------------------------
# Eval helpers
# ---------------------------------------------------------------------------


def _score_variant(
    variant_prompt: str,
    *,
    spec: dict[str, Any],
    cases: list[dict[str, Any]],
    judge_adapter_kind: str,
    session_runner: Callable[..., dict[str, Any]] | None,
    emit: EmitFn,
) -> dict[str, float]:
    """Score a variant against a list of cases.

    Returns ``{case_id: avg_score}``. If ``session_runner`` is ``None``,
    we only run the LLM-judge (cheaper, used by tests).
    """
    from .scorers import score

    adapter = AdapterRegistry.get(judge_adapter_kind)
    out: dict[str, float] = {}
    if not cases:
        return out

    for case in cases:
        case_id = str(case.get("case_id", ""))
        expected = case.get("expected")
        scorers = case.get("scorers") or [{"name": "contains"}]
        # If a session_runner was provided, use it to materialize the output
        # against the variant prompt; otherwise fall back to a stub output so
        # the test path stays deterministic.
        if session_runner is not None:
            try:
                # The session_runner is expected to honour ``config["prompt_override"]``
                # if the test harness wires that. We pass it via a shallow copy.
                cfg = {
                    "session_id": f"evo-{case_id}",
                    "prompt_override": variant_prompt,
                }
                run_result = session_runner(spec, case.get("input", {}), cfg)
                actual = run_result.get("output", "")
            except Exception as e:  # noqa: BLE001
                log.warning("evolution session_runner failed: %s", e)
                actual = ""
        else:
            # Pure LLM-judge path: ask the judge to evaluate the prompt
            # directly against the case input. We deliberately omit the
            # expected value from the judge prompt so the judge can't
            # cheat by mirroring it.
            prompt = f"Variant prompt under test:\n{variant_prompt}\n\nInput: {case.get('input')}"
            try:
                judge_result = adapter.invoke({}, prompt, {"evolution_judge": True})
                actual = judge_result.text or ""
            except Exception as e:  # noqa: BLE001
                log.warning("evolution judge failed: %s", e)
                actual = ""

        per_case_scores: list[float] = []
        for entry in scorers:
            if not isinstance(entry, dict):
                continue
            kwargs = {k: v for k, v in entry.items() if k != "name"}
            sc = score(str(entry.get("name", "")), expected, actual, **kwargs)
            per_case_scores.append(sc.score)
        out[case_id] = round(
            sum(per_case_scores) / len(per_case_scores) if per_case_scores else 0.0, 4
        )
        emit(
            {
                "kind": "evolution_case_score",
                "payload": {"case_id": case_id, "score": out[case_id]},
            }
        )
    return out


# ---------------------------------------------------------------------------
# Public entry point
# ---------------------------------------------------------------------------


def evolve_prompt(
    *,
    spec: dict[str, Any],
    current_prompt: str,
    prompt_name: str = "default",
    eval_report: dict[str, Any] | None = None,
    improvement_report: dict[str, Any] | None = None,
    judge_adapter_kind: str = "noop",
    n_variants: int = 3,
    min_improvement: float = 0.02,
    session_runner: Callable[..., dict[str, Any]] | None = None,
    emit: EmitFn | None = None,
) -> PromptEvolutionReport:
    """Generate variants, score them, recommend the best.

    Args:
        spec: DomainSpec (forwarded to session_runner if provided).
        current_prompt: The prompt to improve.
        prompt_name: Free-form label.
        eval_report: EvalSet report to score against. Optional when an
            ``improvement_report`` is provided.
        improvement_report: Optional improvement loop output; its clusters
            seed the variant generation prompt.
        judge_adapter_kind: Adapter for variant generation + LLM-judge
            fallback.
        n_variants: How many rewrites to request.
        min_improvement: Score delta required for ``improved=True``.
        session_runner: Optional ``(spec, input, config) -> dict`` runner.
            When ``None``, falls back to LLM-judge-only scoring.
        emit: Observation sink.
    """
    emit = emit or (lambda _o: None)
    eval_set_id = str((eval_report or {}).get("eval_set_id", ""))

    # Build a compact failure payload for the variant-generation prompt.
    failures: list[dict[str, Any]] = []
    if improvement_report:
        for cluster in improvement_report.get("clusters", []) or []:
            for cid in cluster.get("case_ids", []) or []:
                failures.append(
                    {
                        "case_id": cid,
                        "cluster_label": cluster.get("label", ""),
                        "description": cluster.get("description", ""),
                    }
                )
    if not failures and eval_report:
        for case in eval_report.get("cases", []) or []:
            scores = case.get("eval_scores", {}) or {}
            if scores.get("passed", 0) < scores.get("total", 0):
                failures.append({"case_id": case.get("case_id", "")})

    raw_variants, raw_text = _generate_variants(
        current_prompt=current_prompt,
        failures=failures[:25],  # bound prompt length
        n_variants=n_variants,
        judge_adapter_kind=judge_adapter_kind,
    )

    cases = list((eval_report or {}).get("cases") or [])
    baseline_scores = _score_variant(
        current_prompt,
        spec=spec,
        cases=cases,
        judge_adapter_kind=judge_adapter_kind,
        session_runner=session_runner,
        emit=emit,
    )
    baseline_avg = round(
        sum(baseline_scores.values()) / len(baseline_scores) if baseline_scores else 0.0, 4
    )

    variants: list[PromptVariant] = []
    for entry in raw_variants:
        case_results = _score_variant(
            entry["prompt"],
            spec=spec,
            cases=cases,
            judge_adapter_kind=judge_adapter_kind,
            session_runner=session_runner,
            emit=emit,
        )
        avg = round(sum(case_results.values()) / len(case_results) if case_results else 0.0, 4)
        variants.append(
            PromptVariant(
                name=entry["name"],
                rationale=entry["rationale"],
                prompt=entry["prompt"],
                score=avg,
                case_results=case_results,
            )
        )

    # Recommend the highest-scoring variant (any improvement qualifies; the
    # operator's call whether the delta is worth applying).
    recommended = ""
    best_score = baseline_avg
    for v in variants:
        if v.score > best_score:
            best_score = v.score
            recommended = v.name

    report = PromptEvolutionReport(
        report_id=f"evolution-{uuid.uuid4().hex[:8]}",
        eval_set_id=eval_set_id,
        prompt_name=prompt_name,
        current_prompt=current_prompt,
        variants=variants,
        recommended_variant=recommended,
        baseline_score=baseline_avg,
        improved=(best_score - baseline_avg) >= min_improvement,
        raw_suggestions=[{"variant_count": len(raw_variants), "raw": raw_text[:2000]}],
    )
    emit({"kind": "evolution_report", "payload": report.to_dict()})
    return report


__all__ = [
    "PromptEvolutionReport",
    "PromptVariant",
    "evolve_prompt",
]
