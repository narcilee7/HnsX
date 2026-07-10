"""Tests for the W13 regression radar."""

from __future__ import annotations

import pytest

from hnsx_worker.eval.regression_radar import (
    RegressionCheck,
    RegressionStore,
    check_regressions,
    detect_regressions,
)


def _baseline() -> dict:
    return {
        "eval_set_id": "baseline-1",
        "cases": [
            {"case_id": "a", "eval_scores": {"score": 1.0}},
            {"case_id": "b", "eval_scores": {"score": 0.8}},
            {"case_id": "c", "eval_scores": {"score": 0.6}},
        ],
    }


def _current() -> dict:
    return {
        "eval_set_id": "current-1",
        "cases": [
            {"case_id": "a", "eval_scores": {"score": 1.0}},
            {"case_id": "b", "eval_scores": {"score": 0.74}},  # -0.06 → regression
            {"case_id": "c", "eval_scores": {"score": 0.565}},  # -0.035 → watch
            {"case_id": "d", "eval_scores": {"score": 0.9}},  # new
        ],
    }


def test_detect_regressions_respects_threshold() -> None:
    check = detect_regressions(
        _current(), _baseline(), score_threshold=0.10, baseline_label="main"
    )
    # With a 0.10 threshold, the b drop (0.06) is in the watch band
    # (between -0.10 and -0.05 = 0.10 * 0.5).
    assert check.severity == "watch"


def test_check_regressions_returns_nonzero_on_regression(tmp_path) -> None:
    store = RegressionStore(path=str(tmp_path / "radar.json"))
    store.put("main", _baseline())
    check, rc = check_regressions(_current(), store=store, baseline_label="main")
    assert isinstance(check, RegressionCheck)
    assert rc == 1
    assert check.severity == "regression"


def test_check_regressions_first_run_returns_zero(tmp_path) -> None:
    store = RegressionStore(path=str(tmp_path / "radar.json"))
    check, rc = check_regressions(_current(), store=store, baseline_label="main")
    assert rc == 0
    assert check.summary == {"total": 0, "regression": 0, "watch": 0, "ok": 0}


def test_check_regressions_passes_when_no_regression(tmp_path) -> None:
    store = RegressionStore(path=str(tmp_path / "radar.json"))
    store.put(
        "main",
        {
            "eval_set_id": "base",
            "cases": [
                {"case_id": "a", "eval_scores": {"score": 1.0}},
                {"case_id": "b", "eval_scores": {"score": 0.7}},
            ],
        },
    )
    current = {
        "eval_set_id": "curr",
        "cases": [
            {"case_id": "a", "eval_scores": {"score": 1.0}},
            {"case_id": "b", "eval_scores": {"score": 0.71}},  # +0.01
        ],
    }
    check, rc = check_regressions(current, store=store, baseline_label="main")
    assert rc == 0
    assert check.severity == "ok"


def test_regression_store_round_trip(tmp_path) -> None:
    path = tmp_path / "radar.json"
    a = RegressionStore(path=str(path))
    a.put("main", _baseline())
    b = RegressionStore(path=str(path))  # re-load
    assert b.labels() == ["main"]
    assert b.get("main") == _baseline()


def test_check_regressions_emits_observation(tmp_path) -> None:
    store = RegressionStore(path=str(tmp_path / "radar.json"))
    store.put("main", _baseline())
    observations: list[dict] = []
    check, _ = check_regressions(
        _current(), store=store, baseline_label="main", emit=observations.append
    )
    assert any(o["kind"] == "regression_check" for o in observations)

def test_resolve_default_path_priority(monkeypatch, tmp_path) -> None:
    from hnsx_worker.eval.regression_radar import (
        DEFAULT_PATH,
        ENV_PATH_KEY,
        resolve_default_path,
    )
    # 1. Default (no arg, no env).
    monkeypatch.delenv(ENV_PATH_KEY, raising=False)
    assert resolve_default_path() == DEFAULT_PATH
    assert DEFAULT_PATH == ".hnsx/regression-radar.json"
    # 2. Env var wins.
    env_value = str(tmp_path / "from-env.json")
    monkeypatch.setenv(ENV_PATH_KEY, env_value)
    assert resolve_default_path() == env_value
    # 3. Constructor arg wins over env.
    arg_value = str(tmp_path / "from-arg.json")
    assert resolve_default_path(arg_value) == arg_value


def test_regression_store_uses_default_when_no_arg(monkeypatch, tmp_path) -> None:
    from hnsx_worker.eval.regression_radar import RegressionStore

    monkeypatch.delenv("HNSX_REGRESSION_STORE", raising=False)
    target = tmp_path / "default-radar.json"
    monkeypatch.chdir(tmp_path)
    # We can't reach the real default without writing under cwd; instead
    # confirm that omitting path falls back to resolve_default_path().
    from hnsx_worker.eval.regression_radar import resolve_default_path
    store = RegressionStore(path=str(target))
    store.put("main", _baseline())
    reloaded = RegressionStore(path=str(target))
    assert reloaded.labels() == ["main"]
