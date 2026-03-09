"""Core agentic loop: LLM calls tools, tools execute, results feed back.

This is the heart of the interactive agent. The loop:
1. Calls the LLM with the current message history and available tools.
2. If the LLM returns tool_calls, executes each one (with policy checks).
3. Appends results to the message history and repeats.
4. If the LLM returns text only (finish_reason="stop"), the loop ends.
"""

from __future__ import annotations

import asyncio
import logging
import time
from dataclasses import dataclass, field
from datetime import UTC, datetime
from typing import TYPE_CHECKING

from opentelemetry import trace
from opentelemetry.trace import StatusCode

from codeforge.json_utils import safe_json_loads
from codeforge.llm import LLMError, is_fallback_eligible
from codeforge.model_resolver import resolve_model
from codeforge.models import (
    AgentLoopResult,
    ConversationMessagePayload,
    ConversationToolCallFunction,
    ConversationToolCallPayload,
    ToolCallDecision,
)
from codeforge.pricing import resolve_cost
from codeforge.routing.blocklist import get_blocklist
from codeforge.tracing import metrics as otel_metrics
from codeforge.tracing import tracing_manager

if TYPE_CHECKING:
    from codeforge.llm import ChatCompletionResponse, LiteLLMClient, ToolCallPart
    from codeforge.runtime import RuntimeClient
    from codeforge.tools import ToolRegistry

logger = logging.getLogger(__name__)

_tracer = tracing_manager.get_tracer()

DEFAULT_MAX_ITERATIONS = 50


