"""HTTP tool — GET / POST / PUT / DELETE / PATCH with secret injection.

This is one of the **API Agent** tools (W3.2). CLI agents (Claude Code /
Codex) bring their own primitives — the Tool layer only constrains and
audits them.

The tool's runtime configuration lives in the DomainSpec under
``agent.tools[].config``. The LLM sees a slim JSON schema (path_params /
query / headers / body overrides); the tool merges those with the static
config from the spec, resolves ``{secret.XXX}`` placeholders against
``ToolContext.secrets``, and dispatches via :mod:`httpx`.

Spec entry shape (in ``harness.agents.<name>.tools[]``)::

    - name: fetch_user           # what the LLM calls
      type: http                 # routes to HttpTool
      config:
        method: GET
        url: "https://api.example.com/users/{id}"
        headers:
          Authorization: "Bearer {secret.api_key}"
        query:
          locale: en
        timeout_seconds: 30
        retry:
          max_attempts: 3
          backoff_seconds: 0.5
          retry_on_status: [429, 503]
        status_whitelist: [200, 201, 204]
        max_response_bytes: 65536

The LLM-facing schema exposes only the override-able bits; the URL
template / static headers / status whitelist all come from the spec.

Audit / safety:

  - ``{secret.XXX}`` placeholders are resolved *before* the call; the
    resolved URL / headers / body never appear in any observation.
  - Response bodies are truncated to ``max_response_bytes`` (default 64 KB)
    so a runaway endpoint can't blow up the observation stream.
  - The ``tool_call`` observation records ``input_keys`` (W3.1) — never the
    resolved values — so audit logs stay safe to ship.
"""

from __future__ import annotations

import json
import logging
import re
import time
from collections.abc import Mapping
from dataclasses import dataclass, field
from typing import Any

import httpx

from .base import Tool, ToolContext, ToolResult

log = logging.getLogger("hnsx_worker.tools.http")

_ALLOWED_METHODS = frozenset({"GET", "POST", "PUT", "DELETE", "PATCH"})

# ``{secret.foo}`` and ``{path_param}`` placeholders. The first character
# after ``{`` disambiguates: ``secret.`` is a secret reference; anything
# else is a path parameter that must come from the LLM-provided input.
_PLACEHOLDER_RE = re.compile(r"\{([^{}]+)\}")

_DEFAULT_MAX_RESPONSE_BYTES = 65536
_DEFAULT_TIMEOUT_SECONDS = 30.0
_DEFAULT_RETRY_BACKOFF = 0.5


@dataclass
class HttpToolConfig:
    """Static configuration for one HttpTool instance.

    Lives next to the tool so executor + tests can introspect what the
    spec said without re-parsing the dict every call.
    """

    method: str = "GET"
    url: str = ""
    headers: dict[str, str] = field(default_factory=dict)
    query: dict[str, str] = field(default_factory=dict)
    body: dict[str, Any] | None = None  # JSON body template (optional)
    timeout_seconds: float = _DEFAULT_TIMEOUT_SECONDS
    retry: dict[str, Any] = field(default_factory=dict)
    status_whitelist: list[int] = field(default_factory=list)
    max_response_bytes: int = _DEFAULT_MAX_RESPONSE_BYTES

    @classmethod
    def from_spec(cls, raw: Mapping[str, Any]) -> HttpToolConfig:
        method = str(raw.get("method", "GET")).upper()
        if method not in _ALLOWED_METHODS:
            raise ValueError(
                f"http tool: method {method!r} not allowed "
                f"(must be one of {sorted(_ALLOWED_METHODS)})"
            )
        url = str(raw.get("url", ""))
        if not url:
            raise ValueError("http tool: config.url is required")
        timeout = float(raw.get("timeout_seconds", _DEFAULT_TIMEOUT_SECONDS))
        if timeout <= 0:
            raise ValueError("http tool: timeout_seconds must be > 0")
        max_bytes = int(raw.get("max_response_bytes", _DEFAULT_MAX_RESPONSE_BYTES))
        if max_bytes < 0:
            raise ValueError("http tool: max_response_bytes must be >= 0")
        # ``body`` may come in as either a raw template (dict / list / scalar)
        # or wrapped in ``{type, template}`` for future content-type dispatch.
        # We unwrap here so the rest of the code only ever sees the template.
        raw_body = raw.get("body")
        if isinstance(raw_body, dict) and "template" in raw_body and "type" in raw_body:
            body: Any | None = raw_body["template"]
        else:
            body = raw_body
        return cls(
            method=method,
            url=url,
            headers={str(k): str(v) for k, v in (raw.get("headers") or {}).items()},
            query={str(k): str(v) for k, v in (raw.get("query") or {}).items()},
            body=body,
            timeout_seconds=timeout,
            retry=dict(raw.get("retry") or {}),
            status_whitelist=[int(s) for s in (raw.get("status_whitelist") or [])],
            max_response_bytes=max_bytes,
        )


