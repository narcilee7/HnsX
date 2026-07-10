"""Tests for the MCP client tool.

These tests spin up tiny JSON-RPC MCP servers over stdio and SSE to verify
that the worker can discover and call remote tools. No external network or
Node packages are required.
"""

from __future__ import annotations

import json
import sys
import tempfile
import threading
from collections.abc import Iterator
from pathlib import Path
from typing import Any

import pytest

from hnsx_worker.tools import ToolContext, ToolRegistry
from hnsx_worker.tools.factory import build_tool
from hnsx_worker.tools.mcp_client import (
    McpClient,
    McpClientTool,
    McpServerConfig,
    McpToolConfig,
    build_mcp_server_map,
    discover_mcp_tools,
)

# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------


def _echo_mcp_server_script() -> str:
    """Return source code for a tiny stdio MCP server that exposes ``echo``."""
    return '''\
import json
import sys


def send(msg):
    sys.stdout.write(json.dumps(msg) + "\\n")
    sys.stdout.flush()


def main():
    for line in sys.stdin:
        line = line.strip()
        if not line:
            continue
        try:
            req = json.loads(line)
        except json.JSONDecodeError:
            continue
        req_id = req.get("id")
        method = req.get("method")
        params = req.get("params") or {}

        if method == "initialize":
            send({
                "jsonrpc": "2.0",
                "id": req_id,
                "result": {
                    "protocolVersion": "2024-11-05",
                    "serverInfo": {"name": "echo", "version": "1.0"},
                    "capabilities": {},
                },
            })
        elif method == "notifications/initialized":
            pass
        elif method == "tools/list":
            send({
                "jsonrpc": "2.0",
                "id": req_id,
                "result": {
                    "tools": [
                        {
                            "name": "echo",
                            "description": "Echo a message",
                            "inputSchema": {
                                "type": "object",
                                "properties": {
                                    "message": {"type": "string"},
                                },
                                "required": ["message"],
                            },
                        }
                    ]
                },
            })
        elif method == "tools/call":
            name = params.get("name")
            args = params.get("arguments") or {}
            if name == "echo":
                send({
                    "jsonrpc": "2.0",
                    "id": req_id,
                    "result": {
                        "content": [
                            {
                                "type": "text",
                                "text": f"echo: {args.get('message', '')}",
                            }
                        ]
                    },
                })
            else:
                send({
                    "jsonrpc": "2.0",
                    "id": req_id,
                    "error": {
                        "code": -32601,
                        "message": f"unknown tool {name}",
                    },
                })
        else:
            send({
                "jsonrpc": "2.0",
                "id": req_id,
                "error": {
                    "code": -32601,
                    "message": f"method {method} not found",
                },
            })


if __name__ == "__main__":
    main()
'''


@pytest.fixture
def echo_server_path() -> Iterator[Path]:
    with tempfile.NamedTemporaryFile(mode="w", suffix=".py", delete=False) as f:
        f.write(_echo_mcp_server_script())
        path = Path(f.name)
    yield path
    path.unlink(missing_ok=True)


# ---------------------------------------------------------------------------
# Configuration parsing
# ---------------------------------------------------------------------------


def test_mcp_server_config_stdio() -> None:
    cfg = McpServerConfig.from_spec(
        {
            "name": "echo",
            "transport": "stdio",
            "command": [sys.executable, "-c", "print('ok')"],
            "env": {"FOO": "bar"},
            "timeout_seconds": 5,
        }
    )
    assert cfg.name == "echo"
    assert cfg.transport == "stdio"
    assert cfg.command[0] == sys.executable
    assert cfg.env == {"FOO": "bar"}
    assert cfg.timeout_seconds == 5


def test_mcp_server_config_sse() -> None:
    cfg = McpServerConfig.from_spec(
        {
            "name": "remote",
            "transport": "sse",
            "url": "http://localhost:3001/sse",
        }
    )
    assert cfg.transport == "sse"
    assert cfg.url == "http://localhost:3001/sse"


def test_mcp_server_config_rejects_unknown_transport() -> None:
    with pytest.raises(ValueError, match="transport"):
        McpServerConfig.from_spec({"transport": "websocket"})


def test_mcp_server_config_rejects_stdio_without_command() -> None:
    with pytest.raises(ValueError, match="command"):
        McpServerConfig.from_spec({"transport": "stdio"})


