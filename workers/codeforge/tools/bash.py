"""Built-in tool: execute a bash command."""

from __future__ import annotations

import asyncio
import logging
from typing import Any

from codeforge.constants import MAX_OUTPUT_CHARS
from codeforge.tools._base import ToolDefinition, ToolExample, ToolExecutor, ToolResult

logger = logging.getLogger(__name__)

MAX_OUTPUT = MAX_OUTPUT_CHARS
HALF_OUTPUT = MAX_OUTPUT // 2

DEFINITION = ToolDefinition(
    name="bash",
    description="Execute a bash command and return stdout and stderr. Runs in the workspace directory.",
    parameters={
        "type": "object",
        "properties": {
            "command": {
                "type": "string",
                "description": "The bash command to execute.",
            },
            "timeout": {
                "type": "integer",
                "description": "Timeout in seconds (default 120).",
            },
        },
        "required": ["command"],
    },
    when_to_use=(
        "Use for running tests, installing dependencies, git operations, compiling code, "
        "or any task that requires shell access. Prefer dedicated tools (read_file, search_files) "
        "over bash equivalents (cat, grep) when possible."
    ),
    output_format="stdout followed by stderr (prefixed with '--- stderr ---') if any. Exit code reported on failure.",
    common_mistakes=[
        "Running interactive commands that wait for input (use non-interactive flags)",
        "Forgetting timeout for long-running commands — set timeout explicitly",
        "Using bash for file reading/searching when read_file or search_files would be better",
    ],
    examples=[
        ToolExample(
            description="Run Python tests",
            tool_call_json='{"command": "python -m pytest tests/ -v", "timeout": 60}',
            expected_result="===== 5 passed in 2.3s =====",
        ),
        ToolExample(
            description="Check git status",
            tool_call_json='{"command": "git status --short"}',
            expected_result="M  src/main.py\\n?? src/new_file.py",
        ),
    ],
)


def _truncate(text: str) -> str:
    """Truncate output that exceeds MAX_OUTPUT, keeping head and tail."""
    if len(text) <= MAX_OUTPUT:
        return text
    return text[:HALF_OUTPUT] + "\n\n... truncated ...\n\n" + text[-HALF_OUTPUT:]


def _check_dangerous_command(command: str) -> str | None:
    """Defense-in-depth check for dangerous commands.

    SECONDARY DEFENSE ONLY -- trivially bypassable via shell expansion,
    flag reordering, or interpreter wrapping. The Go policy engine
    (internal/service/policy.go) is the PRIMARY security boundary.
    This blocklist catches only the most obvious destructive commands
    as a defense-in-depth measure.

    Returns an error message if the command is blocked, None otherwise.
    """
    # Normalize for matching: strip leading whitespace, lowercase.
    normalized = command.strip().lower()

    # Patterns that are dangerous regardless of context.
    blocked_patterns: list[tuple[str, str]] = [
        ("rm -rf /", "recursive deletion of root filesystem"),
        ("rm -rf /*", "recursive deletion of root filesystem"),
        ("rm -fr /", "recursive deletion of root filesystem"),
        ("rm -fr /*", "recursive deletion of root filesystem"),
        ("mkfs.", "filesystem formatting"),
        ("dd if=", "raw disk write"),
        (":(){:|:&};:", "fork bomb"),
        ("> /dev/sda", "raw disk overwrite"),
        ("chmod -r 777 /", "recursive permission change on root"),
        ("chown -r ", "recursive ownership change"),
        ("shutdown", "system shutdown"),
        ("reboot", "system reboot"),
        ("init 0", "system halt"),
        ("init 6", "system reboot"),
    ]

    for pattern, reason in blocked_patterns:
        if pattern in normalized:
            return f"blocked by safety filter: {reason} ({pattern!r})"

    return None


class BashTool(ToolExecutor):
    """Execute a bash command with timeout."""

    async def execute(self, arguments: dict[str, Any], workspace_path: str) -> ToolResult:
        command = arguments.get("command", "")
        timeout = arguments.get("timeout", 120)

        # Defense-in-depth: block catastrophic commands before execution.
        if block_reason := _check_dangerous_command(command):
            return ToolResult(output="", error=block_reason, success=False)

        try:
            proc = await asyncio.create_subprocess_exec(
                "bash",
                "-c",
                command,
                stdout=asyncio.subprocess.PIPE,
                stderr=asyncio.subprocess.PIPE,
                cwd=workspace_path,
            )
        except OSError as exc:
            return ToolResult(output="", error=str(exc), success=False)

        try:
            stdout_bytes, stderr_bytes = await asyncio.wait_for(proc.communicate(), timeout=timeout)
        except TimeoutError:
            proc.kill()
            await proc.wait()
            return ToolResult(output="", error=f"command timed out after {timeout}s", success=False)

        stdout = _truncate(stdout_bytes.decode("utf-8", errors="replace"))
        stderr = _truncate(stderr_bytes.decode("utf-8", errors="replace"))

        success = proc.returncode == 0
        output = stdout
        if stderr:
            output = f"{stdout}\n--- stderr ---\n{stderr}" if stdout else stderr

        return ToolResult(output=output, error="" if success else f"exit code {proc.returncode}", success=success)
