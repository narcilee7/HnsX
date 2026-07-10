"""Eval scorers for W8.

Each scorer is a callable ``(expected, actual, **kwargs) -> Score`` where
``expected`` comes from the EvalCase and ``actual`` is the session output
(usually the final assistant text or the last observation).

Scorers emit an ``eval_score`` observation via the ``emit`` callable when
run through :func:`score`.
"""

from __future__ import annotations

import json
import logging
import re
from collections.abc import Callable
from dataclasses import dataclass
from typing import Any

try:
    import jmespath
except ImportError:  # pragma: no cover
    jmespath = None  # type: ignore[assignment]

from hnsx_worker.adapters import AdapterRegistry

log = logging.getLogger("hnsx_worker.eval.scorers")


@dataclass
class Score:
    """Result of one scorer invocation."""

    score: float  # 0.0 .. 1.0 typically
    passed: bool
    details: dict[str, Any]


ScorerFn = Callable[..., Score]


def score(
    name: str,
    expected: Any,
    actual: Any,
    *,
    case_id: str = "",
    emit: Callable[[dict], None] | None = None,
    **kwargs: Any,
) -> Score:
    """Run a named scorer and optionally emit an ``eval_score`` observation."""
    fn = _SCORERS.get(name)
    if fn is None:
        result = Score(score=0.0, passed=False, details={"error": f"unknown scorer {name!r}"})
    else:
        try:
            result = fn(expected, actual, **kwargs)
        except Exception as e:  # noqa: BLE001
            result = Score(score=0.0, passed=False, details={"error": str(e)})

    if emit is not None:
        emit(
            {
                "kind": "eval_score",
                "payload": {
                    "case_id": case_id,
                    "scorer": name,
                    "score": result.score,
                    "passed": result.passed,
                    "details": result.details,
                },
            }
        )
    return result


# ---------------------------------------------------------------------------
# Built-in scorers
# ---------------------------------------------------------------------------


def exact(expected: Any, actual: Any, **_: Any) -> Score:
    """Exact equality scorer."""
    passed = expected == actual
    return Score(
        score=1.0 if passed else 0.0,
        passed=passed,
        details={"expected": expected, "actual": actual},
    )


def contains(expected: Any, actual: Any, **_: Any) -> Score:
    """Check that ``expected`` is a substring of ``actual``."""
    expected_str = str(expected)
    actual_str = str(actual)
    passed = expected_str in actual_str
    return Score(
        score=1.0 if passed else 0.0,
        passed=passed,
        details={"needle": expected_str, "haystack_len": len(actual_str)},
    )


def regex(expected: Any, actual: Any, **_: Any) -> Score:
    """Check that ``actual`` matches the regex ``expected``."""
    pattern_str = str(expected)
    actual_str = str(actual)
    try:
        passed = re.search(pattern_str, actual_str) is not None
    except re.error as e:
        return Score(score=0.0, passed=False, details={"error": f"invalid regex: {e}"})
    return Score(
        score=1.0 if passed else 0.0,
        passed=passed,
        details={"pattern": pattern_str, "matched": passed},
    )


def jmespath_scorer(expected: Any, actual: Any, **_: Any) -> Score:
    """Evaluate a JMESPath expression against ``actual`` and check truthiness."""
    if jmespath is None:
        return Score(
            score=0.0,
            passed=False,
            details={"error": "jmespath not installed"},
        )
    expr_str = str(expected)
    try:
        value = jmespath.search(expr_str, actual)
    except Exception as e:  # noqa: BLE001
        return Score(score=0.0, passed=False, details={"error": str(e)})
    passed = bool(value)
    return Score(
        score=1.0 if passed else 0.0,
        passed=passed,
        details={"expression": expr_str, "value": value},
    )


def structured_match(expected: Any, actual: Any, **_: Any) -> Score:
    """Best-effort JSON schema-ish match.

    ``expected`` is a dict describing the expected shape:

        {"type": "object", "properties": {"answer": {"type": "string"}}}

    Only ``type`` and top-level ``properties`` / ``required`` are checked.
    """
    if not isinstance(expected, dict):
        return Score(
            score=0.0,
            passed=False,
            details={"error": "expected_schema must be a dict"},
        )

    errors: list[str] = []

    expected_type = expected.get("type")
    if expected_type:
        type_map = {
            "object": dict,
            "array": list,
            "string": str,
            "number": (int, float),
            "integer": int,
            "boolean": bool,
            "null": type(None),
        }
        if expected_type in type_map and not isinstance(actual, type_map[expected_type]):
            errors.append(f"expected type {expected_type}, got {type(actual).__name__}")

    if isinstance(actual, dict):
        required = expected.get("required") or []
        for key in required:
            if key not in actual:
                errors.append(f"missing required key {key!r}")

        properties = expected.get("properties") or {}
        for key, prop_schema in properties.items():
            if key not in actual:
                continue
            prop_type = prop_schema.get("type") if isinstance(prop_schema, dict) else None
            if prop_type:
                type_map = {
                    "object": dict,
                    "array": list,
                    "string": str,
                    "number": (int, float),
                    "integer": int,
                    "boolean": bool,
                    "null": type(None),
                }
                if prop_type in type_map and not isinstance(
                    actual[key], type_map[prop_type]
                ):
                    errors.append(
                        f"key {key!r} expected type {prop_type}, "
                        f"got {type(actual[key]).__name__}"
                    )

    passed = not errors
    return Score(
        score=1.0 if passed else 0.0,
        passed=passed,
        details={"errors": errors},
    )