class HttpTool(Tool):
    """One configured HTTP endpoint, callable by the LLM by ``name``.

    The instance is constructed by :func:`hnsx_worker.tools.factory.build_tool`
    from a spec entry like ``{name, type: 'http', config: {...}}``; the
    factory passes the parsed :class:`HttpToolConfig` here.
    """

    def __init__(self, name: str, config: HttpToolConfig) -> None:
        self._name = name
        self._config = config

    @property
    def name(self) -> str:
        return self._name

    @property
    def config(self) -> HttpToolConfig:
        return self._config

    @property
    def schema(self) -> dict[str, Any]:
        """LLM-facing input schema.

        Only exposes the override-able surface (path params / extra query /
        extra headers / extra body). Static URL / status whitelist / retry
        policy live in the spec and are not negotiable at call time.
        """
        props: dict[str, Any] = {
            "path_params": {
                "type": "object",
                "description": (
                    "Values substituted into {placeholders} in the URL template. "
                    "Keys must match the placeholder names declared in the URL."
                ),
            },
        }
        if self._config.method in {"POST", "PUT", "PATCH"}:
            props["body"] = {
                "type": "object",
                "description": "JSON body to send (merged with the static body template).",
            }
        return {
            "type": "object",
            "properties": props,
            "additionalProperties": False,
        }

    # ------------------------------------------------------------------ invoke

    def invoke(self, ctx: ToolContext, input: dict[str, Any]) -> ToolResult:
        if not isinstance(input, dict):
            return ToolResult(error="input must be a JSON object")

        path_params = input.get("path_params") or {}
        if not isinstance(path_params, dict):
            return ToolResult(error="path_params must be an object")

        try:
            url = _resolve_placeholders(self._config.url, ctx, path_params)
        except _SecretMissingError as e:
            return ToolResult(error=str(e))

        headers: dict[str, str] = {}
        for k, v in self._config.headers.items():
            try:
                headers[k] = _resolve_placeholders(v, ctx, path_params)
            except _SecretMissingError as e:
                return ToolResult(error=str(e))

        query: dict[str, str] = dict(self._config.query)
        llm_query = input.get("query") or {}
        if llm_query:
            if not isinstance(llm_query, dict):
                return ToolResult(error="query must be an object")
            for k, v in llm_query.items():
                try:
                    query[str(k)] = _resolve_placeholders(str(v), ctx, path_params)
                except _SecretMissingError as e:
                    return ToolResult(error=str(e))

        llm_headers = input.get("headers") or {}
        if llm_headers:
            if not isinstance(llm_headers, dict):
                return ToolResult(error="headers must be an object")
            for k, v in llm_headers.items():
                try:
                    headers[str(k)] = _resolve_placeholders(str(v), ctx, path_params)
                except _SecretMissingError as e:
                    return ToolResult(error=str(e))

        body: Any | None = None
        if self._config.method in {"POST", "PUT", "PATCH"}:
            body = _merge_body(self._config.body, input.get("body"), ctx, path_params)

        try:
            return _do_request(
                method=self._config.method,
                url=url,
                headers=headers,
                params=query,
                body=body,
                config=self._config,
            )
        except httpx.HTTPError as e:
            return ToolResult(
                error=f"http error: {e!s}",
                metadata={
                    "url": _safe_url(url),
                    "method": self._config.method,
                    "attempts": 1,
                },
            )


# ---------------------------------------------------------------------------
# Request execution
# ---------------------------------------------------------------------------


