"""Tests for hnsx_worker.skills.resolver — resolve_agent_skills."""

from __future__ import annotations

import pytest

from hnsx_worker.skills.registry import Skill, SkillRegistry
from hnsx_worker.skills.resolver import (
    ResolvedSkill,
    SkillResolutionError,
    resolve_agent_skills,
)


@pytest.fixture
def registry() -> SkillRegistry:
    return SkillRegistry.from_dict(
        {
            "web-search": {
                "id": "web-search",
                "tools": [{"name": "fetch", "type": "http"}],
                "mcp_refs": ["fetch-mcp"],
            },
            "memory": {
                "id": "memory",
                "prompts": [{"id": "remember", "template": "..."}],
                "tools": [{"name": "memory_search", "type": "memory"}],
                "examples": [{"id": "ex1", "input": "i", "output": "o"}],
            },
            "slack": {
                "id": "slack",
                "tools": [{"name": "send_message", "type": "http"}],
            },
        }
    )


# ---------------------------------------------------------------------------
# ResolvedSkill.from_skill
# ---------------------------------------------------------------------------


def test_resolved_skill_from_skill_copies_payload() -> None:
    skill = Skill(
        id="web-search",
        tools=[{"name": "fetch"}],
        mcp_refs=["m1"],
    )
    resolved = ResolvedSkill.from_skill(skill)
    assert resolved.skill_id == "web-search"
    assert resolved.tools == [{"name": "fetch"}]
    assert resolved.mcp_refs == ["m1"]


# ---------------------------------------------------------------------------
# resolve_agent_skills — happy paths
# ---------------------------------------------------------------------------


def test_resolve_returns_empty_when_no_refs(registry: SkillRegistry) -> None:
    agent = {"id": "triage"}
    assert resolve_agent_skills(agent, registry) == []


def test_resolve_returns_empty_when_refs_is_none(registry: SkillRegistry) -> None:
    agent = {"id": "triage", "skill_refs": None}
    assert resolve_agent_skills(agent, registry) == []


def test_resolve_single_ref(registry: SkillRegistry) -> None:
    agent = {"id": "triage", "skill_refs": ["web-search"]}
    out = resolve_agent_skills(agent, registry)
    assert len(out) == 1
    assert out[0].skill_id == "web-search"
    assert out[0].tools == [{"name": "fetch", "type": "http"}]
    assert out[0].mcp_refs == ["fetch-mcp"]


def test_resolve_multiple_refs_preserves_order(registry: SkillRegistry) -> None:
    agent = {"id": "triage", "skill_refs": ["memory", "web-search", "slack"]}
    out = resolve_agent_skills(agent, registry)
    assert [r.skill_id for r in out] == ["memory", "web-search", "slack"]


def test_resolve_dedups_with_first_occurrence_wins(registry: SkillRegistry) -> None:
    agent = {
        "id": "triage",
        "skill_refs": ["web-search", "memory", "web-search"],
    }
    out = resolve_agent_skills(agent, registry)
    assert [r.skill_id for r in out] == ["web-search", "memory"]


def test_resolve_carries_prompts_and_examples(registry: SkillRegistry) -> None:
    agent = {"id": "triage", "skill_refs": ["memory"]}
    out = resolve_agent_skills(agent, registry)
    assert out[0].prompts == [{"id": "remember", "template": "..."}]
    assert out[0].examples == [{"id": "ex1", "input": "i", "output": "o"}]


# ---------------------------------------------------------------------------
# resolve_agent_skills — error paths
# ---------------------------------------------------------------------------


def test_resolve_missing_skill_raises_structured_error(
    registry: SkillRegistry,
) -> None:
    agent = {"id": "triage", "skill_refs": ["does-not-exist"]}
    with pytest.raises(SkillResolutionError) as exc:
        resolve_agent_skills(agent, registry)
    err = exc.value
    assert err.agent_id == "triage"
    assert err.missing_id == "does-not-exist"
    assert "web-search" in err.available
    assert "memory" in err.available


def test_resolve_skill_resolution_error_is_keyerror_subclass(
    registry: SkillRegistry,
) -> None:
    agent = {"id": "triage", "skill_refs": ["nope"]}
    with pytest.raises(KeyError):
        resolve_agent_skills(agent, registry)


def test_resolve_non_list_skill_refs_raises_resolution_error(
    registry: SkillRegistry,
) -> None:
    agent = {"id": "triage", "skill_refs": "not-a-list"}  # type: ignore[list-item]
    with pytest.raises(SkillResolutionError):
        resolve_agent_skills(agent, registry)


def test_resolve_after_dedup_still_validates(registry: SkillRegistry) -> None:
    # First ref valid, second missing → still raises (we validate as we go).
    agent = {"id": "triage", "skill_refs": ["web-search", "nope"]}
    with pytest.raises(SkillResolutionError):
        resolve_agent_skills(agent, registry)
