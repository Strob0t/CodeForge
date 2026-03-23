"""Core agentic loop: LLM calls tools, tools execute, results feed back.

This is the heart of the interactive agent. The loop:
1. Calls the LLM with the current message history and available tools.
2. If the LLM returns tool_calls, executes each one (with policy checks).
3. Appends results to the message history and repeats.
4. If the LLM returns text only (finish_reason="stop"), the loop ends.
"""

from __future__ import annotations

import asyncio
import hashlib
import json
import logging
import math
import os
import time
from collections import Counter, deque
from dataclasses import dataclass, field
from datetime import UTC, datetime
from difflib import SequenceMatcher
from typing import TYPE_CHECKING

from opentelemetry import trace
from opentelemetry.trace import StatusCode

from codeforge.json_utils import safe_json_loads
from codeforge.llm import LLMError, classify_error_type, is_fallback_eligible
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
from codeforge.routing.rate_tracker import RateLimitTracker, get_tracker
from codeforge.tools.capability import TOOLS_BY_CAPABILITY, CapabilityLevel
from codeforge.tracing import metrics as otel_metrics
from codeforge.tracing import tracing_manager

if TYPE_CHECKING:
    from codeforge.llm import ChatCompletionResponse, LiteLLMClient, ToolCallPart
    from codeforge.memory.experience import ExperiencePool
    from codeforge.plan_act import PlanActController
    from codeforge.routing.models import ComplexityTier, RoutingConfig, RoutingMetadata
    from codeforge.runtime import RuntimeClient
    from codeforge.tools import ToolRegistry
    from codeforge.tools._base import ToolResult as _ToolResultType

logger = logging.getLogger(__name__)

_tracer = tracing_manager.get_tracer()

STALL_ESCAPE_PROMPT = (
    "<SYSTEM: You are repeating the same action without progress. "
    "Stop and try a fundamentally different approach. If you were reading, "
    "start writing. If you were searching, use what you found.>"
)

DEFAULT_MAX_ITERATIONS = 50


class StallDetector:
    """Detect when the agent repeats the same tool call and force escape.

    Maintains a sliding window of recent ``(tool_name, args_hash)`` tuples.
    If *stall_threshold* or more entries in the last *window_size* are
    identical, the agent is considered stalled.  After two escape attempts
    the detector signals that the loop should abort.
    """

    def __init__(self, window_size: int = 5, stall_threshold: int = 3) -> None:
        self._window: deque[tuple[str, str]] = deque(maxlen=window_size)
        self._threshold = stall_threshold
        self._escape_count = 0

    @staticmethod
    def _hash_args(name: str, args: dict[str, object]) -> str:
        raw = json.dumps(args, sort_keys=True, default=str)
        return hashlib.sha256(f"{name}:{raw}".encode()).hexdigest()

    def record(self, tool_name: str, args: dict[str, object]) -> None:
        """Append a tool call to the sliding window."""
        self._window.append((tool_name, self._hash_args(tool_name, args)))

    def is_stalled(self) -> bool:
        """Return True if >= threshold entries in the window are identical."""
        if len(self._window) < self._threshold:
            return False
        counts = Counter(self._window)
        return counts.most_common(1)[0][1] >= self._threshold

    def get_repeated_action(self) -> str | None:
        """Return the tool name of the most-repeated action, or None."""
        if not self._window:
            return None
        counts = Counter(self._window)
        entry, count = counts.most_common(1)[0]
        if count >= self._threshold:
            return entry[0]  # tool_name from (tool_name, args_hash)
        return None

    def record_escape(self) -> None:
        """Record that an escape prompt was injected."""
        self._escape_count += 1

    def should_abort(self) -> bool:
        """Return True if the loop should abort (>= 2 escape attempts)."""
        return self._escape_count >= 2

    def get_abort_info(self) -> dict[str, object]:
        """Return structured info about the stall for error reporting."""
        return {
            "repeated_action": self.get_repeated_action(),
            "escape_count": self._escape_count,
        }


