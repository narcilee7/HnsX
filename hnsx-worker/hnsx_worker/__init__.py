"""HnsX Python capability execution plane.

This package contains the worker (parent process) and the session runtime
(child subprocess). The wire contract with the Go control plane lives in
``hnsx_worker.proto.gen.hnsx.v1`` (generated from ``proto/hnsx/v1/*.proto``).

Step 1 of the V1.1 Python Worker Pivot ships only the proto stubs and
scaffolding; actual worker logic arrives in Steps 2 and 3.
"""

from hnsx_worker.version import __version__

__all__ = ["__version__"]