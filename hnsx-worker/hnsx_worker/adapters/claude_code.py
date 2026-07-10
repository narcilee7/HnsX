"""Claude Code CLI adapter.

Runs the ``claude`` CLI as a subprocess, streams its stdout as text deltas,
and heuristically detects shell/file/edit operations so the Harness can audit
and optionally deny them via Policy.

Agent spec example::

    adapter:
      kind: claudecode
      config:
        timeout_seconds: 120
        max_output_bytes: 256000
        workdir: "."
        api_key_env: ANTHROPIC_API_KEY
"""

from __future__ import annotations

import re

from .cli_base import CliAdapter, CliToolPattern


class ClaudeCodeAdapter(CliAdapter):
    """Adapter for the Anthropic ``claude`` CLI."""

    def name(self) -> str:
        return "claudecode"

    def _cli_binary(self) -> str:
        return "claude"

    def _tool_patterns(self) -> list[CliToolPattern]:
        """Detect operations the CLI commonly reports.

        These patterns are best-effort: CLI output is not a stable API. They
        capture enough information for the audit / policy layer to act on.
        """
        return [
            CliToolPattern(
                "bash",
                re.compile(
                    r"(?:\$\s+|Running:| Executing:\s*)(?P<command>[^\n]+)",
                    re.IGNORECASE,
                ),
            ),
            CliToolPattern(
                "file_read",
                re.compile(
                    r"(?:Reading|Read|Viewing)\s+file:?\s+(?P<path>\S+)",
                    re.IGNORECASE,
                ),
            ),
            CliToolPattern(
                "file_write",
                re.compile(
                    r"(?:Writing|Wrote|Creating|Created)\s+(?:file:?\s+)?(?P<path>\S+)",
                    re.IGNORECASE,
                ),
            ),
            CliToolPattern(
                "file_delete",
                re.compile(
                    r"(?:Deleting|Deleted|Removing|Removed)\s+(?:file:?\s+)?(?P<path>\S+)",
                    re.IGNORECASE,
                ),
            ),
            CliToolPattern(
                "edit",
                re.compile(
                    r"(?:Editing|Edited|Applying\s+edit)\s+(?:to\s+)?(?P<path>\S+)",
                    re.IGNORECASE,
                ),
            ),
        ]


__all__ = ["ClaudeCodeAdapter"]
