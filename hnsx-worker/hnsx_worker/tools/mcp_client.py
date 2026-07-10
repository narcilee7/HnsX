"""MCP client tool — call external tools through the Model Context Protocol.

W10 introduces a built-in ``mcp_client`` tool so agents can reuse any MCP
server (filesystem, web search, database, etc.) without the worker re-
implementing the capability. The worker acts as an MCP client; the remote
process / HTTP endpoint is the server.

Supported transports:

  - **stdio**: spawn a local command (e.g. ``npx -y @modelcontextprotocol/server-filesystem /tmp``).
    The worker talks JSON-RPC 2.0 over the child process stdin/stdout.
  - **sse**: connect to an HTTP SSE endpoint that exposes the MCP protocol.
    The worker receives server-sent events for responses and POSTs requests
    to the endpoint advertised by the ``endpoint`` event.

Spec shapes
-----------

Domain-level MCP server declarations (optional, recommended)::

    harness:
      mcp_servers:
        - name: filesystem
          transport: stdio
          command: ["npx", "-y", "@modelcontextprotocol/server-filesystem", "/tmp"]
          env:
            NODE_ENV: production
          timeout_seconds: 30

        - name: remote-tools
          transport: sse
          url: http://localhost:3001/sse
          timeout_seconds: 30

Agent-level tool declaration (references a domain-level server)::

    harness:
      agents:
        my_agent:
          tools:
            - name: list_directory
              type: mcp_client
              description: List files in a directory via MCP
              config:
                server: filesystem
                tool: list_directory   # remote tool name; defaults to tool name

Or inline (no domain-level server)::

    - name: list_directory
      type: mcp_client
      config:
        transport: stdio
        command: ["python", "-m", "my_mcp_server"]
        tool: list_directory

Discovery and schema
--------------------

During tool-registry construction the worker connects to each referenced MCP
server once, lists its tools, and caches the remote tool definitions. The
LLM-facing schema for an ``mcp_client`` tool is the remote tool's input
schema, so the adapter can call it like any other built-in tool.

Policy / audit
--------------

Every MCP call goes through the normal ``ToolRegistry`` policy gate and is
recorded as a ``tool_result`` observation. The worker also emits an
``mcp_call`` observation with server / transport / latency metadata so the
Harness UI can show external tool usage.
"""

from __future__ import annotations

import json
import logging
import os
import queue
import subprocess
import threading
import time
import uuid
from collections.abc import Mapping
from dataclasses import dataclass, field
from typing import Any

import httpx

from .base import Tool, ToolContext, ToolResult

log = logging.getLogger("hnsx_worker.tools.mcp_client")

_MCP_PROTOCOL_VERSION = "2024-11-05"
_DEFAULT_TIMEOUT_SECONDS = 30.0
_DEFAULT_MAX_RESPONSE_BYTES = 262144  # 256 KB


# ---------------------------------------------------------------------------
# Server configuration
# ---------------------------------------------------------------------------


@dataclass
class McpServerConfig:
    """Static configuration for one MCP server endpoint."""

    name: str = ""
    transport: str = "stdio"  # "stdio" | "sse"
    command: list[str] = field(default_factory=list)
    url: str = ""
    env: dict[str, str] = field(default_factory=dict)
    timeout_seconds: float = _DEFAULT_TIMEOUT_SECONDS
    max_response_bytes: int = _DEFAULT_MAX_RESPONSE_BYTES

    @classmethod
    def from_spec(cls, raw: Mapping[str, Any]) -> McpServerConfig:
        if not isinstance(raw, dict):
            raise ValueError("mcp server config must be a dict")

        transport = str(raw.get("transport", "stdio")).lower()
        if transport not in {"stdio", "sse"}:
            raise ValueError(f"mcp server: transport {transport!r} not supported")

        timeout = float(raw.get("timeout_seconds", _DEFAULT_TIMEOUT_SECONDS))
        if timeout <= 0:
            raise ValueError("mcp server: timeout_seconds must be > 0")

        max_bytes = int(raw.get("max_response_bytes", _DEFAULT_MAX_RESPONSE_BYTES))
        if max_bytes < 0:
            raise ValueError("mcp server: max_response_bytes must be >= 0")

        if transport == "stdio":
            command = raw.get("command")
            if not isinstance(command, list) or not command:
                raise ValueError("mcp server (stdio): command must be a non-empty list")
            return cls(
                name=str(raw.get("name", "")),
                transport=transport,
                command=[str(c) for c in command],
                env={str(k): str(v) for k, v in (raw.get("env") or {}).items()},
                timeout_seconds=timeout,
                max_response_bytes=max_bytes,
            )

        # transport == "sse"
        url = str(raw.get("url", ""))
        if not url:
            raise ValueError("mcp server (sse): url is required")
        return cls(
            name=str(raw.get("name", "")),
            transport=transport,
            url=url,
            env={str(k): str(v) for k, v in (raw.get("env") or {}).items()},
            timeout_seconds=timeout,
            max_response_bytes=max_bytes,
        )


