"""Core agentic loop: LLM calls tools, tools execute, results feed back.

This is the heart of the interactive agent. The loop:
1. Calls the LLM with the current message history and available tools.
2. If the LLM returns tool_calls, executes each one (with policy checks).
3. Appends results to the message history and repeats.
4. If the LLM returns text only (finish_reason="stop"), the loop ends.
"""

from __future__ import annotations

import json
import logging
from dataclasses import dataclass, field
from typing import TYPE_CHECKING

from codeforge.models import (
    AgentLoopResult,
    ConversationMessagePayload,
    ConversationToolCallFunction,
    ConversationToolCallPayload,
)
from codeforge.pricing import resolve_cost
from codeforge.tracing.setup import TracingManager

if TYPE_CHECKING:
    from codeforge.llm import ChatCompletionResponse, LiteLLMClient, ToolCallPart
    from codeforge.runtime import RuntimeClient
    from codeforge.tools import ToolRegistry

logger = logging.getLogger(__name__)

_tracing = TracingManager()
_tracer = _tracing.get_tracer()

DEFAULT_MAX_ITERATIONS = 50


@dataclass
class LoopConfig:
    """Configuration for the agentic loop."""

    max_iterations: int = DEFAULT_MAX_ITERATIONS
    max_cost: float = 0.0  # 0 = unlimited
    model: str = ""
    temperature: float = 0.2
    tags: list[str] = field(default_factory=list)


@dataclass
class _LoopState:
    """Mutable accumulator for loop execution state."""

    model: str = ""
    total_cost: float = 0.0
    total_tokens_in: int = 0
    total_tokens_out: int = 0
    step_count: int = 0
    final_content: str = ""
    error: str = ""
    tool_messages: list[ConversationMessagePayload] = field(default_factory=list)


