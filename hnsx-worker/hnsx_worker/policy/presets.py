"""Policy presets — named bundles that expand into concrete policy config.

Presets keep the DomainSpec YAML concise for common scenarios while the
runtime still sees fully expanded budget / permission / approval / guardrail
values.
"""

from __future__ import annotations

from typing import Any

PRESET_REGISTRY: dict[str, dict[str, Any]] = {
    "safe_customer_service": {
        "budget": {
            "max_cost_usd": 1.0,
            "max_turns": 20,
        },
        "permissions": {
            "allow_network": True,
            "allow_file_write": False,
            "allow_file_delete": False,
            "allow_shell": False,
        },
        "approval": {
            "default_timeout_seconds": 600,
            "required_for": {
                "tools": ["issue_refund", "export_customer_data"],
                "resources": ["billing:write", "customer:*"],
            },
        },
        "output_guardrails": {
            "blocked_keywords": ["password", "credit card", "ssn"],
        },
    },
}


def known_presets() -> list[str]:
    """Return every registered preset name."""
    return list(PRESET_REGISTRY.keys())


def expand_presets(policy: dict[str, Any]) -> dict[str, Any]:
    """Return a new policy dict with named presets merged in.

    User-provided values take precedence over preset defaults. Multiple presets
    are merged in order; later presets override earlier ones.

    Raises:
        ValueError: if a preset name is not registered.
    """
    out: dict[str, Any] = {k: v for k, v in policy.items() if k != "presets"}
    presets = policy.get("presets") or []
    for name in presets:
        preset = PRESET_REGISTRY.get(name)
        if preset is None:
            raise ValueError(
                f"unknown policy preset {name!r} (known: {', '.join(known_presets())})"
            )
        out = _merge_policy(out, preset)
    return out


def _merge_policy(dst: dict[str, Any], src: dict[str, Any]) -> dict[str, Any]:
    """Merge ``src`` preset defaults into ``dst`` (user values win)."""
    out: dict[str, Any] = {}
    for key in set(dst) | set(src):
        dst_val = dst.get(key)
        src_val = src.get(key)
        if isinstance(dst_val, dict) and isinstance(src_val, dict):
            out[key] = _merge_dict(dst_val, src_val)
        elif isinstance(dst_val, list) and isinstance(src_val, list):
            out[key] = _merge_list(dst_val, src_val)
        elif dst_val is not None and dst_val != {} and dst_val != []:
            out[key] = dst_val
        else:
            out[key] = src_val
    return out


def _merge_dict(dst: dict[str, Any], src: dict[str, Any]) -> dict[str, Any]:
    out = dict(src)
    for k, v in dst.items():
        if isinstance(v, dict) and isinstance(out.get(k), dict):
            out[k] = _merge_dict(v, out[k])
        elif isinstance(v, list) and isinstance(out.get(k), list):
            out[k] = _merge_list(v, out[k])
        elif v is not None and v != {} and v != []:
            out[k] = v
    return out


def _merge_list(dst: list[Any], src: list[Any]) -> list[Any]:
    seen = set()
    out: list[Any] = []
    for item in dst + src:
        key = _item_key(item)
        if key in seen:
            continue
        seen.add(key)
        out.append(item)
    return out


def _item_key(item: Any) -> Any:
    """Return a hashable key for deduplicating merged list items."""
    if isinstance(item, dict):
        return tuple(sorted((k, _item_key(v)) for k, v in item.items()))
    if isinstance(item, list):
        return tuple(_item_key(v) for v in item)
    return item
