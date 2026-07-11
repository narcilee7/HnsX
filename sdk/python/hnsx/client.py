"""Entry point for the HnsX Python SDK."""

from __future__ import annotations

from hnsx.base import BaseClient
from hnsx.resources import (
    ApprovalsClient,
    DomainRegistryClient,
    EvalsClient,
    SessionsClient,
    TracesClient,
)
from hnsx.sse import SSEEvent, stream_session_events

DEFAULT_BASE_URL = "http://127.0.0.1:50052"


class HnsXClient:
    """Python SDK client for the HnsX REST API."""

    def __init__(self, base_url: str = DEFAULT_BASE_URL, headers: dict[str, str] | None = None) -> None:
        self.base_url = base_url.rstrip("/")
        self.headers = headers or {}
        self.domains = DomainRegistryClient(self.base_url, self.headers)
        self.sessions = SessionsClient(self.base_url, self.headers)
        self.traces = TracesClient(self.base_url, self.headers)
        self.approvals = ApprovalsClient(self.base_url, self.headers)
        self.evals = EvalsClient(self.base_url, self.headers)

    def stream_session_events(
        self,
        session_id: str,
        *,
        timeout: float = 300.0,
        on_event: callable | None = None,
    ) -> None:
        """Yield SSE events for the given session."""
        yield from stream_session_events(
            self.base_url,
            session_id,
            headers=self.headers,
            timeout=timeout,
            on_event=on_event,
        )

    def close(self) -> None:
        for client in (self.domains, self.sessions, self.traces, self.approvals, self.evals):
            if isinstance(client, BaseClient):
                client.close()

    def __enter__(self) -> "HnsXClient":
        return self

    def __exit__(self, *args: object) -> None:
        self.close()


__all__ = [
    "HnsXClient",
    "stream_session_events",
    "SSEEvent",
]
