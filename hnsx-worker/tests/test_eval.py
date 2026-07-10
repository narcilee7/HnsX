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

from hnsx_worker.eval import aggregate_scores, run_eval
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
