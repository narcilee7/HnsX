"""Smoke tests for Step 1: generated proto stubs import cleanly and the
wire contract exposes the expected services and messages."""

from __future__ import annotations


def test_worker_pb2_imports() -> None:
    from hnsx_worker.proto.gen.hnsx.v1 import worker_pb2

    req = worker_pb2.RegisterRequest()
    # RegisterRequest is a plain message; auth_token is the only required-looking
    # scalar at the top level. Touch it to ensure the field is wired.
    assert req.auth_token == ""  # type: ignore[attr-defined]
    info = worker_pb2.WorkerInfo()
    assert info.worker_id == ""  # type: ignore[attr-defined]


def test_worker_grpc_imports() -> None:
    from hnsx_worker.proto.gen.hnsx.v1 import worker_pb2

    services = worker_pb2.DESCRIPTOR.services_by_name
    assert "WorkerService" in services
    assert "SchedulerService" in services
    assert len(services) == 2


def test_observation_reuse() -> None:
    """Confirm Observation from observation.proto is reachable via worker.proto's import."""
    from hnsx_worker.proto.gen.hnsx.v1 import worker_pb2

    batch = worker_pb2.ObservationBatch()
    obs = batch.observations.add()
    assert obs.observation_id == ""  # type: ignore[attr-defined]


def test_worker_info_fields() -> None:
    """Sanity-check the ResourceCapacity + WorkerInfo fields wire-up."""
    from hnsx_worker.proto.gen.hnsx.v1 import worker_pb2

    info = worker_pb2.WorkerInfo(
        worker_id="w-1",  # type: ignore[arg-type]
        tenant_id="t-1",  # type: ignore[arg-type]
        version="0.1.0",  # type: ignore[arg-type]
        region="local",  # type: ignore[arg-type]
    )
    info.capacity.providers.append("anthropic")  # type: ignore[attr-defined]
    info.capacity.providers.append("openai")  # type: ignore[attr-defined]
    info.labels["gpu"] = "none"  # type: ignore[index]

    assert info.worker_id == "w-1"  # type: ignore[attr-defined]
    assert list(info.capacity.providers) == ["anthropic", "openai"]  # type: ignore[attr-defined]
    assert info.labels["gpu"] == "none"  # type: ignore[index]


def test_stream_channel_event_shapes() -> None:
    """Verify the bidi StreamChannel payload types are constructible."""
    from hnsx_worker.proto.gen.hnsx.v1 import worker_pb2

    up = worker_pb2.StreamChannelRequest(worker_id="w-1")  # type: ignore[arg-type]
    batch = worker_pb2.ObservationBatch()
    up.observations.CopyFrom(batch)
    # StreamChannelRequest has a `payload` oneof; ask which case is set.
    assert up.WhichOneof("payload") == "observations"

    down = worker_pb2.StreamChannelResponse()
    down.cancel.session_id = "s-1"  # type: ignore[attr-defined]
    assert down.WhichOneof("payload") == "cancel"


def test_worker_health_enum_prefix() -> None:
    """The STANDARD lint rule requires enum values to be prefixed with the enum name."""
    from hnsx_worker.proto.gen.hnsx.v1 import worker_pb2

    assert worker_pb2.WorkerHealth.Status.Value("STATUS_HEALTHY") == 1  # type: ignore[attr-defined]
    assert worker_pb2.WorkerHealth.Status.Value("STATUS_DEGRADED") == 2  # type: ignore[attr-defined]
    assert worker_pb2.WorkerHealth.Status.Value("STATUS_DRAINING") == 3  # type: ignore[attr-defined]