# ---------------------------------------------------------------------------
# Tool configuration
# ---------------------------------------------------------------------------


@dataclass
class McpToolConfig:
    """Static configuration for one ``mcp_client`` tool instance."""

    server: McpServerConfig | None = None
    server_name: str = ""
    remote_tool_name: str = ""
    input_schema: dict[str, Any] = field(default_factory=dict)
    timeout_seconds: float = _DEFAULT_TIMEOUT_SECONDS
    max_response_bytes: int = _DEFAULT_MAX_RESPONSE_BYTES

    @classmethod
    def from_spec(
        cls,
        raw: Mapping[str, Any],
        *,
        tool_name: str,
        mcp_servers: dict[str, McpServerConfig] | None = None,
        mcp_schemas: dict[str, dict[str, Any]] | None = None,
    ) -> McpToolConfig:
        if not isinstance(raw, dict):
            raise ValueError("mcp_client tool config must be a dict")

        server_name = str(raw.get("server", ""))
        inline_transport = raw.get("transport")

        server: McpServerConfig | None = None
        if inline_transport is not None:
            # Inline server definition; server_name is ignored.
            inline_spec = dict(raw)
            inline_spec.setdefault("name", tool_name)
            server = McpServerConfig.from_spec(inline_spec)
            server_name = server.name or tool_name
        elif server_name:
            # Reference to a domain-level server.
            if mcp_servers is None or server_name not in mcp_servers:
                raise ValueError(f"mcp_client tool references unknown server {server_name!r}")
            server = mcp_servers[server_name]
        else:
            raise ValueError(
                "mcp_client tool: either 'server' (reference) or 'transport' (inline) is required"
            )

        remote_tool_name = str(raw.get("tool", tool_name)).strip() or tool_name
        timeout = float(raw.get("timeout_seconds", _DEFAULT_TIMEOUT_SECONDS))
        max_bytes = int(raw.get("max_response_bytes", _DEFAULT_MAX_RESPONSE_BYTES))

        schema_key = f"{server_name}::{remote_tool_name}"
        schema: dict[str, Any] | None = None
        if mcp_schemas is not None:
            schema = mcp_schemas.get(schema_key)
        if schema is None and mcp_schemas is not None:
            # Fallback: try by server_name alone if the discovery keyed differently.
            schema = mcp_schemas.get(remote_tool_name)
        if schema is None:
            schema = {"type": "object"}

        return cls(
            server=server,
            server_name=server_name,
            remote_tool_name=remote_tool_name,
            input_schema=dict(schema),
            timeout_seconds=timeout,
            max_response_bytes=max_bytes,
        )


# ---------------------------------------------------------------------------
# Low-level JSON-RPC transports
# ---------------------------------------------------------------------------


class _JsonRpcError(Exception):
    """Raised when the server returns a JSON-RPC error object."""

    def __init__(self, code: int, message: str, data: Any = None) -> None:
        super().__init__(message)
        self.code = code
        self.message = message
        self.data = data


class _Transport:
    """Abstract JSON-RPC transport."""

    def send_notification(
        self,
        method: str,
        params: Any = None,
    ) -> None:
        raise NotImplementedError

    def send_request(self, method: str, params: Any = None, *, timeout: float | None = None) -> Any:
        raise NotImplementedError

    def close(self) -> None:
        pass


