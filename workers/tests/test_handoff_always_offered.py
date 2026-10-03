"""handoff_to is offered whenever it is registered (S6-G review, item 2).

KI-38 named the tool correctly in the capability allowlist, but every turn
with a user prompt goes through the ToolRouter, whose selection replaces the
allowlist; handoff_to is neither a base tool nor keyword-triggered there, so a
mode that may hand off never saw the tool.
"""

from __future__ import annotations

import pytest

from codeforge.agent_loop import AgentLoopExecutor
from codeforge.llm import RoutingResult
from codeforge.loop_config import build_loop_config
from codeforge.tools.capability import CapabilityLevel

BASE = ["read_file", "write_file", "edit_file", "bash", "search_files", "glob_files", "list_directory"]


def _tools(names: list[str]) -> list[dict[str, object]]:
    return [{"type": "function", "function": {"name": n, "description": n, "parameters": {}}} for n in names]


def _offered(names: list[str], prompt: str, level: CapabilityLevel) -> set[str]:
    cfg, _ = build_loop_config(
        primary_model="provider/model",
        capability_level=level,
        routing=RoutingResult(model="provider/model"),
        tool_names=names,
        fallback_models=[],
        user_prompt=prompt,
        max_steps=5,
        max_cost=1.0,
        mode_tools=frozenset(),
    )
    result = AgentLoopExecutor._filter_tools_for_capability(
        _tools(names), CapabilityLevel(cfg.capability_level), cfg.mode_tools or None, cfg.selected_tools
    )
    return {t["function"]["name"] for t in result}  # type: ignore[index]


@pytest.mark.parametrize("level", list(CapabilityLevel))
@pytest.mark.parametrize("prompt", ["Fix the failing test in parser.py", "write the docs", "x"])
def test_registered_handoff_is_always_offered(level: CapabilityLevel, prompt: str) -> None:
    offered = _offered([*BASE, "handoff_to", "create_skill"], prompt, level)
    assert "handoff_to" in offered


@pytest.mark.parametrize("level", list(CapabilityLevel))
def test_handoff_is_not_offered_when_not_registered(level: CapabilityLevel) -> None:
    offered = _offered(BASE, "hand this over to the reviewer", level)
    assert "handoff_to" not in offered