def llm_judge(
    expected: Any,
    actual: Any,
    *,
    model: str = "claude-haiku-4-5",
    adapter: str | None = None,
    **_: Any,
) -> Score:
    """LLM-as-judge scorer with a deterministic fallback.

    If ``adapter`` is provided and registered, the scorer invokes that adapter
    with a judge prompt and parses the returned verdict JSON. Otherwise it
    falls back to a keyword-presence heuristic so the eval pipeline stays
    runnable without external API calls.

    Expected verdict shape::

        {"verdict": "pass" | "partial" | "fail", "score": 0.0..1.0, "reason": "..."}
    """
    criteria_str = str(expected)
    actual_str = str(actual)

    if adapter:
        try:
            return _llm_judge_with_adapter(
                criteria_str, actual_str, adapter_kind=adapter, model=model
            )
        except Exception as e:  # noqa: BLE001
            log.warning("llm_judge adapter call failed: %s; falling back", e)

    # Deterministic fallback: if criteria is a string, treat presence of any
    # keyword as a positive signal.
    keywords = [w for w in re.findall(r"\w+", criteria_str.lower()) if len(w) > 3]
    if keywords:
        matches = sum(1 for kw in keywords if kw in actual_str.lower())
        ratio = matches / len(keywords)
        passed = ratio >= 0.5
        return Score(
            score=ratio,
            passed=passed,
            details={"model": model, "criteria": criteria_str, "matches": matches},
        )
    return Score(
        score=0.0,
        passed=False,
        details={"model": model, "criteria": criteria_str, "note": "no keywords"},
    )


def _llm_judge_with_adapter(
    criteria: str,
    actual: str,
    *,
    adapter_kind: str,
    model: str,
) -> Score:
    """Call the configured adapter and parse a verdict JSON."""
    adapter = AdapterRegistry.get(adapter_kind)
    prompt = (
        "You are an evaluator. Given the criteria and the actual output, "
        'return a JSON object with exactly these keys: '
        '{"verdict": "pass" | "partial" | "fail", "score": 0.0..1.0, "reason": "..."}.\n\n'
        f"Criteria: {criteria}\n\nActual output: {actual}\n\nVerdict JSON:"
    )
    agent = {
        "id": "llm-judge",
        "adapter": {"kind": adapter_kind},
        "model": model,
        "system_prompt": prompt,
    }
    result = adapter.invoke(agent, prompt, {"content": ""})
    return _parse_judge_verdict(result.text, model=model, criteria=criteria)


def _parse_judge_verdict(text: str, *, model: str, criteria: str) -> Score:
    """Best-effort parse the verdict JSON returned by a judge model."""
    # Try to extract a fenced JSON block first, then fall back to the raw text.
    block_match = re.search(r"```(?:json)?\s*(\{.*?\})\s*```", text, re.DOTALL)
    raw = block_match.group(1) if block_match else text
    raw = raw.strip()

    try:
        data = json.loads(raw)
    except json.JSONDecodeError:
        # Last resort: grab the first JSON-ish object in the text.
        obj_match = re.search(r"\{.*?\}", raw, re.DOTALL)
        if not obj_match:
            raise ValueError(f"judge returned non-JSON verdict: {text!r}") from None
        data = json.loads(obj_match.group(0))

    verdict = str(data.get("verdict", "")).lower()
    score = float(data.get("score", 0.0))
    reason = str(data.get("reason", ""))

    if verdict == "pass":
        passed = True
        score = max(score, 1.0)
    elif verdict == "partial":
        passed = score >= 0.5
    else:
        passed = False

    return Score(
        score=max(0.0, min(1.0, score)),
        passed=passed,
        details={"model": model, "criteria": criteria, "verdict": verdict, "reason": reason},
    )


_SCORERS: dict[str, ScorerFn] = {
    "exact": exact,
    "contains": contains,
    "regex": regex,
    "jmespath": jmespath_scorer,
    "structured_match": structured_match,
    "llm_judge": llm_judge,
}


def list_scorers() -> list[str]:
    """Return the names of available built-in scorers."""
    return sorted(_SCORERS.keys())


__all__ = [
    "Score",
    "ScorerFn",
    "contains",
    "exact",
    "jmespath_scorer",
    "list_scorers",
    "llm_judge",
    "regex",
    "score",
    "structured_match",
]