class _StdioTransport(_Transport):
    """JSON-RPC over a child process stdin/stdout."""

    def __init__(self, config: McpServerConfig) -> None:
        self._config = config
        self._proc: subprocess.Popen[str] | None = None
        self._lock = threading.Lock()
        self._reader_thread: threading.Thread | None = None
        self._pending: dict[Any, queue.Queue[dict[str, Any]]] = {}
        self._request_id = 0
        self._closed = False
        self._startup_error: str | None = None
        self._start()

    def _start(self) -> None:
        try:
            self._proc = subprocess.Popen(
                self._config.command,
                stdin=subprocess.PIPE,
                stdout=subprocess.PIPE,
                stderr=subprocess.PIPE,
                text=True,
                env=self._build_env(),
            )
        except FileNotFoundError as e:
            self._startup_error = f"mcp stdio server command not found: {e}"
            return
        except OSError as e:
            self._startup_error = f"mcp stdio server failed to start: {e}"
            return

        self._reader_thread = threading.Thread(target=self._read_loop, daemon=True)
        self._reader_thread.start()

    def _build_env(self) -> dict[str, str] | None:
        if not self._config.env:
            return None
        env = dict(os.environ)
        env.update(self._config.env)
        return env

    def _read_loop(self) -> None:
        proc = self._proc
        if proc is None or proc.stdout is None:
            return
        for line in proc.stdout:
            line = line.strip()
            if not line:
                continue
            try:
                msg = json.loads(line)
            except json.JSONDecodeError:
                log.warning("mcp stdio non-json line: %s", line[:200])
                continue
            self._dispatch(msg)
        # Stream closed; wake up any pending requests so they fail cleanly.
        with self._lock:
            for q in self._pending.values():
                q.put({"__eof": True})

    def _dispatch(self, msg: dict[str, Any]) -> None:
        msg_id = msg.get("id")
        if msg_id is None:
            # Notification; log and ignore.
            log.debug("mcp notification: %s", msg.get("method"))
            return
        with self._lock:
            q = self._pending.pop(msg_id, None)
        if q is None:
            log.warning("mcp unexpected response id %s", msg_id)
            return
        q.put(msg)

    def send_notification(
        self,
        method: str,
        params: Any = None,
    ) -> None:
        """Send a JSON-RPC notification (no id, no response expected)."""
        if self._startup_error:
            raise RuntimeError(self._startup_error)
        if self._closed or self._proc is None or self._proc.poll() is not None:
            raise RuntimeError("mcp stdio transport is closed or process has exited")

        msg: dict[str, Any] = {
            "jsonrpc": "2.0",
            "method": method,
        }
        if params is not None:
            msg["params"] = params

        line = json.dumps(msg, ensure_ascii=False)
        try:
            self._proc.stdin.write(line + "\n")
            self._proc.stdin.flush()
        except BrokenPipeError as e:
            raise RuntimeError(f"mcp stdio pipe broken: {e}") from e

    def send_request(
        self,
        method: str,
        params: Any = None,
        *,
        timeout: float | None = None,
    ) -> Any:
        if self._startup_error:
            raise RuntimeError(self._startup_error)
        if self._closed or self._proc is None or self._proc.poll() is not None:
            raise RuntimeError("mcp stdio transport is closed or process has exited")

        timeout = timeout or self._config.timeout_seconds
        with self._lock:
            self._request_id += 1
            req_id = self._request_id
            q: queue.Queue[dict[str, Any]] = queue.Queue(maxsize=1)
            self._pending[req_id] = q

        msg: dict[str, Any] = {
            "jsonrpc": "2.0",
            "id": req_id,
            "method": method,
        }
        if params is not None:
            msg["params"] = params

        line = json.dumps(msg, ensure_ascii=False)
        try:
            self._proc.stdin.write(line + "\n")
            self._proc.stdin.flush()
        except BrokenPipeError as e:
            with self._lock:
                self._pending.pop(req_id, None)
            raise RuntimeError(f"mcp stdio pipe broken: {e}") from e

        try:
            response = q.get(timeout=timeout)
        except Exception as e:
            with self._lock:
                self._pending.pop(req_id, None)
            raise TimeoutError(f"mcp stdio request timeout ({timeout}s): {method}") from e

        if response.get("__eof"):
            raise RuntimeError("mcp stdio server closed connection")
        if "error" in response:
            err = response["error"]
            raise _JsonRpcError(
                code=int(err.get("code", 0)),
                message=str(err.get("message", "unknown error")),
                data=err.get("data"),
            )
        return response.get("result")

    def close(self) -> None:
        self._closed = True
        if self._proc is not None and self._proc.poll() is None:
            try:
                self._proc.terminate()
                self._proc.wait(timeout=2)
            except Exception:  # noqa: BLE001
                try:
                    self._proc.kill()
                except Exception:  # noqa: BLE001
                    pass
        if self._reader_thread is not None and self._reader_thread.is_alive():
            self._reader_thread.join(timeout=1)


