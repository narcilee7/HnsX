#!/usr/bin/env python3
"""Tiny stdio MCP server for the mcp-demo example domain.

Exposes two read-only filesystem tools:

  - ``list_directory(path=".")`` — returns file names.
  - ``read_file(path)`` — returns file contents as text.

This is intentionally dependency-free so it runs with any Python 3.11+.
"""

from __future__ import annotations

import json
import os
import sys


def send(msg: dict) -> None:
    sys.stdout.write(json.dumps(msg) + "\n")
    sys.stdout.flush()


def handle(req: dict) -> None:
    req_id = req.get("id")
    method = req.get("method")
    params = req.get("params") or {}

    if method == "initialize":
        send({
            "jsonrpc": "2.0",
            "id": req_id,
            "result": {
                "protocolVersion": "2024-11-05",
                "serverInfo": {"name": "demo-filesystem", "version": "0.1.0"},
                "capabilities": {},
            },
        })
        return

    if method == "notifications/initialized":
        return

    if method == "tools/list":
        send({
            "jsonrpc": "2.0",
            "id": req_id,
            "result": {
                "tools": [
                    {
                        "name": "list_directory",
                        "description": "List files in a directory",
                        "inputSchema": {
                            "type": "object",
                            "properties": {
                                "path": {
                                    "type": "string",
                                    "description": "Directory path to list (default: current directory)",
                                },
                            },
                        },
                    },
                    {
                        "name": "read_file",
                        "description": "Read a text file",
                        "inputSchema": {
                            "type": "object",
                            "properties": {
                                "path": {"type": "string"},
                            },
                            "required": ["path"],
                        },
                    },
                ]
            },
        })
        return

    if method == "tools/call":
        name = params.get("name")
        args = params.get("arguments") or {}
        try:
            if name == "list_directory":
                path = args.get("path", ".")
                entries = sorted(os.listdir(path))
                text = json.dumps(entries, ensure_ascii=False, indent=2)
            elif name == "read_file":
                path = args.get("path")
                if not path:
                    raise ValueError("path is required")
                with open(path, "r", encoding="utf-8") as f:
                    text = f.read()
            else:
                send({
                    "jsonrpc": "2.0",
                    "id": req_id,
                    "error": {"code": -32601, "message": f"unknown tool {name}"},
                })
                return
            send({
                "jsonrpc": "2.0",
                "id": req_id,
                "result": {"content": [{"type": "text", "text": text}]},
            })
        except Exception as e:  # noqa: BLE001
            send({
                "jsonrpc": "2.0",
                "id": req_id,
                "result": {
                    "isError": True,
                    "content": [{"type": "text", "text": f"error: {e}"}],
                },
            })
        return

    send({
        "jsonrpc": "2.0",
        "id": req_id,
        "error": {"code": -32601, "message": f"method {method} not found"},
    })


def main() -> None:
    for line in sys.stdin:
        line = line.strip()
        if not line:
            continue
        try:
            req = json.loads(line)
        except json.JSONDecodeError:
            continue
        handle(req)


if __name__ == "__main__":
    main()
