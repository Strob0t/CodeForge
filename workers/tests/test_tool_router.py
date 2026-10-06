"""Tests for the ToolRouter: the tools a turn is offered (KI-192).

Before, a keyword match on the user message decided, and MCP tools,
create_skill, search_skills and search_conversations were never offered
("Please create a GitHub issue ..." was not offered mcp__gh__create_issue).
Now every registered tool is offered (the registry holds only what the
agent mode allows); the planning tools only in planning turns (KI-153).
"""

from __future__ import annotations

import pytest

from codeforge.agent_loop import AgentLoopExecutor
from codeforge.llm import RoutingResult
from codeforge.loop_config import build_loop_config
from codeforge.tools.capability import CapabilityLevel
from codeforge.tools.tool_router import ToolRouter

BUILTIN = ["read_file", "write_file", "edit_file", "bash", "search_files", "glob_files", "list_directory"]
WIRED = [
    "search_conversations",
    "search_skills",
    "create_skill",
    "handoff_to",
    "transition_to_act",
    "mcp__gh__create_issue",
    "mcp__gh__search_issues",
    "mcp__docs__scrape_docs",
]
PLANNING = ["propose_goal", "propose_roadmap"]


@pytest.fixture
def router() -> ToolRouter:
    return ToolRouter(all_tool_names=[*BUILTIN, *WIRED, *PLANNING])


@pytest.mark.parametrize("planning", [True, False])
def test_every_registered_tool_is_offered(router: ToolRouter, planning: bool) -> None:
    selected = set(router.select(planning=planning))
    assert selected >= {*BUILTIN, *WIRED}


def test_planning_tools_only_in_planning_turns(router: ToolRouter) -> None:
    assert set(PLANNING) <= set(router.select(planning=True))
    assert not set(PLANNING) & set(router.select(planning=False))


def test_only_registered_tools_are_selected() -> None:
    assert ToolRouter(all_tool_names=["read_file"]).select(planning=True) == ["read_file"]
    assert ToolRouter(all_tool_names=[]).select(planning=True) == []


def test_selection_is_sorted_and_deterministic(router: ToolRouter) -> None:
    a = router.select(planning=True)
    assert a == sorted(a)
    assert a == router.select(planning=True)


def _offered(names: list[str], level: CapabilityLevel, *, implementation_turn: bool = False) -> set[str]:
    cfg, _ = build_loop_config(
        primary_model="provider/model",
        capability_level=level,
        routing=RoutingResult(model="provider/model"),
        tool_names=names,
        fallback_models=[],
        max_steps=5,
        max_cost=1.0,
        mode_tools=frozenset(),
        implementation_turn=implementation_turn,
    )
    tools = [{"type": "function", "function": {"name": n, "description": n, "parameters": {}}} for n in names]
    result = AgentLoopExecutor._filter_tools_for_capability(
        tools, CapabilityLevel(cfg.capability_level), cfg.mode_tools or None, cfg.selected_tools
    )
    return {t["function"]["name"] for t in result}  # type: ignore[index]


@pytest.mark.parametrize("level", list(CapabilityLevel))
def test_the_loop_offers_mcp_skill_and_search_tools(level: CapabilityLevel) -> None:
    """The audit's repro: "Please create a GitHub issue ... and save it as a reusable skill"."""
    offered = _offered([*BUILTIN, *WIRED, *PLANNING], level)
    assert offered >= set(WIRED)


@pytest.mark.parametrize("level", list(CapabilityLevel))
def test_an_implementation_turn_still_gets_no_planning_tools(level: CapabilityLevel) -> None:
    offered = _offered([*BUILTIN, *WIRED, *PLANNING], level, implementation_turn=True)
    assert not offered & set(PLANNING)
    assert offered >= set(WIRED)


def test_a_tool_the_mode_denies_is_not_offered() -> None:
    """The registry is restricted to the mode, and the selection never adds a tool."""
    from codeforge.tools import ToolRegistry
    from codeforge.tools._base import ToolDefinition

    class _Noop:
        async def execute(self, arguments: dict[str, object], workspace_path: str) -> object:
            return None

    registry = ToolRegistry()
    registry.restrict_to_mode(["Read"], ["mcp__gh__create_issue", "Bash"])
    for name in [*BUILTIN, *WIRED]:
        registry.register(ToolDefinition(name=name, description=name), _Noop())  # type: ignore[arg-type]

    offered = _offered(registry.tool_names, CapabilityLevel.FULL)
    assert "mcp__gh__create_issue" not in offered
    assert "bash" not in offered
    assert "write_file" not in offered, "a built-in tool missing from the mode's tools"
    assert {"read_file", "mcp__gh__search_issues", "search_skills"} <= offered