class _SseTransport(_Transport):
    """JSON-RPC over HTTP Server-Sent Events.

    The MCP SSE transport works like this:

      1. Open a streaming GET to the SSE URL.
      2. The first event is ``endpoint`` and advertises the POST URL for requests.
      3. Client POSTs JSON-RPC requests to that endpoint.
      4. Server sends JSON-RPC responses as ``message`` events on the SSE stream.
    """

    def __init__(self, config: McpServerConfig) -> None:
        self._config = config
        self._client = httpx.Client(timeout=config.timeout_seconds)
        self._post_url: str | None = None
        self._pending: dict[Any, queue.Queue[dict[str, Any]]] = {}
        self._lock = threading.Lock()
        self._reader_thread: threading.Thread | None = None
        self._request_id = 0
        self._closed = False
        self._connected_event = threading.Event()
        self._connect_error: str | None = None
        self._start()

    def _start(self) -> None:
        self._reader_thread = threading.Thread(target=self._read_loop, daemon=True)
        self._reader_thread.start()
        # Wait for endpoint event or connection error.
        if not self._connected_event.wait(timeout=self._config.timeout_seconds):
            self.close()
            raise TimeoutError(
                f"mcp sse endpoint not received within {self._config.timeout_seconds}s"
            )
        if self._connect_error:
            self.close()
            raise RuntimeError(self._connect_error)

    def _read_loop(self) -> None:
        try:
            with self._client.stream(
                "GET",
                self._config.url,
                headers={"Accept": "text/event-stream"},
            ) as response:
                response.raise_for_status()
                self._parse_stream(response.iter_lines())
        except Exception as e:  # noqa: BLE001
            self._connect_error = f"mcp sse connection failed: {e!s}"
            self._connected_event.set()

    def _parse_stream(self, lines) -> None:
        event_name = "message"
        data_lines: list[str] = []

        def flush() -> None:
            nonlocal event_name, data_lines
            if not data_lines:
                event_name = "message"
                return
            data = "\n".join(data_lines)
            if event_name == "endpoint":
                self._post_url = self._resolve_post_url(data.strip())
                self._connected_event.set()
            elif event_name == "message":
                try:
                    msg = json.loads(data)
                except json.JSONDecodeError:
                    log.warning("mcp sse non-json message: %s", data[:200])
                    return
                self._dispatch(msg)
            event_name = "message"
            data_lines = []

        for raw in lines:
            line = raw if isinstance(raw, str) else raw.decode("utf-8", errors="replace")
            if line.startswith("event:"):
                event_name = line[len("event:") :].strip()
            elif line.startswith("data:"):
                data_lines.append(line[len("data:") :].strip())
            elif line == "":
                flush()
            else:
                # Comments / unknown lines; ignore.
                pass
        flush()

    def _resolve_post_url(self, endpoint: str) -> str:
        if endpoint.startswith("http://") or endpoint.startswith("https://"):
            return endpoint
        base = self._config.url
        if endpoint.startswith("/"):
            # Absolute path; replace path on base URL.
            parsed = httpx.URL(base)
            return str(parsed.copy_with(path=endpoint))
        # Relative path; append to base directory.
        if base.endswith("/"):
            return base + endpoint
        return base.rsplit("/", 1)[0] + "/" + endpoint

    def _dispatch(self, msg: dict[str, Any]) -> None:
        msg_id = msg.get("id")
        if msg_id is None:
            log.debug("mcp sse notification: %s", msg.get("method"))
            return
        with self._lock:
            q = self._pending.pop(msg_id, None)
        if q is None:
            log.warning("mcp sse unexpected response id %s", msg_id)
            return
        q.put(msg)

    def send_notification(
        self,
        method: str,
        params: Any = None,
    ) -> None:
        """Send a JSON-RPC notification via POST (no id, no response expected)."""
        if self._closed:
            raise RuntimeError("mcp sse transport is closed")
        if not self._post_url:
            raise RuntimeError("mcp sse endpoint not yet received")

        msg: dict[str, Any] = {
            "jsonrpc": "2.0",
            "method": method,
        }
        if params is not None:
            msg["params"] = params

        try:
            resp = self._client.post(
                self._post_url,
                json=msg,
                headers={"Content-Type": "application/json"},
                timeout=self._config.timeout_seconds,
            )
            resp.raise_for_status()
        except httpx.HTTPError as e:
            raise RuntimeError(f"mcp sse notification POST failed: {e!s}") from e

    def send_request(
        self,
        method: str,
        params: Any = None,
        *,
        timeout: float | None = None,
    ) -> Any:
        if self._closed:
            raise RuntimeError("mcp sse transport is closed")
        if not self._post_url:
            raise RuntimeError("mcp sse endpoint not yet received")

        timeout = timeout or self._config.timeout_seconds
        with self._lock:
            self._request_id += 1
            req_id = self._request_id
            q: queue.Queue[dict[str, Any]] = queue.Queue(maxsize=1)
            self._pending[req_id] = q

        msg: dict[str, Any] = {
            "jsonrpc": "2.0",
            "id": req_id,
            "method": method,
        }
        if params is not None:
            msg["params"] = params

        try:
            resp = self._client.post(
                self._post_url,
                json=msg,
                headers={"Content-Type": "application/json"},
                timeout=timeout,
            )
            resp.raise_for_status()
        except httpx.HTTPError as e:
            with self._lock:
                self._pending.pop(req_id, None)
            raise RuntimeError(f"mcp sse POST failed: {e!s}") from e

        try:
            response = q.get(timeout=timeout)
        except Exception as e:
            with self._lock:
                self._pending.pop(req_id, None)
            raise TimeoutError(f"mcp sse response timeout ({timeout}s): {method}") from e

        if "error" in response:
            err = response["error"]
            raise _JsonRpcError(
                code=int(err.get("code", 0)),
                message=str(err.get("message", "unknown error")),
                data=err.get("data"),
            )
        return response.get("result")

    def close(self) -> None:
        self._closed = True
        self._connected_event.set()
        try:
            self._client.close()
        except Exception:  # noqa: BLE001
            pass
        if self._reader_thread is not None and self._reader_thread.is_alive():
            self._reader_thread.join(timeout=1)


