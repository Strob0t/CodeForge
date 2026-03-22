"""Plan/Act mode controller for the agentic conversation loop.

Splits the agent loop into two phases:
1. **Plan phase**: read-only tools only (read_file, search_files, glob_files, list_directory).
   The agent analyzes the problem and creates a plan.
2. **Act phase**: all tools available. The agent executes the plan.

Enabled when the mode's autonomy level >= 4 (set by Go dispatcher via plan_act_enabled).
"""

from __future__ import annotations

from codeforge.config import get_settings

_DEFAULT_MAX_PLAN_ITERATIONS = 10

# Read-only tools allowed during the plan phase.
PLAN_TOOLS: frozenset[str] = frozenset(
    {
        "read_file",
        "search_files",
        "glob_files",
        "list_directory",
    }
)


def get_max_plan_iterations() -> int:
    """Read max plan iterations from settings."""
    return get_settings().plan_act_max_iterations


class PlanActController:
    """Controls plan/act phase transitions in the agent loop."""

    __slots__ = ("enabled", "extra_plan_tools", "max_plan_iterations", "phase", "plan_iterations")

    def __init__(
        self,
        enabled: bool,
        max_plan_iterations: int = _DEFAULT_MAX_PLAN_ITERATIONS,
        extra_plan_tools: frozenset[str] = frozenset(),
    ) -> None:
        self.enabled: bool = enabled
        self.phase: str = "plan" if enabled else "act"
        self.plan_iterations: int = 0
        self.max_plan_iterations: int = max_plan_iterations
        self.extra_plan_tools: frozenset[str] = extra_plan_tools

    def is_tool_allowed(self, tool_name: str) -> bool:
        """Check if a tool is allowed in the current phase."""
        if not self.enabled or self.phase == "act":
            return True
        return tool_name.lower() in PLAN_TOOLS or tool_name in self.extra_plan_tools

    def transition_to_act(self) -> None:
        """Transition from plan to act phase."""
        self.phase = "act"

    def tick_and_should_transition(self) -> bool:
        """Increment plan iteration count and check if auto-transition is due.

        FIX-090: Renamed from should_auto_transition() to make the side effect
        (incrementing plan_iterations) explicit in the method name.

        Returns True when the plan iteration count reaches max_plan_iterations.
        Does not fire when already in act phase or when disabled.
        """
        if not self.enabled or self.phase != "plan":
            return False
        self.plan_iterations += 1
        return self.plan_iterations >= self.max_plan_iterations

    def get_system_suffix(self) -> str:
        """Get phase-specific system prompt addition."""
        if not self.enabled:
            return ""
        if self.phase == "plan":
            extra = ""
            if self.extra_plan_tools:
                extra = f" Additionally, these mode-specific tools are available: {', '.join(sorted(self.extra_plan_tools))}."
            return (
                "\n\nYou are in PLAN phase. Only use read-only tools "
                "(read_file, search_files, glob_files, list_directory). "
                "Analyze the problem and create a plan. "
                f"When ready, call the 'transition_to_act' tool to start implementing.{extra}"
            )
        return "\n\nYou are in ACT phase. Execute your plan using all available tools."

    def get_routing_tag(self) -> str:
        """Return the LLM routing scenario tag for the current phase."""
        if not self.enabled:
            return "default"
        return "plan" if self.phase == "plan" else "default"
