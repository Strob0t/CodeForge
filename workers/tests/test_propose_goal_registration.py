"""Test that propose_goal is registered instead of manage_goals."""

from unittest.mock import AsyncMock, MagicMock

from codeforge.consumer._conversation_skill_integration import register_propose_goal_tool


def test_register_propose_goal_tool() -> None:
    registry = MagicMock()
    runtime = AsyncMock()

    register_propose_goal_tool(registry, runtime)

    registry.register.assert_called_once()
    defn = registry.register.call_args[0][0]
    assert defn.name == "propose_goal"
    executor = registry.register.call_args[0][1]
    assert executor._runtime is runtime
