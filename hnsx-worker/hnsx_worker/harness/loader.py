"""DomainSpec data view for multi-agent orchestration.

W16+: this module is intentionally a thin data view. Authoritative
validation of a DomainSpec happens in the Go server
(``pkg/domain/loader.go::Validate``) and is invoked by
``session_runtime.main()`` via the ``ValidateDomain`` gRPC call before any
session runs.

The Python worker keeps only:

  - :class:`HarnessSpec` — typed accessors over the harness section.
  - :class:`SkillRegistry` construction — proto-shaped marshalling, not
    domain-rule validation.
  - :exc:`HarnessValidationError` — used when a runtime lookup (e.g.
    ``get_agent``) cannot resolve a reference.

Do not add new orchestration/supervisor/skill-cross-reference checks here.
Add them server-side and expose them through ``ValidateDomain``.
"""

from __future__ import annotations

from typing import Any

from hnsx_worker.skills import SkillRegistry, SkillSpecError


class HarnessValidationError(Exception):
    """Raised when a runtime harness lookup fails."""


class HarnessSpec:
    """A typed view of the ``harness`` section of a DomainSpec."""

    def __init__(self, spec: dict[str, Any]) -> None:
        self.spec = spec
        self.id = str(spec.get("id", ""))
        self.version = str(spec.get("version", ""))
        self.harness = spec.get("harness", {}) or {}
        self.agents: dict = self.harness.get("agents", {}) or {}
        self.prompts: dict = self.harness.get("prompts", {}) or {}
        try:
            self.skills: SkillRegistry = SkillRegistry.from_dict(
                self.harness.get("skills") or {}
            )
        except SkillSpecError as exc:
            raise HarnessValidationError(str(exc)) from exc
        self.session = self.harness.get("session", {}) or {}
        self.mode = str(self.session.get("mode", ""))
        self.supervisor_cfg = self.session.get("supervisor") or {}
        self.orchestration = self.harness.get("orchestration") or {}
        self.strategy = str(self.orchestration.get("strategy", "direct"))

    def get_agent(self, name: str) -> dict:
        """Return the agent dict by id/name, raising if missing."""
        if name not in self.agents:
            raise HarnessValidationError(f"agent {name!r} not found")
        return self.agents[name]


def load(spec: dict[str, Any]) -> HarnessSpec:
    """Return a :class:`HarnessSpec` view without duplicate validation."""
    return HarnessSpec(spec)


__all__ = ["HarnessSpec", "HarnessValidationError", "load"]
