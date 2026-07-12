"""Skill + SkillRegistry — the Python mirror of ``proto.Skill``.

The dataclass shape tracks the proto message in
``proto/hnsx/v1/domain.proto``:

  - id          (str, required)
  - description (str)
  - prompts     (list of dicts, each with at least ``id`` + ``template``)
  - tools       (list of tool-spec dicts — see ``tools/factory.py``)
  - mcp_refs    (list of MCP server ids declared in ``harness.mcps``)
  - examples    (list of dicts, each with ``id`` + ``input`` + ``output``)

Skills are **loaded from dict** (not from generated proto stubs) so the
loader stays decoupled from wire format and the same API can later absorb
``hnsx_skill.toml`` packages without rewriting.
"""

from __future__ import annotations

from collections.abc import Iterator, Mapping
from dataclasses import dataclass, field
from typing import Any

class SkillSpecError(ValueError):
    """Raised when a Skill spec dict is invalid (missing id, etc.)."""


@dataclass
class Skill:
    """A single named skill bundle.

    Fields mirror the proto message exactly; ``from_spec`` enforces the
    small set of invariants (``id`` non-empty, at least one content field
    populated) so a downstream resolver can't accidentally dispatch on a
    null skill.
    """

    id: str
    description: str = ""
    prompts: list[dict[str, Any]] = field(default_factory=list)
    tools: list[dict[str, Any]] = field(default_factory=list)
    mcp_refs: list[str] = field(default_factory=list)
    examples: list[dict[str, Any]] = field(default_factory=list)

    # ---------------------------------------------------------------- helpers

    def has_content(self) -> bool:
        """Return True if the skill carries any usable payload."""
        return bool(
            self.prompts or self.tools or self.mcp_refs or self.examples
        )

    def to_dict(self) -> dict[str, Any]:
        return {
            "id": self.id,
            "description": self.description,
            "prompts": list(self.prompts),
            "tools": list(self.tools),
            "mcp_refs": list(self.mcp_refs),
            "examples": list(self.examples),
        }

    # ------------------------------------------------------------ factory

    @classmethod
    def from_spec(cls, spec: Any) -> "Skill":
        """Build a :class:`Skill` from a YAML/JSON-style dict.

        Args:
            spec: A dict with at least an ``id`` key. Missing fields
                default to empty.

        Raises:
            SkillSpecError: When ``spec`` is not a dict, ``id`` is empty,
                or the skill has no content at all (an empty-id skill is
                almost always a typo and an empty-content skill is
                useless).
        """
        if not isinstance(spec, dict):
            raise SkillSpecError(
                f"skill spec must be a dict, got {type(spec).__name__}"
            )

        skill_id = str(spec.get("id", "") or "").strip()
        if not skill_id:
            raise SkillSpecError("skill spec requires a non-empty 'id'")

        description = str(spec.get("description", "") or "")

        prompts = _coerce_dict_list(spec.get("prompts"), field="prompts")
        tools = _coerce_dict_list(spec.get("tools"), field="tools")
        mcp_refs = _coerce_str_list(spec.get("mcp_refs"), field="mcp_refs")
        examples = _coerce_dict_list(spec.get("examples"), field="examples")

        skill = cls(
            id=skill_id,
            description=description,
            prompts=prompts,
            tools=tools,
            mcp_refs=mcp_refs,
            examples=examples,
        )
        if not skill.has_content():
            raise SkillSpecError(
                f"skill {skill_id!r} has no prompts/tools/mcp_refs/examples"
            )
        return skill


class SkillRegistry(Mapping[str, Skill]):
    """Immutable dict-like view of ``harness.skills[]``.

    Built once at load time and shared between the loader and runtime
    resolvers. Mapping semantics make ``id in registry`` work like a dict.
    """

    def __init__(self, skills: Mapping[str, Skill]) -> None:
        # Defensive copy so external mutation can't surprise callers.
        self._skills: dict[str, Skill] = dict(skills)
        # Catch both "dict key dup" and "dict key disagrees with Skill.id"
        # in one pass — the Skill.id field is the canonical identifier.
        seen: dict[str, str] = {}
        bad_pairs: list[tuple[str, str]] = []
        for k, s in self._skills.items():
            if k in seen or s.id in seen:
                bad_pairs.append((k, s.id))
            else:
                seen[k] = s.id
            if k != s.id:
                bad_pairs.append((k, s.id))
        if bad_pairs:
            raise SkillSpecError(
                f"inconsistent skill identifiers in registry: {bad_pairs}"
            )

    # --------------------------------------------------------- Mapping API

    def __getitem__(self, key: str) -> Skill:
        return self._skills[key]

    def __iter__(self) -> Iterator[str]:
        return iter(self._skills)

    def __len__(self) -> int:
        return len(self._skills)

    def __contains__(self, key: object) -> bool:
        return key in self._skills

    # ----------------------------------------------------------- factory

    @classmethod
    def from_dict(cls, raw: Mapping[str, Any] | None) -> "SkillRegistry":
        """Build a :class:`SkillRegistry` from a YAML ``skills:`` mapping.

        Accepts both shapes:

        - The proto-shaped list (``[ {id, ...}, ... ]``) — entries are
          keyed by ``id``.
        - A pre-indexed dict (``{id: spec, ...}``) — convenient for
          callers that hand-build the mapping.

        ``None`` or empty input returns an empty registry.
        """
        if not raw:
            return cls({})

        # List shape → index by id.
        if isinstance(raw, list):
            indexed: dict[str, dict[str, Any]] = {}
            for entry in raw:
                if not isinstance(entry, dict):
                    raise SkillSpecError(
                        f"harness.skills[] entries must be dicts, got "
                        f"{type(entry).__name__}"
                    )
                entry_id = str(entry.get("id", "") or "").strip()
                if not entry_id:
                    raise SkillSpecError(
                        "harness.skills[] entry missing 'id'"
                    )
                if entry_id in indexed:
                    raise SkillSpecError(
                        f"duplicate skill id in harness.skills[]: "
                        f"{entry_id!r}"
                    )
                indexed[entry_id] = entry
            return cls({sid: Skill.from_spec(entry) for sid, entry in indexed.items()})

        # Dict shape → values are specs.
        if not isinstance(raw, Mapping):
            raise SkillSpecError(
                f"harness.skills must be a list or dict, got "
                f"{type(raw).__name__}"
            )
        return cls(
            {sid: Skill.from_spec(entry) for sid, entry in raw.items()}
        )


# ---------------------------------------------------------------------------
# Internal helpers
# ---------------------------------------------------------------------------


def _coerce_dict_list(raw: Any, *, field: str) -> list[dict[str, Any]]:
    if raw is None:
        return []
    if not isinstance(raw, list):
        raise SkillSpecError(f"skill.{field} must be a list of dicts")
    out: list[dict[str, Any]] = []
    for entry in raw:
        if not isinstance(entry, dict):
            raise SkillSpecError(
                f"skill.{field} entries must be dicts, got "
                f"{type(entry).__name__}"
            )
        out.append(dict(entry))
    return out


def _coerce_str_list(raw: Any, *, field: str) -> list[str]:
    if raw is None:
        return []
    if not isinstance(raw, list):
        raise SkillSpecError(f"skill.{field} must be a list of strings")
    return [str(v) for v in raw]


__all__ = ["Skill", "SkillRegistry", "SkillSpecError"]
