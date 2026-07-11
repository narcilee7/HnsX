"""SSE streaming helper for the HnsX Python SDK."""

from __future__ import annotations

from typing import Callable

import httpx

from hnsx.errors import APIError


class SSEEvent:
    """One Server-Sent Event."""

    def __init__(self, name: str, payload: str) -> None:
        self.name = name
        self.payload = payload

    def __repr__(self) -> str:
        return f"SSEEvent(name={self.name!r}, payload={self.payload!r})"


def stream_session_events(
    base_url: str,
    session_id: str,
    *,
    headers: dict[str, str] | None = None,
    timeout: float = 300.0,
    on_event: Callable[[SSEEvent], None] | None = None,
):
    """Consume the live SSE event stream for a session.

    This is a blocking generator. Use it in a thread or asyncio task if you need
    concurrent execution.
    """
    url = f"{base_url.rstrip('/')}/api/v1/sessions/{session_id}/events"
    with httpx.stream(
        "GET",
        url,
        headers={"Accept": "text/event-stream", **(headers or {})},
        timeout=timeout,
    ) as response:
        if not response.is_success:
            _raise_sse_error(response)

        current_name = ""
        current_payload = ""
        for line in response.iter_lines():
            if line == "":
                if current_name:
                    event = SSEEvent(current_name, current_payload)
                    if on_event:
                        on_event(event)
                    yield event
                current_name = ""
                current_payload = ""
            elif line.startswith("event: "):
                current_name = line[7:].strip()
            elif line.startswith("data: "):
                current_payload += line[6:]


def _raise_sse_error(response: httpx.Response) -> None:
    try:
        body = response.json()
    except Exception:
        body = {}
    raise APIError(
        code=body.get("code") or f"HTTP_{response.status_code}",
        message=body.get("message") or response.reason_phrase or "SSE request failed",
        status=response.status_code,
        details=body.get("details"),
    )