def test_mcp_server_config_rejects_sse_without_url() -> None:
    with pytest.raises(ValueError, match="url"):
        McpServerConfig.from_spec({"transport": "sse"})


def test_mcp_tool_config_inline() -> None:
    cfg = McpToolConfig.from_spec(
        {
            "transport": "stdio",
            "command": [sys.executable, "-c", "pass"],
        },
        tool_name="echo",
    )
    assert cfg.server is not None
    assert cfg.server.transport == "stdio"
    assert cfg.remote_tool_name == "echo"


def test_mcp_tool_config_reference_unknown_server() -> None:
    with pytest.raises(ValueError, match="unknown server"):
        McpToolConfig.from_spec(
            {"server": "missing"},
            tool_name="echo",
            mcp_servers={},
        )


def test_mcp_tool_config_reference_resolved() -> None:
    server = McpServerConfig.from_spec(
        {"name": "echo", "transport": "stdio", "command": [sys.executable, "-c", "pass"]}
    )
    cfg = McpToolConfig.from_spec(
        {"server": "echo", "tool": "echo"},
        tool_name="my_echo",
        mcp_servers={"echo": server},
        mcp_schemas={"echo::echo": {"type": "object", "properties": {}}},
    )
    assert cfg.server is server
    assert cfg.remote_tool_name == "echo"
    assert cfg.input_schema == {"type": "object", "properties": {}}


# ---------------------------------------------------------------------------
# MCP client (stdio)
# ---------------------------------------------------------------------------


def test_mcp_client_lists_and_calls_tools(echo_server_path: Path) -> None:
    cfg = McpServerConfig(
        name="echo",
        transport="stdio",
        command=[sys.executable, str(echo_server_path)],
        timeout_seconds=5,
    )
    client = McpClient(cfg)
    try:
        tools = client.list_tools()
        assert [t["name"] for t in tools] == ["echo"]

        schema = client.get_tool_schema("echo")
        assert schema["type"] == "object"
        assert "message" in schema["properties"]

        result = client.call_tool("echo", {"message": "hello"})
        assert result["content"][0]["text"] == "echo: hello"
    finally:
        client.close()


def test_mcp_client_stdio_command_not_found() -> None:
    cfg = McpServerConfig(
        name="bad",
        transport="stdio",
        command=["this_command_definitely_does_not_exist_12345"],
        timeout_seconds=2,
    )
    client = McpClient(cfg)
    try:
        with pytest.raises(RuntimeError, match="not found"):
            client.initialize()
    finally:
        client.close()


def test_mcp_client_stdio_timeout() -> None:
    # A script that never responds; initialize should time out.
    script = 'import time\ntime.sleep(60)\n'
    with tempfile.NamedTemporaryFile(mode="w", suffix=".py", delete=False) as f:
        f.write(script)
        path = Path(f.name)
    try:
        cfg = McpServerConfig(
            name="slow",
            transport="stdio",
            command=[sys.executable, str(path)],
            timeout_seconds=0.5,
        )
        client = McpClient(cfg)
        try:
            with pytest.raises(TimeoutError):
                client.initialize()
        finally:
            client.close()
    finally:
        path.unlink(missing_ok=True)


# ---------------------------------------------------------------------------
# Tool integration
# ---------------------------------------------------------------------------


def test_mcp_client_tool_invokes_remote(echo_server_path: Path) -> None:
    cfg = McpToolConfig.from_spec(
        {
            "transport": "stdio",
            "command": [sys.executable, str(echo_server_path)],
            "tool": "echo",
            "timeout_seconds": 5,
        },
        tool_name="echo",
    )
    tool = McpClientTool("echo", cfg)
    ctx = ToolContext(session_id="s", domain_id="d", agent_id="a")
    result = tool.invoke(ctx, {"message": "world"})
    assert result.ok
    assert result.output["content"][0]["text"] == "echo: world"


def test_mcp_client_tool_returns_error_for_unknown_remote_tool(
    echo_server_path: Path,
) -> None:
    cfg = McpToolConfig.from_spec(
        {
            "transport": "stdio",
            "command": [sys.executable, str(echo_server_path)],
            "tool": "missing",
            "timeout_seconds": 5,
        },
        tool_name="missing",
    )
    tool = McpClientTool("missing", cfg)
    ctx = ToolContext()
    result = tool.invoke(ctx, {})
    assert not result.ok
    assert "unknown tool" in result.error


