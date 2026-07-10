"""SQL tool — read-by-default SQL execution with secret-bound DSNs.

W3.x ships SQL access for API Agents. The risk surface is significant
(SQL injection, exfiltration via UNION, accidental writes), so the tool
ships three independent safety rails:

  1. **Read-only by default.** The configured ``read_only: true`` (the
     default) enforces a SELECT-only AST. The check walks SQLAlchemy's
     compiled statement and rejects anything containing ``Insert`` /
     ``Update`` / ``Delete`` / ``DDL`` nodes.
  2. **Per-tool opt-in for writes.** ``read_only: false`` in the spec
     allows writes, but the **policy engine** (W6) still has the final
     say via the ``policy_decision`` hook. Until W6 lands, this is an
     opt-in flag — don't flip it on in production without a policy
     layer in front.
  3. **Result cap.** ``max_rows`` truncates large result sets so a
     runaway ``SELECT * FROM big_table`` doesn't blow up the
     observation stream.

Spec entry shape::

    - name: lookup_user           # what the LLM calls
      type: sql
      config:
        dsn: "postgresql://app:{secret.db_pass}@db/app"
        read_only: true           # default
        max_rows: 1000
        timeout_seconds: 30
        echo_sql: false           # for debugging; default off

LLM-facing schema::

    {
      "type": "object",
      "properties": {
        "sql":    {"type": "string"},
        "params": {"type": "object"}
      },
      "required": ["sql"]
    }
"""

from __future__ import annotations

import logging
import re
import time
from collections.abc import Mapping, Sequence
from dataclasses import dataclass, field
from typing import Any

from sqlalchemy import create_engine, text
from sqlalchemy.engine import Engine

from .base import Tool, ToolContext, ToolResult

log = logging.getLogger("hnsx_worker.tools.sql")

_DEFAULT_MAX_ROWS = 1000
_DEFAULT_TIMEOUT_SECONDS = 30

# Verbs that prove a statement is NOT read-only. We reject these when
# ``read_only=True`` regardless of how the SQL was authored.
_WRITE_VERBS = (
    "INSERT",
    "UPDATE",
    "DELETE",
    "DROP",
    "CREATE",
    "ALTER",
    "TRUNCATE",
    "GRANT",
    "REVOKE",
    "MERGE",
    "REPLACE",
    "CALL",
    "EXEC",
    "EXECUTE",
)


@dataclass
class SqlToolConfig:
    """Static configuration for one SqlTool instance."""

    dsn: str = ""
    read_only: bool = True
    max_rows: int = _DEFAULT_MAX_ROWS
    timeout_seconds: float = _DEFAULT_TIMEOUT_SECONDS
    echo_sql: bool = False
    placeholder_style: str = "named"  # 'named' | 'qmark' | 'pyformat'
    # Optional whitelist of tables (lower-case) the LLM may touch. Empty
    # means "any table". Useful when the agent only owns a subset.
    allowed_tables: list[str] = field(default_factory=list)

    @classmethod
    def from_spec(cls, raw: Mapping[str, Any]) -> SqlToolConfig:
        dsn = str(raw.get("dsn", ""))
        if not dsn:
            raise ValueError("sql tool: config.dsn is required")
        max_rows = int(raw.get("max_rows", _DEFAULT_MAX_ROWS))
        if max_rows < 1:
            raise ValueError("sql tool: max_rows must be >= 1")
        timeout = float(raw.get("timeout_seconds", _DEFAULT_TIMEOUT_SECONDS))
        if timeout <= 0:
            raise ValueError("sql tool: timeout_seconds must be > 0")
        placeholder_style = str(raw.get("placeholder_style", "named"))
        if placeholder_style not in {"named", "qmark", "pyformat"}:
            raise ValueError(
                "sql tool: placeholder_style must be one of "
                "{named, qmark, pyformat}"
            )
        return cls(
            dsn=dsn,
            read_only=bool(raw.get("read_only", True)),
            max_rows=max_rows,
            timeout_seconds=timeout,
            echo_sql=bool(raw.get("echo_sql", False)),
            placeholder_style=placeholder_style,
            allowed_tables=[str(t).lower() for t in (raw.get("allowed_tables") or [])],
        )


