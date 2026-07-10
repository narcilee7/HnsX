"""W13 ``eval_self_check`` tool — agent self-evaluates a candidate output.

Exposed via the agent's ``tools:`` list::

    tools:
      - name: eval_self_check
        type: self_check
        config:
          rubric: |
            Output must (a) cite file:line, (b) be < 4 sentences.
          judge_adapter: scripted_judge
          threshold: 0.7

The agent calls::

    eval_self_check(task=..., candidate_output=..., criteria=["completeness", "safety"])

…and receives a :class:`ToolResult` with ``output = {"score", "passed",
"details"}``. Internally it dispatches to the configured LLM judge through
the existing :mod:`hnsx_worker.eval.scorers` plumbing — the same code
path that powers offline EvalSet scoring.
"""

from __future__ import annotations

import json
import logging
import threading
from dataclasses import dataclass, field
from typing import Any

from hnsx_worker.adapters import AdapterRegistry
from hnsx_worker.eval.scorers import Score, score

from .base import Tool, ToolContext, ToolResult

log = logging.getLogger("hnsx_worker.tools.self_check")

_SELF_CHECK_PROMPT = """\
You are an eval judge. Score the candidate output for the task below.

Task:
{task}

Candidate output:
{candidate}

Rubric:
{rubric}

Criteria:
{criteria}

Reply with a JSON object: {{ "score": <0..1>, "passed": <bool>, "rationale": "..." }}
"""


@dataclass
class SelfCheckToolConfig:
    """Configuration for the ``self_check`` tool."""

    rubric: str = ""
    judge_adapter: str = "noop"
    threshold: float = 0.7
    default_criteria: list[str] = field(default_factory=list)
    extra: dict[str, Any] = field(default_factory=dict)

    @classmethod
    def from_spec(cls, raw: dict[str, Any]) -> "SelfCheckToolConfig":
        if not isinstance(raw, dict):
            raise ValueError("self_check config must be a dict")
        judge = raw.get("judge_adapter") or raw.get("adapter") or "noop"
        criteria = raw.get("default_criteria") or raw.get("criteria") or []
        if not isinstance(criteria, list):
            raise ValueError("self_check.default_criteria must be a list")
        return cls(
            rubric=str(raw.get("rubric") or ""),
            judge_adapter=str(judge),
            threshold=float(raw.get("threshold", 0.7)),
            default_criteria=[str(c) for c in criteria],
        )


class SelfCheckTool(Tool):
    """Tool: ``eval_self_check(task, candidate_output, criteria=None)``."""

    def __init__(self, name: str, config: SelfCheckToolConfig) -> None:
        self._name = name
        self._config = config

    @property
    def name(self) -> str:
        return self._name

    @property
    def schema(self) -> dict[str, Any]:
        return {
            "type": "object",
            "properties": {
                "task": {
                    "type": "string",
                    "description": "The original task the candidate is meant to solve.",
                },
                "candidate_output": {
                    "type": "string",
                    "description": "The candidate answer to evaluate.",
                },
                "criteria": {
                    "type": "array",
                    "items": {"type": "string"},
                    "description": "Optional criteria to score against.",
                },
                "rubric": {
                    "type": "string",
                    "description": "Optional rubric overriding the configured default.",
                },
            },
            "required": ["task", "candidate_output"],
            "additionalProperties": False,
        }

    def invoke(self, ctx: ToolContext, input: dict[str, Any]) -> ToolResult:
        if not isinstance(input, dict):
            return ToolResult(error="eval_self_check input must be a JSON object")

        task = str(input.get("task", "")).strip()
        candidate = input.get("candidate_output")
        if not task:
            return ToolResult(error="eval_self_check requires 'task'")
        if candidate is None:
            return ToolResult(error="eval_self_check requires 'candidate_output'")

        rubric = str(input.get("rubric") or self._config.rubric or "(no rubric)")
        criteria = input.get("criteria") or self._config.default_criteria or ["overall"]
        if not isinstance(criteria, list):
            return ToolResult(error="criteria must be a list of strings")

        # Self-check is a different shape from the offline llm_judge scorer
        # (which grades against an ``expected`` value). Here we ask the
        # judge adapter for a fresh verdict against ``task`` + ``rubric``.
        scorer_result = self._ask_judge_directly(task, candidate, criteria, rubric)

        passed = scorer_result.score >= self._config.threshold
        output = {
            "score": scorer_result.score,
            "passed": passed,
            "threshold": self._config.threshold,
            "details": scorer_result.details,
        }
        return ToolResult(
            output=output,
            metadata={"scorer": "llm_judge", "judge_adapter": self._config.judge_adapter},
        )

    # ---------------------------------------------------------------- helpers

    def _run_llm_judge_scorer(
        self,
        task: str,
        candidate: Any,
        criteria: list[str],
        rubric: str,
    ) -> Score | None:
        """Try to use the existing ``llm_judge`` scorer with our judge adapter."""
        try:
            adapter = AdapterRegistry.get(self._config.judge_adapter)
        except Exception:  # noqa: BLE001
            return None

        expected = {"task": task, "criteria": criteria, "rubric": rubric}
        return score(
            "llm_judge",
            expected,
            candidate,
            judge_adapter=self._config.judge_adapter,
            criteria=criteria,
            rubric=rubric,
        )

    def _ask_judge_directly(
        self,
        task: str,
        candidate: Any,
        criteria: list[str],
        rubric: str,
    ) -> Score:
        adapter = AdapterRegistry.get(self._config.judge_adapter)
        prompt = _SELF_CHECK_PROMPT.format(
            task=task,
            candidate=str(candidate),
            criteria=", ".join(criteria) or "overall",
            rubric=rubric,
        )
        try:
            result = adapter.invoke({}, prompt, {"self_check": True})
            text = result.text or ""
        except Exception as e:  # noqa: BLE001
            log.warning("self_check judge.invoke failed: %s", e)
            text = ""

        try:
            parsed = json.loads(text)
            score_v = float(parsed.get("score", 0.0))
            passed = bool(parsed.get("passed", False))
            rationale = str(parsed.get("rationale", ""))
        except (json.JSONDecodeError, ValueError, TypeError):
            score_v = 0.0
            passed = False
            rationale = f"judge returned unparseable output: {text[:200]}"
        return Score(score=score_v, passed=passed, details={"rationale": rationale})

    def _run_llm_judge_scorer(
        self,
        task: str,
        candidate: Any,
        criteria: list[str],
        rubric: str,
    ) -> Score | None:
        """Reserved for future use; left here so :func:`score` stays reachable.

        ``llm_judge`` (offline scorer) compares against an ``expected`` value
        and doesn't take a free-form task + rubric, so the public tool path
        uses :meth:`_ask_judge_directly` instead.
        """
        return None


def build_self_check_tool(
    name: str,
    raw_config: dict[str, Any],
    stop_event: threading.Event | None = None,  # signature parity with other tools
) -> Tool:
    """Helper used by :func:`tools.factory.build_tool`."""
    return SelfCheckTool(name, SelfCheckToolConfig.from_spec(raw_config))


__all__ = ["SelfCheckTool", "SelfCheckToolConfig", "build_self_check_tool"]