@dataclass
class LoopConfig:
    """Configuration for the agentic loop."""

    max_iterations: int = DEFAULT_MAX_ITERATIONS
    max_cost: float = 0.0  # 0 = unlimited
    model: str = ""
    temperature: float = 0.2
    tags: list[str] = field(default_factory=list)
    fallback_models: list[str] = field(default_factory=list)
    output_schema: str = ""  # Pydantic schema name from codeforge.schemas
    routing_layer: str = ""
    complexity_tier: str = ""
    task_type: str = ""
    provider_api_key: str = ""


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
    failed_models: set[str] = field(default_factory=set)


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

    @staticmethod
    def _pick_next_fallback(cfg: LoopConfig, state: _LoopState) -> str | None:
        """Return the next untried fallback model, or None if exhausted."""
        for m in cfg.fallback_models:
            if m not in state.failed_models:
                return m
        return None

    async def _try_model_fallback(
        self,
        cfg: LoopConfig,
        state: _LoopState,
        exc: LLMError,
    ) -> str | None:
        """Attempt to switch to a fallback model. Returns error string or None (retry)."""
        if not is_fallback_eligible(exc) or not cfg.fallback_models:
            return f"LLM call failed: {exc}"
        failed_model = cfg.model
        state.failed_models.add(failed_model)
        if exc.status_code in (401, 403):
            get_blocklist().block_auth(failed_model, reason=f"HTTP {exc.status_code}")
        next_model = self._pick_next_fallback(cfg, state)
        if next_model is None:
            return f"LLM call failed: {exc}"
        cfg.model = next_model
        logger.warning(
            "model fallback: %s -> %s (status %d)",
            failed_model,
            next_model,
            exc.status_code,
        )
        notice = f"\n[Model {failed_model} unavailable ({exc.status_code}). Switching to {next_model}]\n"
        await self._runtime.send_output(notice)
        return None

    async def _handle_llm_error(
        self,
        cfg: LoopConfig,
        state: _LoopState,
        exc: LLMError,
        iteration: int,
    ) -> str | None:
        """Handle an LLM error: record outcome, attempt fallback. Returns error or None."""
        logger.exception("LLM call failed on iteration %d", iteration)
        if cfg.routing_layer:
            await _record_routing_outcome(
                model=cfg.model,
                task_type=cfg.task_type,
                complexity_tier=cfg.complexity_tier,
                success=False,
                cost_usd=0.0,
                latency_ms=0,
                tokens_in=0,
                tokens_out=0,
                routing_layer=cfg.routing_layer,
                run_id=self._runtime.run_id,
            )
        return await self._try_model_fallback(cfg, state, exc)

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
        loop_start = time.monotonic()

        for iteration in range(cfg.max_iterations):
            otel_metrics.loop_iterations.add(1)
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
            state.error = f"iteration limit reached ({cfg.max_iterations})"

        otel_metrics.loop_duration.record(time.monotonic() - loop_start)

        # When an output_schema is specified and the loop produced final content,
        # validate/reparse it through the StructuredOutputParser.
        if cfg.output_schema and state.final_content and not state.error:
            state = await self._validate_output_schema(cfg, state, messages)

        try:
            await self._runtime.publish_trajectory_event(
                {
                    "event_type": "agent.finished",
                    "final_content_length": len(state.final_content),
                    "total_cost": state.total_cost,
                    "total_tokens_in": state.total_tokens_in,
                    "total_tokens_out": state.total_tokens_out,
                    "step_count": state.step_count,
                    "model": state.model,
                    "error": state.error or None,
                    "timestamp": datetime.now(UTC).isoformat(),
                }
            )
        except Exception as exc:
            logger.debug("failed to publish finished trajectory event: %s", exc)

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

    async def _validate_output_schema(
        self,
        cfg: LoopConfig,
        state: _LoopState,
        messages: list[dict[str, object]],
    ) -> _LoopState:
        """Validate/reparse final content against the specified output schema."""
        from codeforge.schemas.parser import StructuredOutputParser

        schema_cls = _resolve_schema(cfg.output_schema)
        if schema_cls is None:
            logger.warning("unknown output_schema %r, skipping validation", cfg.output_schema)
            return state

        # First try direct validation of the existing content.
        import json as _json

        from pydantic import ValidationError

        try:
            parsed = _json.loads(state.final_content)
            schema_cls.model_validate(parsed)
            return state  # Already valid JSON matching the schema.
        except (ValueError, ValidationError):
            pass  # Content is not valid JSON or does not match schema; use parser.

        parser = StructuredOutputParser(self._llm)
        reparse_messages: list[dict[str, object]] = list(messages)
        reparse_messages.append(
            {
                "role": "user",
                "content": (
                    f"Reformat your previous response as valid JSON matching the {cfg.output_schema} schema. "
                    "Return ONLY the JSON object."
                ),
            }
        )
        try:
            result = await parser.parse(
                messages=reparse_messages,
                schema=schema_cls,
                model=cfg.model,
                temperature=cfg.temperature,
                tags=cfg.tags or None,
            )
            state.final_content = result.model_dump_json()
        except ValueError as exc:
            logger.warning("output_schema validation failed: %s", exc)
            state.error = f"output_schema validation failed: {exc}"
        return state

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

        tracer = trace.get_tracer("codeforge")
        model_name = cfg.model or resolve_model()
        llm_start = time.monotonic()

        streamed_text: list[str] = []
        loop = asyncio.get_running_loop()
        pending_sends: list[asyncio.Task[None]] = []

        def _on_chunk(chunk_text: str) -> None:
            streamed_text.append(chunk_text)
            task = loop.create_task(self._runtime.send_output(chunk_text))
            pending_sends.append(task)

        with tracer.start_as_current_span(
            "llm.chat_completion",
            attributes={
                "gen_ai.request.model": model_name,
                "gen_ai.system": model_name.split("/")[0] if "/" in model_name else "unknown",
            },
        ) as llm_span:
            try:
                response = await self._llm.chat_completion_stream(
                    messages=messages,
                    model=model_name,
                    tools=tools_array or None,
                    temperature=cfg.temperature,
                    tags=cfg.tags or None,
                    on_chunk=_on_chunk,
                    provider_api_key=cfg.provider_api_key,
                )
            except LLMError as exc:
                llm_span.set_status(StatusCode.ERROR, str(exc))
                llm_span.record_exception(exc)
                return await self._handle_llm_error(cfg, state, exc, iteration)
            except Exception as exc:
                llm_span.set_status(StatusCode.ERROR, str(exc))
                llm_span.record_exception(exc)
                logger.exception("LLM call failed on iteration %d (unexpected)", iteration)
                # Wrap unexpected errors as LLMError so fallback logic can try another model.
                wrapped = LLMError(status_code=500, model=model_name, body=str(exc))
                return await self._handle_llm_error(cfg, state, wrapped, iteration)

            llm_span.set_attribute("gen_ai.usage.input_tokens", response.tokens_in)
            llm_span.set_attribute("gen_ai.usage.output_tokens", response.tokens_out)
            if response.model:
                llm_span.set_attribute("gen_ai.response.model", response.model)

        if pending_sends:
            await asyncio.gather(*pending_sends, return_exceptions=True)

        otel_metrics.llm_call_duration.record(time.monotonic() - llm_start)
        otel_metrics.llm_tokens.add(response.tokens_in + response.tokens_out)

        full_text = "".join(streamed_text)
        if full_text and not pending_sends:
            await self._runtime.send_output(full_text)

        return await self._process_llm_response(cfg, state, response, llm_decision, full_text, messages)

    async def _process_llm_response(
        self,
        cfg: LoopConfig,
        state: _LoopState,
        response: ChatCompletionResponse,
        llm_decision: ToolCallDecision,
        full_text: str,
        messages: list[dict[str, object]],
    ) -> bool | None:
        """Process LLM response: update state, report results, execute tool calls."""
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

        try:
            await self._runtime.publish_trajectory_event(
                {
                    "event_type": "agent.step_done",
                    "model": response.model,
                    "tokens_in": response.tokens_in,
                    "tokens_out": response.tokens_out,
                    "cost_usd": cost,
                    "has_tool_calls": bool(response.tool_calls),
                    "timestamp": datetime.now(UTC).isoformat(),
                }
            )
        except Exception as exc:
            logger.debug("failed to publish step_done trajectory event: %s", exc)

        if cfg.routing_layer:
            await _record_routing_outcome(
                model=response.model or cfg.model,
                task_type=cfg.task_type,
                complexity_tier=cfg.complexity_tier,
                success=True,
                cost_usd=cost,
                latency_ms=0,
                tokens_in=response.tokens_in,
                tokens_out=response.tokens_out,
                routing_layer=cfg.routing_layer,
                run_id=self._runtime.run_id,
            )

        if not response.tool_calls:
            state.final_content = response.content
            return True

        assistant_msg = _build_assistant_message(response)
        state.tool_messages.append(assistant_msg)
        messages.append(_payload_to_dict(assistant_msg))

        for i, tc in enumerate(response.tool_calls):
            state.step_count += 1
            await self._execute_tool_call(tc, messages, state)
            if self._runtime.is_cancelled:
                # Append placeholder results for remaining tool calls so the
                # message history stays balanced (required by strict providers
                # like Mistral).
                for remaining_tc in response.tool_calls[i + 1 :]:
                    self._append_tool_result(remaining_tc, "Cancelled", messages, state)
                break

        return None

    async def _execute_tool_call(
        self,
        tc: ToolCallPart,
        messages: list[dict[str, object]],
        state: _LoopState,
    ) -> None:
        """Execute a single tool call with policy check and error handling."""
        arguments: dict = safe_json_loads(tc.arguments, {}) if tc.arguments else {}

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
            try:
                await self._runtime.publish_trajectory_event(
                    {
                        "event_type": "agent.tool_called",
                        "tool_name": tc.name,
                        "input": (tc.arguments or "")[:500],
                        "output": result_text[:500],
                        "success": False,
                        "duration_ms": 0,
                        "step": state.step_count,
                        "timestamp": datetime.now(UTC).isoformat(),
                    }
                )
            except Exception as exc:
                logger.debug("failed to publish tool_called trajectory event: %s", exc)
            return

        tracer = trace.get_tracer("codeforge")
        tool_start = time.monotonic()
        with tracer.start_as_current_span(
            f"tool.execute:{tc.name}",
            attributes={"tool.name": tc.name},
        ) as tool_span:
            try:
                result = await self._tools.execute(tc.name, arguments, self._workspace)
            except Exception as exc:
                tool_span.set_status(StatusCode.ERROR, str(exc))
                tool_span.record_exception(exc)
                logger.exception("tool %s execution error", tc.name)
                result_text = f"Error executing {tc.name}: {exc}"
                correction = _build_correction_hint(tc.name, str(exc))
                if correction:
                    result_text = f"{result_text}\n\n{correction}"
                self._append_tool_result(tc, result_text, messages, state)
                await self._runtime.report_tool_result(
                    call_id=decision.call_id,
                    tool=tc.name,
                    success=False,
                    error=result_text,
                )
                elapsed_ms = (time.monotonic() - tool_start) * 1000
                try:
                    await self._runtime.publish_trajectory_event(
                        {
                            "event_type": "agent.tool_called",
                            "tool_name": tc.name,
                            "input": (tc.arguments or "")[:500],
                            "output": result_text[:500],
                            "success": False,
                            "duration_ms": round(elapsed_ms, 1),
                            "step": state.step_count,
                            "timestamp": datetime.now(UTC).isoformat(),
                        }
                    )
                except Exception as traj_exc:
                    logger.debug("failed to publish tool_called trajectory event: %s", traj_exc)
                return

            if not result.success and result.error:
                tool_span.set_status(StatusCode.ERROR, result.error)
                correction = _build_correction_hint(tc.name, result.error)
                result_text = f"Error: {result.error}\n\n{correction}" if correction else f"Error: {result.error}"
            elif not result.success:
                tool_span.set_status(StatusCode.ERROR, "Tool returned an error")
                result_text = "Tool returned an error"
            else:
                result_text = result.output
            self._append_tool_result(tc, result_text, messages, state)
            await self._runtime.report_tool_result(
                call_id=decision.call_id,
                tool=tc.name,
                success=result.success,
                output=result.output[:500] if result.output else "",
                error=result.error,
            )
            elapsed_ms = (time.monotonic() - tool_start) * 1000
            otel_metrics.tool_duration.record(elapsed_ms / 1000)
            try:
                await self._runtime.publish_trajectory_event(
                    {
                        "event_type": "agent.tool_called",
                        "tool_name": tc.name,
                        "input": (tc.arguments or "")[:500],
                        "output": result_text[:500],
                        "success": result.success,
                        "duration_ms": round(elapsed_ms, 1),
                        "step": state.step_count,
                        "timestamp": datetime.now(UTC).isoformat(),
                    }
                )
            except Exception as exc:
                logger.debug("failed to publish tool_called trajectory event: %s", exc)

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


