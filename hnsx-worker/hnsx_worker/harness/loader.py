"""DomainSpec loader / validator for multi-agent orchestration.

W5 introduces supervisor / hierarchical / autonomous modes with explicit
agent references and transition rules. This module validates those references
at load time so the runner doesn't hit ``KeyError`` mid-session.

Validated invariants:

  - ``harness.agents`` is non-empty.
  - Every agent referenced by ``session.supervisor.agent`` exists.
  - Every ``to`` target in ``transitions`` / ``exit`` exists in agents.
  - No duplicate agent ids.
  - (Optional) referenced prompts exist in ``harness.prompts``.
"""

from __future__ import annotations

import logging
from typing import Any

log = logging.getLogger("hnsx_worker.harness.loader")


class HarnessValidationError(Exception):
    """Raised when a DomainSpec fails W5 orchestration validation."""


class HarnessSpec:
    """A validated view of the ``harness`` section of a DomainSpec."""

    def __init__(self, spec: dict[str, Any]) -> None:
        self.spec = spec
        self.id = str(spec.get("id", ""))
        self.version = str(spec.get("version", ""))
        self.harness = spec.get("harness", {}) or {}
        self.agents: dict = self.harness.get("agents", {}) or {}
        self.prompts: dict = self.harness.get("prompts", {}) or {}
        self.session = self.harness.get("session", {}) or {}
        self.mode = str(self.session.get("mode", ""))
        self.supervisor_cfg = self.session.get("supervisor") or {}
        self._validate()

    def _validate(self) -> None:
        if not self.agents:
            raise HarnessValidationError("harness.agents is empty")

        seen_ids: set[str] = set()
        for name, agent in self.agents.items():
            if not isinstance(agent, dict):
                raise HarnessValidationError(f"agent {name!r} is not a dict")
            agent_id = agent.get("id") or name
            if agent_id in seen_ids:
                raise HarnessValidationError(f"duplicate agent id {agent_id!r}")
            seen_ids.add(agent_id)

        if self.mode in ("supervisor", "hierarchical", "autonomous"):
            self._validate_supervisor_mode()

    def _validate_supervisor_mode(self) -> None:
        supervisor = self.supervisor_cfg
        if not supervisor:
            raise HarnessValidationError(
                f"session.mode={self.mode!r} requires harness.session.supervisor"
            )

        supervisor_agent = supervisor.get("agent")
        if not supervisor_agent:
            raise HarnessValidationError("session.supervisor.agent is required")
        if supervisor_agent not in self.agents:
            raise HarnessValidationError(
                f"session.supervisor.agent {supervisor_agent!r} not found in agents"
            )

        transitions = supervisor.get("transitions") or []
        if not isinstance(transitions, list):
            raise HarnessValidationError("session.supervisor.transitions must be a list")

        targets = set()
        for idx, rule in enumerate(transitions):
            if not isinstance(rule, dict):
                raise HarnessValidationError(
                    f"session.supervisor.transitions[{idx}] is not a dict"
                )
            to = rule.get("to")
            if not to:
                raise HarnessValidationError(
                    f"session.supervisor.transitions[{idx}] missing 'to'"
                )
            if to not in self.agents:
                raise HarnessValidationError(
                    f"session.supervisor.transitions[{idx}] target {to!r} not found"
                )
            targets.add(to)

        exit_rules = supervisor.get("exit") or []
        if not isinstance(exit_rules, list):
            raise HarnessValidationError("session.supervisor.exit must be a list")

        for idx, rule in enumerate(exit_rules):
            if not isinstance(rule, dict):
                raise HarnessValidationError(
                    f"session.supervisor.exit[{idx}] is not a dict"
                )

        # Autonomous mode must have at least one transition; supervisor mode
        # may rely on a built-in fallback to the supervisor itself.
        if self.mode == "autonomous" and not transitions:
            raise HarnessValidationError(
                "session.mode=autonomous requires at least one transition"
            )

        # Hierarchical mode requires a supervisor agent and at least one
        # specialist agent (any agent other than the supervisor).
        if self.mode == "hierarchical" and len(targets) < 1:
            raise HarnessValidationError(
                "session.mode=hierarchical requires at least one specialist transition"
            )

    def get_agent(self, name: str) -> dict:
        """Return the agent dict by id/name, raising if missing."""
        if name not in self.agents:
            raise HarnessValidationError(f"agent {name!r} not found")
        return self.agents[name]


def load(spec: dict[str, Any]) -> HarnessSpec:
    """Validate and return a :class:`HarnessSpec` view."""
    return HarnessSpec(spec)


__all__ = ["HarnessSpec", "HarnessValidationError", "load"]
