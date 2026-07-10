"""W15 Phase 6 — Skill primitives and resolver.

A *skill* is a named bundle of prompts + tools + mcp_refs + examples that
agents can pick up via ``agent.skill_refs: [skill_id, ...]``. Skills are
declared inline in ``harness.skills[]`` (matches the proto message in
``proto/hnsx/v1/domain.proto:Skill``); resolving an agent's skill refs
yields a flat list of :class:`ResolvedSkill` whose prompts/tools can be
injected into the agent's runtime context.

Public API:

  - :class:`Skill`         — Python mirror of the proto message.
  - :class:`SkillRegistry` — ``Dict[str, Skill]`` with load helpers.
  - :class:`ResolvedSkill` — one entry per referenced skill id.
  - :func:`resolve_agent_skills` — map ``agent.skill_refs`` →
    :class:`ResolvedSkill` (with de-dup and missing-ref errors).
  - :class:`SkillResolutionError` — typed error carrying the missing id.

External skill packages (``hnsx_skill.toml`` + ``skills/`` directory +
``hnsx skill install``) are intentionally **not** part of this iteration;
they require a distribution story that's left to a follow-up.
"""

from __future__ import annotations

from .registry import Skill, SkillRegistry, SkillSpecError
from .resolver import ResolvedSkill, SkillResolutionError, resolve_agent_skills

__all__ = [
    "ResolvedSkill",
    "Skill",
    "SkillRegistry",
    "SkillResolutionError",
    "SkillSpecError",
    "resolve_agent_skills",
]
