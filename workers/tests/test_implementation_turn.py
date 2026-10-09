"""Implementation turns (KI-153).

In benchmark run 1 the auto-agent's feature turns were offered the planning
tools (propose_goal, propose_roadmap from the ToolRouter's BASE_TOOLS); the
model called them instead of writing code, and the turn ended. It also ended
turns with an announced action ("I will now ...") and no tool call.
Implementation turns (the auto-agent's feature turns, runs.start) are no
longer offered the planning tools, and an announced action without a tool
call gets one "continue" nudge per turn.
"""

from __future__ import annotations

import json
from unittest.mock import AsyncMock, MagicMock, patch

import pytest

from codeforge.agent_loop import AgentLoopExecutor, LoopConfig, announces_action
from codeforge.consumer._base import ConsumerBaseMixin
from codeforge.consumer._conversation import ConversationHandlerMixin
from codeforge.llm import ChatCompletionResponse, RoutingResult, ToolCallPart
from codeforge.loop_config import ModelCapability, build_loop_config
from codeforge.models import ConversationMessagePayload, ConversationRunStartMessage, ToolCallDecision
from codeforge.tools import ToolRegistry, ToolResult
from codeforge.tools._base import ToolDefinition
from codeforge.tools.capability import CapabilityLevel
from codeforge.tools.tool_router import ToolRouter

_PLANNING = {"propose_goal", "propose_roadmap"}
_TOOLS = ["read_file", "write_file", "edit_file", "bash", "search_files", "glob_files", "list_directory", *_PLANNING]


@pytest.fixture(autouse=True)
def _mock_resolve_model():
    with patch("codeforge.agent_loop.resolve_model", return_value="fake-model"):
        yield


# --- tool selection ---


def test_planning_tools_are_the_routers() -> None:
    assert frozenset(_PLANNING) == ToolRouter.PLANNING_TOOLS


def test_router_offers_planning_tools_only_when_planning() -> None:
    router = ToolRouter(all_tool_names=_TOOLS)
    assert not (set(router.select()) & _PLANNING)
    assert set(router.select(planning=True)) >= _PLANNING


def test_planning_tools_need_to_be_registered() -> None:
    assert not (set(ToolRouter(all_tool_names=["read_file"]).select(planning=True)) & _PLANNING)


@pytest.mark.parametrize(("implementation_turn", "offered"), [(True, False), (False, True)])
def test_loop_config_of_a_turn(implementation_turn: bool, offered: bool) -> None:
    cfg, _ = build_loop_config(
        primary_model="openai/gpt-4o",
        capability_level=CapabilityLevel.FULL,
        routing=RoutingResult(),
        tool_names=_TOOLS,
        fallback_models=[],
        max_steps=10,
        max_cost=0,
        mode_tools=frozenset(),
        implementation_turn=implementation_turn,
    )
    assert cfg.implementation_turn is implementation_turn
    assert cfg.selected_tools is not None
    assert bool(set(cfg.selected_tools) & _PLANNING) is offered


# --- registration in conversation turns ---


class _Handler(ConversationHandlerMixin, ConsumerBaseMixin):
    pass


@pytest.mark.parametrize(("implementation_turn", "registered"), [(True, False), (False, True)])
async def test_conversation_turn_registers_planning_tools_only_outside_implementation(
    implementation_turn: bool, registered: bool
) -> None:
    handler = _Handler()
    handler._llm = MagicMock()  # type: ignore[assignment]
    handler._db_url = ""
    handler._js = None
    handler._litellm_key = ""
    run_msg = ConversationRunStartMessage(
        run_id="r1",
        conversation_id="c1",
        project_id="p1",
        messages=[ConversationMessagePayload(role="user", content="Implement the feature")],
        system_prompt="s",
        model="openai/gpt-4o",
        implementation_turn=implementation_turn,
    )
    with (
        patch("codeforge.consumer._conversation.build_system_prompt", AsyncMock(return_value=("prompt", []))),
        patch(
            "codeforge.consumer._conversation.resolve_model_capability",
            AsyncMock(return_value=ModelCapability(CapabilityLevel.FULL, 128_000)),
        ),
        patch("codeforge.consumer._conversation.wire_skill_tools"),
        patch("codeforge.consumer._conversation.register_handoff_tool"),
        patch("codeforge.consumer._conversation.register_propose_goal_tool") as goal,
        patch("codeforge.consumer._conversation.register_propose_roadmap_tool") as roadmap,
    ):
        await handler._build_conversation_messages(run_msg, MagicMock(), MagicMock(), MagicMock(), model="m")

    assert goal.called is registered
    assert roadmap.called is registered