def test_factory_builds_mcp_client_tool(echo_server_path: Path) -> None:
    tool = build_tool(
        {
            "name": "echo",
            "type": "mcp_client",
            "config": {
                "transport": "stdio",
                "command": [sys.executable, str(echo_server_path)],
                "tool": "echo",
            },
        }
    )
    assert isinstance(tool, McpClientTool)
    assert tool.schema["type"] == "object"


def test_registry_dispatches_mcp_client_tool(echo_server_path: Path) -> None:
    tool = build_tool(
        {
            "name": "echo",
            "type": "mcp_client",
            "config": {
                "transport": "stdio",
                "command": [sys.executable, str(echo_server_path)],
                "tool": "echo",
            },
        }
    )
    reg = ToolRegistry()
    reg.register(tool)
    result = reg.call("echo", ToolContext(), {"message": "registry"})
    assert result.ok
    assert result.output["content"][0]["text"] == "echo: registry"


# ---------------------------------------------------------------------------
# Domain-level MCP server helpers
# ---------------------------------------------------------------------------


def test_build_mcp_server_map(echo_server_path: Path) -> None:
    spec = {
        "harness": {
            "mcp_servers": [
                {
                    "name": "echo",
                    "transport": "stdio",
                    "command": [sys.executable, str(echo_server_path)],
                }
            ]
        }
    }
    mapping = build_mcp_server_map(spec)
    assert "echo" in mapping
    assert mapping["echo"].transport == "stdio"


def test_discover_mcp_tools(echo_server_path: Path) -> None:
    cfg = McpServerConfig(
        name="echo",
        transport="stdio",
        command=[sys.executable, str(echo_server_path)],
        timeout_seconds=5,
    )
    schemas = discover_mcp_tools(cfg)
    assert "echo" in schemas
    assert schemas["echo"]["type"] == "object"


def test_mcp_demo_domain_tool_registry() -> None:
    """Verify the example mcp-demo DomainSpec shape builds a valid registry."""
    from hnsx_worker.session_executor import _build_tool_registry

    repo_root = Path(__file__).resolve().parents[2]
    server_script = repo_root / "example-domains" / "mcp-demo" / "mcp_server.py"
    assert server_script.exists(), server_script
    spec = {
        "id": "mcp-demo",
        "harness": {
            "mcp_servers": [
                {
                    "name": "demo_filesystem",
                    "transport": "stdio",
                    "command": [sys.executable, str(server_script)],
                    "timeout_seconds": 5,
                }
            ],
            "agents": {
                "demo_agent": {
                    "id": "demo_agent",
                    "provider": "noop",
                    "adapter": {"kind": "noop"},
                    "system_prompt": "default",
                    "tools": [
                        {
                            "name": "list_directory",
                            "type": "mcp_client",
                            "config": {"server": "demo_filesystem", "tool": "list_directory"},
                        },
                        {
                            "name": "read_file",
                            "type": "mcp_client",
                            "config": {"server": "demo_filesystem", "tool": "read_file"},
                        },
                    ],
                }
            },
        },
    }
    agent = spec["harness"]["agents"]["demo_agent"]
    registry, failures = _build_tool_registry(
        spec=spec,
        agent=agent,
        session_id="s",
        domain_id="mcp-demo",
        emit=lambda o: None,
    )
    assert failures == []
    assert "list_directory" in registry
    assert "read_file" in registry
    schemas = {t["name"]: t["input_schema"] for t in agent["tools"]}
    assert schemas["list_directory"]["type"] == "object"
    assert "path" in schemas["list_directory"].get("properties", {})
    assert "path" in schemas["read_file"].get("properties", {})


# ---------------------------------------------------------------------------
# SSE transport (smoke test with a local threaded HTTP server)
# ---------------------------------------------------------------------------


