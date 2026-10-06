"""Selects the tools a turn is offered.

Every registered tool is offered: the registry already holds only the tools
the agent mode allows (ToolRegistry.restrict_to_mode) and the tools the turn
wired (MCP servers, skills, conversation search, handoff). The planning
tools (propose_goal, propose_roadmap) are offered in planning turns only
(KI-153). Before, a keyword match on the user message decided, and MCP,
skill and conversation-search tools were never offered (KI-192). MCP tools
are capped at MAX_MCP_TOOLS_PER_TURN (in name order); built-in tools never
are.
"""

from __future__ import annotations

import logging
from typing import ClassVar

from codeforge.constants import MAX_MCP_TOOLS_PER_TURN

logger = logging.getLogger(__name__)


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
        """Return the sorted registered tools, without the planning tools unless *planning*.

        Of the MCP tools, the first MAX_MCP_TOOLS_PER_TURN by name are offered.
        """
        excluded = frozenset() if planning else self.PLANNING_TOOLS
        names = sorted(set(self._all_tools) - excluded)
        mcp = [n for n in names if n.startswith("mcp__")]
        if len(mcp) <= MAX_MCP_TOOLS_PER_TURN:
            return names
        dropped = frozenset(mcp[MAX_MCP_TOOLS_PER_TURN:])
        logger.warning(
            "offering %d of %d MCP tools (%d not offered): the turn's limit is %d",
            MAX_MCP_TOOLS_PER_TURN,
            len(mcp),
            len(dropped),
            MAX_MCP_TOOLS_PER_TURN,
        )
        return [n for n in names if n not in dropped]
