"""Base HTTP client for the HnsX Python SDK."""

from __future__ import annotations

from typing import Any
from urllib.parse import urlencode

import httpx

from hnsx.errors import raise_for_status


class BaseClient:
    """Provides low-level REST helpers used by resource clients."""

    def __init__(self, base_url: str, headers: dict[str, str] | None = None) -> None:
        self.base_url = base_url.rstrip("/")
        self.headers = headers or {}
        self._client = httpx.Client(
            base_url=f"{self.base_url}/api/v1",
            headers={"Accept": "application/json", **self.headers},
            timeout=30.0,
        )

    def _request(
        self,
        method: str,
        path: str,
        *,
        body: Any = None,
        extra_headers: dict[str, str] | None = None,
    ) -> Any:
        headers = {**(extra_headers or {})}
        content = None
        json_body = None
        if body is not None:
            if headers.get("Content-Type", "").startswith("application/x-yaml"):
                content = body if isinstance(body, bytes) else body.encode("utf-8")
            else:
                headers.setdefault("Content-Type", "application/json")
                json_body = body

        response = self._client.request(
            method,
            path,
            content=content,
            json=json_body,
            headers=headers,
        )
        if not response.is_success:
            raise_for_status(response)
        if response.status_code == 204:
            return None
        return response.json()

    def _get(self, path: str) -> Any:
        return self._request("GET", path)

    def _post(self, path: str, body: Any = None) -> Any:
        return self._request("POST", path, body=body)

    def _put(self, path: str, body: Any = None) -> Any:
        return self._request("PUT", path, body=body)

    def _delete(self, path: str) -> Any:
        return self._request("DELETE", path)

    @staticmethod
    def _query_string(params: dict[str, Any]) -> str:
        filtered = {k: v for k, v in params.items() if v is not None and v != ""}
        if not filtered:
            return ""
        return f"?{urlencode(filtered)}"

    def close(self) -> None:
        self._client.close()
