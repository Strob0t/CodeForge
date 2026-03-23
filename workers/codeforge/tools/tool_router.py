"""Pre-selects relevant tools based on user message keywords.

No LLM call needed. Uses keyword matching against tool names and descriptions.
Base tools (read/write/edit/bash/search/glob/listdir) always included.
MCP read-only tools included when docs-related keywords match.
"""

from __future__ import annotations

from typing import ClassVar


class ToolRouter:
    """Keyword-based tool pre-selection for the agentic loop.

    Reduces the tool list sent to the LLM by selecting only relevant tools
    based on the user's message content. This keeps the prompt smaller and
    avoids confusing weaker models with too many tool definitions.

    Usage::

        router = ToolRouter(all_tool_names=registry.tool_names)
        selected = router.select(user_message)
    """

    BASE_TOOLS: ClassVar[frozenset[str]] = frozenset(
        {
            "read_file",
            "write_file",
            "edit_file",
            "bash",
            "search_files",
            "glob_files",
            "list_directory",
            "propose_goal",
            "transition_to_act",
        }
    )

    # Keywords that trigger inclusion of documentation/search MCP tools.
    DOCS_KEYWORDS: ClassVar[frozenset[str]] = frozenset(
        {
            "docs",
            "documentation",
            "how to",
            "api",
            "reference",
            "example",
            "usage",
            "tutorial",
            "guide",
            "library",
        }
    )

    # MCP tool name fragments considered read-only and safe to auto-include.
    _MCP_READONLY_FRAGMENTS: ClassVar[frozenset[str]] = frozenset(
        {
            "search",
            "list",
            "find",
            "fetch",
        }
    )

    # Keywords that trigger inclusion of specific built-in tools.
    TOOL_KEYWORDS: ClassVar[dict[str, list[str]]] = {
        "test": ["bash"],
        "install": ["bash"],
        "search": ["search_files"],
        "find": ["search_files", "glob_files"],
        "create": ["write_file"],
        "modify": ["edit_file", "read_file"],
        "fix": ["edit_file", "read_file", "bash"],
        "run": ["bash"],
        "commit": ["bash"],
        "git": ["bash"],
    }

    def __init__(self, all_tool_names: list[str]) -> None:
        self._all_tools = all_tool_names

    def select(self, user_message: str, max_tools: int = 12) -> list[str]:
        """Select relevant tools for the given user message.

        Always includes BASE_TOOLS (intersected with available tools).
        Adds MCP read-only tools when docs-related keywords are detected.
        Adds keyword-triggered tools based on message content.

        Returns a sorted, deduplicated list capped at *max_tools*.
        """
        available = set(self._all_tools)
        selected: set[str] = set(self.BASE_TOOLS & available)

        if not user_message:
            return sorted(selected)

        msg_lower = user_message.lower()

        # Add MCP read-only tools if docs-related keywords found.
        if any(kw in msg_lower for kw in self.DOCS_KEYWORDS):
            for tool in self._all_tools:
                if tool.startswith("mcp__") and any(frag in tool for frag in self._MCP_READONLY_FRAGMENTS):
                    selected.add(tool)

        # Add tools matching keyword triggers.
        for keyword, tools in self.TOOL_KEYWORDS.items():
            if keyword in msg_lower:
                for t in tools:
                    if t in available:
                        selected.add(t)

        return sorted(selected)[:max_tools]
