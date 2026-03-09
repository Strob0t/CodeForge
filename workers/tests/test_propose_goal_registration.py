"""Test that propose_goal is registered instead of manage_goals."""

from unittest.mock import AsyncMock, MagicMock

from codeforge.consumer._conversation import ConversationHandlerMixin


def test_register_propose_goal_tool() -> None:
    handler = ConversationHandlerMixin.__new__(ConversationHandlerMixin)
    handler._js = MagicMock()
    registry = MagicMock()
    runtime = AsyncMock()

    handler._register_propose_goal_tool(registry, runtime)

    registry.register.assert_called_once()
    defn = registry.register.call_args[0][0]
    assert defn.name == "propose_goal"
    executor = registry.register.call_args[0][1]
    assert hasattr(executor, "_runtime")
