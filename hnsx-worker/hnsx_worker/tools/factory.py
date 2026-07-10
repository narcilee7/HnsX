"""Tool factory — instantiate a ``Tool`` from a spec entry.

A spec entry in ``agent.tools[]`` looks like::

    - name: fetch_user           # required
      type: http                 # required for built-in tools
      config:                    # type-specific config
        method: GET
        url: "..."

The factory maps the ``type`` field to a class. Unknown types raise
``ValueError`` so the executor surfaces a structured error instead of
silently dropping the tool.

Entries that have a ``name`` but no ``type`` are treated as **external
references** — the LLM sees the tool but the executor doesn't dispatch
to it. This is how CLI-agent tools (Claude Code / Codex) and remote
tools will be declared in W4 / W5.

Spec entries that lack ``name`` are rejected: the LLM-facing schema
needs a stable identifier.
"""

from __future__ import annotations

from typing import Any

from .base import Tool
from .http import HttpTool, HttpToolConfig
from .python import PythonTool, PythonToolConfig
from .sql import SqlTool, SqlToolConfig

# Mapping from spec type string → (config_class, tool_class).
# Adding a new built-in tool is a one-line edit here.
_BUILTIN_TOOLS: dict[str, tuple[type, type]] = {
    "http": (HttpToolConfig, HttpTool),
    "sql": (SqlToolConfig, SqlTool),
    "python": (PythonToolConfig, PythonTool),
}


def build_tool(spec: dict[str, Any]) -> Tool:
    """Construct a ``Tool`` from a spec entry.

    Raises:
        ValueError: when the entry is missing ``name`` / ``type`` or when
            ``type`` is unknown.

    Returns:
        A configured :class:`Tool` instance ready to register in a
        :class:`ToolRegistry`.
    """
    if not isinstance(spec, dict):
        raise ValueError(f"tool spec must be a dict, got {type(spec).__name__}")

    name = str(spec.get("name", "")).strip()
    if not name:
        raise ValueError("tool spec requires 'name'")

    tool_type = spec.get("type")
    if not tool_type:
        # External reference: no built-in implementation. Callers that
        # need this (CLI-agent adapters in W4) can register their own
        # Tool subclass via ToolRegistry.register before the executor
        # dispatches. Here we surface a structured error so a missing
        # type doesn't silently pass through.
        raise ValueError(
            f"tool {name!r}: spec has no 'type'; built-in tools must declare "
            f"one of {sorted(_BUILTIN_TOOLS)} or be registered externally"
        )

    entry = _BUILTIN_TOOLS.get(str(tool_type))
    if entry is None:
        raise ValueError(
            f"tool {name!r}: unknown type {tool_type!r} "
            f"(known: {sorted(_BUILTIN_TOOLS)})"
        )
    config_cls, tool_cls = entry
    try:
        config = config_cls.from_spec(spec.get("config") or {})
    except ValueError as e:
        raise ValueError(f"tool {name!r}: {e}") from e
    return tool_cls(name, config)


def tool_schemas_for_adapter(spec: dict[str, Any]) -> list[dict[str, Any]]:
    """Translate ``agent.tools`` into the LLM-facing tool definition list.

    Only entries with a known built-in type are translated; unknown /
    external entries are passed through with their declared
    ``name`` / ``description`` / ``input_schema`` if present, otherwise
    as a name-only stub. This is what adapters (anthropic / openai /
    ollama) ultimately see in their ``tools=`` request parameter.
    """
    raw_tools = spec.get("tools") or []
    out: list[dict[str, Any]] = []
    for entry in raw_tools:
        if not isinstance(entry, dict):
            continue
        name = str(entry.get("name", "")).strip()
        if not name:
            continue
        tool_type = entry.get("type")
        # If we can build the tool, ask it for its schema.
        if tool_type and tool_type in _BUILTIN_TOOLS:
            try:
                tool = build_tool(entry)
                out.append(
                    {
                        "name": tool.name,
                        "description": entry.get("description", ""),
                        "input_schema": tool.schema,
                    }
                )
                continue
            except ValueError:
                # Fall through to a passthrough shape below.
                pass
        # External / unknown: use whatever the spec declared, with a
        # permissive fallback schema so the LLM can still call it.
        out.append(
            {
                "name": name,
                "description": entry.get("description", ""),
                "input_schema": entry.get("input_schema")
                or entry.get("schema")
                or {"type": "object"},
            }
        )
    return out


__all__ = ["build_tool", "tool_schemas_for_adapter"]
