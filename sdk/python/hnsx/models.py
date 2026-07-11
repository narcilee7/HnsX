"""Wire models for the HnsX REST API."""

from __future__ import annotations

from dataclasses import dataclass
from typing import Any


@dataclass
class DomainSummary:
    id: str
    version: str
    status: str
    description: str | None = None
    created_at: str | None = None
    updated_at: str | None = None


@dataclass
class Domain:
    id: str
    version: str
    status: str | None = None
    description: str | None = None
    harness: dict[str, Any] | None = None
    created_at: str | None = None
    updated_at: str | None = None


@dataclass
class SessionSummary:
    id: str
    domain_id: str
    state: str
    domain_version: str | None = None
    orchestration: str | None = None
    started_at: str | None = None
    completed_at: str | None = None


@dataclass
class Session:
    id: str
    domain_id: str
    state: str
    domain_version: str | None = None
    orchestration: str | None = None
    trigger: dict[str, Any] | None = None
    started_at: str | None = None
    completed_at: str | None = None
    result: dict[str, Any] | None = None


@dataclass
class TraceSummary:
    trace_id: str
    session_id: str
    domain_id: str
    status: str
    domain_version: str | None = None
    started_at: str | None = None
    completed_at: str | None = None
    duration_ms: int | None = None
    observation_count: int | None = None
    total_cost_usd: float | None = None
    prompt_tokens: int | None = None
    completion_tokens: int | None = None
    agent_invocations: int | None = None
    tool_invocations: int | None = None


@dataclass
class Trace(TraceSummary):
    observations: list[Observation] | None = None


@dataclass
class Observation:
    kind: str
    trace_id: str | None = None
    session_id: str | None = None
    domain_id: str | None = None
    domain_version: str | None = None
    step_id: str | None = None
    agent_id: str | None = None
    payload: dict[str, Any] | None = None
    metadata: dict[str, Any] | None = None
    cost_usd: float | None = None
    prompt_tokens: int | None = None
    completion_tokens: int | None = None
    latency_ms: int | None = None
    timestamp: str | None = None


@dataclass
class Approval:
    id: str
    status: str
    session_id: str | None = None
    domain_id: str | None = None
    action: str | None = None
    resource: str | None = None
    risk_level: str | None = None
    context: dict[str, Any] | None = None
    requested_by: str | None = None
    reviewed_by: str | None = None
    comment: str | None = None
    created_at: str | None = None
    updated_at: str | None = None
    resolved_at: str | None = None


@dataclass
class EvalSetSummary:
    id: str
    set_id: str
    domain_id: str
    description: str | None = None
    case_count: int | None = None
    created_at: str | None = None


@dataclass
class EvalCase:
    id: str
    input: dict[str, Any]
    name: str | None = None
    expect: dict[str, Any] | None = None
    scorer: EvalScorer | None = None


@dataclass
class EvalScorer:
    type: str
    config: dict[str, Any] | None = None


@dataclass
class EvalSet(EvalSetSummary):
    cases: list[EvalCase] | None = None
    updated_at: str | None = None


@dataclass
class EvalCaseResult:
    case_id: str
    session_id: str | None = None
    score: float | None = None
    passed: bool | None = None
    actual: dict[str, Any] | None = None
    details: dict[str, Any] | None = None
    duration_ms: int | None = None
    cost_usd: float | None = None


@dataclass
class EvalRun:
    id: str
    eval_set_id: str
    domain_id: str
    state: str
    domain_version: str | None = None
    orchestration: str | None = None
    score: float | None = None
    total_cases: int | None = None
    passed_cases: int | None = None
    total_cost_usd: float | None = None
    duration_ms: int | None = None
    cases: list[EvalCaseResult] | None = None
    created_at: str | None = None
    completed_at: str | None = None


@dataclass
class ListEnvelope:
    items: list[Any]
    total: int
    limit: int | None = None
    offset: int | None = None
