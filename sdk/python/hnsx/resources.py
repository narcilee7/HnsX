"""Resource clients for the HnsX Python SDK."""

from __future__ import annotations

from typing import Any

from hnsx.base import BaseClient
from hnsx.models import (
    Approval,
    Domain,
    DomainSummary,
    EvalCase,
    EvalRun,
    EvalSet,
    EvalSetSummary,
    ListEnvelope,
    Session,
    SessionSummary,
    Trace,
    TraceSummary,
)


def _unwrap_list(data: dict[str, Any]) -> ListEnvelope:
    return ListEnvelope(
        items=data.get("items", []),
        total=data.get("total", 0),
        limit=data.get("limit"),
        offset=data.get("offset"),
    )


class DomainRegistryClient(BaseClient):
    def list(self, *, limit: int | None = None, offset: int | None = None) -> ListEnvelope:
        data = self._get(f"/domains{self._query_string({'limit': limit, 'offset': offset})}")
        return _unwrap_list(data)

    def get(self, id: str) -> Domain:  # noqa: A002
        data = self._get(f"/domains/{id}")
        return Domain(**data)

    def get_yaml(self, id: str) -> str:  # noqa: A002
        response = self._client.get(f"/domains/{id}/yaml", headers={"Accept": "application/yaml"})
        if not response.is_success:
            from hnsx.errors import raise_for_status

            raise_for_status(response)
        return response.text

    def register_yaml(self, yaml: str) -> Domain:
        data = self._request(
            "POST",
            "/domains",
            body=yaml,
            extra_headers={"Content-Type": "application/x-yaml"},
        )
        return Domain(**data)

    def update_yaml(self, id: str, yaml: str) -> Domain:  # noqa: A002
        data = self._request(
            "PUT",
            f"/domains/{id}",
            body=yaml,
            extra_headers={"Content-Type": "application/x-yaml"},
        )
        return Domain(**data)

    def delete(self, id: str) -> None:  # noqa: A002
        self._delete(f"/domains/{id}")

    def validate_yaml(self, id: str, yaml: str) -> dict[str, Any]:  # noqa: A002
        return self._request(
            "POST",
            f"/domains/{id}/validate",
            body=yaml,
            extra_headers={"Content-Type": "application/x-yaml"},
        )


class SessionsClient(BaseClient):
    def list(
        self,
        *,
        domain: str | None = None,
        state: str | None = None,
        limit: int | None = None,
        offset: int | None = None,
    ) -> ListEnvelope:
        params = {"domain": domain, "state": state, "limit": limit, "offset": offset}
        data = self._get(f"/sessions{self._query_string(params)}")
        return _unwrap_list(data)

    def get(self, id: str) -> Session:  # noqa: A002
        data = self._get(f"/sessions/{id}")
        return Session(**data)

    def trigger(self, *, domain_id: str, trigger: dict[str, Any] | None = None) -> Session:
        data = self._post("/sessions", {"domain_id": domain_id, "trigger": trigger or {}})
        return Session(**data)

    def cancel(self, id: str) -> Session:  # noqa: A002
        data = self._post(f"/sessions/{id}/cancel")
        return Session(**data)

    def rerun(self, id: str) -> Session:  # noqa: A002
        data = self._post(f"/sessions/{id}/rerun")
        return Session(**data)


class TracesClient(BaseClient):
    def list(
        self,
        *,
        domain: str | None = None,
        session: str | None = None,
        agent: str | None = None,
        from_time: str | None = None,
        to_time: str | None = None,
        limit: int | None = None,
        offset: int | None = None,
    ) -> ListEnvelope:
        params = {
            "domain": domain,
            "session": session,
            "agent": agent,
            "from": from_time,
            "to": to_time,
            "limit": limit,
            "offset": offset,
        }
        data = self._get(f"/traces{self._query_string(params)}")
        return _unwrap_list(data)

    def get(self, trace_id: str) -> Trace:
        data = self._get(f"/traces/{trace_id}")
        return Trace(**data)


class ApprovalsClient(BaseClient):
    def list(
        self,
        *,
        domain: str | None = None,
        session: str | None = None,
        status: str | None = None,
    ) -> ListEnvelope:
        params = {"domain": domain, "session": session, "status": status}
        data = self._get(f"/approvals{self._query_string(params)}")
        return _unwrap_list(data)

    def get(self, id: str) -> Approval:  # noqa: A002
        data = self._get(f"/approvals/{id}")
        return Approval(**data)

    def approve(self, id: str) -> Approval:  # noqa: A002
        data = self._post(f"/approvals/{id}/approve")
        return Approval(**data)

    def reject(self, id: str) -> Approval:  # noqa: A002
        data = self._post(f"/approvals/{id}/reject")
        return Approval(**data)


class EvalsClient(BaseClient):
    def list_sets(self) -> ListEnvelope:
        data = self._get("/evals")
        return _unwrap_list(data)

    def get_set(self, id: str) -> EvalSet:  # noqa: A002
        data = self._get(f"/evals/{id}")
        return EvalSet(**data)

    def create_set(
        self,
        *,
        set_id: str,
        domain_id: str,
        description: str | None = None,
        cases: list[EvalCase],
    ) -> EvalSet:
        data = self._post(
            "/evals",
            {
                "set_id": set_id,
                "domain_id": domain_id,
                "description": description,
                "cases": cases,
            },
        )
        return EvalSet(**data)

    def update_set(
        self,
        id: str,
        *,
        description: str | None = None,
        cases: list[EvalCase],
    ) -> EvalSet:  # noqa: A002
        data = self._put(
            f"/evals/{id}",
            {"description": description, "cases": cases},
        )
        return EvalSet(**data)

    def delete_set(self, id: str) -> None:  # noqa: A002
        self._delete(f"/evals/{id}")

    def run_set(self, id: str) -> dict[str, Any]:  # noqa: A002
        return self._post(f"/evals/{id}/run")

    def list_runs(self, set_id: str) -> ListEnvelope:
        data = self._get(f"/evals/{set_id}/runs")
        return _unwrap_list(data)

    def get_run(self, set_id: str, run_id: str) -> EvalRun:
        data = self._get(f"/evals/{set_id}/runs/{run_id}")
        return EvalRun(**data)
