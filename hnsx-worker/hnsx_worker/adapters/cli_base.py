"""Base class for CLI-agent adapters (Claude Code / Codex).

CLI agents bring their own shell / file / edit primitives. The Tool layer does
not re-implement them; it only **constrains and audits** them. This module
provides the shared subprocess plumbing:

  - Spawn the configured CLI binary with a prompt / context.
  - Stream stdout line-by-line into :class:`StreamChunk` (text / tool_use).
  - Capture stderr for diagnostics.
  - Emit detected tool operations as ``tool_call`` observations via the
    standard adapter stream so the executor can route them through Policy.

The detection heuristics are intentionally simple regexes — CLI output is not
a stable API. W4 ships the minimal viable parser; later work can plug in
provider-specific JSON protocols if they become available.
"""

from __future__ import annotations

import logging
import os
import re
import shutil
import subprocess
from collections.abc import Iterator
from dataclasses import dataclass
from typing import Any

from hnsx_worker.adapters.base import Adapter, AdapterResult, Cost, StreamChunk, ToolCall

log = logging.getLogger("hnsx_worker.adapters.cli")

_DEFAULT_TIMEOUT_SECONDS = 120.0
_DEFAULT_MAX_OUTPUT_BYTES = 256_000


@dataclass(frozen=True)
class CliToolPattern:
    """A regex pattern that maps a CLI output line to a tool call.

    Attributes:
        tool_name: Name emitted in the ``tool_call`` observation.
        pattern: Compiled regex. Named groups become the tool input dict.
    """

    tool_name: str
    pattern: re.Pattern[str]


class CliAdapter(Adapter):
    """Run an external CLI agent and stream its output as observations.

    Subclasses define ``_cli_binary()`` (the executable name) and
    ``_tool_patterns()`` (the heuristics used to detect shell/file/edit
    operations). The runtime shares one adapter instance, but each
    ``invoke_stream`` call spawns a fresh subprocess so per-session state
    stays isolated.
    """

    def __init__(self) -> None:
        self._timeout_seconds = _DEFAULT_TIMEOUT_SECONDS
        self._max_output_bytes = _DEFAULT_MAX_OUTPUT_BYTES

    # ------------------------------------------------------------------ abstract

    def _cli_binary(self) -> str:
        """Return the executable name (e.g. ``'claude'`` or ``'codex'``)."""
        raise NotImplementedError

    def _build_command(
        self,
        agent: dict,
        prompt: str,
        input: dict,
    ) -> list[str]:
        """Build the subprocess argv.

        The default implementation runs ``<binary> -p <prompt>``. Subclasses
        can override to add flags like ``--cwd`` or read agent-specific config.
        """
        binary = self._resolve_binary(agent)
        return [binary, "-p", prompt]

    def _tool_patterns(self) -> list[CliToolPattern]:
        """Return regex patterns that detect tool operations in CLI output."""
        return []

    # ------------------------------------------------------------------ Adapter

    def invoke(self, agent: dict, prompt: str, input: dict) -> AdapterResult:
        """Non-streaming fallback: collect all chunks into a single result."""
        text_parts: list[str] = []
        tool_calls: list[ToolCall] = []
        cost: Cost | None = None
        for chunk in self.invoke_stream(agent, prompt, input):
            if chunk.text_delta:
                text_parts.append(chunk.text_delta)
            if chunk.tool_call is not None:
                tool_calls.append(chunk.tool_call)
            if chunk.cost is not None:
                cost = chunk.cost
        return AdapterResult(text="".join(text_parts), tool_calls=tool_calls, cost=cost)

    def invoke_stream(
        self,
        agent: dict,
        prompt: str,
        input: dict,
    ) -> Iterator[StreamChunk]:
        """Spawn the CLI and yield its output as stream chunks."""
        cfg = agent.get("adapter", {}).get("config") or {}
        self._timeout_seconds = float(cfg.get("timeout_seconds", _DEFAULT_TIMEOUT_SECONDS))
        self._max_output_bytes = int(
            cfg.get("max_output_bytes", _DEFAULT_MAX_OUTPUT_BYTES)
        )

        command = self._build_command(agent, prompt, input)
        workdir = cfg.get("workdir") or agent.get("workdir") or "."
        env = self._build_env(agent, cfg)

        log.debug("%s adapter: spawning %s in %s", self.name(), command, workdir)
        started = __import__("time").time()
        try:
            proc = subprocess.Popen(
                command,
                cwd=str(workdir),
                env=env,
                stdout=subprocess.PIPE,
                stderr=subprocess.PIPE,
                text=True,
                bufsize=1,
            )
        except FileNotFoundError as e:
            raise RuntimeError(
                f"{self.name()} adapter: CLI binary {command[0]!r} not found"
            ) from e

        text_so_far = ""
        try:
            assert proc.stdout is not None
            for line in proc.stdout:
                if len(text_so_far) + len(line) > self._max_output_bytes:
                    line = line[: max(0, self._max_output_bytes - len(text_so_far))]
                text_so_far += line
                yield StreamChunk(text_delta=line)

                # Heuristic tool detection on every line.
                stripped = line.rstrip("\n")
                for pat in self._tool_patterns():
                    match = pat.pattern.search(stripped)
                    if match:
                        tc_input: dict[str, Any] = {"raw": stripped}
                        tc_input.update(match.groupdict())
                        yield StreamChunk(
                            tool_call=ToolCall(
                                id=_make_tool_call_id(),
                                name=pat.tool_name,
                                input=tc_input,
                                raw_input=stripped,
                            )
                        )

            try:
                proc.wait(timeout=self._timeout_seconds)
            except subprocess.TimeoutExpired as e:
                proc.kill()
                raise RuntimeError(
                    f"{self.name()} adapter: subprocess timed out after "
                    f"{self._timeout_seconds}s"
                ) from e

            stderr = (proc.stderr.read() or "") if proc.stderr else ""
            if proc.returncode != 0:
                raise RuntimeError(
                    f"{self.name()} adapter: subprocess exited {proc.returncode}: "
                    f"{stderr[:512]}"
                )
        finally:
            if proc.poll() is None:
                proc.kill()
                proc.wait()

        elapsed_ms = int((__import__("time").time() - started) * 1000)
        yield StreamChunk(
            cost=Cost(prompt_tokens=0, completion_tokens=0, latency_ms=elapsed_ms)
        )

    # ------------------------------------------------------------------ helpers

    def _resolve_binary(self, agent: dict) -> str:
        """Return the full path to the CLI binary or just its name."""
        cfg = agent.get("adapter", {}).get("config") or {}
        binary = cfg.get("binary") or self._cli_binary()
        if os.path.isabs(binary) or "/" in binary:
            return binary
        resolved = shutil.which(binary)
        if resolved:
            return resolved
        return binary

    def _build_env(self, agent: dict, cfg: dict) -> dict[str, str]:
        """Build the subprocess environment.

        Inherits the worker's environment and injects any keys declared in
        ``agent.adapter.config.api_key_env``. The resolved value is forwarded
        to the CLI so it can authenticate directly with the provider.
        """
        env = dict(os.environ)
        api_key_env = cfg.get("api_key_env")
        if api_key_env:
            value = os.environ.get(str(api_key_env))
            if value:
                env[str(api_key_env)] = value
        return env


def _make_tool_call_id() -> str:
    """Return a short unique id for detected CLI tool calls."""
    import uuid

    return f"cli-{uuid.uuid4().hex[:8]}"
