"""Worker runtime configuration.

`WorkerConfig` is the immutable configuration that drives one Python worker
process. It can be constructed from CLI flags (``WorkerConfig.from_cli``) or
programmatically (used by tests).
"""

from __future__ import annotations

from dataclasses import dataclass, field

from hnsx_worker.proto.gen.hnsx.v1 import worker_pb2


@dataclass
class WorkerConfig:
    """Configuration for one Python worker process.

    Attributes:
        server_addr: gRPC server address (host:port) the worker connects to.
        worker_id: Stable worker identifier. Empty means "let server assign".
        region: Free-form region tag (e.g. "local", "us-west-2").
        capacity: ResourceCapacity proto (see proto/hnsx/v1/worker.proto).
        heartbeat_interval_seconds: Seconds between Heartbeat RPCs.
        auth_token: Bearer token forwarded on Register (V1.1 unused; reserved).
    """

    server_addr: str = "127.0.0.1:50051"
    worker_id: str = ""
    region: str = "local"
    capacity: worker_pb2.ResourceCapacity = field(
        default_factory=worker_pb2.ResourceCapacity
    )
    heartbeat_interval_seconds: int = 5
    auth_token: str = ""

    @classmethod
    def from_cli(
        cls,
        *,
        server: str,
        worker_id: str,
        region: str,
        max_concurrent_sessions: int,
        providers: str,
        models: str,
        heartbeat_interval: int,
    ) -> "WorkerConfig":
        capacity = worker_pb2.ResourceCapacity(
            max_concurrent_sessions=max_concurrent_sessions,
        )
        for p in _split_csv(providers):
            capacity.providers.append(p)  # type: ignore[attr-defined]
        for m in _split_csv(models):
            capacity.models.append(m)  # type: ignore[attr-defined]
        return cls(
            server_addr=server,
            worker_id=worker_id,
            region=region,
            capacity=capacity,
            heartbeat_interval_seconds=heartbeat_interval,
        )


def _split_csv(s: str) -> list[str]:
    return [item.strip() for item in s.split(",") if item.strip()]
