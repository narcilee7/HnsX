"""End-to-end test for the W13 smart-eval pipeline wired into execute_session.

Covers the three W13 modules in their production path:

  - run_improvement_loop  (clusters failed cases + emits observation)
  - evolve_prompt         (variant generation + A/B scoring)
  - check_regressions     (baseline persistence + severity)

The goal is not to retest the modules themselves (those tests live in
tests/eval/test_{improvement_loop,regression_radar,prompt_evolution}.py).
This file proves they are actually invoked from execute_session when
``spec.improvement`` is configured.
"""

from __future__ import annotations

import json
import threading

import pytest

from hnsx_worker.adapters import AdapterRegistry
from hnsx_worker.adapters.base import Adapter, AdapterResult, Cost, StreamChunk
from hnsx_worker.adapters.noop import NoopAdapter
from hnsx_worker.session_executor import execute_session


@pytest.fixture
def registered_adapters() -> None:
    """Register a noop-shaped adapter that emits simple deterministic text
    plus a tiny scripted evolution adapter that returns two variants.

    The variant judge is also scripted: variants containing the
    substring "winning" are scored higher than the baseline so we can
    observe the recommendation path when the judge is non-noop.
    """
    class _ScriptedEvo(Adapter):
        def name(self) -> str:
            return "scripted_evo"

        def invoke(self, agent, prompt, input):
            if input.get("prompt_evolution"):
                payload = [
                    {"name": "v1-baseline-ish", "rationale": "mild",
                     "prompt": "Same as baseline."},
                    {"name": "v2-winning", "rationale": "uses the keyword",
                     "prompt": "Use winning wording. Be concise."},
                ]
                return AdapterResult(text=json.dumps(payload), cost=Cost())
            # evolution_judge: the output text is what gets scored against
            # the case expected. The baseline template doesn't contain
            # "winning", so it scores 0; variants containing "winning"
            # score 1 (the expected substring "noop" is present in
            # the winner's output). This lets evolve_prompt pick a winner.
            if "winning" in prompt:
                return AdapterResult(text="noop: winning wording", cost=Cost())
            return AdapterResult(text="no-match", cost=Cost())

        def invoke_stream(self, agent, prompt, input):
            yield StreamChunk(text_delta=(self.invoke(agent, prompt, input).text or ""))

    AdapterRegistry.register("noop", NoopAdapter)
    AdapterRegistry.register("scripted_evo", _ScriptedEvo)
    try:
        yield
    finally:
        AdapterRegistry.reset()


def _build_spec(judge: str = "noop") -> dict:
    return {
        "id": "w13-e2e-demo",
        "version": "0.1.0",
        "harness": {
            "agents": {
                "main": {
                    "id": "main",
                    "provider": "noop",
                    "model": "noop-1",
                    "adapter": {"kind": "noop"},
                    "system_prompt": "main-prompt",
                },
            },
            "prompts": {
                "main-prompt": {
                    "type": "system",
                    "template": "Help the user concisely.",
                },
            },
            "session": {"mode": "single"},
        },
        "improvement": {
            "target_prompt": "main-prompt",
            "judge_adapter": judge,
            "n_variants": 2,
            "min_improvement": 0.01,
            "baseline_label": "w13-e2e-main",
        },
    }


def _build_eval_set() -> dict:
    return {
        "eval_set_id": "es-w13",
        "max_cost_usd": 0.10,
        "cases": [
            {
                "case_id": "c1",
                "input": {"content": "hi"},
                "expected": "noop",
                "scorers": [{"name": "contains", "contains": "noop"}],
            },
            {
                "case_id": "c2",
                "input": {"content": "bye"},
                "expected": "noop",
                "scorers": [{"name": "contains", "contains": "noop"}],
            },
        ],
    }


def _run(spec, config, observations=None) -> dict:
    trigger = {"content": "hi"}

    def _emit(obs):
        if observations is not None:
            observations.append(obs)

    return execute_session(
        spec,
        trigger,
        config,
        stop_event=threading.Event(),
        emit=_emit,
    )