async def _record_routing_outcome(
    model: str,
    task_type: str,
    complexity_tier: str,
    success: bool,
    cost_usd: float,
    latency_ms: int,
    tokens_in: int,
    tokens_out: int,
    routing_layer: str,
    run_id: str,
) -> None:
    """Post a routing outcome to Go Core for MAB learning. Fire-and-forget."""
    import os

    import httpx

    from codeforge.routing.models import RoutingConfig
    from codeforge.routing.reward import compute_reward

    quality = 1.0 if success else 0.0
    reward = compute_reward(success, quality, cost_usd, latency_ms, RoutingConfig())
    core_url = os.environ.get("CODEFORGE_CORE_URL", "http://localhost:8080")
    internal_key = os.environ.get("CODEFORGE_INTERNAL_KEY", "")
    headers: dict[str, str] = {}
    if internal_key:
        headers["X-API-Key"] = internal_key
    for attempt in range(2):
        try:
            async with httpx.AsyncClient(timeout=3.0) as client:
                await client.post(
                    f"{core_url}/api/v1/routing/outcomes",
                    json={
                        "model_name": model,
                        "task_type": task_type or "chat",
                        "complexity_tier": complexity_tier or "simple",
                        "success": success,
                        "quality_score": quality,
                        "cost_usd": cost_usd,
                        "latency_ms": latency_ms,
                        "tokens_in": tokens_in,
                        "tokens_out": tokens_out,
                        "reward": reward,
                        "routing_layer": routing_layer,
                        "run_id": run_id,
                    },
                    headers=headers,
                )
            return
        except Exception as exc:
            if attempt == 0:
                await asyncio.sleep(1)
                continue
            logger.warning("failed to record routing outcome after retries: %s", exc, exc_info=True)