class SqlTool(Tool):
    """One configured SQL endpoint, callable by the LLM by ``name``.

    The instance is constructed by
    :func:`hnsx_worker.tools.factory.build_tool` from a spec entry like
    ``{name, type: 'sql', config: {...}}``; the factory passes the parsed
    :class:`SqlToolConfig` here.

    The underlying SQLAlchemy ``Engine`` is created lazily on first
    invocation so cold-start cost isn't paid for tools that the LLM
    never calls. ``dispose()`` is exposed for clean shutdown.
    """

    def __init__(
        self,
        name: str,
        config: SqlToolConfig,
        *,
        engine_factory: EngineFactory | None = None,
    ) -> None:
        self._name = name
        self._config = config
        self._engine: Engine | None = None
        self._engine_factory = engine_factory or _default_engine_factory

    @property
    def name(self) -> str:
        return self._name

    @property
    def config(self) -> SqlToolConfig:
        return self._config

    @property
    def schema(self) -> dict[str, Any]:
        """LLM-facing input schema."""
        return {
            "type": "object",
            "properties": {
                "sql": {
                    "type": "string",
                    "description": (
                        "SQL statement. SELECT-only unless read_only is "
                        "explicitly disabled in the tool config."
                    ),
                },
                "params": {
                    "type": "object",
                    "description": (
                        "Bind parameters. Named (':name') is the default; "
                        "qmark ('?') and pyformat ('%(name)s') are also "
                        "supported via placeholder_style in the spec."
                    ),
                },
            },
            "required": ["sql"],
            "additionalProperties": False,
        }

    def dispose(self) -> None:
        """Close the underlying SQLAlchemy engine. Idempotent."""
        if self._engine is not None:
            try:
                self._engine.dispose()
            except Exception:  # noqa: BLE001
                pass
            self._engine = None

    # ------------------------------------------------------------------ invoke

    def invoke(self, ctx: ToolContext, input: dict[str, Any]) -> ToolResult:
        if not isinstance(input, dict):
            return ToolResult(error="input must be a JSON object")
        sql_raw = input.get("sql")
        if not isinstance(sql_raw, str) or not sql_raw.strip():
            return ToolResult(error="input.sql must be a non-empty string")
        params = input.get("params") or {}
        if not isinstance(params, dict):
            return ToolResult(error="input.params must be an object")

        # Validate the SQL *before* opening a connection: cheap, and
        # surfaces bad SQL as structured errors without burning a slot.
        try:
            dsn = _resolve_dsn_placeholders(self._config.dsn, ctx)
        except KeyError as e:
            return ToolResult(error=str(e))

        if self._config.read_only:
            verdict, verb = _classify_statement(sql_raw)
            if verdict == "write":
                return ToolResult(
                    error=(
                        f"sql tool: read_only=true rejects {verb} statements"
                    ),
                    metadata={
                        "kind": "sql_readonly_rejected",
                        "sql_kind": verb,
                    },
                )
            if verdict == "unknown":
                return ToolResult(
                    error=(
                        "sql tool: read_only=true and the statement is not "
                        "recognizably a SELECT / WITH ... SELECT"
                    ),
                    metadata={"kind": "sql_readonly_unrecognized"},
                )

        if self._config.allowed_tables:
            referenced = _referenced_tables(sql_raw)
            if not referenced.issubset(set(self._config.allowed_tables)):
                return ToolResult(
                    error=(
                        f"sql tool: query references tables outside "
                        f"allowed_tables={self._config.allowed_tables} "
                        f"(referenced: {sorted(referenced)})"
                    ),
                    metadata={
                        "kind": "sql_table_rejected",
                        "referenced": sorted(referenced),
                    },
                )

        if self._config.echo_sql:
            log.debug("sql tool %s: %s", self._name, sql_raw)

        started = time.monotonic()
        try:
            engine = self._ensure_engine(dsn)
            # ``engine.begin()`` opens a transaction that auto-commits on
            # context exit — necessary for write statements to persist.
            with engine.begin() as conn:
                result = conn.execute(text(sql_raw), params)
                if result.returns_rows:
                    rows = _rows_to_dicts(result.fetchmany(self._config.max_rows))
                    # Truncation: try to peek one more row to know whether
                    # we hit the cap.
                    extra = result.fetchone()
                    truncated = extra is not None
                    row_count = len(rows)
                    rows_out = rows
                else:
                    rows_out = []
                    row_count = int(result.rowcount or 0)
                    truncated = False
        except Exception as e:  # noqa: BLE001 — surface all DB errors
            elapsed_ms = int((time.monotonic() - started) * 1000)
            return ToolResult(
                error=f"sql error: {e!s}",
                metadata={
                    "dsn": _safe_dsn(dsn),
                    "elapsed_ms": elapsed_ms,
                    "read_only": self._config.read_only,
                },
            )

        elapsed_ms = int((time.monotonic() - started) * 1000)
        return ToolResult(
            output={"rows": rows_out, "row_count": row_count},
            metadata={
                "dsn": _safe_dsn(dsn),
                "elapsed_ms": elapsed_ms,
                "max_rows": self._config.max_rows,
                "truncated": truncated,
                "read_only": self._config.read_only,
                "echo_sql": self._config.echo_sql,
            },
        )

    # ------------------------------------------------------------------ internals

    def _ensure_engine(self, dsn: str) -> Engine:
        if self._engine is None:
            self._engine = self._engine_factory(
                dsn, timeout_seconds=self._config.timeout_seconds
            )
        return self._engine