def _sse_mcp_server() -> Iterator[tuple[str, threading.Event]]:
    """Start a tiny MCP-over-SSE server; yield base URL and a shutdown event."""
    from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

    queue: list[tuple[int, dict[str, Any]]] = []
    queue_lock = threading.Lock()
    queue_event = threading.Event()
    shutdown_event = threading.Event()

    class Handler(BaseHTTPRequestHandler):
        def log_message(self, format: str, *args: Any) -> None:
            pass

        def do_GET(self) -> None:
            if self.path != "/sse":
                self.send_error(404)
                return
            self.send_response(200)
            self.send_header("Content-Type", "text/event-stream")
            self.send_header("Cache-Control", "no-cache")
            self.end_headers()
            # Endpoint event
            self.wfile.write(b"event: endpoint\ndata: /messages\n\n")
            self.wfile.flush()

            while not shutdown_event.is_set():
                if queue_event.wait(0.05):
                    with queue_lock:
                        items = list(queue)
                        queue.clear()
                    queue_event.clear()
                    for req_id, msg in items:
                        payload = json.dumps({"jsonrpc": "2.0", "id": req_id, "result": msg})
                        self.wfile.write(
                            f"event: message\ndata: {payload}\n\n".encode()
                        )
                    self.wfile.flush()

        def do_POST(self) -> None:
            if self.path != "/messages":
                self.send_error(404)
                return
            length = int(self.headers.get("Content-Length", "0"))
            body = self.rfile.read(length)
            try:
                req = json.loads(body)
            except json.JSONDecodeError:
                self.send_error(400)
                return
            method = req.get("method")
            req_id = req.get("id")
            params = req.get("params") or {}

            if method == "initialize":
                result = {
                    "protocolVersion": "2024-11-05",
                    "serverInfo": {"name": "sse-echo", "version": "1.0"},
                    "capabilities": {},
                }
            elif method == "tools/list":
                result = {
                    "tools": [
                        {
                            "name": "greet",
                            "inputSchema": {
                                "type": "object",
                                "properties": {"name": {"type": "string"}},
                            },
                        }
                    ]
                }
            elif method == "tools/call":
                name = params.get("name")
                args = params.get("arguments") or {}
                if name == "greet":
                    result = {
                        "content": [{"type": "text", "text": f"hello {args.get('name')}"}]
                    }
                else:
                    result = {"error": {"code": -32601, "message": "unknown"}}
            else:
                result = {"error": {"code": -32601, "message": f"{method}"}}

            with queue_lock:
                queue.append((req_id, result))
            queue_event.set()

            self.send_response(202)
            self.end_headers()

    server = ThreadingHTTPServer(("127.0.0.1", 0), Handler)
    thread = threading.Thread(target=server.serve_forever, daemon=True)
    thread.start()
    try:
        yield f"http://127.0.0.1:{server.server_port}/sse", shutdown_event
    finally:
        shutdown_event.set()
        server.shutdown()
        server.server_close()
        thread.join(timeout=2)


def test_mcp_client_sse_list_and_call() -> None:
    for url, shutdown in _sse_mcp_server():
        try:
            cfg = McpServerConfig(
                name="sse-echo",
                transport="sse",
                url=url,
                timeout_seconds=5,
            )
            client = McpClient(cfg)
            try:
                tools = client.list_tools()
                assert [t["name"] for t in tools] == ["greet"]
                result = client.call_tool("greet", {"name": "mcp"})
                assert result["content"][0]["text"] == "hello mcp"
            finally:
                client.close()
        finally:
            shutdown.set()


def test_mcp_client_sse_tool_invocation() -> None:
    for url, shutdown in _sse_mcp_server():
        try:
            cfg = McpToolConfig.from_spec(
                {"transport": "sse", "url": url, "tool": "greet"},
                tool_name="greet",
            )
            tool = McpClientTool("greet", cfg)
            result = tool.invoke(ToolContext(), {"name": "harness"})
            assert result.ok
            assert result.output["content"][0]["text"] == "hello harness"
        finally:
            shutdown.set()


# ---------------------------------------------------------------------------
# Truncation
# ---------------------------------------------------------------------------


def test_mcp_result_truncation() -> None:
    from hnsx_worker.tools.mcp_client import _sanitize_mcp_result

    big = "x" * 1000
    result = _sanitize_mcp_result({"content": [{"type": "text", "text": big}]}, max_bytes=100)
    text = result["content"][0]["text"]
    assert len(text.encode("utf-8")) <= 100
    assert result["content"][0].get("truncated") is True
