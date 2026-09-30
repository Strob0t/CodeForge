"""Mode tool lists use canonical names; the loop compares them canonically.

Go sends a mode's tools as canonical policy names (Read, Write, Edit, Bash,
Grep, Glob, ListDir). The capability filter added them to its allowlist of
worker tool names (read_file, edit_file, ...), so a mode's built-in tools were
never added (a weak model lost edit_file although the mode needs it), and the
plan phase announced "Bash, Edit, Write" as extra plan tools.
"""

from __future__ import annotations

from codeforge.agent_loop import AgentLoopExecutor
from codeforge.llm import RoutingResult
from codeforge.loop_config import build_loop_config
from codeforge.plan_act import PlanActController
from codeforge.tools.capability import CapabilityLevel

NAMES = ["read_file", "write_file", "edit_file", "bash", "search_files", "glob_files", "list_directory", "propose_goal"]
TOOLS: list[dict[str, object]] = [
    {"type": "function", "function": {"name": n, "description": n, "parameters": {}}} for n in NAMES
]


def _offered(**kwargs: object) -> set[str]:
    result = AgentLoopExecutor._filter_tools_for_capability(TOOLS, **kwargs)  # type: ignore[arg-type]
    return {t["function"]["name"] for t in result}  # type: ignore[index]


def test_canonical_mode_tools_extend_the_capability_allowlist() -> None:
    offered = _offered(capability=CapabilityLevel.PURE_COMPLETION, mode_tools=frozenset({"Edit", "Glob"}))
    assert {"edit_file", "glob_files"} <= offered
    assert "list_directory" not in offered, "not in the mode's tools, not in the capability allowlist"


def test_canonical_mode_tools_extend_the_router_selection() -> None:
    offered = _offered(
        capability=CapabilityLevel.API_WITH_TOOLS,
        mode_tools=frozenset({"Bash", "ListDir"}),
        selected_tools=["read_file"],
    )
    assert offered == {"read_file", "bash", "list_directory"}


def test_worker_names_in_mode_tools_still_work() -> None:
    offered = _offered(capability=CapabilityLevel.PURE_COMPLETION, mode_tools=frozenset({"edit_file"}))
    assert "edit_file" in offered


def _loop_config(mode_tools: frozenset[str]) -> object:
    cfg, _ = build_loop_config(
        primary_model="openai/gpt-4o",
        routing=RoutingResult(model="openai/gpt-4o"),
        tool_names=NAMES,
        fallback_models=[],
        user_prompt="",
        max_steps=5,
        max_cost=1.0,
        mode_tools=mode_tools,
        plan_act_enabled=True,
    )
    return cfg


def test_plan_phase_stays_read_only_for_modes_with_write_tools() -> None:
    cfg = _loop_config(frozenset({"Read", "Write", "Edit", "Bash", "propose_goal"}))

    assert cfg.extra_plan_tools == frozenset({"propose_goal"})  # type: ignore[attr-defined]
    plan = PlanActController(enabled=True, extra_plan_tools=cfg.extra_plan_tools)  # type: ignore[attr-defined]
    for tool in ("bash", "write_file", "edit_file", "Bash"):
        assert not plan.is_tool_allowed(tool), tool
    assert plan.is_tool_allowed("read_file")
    assert plan.is_tool_allowed("propose_goal")
    suffix = plan.get_system_suffix()
    assert "Bash" not in suffix
    assert "Write" not in suffix
