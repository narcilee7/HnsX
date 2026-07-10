"""Tests for the SQL tool (W3.x).

Coverage:

  - Config parsing + validation (dsn / max_rows / timeout / placeholder).
  - Schema exposure (sql + params).
  - SELECT happy path returns rows + metadata.
  - INSERT/UPDATE/DELETE blocked when read_only=True (default).
  - INSERT allowed when read_only=False.
  - allowed_tables enforces access (positive + negative).
  - Empty sql → structured error.
  - SQL parse error → structured error.
  - max_rows truncation.
  - {secret.X} resolved in DSN.
  - _safe_dsn strips the password.
  - Tool uses injected engine factory (no real DB connection).
"""

from __future__ import annotations

from typing import Any

import pytest
from sqlalchemy import create_engine, text

from hnsx_worker.tools import ToolContext
from hnsx_worker.tools.sql import (
    SqlTool,
    SqlToolConfig,
    _classify_statement,
    _json_safe,
    _referenced_tables,
    _rows_to_dicts,
    _safe_dsn,
)

# ---------------------------------------------------------------------------
# Test fixtures: an in-memory sqlite with a couple of tables
# ---------------------------------------------------------------------------


@pytest.fixture()
def engine():
    eng = create_engine("sqlite:///:memory:", future=True)
    with eng.begin() as conn:
        conn.execute(text("CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT, age INTEGER)"))
        conn.execute(text("CREATE TABLE secrets (id INTEGER PRIMARY KEY, value TEXT)"))
        conn.execute(text("INSERT INTO users (name, age) VALUES ('alice', 30)"))
        conn.execute(text("INSERT INTO users (name, age) VALUES ('bob', 25)"))
        conn.execute(text("INSERT INTO secrets (value) VALUES ('shh')"))
    yield eng
    eng.dispose()


def _factory(eng):
    """Return an engine_factory that ignores its dsn arg and returns ``eng``."""

    def _factory(dsn: str, *, timeout_seconds: float) -> Any:
        return eng

    return _factory


def _make_tool(
    eng,
    *,
    name: str = "lookup",
    config: dict[str, Any] | None = None,
) -> SqlTool:
    base = {"dsn": "sqlite:///:memory:", "read_only": True, "max_rows": 100}
    return SqlTool(
        name,
        SqlToolConfig.from_spec({**base, **(config or {})}),
        engine_factory=_factory(eng),
    )


# ---------------------------------------------------------------------------
# Config parsing
# ---------------------------------------------------------------------------


def test_config_requires_dsn() -> None:
    with pytest.raises(ValueError, match="dsn is required"):
        SqlToolConfig.from_spec({})


def test_config_rejects_non_positive_max_rows() -> None:
    with pytest.raises(ValueError, match="max_rows"):
        SqlToolConfig.from_spec({"dsn": "x", "max_rows": 0})


def test_config_rejects_non_positive_timeout() -> None:
    with pytest.raises(ValueError, match="timeout_seconds"):
        SqlToolConfig.from_spec({"dsn": "x", "timeout_seconds": -1})


def test_config_rejects_unknown_placeholder_style() -> None:
    with pytest.raises(ValueError, match="placeholder_style"):
        SqlToolConfig.from_spec({"dsn": "x", "placeholder_style": "weird"})


def test_config_defaults_to_read_only() -> None:
    cfg = SqlToolConfig.from_spec({"dsn": "x"})
    assert cfg.read_only is True
    assert cfg.max_rows == 1000
    assert cfg.timeout_seconds == 30
    assert cfg.allowed_tables == []


def test_config_parses_all_fields() -> None:
    cfg = SqlToolConfig.from_spec(
        {
            "dsn": "postgresql://x/y",
            "read_only": False,
            "max_rows": 50,
            "timeout_seconds": 5,
            "placeholder_style": "qmark",
            "allowed_tables": ["users"],
        }
    )
    assert cfg.read_only is False
    assert cfg.max_rows == 50
    assert cfg.timeout_seconds == 5
    assert cfg.placeholder_style == "qmark"
    assert cfg.allowed_tables == ["users"]


# ---------------------------------------------------------------------------
# Schema
# ---------------------------------------------------------------------------