def test_run_start_message_defaults_to_a_planning_capable_turn() -> None:
    msg = ConversationRunStartMessage(
        run_id="r1", conversation_id="c1", project_id="p1", messages=[], system_prompt="s", model="m"
    )
    assert msg.implementation_turn is False


# --- the "continue" nudge ---


@pytest.mark.parametrize(
    ("content", "announced"),
    [
        ("I will now proceed to implement the CLI.", True),
        ("Now I'll create `cli.py` with the argument parser:", True),
        ("Let me write the tests next.", True),
        ("I am going to update pyproject.toml.", True),
        ("Next, I will run the tests.", True),
        ("The CLI is implemented and the tests pass.", False),
        ("Done. Let me know if you need anything else.", False),
        ("", False),
    ],
)
def test_announces_action(content: str, announced: bool) -> None:
    assert announces_action(content) is announced


class _RecordingLLM:
    """Returns the planned responses and records the messages of each call."""

    def __init__(self, responses: list[ChatCompletionResponse]) -> None:
        self._responses = responses
        self.calls: list[list[dict[str, object]]] = []

    async def chat_completion_stream(self, **kwargs: object) -> ChatCompletionResponse:
        self.calls.append([dict(m) for m in kwargs["messages"]])  # type: ignore[union-attr]
        if not self._responses:
            return ChatCompletionResponse(
                content="(exhausted)", tool_calls=[], finish_reason="stop", model="m", tokens_in=1, tokens_out=1
            )
        return self._responses.pop(0)


def _text(content: str) -> ChatCompletionResponse:
    return ChatCompletionResponse(
        content=content, tool_calls=[], finish_reason="stop", model="m", tokens_in=1, tokens_out=1
    )


def _tool_call() -> ChatCompletionResponse:
    call = ToolCallPart(id="c1", name="echo", arguments=json.dumps({"x": 1}))
    return ChatCompletionResponse(
        content="", tool_calls=[call], finish_reason="tool_calls", model="m", tokens_in=1, tokens_out=1
    )


def _runtime() -> MagicMock:
    runtime = MagicMock()
    runtime.run_id = "run-1"
    runtime.project_id = "proj-1"
    runtime.is_cancelled = False
    runtime.send_output = AsyncMock()
    runtime.request_tool_call = AsyncMock(return_value=ToolCallDecision(call_id="tc", decision="allow", reason=""))
    runtime.report_tool_result = AsyncMock()
    runtime.publish_trajectory_event = AsyncMock()
    return runtime


def _registry() -> ToolRegistry:
    class _Echo:
        async def execute(self, arguments: dict, workspace_path: str) -> ToolResult:
            return ToolResult(output="ok", success=True)

    registry = ToolRegistry()
    registry.register(ToolDefinition(name="echo", description="Echo", parameters={"type": "object"}), _Echo())
    return registry


def _nudges(calls: list[list[dict[str, object]]]) -> int:
    return sum(1 for m in calls[-1] if m.get("role") == "user" and "call the tool now" in str(m.get("content")))


async def _run(llm: _RecordingLLM, *, implementation_turn: bool) -> str:
    executor = AgentLoopExecutor(llm, _registry(), _runtime(), "/tmp/workspace")
    result = await executor.run(
        [{"role": "user", "content": "Implement the feature"}],
        config=LoopConfig(max_iterations=10, implementation_turn=implementation_turn),
    )
    return result.final_content


async def test_announced_action_gets_one_nudge_in_an_implementation_turn() -> None:
    llm = _RecordingLLM([_tool_call(), _text("I will now create the CLI."), _tool_call(), _text("The CLI is done.")])

    final = await _run(llm, implementation_turn=True)

    assert final == "The CLI is done."
    assert len(llm.calls) == 4
    assert _nudges(llm.calls) == 1


async def test_nudge_is_sent_once_per_turn() -> None:
    llm = _RecordingLLM([_text("I will now create the CLI."), _text("I will now write the tests.")])

    final = await _run(llm, implementation_turn=True)

    assert final == "I will now write the tests."
    assert len(llm.calls) == 2
    assert _nudges(llm.calls) == 1


@pytest.mark.parametrize(
    ("implementation_turn", "content"),
    [(False, "I will now create the CLI."), (True, "The CLI is implemented and the tests pass.")],
)
async def test_no_nudge(implementation_turn: bool, content: str) -> None:
    llm = _RecordingLLM([_text(content)])

    final = await _run(llm, implementation_turn=implementation_turn)

    assert final == content
    assert len(llm.calls) == 1
