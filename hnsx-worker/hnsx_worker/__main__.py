"""``python -m hnsx_worker`` entrypoint.

Step 1: only ``--version`` and ``check-proto`` are wired up. The real
``run`` / ``register`` / ``drain`` subcommands land in Step 2.
"""

from __future__ import annotations

import sys

import click

from hnsx_worker import __version__


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

    # Touch a type from each service to ensure the import path is fully wired.
    worker_pb2.RegisterRequest()
    worker_pb2.PullSessionRequest()
    _ = worker_pb2_grpc.WorkerServiceServicer
    _ = worker_pb2_grpc.SchedulerServiceServicer

    click.echo(f"hnsx-worker {__version__} — services: {sorted(full_names)}")
    click.echo("proto stubs OK")


if __name__ == "__main__":
    sys.exit(cli(obj={}))