def test_schema_requires_sql() -> None:
    tool = _make_tool(None)  # type: ignore[arg-type]
    schema = tool.schema
    assert "sql" in schema["properties"]
    assert schema["required"] == ["sql"]
    assert "params" in schema["properties"]


# ---------------------------------------------------------------------------
# Happy path
# ---------------------------------------------------------------------------


def test_select_returns_rows(engine) -> None:
    tool = _make_tool(engine)
    result = tool.invoke(ToolContext(), {"sql": "SELECT id, name FROM users ORDER BY id"})

    assert result.ok, result.error
    assert result.output["rows"] == [
        {"id": 1, "name": "alice"},
        {"id": 2, "name": "bob"},
    ]
    assert result.output["row_count"] == 2
    assert result.metadata["read_only"] is True
    assert result.metadata["truncated"] is False
    assert result.metadata["max_rows"] == 100
    assert result.metadata["dsn"] == "sqlite:///:memory:"


def test_select_with_named_params(engine) -> None:
    tool = _make_tool(engine)
    result = tool.invoke(
        ToolContext(),
        {"sql": "SELECT name FROM users WHERE age > :min_age", "params": {"min_age": 26}},
    )
    assert result.ok
    assert result.output["rows"] == [{"name": "alice"}]


def test_select_with_qmark_params(engine) -> None:
    cfg = {"placeholder_style": "qmark"}
    tool = _make_tool(engine, config=cfg)
    result = tool.invoke(
        ToolContext(),
        {"sql": "SELECT name FROM users WHERE age > ?", "params": {"age": 26}},
    )
    # SQLAlchemy text() requires named placeholders for params mapping, so
    # this should fall through as a SQL error (structured), not a crash.
    assert not result.ok
    assert "sql error" in (result.error or "")


# ---------------------------------------------------------------------------
# Read-only enforcement
# ---------------------------------------------------------------------------


def test_insert_blocked_when_read_only(engine) -> None:
    tool = _make_tool(engine)
    result = tool.invoke(
        ToolContext(),
        {"sql": "INSERT INTO users (name, age) VALUES ('eve', 40)"},
    )
    assert not result.ok
    assert "read_only" in (result.error or "")
    assert result.metadata.get("kind") == "sql_readonly_rejected"

    # And the table is unchanged.
    with engine.connect() as conn:
        cnt = conn.execute(text("SELECT COUNT(*) FROM users")).scalar_one()
    assert cnt == 2


def test_update_blocked_when_read_only(engine) -> None:
    tool = _make_tool(engine)
    result = tool.invoke(
        ToolContext(),
        {"sql": "UPDATE users SET age = 99 WHERE id = 1"},
    )
    assert not result.ok
    assert "read_only" in (result.error or "")


def test_delete_blocked_when_read_only(engine) -> None:
    tool = _make_tool(engine)
    result = tool.invoke(
        ToolContext(),
        {"sql": "DELETE FROM users WHERE id = 1"},
    )
    assert not result.ok
    assert "read_only" in (result.error or "")


def test_create_table_blocked_when_read_only(engine) -> None:
    tool = _make_tool(engine)
    result = tool.invoke(
        ToolContext(),
        {"sql": "CREATE TABLE evil (id INTEGER)"},
    )
    assert not result.ok
    assert "read_only" in (result.error or "")


def test_insert_allowed_when_read_only_false(engine) -> None:
    tool = _make_tool(engine, config={"read_only": False})
    result = tool.invoke(
        ToolContext(),
        {"sql": "INSERT INTO users (name, age) VALUES ('eve', 40)"},
    )
    assert result.ok, result.error
    with engine.connect() as conn:
        cnt = conn.execute(text("SELECT COUNT(*) FROM users")).scalar_one()
    assert cnt == 3


# ---------------------------------------------------------------------------
# Table allowlist
# ---------------------------------------------------------------------------


def test_allowed_tables_blocks_unlisted_table(engine) -> None:
    tool = _make_tool(engine, config={"allowed_tables": ["users"]})
    result = tool.invoke(
        ToolContext(),
        {"sql": "SELECT value FROM secrets LIMIT 1"},
    )
    assert not result.ok
    assert "allowed_tables" in (result.error or "")


