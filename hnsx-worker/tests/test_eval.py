"""Tests for W8 Eval scoring platform.

Covers the built-in scorers, the eval runner/aggregator, and the
session_executor integration that emits ``eval_score`` observations and
writes ``eval_scores`` into the session result.
"""

from __future__ import annotations

import json
import subprocess
import sys
import threading
from typing import Any

from hnsx_worker.adapters import AdapterRegistry
from hnsx_worker.adapters.base import Adapter, AdapterResult
from hnsx_worker.eval import aggregate_scores, compare_eval_reports, run_eval, run_eval_set
from hnsx_worker.eval.scorers import Score, score
from hnsx_worker.session_executor import execute_session


def _noop_spec() -> dict[str, Any]:
    return {
        "id": "eval-domain",
        "version": "0.1.0",
        "harness": {
            "agents": {
                "primary": {
                    "id": "primary",
                    "provider": "noop",
                    "model": "noop-1",
                    "adapter": {"kind": "noop"},
                    "system_prompt": "hello",
                }
            },
            "session": {"mode": "single-task", "agent": "primary"},
        },
    }


# ---------------------------------------------------------------------------
# scorer unit tests
# ---------------------------------------------------------------------------


def test_exact_scorer_passes_and_fails() -> None:
    assert score("exact", "ok", "ok").passed is True
    assert score("exact", "ok", "not ok").passed is False


def test_contains_scorer() -> None:
    assert score("contains", "needle", "a needle here").passed is True
    assert score("contains", "needle", "no match").passed is False


def test_regex_scorer() -> None:
    assert score("regex", r"\d+", "order 42").passed is True
    assert score("regex", r"^\d+$", "order 42").passed is False


def test_regex_scorer_reports_invalid_pattern() -> None:
    s = score("regex", r"[", "input")
    assert s.passed is False
    assert "invalid regex" in s.details["error"]


def test_structured_match_passes() -> None:
    schema = {
        "type": "object",
        "required": ["answer"],
        "properties": {"answer": {"type": "string"}},
    }
    s = score("structured_match", schema, {"answer": "42"})
    assert s.passed is True
    assert s.score == 1.0


def test_structured_match_fails_on_wrong_type() -> None:
    schema = {"type": "object"}
    s = score("structured_match", schema, ["not", "an", "object"])
    assert s.passed is False
    assert "expected type object" in s.details["errors"][0]


def test_llm_judge_keyword_fallback() -> None:
    s = score("llm_judge", "contains helpful explanation", "This is a helpful explanation.")
    assert s.passed is True
    assert s.score > 0.0


def test_unknown_scorer_returns_failure() -> None:
    s = score("not_a_scorer", "expected", "actual")
    assert s.passed is False
    assert "unknown scorer" in s.details["error"]


# ---------------------------------------------------------------------------
# runner / aggregator
# ---------------------------------------------------------------------------


def test_run_eval_runs_all_scorers() -> None:
    eval_case = {
        "case_id": "c1",
        "expected": "hello",
        "scorers": [
            {"name": "exact"},
            {"name": "contains"},
        ],
    }
    results = run_eval(eval_case, "hello")
    assert set(results.keys()) == {"exact", "contains"}
    assert results["exact"].passed is True
    assert results["contains"].passed is True


def test_aggregate_scores_computes_summary() -> None:
    results = {
        "a": Score(score=1.0, passed=True, details={}),
        "b": Score(score=0.0, passed=False, details={}),
    }
    summary = aggregate_scores(results)
    assert summary["total"] == 2
    assert summary["passed"] == 1
    assert summary["score"] == 0.5
    assert "details" in summary


def test_aggregate_scores_handles_empty() -> None:
    assert aggregate_scores({}) == {"total": 0, "passed": 0, "score": 0.0}


# ---------------------------------------------------------------------------
# executor integration
# ---------------------------------------------------------------------------


def test_execute_session_runs_eval_and_emits_observations() -> None:
    spec = _noop_spec()
    config: dict[str, Any] = {
        "session_id": "s-eval",
        "eval_case": {
            "case_id": "noop-case",
            "expected": "agent=primary",
            "scorers": [
                {"name": "contains"},
                {"name": "regex"},
            ],
        },
    }
    obs: list[dict] = []
    result = execute_session(
        spec,
        {"question": "hi"},
        config,
        stop_event=threading.Event(),
        emit=lambda o: obs.append(o),
    )

    assert "eval_scores" in result
    eval_scores = result["eval_scores"]
    assert eval_scores["total"] == 2
    assert eval_scores["passed"] == 2
    assert eval_scores["score"] == 1.0

    eval_obs = [o for o in obs if o["kind"] == "eval_score"]
    assert len(eval_obs) == 2
    assert all(o["payload"]["case_id"] == "noop-case" for o in eval_obs)


# ---------------------------------------------------------------------------
# session_runtime subprocess integration
# ---------------------------------------------------------------------------


