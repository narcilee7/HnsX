"""Worker runtime configuration.

`WorkerConfig` is the immutable configuration that drives one Python worker
process. It can be constructed from CLI flags (``WorkerConfig.from_cli``) or
programmatically (used by tests).
"""

from __future__ import annotations

from dataclasses import dataclass, field

from hnsx_worker.proto_client import ResourceCapacity


@dataclass
class WorkerConfig:
    """Configuration for one Python worker process.

    Attributes:
        server_addr: gRPC server address (host:port) the worker connects to.
        worker_id: Stable worker identifier. Empty means "let server assign".
        region: Free-form region tag (e.g. "local", "us-west-2").
        capacity: ResourceCapacity advertised to the server at Register time.
        heartbeat_interval_seconds: Seconds between Heartbeat RPCs.
        auth_token: Bearer token forwarded on Register (V1.1 unused; reserved).
    """

    server_addr: str = "127.0.0.1:50061"
    worker_id: str = ""
    region: str = "local"
    capacity: ResourceCapacity = field(default_factory=ResourceCapacity)
    heartbeat_interval_seconds: int = 5
    auth_token: str = ""
    pool_size: int = 0  # W7: number of pre-forked session_runtime processes

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
    ) -> WorkerConfig:
        capacity = ResourceCapacity(
            max_concurrent_sessions=max_concurrent_sessions,
            providers=_split_csv(providers),
            models=_split_csv(models),
        )
        return cls(
            server_addr=server,
            worker_id=worker_id,
            region=region,
            capacity=capacity,
            heartbeat_interval_seconds=heartbeat_interval,
        )


def _split_csv(s: str) -> list[str]:
    return [item.strip() for item in s.split(",") if item.strip()]