def test_allowed_tables_allows_listed_table(engine) -> None:
    tool = _make_tool(engine, config={"allowed_tables": ["users"]})
    result = tool.invoke(
        ToolContext(),
        {"sql": "SELECT name FROM users LIMIT 1"},
    )
    assert result.ok


def test_allowed_tables_case_insensitive(engine) -> None:
    tool = _make_tool(engine, config={"allowed_tables": ["USERS"]})
    result = tool.invoke(
        ToolContext(),
        {"sql": "SELECT name FROM users LIMIT 1"},
    )
    assert result.ok


# ---------------------------------------------------------------------------
# Input validation
# ---------------------------------------------------------------------------


def test_empty_sql_returns_error(engine) -> None:
    tool = _make_tool(engine)
    result = tool.invoke(ToolContext(), {"sql": "   "})
    assert not result.ok
    assert "non-empty" in (result.error or "")


def test_params_must_be_dict(engine) -> None:
    tool = _make_tool(engine)
    result = tool.invoke(ToolContext(), {"sql": "SELECT 1", "params": "bad"})
    assert not result.ok
    assert "params" in (result.error or "")


def test_invalid_sql_returns_parse_error(engine) -> None:
    tool = _make_tool(engine)
    result = tool.invoke(ToolContext(), {"sql": "NOT VALID SQL AT ALL"})
    assert not result.ok
    # Either a parse error from SQLAlchemy or a runtime error from the
    # driver is acceptable — what matters is "structured, no crash".
    assert result.error is not None


def test_input_must_be_dict(engine) -> None:
    tool = _make_tool(engine)
    result = tool.invoke(ToolContext(), "not a dict")  # type: ignore[arg-type]
    assert not result.ok
    assert "JSON object" in (result.error or "")


# ---------------------------------------------------------------------------
# max_rows truncation
# ---------------------------------------------------------------------------


def test_max_rows_truncates_large_result(engine) -> None:
    cfg = {"max_rows": 1}
    tool = _make_tool(engine, config=cfg)
    result = tool.invoke(ToolContext(), {"sql": "SELECT id FROM users ORDER BY id"})
    assert result.ok
    assert result.output["row_count"] == 1
    assert result.metadata["truncated"] is True


# ---------------------------------------------------------------------------
# DSN secret injection + safe_dsn
# ---------------------------------------------------------------------------


def test_dsn_secret_is_resolved(engine) -> None:
    """The factory we inject receives the resolved DSN — verify that path."""
    seen: list[str] = []

    def factory(dsn: str, *, timeout_seconds: float) -> Any:
        seen.append(dsn)
        return engine

    tool = SqlTool(
        "q",
        SqlToolConfig.from_spec({"dsn": "postgresql://u:{secret.pw}@h/db"}),
        engine_factory=factory,
    )
    result = tool.invoke(ToolContext(secrets={"pw": "p@ss w0rd"}), {"sql": "SELECT 1"})
    assert result.ok
    # URL-encoded value: 'p%40ss%20w0rd'
    assert "p%40ss%20w0rd" in seen[0]
    assert "{secret.pw}" not in seen[0]


def test_missing_secret_returns_error(engine) -> None:
    tool = SqlTool(
        "q",
        SqlToolConfig.from_spec({"dsn": "postgresql://u:{secret.missing}@h/db"}),
        engine_factory=_factory(engine),
    )
    result = tool.invoke(ToolContext(secrets={}), {"sql": "SELECT 1"})
    assert not result.ok
    assert "missing secret" in (result.error or "")


def test_safe_dsn_strips_password() -> None:
    assert _safe_dsn("postgresql://user:secret@host/db") == "postgresql://user@host/db"
    assert _safe_dsn("postgresql://user@host/db") == "postgresql://user@host/db"
    assert _safe_dsn("sqlite:///:memory:") == "sqlite:///:memory:"


# ---------------------------------------------------------------------------
# Row serialization
# ---------------------------------------------------------------------------


def test_rows_to_dicts_handles_mapping_rows(engine) -> None:
    with engine.connect() as conn:
        result = conn.execute(text("SELECT id, name FROM users ORDER BY id"))
        rows = _rows_to_dicts(result.fetchall())
    assert rows == [{"id": 1, "name": "alice"}, {"id": 2, "name": "bob"}]


