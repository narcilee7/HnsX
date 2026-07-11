"""Builder helper for constructing HnsX DomainSpec dictionaries."""

from __future__ import annotations

from typing import Any


class DomainSpecBuilder:
    """Fluent builder for a HnsX DomainSpec YAML-serializable dict."""

    def __init__(self, id: str, version: str = "1.0.0", description: str = ""):  # noqa: A002
        self._spec: dict[str, Any] = {
            "id": id,
            "version": version,
            "description": description,
            "harness": {
                "agents": {},
                "prompts": {},
                "tools": {},
                "sandbox": {"policy": "none"},
                "policy": {},
                "session": {"mode": "single"},
            },
        }

    def with_agent(
        self,
        agent_id: str,
        *,
        provider: str,
        model: str,
        system_prompt: str,
        adapter: str | None = None,
    ) -> "DomainSpecBuilder":
        self._spec["harness"]["agents"][agent_id] = {
            "id": agent_id,
            "provider": provider,
            "model": model,
            "adapter": {"kind": adapter or provider},
            "system_prompt": system_prompt,
        }
        return self

    def with_prompt(self, prompt_id: str, template: str, *, prompt_type: str = "system") -> "DomainSpecBuilder":
        self._spec["harness"]["prompts"][prompt_id] = {
            "id": prompt_id,
            "type": prompt_type,
            "template": template,
        }
        return self

    def with_tool(self, tool_id: str, *, kind: str, name: str, description: str, config: dict[str, Any] | None = None) -> "DomainSpecBuilder":
        self._spec["harness"]["tools"][tool_id] = {
            "kind": kind,
            "name": name,
            "description": description,
            "config": config or {},
        }
        return self

    def with_session_mode(self, mode: str, *, agent: str | None = None) -> "DomainSpecBuilder":
        self._spec["harness"]["session"]["mode"] = mode
        if agent:
            self._spec["harness"]["session"]["agent"] = agent
        return self

    def with_trigger_schema(self, schema: dict[str, Any]) -> "DomainSpecBuilder":
        self._spec["harness"]["session"]["trigger_schema"] = schema
        return self

    def with_policy(
        self,
        *,
        max_cost_usd: float | None = None,
        max_turns: int | None = None,
        allow_shell: bool | None = None,
        allow_network: bool | None = None,
        presets: list[str] | None = None,
    ) -> "DomainSpecBuilder":
        policy = self._spec["harness"]["policy"]
        if max_cost_usd is not None:
            policy.setdefault("budget", {})["max_cost_usd"] = max_cost_usd
        if max_turns is not None:
            policy.setdefault("budget", {})["max_turns"] = max_turns
        if allow_shell is not None or allow_network is not None:
            perms = policy.setdefault("permissions", {})
            if allow_shell is not None:
                perms["allow_shell"] = allow_shell
            if allow_network is not None:
                perms["allow_network"] = allow_network
        if presets:
            policy["presets"] = presets
        return self

    def build(self) -> dict[str, Any]:
        return self._spec

    def to_yaml(self) -> str:
        import yaml

        return yaml.safe_dump(self._spec, sort_keys=False)