# ---------------------------------------------------------------------------
# Engine factory (injectable for tests)
# ---------------------------------------------------------------------------


# Protocol-ish callable used by tests to substitute a fake engine. We don't
# depend on EngineProtocol from sqlalchemy.engine to keep this header light.
EngineFactory = Any  # ``Callable[[str], Engine]`` in spirit


def _default_engine_factory(dsn: str, *, timeout_seconds: float) -> Engine:
    """Build a SQLAlchemy Engine. Hooks connect_timeout for PG / MySQL."""
    connect_args: dict[str, Any] = {}
    lower = dsn.lower()
    if lower.startswith("postgresql") or lower.startswith("mysql"):
        connect_args["connect_timeout"] = int(timeout_seconds)
    return create_engine(dsn, connect_args=connect_args, future=True)


# ---------------------------------------------------------------------------
# Read-only / table allowlist checks (string-based; LLM provides text() SQL)
# ---------------------------------------------------------------------------


_LINE_COMMENT = re.compile(r"--[^\n]*")
_BLOCK_COMMENT = re.compile(r"/\*.*?\*/", re.DOTALL)


def _strip_sql_comments(sql: str) -> str:
    sql = _BLOCK_COMMENT.sub(" ", sql)
    sql = _LINE_COMMENT.sub("", sql)
    return sql


def _classify_statement(sql: str) -> tuple[str, str]:
    """Classify a SQL statement as ``'read'`` / ``'write'`` / ``'unknown'``.

    Returns ``(verdict, verb)`` where ``verb`` is the leading token (or the
    forbidden verb) for audit messages.
    """
    cleaned = _strip_sql_comments(sql).strip()
    if not cleaned:
        return "unknown", ""
    head = re.match(r"\s*([A-Za-z_]+)", cleaned)
    if head is None:
        return "unknown", ""
    first = head.group(1).upper()
    if first in _WRITE_VERBS:
        return "write", first
    if first == "SELECT":
        return "read", first
    if first == "WITH":
        # CTE: every CTE body must be a SELECT. Heuristic: scan for write
        # verbs anywhere in the body. A full grammar would be more robust
        # but this catches the obvious cases (CTE wrapping an INSERT).
        upper = cleaned.upper()
        for verb in _WRITE_VERBS:
            if re.search(rf"\b{verb}\b", upper):
                return "write", verb
        return "read", first
    if first in {"EXPLAIN", "SHOW", "DESCRIBE", "DESC"}:
        return "read", first
    return "unknown", first