def _build_correction_hint(tool_name: str, error: str) -> str:
    """Generate a correction hint for common tool argument errors.

    Returns an empty string if no specific hint applies.
    """
    error_lower = error.lower()

    if "not found" in error_lower or "no such file" in error_lower:
        return (
            f"Hint: The file or path was not found. Use list_directory or glob_files "
            f"to verify the correct path before retrying {tool_name}."
        )

    if "path traversal" in error_lower:
        return "Hint: Use paths relative to the workspace root. Do not use absolute paths or '..'."

    if tool_name == "edit_file":
        if "not found in file" in error_lower:
            return (
                "Hint: The old_text was not found. Use read_file to view the current file "
                "content and copy the exact text (including whitespace and indentation)."
            )
        if "found" in error_lower and "times" in error_lower:
            return (
                "Hint: The old_text matches multiple locations. Include more surrounding "
                "context lines in old_text to make it unique."
            )

    if "timed out" in error_lower:
        return "Hint: The command timed out. Try increasing the timeout parameter or breaking it into smaller steps."

    if "permission denied" in error_lower:
        return "Hint: Permission denied. Check if the file exists and is writable."

    # Generic argument error patterns.
    if "missing" in error_lower and ("required" in error_lower or "argument" in error_lower):
        return f"Hint: A required argument is missing. Check the {tool_name} tool definition for required parameters."

    return ""


def _resolve_schema(name: str) -> type | None:
    """Resolve a schema name to the corresponding Pydantic model class from codeforge.schemas."""
    import codeforge.schemas as _schemas_mod

    return getattr(_schemas_mod, name, None)


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
