"""``python -m hnsx_worker`` entrypoint.

Commands:

  - ``hnsx-worker --version``                — print package version
  - ``hnsx-worker check-proto``              — import proto stubs and verify contract
  - ``hnsx-worker run [OPTIONS]``            — start the worker parent process
"""

from __future__ import annotations

import logging
import sys

import click

from hnsx_worker import __version__
from hnsx_worker.logging import configure_logging


def _setup_logging(verbose: bool) -> None:
    configure_logging(level=logging.DEBUG if verbose else logging.INFO)


@click.group()
@click.version_option(__version__, prog_name="hnsx-worker")
def cli() -> None:
    """HnsX Python capability worker."""


@cli.command("check-proto")
def check_proto() -> None:
    """Import the generated proto stubs and verify the wire contract."""
    from hnsx_worker.proto.gen.hnsx.v1 import worker_pb2, worker_pb2_grpc

    file_descriptor = worker_pb2.DESCRIPTOR
    pkg = file_descriptor.package
    short_names = set(file_descriptor.services_by_name.keys())
    full_names = {f"{pkg}.{n}" for n in short_names}
    expected_short = {"WorkerService", "SchedulerService"}
    missing = expected_short - short_names
    if missing:
        click.echo(f"FAIL: missing services {sorted(missing)}", err=True)
        sys.exit(1)

    worker_pb2.RegisterRequest()
    worker_pb2.PullSessionRequest()
    _ = worker_pb2_grpc.WorkerServiceServicer
    _ = worker_pb2_grpc.SchedulerServiceServicer

    click.echo(f"hnsx-worker {__version__} — services: {sorted(full_names)}")
    click.echo("proto stubs OK")


@cli.command("run")
@click.option("--server", default="127.0.0.1:50061", help="hnsx-server gRPC address (host:port).")
@click.option("--worker-id", default="", help="Stable worker id (empty = server-assigned).")
@click.option("--region", default="local", help="Free-form region tag.")
@click.option(
    "--max-concurrent-sessions",
    default=4,
    type=int,
    help="Max subprocesses this worker runs in parallel.",
)
@click.option(
    "--providers",
    default="anthropic,openai,claudecode,codex,ollama,noop,echo",
    help="Comma-separated provider kinds this worker supports.",
)
@click.option(
    "--models", default="", help="Comma-separated model names this worker supports."
)
@click.option(
    "--sandbox-runtimes",
    default="none",
    help="Comma-separated sandbox runtimes this worker supports.",
)
@click.option("--heartbeat-interval", default=5, type=int, help="Heartbeat cadence in seconds.")
@click.option("-v", "--verbose", is_flag=True, help="Enable debug logging.")
def run(
    server: str,
    worker_id: str,
    region: str,
    max_concurrent_sessions: int,
    providers: str,
    models: str,
    sandbox_runtimes: str,
    heartbeat_interval: int,
    verbose: bool,
) -> None:
    """Start the worker parent process and block until interrupted."""
    _setup_logging(verbose)
    from hnsx_worker.config import WorkerConfig
    from hnsx_worker.worker_service import WorkerService

    config = WorkerConfig.from_cli(
        server=server,
        worker_id=worker_id,
        region=region,
        max_concurrent_sessions=max_concurrent_sessions,
        providers=providers,
        models=models,
        sandbox_runtimes=sandbox_runtimes,
        heartbeat_interval=heartbeat_interval,
    )
    WorkerService(config).run()


if __name__ == "__main__":
    sys.exit(cli(obj={}))