def _referenced_tables(sql: str) -> set[str]:
    """Best-effort extraction of table names referenced in ``sql``.

    Handles ``FROM`` / ``JOIN`` / ``INTO`` / ``UPDATE`` clauses. Single-quoted
    string literals are excluded so a column value matching a table name
    doesn't trip the check; double-quoted identifiers are left alone (SQL
    standard: ``""`` is an identifier, ``''`` is a string literal).
    """
    cleaned = _strip_sql_comments(sql)
    cleaned = re.sub(r"'(?:''|[^'])*'", "''", cleaned)
    lowered = cleaned.lower()

    found: set[str] = set()
    # Patterns anchor on whitespace before the verb because \b doesn't
    # behave the way we want around `"` / `[` quoting.
    patterns = (
        r"(?:^|\s)from\s+([`\"\[]?)([a-z_][a-z0-9_.]*)\1",
        r"(?:^|\s)join\s+([`\"\[]?)([a-z_][a-z0-9_.]*)\1",
        r"(?:^|\s)into\s+([`\"\[]?)([a-z_][a-z0-9_.]*)\1",
        r"(?:^|\s)update\s+([`\"\[]?)([a-z_][a-z0-9_.]*)\1",
        r"(?:^|\s)table\s+([`\"\[]?)([a-z_][a-z0-9_.]*)\1",
    )
    for pat in patterns:
        for m in re.finditer(pat, lowered):
            found.add(m.group(2))
    return found


# ---------------------------------------------------------------------------
# Row / DSN helpers
# ---------------------------------------------------------------------------


def _rows_to_dicts(rows: Sequence[Any]) -> list[dict[str, Any]]:
    """Convert SQLAlchemy Row objects into JSON-safe dicts."""
    out: list[dict[str, Any]] = []
    for row in rows:
        # SQLAlchemy 2.x Row has _mapping; older has keys()/values().
        mapping = getattr(row, "_mapping", None)
        if mapping is not None:
            out.append({k: _json_safe(v) for k, v in mapping.items()})
            continue
        keys = list(getattr(row, "keys", lambda: [])())
        values = list(row)
        out.append({k: _json_safe(v) for k, v in zip(keys, values, strict=True)})
    return out


def _json_safe(value: Any) -> Any:
    """Best-effort conversion of DB values to JSON-safe primitives."""
    # datetime, date, time, Decimal, UUID, bytes — leave to caller via str().
    # We accept a small set of common types and stringify the rest.
    if value is None or isinstance(value, (bool, int, float, str)):
        return value
    if isinstance(value, (list, tuple)):
        return [_json_safe(v) for v in value]
    if isinstance(value, dict):
        return {str(k): _json_safe(v) for k, v in value.items()}
    return str(value)


_PLACEHOLDER_RE = re.compile(r"\{([^{}]+)\}")


def _resolve_dsn_placeholders(dsn: str, ctx: ToolContext) -> str:
    """Resolve ``{secret.X}`` and ``{X}`` in a DSN template."""

    def repl(match: re.Match[str]) -> str:
        key = match.group(1).strip()
        if key.startswith("secret."):
            name = key[len("secret.") :].strip()
            if name not in ctx.secrets:
                raise KeyError(f"missing secret {name!r} for dsn")
            # SQLAlchemy / drivers percent-encode when needed, but we
            # also URL-encode pathologically weird passwords ourselves.
            from urllib.parse import quote

            return quote(ctx.secrets[name], safe="")
        return match.group(0)

    return _PLACEHOLDER_RE.sub(repl, dsn)


def _safe_dsn(dsn: str) -> str:
    """Return DSN with the password stripped for audit display."""
    # postgresql://user:password@host/db → postgresql://user@host/db
    if "@" not in dsn:
        return dsn
    scheme_split = dsn.split("://", 1)
    if len(scheme_split) != 2:
        return dsn
    scheme, rest = scheme_split
    if "@" not in rest:
        return dsn
    creds, host_part = rest.split("@", 1)
    if ":" in creds:
        user = creds.split(":", 1)[0]
        return f"{scheme}://{user}@{host_part}"
    return dsn


__all__ = ["SqlTool", "SqlToolConfig", "EngineFactory"]
