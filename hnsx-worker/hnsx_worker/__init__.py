"""HnsX Python capability execution plane.

This package contains the worker (parent process) and the session runtime
(child subprocess). The wire contract with the Go control plane lives in
``hnsx_worker.proto.gen.hnsx.v1`` (generated from ``proto/hnsx/v1/*.proto``).
"""

from hnsx_worker.config import WorkerConfig
from hnsx_worker.version import __version__
from hnsx_worker.worker_service import WorkerService

__all__ = ["__version__", "WorkerConfig", "WorkerService"]