def _do_request(
    *,
    method: str,
    url: str,
    headers: dict[str, str],
    params: dict[str, str],
    body: Any | None,
    config: HttpToolConfig,
) -> ToolResult:
    """Execute the request with retries; return a ToolResult.

    Non-allowed status codes are surfaced as ``ToolResult(error=...)`` —
    never raised — so the agent sees the failure and can adapt.
    """
    max_attempts = max(1, int(config.retry.get("max_attempts", 1)))
    backoff = float(config.retry.get("backoff_seconds", _DEFAULT_RETRY_BACKOFF))
    retry_on_status = set(int(s) for s in (config.retry.get("retry_on_status") or []))

    last_status: int | None = None
    last_error: str | None = None
    last_body_preview: Any = ""

    for attempt in range(1, max_attempts + 1):
        started = time.monotonic()
        try:
            with httpx.Client(timeout=config.timeout_seconds) as client:
                request_kwargs: dict[str, Any] = {
                    "method": method,
                    "url": url,
                    "headers": headers,
                    "params": params,
                }
                if body is not None and method in {"POST", "PUT", "PATCH"}:
                    request_kwargs["json"] = body

                with client.stream(**request_kwargs) as resp:
                    buf = bytearray()
                    for chunk in resp.iter_bytes():
                        if len(buf) >= config.max_response_bytes:
                            if chunk:
                                buf.extend(chunk[:1])
                            break
                        buf.extend(chunk)
                    elapsed_ms = int((time.monotonic() - started) * 1000)
                    status = int(resp.status_code)
                    last_status = status

                    content_type = resp.headers.get("content-type", "")
                    raw = bytes(buf)
                    truncated = len(raw) > config.max_response_bytes
                    if truncated:
                        raw = raw[: config.max_response_bytes]
                    decoded = _decode_body(raw, content_type)
                    if isinstance(decoded, str):
                        last_body_preview = decoded[:512]
                    else:
                        last_body_preview = decoded

                    # Retry on configured statuses (independent of whitelist).
                    if status in retry_on_status:
                        if attempt < max_attempts:
                            time.sleep(backoff * (2 ** (attempt - 1)))
                            continue
                        # Exhausted retries on a transient status — fail.
                        return _fail(
                            url=url,
                            method=method,
                            attempts=attempt,
                            elapsed_ms=elapsed_ms,
                            error=(
                                f"http: exhausted {attempt} attempts "
                                f"on transient status {status}"
                            ),
                            status=status,
                            body_preview=last_body_preview,
                        )

                    # Status whitelist: empty == accept any; otherwise enforce.
                    if config.status_whitelist and status not in config.status_whitelist:
                        return _fail(
                            url=url,
                            method=method,
                            attempts=attempt,
                            elapsed_ms=elapsed_ms,
                            error=(
                                f"http status {status} not in whitelist "
                                f"{config.status_whitelist}"
                            ),
                            status=status,
                            body_preview=last_body_preview,
                        )

                    return ToolResult(
                        output={
                            "status": status,
                            "headers": _safe_response_headers(resp.headers),
                            "body": decoded,
                        },
                        metadata={
                            "url": _safe_url(url),
                            "method": method,
                            "elapsed_ms": elapsed_ms,
                            "bytes": len(raw),
                            "truncated": truncated,
                            "attempts": attempt,
                        },
                    )
        except httpx.HTTPError as e:
            last_error = f"http error: {e!s}"
            if attempt < max_attempts:
                time.sleep(backoff * (2 ** (attempt - 1)))
                continue
            return ToolResult(
                error=last_error,
                metadata={
                    "url": _safe_url(url),
                    "method": method,
                    "attempts": attempt,
                },
            )

    # Defensive: should be unreachable since the loop returns on success.
    if last_status is not None:
        return _fail(
            url=url,
            method=method,
            attempts=max_attempts,
            elapsed_ms=0,
            error=f"http: gave up after {max_attempts} attempts (last status={last_status})",
            status=last_status,
            body_preview=last_body_preview,
        )
    return ToolResult(
        error=last_error or f"http: gave up after {max_attempts} attempts",
        metadata={
            "url": _safe_url(url),
            "method": method,
            "attempts": max_attempts,
        },
    )