class _ToolErrorTracker:
    """Tracks per-tool error counts to prevent retry loops.

    When a tool produces the same error twice (after normalization), it is
    marked as NON-RETRYABLE and the agent receives a block message telling
    it to stop retrying and move on.
    """

    __slots__ = ("_counts", "_max_identical")

    def __init__(self, max_identical: int = 2) -> None:
        self._counts: dict[tuple[str, str], int] = {}  # (tool, error_sig) -> count
        self._max_identical = max_identical

    def record_error(self, tool_name: str, error: str) -> bool:
        """Record an error. Returns True if this is NON-RETRYABLE (exceeded max)."""
        sig = self._normalize_error(error)
        key = (tool_name, sig)
        self._counts[key] = self._counts.get(key, 0) + 1
        return self._counts[key] >= self._max_identical

    def get_block_message(self, tool_name: str) -> str:
        """Return a message telling the agent this tool is blocked."""
        return (
            f"[NON-RETRYABLE] Tool '{tool_name}' has failed {self._max_identical} times "
            f"with the same error. Do NOT retry. Continue with your main task "
            f"using read_file, write_file, or bash."
        )

    @staticmethod
    def _normalize_error(error: str) -> str:
        """Strip variable parts (line numbers, paths, UUIDs) for comparison."""
        import re

        # Strip UUIDs before numbers so hex digits are caught.
        s = re.sub(r"[0-9a-f]{8}-[0-9a-f]{4}", "UUID", error)
        s = re.sub(r"\d+", "N", s)
        return s[:200]


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
    plan_act_enabled: bool = False
    extra_plan_tools: frozenset[str] = field(default_factory=frozenset)
    rollout_id: int = -1  # -1 = not a rollout
    routing_metadata: RoutingMetadata | None = None  # RoutingMetadata from initial route
    routing_config: RoutingConfig | None = None  # RoutingConfig instance (active config for reward computation)
    capability_level: str = "full"  # CapabilityLevel value for tool filtering
    mode_tools: frozenset[str] = field(default_factory=frozenset)  # Mode-declared extra tools
    top_p: float | None = None  # Sampling top_p (None = provider default)
    extra_body: dict[str, object] | None = None  # Extra LiteLLM payload params


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
    quality_tracker: IterationQualityTracker | None = None  # set at runtime


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
        experience_pool: ExperiencePool | None = None,
    ) -> None:
        self._llm = llm
        self._tools = tool_registry
        self._runtime = runtime
        self._workspace = workspace_path
        self._experience_pool = experience_pool

    @staticmethod
    def _filter_tools_for_capability(
        tools_array: list[dict[str, object]],
        capability: CapabilityLevel,
        mode_tools: frozenset[str] | None = None,
    ) -> list[dict[str, object]]:
        """Filter tools based on model capability level.

        FULL capability returns all tools (no filtering).
        For weaker models, only the allowlisted tools plus any mode-declared
        tools are kept.
        """
        allowed = TOOLS_BY_CAPABILITY.get(capability, frozenset())
        if not allowed:  # FULL capability = no filtering
            return tools_array
        # Merge mode-declared tools so they are always available.
        if mode_tools:
            allowed = allowed | mode_tools
        return [t for t in tools_array if t.get("function", {}).get("name") in allowed]

    @staticmethod
    def _extract_user_prompt(messages: list[dict[str, object]]) -> str:
        """Extract the last user message content from the conversation."""
        for msg in reversed(messages):
            if msg.get("role") == "user":
                content = msg.get("content", "")
                if isinstance(content, str):
                    return content
                if isinstance(content, list):
                    text_parts = [p.get("text", "") for p in content if isinstance(p, dict) and p.get("type") == "text"]
                    return " ".join(text_parts).strip()
                return str(content)
        return ""

    async def _publish_routing_decision(self, cfg: LoopConfig) -> None:
        """Publish a trajectory.routing_decision event if routing is active (C1.7)."""
        if not cfg.routing_layer:
            return

        event: dict[str, object] = {
            "event_type": "trajectory.routing_decision",
            "selected_model": cfg.model,
            "complexity_tier": cfg.complexity_tier,
            "task_type": cfg.task_type,
            "routing_layer": cfg.routing_layer,
            "reason": "",
            "alternatives": [],
            "timestamp": datetime.now(UTC).isoformat(),
        }

        # Enrich from RoutingMetadata if available.
        metadata = cfg.routing_metadata
        if metadata is not None:
            event["reason"] = getattr(metadata, "reason", "")
            event["mab_score"] = getattr(metadata, "mab_score", 0.0)
            raw_alts = getattr(metadata, "alternatives", ())
            event["alternatives"] = [dict(a) for a in raw_alts] if raw_alts else []
        else:
            event["reason"] = f"Routed via {cfg.routing_layer} layer"

        try:
            await self._runtime.publish_trajectory_event(event)
        except Exception as exc:
            logger.debug("failed to publish routing_decision trajectory event: %s", exc)

    @staticmethod
    def _validate_model_name(model: str) -> bool:
        """Validate that model name has exactly ``provider/model`` format."""
        parts = model.split("/")
        return len(parts) == 2 and all(p.strip() for p in parts)

    @staticmethod
    def _pick_next_fallback(
        cfg: LoopConfig,
        state: _LoopState,
        rate_tracker: RateLimitTracker | None = None,
    ) -> str | None:
        """Return the next untried fallback model, or None if exhausted.

        Skips models whose provider is currently rate-limited (via the rate
        tracker) or whose name is not in ``provider/model`` format to avoid
        wasting time on providers that will 429 again.
        """
        for m in cfg.fallback_models:
            if m in state.failed_models:
                continue
            if not AgentLoopExecutor._validate_model_name(m):
                logger.warning("skipping fallback model with invalid format: %r", m)
                continue
            if rate_tracker is not None:
                provider = m.split("/", 1)[0] if "/" in m else ""
                if provider and rate_tracker.is_exhausted(provider):
                    continue
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
        # Mark the provider as exhausted in the rate tracker so the HybridRouter
        # skips it on subsequent calls within the same conversation.
        tracker = get_tracker()
        error_type = classify_error_type(exc)
        if error_type:
            provider = failed_model.split("/", 1)[0] if "/" in failed_model else failed_model
            tracker.record_error(provider, error_type=error_type)
        if exc.status_code in (401, 403):
            get_blocklist().block_auth(failed_model, reason=f"HTTP {exc.status_code}")
        next_model = self._pick_next_fallback(cfg, state, rate_tracker=tracker)
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
                routing_config=cfg.routing_config,
            )
        return await self._try_model_fallback(cfg, state, exc)

    async def _check_experience_cache(
        self,
        user_prompt: str,
        model: str,
    ) -> AgentLoopResult | None:
        """Return a cached result from the experience pool, or None."""
        if not self._experience_pool or not user_prompt:
            return None
        try:
            cached = await self._experience_pool.lookup(user_prompt, self._runtime.project_id)
            if cached:
                logger.info(
                    "experience cache hit in agent loop, entry_id=%s similarity=%.3f",
                    cached["id"],
                    cached["similarity"],
                )
                return AgentLoopResult(
                    final_content=cached["result_output"],
                    tool_messages=[],
                    total_cost=0.0,
                    total_tokens_in=0,
                    total_tokens_out=0,
                    step_count=0,
                    model=model,
                    error="",
                )
        except (ConnectionError, TimeoutError, OSError) as exc:
            logger.warning("experience cache lookup failed (transient): %s", exc)
        except ValueError as exc:
            logger.error("experience cache data corruption: %s", exc)
        except Exception as exc:
            logger.error("unexpected experience cache error: %s", type(exc).__name__, exc_info=True)
        return None

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
        quality_tracker = IterationQualityTracker()
        state = _LoopState(model=cfg.model, quality_tracker=quality_tracker)
        stall_detector = StallDetector()
        error_tracker = _ToolErrorTracker()

        # Check experience pool cache before starting the loop
        user_prompt = self._extract_user_prompt(messages)
        cached_result = await self._check_experience_cache(user_prompt, cfg.model)
        if cached_result is not None:
            return cached_result

        plan_act = _init_plan_act(cfg, messages)
        tools_array = self._tools.get_openai_tools()

        # Filter tools based on model capability level (M1).
        cap_level = CapabilityLevel(cfg.capability_level) if cfg.capability_level else CapabilityLevel.FULL
        tools_array = self._filter_tools_for_capability(tools_array, cap_level, cfg.mode_tools or None)

        loop_start = time.monotonic()

        # Publish initial routing decision as trajectory event (C1.7).
        await self._publish_routing_decision(cfg)

        for iteration in range(cfg.max_iterations):
            otel_metrics.loop_iterations.add(1)
            if self._runtime.is_cancelled:
                state.error = "cancelled"
                break

            # Stall detection check.
            if await self._check_stall(stall_detector, messages, state):
                break

            # Check mid-loop model switch before LLM call (C1).
            _check_model_switch(quality_tracker, cfg)

            # Auto-transition from plan to act when max plan iterations reached.
            _check_plan_act_transition(plan_act, messages)

            result = await self._do_llm_iteration(
                cfg, tools_array, messages, state, iteration, plan_act=plan_act, error_tracker=error_tracker
            )
            if result is not None:
                # result is True for "stop" (final text), string for error.
                if isinstance(result, str):
                    state.error = result
                break

            # Record tool calls in the stall detector from the latest step.
            self._record_tool_calls_for_stall(state, stall_detector)

            # End iteration for quality tracking (C1).
            quality_tracker.end_iteration()

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

        # Store successful result in experience pool
        if self._experience_pool and not state.error and state.final_content and user_prompt:
            try:
                await self._experience_pool.store(
                    task_desc=user_prompt,
                    project_id=self._runtime.project_id,
                    result_output=state.final_content,
                    result_cost=state.total_cost,
                    result_status="completed",
                    run_id=self._runtime.run_id,
                )
            except Exception as exc:
                logger.warning("experience pool store failed: %s", exc)

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

    async def _check_stall(
        self,
        stall_detector: StallDetector,
        messages: list[dict[str, object]],
        state: _LoopState,
    ) -> bool:
        """Check for stall and handle abort or escape injection.

        Returns True if the loop should break (abort), False otherwise.
        """
        if stall_detector.should_abort():
            abort_info = stall_detector.get_abort_info()
            state.error = (
                f"stall detected: repeated {abort_info['repeated_action']} "
                f"after {abort_info['escape_count']} escape attempts"
            )
            logger.warning("agent loop aborted due to stall: %s", state.error)
            try:
                await self._runtime.publish_trajectory_event(
                    {
                        "event_type": "stall_detected",
                        "repeated_action": abort_info["repeated_action"],
                        "escape_count": abort_info["escape_count"],
                        "timestamp": datetime.now(UTC).isoformat(),
                    }
                )
            except Exception as exc:
                logger.debug("failed to publish stall_detected trajectory event: %s", exc)
            return True

        if stall_detector.is_stalled():
            logger.info(
                "stall detected (repeated %s), injecting escape prompt",
                stall_detector.get_repeated_action(),
            )
            messages.append({"role": "user", "content": STALL_ESCAPE_PROMPT})
            stall_detector.record_escape()

        return False

    async def _do_llm_iteration(
        self,
        cfg: LoopConfig,
        tools_array: list[dict[str, object]],
        messages: list[dict[str, object]],
        state: _LoopState,
        iteration: int,
        plan_act: PlanActController | None = None,
        error_tracker: _ToolErrorTracker | None = None,
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
            sanitize_tool_messages(messages)
            try:
                response = await self._llm.chat_completion_stream(
                    messages=messages,
                    model=model_name,
                    tools=tools_array or None,
                    temperature=cfg.temperature,
                    tags=cfg.tags or None,
                    on_chunk=_on_chunk,
                    provider_api_key=cfg.provider_api_key,
                    top_p=cfg.top_p,
                    extra_body=cfg.extra_body,
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

        return await self._process_llm_response(
            cfg,
            state,
            response,
            llm_decision,
            full_text,
            messages,
            plan_act=plan_act,
            error_tracker=error_tracker,
        )

    async def _process_llm_response(
        self,
        cfg: LoopConfig,
        state: _LoopState,
        response: ChatCompletionResponse,
        llm_decision: ToolCallDecision,
        full_text: str,
        messages: list[dict[str, object]],
        plan_act: PlanActController | None = None,
        error_tracker: _ToolErrorTracker | None = None,
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
                routing_config=cfg.routing_config,
            )

        if not response.tool_calls:
            state.final_content = response.content
            return True

        assistant_msg = _build_assistant_message(response)
        state.tool_messages.append(assistant_msg)
        messages.append(_payload_to_dict(assistant_msg))

        for i, tc in enumerate(response.tool_calls):
            state.step_count += 1

            # Plan/Act phase gate: handle transition_to_act tool or block disallowed tools.
            if plan_act is not None and plan_act.enabled:
                if tc.name == "transition_to_act":
                    plan_act.transition_to_act()
                    logger.info("plan/act: transitioned to act phase via tool call")
                    _update_system_suffix(messages, plan_act.get_system_suffix())
                    self._append_tool_result(
                        tc, "Transitioned to ACT phase. All tools are now available.", messages, state
                    )
                    continue
                if not plan_act.is_tool_allowed(tc.name):
                    blocked_msg = (
                        f"Tool '{tc.name}' is not available in PLAN phase. "
                        "Only read-only tools (read_file, search_files, glob_files, list_directory) are allowed. "
                        "Call 'transition_to_act' when your plan is ready."
                    )
                    self._append_tool_result(tc, blocked_msg, messages, state)
                    continue

            await self._execute_tool_call(
                tc, messages, state, quality_tracker=state.quality_tracker, error_tracker=error_tracker
            )
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
        quality_tracker: IterationQualityTracker | None = None,
        error_tracker: _ToolErrorTracker | None = None,
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

            result_text = _build_tool_result_text(result, tc.name, error_tracker)
            if not result.success:
                tool_span.set_status(StatusCode.ERROR, result.error or "Tool returned an error")
            self._append_tool_result(tc, result_text, messages, state)
            await self._runtime.report_tool_result(
                call_id=decision.call_id,
                tool=tc.name,
                success=result.success,
                output=result.output[:500] if result.output else "",
                error=result.error,
                diff=result.diff,
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

            # Record tool outcome for quality tracking (C1).
            qt = quality_tracker or (state.quality_tracker if state else None)
            if qt is not None:
                qt.record(tool_success=result.success, output_length=len(result_text))

            # Emit action suggestion after file-modifying tool calls.
            if result.success and tc.name in ("edit_file", "write_file"):
                try:
                    await self._runtime.publish_trajectory_event(
                        {
                            "event_type": "agent.action_suggestion",
                            "label": "Run tests",
                            "action": "send_message",
                            "value": "Run the test suite to verify the changes",
                        }
                    )
                except Exception as exc:
                    logger.debug("failed to publish action_suggestion event: %s", exc)

    @staticmethod
    def _record_tool_calls_for_stall(
        state: _LoopState,
        stall_detector: StallDetector,
    ) -> None:
        """Record the most recent tool calls in the stall detector.

        Examines tool_messages from the end to find the latest batch of
        tool-result messages (preceded by an assistant message with tool_calls).
        """
        # Walk backwards through tool_messages to find the latest assistant
        # message with tool_calls, then record each tool call.
        for msg in reversed(state.tool_messages):
            if msg.role == "assistant" and msg.tool_calls:
                for tc in msg.tool_calls:
                    args: dict[str, object] = (
                        safe_json_loads(tc.function.arguments, {}) if tc.function.arguments else {}
                    )
                    stall_detector.record(tc.function.name, args)
                break

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


# ---------------------------------------------------------------------------
# C1.6 — IterationQualityTracker (routing transparency + mid-loop model switch)
# ---------------------------------------------------------------------------

_QUALITY_WINDOW = 3
_LOW_QUALITY_THRESHOLD = 0.3
_MIN_MEANINGFUL_OUTPUT = 50


class IterationQualityTracker:
    """Track per-iteration quality signals for mid-loop model switching (C1)."""

    MAX_SWITCHES: int = 2

    def __init__(self) -> None:
        self._records: deque[float] = deque(maxlen=_QUALITY_WINDOW)
        self._iteration_signals: list[float] = []
        self.switch_count: int = 0

    def record(self, tool_success: bool, output_length: int) -> None:
        """Record a single tool call outcome within the current iteration."""
        if tool_success and output_length >= _MIN_MEANINGFUL_OUTPUT:
            self._records.append(1.0)
        elif tool_success:
            self._records.append(0.0)  # success but empty/short output
        else:
            self._records.append(0.0)

    def signal(self) -> float:
        """Compute quality signal from the last N tool calls. 0.5 if no data."""
        if not self._records:
            return 0.5
        return sum(self._records) / len(self._records)

    def end_iteration(self) -> None:
        """Mark end of an iteration, recording its signal for consecutive-low tracking."""
        self._iteration_signals.append(self.signal())
        self._records.clear()

    def should_switch(self) -> bool:
        """Return True if 2+ consecutive iterations had low quality AND switches remain."""
        if self.switch_count >= self.MAX_SWITCHES:
            return False
        if len(self._iteration_signals) < 2:
            return False
        last_two = self._iteration_signals[-2:]
        return all(s < _LOW_QUALITY_THRESHOLD for s in last_two)

    def register_switch(self) -> None:
        """Record that a model switch occurred."""
        self.switch_count += 1
        self._iteration_signals.clear()

    @staticmethod
    def bump_tier(current: ComplexityTier) -> ComplexityTier:
        """Bump complexity tier by one level, capping at REASONING."""
        from codeforge.routing.models import ComplexityTier

        order = [ComplexityTier.SIMPLE, ComplexityTier.MEDIUM, ComplexityTier.COMPLEX, ComplexityTier.REASONING]
        try:
            idx = order.index(current)
        except ValueError:
            return ComplexityTier.MEDIUM
        return order[min(idx + 1, len(order) - 1)]


# ---------------------------------------------------------------------------
# A4 — Inference-Time Scaling helpers
# ---------------------------------------------------------------------------


async def _snapshot_workspace(workspace_path: str, rollout_id: int) -> None:
    """Snapshot workspace state via git stash."""
    proc = await asyncio.create_subprocess_exec(
        "git",
        "stash",
        "push",
        "-m",
        f"rollout-{rollout_id}",
        "--include-untracked",
        cwd=workspace_path,
        stdout=asyncio.subprocess.PIPE,
        stderr=asyncio.subprocess.PIPE,
    )
    await proc.communicate()


async def _restore_workspace(workspace_path: str) -> None:
    """Restore workspace state via git checkout + clean."""
    proc = await asyncio.create_subprocess_exec(
        "git",
        "checkout",
        ".",
        cwd=workspace_path,
        stdout=asyncio.subprocess.PIPE,
        stderr=asyncio.subprocess.PIPE,
    )
    await proc.communicate()
    proc = await asyncio.create_subprocess_exec(
        "git",
        "clean",
        "-fd",
        cwd=workspace_path,
        stdout=asyncio.subprocess.PIPE,
        stderr=asyncio.subprocess.PIPE,
    )
    await proc.communicate()


def _compute_rollout_score(result: AgentLoopResult) -> float:
    """Score a single rollout result using quality metrics.

    Scoring formula (0.0-1.0):
    - 0.0 if the result has an error
    - Otherwise: weighted combination of content length, step count, and tool usage
    """
    if result.error:
        return 0.0

    content_len = len(result.final_content)
    if content_len == 0:
        return 0.0

    # Content quality: logarithmic scale, capped at 1.0
    # Short outputs (<50 chars) score low; 500+ chars score near 1.0
    content_score = min(1.0, math.log1p(content_len) / math.log1p(500))

    # Step efficiency: penalize excessive steps (>30 steps starts to reduce score)
    max_efficient_steps = 30
    step_score = min(1.0, result.step_count / max_efficient_steps) if result.step_count > 0 else 0.1

    # Tool usage: having tool messages indicates productive work
    tool_score = min(1.0, len(result.tool_messages) / 5) if result.tool_messages else 0.0

    # Weighted combination: content is most important, then tool usage, then step efficiency
    return 0.5 * content_score + 0.3 * tool_score + 0.2 * step_score


def _select_best_rollout(
    results: list[AgentLoopResult],
    scores: list[float],
) -> int:
    """Select the best rollout index by score, excluding errored results."""
    best_idx = 0
    best_score = -1.0
    for i, (result, score) in enumerate(zip(results, scores, strict=True)):
        has_error = bool(getattr(result, "error", ""))
        effective_score = score if not has_error else -1.0
        if effective_score > best_score:
            best_score = effective_score
            best_idx = i
    return best_idx


def _should_early_stop(
    outputs: list[str],
    exit_codes: list[int],
    total_rollouts: int,
    threshold: float = 0.9,
    quorum: int = 3,
) -> bool:
    """Return True if enough rollouts agree to stop early."""
    if total_rollouts <= 3:
        return False
    if len(outputs) < quorum:
        return False

    # Check if quorum of outputs are similar AND all have exit_code == 0.
    n = len(outputs)
    for i in range(n):
        if exit_codes[i] != 0:
            continue
        cluster = [i]
        for j in range(i + 1, n):
            if exit_codes[j] != 0:
                continue
            sim = SequenceMatcher(None, outputs[i], outputs[j]).ratio()
            if sim >= threshold:
                cluster.append(j)
        if len(cluster) >= quorum:
            return True
    return False


_MAX_ROLLOUT_COUNT = 8


class ConversationRolloutExecutor:
    """Multi-rollout wrapper for agent loop conversations (A4)."""

    def __init__(
        self,
        agent_loop_executor: AgentLoopExecutor,
        rollout_count: int,
        workspace_path: str,
        runtime: RuntimeClient | None = None,
    ) -> None:
        self._executor = agent_loop_executor
        # Clamp: 0 or negative -> 1, cap at _MAX_ROLLOUT_COUNT.
        self._rollout_count = max(1, min(rollout_count, _MAX_ROLLOUT_COUNT))
        self._workspace = workspace_path
        self._runtime = runtime

    async def execute(
        self,
        messages: list[dict[str, object]],
        config: LoopConfig,
    ) -> AgentLoopResult:
        """Execute rollouts and return the best result."""
        # Non-git workspace: fall back to single rollout.
        if self._rollout_count > 1 and not os.path.isdir(os.path.join(self._workspace, ".git")):
            logger.warning("rollout requested but no .git found, falling back to single")
            result = await self._executor.run(messages, config=config)
            result.metadata = {"fallback_reason": "no_git_repo"}
            return result

        # Single rollout: pass through directly.
        if self._rollout_count <= 1:
            return await self._executor.run(messages, config=config)

        results: list[AgentLoopResult] = []
        outputs: list[str] = []
        exit_codes: list[int] = []
        total_cost = 0.0
        total_tokens_in = 0
        total_tokens_out = 0
        early_stopped = False

        for rollout_id in range(self._rollout_count):
            if rollout_id > 0:
                await _restore_workspace(self._workspace)

            # Set rollout_id on config for tracking.
            config.rollout_id = rollout_id

            await _snapshot_workspace(self._workspace, rollout_id)
            result = await self._executor.run(list(messages), config=config)
            results.append(result)
            outputs.append(result.final_content)
            exit_codes.append(1 if result.error else 0)
            total_cost += result.total_cost
            total_tokens_in += result.total_tokens_in
            total_tokens_out += result.total_tokens_out

            # Early stopping check.
            if _should_early_stop(outputs, exit_codes, self._rollout_count):
                logger.info("early stop at rollout %d/%d", rollout_id + 1, self._rollout_count)
                early_stopped = True
                break

        # Score rollouts using quality tracker data and result metrics.
        scores = [_compute_rollout_score(r) for r in results]
        best_idx = _select_best_rollout(results, scores)
        best = results[best_idx]

        # Publish trajectory event with rollout metadata.
        await self._publish_rollout_trajectory(
            total_rollouts=len(results),
            selected_index=best_idx,
            scores=scores,
            early_stopped=early_stopped,
        )

        return AgentLoopResult(
            final_content=best.final_content,
            tool_messages=best.tool_messages,
            total_cost=total_cost,
            total_tokens_in=total_tokens_in,
            total_tokens_out=total_tokens_out,
            step_count=best.step_count,
            model=best.model,
            error=best.error,
            metadata={
                "rollout_count": len(results),
                "selected_index": best_idx,
                "scores": scores,
                "early_stopped": early_stopped,
            },
        )

    async def _publish_rollout_trajectory(
        self,
        total_rollouts: int,
        selected_index: int,
        scores: list[float],
        early_stopped: bool,
    ) -> None:
        """Publish a trajectory event summarizing rollout execution."""
        if self._runtime is None:
            return
        try:
            await self._runtime.publish_trajectory_event(
                {
                    "event_type": "trajectory.rollout_complete",
                    "total_rollouts": total_rollouts,
                    "selected_index": selected_index,
                    "scores": scores,
                    "early_stopped": early_stopped,
                }
            )
        except Exception as exc:
            logger.warning("failed to publish rollout trajectory event: %s", exc)


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
    routing_config: RoutingConfig | None = None,
) -> None:
    """Post a routing outcome to Go Core for MAB learning. Fire-and-forget."""
    import httpx

    from codeforge.config import get_settings
    from codeforge.routing.models import RoutingConfig
    from codeforge.routing.reward import compute_reward

    quality = 1.0 if success else 0.0
    config = routing_config if routing_config is not None else RoutingConfig()
    reward = compute_reward(success, quality, cost_usd, latency_ms, config)
    settings = get_settings()
    core_url = settings.core_url
    internal_key = settings.internal_key
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


def _build_tool_result_text(
    result: _ToolResultType,
    tool_name: str,
    error_tracker: _ToolErrorTracker | None,
) -> str:
    """Build the result text for a tool call, including correction hints and error tracking."""
    if result.success:
        return result.output
    if not result.error:
        return "Tool returned an error"
    correction = _build_correction_hint(tool_name, result.error)
    text = f"Error: {result.error}\n\n{correction}" if correction else f"Error: {result.error}"
    # Track repeated errors and block if NON-RETRYABLE (M5).
    if error_tracker is not None and error_tracker.record_error(tool_name, result.error):
        text = error_tracker.get_block_message(tool_name)
    return text


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
    if msg.role == "tool":
        # Tool messages MUST always include 'content' — some providers (e.g. Groq)
        # reject messages with role:tool missing the content field.
        d["content"] = msg.content or ""
    elif msg.content:
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


def sanitize_tool_messages(messages: list[dict[str, object]]) -> list[dict[str, object]]:
    """Normalize tool messages for cross-provider compatibility.

    Ensures every ``role:tool`` message has a non-empty ``content`` field and
    a ``tool_call_id``.  Some providers (e.g. Groq) reject tool messages that
    are missing these fields, even though other providers (e.g. Gemini) may
    omit them.
    """
    for msg in messages:
        if msg.get("role") != "tool":
            continue
        if "content" not in msg or msg["content"] is None:
            msg["content"] = ""
        if "tool_call_id" not in msg or not msg["tool_call_id"]:
            msg["tool_call_id"] = f"_sanitized_{id(msg)}"
    return messages


# --- Plan/Act helpers ---


def _init_plan_act(cfg: LoopConfig, messages: list[dict[str, object]]) -> PlanActController:
    """Initialize the Plan/Act controller and inject the system prompt suffix."""
    from codeforge.plan_act import PlanActController, get_max_plan_iterations

    plan_act = PlanActController(
        enabled=cfg.plan_act_enabled,
        max_plan_iterations=get_max_plan_iterations(),
        extra_plan_tools=cfg.extra_plan_tools,
    )
    suffix = plan_act.get_system_suffix()
    if suffix:
        _append_system_suffix(messages, suffix)
    return plan_act


_PLAN_ACT_MARKER = "\n\nYou are in "


def _check_model_switch(quality_tracker: IterationQualityTracker, cfg: LoopConfig) -> None:
    """Bump complexity tier and log if quality tracker recommends a model switch (C1)."""
    if not quality_tracker.should_switch() or not cfg.routing_layer:
        return
    from codeforge.routing.models import ComplexityTier

    old_tier = cfg.complexity_tier
    new_tier = quality_tracker.bump_tier(ComplexityTier(old_tier) if old_tier else ComplexityTier.SIMPLE)
    quality_tracker.register_switch()
    cfg.complexity_tier = str(new_tier)
    logger.info(
        "mid-loop model switch: tier %s -> %s (switch #%d)",
        old_tier,
        new_tier,
        quality_tracker.switch_count,
    )


def _check_plan_act_transition(plan_act: PlanActController, messages: list[dict[str, object]]) -> None:
    """Auto-transition from plan to act when max plan iterations reached."""
    if plan_act.tick_and_should_transition():
        plan_act.transition_to_act()
        logger.info("plan/act auto-transition to act phase after %d plan iterations", plan_act.plan_iterations)
        _update_system_suffix(messages, plan_act.get_system_suffix())


def _append_system_suffix(messages: list[dict[str, object]], suffix: str) -> None:
    """Append plan/act suffix to the first system message."""
    for msg in messages:
        if msg.get("role") == "system":
            content = msg.get("content", "")
            msg["content"] = str(content) + suffix if content else suffix
            return


def _update_system_suffix(messages: list[dict[str, object]], new_suffix: str) -> None:
    """Replace plan/act suffix on the first system message (phase transition)."""
    for msg in messages:
        if msg.get("role") == "system":
            content = str(msg.get("content", ""))
            idx = content.find(_PLAN_ACT_MARKER)
            if idx >= 0:
                content = content[:idx]
            msg["content"] = content + new_suffix
            return