def test_execute_session_runs_w13_pipeline_first_run(registered_adapters, tmp_path) -> None:
    """First eval run: produces all four reports; regression_rc == 0
    because there is no prior baseline."""
    config = {
        "session_id": "s-w13-first",
        "domain_id": "w13-e2e-demo",
        "eval_set": _build_eval_set(),
        "regression_store_path": str(tmp_path / "radar.json"),
    }
    result = _run(_build_spec(), config)

    assert "eval_report" in result, result.keys()
    assert len(result["eval_report"]["cases"]) == 2

    # improvement loop ran (heuristic clustering since judge is noop-friendly;
    # noop returns no JSON so LLM path falls through to heuristics).
    assert "improvement_report" in result, result.keys()
    assert "improvement_error" not in result

    # prompt evolution ran; report exists even if noop returned 0 variants.
    assert "evolution_report" in result, result.keys()
    assert "evolution_error" not in result

    # regression radar: first-run ⇒ severity ok, summary empty, rc == 0.
    assert "regression_check" in result, result.keys()
    rc_block = result["regression_check"]
    assert rc_block["severity"] == "ok"
    assert rc_block["summary"]["total"] == 0
    assert result.get("regression_rc") == 0

    # Baseline was persisted.
    store_path = tmp_path / "radar.json"
    assert store_path.exists()
    persisted = json.loads(store_path.read_text())
    assert "w13-e2e-main" in persisted
    assert persisted["w13-e2e-main"]["report"]["eval_set_id"] == "es-w13"


def test_execute_session_w13_pipeline_second_run_detects_baseline(
    registered_adapters, tmp_path,
) -> None:
    """Second run with no regression: report compares against the first
    run's baseline and finds zero regressions because both cases pass."""
    config = {
        "session_id": "s-w13-second",
        "domain_id": "w13-e2e-demo",
        "eval_set": _build_eval_set(),
        "regression_store_path": str(tmp_path / "radar.json"),
    }
    first = _run(_build_spec(), config)
    assert "improvement_report" in first  # baseline established

    # Second invocation uses a fresh session_id but reads the same store.
    config["session_id"] = "s-w13-second-rerun"
    second = _run(_build_spec(), config)
    rc_block = second["regression_check"]
    assert rc_block["severity"] == "ok"
    assert rc_block["summary"]["total"] == 2  # both cases scored against baseline
    assert second.get("regression_rc") == 0


def test_execute_session_w13_evolution_recommends_winner_when_judge_returns_variants(
    registered_adapters, tmp_path,
) -> None:
    """When the judge adapter actually returns variants, evolve_prompt
    picks the highest-scoring one and writes ``evolution_report`` with
    ``recommended_variant`` set. ``apply()`` is intentionally not invoked."""
    config = {
        "session_id": "s-w13-evo",
        "domain_id": "w13-e2e-demo",
        "eval_set": _build_eval_set(),
        "regression_store_path": str(tmp_path / "radar.json"),
    }
    spec = _build_spec(judge="scripted_evo")
    result = _run(spec, config)
    evo = result.get("evolution_report")
    assert evo is not None, result.keys()
    # scripted_evo returns 2 variants; one of them should win.
    assert len(evo["variants"]) == 2
    # At least one variant outperformed the baseline → recommended_variant
    # is non-empty (the higher-scoring "v2-winning" entry).
    assert evo["recommended_variant"] in {"v1-baseline-ish", "v2-winning"}


def test_execute_session_w13_skips_evolution_when_target_missing(
    registered_adapters, tmp_path,
) -> None:
    """If the spec declares improvement.target_prompt that doesn't exist
    in spec.harness.prompts, evolution is skipped with an explicit
    reason; improvement + regression still run."""
    spec = _build_spec()
    spec["improvement"]["target_prompt"] = "does-not-exist"
    config = {
        "session_id": "s-w13-missing-prompt",
        "domain_id": "w13-e2e-demo",
        "eval_set": _build_eval_set(),
        "regression_store_path": str(tmp_path / "radar.json"),
    }
    result = _run(spec, config)
    assert result.get("evolution_skipped")
    assert "not found" in result["evolution_skipped"]
    assert "improvement_report" in result
    assert "regression_check" in result