def test_json_safe_converts_decimals_and_dates() -> None:
    from datetime import date
    from decimal import Decimal

    assert _json_safe(Decimal("1.23")) == "1.23"
    assert _json_safe(date(2026, 7, 9)) == "2026-07-09"
    assert _json_safe([1, "x"]) == [1, "x"]
    assert _json_safe({"k": Decimal("2.5")}) == {"k": "2.5"}
    assert _json_safe(None) is None
    assert _json_safe(True) is True


# ---------------------------------------------------------------------------
# Read-only AST walker (unit tests for the helper)
# ---------------------------------------------------------------------------


def test_classify_select_as_read() -> None:
    verdict, verb = _classify_statement("SELECT 1")
    assert verdict == "read"
    assert verb == "SELECT"


def test_classify_with_select_as_read() -> None:
    verdict, _ = _classify_statement("WITH x AS (SELECT 1) SELECT * FROM x")
    assert verdict == "read"


def test_classify_with_insert_as_write() -> None:
    verdict, verb = _classify_statement("WITH x AS (INSERT INTO users VALUES (1)) SELECT * FROM x")
    assert verdict == "write"
    assert verb == "INSERT"


def test_classify_unknown_for_garbage() -> None:
    verdict, _ = _classify_statement("FOOBAR a b c")
    assert verdict == "unknown"


def test_classify_handles_block_and_line_comments() -> None:
    verdict, _ = _classify_statement("/* hi */ -- bye\n  SELECT 1")
    assert verdict == "read"


def test_referenced_tables_picks_from_clause() -> None:
    assert _referenced_tables("SELECT * FROM users WHERE id = 1") == {"users"}


def test_referenced_tables_handles_joins() -> None:
    tables = _referenced_tables(
        "SELECT u.name FROM users u JOIN secrets s ON u.id = s.user_id"
    )
    assert {"users", "secrets"}.issubset(tables)


def test_referenced_tables_handles_insert_into() -> None:
    assert _referenced_tables("INSERT INTO users (name) VALUES ('alice')") == {"users"}


def test_referenced_tables_ignores_string_literals() -> None:
    # A column value that happens to match a table name must not trip.
    assert _referenced_tables("SELECT * FROM users WHERE name = 'secrets'") == {"users"}


def test_referenced_tables_handles_quoted_identifiers() -> None:
    assert _referenced_tables('SELECT * FROM "Users"') == {"users"}


# ---------------------------------------------------------------------------
# Dispose is idempotent
# ---------------------------------------------------------------------------


def test_dispose_is_idempotent(engine) -> None:
    tool = _make_tool(engine)
    # Force a connection first.
    tool.invoke(ToolContext(), {"sql": "SELECT 1"})
    assert tool._engine is not None
    tool.dispose()
    assert tool._engine is None
    tool.dispose()  # should not raise


# ---------------------------------------------------------------------------
# Lazy engine creation
# ---------------------------------------------------------------------------


def test_engine_created_on_first_invoke(engine) -> None:
    tool = _make_tool(engine)
    assert tool._engine is None
    tool.invoke(ToolContext(), {"sql": "SELECT 1"})
    assert tool._engine is not None


def test_repeated_invocations_reuse_engine(engine) -> None:
    tool = _make_tool(engine)
    tool.invoke(ToolContext(), {"sql": "SELECT 1"})
    first = tool._engine
    tool.invoke(ToolContext(), {"sql": "SELECT 2"})
    assert tool._engine is first


# ---------------------------------------------------------------------------
# ToolResult serialization compatibility with registry
# ---------------------------------------------------------------------------


def test_toolresult_serializes_through_registry(engine) -> None:
    """Smoke test: wire the tool through ToolRegistry.call and ensure
    the executor / registry boundary works."""
    from hnsx_worker.tools import ToolRegistry

    reg = ToolRegistry()
    reg.register(_make_tool(engine))
    result = reg.call("lookup", ToolContext(), {"sql": "SELECT COUNT(*) AS n FROM users"})
    assert result.ok
    assert result.output["rows"] == [{"n": 2}]