class AgentLoopExecutor:
    """Executes the agentic tool-use loop.

    Constructor arguments:
        llm: LiteLLM client for chat completions.
        tool_registry: Registry of available tools (built-in + MCP).
        runtime: RuntimeClient for policy checks and streaming output.
        workspace_path: Absolute path to the project workspace.
    """

    def __init__(
        self,
        llm: LiteLLMClient,
        tool_registry: ToolRegistry,
        runtime: RuntimeClient,
        workspace_path: str,
    ) -> None:
        self._llm = llm
        self._tools = tool_registry
        self._runtime = runtime
        self._workspace = workspace_path

    @_tracer.trace_agent("agent_loop")
    async def run(
        self,
        messages: list[dict[str, object]],
        config: LoopConfig | None = None,
    ) -> AgentLoopResult:
        """Execute the agentic loop until the LLM stops or limits are hit.

        *messages* should include the system prompt and full conversation
        history (as assembled by ConversationHistoryManager).

        Returns an AgentLoopResult with the final assistant content,
        all intermediate tool messages, and accumulated cost/token stats.
        """
        cfg = config or LoopConfig()
        state = _LoopState(model=cfg.model)
        tools_array = self._tools.get_openai_tools()

        for iteration in range(cfg.max_iterations):
            if self._runtime.is_cancelled:
                state.error = "cancelled"
                break

            result = await self._do_llm_iteration(cfg, tools_array, messages, state, iteration)
            if result is not None:
                # result is True for "stop" (final text), string for error.
                if isinstance(result, str):
                    state.error = result
                break

            # Check termination conditions.
            if cfg.max_cost > 0 and state.total_cost >= cfg.max_cost:
                logger.info("cost limit reached: %.4f >= %.4f", state.total_cost, cfg.max_cost)
                break
        else:
            logger.warning("agent loop hit max iterations (%d)", cfg.max_iterations)

        return AgentLoopResult(
            final_content=state.final_content,
            tool_messages=state.tool_messages,
            total_cost=state.total_cost,
            total_tokens_in=state.total_tokens_in,
            total_tokens_out=state.total_tokens_out,
            step_count=state.step_count,
            model=state.model,
            error=state.error,
        )

    async def _do_llm_iteration(
        self,
        cfg: LoopConfig,
        tools_array: list[dict[str, object]],
        messages: list[dict[str, object]],
        state: _LoopState,
        iteration: int,
    ) -> bool | str | None:
        """Run one LLM iteration. Returns True on stop, error string on failure, None to continue."""
        llm_decision = await self._runtime.request_tool_call(tool="LLM", command="chat_completion")
        if llm_decision.decision != "allow":
            logger.warning("LLM call denied by policy: %s", llm_decision.reason)
            return f"LLM call denied: {llm_decision.reason}"

        streamed_text: list[str] = []
        try:
            response = await self._llm.chat_completion_stream(
                messages=messages,
                model=cfg.model or "ollama/llama3.2",
                tools=tools_array or None,
                temperature=cfg.temperature,
                tags=cfg.tags or None,
                on_chunk=streamed_text.append,
            )
        except Exception as exc:
            logger.exception("LLM call failed on iteration %d", iteration)
            return f"LLM call failed: {exc}"

        full_text = "".join(streamed_text)
        if full_text:
            await self._runtime.send_output(full_text)

        cost = resolve_cost(response.cost_usd, response.model, response.tokens_in, response.tokens_out)
        state.total_cost += cost
        state.total_tokens_in += response.tokens_in
        state.total_tokens_out += response.tokens_out
        if response.model:
            state.model = response.model

        await self._runtime.report_tool_result(
            call_id=llm_decision.call_id,
            tool="LLM",
            success=True,
            output=full_text[:200] if full_text else "(tool_calls)",
            cost_usd=cost,
            tokens_in=response.tokens_in,
            tokens_out=response.tokens_out,
            model=response.model,
        )

        if not response.tool_calls:
            state.final_content = response.content
            return True

        assistant_msg = _build_assistant_message(response)
        state.tool_messages.append(assistant_msg)
        messages.append(_payload_to_dict(assistant_msg))

        for tc in response.tool_calls:
            state.step_count += 1
            await self._execute_tool_call(tc, messages, state)
            if self._runtime.is_cancelled:
                break

        return None

    async def _execute_tool_call(
        self,
        tc: ToolCallPart,
        messages: list[dict[str, object]],
        state: _LoopState,
    ) -> None:
        """Execute a single tool call with policy check and error handling."""
        try:
            arguments = json.loads(tc.arguments) if tc.arguments else {}
        except json.JSONDecodeError:
            arguments = {}

        decision = await self._runtime.request_tool_call(
            tool=tc.name,
            command=tc.arguments[:200] if tc.arguments else "",
        )

        if decision.decision != "allow":
            result_text = f"Permission denied: {decision.reason}"
            self._append_tool_result(tc, result_text, messages, state)
            await self._runtime.report_tool_result(
                call_id=decision.call_id,
                tool=tc.name,
                success=False,
                error=result_text,
            )
            return

        try:
            result = await self._tools.execute(tc.name, arguments, self._workspace)
        except Exception as exc:
            logger.exception("tool %s execution error", tc.name)
            result_text = f"Error executing {tc.name}: {exc}"
            self._append_tool_result(tc, result_text, messages, state)
            await self._runtime.report_tool_result(
                call_id=decision.call_id,
                tool=tc.name,
                success=False,
                error=result_text,
            )
            return

        result_text = (
            result.output
            if result.success
            else (f"Error: {result.error}" if result.error else "Tool returned an error")
        )
        self._append_tool_result(tc, result_text, messages, state)
        await self._runtime.report_tool_result(
            call_id=decision.call_id,
            tool=tc.name,
            success=result.success,
            output=result.output[:500] if result.output else "",
            error=result.error,
        )

    @staticmethod
    def _append_tool_result(
        tc: ToolCallPart,
        content: str,
        messages: list[dict[str, object]],
        state: _LoopState,
    ) -> None:
        """Build and append a tool result message to state and messages."""
        msg = _build_tool_result_message(tc, content)
        state.tool_messages.append(msg)
        messages.append(_payload_to_dict(msg))


def _build_assistant_message(response: ChatCompletionResponse) -> ConversationMessagePayload:
    """Build a ConversationMessagePayload for an assistant message with tool_calls."""
    return ConversationMessagePayload(
        role="assistant",
        content=response.content,
        tool_calls=[
            ConversationToolCallPayload(
                id=tc.id,
                type="function",
                function=ConversationToolCallFunction(name=tc.name, arguments=tc.arguments),
            )
            for tc in response.tool_calls
        ],
    )


def _build_tool_result_message(tc: ToolCallPart, content: str) -> ConversationMessagePayload:
    """Build a ConversationMessagePayload for a tool result."""
    return ConversationMessagePayload(
        role="tool",
        content=content,
        tool_call_id=tc.id,
        name=tc.name,
    )


def _payload_to_dict(msg: ConversationMessagePayload) -> dict[str, object]:
    """Convert a ConversationMessagePayload to an OpenAI-compatible message dict."""
    d: dict[str, object] = {"role": msg.role}
    if msg.content:
        d["content"] = msg.content
    if msg.tool_calls:
        d["tool_calls"] = [
            {
                "id": tc.id,
                "type": tc.type,
                "function": {"name": tc.function.name, "arguments": tc.function.arguments},
            }
            for tc in msg.tool_calls
        ]
    if msg.tool_call_id:
        d["tool_call_id"] = msg.tool_call_id
    if msg.name:
        d["name"] = msg.name
    return d