# ---------------------------------------------------------------------------
# MCP client
# ---------------------------------------------------------------------------


class McpClient:
    """A reusable MCP client for one server.

    Lazily initializes the session (``initialize`` handshake + ``tools/list``
    caching) and exposes ``call_tool``.
    """

    def __init__(self, config: McpServerConfig) -> None:
        self._config = config
        self._transport: _Transport | None = None
        self._initialized = False
        self._tools: list[dict[str, Any]] | None = None
        self._lock = threading.Lock()

    @property
    def server_name(self) -> str:
        return self._config.name or self._config.url or "stdio"

    def _transport_for_config(self) -> _Transport:
        if self._config.transport == "stdio":
            return _StdioTransport(self._config)
        if self._config.transport == "sse":
            return _SseTransport(self._config)
        raise ValueError(f"unsupported mcp transport {self._config.transport!r}")

    def _ensure_transport(self) -> _Transport:
        if self._transport is None:
            self._transport = self._transport_for_config()
        return self._transport

    def initialize(self) -> None:
        """Run the MCP initialize handshake if not already done."""
        with self._lock:
            if self._initialized:
                return
            transport = self._ensure_transport()
            result = transport.send_request(
                "initialize",
                {
                    "protocolVersion": _MCP_PROTOCOL_VERSION,
                    "capabilities": {},
                    "clientInfo": {"name": "hnsx-worker", "version": "0.1.0"},
                },
                timeout=self._config.timeout_seconds,
            )
            log.debug("mcp initialized: %s", result)
            # Send notification that we are initialized.
            transport.send_notification("notifications/initialized", {})
            self._initialized = True

    def list_tools(self) -> list[dict[str, Any]]:
        """Return the remote tool list, cached after first call."""
        self.initialize()
        if self._tools is not None:
            return list(self._tools)
        with self._lock:
            if self._tools is not None:
                return list(self._tools)
            transport = self._ensure_transport()
            result = transport.send_request(
                "tools/list",
                {},
                timeout=self._config.timeout_seconds,
            )
            tools = result.get("tools") if isinstance(result, dict) else []
            self._tools = list(tools) if tools else []
            return list(self._tools)

    def get_tool_schema(self, name: str) -> dict[str, Any] | None:
        """Find the input schema for a named remote tool."""
        for tool in self.list_tools():
            if tool.get("name") == name:
                return tool.get("inputSchema") or tool.get("input_schema")
        return None

    def call_tool(
        self,
        name: str,
        arguments: dict[str, Any],
        *,
        timeout: float | None = None,
    ) -> dict[str, Any]:
        """Call a remote MCP tool and return its content."""
        self.initialize()
        timeout = timeout or self._config.timeout_seconds
        transport = self._ensure_transport()
        result = transport.send_request(
            "tools/call",
            {"name": name, "arguments": arguments},
            timeout=timeout,
        )
        if not isinstance(result, dict):
            return {"content": [{"type": "text", "text": json.dumps(result, default=str)}]}
        return result

    def close(self) -> None:
        if self._transport is not None:
            self._transport.close()
            self._transport = None
        self._initialized = False
        self._tools = None

    def __del__(self) -> None:
        try:
            self.close()
        except Exception:  # noqa: BLE001
            pass


