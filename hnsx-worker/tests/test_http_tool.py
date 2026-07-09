"""Tests for the HTTP tool (W3.2).

Coverage:

  - Config parsing + validation (method / url / timeout / max bytes).
  - LLM-facing schema reflects method (body only for POST/PUT/PATCH).
  - GET happy path with status whitelist.
  - Status not in whitelist → structured error.
  - Status in retry_on_status → retries, then succeeds.
  - Status not in retry_on_status → no retry, immediate failure.
  - ``{secret.X}`` resolved from ToolContext.secrets.
  - ``{path_param}`` resolved from input.path_params.
  - Missing secret → structured error, no request issued.
  - max_response_bytes truncation.
  - Body merging for POST/PUT.
  - _safe_url strips query string.
"""

from __future__ import annotations

import json
from typing import Any

import httpx
import pytest

from hnsx_worker.tools import ToolContext
from hnsx_worker.tools.http import HttpTool, HttpToolConfig

# ---------------------------------------------------------------------------
# Test infrastructure
# ---------------------------------------------------------------------------


def _make_tool(
    name: str = "fetch_user",
    config: dict[str, Any] | None = None,
) -> HttpTool:
    raw = {
        "method": "GET",
        "url": "https://api.example.com/users/{id}",
        "headers": {"Authorization": "Bearer {secret.api_key}"},
        "query": {"locale": "en"},
        "timeout_seconds": 5,
        "max_response_bytes": 1024,
        **({"retry": {}, "status_whitelist": [200]} | (config or {})),
    }
    return HttpTool(name, HttpToolConfig.from_spec(raw))


def _patch_transport(
    monkeypatch: pytest.MonkeyPatch,
    handler,
) -> list[httpx.Request]:
    """Install an httpx.MockTransport and return a list that captures requests."""
    captured: list[httpx.Request] = []

    def _send(request: httpx.Request) -> httpx.Response:
        captured.append(request)
        return handler(request)

    transport = httpx.MockTransport(_send)
    # httpx.Client picks up a ``transport`` kwarg; we patch the module-level
    # Client to always use ours so retry loops share the same captured list.
    real_init = httpx.Client.__init__

    def _init(self: httpx.Client, *args: Any, **kwargs: Any) -> None:
        kwargs["transport"] = transport
        real_init(self, *args, **kwargs)

    monkeypatch.setattr(httpx.Client, "__init__", _init)
    return captured


def _ok_response(body: Any = None, status: int = 200) -> httpx.Response:
    if body is None:
        body = {"ok": True}
    return httpx.Response(
        status,
        json=body if not isinstance(body, (str, bytes)) else None,
        content=body if isinstance(body, (str, bytes)) else None,
        headers={"content-type": "application/json"},
    )


# ---------------------------------------------------------------------------
# Config parsing
# ---------------------------------------------------------------------------


def test_config_requires_url() -> None:
    with pytest.raises(ValueError, match="url is required"):
        HttpToolConfig.from_spec({"method": "GET"})


def test_config_rejects_unknown_method() -> None:
    with pytest.raises(ValueError, match="not allowed"):
        HttpToolConfig.from_spec({"method": "TRACE", "url": "https://x"})


def test_config_rejects_non_positive_timeout() -> None:
    with pytest.raises(ValueError, match="timeout_seconds"):
        HttpToolConfig.from_spec({"url": "https://x", "timeout_seconds": 0})


def test_config_parses_all_fields() -> None:
    cfg = HttpToolConfig.from_spec(
        {
            "method": "post",
            "url": "https://x/{id}",
            "headers": {"X-Source": "hnsx"},
            "query": {"v": "1"},
            "body": {"type": "json", "template": {"a": "{a}"}},
            "timeout_seconds": 12,
            "retry": {"max_attempts": 3},
            "status_whitelist": [200, 201],
            "max_response_bytes": 128,
        }
    )
    assert cfg.method == "POST"
    assert cfg.url == "https://x/{id}"
    assert cfg.headers == {"X-Source": "hnsx"}
    assert cfg.query == {"v": "1"}
    assert cfg.timeout_seconds == 12
    assert cfg.retry == {"max_attempts": 3}
    assert cfg.status_whitelist == [200, 201]
    assert cfg.max_response_bytes == 128


