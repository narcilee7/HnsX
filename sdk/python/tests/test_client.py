import httpx
import pytest

from hnsx import APIError, DomainSpecBuilder, HnsXClient


@pytest.fixture
def client():
    return HnsXClient("http://localhost:50052")


def test_list_domains(httpx_mock, client):
    httpx_mock.add_response(
        url="http://localhost:50052/api/v1/domains",
        json={"items": [{"id": "customer-service", "version": "1.0.0", "status": "active"}], "total": 1},
    )
    result = client.domains.list()
    assert result.total == 1
    assert result.items[0]["id"] == "customer-service"


def test_get_domain(httpx_mock, client):
    httpx_mock.add_response(
        url="http://localhost:50052/api/v1/domains/customer-service",
        json={"id": "customer-service", "version": "1.0.0", "status": "active"},
    )
    domain = client.domains.get("customer-service")
    assert domain.id == "customer-service"


def test_register_domain_yaml(httpx_mock, client):
    def callback(request):
        assert "yaml" in request.headers.get("content-type")
        return httpx.Response(200, json={"id": "customer-service", "version": "1.0.0"})

    httpx_mock.add_callback(callback, url="http://localhost:50052/api/v1/domains")
    domain = client.domains.register_yaml("id: customer-service\nversion: '1.0.0'\n")
    assert domain.id == "customer-service"


def test_trigger_session(httpx_mock, client):
    httpx_mock.add_response(
        url="http://localhost:50052/api/v1/sessions",
        method="POST",
        json={"id": "sess-123", "state": "running", "domain_id": "customer-service"},
    )
    session = client.sessions.trigger(domain_id="customer-service", trigger={"question": "hi"})
    assert session.id == "sess-123"


def test_list_sessions_with_params(httpx_mock, client):
    httpx_mock.add_response(
        url="http://localhost:50052/api/v1/sessions?domain=customer-service&limit=10",
        json={"items": [{"id": "s1", "state": "completed"}], "total": 1},
    )
    result = client.sessions.list(domain="customer-service", limit=10)
    assert result.items[0]["id"] == "s1"


def test_approve_approval(httpx_mock, client):
    httpx_mock.add_response(
        url="http://localhost:50052/api/v1/approvals/approve-1/approve",
        method="POST",
        json={"id": "approve-1", "status": "approved"},
    )
    approval = client.approvals.approve("approve-1")
    assert approval.status == "approved"


def test_api_error(httpx_mock, client):
    httpx_mock.add_response(
        url="http://localhost:50052/api/v1/domains/missing",
        status_code=404,
        json={"code": "DOMAIN_NOT_FOUND", "message": "not found"},
    )
    with pytest.raises(APIError) as exc_info:
        client.domains.get("missing")
    assert exc_info.value.code == "DOMAIN_NOT_FOUND"
    assert exc_info.value.status == 404


def test_domain_spec_builder():
    spec = (
        DomainSpecBuilder("my-agent")
        .with_agent("assistant", provider="openai", model="gpt-4o-mini", system_prompt="default")
        .with_prompt("default", "You are helpful.")
        .with_session_mode("single", agent="assistant")
        .with_policy(max_cost_usd=1.0, max_turns=20, presets=["safe_customer_service"])
        .build()
    )
    assert spec["id"] == "my-agent"
    assert "assistant" in spec["harness"]["agents"]
    assert spec["harness"]["policy"]["presets"] == ["safe_customer_service"]