# ---------------------------------------------------------------------------
# Tool implementation
# ---------------------------------------------------------------------------


class McpClientTool(Tool):
    """A worker Tool that delegates to an MCP server tool."""

    def __init__(self, name: str, config: McpToolConfig) -> None:
        self._name = name
        self._config = config

    @property
    def name(self) -> str:
        return self._name

    @property
    def schema(self) -> dict[str, Any]:
        schema = (
            dict(self._config.input_schema) if self._config.input_schema else {"type": "object"}
        )
        # Ensure required JSON schema fields.
        if "type" not in schema:
            schema["type"] = "object"
        return schema

    @property
    def config(self) -> McpToolConfig:
        return self._config

    def invoke(self, ctx: ToolContext, input: dict[str, Any]) -> ToolResult:
        if not isinstance(input, dict):
            return ToolResult(error="mcp_client input must be a JSON object")

        server = self._config.server
        if server is None:
            return ToolResult(
                error=f"mcp_client tool {self._name!r}: server config not resolved",
            )

        started = time.monotonic()
        client: McpClient | None = None
        try:
            client = McpClient(server)
            result = client.call_tool(
                self._config.remote_tool_name,
                dict(input),
                timeout=self._config.timeout_seconds,
            )
            elapsed_ms = int((time.monotonic() - started) * 1000)
            self._emit_mcp_call(ctx, server, elapsed_ms, ok=True)
            return ToolResult(
                output=_sanitize_mcp_result(result, self._config.max_response_bytes),
                metadata={
                    "server": server.name or server.url or "inline-stdio",
                    "transport": server.transport,
                    "remote_tool": self._config.remote_tool_name,
                    "elapsed_ms": elapsed_ms,
                },
            )
        except _JsonRpcError as e:
            elapsed_ms = int((time.monotonic() - started) * 1000)
            self._emit_mcp_call(ctx, server, elapsed_ms, ok=False)
            return ToolResult(
                error=f"mcp error [{e.code}]: {e.message}",
                metadata={"remote_tool": self._config.remote_tool_name, "elapsed_ms": elapsed_ms},
            )
        except Exception as e:  # noqa: BLE001
            elapsed_ms = int((time.monotonic() - started) * 1000)
            self._emit_mcp_call(ctx, server, elapsed_ms, ok=False)
            return ToolResult(
                error=f"mcp call failed: {e!s}",
                metadata={"remote_tool": self._config.remote_tool_name, "elapsed_ms": elapsed_ms},
            )
        finally:
            if client is not None:
                client.close()

    def _emit_mcp_call(
        self,
        ctx: ToolContext,
        server: McpServerConfig,
        elapsed_ms: int,
        *,
        ok: bool,
    ) -> None:
        if ctx.emit is None:
            return
        ctx.emit(
            {
                "kind": "mcp_call",
                "session_id": ctx.session_id,
                "domain_id": ctx.domain_id,
                "agent_id": ctx.agent_id,
                "payload": {
                    "tool_call_id": ctx.tool_call_id,
                    "name": self._name,
                    "remote_tool": self._config.remote_tool_name,
                    "server": server.name or server.url or "inline-stdio",
                    "transport": server.transport,
                    "elapsed_ms": elapsed_ms,
                    "ok": ok,
                },
            }
        )


