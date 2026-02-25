"""Built-in tool: execute a bash command."""

from __future__ import annotations

import asyncio
import logging
from typing import Any

from codeforge.tools._base import ToolDefinition, ToolExecutor, ToolResult

logger = logging.getLogger(__name__)

MAX_OUTPUT = 50_000
HALF_OUTPUT = MAX_OUTPUT // 2

DEFINITION = ToolDefinition(
    name="bash",
    description="Execute a bash command and return stdout and stderr.",
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
)


def _truncate(text: str) -> str:
    """Truncate output that exceeds MAX_OUTPUT, keeping head and tail."""
    if len(text) <= MAX_OUTPUT:
        return text
    return text[:HALF_OUTPUT] + "\n\n... truncated ...\n\n" + text[-HALF_OUTPUT:]


class BashTool(ToolExecutor):
    """Execute a bash command with timeout."""

    async def execute(self, arguments: dict[str, Any], workspace_path: str) -> ToolResult:
        command = arguments.get("command", "")
        timeout = arguments.get("timeout", 120)

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
