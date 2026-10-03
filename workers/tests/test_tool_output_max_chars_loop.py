"""agent.tool_output_max_chars applies inside the running loop (S6-G review, item 12).

It only truncated tool results of earlier turns (history); a result produced
in the running loop went to the model in full. The limit now reaches the
loop (LoopConfig, from conversation.run.start and runs.start) and every tool
result appended in the loop is truncated to it.
"""

from __future__ import annotations

from codeforge.agent_loop import LoopConfig, _LoopState
from codeforge.history import DEFAULT_TOOL_OUTPUT_MAX_CHARS
from codeforge.llm import RoutingResult, ToolCallPart
from codeforge.loop_config import build_loop_config
from codeforge.models import RunStartMessage
from codeforge.tool_executor import ToolExecutor
from codeforge.tools.capability import CapabilityLevel


def test_tool_results_appended_in_the_loop_are_truncated() -> None:
    state = _LoopState(tool_output_max_chars=100)
    messages: list[dict[str, object]] = []
    ToolExecutor.append_result(ToolCallPart(id="c1", name="bash", arguments="{}"), "x" * 1000, messages, state)

    content = str(messages[0]["content"])
    assert len(content) < 200
    assert "characters omitted" in content
    assert state.tool_messages[0].content == content


def test_short_results_and_the_default_limit() -> None:
    state = _LoopState()
    assert state.tool_output_max_chars == DEFAULT_TOOL_OUTPUT_MAX_CHARS
    messages: list[dict[str, object]] = []
    ToolExecutor.append_result(ToolCallPart(id="c1", name="bash", arguments="{}"), "short", messages, state)
    assert messages[0]["content"] == "short"


def test_build_loop_config_carries_the_limit() -> None:
    cfg, _ = build_loop_config(
        primary_model="openai/gpt-4o",
        capability_level=CapabilityLevel.FULL,
        routing=RoutingResult(model="openai/gpt-4o"),
        tool_names=["bash"],
        fallback_models=[],
        user_prompt="",
        max_steps=5,
        max_cost=1.0,
        mode_tools=frozenset(),
        tool_output_max_chars=1234,
    )
    assert cfg.tool_output_max_chars == 1234
    assert LoopConfig().tool_output_max_chars == 0  # 0 = the worker's default


def test_run_start_message_carries_the_limit() -> None:
    msg = RunStartMessage.model_validate(
        {"run_id": "r", "task_id": "t", "project_id": "p", "agent_id": "a", "prompt": "x", "tool_output_max_chars": 77}
    )
    assert msg.tool_output_max_chars == 77
    assert (
        RunStartMessage.model_validate(
            {"run_id": "r", "task_id": "t", "project_id": "p", "agent_id": "a", "prompt": "x"}
        ).tool_output_max_chars
        == 0
    )