# ---------------------------------------------------------------------------
# Schema exposure
# ---------------------------------------------------------------------------


def test_get_schema_exposes_path_params_only() -> None:
    tool = _make_tool()
    schema = tool.schema
    assert schema["properties"]["path_params"]["type"] == "object"
    assert "body" not in schema["properties"]


def test_post_schema_exposes_body() -> None:
    raw = {
        "method": "POST",
        "url": "https://x/{id}",
        "headers": {},
        "timeout_seconds": 5,
    }
    tool = HttpTool("create", HttpToolConfig.from_spec(raw))
    assert "body" in tool.schema["properties"]


def test_tool_name_is_specified_name_not_method() -> None:
    tool = _make_tool(name="my_get")
    assert tool.name == "my_get"


# ---------------------------------------------------------------------------
# Happy path
# ---------------------------------------------------------------------------


def test_get_with_path_params_and_secret(monkeypatch: pytest.MonkeyPatch) -> None:
    captured = _patch_transport(monkeypatch, lambda _: _ok_response({"id": 7}))
    tool = _make_tool()
    ctx = ToolContext(session_id="s", secrets={"api_key": "sk-abc"})
    result = tool.invoke(ctx, {"path_params": {"id": "7"}})

    assert result.ok, result.error
    assert result.output["status"] == 200
    assert result.output["body"] == {"id": 7}
    assert result.metadata["url"] == "https://api.example.com/users/7"
    assert result.metadata["method"] == "GET"
    assert result.metadata["attempts"] == 1

    # Request inspection.
    assert len(captured) == 1
    req = captured[0]
    assert req.headers.get("authorization") == "Bearer sk-abc"
    assert req.url.params["locale"] == "en"
    assert req.url.path == "/users/7"


def test_status_whitelist_accepts_allowed_status(monkeypatch: pytest.MonkeyPatch) -> None:
    captured = _patch_transport(monkeypatch, lambda _: _ok_response(status=204))
    cfg = {"status_whitelist": [200, 204]}
    spec = {"method": "DELETE", "url": "https://x/{id}", **cfg}
    tool = HttpTool("noop", HttpToolConfig.from_spec(spec))
    result = tool.invoke(ToolContext(), {"path_params": {"id": "1"}})
    assert result.ok
    assert result.output["status"] == 204
    assert len(captured) == 1


# ---------------------------------------------------------------------------
# Failure paths
# ---------------------------------------------------------------------------


