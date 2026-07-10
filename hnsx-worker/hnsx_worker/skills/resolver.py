"""Resolve ``agent.skill_refs`` against a :class:`SkillRegistry`.

The resolver is a flat data transformation — it doesn't touch the LLM
or the runtime; it just produces a list of :class:`ResolvedSkill` that
the runtime can merge into the agent's tools/prompts/mcps.

Two contracts:

  - Missing skill id → :class:`SkillResolutionError` with the missing
    id and the agent context, so observability can surface "agent X
    references unknown skill Y".
  - Duplicate refs in the same agent → de-duped, **first occurrence
    wins** (preserves declaration order so callers can reason about
    which skill "comes first").
"""

from __future__ import annotations

from dataclasses import dataclass, field
from typing import Any

from .registry import Skill, SkillRegistry


class SkillResolutionError(KeyError):
    """Raised when an agent references a skill id that isn't registered.

    Inherits :class:`KeyError` so ``except KeyError`` still catches it
    while keeping a distinct type for observability assertions.
    """

    def __init__(self, agent_id: str, missing_id: str, available: list[str]) -> None:
        self.agent_id = agent_id
        self.missing_id = missing_id
        self.available = sorted(available)
        super().__init__(
            f"agent {agent_id!r} references unknown skill {missing_id!r} "
            f"(available: {self.available})"
        )


@dataclass
class ResolvedSkill:
    """The flattened payload from one referenced skill.

    Wrapping a :class:`Skill` in :class:`ResolvedSkill` lets the runtime
    hold the originating skill id alongside the injected payload — useful
    when emitting ``skill_injected`` observations in a later phase.
    """

    skill_id: str
    prompts: list[dict[str, Any]] = field(default_factory=list)
    tools: list[dict[str, Any]] = field(default_factory=list)
    mcp_refs: list[str] = field(default_factory=list)
    examples: list[dict[str, Any]] = field(default_factory=list)

    @classmethod
    def from_skill(cls, skill: Skill) -> "ResolvedSkill":
        return cls(
            skill_id=skill.id,
            prompts=list(skill.prompts),
            tools=list(skill.tools),
            mcp_refs=list(skill.mcp_refs),
            examples=list(skill.examples),
        )


def resolve_agent_skills(
    agent: dict[str, Any],
    registry: SkillRegistry,
) -> list[ResolvedSkill]:
    """Map an agent's ``skill_refs[]`` to :class:`ResolvedSkill` entries.

    Args:
        agent: An agent dict; reads ``agent.get("skill_refs")`` which is
            ``None`` / a list of strings by convention.
        registry: The :class:`SkillRegistry` to resolve against.

    Returns:
        A list of :class:`ResolvedSkill`, in the order each ref first
        appeared in ``agent["skill_refs"]`` with duplicates removed.

    Raises:
        SkillResolutionError: When a ref id isn't present in
            ``registry``. The exception carries the agent id and the
            available skill ids for diagnostics.
    """
    refs = agent.get("skill_refs") or []
    if not refs:
        return []

    if not isinstance(refs, list):
        raise SkillResolutionError(
            agent_id=str(agent.get("id", "")),
            missing_id="<invalid>",
            available=list(registry),
        )

    agent_id = str(agent.get("id", "") or "")

    seen: set[str] = set()
    resolved: list[ResolvedSkill] = []
    for ref in refs:
        ref_id = str(ref)
        if ref_id in seen:
            continue
        seen.add(ref_id)
        if ref_id not in registry:
            raise SkillResolutionError(
                agent_id=agent_id,
                missing_id=ref_id,
                available=list(registry),
            )
        resolved.append(ResolvedSkill.from_skill(registry[ref_id]))
    return resolved


__all__ = ["ResolvedSkill", "SkillResolutionError", "resolve_agent_skills"]
