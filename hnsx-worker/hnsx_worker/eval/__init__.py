"""Eval platform support (W8 / W9 / W13).
"""

from __future__ import annotations

from .improvement_loop import (
    FailureCase,
    FailureCluster,
    ImprovementReport,
    run_improvement_loop,
)
from .prompt_evolution import (
    PromptEvolutionReport,
    PromptVariant,
    evolve_prompt,
)
from .regression_radar import (
    RegressionAlert,
    RegressionStore,
    check_regressions,
    detect_regressions,
)
from .runner import aggregate_scores, compare_eval_reports, run_eval, run_eval_set
from .scorers import Score, list_scorers, score

__all__ = [
    "FailureCase",
    "FailureCluster",
    "ImprovementReport",
    "PromptEvolutionReport",
    "PromptVariant",
    "RegressionAlert",
    "RegressionStore",
    "Score",
    "aggregate_scores",
    "check_regressions",
    "compare_eval_reports",
    "detect_regressions",
    "evolve_prompt",
    "list_scorers",
    "run_eval",
    "run_eval_set",
    "run_improvement_loop",
    "score",
]