def test_session_runtime_includes_eval_scores_in_session_end() -> None:
    spec = _noop_spec()
    payload = {
        "session_id": "s-runtime-eval",
        "domain_spec_json": json.dumps(spec),
        "trigger_payload_json": json.dumps({"question": "hi"}),
        "eval_case": {
            "case_id": "runtime-case",
            "expected": "noop",
            "scorers": [{"name": "contains"}],
        },
    }
    proc = subprocess.run(
        [sys.executable, "-m", "hnsx_worker.session_runtime"],
        input=json.dumps(payload),
        capture_output=True,
        text=True,
        timeout=15,
    )
    assert proc.returncode == 0, f"stderr: {proc.stderr}"
    obs = [json.loads(line) for line in proc.stdout.splitlines() if line.strip()]
    end = [o for o in obs if o["kind"] == "session_end"][-1]
    result = end.get("payload", {}).get("result", {})
    eval_scores = result.get("eval_scores", {})
    assert eval_scores.get("passed") == 1
    assert eval_scores.get("score") == 1.0


# ---------------------------------------------------------------------------
# W9: real LLM judge + EvalSet
# ---------------------------------------------------------------------------


class _FakeJudgeAdapter(Adapter):
    """Stub judge that returns a JSON verdict based on prompt keywords."""

    def name(self) -> str:
        return "fake_judge"

    def invoke(self, agent: dict, prompt: str, input: dict) -> AdapterResult:
        verdict = "pass" if "good" in prompt.lower() else "fail"
        score = 1.0 if verdict == "pass" else 0.0
        return AdapterResult(
            text=json.dumps(
                {"verdict": verdict, "score": score, "reason": "fake judge reasoning"}
            )
        )


def test_llm_judge_with_adapter_parses_verdict() -> None:
    AdapterRegistry.register("fake_judge", _FakeJudgeAdapter)
    try:
        s = score(
            "llm_judge",
            "the output mentions good performance",
            "good performance",
            adapter="fake_judge",
        )
        assert s.passed is True
        assert s.score == 1.0
        assert s.details["verdict"] == "pass"

        s2 = score(
            "llm_judge",
            "the output mentions bad performance",
            "bad performance",
            adapter="fake_judge",
        )
        assert s2.passed is False
        assert s2.score == 0.0
    finally:
        AdapterRegistry._registry.pop("fake_judge", None)  # type: ignore[attr-defined]
        AdapterRegistry._singletons.pop("fake_judge", None)  # type: ignore[attr-defined]


def test_run_eval_set_runs_multiple_cases() -> None:
    spec = _noop_spec()
    eval_set = {
        "eval_set_id": "es-1",
        "cases": [
            {
                "case_id": "c1",
                "input": {"question": "hi"},
                "expected": "noop",
                "scorers": [{"name": "contains"}],
            },
            {
                "case_id": "c2",
                "input": {"question": "hi"},
                "expected": "missing",
                "scorers": [{"name": "contains"}],
            },
        ],
    }
    report = run_eval_set(
        eval_set,
        execute_session,
        spec,
        {"session_id": "s-es"},
        stop_event=threading.Event(),
    )
    assert report["eval_set_id"] == "es-1"
    assert report["total_cases"] == 2
    assert len(report["cases"]) == 2
    assert report["passed_cases"] == 1
    assert 0.0 < report["avg_score"] < 1.0


def test_compare_eval_reports_detects_regression_and_improvement() -> None:
    baseline = {
        "eval_set_id": "base",
        "cases": [
            {"case_id": "a", "eval_scores": {"score": 1.0}},
            {"case_id": "b", "eval_scores": {"score": 0.0}},
        ],
    }
    current = {
        "eval_set_id": "cur",
        "cases": [
            {"case_id": "a", "eval_scores": {"score": 0.0}},
            {"case_id": "b", "eval_scores": {"score": 1.0}},
            {"case_id": "c", "eval_scores": {"score": 0.5}},
        ],
    }
    comparison = compare_eval_reports(current, baseline)
    assert comparison["regressions"] == 1
    assert comparison["improvements"] == 1
    assert comparison["new"] == 1
    assert comparison["unchanged"] == 0

    a_change = [c for c in comparison["changes"] if c["case_id"] == "a"][0]
    assert a_change["change"] == "regression"


def test_execute_session_with_eval_set_returns_report() -> None:
    spec = _noop_spec()
    config: dict[str, Any] = {
        "session_id": "s-es-exec",
        "eval_set": {
            "eval_set_id": "es-exec",
            "cases": [
                {
                    "case_id": "only",
                    "input": {"question": "hi"},
                    "expected": "noop",
                    "scorers": [{"name": "contains"}],
                }
            ],
        },
    }
    result = execute_session(
        spec,
        {"question": "hi"},
        config,
        stop_event=threading.Event(),
        emit=lambda o: None,
    )
    assert "eval_report" in result
    report = result["eval_report"]
    assert report["eval_set_id"] == "es-exec"
    assert report["passed_cases"] == 1
    assert isinstance(result["output"], str)
