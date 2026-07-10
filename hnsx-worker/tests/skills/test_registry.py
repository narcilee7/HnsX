"""Tests for hnsx_worker.skills.registry — Skill dataclass + SkillRegistry."""

from __future__ import annotations

import pytest

from hnsx_worker.skills.registry import (
    Skill,
    SkillRegistry,
    SkillSpecError,
)


# ---------------------------------------------------------------------------
# Skill.from_spec
# ---------------------------------------------------------------------------


def test_skill_from_spec_minimal() -> None:
    skill = Skill.from_spec({"id": "web-search", "tools": [{"name": "fetch"}]})
    assert skill.id == "web-search"
    assert skill.description == ""
    assert skill.tools == [{"name": "fetch"}]
    assert skill.prompts == []
    assert skill.mcp_refs == []
    assert skill.examples == []
    assert skill.has_content() is True
    assert skill.to_dict() == {
        "id": "web-search",
        "description": "",
        "prompts": [],
        "tools": [{"name": "fetch"}],
        "mcp_refs": [],
        "examples": [],
    }


def test_skill_from_spec_full() -> None:
    skill = Skill.from_spec(
        {
            "id": "memory",
            "description": "episodic memory tools",
            "prompts": [{"id": "memory-prompt", "template": "remember this"}],
            "tools": [{"name": "memory_search", "type": "memory"}],
            "mcp_refs": ["fs-mcp"],
            "examples": [{"id": "ex1", "input": "i", "output": "o"}],
        }
    )
    assert skill.id == "memory"
    assert skill.description == "episodic memory tools"
    assert len(skill.prompts) == 1
    assert len(skill.tools) == 1
    assert skill.mcp_refs == ["fs-mcp"]
    assert len(skill.examples) == 1
    assert skill.has_content() is True


def test_skill_from_spec_missing_id_raises() -> None:
    with pytest.raises(SkillSpecError) as exc:
        Skill.from_spec({"description": "oops", "tools": [{"name": "t"}]})
    assert "id" in str(exc.value)


def test_skill_from_spec_empty_id_raises() -> None:
    with pytest.raises(SkillSpecError):
        Skill.from_spec({"id": "  ", "tools": [{"name": "t"}]})


def test_skill_from_spec_not_a_dict_raises() -> None:
    with pytest.raises(SkillSpecError):
        Skill.from_spec("not a dict")  # type: ignore[arg-type]


def test_skill_from_spec_empty_content_raises() -> None:
    with pytest.raises(SkillSpecError) as exc:
        Skill.from_spec({"id": "useless", "description": "no content"})
    assert "no prompts/tools/mcp_refs/examples" in str(exc.value)


def test_skill_from_spec_rejects_non_list_prompts() -> None:
    with pytest.raises(SkillSpecError):
        Skill.from_spec({"id": "s", "prompts": "not a list"})


def test_skill_from_spec_rejects_non_dict_tool_entry() -> None:
    with pytest.raises(SkillSpecError):
        Skill.from_spec({"id": "s", "tools": ["bad"]})


def test_skill_has_content_false_on_fresh_dataclass() -> None:
    # Plain empty dataclass (no content) → no content; from_spec would
    # refuse it but the dataclass itself allows it for advanced cases.
    s = Skill(id="x")
    assert s.has_content() is False


# ---------------------------------------------------------------------------
# SkillRegistry.from_dict
# ---------------------------------------------------------------------------


def _spec_dict() -> dict:
    return {
        "web-search": {
            "id": "web-search",
            "description": "fetch URLs",
            "tools": [{"name": "fetch", "type": "http"}],
        },
        "memory": {
            "id": "memory",
            "description": "memory tools",
            "tools": [{"name": "memory_search", "type": "memory"}],
            "mcp_refs": ["fs-mcp"],
        },
    }


def test_skill_registry_from_dict_indexes_by_id() -> None:
    reg = SkillRegistry.from_dict(_spec_dict())
    assert len(reg) == 2
    assert "web-search" in reg
    assert "memory" in reg
    assert isinstance(reg["web-search"], Skill)
    assert reg["web-search"].id == "web-search"


def test_skill_registry_from_dict_iteration_order_matches_insertion() -> None:
    reg = SkillRegistry.from_dict(_spec_dict())
    assert list(reg) == ["web-search", "memory"]


def test_skill_registry_from_dict_list_shape() -> None:
    reg = SkillRegistry.from_dict(
        [
            {"id": "a", "tools": [{"name": "t"}]},
            {"id": "b", "mcp_refs": ["m1"]},
        ]
    )
    assert set(reg) == {"a", "b"}
    assert reg["a"].tools == [{"name": "t"}]
    assert reg["b"].mcp_refs == ["m1"]


def test_skill_registry_from_dict_rejects_duplicate_ids_list() -> None:
    with pytest.raises(SkillSpecError) as exc:
        SkillRegistry.from_dict(
            [
                {"id": "x", "tools": [{"name": "t"}]},
                {"id": "x", "mcp_refs": ["m"]},
            ]
        )
    assert "duplicate" in str(exc.value)


def test_skill_registry_from_dict_rejects_list_entry_missing_id() -> None:
    with pytest.raises(SkillSpecError):
        SkillRegistry.from_dict([{"tools": [{"name": "t"}]}])


def test_skill_registry_from_dict_rejects_non_list_non_dict_input() -> None:
    with pytest.raises(SkillSpecError):
        SkillRegistry.from_dict("not ok")  # type: ignore[arg-type]


def test_skill_registry_from_dict_handles_none_and_empty() -> None:
    assert len(SkillRegistry.from_dict(None)) == 0
    assert len(SkillRegistry.from_dict({})) == 0


def test_skill_registry_rejects_duplicate_ids_after_construction() -> None:
    # Pathological: build directly with duplicates.
    with pytest.raises(SkillSpecError):
        SkillRegistry(
            {"a": Skill(id="a", tools=[{"name": "t"}]), "b": Skill(id="a")}
        )
