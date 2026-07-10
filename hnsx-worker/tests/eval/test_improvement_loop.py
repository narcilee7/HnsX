"""Tests for the W13 improvement loop."""

from __future__ import annotations

import pytest

from hnsx_worker.eval.improvement_loop import (
    FailureCase,
    FailureCluster,
    ImprovementReport,
    _extract_failures,
    _adjective_clusters,
    run_improvement_loop,
)


def _sample_report() -> dict:
    return {
        "eval_set_id": "es-1",
        "total_cases": 3,
        "passed_cases": 1,
        "cases": [
            {
                "case_id": "c1",
                "input": {"q": "refund?"},
                "output": "yes",
                "expected": "refund_issued",
                "eval_scores": {
                    "total": 2,
                    "passed": 1,
                    "score": 0.5,
                    "details": {
                        "contains": {"score": 1.0, "passed": True},
                        "exact": {
                            "score": 0.0,
                            "passed": False,
                            "error": "expected=refund_issued got=yes",
                        },
                    },
                },
            },
            {
                "case_id": "c2",
                "input": {"q": "reset pwd"},
                "output": "use /reset",
                "expected": "reset_instructions",
                "eval_scores": {
                    "total": 1,
                    "passed": 0,
                    "score": 0.0,
                    "details": {
                        "jmespath": {
                            "score": 0.0,
                            "passed": False,
                            "error": "missing key 'reset_url'",
                        },
                    },
                },
            },
            {
                "case_id": "c3",
                "input": {"q": "thanks"},
                "output": "you are welcome",
                "expected": "you are welcome",
                "eval_scores": {
                    "total": 1,
                    "passed": 1,
                    "score": 1.0,
                    "details": {"exact": {"score": 1.0, "passed": True}},
                },
            },
        ],
    }


def test_extract_failures_pulls_only_failed_scorers() -> None:
    failures = _extract_failures(_sample_report())
    assert len(failures) == 2
    assert {f.case_id for f in failures} == {"c1", "c2"}


def test_adjective_clusters_groups_by_scorer() -> None:
    failures = _extract_failures(_sample_report())
    clusters = _adjective_clusters(failures)
    assert len(clusters) == 2
    labels = {c.label for c in clusters}
    assert "exact failures" in labels
    assert "jmespath failures" in labels


def test_run_improvement_loop_without_judge_uses_heuristic() -> None:
    observations: list[dict] = []
    report = run_improvement_loop(
        _sample_report(),
        {"id": "demo"},
        emit=observations.append,
        generated_at_ms=12345,
    )
    assert isinstance(report, ImprovementReport)
    assert report.total_cases == 3
    assert report.failed_cases == 2
    assert report.passed_rate == pytest.approx(1 / 3)
    assert len(report.clusters) >= 1
    assert any(o["kind"] == "improvement_report" for o in observations)
    payload = observations[-1]["payload"]
    assert payload["report_id"] == report.report_id
    assert payload["passed_rate"] == pytest.approx(round(1 / 3, 4))


def test_improvement_report_apply_is_safe_noop() -> None:
    report = ImprovementReport(
        report_id="x",
        eval_set_id="y",
        generated_at_ms=0,
        total_cases=1,
        failed_cases=1,
        passed_rate=0.0,
        clusters=[
            FailureCluster(
                cluster_id="c",
                label="L",
                description="d",
                cases=[
                    FailureCase(
                        case_id="c1",
                        input=None,
                        output="o",
                        expected="e",
                        scorer="exact",
                        score=0.0,
                    )
                ],
            )
        ],
    )
    # Should not raise; intentionally no-op.
    report.apply()
    assert report.to_dict()["clusters"][0]["case_ids"] == ["c1"]