def test_status_not_in_whitelist_returns_structured_error(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    captured = _patch_transport(monkeypatch, lambda _: _ok_response(status=500))
    tool = _make_tool()  # status_whitelist=[200]
    result = tool.invoke(ToolContext(secrets={"api_key": "k"}), {"path_params": {"id": "x"}})

    assert not result.ok
    assert "500" in (result.error or "")
    assert result.metadata["status"] == 500
    # No retry attempted (retry_on_status empty).
    assert len(captured) == 1


def test_status_in_retry_on_status_retries_then_succeeds(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    call_count = {"n": 0}

    def handler(_: httpx.Request) -> httpx.Response:
        call_count["n"] += 1
        if call_count["n"] < 3:
            return _ok_response(status=503)
        return _ok_response(status=200)

    captured = _patch_transport(monkeypatch, handler)
    cfg = {"retry": {"max_attempts": 3, "backoff_seconds": 0, "retry_on_status": [503]}}
    tool = HttpTool("flaky", HttpToolConfig.from_spec({"method": "GET", "url": "https://x", **cfg}))
    result = tool.invoke(ToolContext(), {})

    assert result.ok
    assert call_count["n"] == 3
    assert result.metadata["attempts"] == 3
    assert len(captured) == 3


def test_status_in_retry_on_status_exhausts_attempts(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    captured = _patch_transport(monkeypatch, lambda _: _ok_response(status=503))
    cfg = {"retry": {"max_attempts": 2, "backoff_seconds": 0, "retry_on_status": [503]}}
    tool = HttpTool("flaky", HttpToolConfig.from_spec({"method": "GET", "url": "https://x", **cfg}))
    result = tool.invoke(ToolContext(), {})

    assert not result.ok
    assert "503" in (result.error or "")
    assert result.metadata["attempts"] == 2
    assert result.metadata["status"] == 503
    assert len(captured) == 2


def test_missing_secret_returns_error_no_request(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    captured = _patch_transport(monkeypatch, lambda _: _ok_response())
    tool = _make_tool()  # needs secret "api_key"
    result = tool.invoke(ToolContext(secrets={}), {"path_params": {"id": "1"}})

    assert not result.ok
    assert "missing secret" in (result.error or "")
    assert "api_key" in (result.error or "")
    assert len(captured) == 0  # No request issued.


def test_network_error_returns_structured_error(monkeypatch: pytest.MonkeyPatch) -> None:
    def handler(_: httpx.Request) -> httpx.Response:
        raise httpx.ConnectError("nope", request=httpx.Request("GET", "https://x"))

    _patch_transport(monkeypatch, handler)
    tool = _make_tool()
    result = tool.invoke(ToolContext(secrets={"api_key": "k"}), {"path_params": {"id": "1"}})

    assert not result.ok
    assert "http error" in (result.error or "")
    assert result.metadata["attempts"] == 1


def test_unknown_placeholder_is_left_alone(monkeypatch: pytest.MonkeyPatch) -> None:
    """URLs with literal braces that aren't placeholders should not crash."""
    captured = _patch_transport(monkeypatch, lambda _: _ok_response())
    raw = {
        "method": "GET",
        "url": "https://api.example.com/v1/{literal}",  # not a path param
        "headers": {},
        "timeout_seconds": 5,
    }
    tool = HttpTool("odd", HttpToolConfig.from_spec(raw))
    result = tool.invoke(ToolContext(), {})
    assert result.ok
    # URL is passed through, brace literal preserved.
    assert captured[0].url.path == "/v1/{literal}"


# ---------------------------------------------------------------------------
# Truncation
# ---------------------------------------------------------------------------


def test_response_truncated_when_exceeds_max_bytes(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    big = "x" * 4096

    def handler(_: httpx.Request) -> httpx.Response:
        return httpx.Response(200, content=big, headers={"content-type": "text/plain"})

    _patch_transport(monkeypatch, handler)
    cfg = {"max_response_bytes": 128}
    raw = {"method": "GET", "url": "https://x", "headers": {}, "timeout_seconds": 5, **cfg}
    tool = HttpTool("big", HttpToolConfig.from_spec(raw))
    result = tool.invoke(ToolContext(), {})

    assert result.ok
    assert result.metadata["truncated"] is True
    assert result.metadata["bytes"] <= 129  # +1 detection byte
    assert len(result.output["body"]) <= 128


# ---------------------------------------------------------------------------
# POST / body merging
# ---------------------------------------------------------------------------


def test_post_merges_static_body_with_override(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    captured = _patch_transport(monkeypatch, lambda _: _ok_response({"created": True}))
    raw = {
        "method": "POST",
        "url": "https://api.example.com/users",
        "headers": {"Authorization": "Bearer {secret.api_key}"},
        "body": {"type": "json", "template": {"role": "admin", "active": True}},
        "timeout_seconds": 5,
    }
    tool = HttpTool("create_user", HttpToolConfig.from_spec(raw))
    ctx = ToolContext(secrets={"api_key": "sk-abc"})
    result = tool.invoke(
        ctx,
        {
            "body": {
                "name": "alice",  # override-only key
                "active": False,  # override of static key
            }
        },
    )

    assert result.ok, result.error
    sent = json.loads(captured[0].content.decode("utf-8"))
    assert sent == {"role": "admin", "active": False, "name": "alice"}


def test_post_with_secret_in_body(monkeypatch: pytest.MonkeyPatch) -> None:
    captured = _patch_transport(monkeypatch, lambda _: _ok_response())
    raw = {
        "method": "POST",
        "url": "https://x",
        "headers": {},
        "body": {"type": "json", "template": {"token": "{secret.t}"}},
        "timeout_seconds": 5,
    }
    tool = HttpTool("p", HttpToolConfig.from_spec(raw))
    result = tool.invoke(ToolContext(secrets={"t": "real-token"}), {})
    assert result.ok
    sent = json.loads(captured[0].content.decode("utf-8"))
    assert sent == {"token": "real-token"}


def test_post_resolves_secret_in_body_override(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    captured = _patch_transport(monkeypatch, lambda _: _ok_response())
    raw = {
        "method": "POST",
        "url": "https://x",
        "headers": {},
        "timeout_seconds": 5,
    }
    tool = HttpTool("p", HttpToolConfig.from_spec(raw))
    result = tool.invoke(
        ToolContext(secrets={"my_token": "shh"}),
        {"body": {"auth": "Bearer {secret.my_token}"}},
    )
    assert result.ok
    assert json.loads(captured[0].content.decode("utf-8")) == {"auth": "Bearer shh"}


# ---------------------------------------------------------------------------
# LLM override surface
# ---------------------------------------------------------------------------


def test_llm_query_and_headers_override_static(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    captured = _patch_transport(monkeypatch, lambda _: _ok_response())
    raw = {
        "method": "GET",
        "url": "https://api.example.com/{id}",
        "headers": {"X-Source": "hnsx", "X-Trace": "static"},
        "query": {"locale": "en"},
        "timeout_seconds": 5,
    }
    tool = HttpTool("ovr", HttpToolConfig.from_spec(raw))
    ctx = ToolContext(secrets={"api_key": "k"})
    result = tool.invoke(
        ctx,
        {
            "path_params": {"id": "7"},
            "query": {"locale": "ja", "extra": "1"},
            "headers": {"X-Trace": "dynamic"},
        },
    )
    assert result.ok
    req = captured[0]
    assert req.url.params["locale"] == "ja"
    assert req.url.params["extra"] == "1"
    assert req.headers.get("x-trace") == "dynamic"
    assert req.headers.get("x-source") == "hnsx"


def test_unknown_placeholder_inside_body_is_left_alone() -> None:
    """Recursive walker should pass non-string scalars through unchanged."""
    from hnsx_worker.tools.http import _walk_resolve

    ctx = ToolContext(secrets={})
    out = _walk_resolve({"a": 1, "b": [2, 3], "c": {"d": "x"}}, ctx, {})
    assert out == {"a": 1, "b": [2, 3], "c": {"d": "x"}}


# ---------------------------------------------------------------------------
# Input validation
# ---------------------------------------------------------------------------


def test_input_must_be_dict() -> None:
    tool = _make_tool()
    result = tool.invoke(ToolContext(), "not a dict")  # type: ignore[arg-type]
    assert not result.ok
    assert "JSON object" in (result.error or "")


def test_path_params_must_be_dict() -> None:
    tool = _make_tool()
    result = tool.invoke(ToolContext(), {"path_params": "bad"})
    assert not result.ok
    assert "path_params" in (result.error or "")


def test_safe_url_strips_query() -> None:
    from hnsx_worker.tools.http import _safe_url

    assert _safe_url("https://x.example/foo?secret=hunter2") == "https://x.example/foo"
    assert _safe_url("https://x.example/foo") == "https://x.example/foo"


def test_response_headers_filter_secrets() -> None:
    from hnsx_worker.tools.http import _safe_response_headers

    headers = httpx.Headers(
        {
            "content-type": "application/json",
            "authorization": "Bearer leak-me",
            "set-cookie": "session=leak-me",
            "x-api-key": "leak-me",
            "x-secret-thing": "leak-me",
        }
    )
    safe = _safe_response_headers(headers)
    assert safe == {"content-type": "application/json"}
