"""Tests for the W13 auto prompt evolution."""

from __future__ import annotations

import json
import threading

import pytest

from hnsx_worker.adapters import AdapterRegistry
from hnsx_worker.adapters.base import Adapter, AdapterResult, Cost, StreamChunk
from hnsx_worker.eval.prompt_evolution import (
    PromptEvolutionReport,
    PromptVariant,
    evolve_prompt,
)


@pytest.fixture
def scripted_evo_adapter() -> None:
    """Adapter that returns predetermined variants + judge outputs."""

    class _ScriptedEvoAdapter(Adapter):
        def name(self) -> str:
            return "scripted_evo"

        def invoke(self, agent: dict, prompt: str, input: dict) -> AdapterResult:
            if input.get("prompt_evolution"):
                return AdapterResult(
                    text=json.dumps(
                        [
                            {
                                "name": "v1-terse",
                                "rationale": "tighter instructions",
                                "prompt": "Answer the question briefly.",
                            },
                            {
                                "name": "v2-needle",
                                "rationale": "explicit needle word",
                                "prompt": (
                                    "Use the provided needle word. "
                                    "Cite the source. Be concise."
                                ),
                            },
                        ]
                    ),
                    cost=Cost(),
                )
            if input.get("evolution_judge"):
                # Score the variant prompt based on a substring match against
                # the expected output. This lets us test relative ranking.
                if "needle" in prompt:
                    return AdapterResult(text="matches the needle", cost=Cost())
                if "briefly" in prompt:
                    return AdapterResult(text="wrong but short", cost=Cost())
                return AdapterResult(text="", cost=Cost())
            return AdapterResult(text="", cost=Cost())

        def invoke_stream(self, agent: dict, prompt: str, input: dict):
            yield StreamChunk(text_delta=(self.invoke(agent, prompt, input).text or ""))

    AdapterRegistry.register("scripted_evo", _ScriptedEvoAdapter)
    yield
    AdapterRegistry.reset()


def _eval_report() -> dict:
    return {
        "eval_set_id": "es-evo",
        "cases": [
            {
                "case_id": "q1",
                "input": {"q": "what is the refund policy?"},
                # ``expected`` is the needle the scorer looks for in the
                # judge output. The scripted judge returns "matches the
                # needle" only when the variant prompt mentions "needle".
                "expected": "needle",
                "scorers": [{"name": "contains"}],
            },
            {
                "case_id": "q2",
                "input": {"q": "how to reset pwd?"},
                "expected": "needle",
                "scorers": [{"name": "contains"}],
            },
        ],
    }


def test_evolve_prompt_generates_and_ranks_variants(scripted_evo_adapter) -> None:
    observations: list[dict] = []
    report = evolve_prompt(
        spec={"id": "demo"},
        current_prompt="Answer the user.",
        prompt_name="answer-prompt",
        eval_report=_eval_report(),
        judge_adapter_kind="scripted_evo",
        n_variants=2,
        emit=observations.append,
    )
    assert isinstance(report, PromptEvolutionReport)
    assert len(report.variants) == 2
    # The "rubric" variant is judged to match the expected substring,
    # the "briefly" variant doesn't, so ranking picks v2-rubric.
    by_name = {v.name: v for v in report.variants}
    assert by_name["v2-needle"].score > by_name["v1-terse"].score
    assert report.recommended_variant == "v2-needle"
    assert report.improved is True
    assert any(o["kind"] == "evolution_report" for o in observations)


def test_evolve_prompt_falls_back_when_no_variants(scripted_evo_adapter) -> None:
    # Override the adapter to return malformed JSON.
    class _BrokenAdapter(_ := AdapterRegistry.get("scripted_evo").__class__):  # type: ignore[assignment]
        pass

    class _Broken(Adapter):
        def name(self) -> str:
            return "broken_evo"

        def invoke(self, agent: dict, prompt: str, input: dict) -> AdapterResult:
            return AdapterResult(text="not parseable", cost=Cost())

        def invoke_stream(self, agent: dict, prompt: str, input: dict):
            yield StreamChunk(text_delta="")

    AdapterRegistry.register("broken_evo", _Broken)
    AdapterRegistry.reset()
    AdapterRegistry.register("broken_evo", _Broken)
    AdapterRegistry.register("scripted_evo", AdapterRegistry.get("scripted_evo").__class__)  # noqa: E501
    try:
        report = evolve_prompt(
            spec={"id": "demo"},
            current_prompt="Answer the user.",
            prompt_name="answer-prompt",
            eval_report=_eval_report(),
            judge_adapter_kind="broken_evo",
            n_variants=2,
        )
    finally:
        AdapterRegistry.reset()
    assert report.variants == []
    assert report.recommended_variant == ""
    assert report.improved is False


def test_evolve_prompt_uses_improvement_report_seeds(scripted_evo_adapter) -> None:
    improvement = {
        "clusters": [
            {
                "cluster_id": "c1",
                "label": "missing rubric",
                "description": "agent doesn't reference rubric",
                "case_ids": ["q1", "q2"],
            }
        ]
    }
    report = evolve_prompt(
        spec={"id": "demo"},
        current_prompt="Answer the user.",
        prompt_name="answer-prompt",
        eval_report=_eval_report(),
        improvement_report=improvement,
        judge_adapter_kind="scripted_evo",
    )
    assert report.variants  # at least one variant came back


def test_evolution_report_apply_returns_recommended() -> None:
    report = PromptEvolutionReport(
        report_id="x",
        eval_set_id="y",
        prompt_name="p",
        current_prompt="orig",
        variants=[
            PromptVariant(name="a", rationale="", prompt="new-prompt-text"),
        ],
        recommended_variant="a",
        baseline_score=0.5,
    )
    assert report.apply() == {"prompt_name": "p", "prompt": "new-prompt-text"}


def test_evolution_report_apply_none_when_no_recommendation() -> None:
    report = PromptEvolutionReport(
        report_id="x",
        eval_set_id="y",
        prompt_name="p",
        current_prompt="orig",
    )
    assert report.apply() is None