def _sanitize_mcp_result(result: dict[str, Any], max_bytes: int) -> dict[str, Any]:
    """Trim MCP tool result content to keep observations bounded."""
    content = result.get("content")
    if isinstance(content, list):
        trimmed = []
        total = 0
        for item in content:
            item_copy = dict(item)
            text = item_copy.get("text")
            if isinstance(text, str):
                if total + len(text.encode("utf-8")) > max_bytes:
                    allowed = max(0, max_bytes - total)
                    if allowed > 0:
                        item_copy["text"] = text.encode("utf-8")[:allowed].decode(
                            "utf-8", errors="ignore"
                        )
                        item_copy["truncated"] = True
                        trimmed.append(item_copy)
                    break
                total += len(text.encode("utf-8"))
            trimmed.append(item_copy)
        return {"content": trimmed, "isError": result.get("isError", False)}
    return result


# ---------------------------------------------------------------------------
# Server discovery helpers used by session_executor
# ---------------------------------------------------------------------------


def build_mcp_server_map(spec: dict[str, Any]) -> dict[str, McpServerConfig]:
    """Build a name -> McpServerConfig map from the DomainSpec harness block."""
    harness = spec.get("harness") or {}
    servers = harness.get("mcp_servers") or []
    out: dict[str, McpServerConfig] = {}
    for entry in servers:
        if not isinstance(entry, dict):
            log.warning("ignoring non-dict mcp_servers entry: %r", entry)
            continue
        try:
            cfg = McpServerConfig.from_spec(entry)
        except ValueError as e:
            log.warning("invalid mcp_servers entry: %s", e)
            continue
        name = cfg.name or str(uuid.uuid4())[:8]
        out[name] = cfg
    return out


def discover_mcp_tools(
    server_config: McpServerConfig,
) -> dict[str, dict[str, Any]]:
    """Connect once to an MCP server and return remote tool name -> input schema.

    The caller is responsible for closing the returned client if it wants to
    reuse the connection; this function closes the client before returning.
    """
    client = McpClient(server_config)
    try:
        tools = client.list_tools()
    finally:
        client.close()
    out: dict[str, dict[str, Any]] = {}
    for tool in tools:
        tool_name = str(tool.get("name", ""))
        if not tool_name:
            continue
        out[tool_name] = dict(tool.get("inputSchema") or tool.get("input_schema") or {})
    return out


__all__ = [
    "McpClient",
    "McpClientTool",
    "McpServerConfig",
    "McpToolConfig",
    "build_mcp_server_map",
    "discover_mcp_tools",
]
