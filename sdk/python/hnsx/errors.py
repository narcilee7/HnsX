"""Canonical error envelope returned by the HnsX server."""

from __future__ import annotations

from typing import Any

class APIError(Exception):
    """Raised when the HnsX server returns a non-2xx response."""

    def __init__(
        self,
        code: str,
        message: str,
        status: int,
        details: dict[str, Any] | None = None,
    ) -> None:
        super().__init__(message)
        self.code = code
        self.status = status
        self.details = details or {}

    def __repr__(self) -> str:
        return f"APIError(code={self.code!r}, status={self.status}, message={self.args[0]!r})"


def raise_for_status(response: httpx.Response) -> None:
    """Parse a non-2xx response and raise APIError."""
    import httpx

    try:
        body = response.json()
    except (httpx.DecoderError, ValueError):
        body = {}
    raise APIError(
        code=body.get("code") or f"HTTP_{response.status_code}",
        message=body.get("message") or response.reason_phrase or "Request failed",
        status=response.status_code,
        details=body.get("details"),
    )
