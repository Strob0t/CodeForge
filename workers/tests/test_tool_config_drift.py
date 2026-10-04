"""Tool configuration matches the tools and settings that exist (KI-38).

The capability allowlist named ``handoff`` while the tool is ``handoff_to``, so
models with the api_with_tools level never got it; and the Go setting
``agent.tool_output_max_chars`` never reached the worker, which always cut
tool results in the history at its own default.
"""

from __future__ import annotations

from unittest.mock import AsyncMock, MagicMock, patch

from codeforge.consumer._base import ConsumerBaseMixin
from codeforge.consumer._conversation import ConversationHandlerMixin
from codeforge.loop_config import ModelCapability
from codeforge.models import (
    ConversationMessagePayload,
    ConversationRunStartMessage,
    ConversationToolCallFunction,
    ConversationToolCallPayload,
)
from codeforge.tools.capability import TOOLS_BY_CAPABILITY, CapabilityLevel
from codeforge.tools.handoff import HANDOFF_TOOL_DEF
from codeforge.tools.tool_router import ToolRouter


def test_capability_allowlists_name_registered_tools() -> None:
    handoff = str(HANDOFF_TOOL_DEF["function"]["name"])
    assert handoff in TOOLS_BY_CAPABILITY[CapabilityLevel.API_WITH_TOOLS]
    for level, names in TOOLS_BY_CAPABILITY.items():
        assert "handoff" not in names, level


def test_tool_router_base_tools_name_registered_tools() -> None:
    assert "handoff" not in ToolRouter.BASE_TOOLS


class _Handler(ConversationHandlerMixin, ConsumerBaseMixin):
    pass


async def test_history_truncates_tool_results_at_the_configured_limit() -> None:
    handler = _Handler()
    handler._llm = MagicMock()  # type: ignore[assignment]
    handler._db_url = ""
    handler._js = None
    handler._litellm_key = ""
    long_output = "x" * 5_000
    run_msg = ConversationRunStartMessage(
        run_id="r1",
        conversation_id="c1",
        project_id="p1",
        tenant_id="aaaaaaaa-0000-4000-8000-000000000001",
        messages=[
            ConversationMessagePayload(role="user", content="list the files"),
            ConversationMessagePayload(
                role="assistant",
                tool_calls=[
                    ConversationToolCallPayload(
                        id="call-1", function=ConversationToolCallFunction(name="bash", arguments='{"command": "ls"}')
                    )
                ],
            ),
            ConversationMessagePayload(role="tool", content=long_output, tool_call_id="call-1", name="bash"),
        ],
        system_prompt="s",
        model="openai/gpt-4o",
        tool_output_max_chars=200,
    )
    with (
        patch("codeforge.consumer._conversation.build_system_prompt", AsyncMock(return_value=("prompt", []))),
        patch(
            "codeforge.consumer._conversation.resolve_model_capability",
            AsyncMock(return_value=ModelCapability(CapabilityLevel.FULL, 128_000)),
        ),
        patch("codeforge.consumer._conversation.wire_skill_tools"),
        patch("codeforge.consumer._conversation.register_handoff_tool"),
        patch("codeforge.consumer._conversation.register_propose_goal_tool"),
        patch("codeforge.consumer._conversation.register_propose_roadmap_tool"),
    ):
        messages = await handler._build_conversation_messages(run_msg, MagicMock(), MagicMock(), MagicMock())

    tool_messages = [m for m in messages if m.get("role") == "tool"]
    assert len(tool_messages) == 1
    content = str(tool_messages[0]["content"])
    assert len(content) < 400, len(content)
    assert "characters omitted" in content


def test_unset_limit_keeps_the_worker_default() -> None:
    run_msg = ConversationRunStartMessage(
        run_id="r1", conversation_id="c1", project_id="p1", messages=[], system_prompt="s", model="m"
    )
    assert run_msg.tool_output_max_chars == 0
