"""A failed Claude Code turn falls back to the next model only when retrying it is safe (KI-72 review).

Re-running a turn on another model after Claude Code already applied some
changes (or after the user stopped the run) would work on a partly modified
workspace without knowing it.
"""

from __future__ import annotations

from unittest.mock import AsyncMock, MagicMock, patch

import pytest

from codeforge.consumer._conversation import ConversationHandlerMixin
from codeforge.models import AgentLoopResult, ConversationRunStartMessage


def _run_msg() -> ConversationRunStartMessage:
    return ConversationRunStartMessage(
        run_id="run-cc",
        conversation_id="conv-cc",
        project_id="proj-1",
        messages=[],
        system_prompt="You are a helper.",
        model="claudecode/default",
        agentic=True,
        workspace_path="/tmp/ws",
    )


async def _execute(result: AgentLoopResult) -> tuple[AgentLoopResult, AsyncMock]:
    mixin = type("_TestMixin", (ConversationHandlerMixin,), {})()
    fallback_result = AgentLoopResult(final_content="from the fallback model")
    mixin._execute_litellm_loop = AsyncMock(return_value=fallback_result)
    executor = MagicMock()
    executor.run = AsyncMock(return_value=result)
    runtime = MagicMock()
    runtime.send_output = AsyncMock()
    with patch("codeforge.claude_code_executor.ClaudeCodeExecutor", return_value=executor):
        out = await mixin._execute_conversation_run(
            run_msg=_run_msg(),
            messages=[{"role": "user", "content": "fix it"}],
            primary_model="claudecode/default",
            routing=MagicMock(),
            runtime=runtime,
            registry=MagicMock(),
            fallback_models=["openai/gpt-4o"],
        )
    return out, mixin._execute_litellm_loop


async def test_safe_failure_falls_back() -> None:
    failed = AgentLoopResult(error="not supported", metadata={"executor": "claude-code-cli", "fallback_safe": True})

    out, fallback = await _execute(failed)

    fallback.assert_awaited_once()
    assert out.final_content == "from the fallback model"


@pytest.mark.parametrize(
    "metadata",
    [
        {"executor": "claude-code-cli", "fallback_safe": False},
        {"executor": "claude-code-cli"},
        {},
    ],
)
async def test_unsafe_failure_is_returned_without_fallback(metadata: dict[str, object]) -> None:
    failed = AgentLoopResult(
        error="Claude Code CLI timed out. The workspace may be partly modified.",
        final_content="partial",
        metadata=metadata,
    )

    out, fallback = await _execute(failed)

    fallback.assert_not_awaited()
    assert out is failed


async def test_success_is_returned() -> None:
    ok = AgentLoopResult(final_content="done", metadata={"executor": "claude-code-cli", "fallback_safe": True})

    out, fallback = await _execute(ok)

    fallback.assert_not_awaited()
    assert out is ok
