"""Built-in tool registry for agent tool calling.

Provides a ToolRegistry that holds built-in tools and can merge MCP-discovered
tools under the ``mcp__{server}__{tool}`` namespace.
"""

from __future__ import annotations

import logging
from typing import TYPE_CHECKING, Any

from codeforge.tools._base import ToolDefinition, ToolExecutor, ToolResult

if TYPE_CHECKING:
    from codeforge.mcp_workbench import McpWorkbench

logger = logging.getLogger(__name__)

__all__ = [
    "ToolDefinition",
    "ToolExecutor",
    "ToolRegistry",
    "ToolResult",
    "build_default_registry",
]


class ToolRegistry:
    """Container for tool definitions and their executors."""

    def __init__(self) -> None:
        self._tools: dict[str, tuple[ToolDefinition, ToolExecutor]] = {}

    def register(self, definition: ToolDefinition, executor: ToolExecutor) -> None:
        """Register a tool definition with its executor."""
        self._tools[definition.name] = (definition, executor)

    def get_openai_tools(self) -> list[dict[str, Any]]:
        """Return all tool definitions in OpenAI function-calling format."""
        return [
            {
                "type": "function",
                "function": {
                    "name": defn.name,
                    "description": defn.description,
                    "parameters": defn.parameters,
                },
            }
            for defn, _ in self._tools.values()
        ]

    async def execute(self, name: str, arguments: dict[str, Any], workspace_path: str) -> ToolResult:
        """Execute a tool by name. Returns error result if tool is unknown."""
        entry = self._tools.get(name)
        if entry is None:
            return ToolResult(output="", error=f"unknown tool: {name}", success=False)
        _, executor = entry
        return await executor.execute(arguments, workspace_path)

    def merge_mcp_tools(self, workbench: McpWorkbench) -> None:
        """Merge MCP-discovered tools into the registry.

        Each MCP tool is registered as ``mcp__{server_id}__{tool_name}`` with
        an executor that delegates to the workbench.
        """
        for tool_def in workbench.get_tools_for_llm():
            func = tool_def["function"]
            name = str(func["name"])
            parts = name.split("__", 2)
            # Expected format: mcp__{server}__{tool}
            if len(parts) != 3:
                continue
            server_id = parts[1]
            tool_name = parts[2]

            definition = ToolDefinition(
                name=name,
                description=str(func.get("description", "")),
                parameters=func.get("parameters", {}),
            )
            executor = _McpToolProxy(workbench, server_id, tool_name)
            self.register(definition, executor)

    @property
    def tool_names(self) -> list[str]:
        """Return sorted list of registered tool names."""
        return sorted(self._tools.keys())


class _McpToolProxy:
    """Executor proxy that delegates to an MCP workbench."""

    def __init__(self, workbench: McpWorkbench, server_id: str, tool_name: str) -> None:
        self._workbench = workbench
        self._server_id = server_id
        self._tool_name = tool_name

    async def execute(self, arguments: dict[str, Any], workspace_path: str) -> ToolResult:
        result = await self._workbench.call_tool(self._server_id, self._tool_name, arguments)
        return ToolResult(
            output=result.output,
            error=result.error,
            success=result.success,
        )


def build_default_registry() -> ToolRegistry:
    """Create a ToolRegistry with all built-in tools registered."""
    from codeforge.tools import bash, edit_file, glob_files, list_directory, read_file, search_files, write_file

    registry = ToolRegistry()
    registry.register(read_file.DEFINITION, read_file.ReadFileTool())
    registry.register(write_file.DEFINITION, write_file.WriteFileTool())
    registry.register(edit_file.DEFINITION, edit_file.EditFileTool())
    registry.register(bash.DEFINITION, bash.BashTool())
    registry.register(search_files.DEFINITION, search_files.SearchFilesTool())
    registry.register(glob_files.DEFINITION, glob_files.GlobFilesTool())
    registry.register(list_directory.DEFINITION, list_directory.ListDirectoryTool())
    return registry