def _fail(
    *,
    url: str,
    method: str,
    attempts: int,
    elapsed_ms: int,
    error: str,
    status: int,
    body_preview: Any,
) -> ToolResult:
    """Build a structured failure ToolResult with full audit metadata."""
    preview = body_preview if not isinstance(body_preview, str) else body_preview[:256]
    return ToolResult(
        error=error,
        metadata={
            "url": _safe_url(url),
            "method": method,
            "attempts": attempts,
            "elapsed_ms": elapsed_ms,
            "status": status,
            "body_preview": preview,
        },
    )


def _decode_body(raw: bytes, content_type: str) -> Any:
    """Decode the response body. JSON if the content-type says so; else utf-8."""
    if not raw:
        return ""
    ct = content_type.lower()
    if "json" in ct:
        try:
            return json.loads(raw.decode("utf-8", errors="replace"))
        except (ValueError, UnicodeDecodeError):
            return raw.decode("utf-8", errors="replace")
    try:
        return raw.decode("utf-8", errors="replace")
    except Exception:  # noqa: BLE001
        return raw.hex()


def _safe_response_headers(headers: httpx.Headers) -> dict[str, str]:
    """Drop hop-by-hop / secret-leaking headers from the response."""
    blocked = {"authorization", "set-cookie", "cookie", "x-api-key", "x-auth-token"}
    return {
        k: v
        for k, v in headers.items()
        if k.lower() not in blocked and not k.lower().startswith("x-secret")
    }


def _safe_url(url: str) -> str:
    """Strip query string for audit display — secrets may have ended up there."""
    q = url.find("?")
    return url if q < 0 else url[:q]


# ---------------------------------------------------------------------------
# Placeholder / secret resolution
# ---------------------------------------------------------------------------


class _SecretMissingError(Exception):
    """Raised when a {secret.XXX} placeholder can't be resolved."""


def _resolve_placeholders(
    template: str,
    ctx: ToolContext,
    path_params: Mapping[str, Any],
) -> str:
    """Resolve ``{secret.X}`` / ``{X}`` placeholders inside ``template``.

    Raises :class:`_SecretMissingError` if a secret reference can't be
    resolved — the caller converts that into a structured ToolResult
    so the agent can adapt instead of crashing.
    """

    def repl(match: re.Match[str]) -> str:
        key = match.group(1).strip()
        if key.startswith("secret."):
            name = key[len("secret.") :].strip()
            if name not in ctx.secrets:
                raise _SecretMissingError(f"missing secret {name!r}")
            return ctx.secrets[name]
        if key in path_params:
            return str(path_params[key])
        # Unknown placeholder → leave as-is so debugging is easier. This is
        # intentionally not an error: the URL may have literal braces.
        return match.group(0)

    return _PLACEHOLDER_RE.sub(repl, template)


def _merge_body(
    static: Any | None,
    override: Any | None,
    ctx: ToolContext,
    path_params: Mapping[str, Any],
) -> Any | None:
    """Merge a static body template with LLM-provided overrides.

    Both sides may carry ``{secret.X}`` placeholders. Both are resolved.
    Override keys win on conflict.
    """
    if static is None and override is None:
        return None
    if static is None:
        return _walk_resolve(override, ctx, path_params)
    if override is None:
        return _walk_resolve(static, ctx, path_params)
    if isinstance(static, dict) and isinstance(override, dict):
        merged = dict(static)
        for k, v in override.items():
            if isinstance(v, dict) and isinstance(merged.get(k), dict):
                merged[k] = {**merged[k], **v}
            else:
                merged[k] = v
        return _walk_resolve(merged, ctx, path_params)
    # Non-dict: override wins.
    return _walk_resolve(override, ctx, path_params)


def _walk_resolve(value: Any, ctx: ToolContext, path_params: Mapping[str, Any]) -> Any:
    if isinstance(value, str):
        try:
            return _resolve_placeholders(value, ctx, path_params)
        except _SecretMissingError:
            raise
    if isinstance(value, dict):
        return {k: _walk_resolve(v, ctx, path_params) for k, v in value.items()}
    if isinstance(value, list):
        return [_walk_resolve(v, ctx, path_params) for v in value]
    return value


__all__ = ["HttpTool", "HttpToolConfig"]
