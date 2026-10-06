"""Selects the tools a turn is offered.

Every registered tool is offered: the registry already holds only the tools
the agent mode allows (ToolRegistry.restrict_to_mode) and the tools the turn
wired (MCP servers, skills, conversation search, handoff). The planning
tools (propose_goal, propose_roadmap) are offered in planning turns only
(KI-153). Before, a keyword match on the user message decided, and MCP,
skill and conversation-search tools were never offered (KI-192).
"""

from __future__ import annotations

from typing import ClassVar


class ToolRouter:
    """Per-turn tool selection for the agentic loop.

    Usage::

        router = ToolRouter(all_tool_names=registry.tool_names)
        selected = router.select(planning=True)
    """

    # Planning tools, offered in planning turns only (KI-153): in an
    # implementation turn a weak model called them instead of writing code.
    PLANNING_TOOLS: ClassVar[frozenset[str]] = frozenset({"propose_goal", "propose_roadmap"})

    def __init__(self, all_tool_names: list[str]) -> None:
        self._all_tools = all_tool_names

    def select(self, *, planning: bool = False) -> list[str]:
        """Return the sorted registered tools, without the planning tools unless *planning*."""
        excluded = frozenset() if planning else self.PLANNING_TOOLS
        return sorted(set(self._all_tools) - excluded)
