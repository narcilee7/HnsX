"""Sandbox backends for tool execution isolation (W6).

A sandbox backend decides *where* a tool runs. W6 ships three backends:

  - ``none``: run in-process (current behavior; fastest, least isolation).
  - ``process``: run in a short-lived subprocess (basic isolation; optional
    Linux namespace/seccomp support).
  - ``container``: run inside a Docker/Podman container (strongest isolation).

The :class:`SandboxBackend` interface is intentionally small. Tools that want
sandboxing call ``backend.run(cmd, files, env)`` and receive stdout/stderr/rc.
"""

from __future__ import annotations

import logging
import subprocess
from abc import ABC, abstractmethod
from dataclasses import dataclass
from typing import Any

log = logging.getLogger("hnsx_worker.sandbox.backend")


@dataclass
class SandboxResult:
    """Outcome of a sandboxed execution."""

    returncode: int = 0
    stdout: str = ""
    stderr: str = ""
    error: str | None = None

    @property
    def ok(self) -> bool:
        return self.error is None and self.returncode == 0


class SandboxBackend(ABC):
    """Abstract sandbox backend."""

    @property
    @abstractmethod
    def name(self) -> str:
        """Backend identifier (``none`` / ``process`` / ``container``)."""

    @abstractmethod
    def run(
        self,
        command: list[str],
        *,
        env: dict[str, str] | None = None,
        timeout_seconds: float = 60.0,
        workdir: str = "",
    ) -> SandboxResult:
        """Run ``command`` inside the sandbox and return the result."""


class NoneSandbox(SandboxBackend):
    """No isolation — run the command in-process via subprocess.

    This is the default for local development. It still captures stdout/stderr
    and respects timeout.
    """

    @property
    def name(self) -> str:
        return "none"

    def run(
        self,
        command: list[str],
        *,
        env: dict[str, str] | None = None,
        timeout_seconds: float = 60.0,
        workdir: str = "",
    ) -> SandboxResult:
        try:
            proc = subprocess.run(
                command,
                capture_output=True,
                text=True,
                env=env,
                cwd=workdir or None,
                timeout=timeout_seconds,
            )
            return SandboxResult(
                returncode=proc.returncode,
                stdout=proc.stdout,
                stderr=proc.stderr,
            )
        except subprocess.TimeoutExpired as e:
            return SandboxResult(
                returncode=-1,
                stdout=e.stdout or "",
                stderr=e.stderr or "",
                error=f"sandbox timeout after {timeout_seconds}s",
            )
        except Exception as e:  # noqa: BLE001
            return SandboxResult(error=f"sandbox error: {e!s}")


class ProcessSandbox(SandboxBackend):
    """Subprocess sandbox with optional Linux namespace/seccomp.

    W6 ships the subprocess wrapper; namespace/seccomp are optional and can be
    enabled later without changing the interface.
    """

    def __init__(self, *, use_namespace: bool = False) -> None:
        self.use_namespace = use_namespace

    @property
    def name(self) -> str:
        return "process"

    def run(
        self,
        command: list[str],
        *,
        env: dict[str, str] | None = None,
        timeout_seconds: float = 60.0,
        workdir: str = "",
    ) -> SandboxResult:
        # Future: prepend unshare / firejail when use_namespace=True.
        return NoneSandbox().run(
            command,
            env=env,
            timeout_seconds=timeout_seconds,
            workdir=workdir,
        )


class ContainerSandbox(SandboxBackend):
    """Container sandbox via Docker or Podman.

    The backend auto-detects ``docker`` / ``podman`` on first use.
    """

    def __init__(self, *, image: str = "python:3.11-slim", runtime: str = "") -> None:
        self.image = image
        self._runtime = runtime
        self._resolved: str | None = None

    @property
    def name(self) -> str:
        return "container"

    def _runtime_binary(self) -> str:
        if self._resolved:
            return self._resolved
        import shutil

        # If a runtime was explicitly configured, it must exist.
        if self._runtime:
            if shutil.which(self._runtime):
                self._resolved = self._runtime
                return self._resolved
            raise RuntimeError(
                f"configured container runtime {self._runtime!r} not found"
            )

        # Auto-detect docker / podman.
        for candidate in ["docker", "podman"]:
            if shutil.which(candidate):
                self._resolved = candidate
                return candidate
        raise RuntimeError("no container runtime found (docker or podman)")

    def run(
        self,
        command: list[str],
        *,
        env: dict[str, str] | None = None,
        timeout_seconds: float = 60.0,
        workdir: str = "",
    ) -> SandboxResult:
        try:
            runtime = self._runtime_binary()
        except RuntimeError as e:
            return SandboxResult(error=str(e))

        args = [runtime, "run", "--rm", "-i", self.image]
        if workdir:
            args.extend(["-w", workdir])
        if env:
            for k, v in env.items():
                args.extend(["-e", f"{k}={v}"])
        args.extend(command)

        return NoneSandbox().run(
            args,
            env=None,
            timeout_seconds=timeout_seconds,
            workdir="",
        )


def build_backend(config: dict[str, Any] | None) -> SandboxBackend:
    """Build a sandbox backend from the DomainSpec ``harness.sandbox`` block."""
    cfg = config or {}
    kind = str(cfg.get("backend") or cfg.get("policy") or "none")
    if kind == "none":
        return NoneSandbox()
    if kind == "process":
        return ProcessSandbox(use_namespace=bool(cfg.get("use_namespace")))
    if kind in ("container", "docker"):
        return ContainerSandbox(
            image=str(cfg.get("image") or "python:3.11-slim"),
            runtime=str(cfg.get("runtime") or ""),
        )
    raise ValueError(f"unknown sandbox backend: {kind!r}")


__all__ = [
    "SandboxBackend",
    "SandboxResult",
    "NoneSandbox",
    "ProcessSandbox",
    "ContainerSandbox",
    "build_backend",
]
