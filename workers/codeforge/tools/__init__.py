"""Built-in tool registry for agent tool calling.

Provides a ToolRegistry that holds built-in tools and can merge MCP-discovered
tools under the ``mcp__{server}__{tool}`` namespace.
"""

from __future__ import annotations

import logging
from typing import TYPE_CHECKING, Any

from codeforge.policy_args import mode_allows_tool
from codeforge.tools._base import ToolDefinition, ToolExample, ToolExecutor, ToolResult

if TYPE_CHECKING:
    from collections.abc import Iterator, Sequence

    from codeforge.mcp_workbench import McpWorkbench

logger = logging.getLogger(__name__)

__all__ = [
    "ToolDefinition",
    "ToolExample",
    "ToolExecutor",
    "ToolRegistry",
    "ToolResult",
    "build_default_registry",
]


class ToolRegistry:
    """Container for tool definitions and their executors."""

    def __init__(self) -> None:
        self._tools: dict[str, tuple[ToolDefinition, ToolExecutor]] = {}
        self._mode_tools: tuple[str, ...] = ()
        self._mode_denied: tuple[str, ...] = ()

    def register(self, definition: ToolDefinition, executor: ToolExecutor) -> None:
        """Register a tool definition with its executor, unless the agent mode forbids the tool."""
        if not mode_allows_tool(definition.name, self._mode_tools, self._mode_denied):
            logger.debug("tool %s not registered: forbidden by the agent mode", definition.name)
            return
        self._tools[definition.name] = (definition, executor)

    def restrict_to_mode(self, tools: Sequence[str], denied: Sequence[str]) -> None:
        """Offer only the tools the agent mode may use, now and for tools registered later.

        The Go policy denies every call to a tool the mode denies or, for a
        built-in tool, does not list (canonical names); offering such a tool
        to the LLM would only waste turns on calls that are denied.
        """
        self._mode_tools, self._mode_denied = tuple(tools), tuple(denied)
        for name in [n for n in self._tools if not mode_allows_tool(n, tools, denied)]:
            del self._tools[name]

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

    def get_definitions(self) -> list[ToolDefinition]:
        """Return all registered tool definitions (sorted by name)."""
        return [defn for defn, _ in sorted(self._tools.values(), key=lambda t: t[0].name)]

    def iter_executors(self) -> Iterator[tuple[ToolDefinition, ToolExecutor]]:
        """Iterate over all (definition, executor) pairs."""
        yield from self._tools.values()


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


def build_default_registry(*, skill_tools: bool = True) -> ToolRegistry:
    """Create a ToolRegistry with all built-in tools registered.

    *skill_tools* False leaves out search_skills and create_skill, which only
    work once the conversation path wired them (wire_skill_tools).
    """
    from codeforge.tools import (
        bash,
        create_skill,
        edit_file,
        glob_files,
        list_directory,
        read_file,
        search_conversations,
        search_files,
        search_skills,
        write_file,
    )

    registry = ToolRegistry()
    registry.register(read_file.DEFINITION, read_file.ReadFileTool())
    registry.register(write_file.DEFINITION, write_file.WriteFileTool())
    registry.register(edit_file.DEFINITION, edit_file.EditFileTool())
    registry.register(bash.DEFINITION, bash.BashTool())
    registry.register(search_files.DEFINITION, search_files.SearchFilesTool())
    registry.register(search_conversations.DEFINITION, search_conversations.SearchConversationsTool())
    registry.register(glob_files.DEFINITION, glob_files.GlobFilesTool())
    registry.register(list_directory.DEFINITION, list_directory.ListDirectoryTool())
    if skill_tools:
        registry.register(search_skills.DEFINITION, search_skills.SearchSkillsTool())
        registry.register(create_skill.DEFINITION, create_skill.CreateSkillTool())
    return registry
