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
import os
import time
from dataclasses import dataclass, field
from datetime import UTC, datetime
from typing import TYPE_CHECKING, Literal

import httpx
from opentelemetry import trace
from opentelemetry.trace import StatusCode

from codeforge.json_utils import safe_json_loads
from codeforge.llm import LLMError, classify_error_type, is_fallback_eligible
from codeforge.loop_helpers import (
    ToolErrorTracker,
    build_assistant_message,
    check_model_switch,
    check_plan_act_transition,
    init_plan_act,
    payload_to_dict,
    resolve_schema,
    sanitize_tool_messages,
    update_system_suffix,
)
from codeforge.model_resolver import resolve_model
from codeforge.models import (
    AgentLoopResult,
    ConversationMessagePayload,
)
from codeforge.pricing import resolve_cost
from codeforge.quality_tracking import (
    IterationQualityTracker,
    compute_rollout_score,
    select_best_rollout,
    should_early_stop,
)
from codeforge.routing.blocklist import get_blocklist
from codeforge.routing.rate_tracker import RateLimitTracker, get_tracker
from codeforge.stall_detection import StallDetector
from codeforge.tool_executor import ToolExecutor
from codeforge.tools.capability import TOOLS_BY_CAPABILITY, CapabilityLevel
from codeforge.tracing import metrics as otel_metrics
from codeforge.tracing import tracing_manager

if TYPE_CHECKING:
    from codeforge.llm import ChatCompletionResponse, LiteLLMClient, ToolCallPart
    from codeforge.memory.experience import ExperiencePool
    from codeforge.models import ToolCallDecision
    from codeforge.plan_act import PlanActController
    from codeforge.routing.models import RoutingConfig, RoutingMetadata
    from codeforge.runtime import RuntimeClient
    from codeforge.tools import ToolRegistry

logger = logging.getLogger(__name__)

_tracer = tracing_manager.get_tracer()


# ---------------------------------------------------------------------------
# F-QUA-008: Typed iteration outcomes
# ---------------------------------------------------------------------------


@dataclass(frozen=True, slots=True)
class IterationStop:
    """LLM returned final content (no tool calls). Loop should end."""

    kind: Literal["stop"] = "stop"


@dataclass(frozen=True, slots=True)
class IterationContinue:
    """Tool calls processed. Loop should continue."""

    kind: Literal["continue"] = "continue"


@dataclass(frozen=True, slots=True)
class IterationError:
    """An error occurred. Loop should end with error."""

    message: str
    kind: Literal["error"] = "error"


IterationOutcome = IterationStop | IterationContinue | IterationError

MEMORY_THRESHOLD_MB = int(os.getenv("CODEFORGE_WORKER_MEMORY_THRESHOLD_MB", "3500"))

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
    plan_act_enabled: bool = False
    extra_plan_tools: frozenset[str] = field(default_factory=frozenset)
    rollout_id: int = -1  # -1 = not a rollout
    routing_metadata: RoutingMetadata | None = None
    routing_config: RoutingConfig | None = None
    capability_level: str = "full"
    mode_tools: frozenset[str] = field(default_factory=frozenset)
    top_p: float | None = None
    extra_body: dict[str, object] | None = None
    selected_tools: list[str] | None = None


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
    quality_tracker: IterationQualityTracker | None = None
    files_read: set[str] = field(default_factory=set)
    writes_since_verify: int = 0


# ---------------------------------------------------------------------------
# Trajectory analysis: tool-usage metrics computed at run completion.
# ---------------------------------------------------------------------------

# Tool names considered "exploration" (read-only, information-gathering).
_EXPLORE_TOOLS: frozenset[str] = frozenset(
    {
        "read_file",
        "search_files",
        "glob_files",
        "list_directory",
    }
)
# Tool names considered "write" (mutating the workspace).
_WRITE_TOOLS: frozenset[str] = frozenset({"write_file", "edit_file"})


def _compute_trajectory_metrics(
    tool_messages: list[ConversationMessagePayload],
    available_tool_count: int,
) -> dict[str, object]:
    """Derive trajectory analysis metrics from the sequence of tool messages.

    Counts are derived from tool-result messages (role="tool", name set)
    and from assistant messages that carry tool_calls.
    """
    tool_counts: dict[str, int] = {}

    for msg in tool_messages:
        # Tool-result messages have role="tool" and name set to the tool name.
        if msg.name:
            tool_counts[msg.name] = tool_counts.get(msg.name, 0) + 1
        # Assistant messages carry outgoing tool_calls (ConversationToolCallPayload).
        for tc in msg.tool_calls:
            # tc.function is a ConversationToolCallFunction with a .name field.
            func = getattr(tc, "function", None)
            fname = getattr(func, "name", "") if func else ""
            if fname:
                tool_counts[fname] = tool_counts.get(fname, 0) + 1

    total_calls = sum(tool_counts.values())
    unique_tools = len(tool_counts)
    explore_calls = sum(tool_counts.get(t, 0) for t in _EXPLORE_TOOLS)
    write_calls = sum(tool_counts.get(t, 0) for t in _WRITE_TOOLS)

    return {
        "tool_diversity": unique_tools / max(available_tool_count, 1),
        "explore_ratio": explore_calls / max(total_calls, 1),
        "write_calls": write_calls,
        "total_tool_calls": total_calls,
        "unique_tools": unique_tools,
        "tool_counts": tool_counts,
    }


class AgentLoopExecutor:
    """Executes the agentic tool-use loop."""

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
        self._tool_executor = ToolExecutor(tool_registry, runtime, workspace_path)

    _MCP_READONLY_KEYWORDS: frozenset[str] = frozenset({"search", "list", "find", "get", "fetch_url"})

    @staticmethod
    def _filter_tools_for_capability(
        tools_array: list[dict[str, object]],
        capability: CapabilityLevel,
        mode_tools: frozenset[str] | None = None,
        selected_tools: list[str] | None = None,
    ) -> list[dict[str, object]]:
        """Filter tools based on model capability level and ToolRouter selection."""
        if selected_tools is not None:
            allowed: frozenset[str] = frozenset(selected_tools)
            if mode_tools:
                allowed = allowed | mode_tools
        else:
            allowed = TOOLS_BY_CAPABILITY.get(capability, frozenset())
            if not allowed:
                return tools_array
            if mode_tools:
                allowed = allowed | mode_tools

        def _is_allowed(tool: dict[str, object]) -> bool:
            name = tool.get("function", {}).get("name", "")
            if name in allowed:
                return True
            if selected_tools is None and name.startswith("mcp__"):
                tool_action = name.rsplit("__", 1)[-1]
                return any(kw in tool_action for kw in AgentLoopExecutor._MCP_READONLY_KEYWORDS)
            return False

        return [t for t in tools_array if _is_allowed(t)]

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
        except (ConnectionError, TimeoutError, OSError) as exc:
            logger.debug("failed to publish routing_decision trajectory event: %s", exc)

    @staticmethod
    def _validate_model_name(model: str) -> bool:
        """Validate that model name has exactly ``provider/model`` format."""
        parts = model.split("/")
        return len(parts) == 2 and all(p.strip() for p in parts)

    @staticmethod
    def _check_memory_pressure(state: _LoopState) -> bool:
        """Return True if RSS exceeds MEMORY_THRESHOLD_MB (abort signal)."""
        try:
            import psutil

            rss_mb = psutil.Process().memory_info().rss // (1024 * 1024)
        except ImportError:
            return False
        if rss_mb > MEMORY_THRESHOLD_MB:
            state.error = f"Memory threshold exceeded ({rss_mb}MB > {MEMORY_THRESHOLD_MB}MB)"
            logger.warning(
                "aborting run due to memory pressure", extra={"rss_mb": rss_mb, "threshold": MEMORY_THRESHOLD_MB}
            )
            return True
        return False

    @staticmethod
    def _pick_next_fallback(
        cfg: LoopConfig,
        state: _LoopState,
        rate_tracker: RateLimitTracker | None = None,
    ) -> str | None:
        """Return the next untried fallback model, or None if exhausted."""
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

    async def _try_model_fallback(self, cfg: LoopConfig, state: _LoopState, exc: LLMError) -> str | None:
        """Attempt to switch to a fallback model. Returns error string or None (retry)."""
        if not is_fallback_eligible(exc) or not cfg.fallback_models:
            return f"LLM call failed: {exc}"
        failed_model = cfg.model
        state.failed_models.add(failed_model)
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
        logger.warning("model fallback: %s -> %s (status %d)", failed_model, next_model, exc.status_code)
        notice = f"\n[Model {failed_model} unavailable ({exc.status_code}). Switching to {next_model}]\n"
        await self._runtime.send_output(notice)
        return None

    async def _handle_llm_error(self, cfg: LoopConfig, state: _LoopState, exc: LLMError, iteration: int) -> str | None:
        """Handle an LLM error: record outcome, attempt fallback."""
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

    async def _check_experience_cache(self, user_prompt: str, model: str) -> AgentLoopResult | None:
        """Return a cached result from the experience pool, or None."""
        if not self._experience_pool or not user_prompt:
            return None
        try:
            cached = await self._experience_pool.lookup(user_prompt, self._runtime.project_id)
            if cached:
                logger.info("experience cache hit, entry_id=%s similarity=%.3f", cached["id"], cached["similarity"])
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
    async def run(self, messages: list[dict[str, object]], config: LoopConfig | None = None) -> AgentLoopResult:  # noqa: C901
        """Execute the agentic loop until the LLM stops or limits are hit."""
        cfg = config or LoopConfig()
        quality_tracker = IterationQualityTracker()
        state = _LoopState(model=cfg.model, quality_tracker=quality_tracker)
        stall_detector = StallDetector()
        error_tracker = ToolErrorTracker()

        user_prompt = self._extract_user_prompt(messages)
        cached_result = await self._check_experience_cache(user_prompt, cfg.model)
        if cached_result is not None:
            return cached_result

        plan_act = init_plan_act(cfg, messages)
        tools_array = self._tools.get_openai_tools()
        cap_level = CapabilityLevel(cfg.capability_level) if cfg.capability_level else CapabilityLevel.FULL
        tools_array = self._filter_tools_for_capability(
            tools_array, cap_level, cfg.mode_tools or None, cfg.selected_tools
        )

        # If the model doesn't support function calling, don't send tools param.
        # Tools are already injected into the system prompt via tool_guide for these models.
        if cap_level == CapabilityLevel.PURE_COMPLETION:
            tools_array = []  # Empty = won't be sent to LLM (see _build_stream_payload)

        loop_start = time.monotonic()
        await self._publish_routing_decision(cfg)

        for iteration in range(cfg.max_iterations):
            otel_metrics.loop_iterations.add(1)
            if self._runtime.is_cancelled:
                state.error = "cancelled"
                break

            # Post-write auto-verification nudge: trigger after every write
            # to enforce compile/test after each file change (prevents the
            # common failure mode where agents write many files then discover
            # cross-file inconsistencies too late to recover).
            if state.writes_since_verify >= 1:
                messages.append(
                    {
                        "role": "user",
                        "content": (
                            "[System] You just wrote/edited a file. Verify it compiles "
                            "before writing more code: run `python -m py_compile <file>` "
                            "or the appropriate build command."
                        ),
                    }
                )
                state.writes_since_verify = 0

            if await self._check_stall(stall_detector, messages, state):
                break
            if self._check_memory_pressure(state):
                break
            check_model_switch(quality_tracker, cfg)
            check_plan_act_transition(plan_act, messages)

            result = await self._do_llm_iteration(
                cfg, tools_array, messages, state, iteration, plan_act=plan_act, error_tracker=error_tracker
            )
            match result:
                case IterationStop():
                    break
                case IterationError(message=msg):
                    state.error = msg
                    break
                case IterationContinue():
                    pass

            self._record_tool_calls_for_stall(state, stall_detector)
            quality_tracker.end_iteration()

            if cfg.max_cost > 0 and state.total_cost >= cfg.max_cost:
                logger.info("cost limit reached: %.4f >= %.4f", state.total_cost, cfg.max_cost)
                break
        else:
            logger.warning("agent loop hit max iterations (%d)", cfg.max_iterations)
            state.error = f"iteration limit reached ({cfg.max_iterations})"

        otel_metrics.loop_duration.record(time.monotonic() - loop_start)

        if cfg.output_schema and state.final_content and not state.error:
            state = await self._validate_output_schema(cfg, state, messages)

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
            except (ConnectionError, TimeoutError, OSError, ValueError) as exc:
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
        except (ConnectionError, TimeoutError, OSError) as exc:
            logger.debug("failed to publish finished trajectory event: %s", exc)

        # Compute trajectory analysis metrics from tool usage patterns.
        trajectory_metrics = _compute_trajectory_metrics(state.tool_messages, len(self._tools.tool_names))
        logger.info(
            "trajectory metrics: diversity=%.2f explore=%.2f writes=%d total=%d unique=%d",
            trajectory_metrics["tool_diversity"],
            trajectory_metrics["explore_ratio"],
            trajectory_metrics["write_calls"],
            trajectory_metrics["total_tool_calls"],
            trajectory_metrics["unique_tools"],
        )

        return AgentLoopResult(
            final_content=state.final_content,
            tool_messages=state.tool_messages,
            total_cost=state.total_cost,
            total_tokens_in=state.total_tokens_in,
            total_tokens_out=state.total_tokens_out,
            step_count=state.step_count,
            model=state.model,
            error=state.error,
            metadata={"trajectory": trajectory_metrics},
        )

    async def _validate_output_schema(
        self, cfg: LoopConfig, state: _LoopState, messages: list[dict[str, object]]
    ) -> _LoopState:
        """Validate/reparse final content against the specified output schema."""
        from codeforge.schemas.parser import StructuredOutputParser

        schema_cls = resolve_schema(cfg.output_schema)
        if schema_cls is None:
            logger.warning("unknown output_schema %r, skipping validation", cfg.output_schema)
            return state

        import json as _json

        from pydantic import ValidationError

        try:
            parsed = _json.loads(state.final_content)
            schema_cls.model_validate(parsed)
            return state
        except (ValueError, ValidationError):
            pass

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
        self, stall_detector: StallDetector, messages: list[dict[str, object]], state: _LoopState
    ) -> bool:
        """Check for stall and handle abort or escape injection. Returns True to break."""
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
            except (ConnectionError, TimeoutError, OSError) as exc:
                logger.debug("failed to publish stall_detected trajectory event: %s", exc)
            return True

        if stall_detector.is_stalled():
            recent_tools = stall_detector.get_recent_tool_names()
            escape_prompt = stall_detector.get_contextual_escape_prompt(recent_tools)
            logger.info("stall detected (repeated %s), injecting escape prompt", stall_detector.get_repeated_action())
            messages.append({"role": "user", "content": escape_prompt})
            stall_detector.record_escape()
        return False

    async def _do_llm_iteration(
        self,
        cfg: LoopConfig,
        tools_array: list[dict[str, object]],
        messages: list[dict[str, object]],
        state: _LoopState,
        iteration: int,
        *,
        plan_act: PlanActController | None = None,
        error_tracker: ToolErrorTracker | None = None,
    ) -> IterationOutcome:
        """Run one LLM iteration. Returns typed IterationOutcome."""
        llm_decision = await self._runtime.request_tool_call(tool="LLM", command="chat_completion")
        if llm_decision.decision != "allow":
            logger.warning("LLM call denied by policy: %s", llm_decision.reason)
            return IterationError(f"LLM call denied: {llm_decision.reason}")

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
                err = await self._handle_llm_error(cfg, state, exc, iteration)
                return IterationError(err) if err else IterationContinue()
            except asyncio.CancelledError:
                raise
            except (httpx.HTTPError, OSError, RuntimeError, ValueError) as exc:
                llm_span.set_status(StatusCode.ERROR, str(exc))
                llm_span.record_exception(exc)
                logger.exception("LLM call failed on iteration %d (unexpected)", iteration)
                wrapped = LLMError(status_code=500, model=model_name, body=str(exc))
                err = await self._handle_llm_error(cfg, state, wrapped, iteration)
                return IterationError(err) if err else IterationContinue()
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
            iteration=iteration,
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
        *,
        iteration: int = 0,
        plan_act: PlanActController | None = None,
        error_tracker: ToolErrorTracker | None = None,
    ) -> IterationOutcome:
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
        except (ConnectionError, TimeoutError, OSError) as exc:
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
            # On first iteration of agentic run, if model returns text without tool calls,
            # re-prompt instead of stopping -- the model may have ignored the tools.
            if iteration == 0 and len(response.content) > 100:
                messages.append({"role": "assistant", "content": response.content})
                messages.append(
                    {
                        "role": "user",
                        "content": (
                            "You have tools available to complete this task. "
                            "Do NOT describe what you would do -- use the tools to actually do it. "
                            "Start by calling list_directory or read_file to explore the workspace."
                        ),
                    }
                )
                logger.warning("no tool calls on first iteration, re-prompting agent to use tools")
                return IterationContinue()
            state.final_content = response.content
            return IterationStop()

        assistant_msg = build_assistant_message(response)
        state.tool_messages.append(assistant_msg)
        messages.append(payload_to_dict(assistant_msg))

        for i, tc in enumerate(response.tool_calls):
            state.step_count += 1
            if self._apply_plan_act_gate(tc, plan_act, messages, state):
                continue

            await self._tool_executor.execute(
                tc, messages, state, quality_tracker=state.quality_tracker, error_tracker=error_tracker
            )
            self._track_write_verification(tc, state)

            if self._runtime.is_cancelled:
                for remaining_tc in response.tool_calls[i + 1 :]:
                    self._tool_executor.append_result(remaining_tc, "Cancelled", messages, state)
                break
        return IterationContinue()

    def _apply_plan_act_gate(
        self,
        tc: ToolCallPart,
        plan_act: PlanActController | None,
        messages: list[dict[str, object]],
        state: _LoopState,
    ) -> bool:
        """Handle plan/act gating for a tool call.

        Returns True if the tool call was handled (caller should ``continue``),
        False if normal execution should proceed.
        """
        if plan_act is None or not plan_act.enabled:
            return False
        if tc.name == "transition_to_act":
            plan_act.transition_to_act()
            logger.info("plan/act: transitioned to act phase via tool call")
            update_system_suffix(messages, plan_act.get_system_suffix())
            self._tool_executor.append_result(
                tc, "Transitioned to ACT phase. All tools are now available.", messages, state
            )
            return True
        if not plan_act.is_tool_allowed(tc.name):
            blocked_msg = (
                f"Tool '{tc.name}' is not available in PLAN phase. "
                "Only read-only tools (read_file, search_files, glob_files, list_directory) are allowed. "
                "Call 'transition_to_act' when your plan is ready."
            )
            self._tool_executor.append_result(tc, blocked_msg, messages, state)
            return True
        return False

    @staticmethod
    def _track_write_verification(tc: ToolCallPart, state: _LoopState) -> None:
        """Track writes and verification commands for the verify-nudge (TODO-5)."""
        if tc.name in ("write_file", "edit_file"):
            state.writes_since_verify += 1
        elif tc.name == "bash":
            cmd = (safe_json_loads(tc.arguments, {}) if tc.arguments else {}).get("command", "")
            if any(kw in cmd for kw in ("test", "pytest", "compile", "tsc", "build", "check")):
                state.writes_since_verify = 0

    @staticmethod
    def _record_tool_calls_for_stall(state: _LoopState, stall_detector: StallDetector) -> None:
        """Record the most recent tool calls in the stall detector."""
        for msg in reversed(state.tool_messages):
            if msg.role == "assistant" and msg.tool_calls:
                for tc in msg.tool_calls:
                    args: dict[str, object] = (
                        safe_json_loads(tc.function.arguments, {}) if tc.function.arguments else {}
                    )
                    stall_detector.record(tc.function.name, args)
                break


# ---------------------------------------------------------------------------
# A4 -- Inference-Time Scaling
# ---------------------------------------------------------------------------


async def _run_git(workspace_path: str, *args: str) -> None:
    """Run a git sub-command and raise on non-zero exit.

    Uses create_subprocess_exec (no shell) to avoid injection risks.
    """
    proc = await asyncio.create_subprocess_exec(
        "git",
        *args,
        cwd=workspace_path,
        stdout=asyncio.subprocess.PIPE,
        stderr=asyncio.subprocess.PIPE,
    )
    _, stderr = await proc.communicate()
    if proc.returncode:
        cmd = " ".join(args)
        raise RuntimeError(f"git {cmd} failed (exit {proc.returncode}): {stderr.decode()}")


async def _snapshot_workspace(workspace_path: str, rollout_id: int) -> None:
    """Snapshot workspace state via git stash."""
    await _run_git(workspace_path, "stash", "push", "-m", f"rollout-{rollout_id}", "--include-untracked")


async def _restore_workspace(workspace_path: str) -> None:
    """Restore workspace state via git checkout + clean."""
    await _run_git(workspace_path, "checkout", ".")
    await _run_git(workspace_path, "clean", "-fd")


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
        self._rollout_count = max(1, min(rollout_count, _MAX_ROLLOUT_COUNT))
        self._workspace = workspace_path
        self._runtime = runtime

    async def execute(self, messages: list[dict[str, object]], config: LoopConfig) -> AgentLoopResult:
        """Execute rollouts and return the best result."""
        if self._rollout_count > 1 and not os.path.isdir(os.path.join(self._workspace, ".git")):
            logger.warning("rollout requested but no .git found, falling back to single")
            result = await self._executor.run(messages, config=config)
            result.metadata = {"fallback_reason": "no_git_repo"}
            return result

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
            config.rollout_id = rollout_id
            await _snapshot_workspace(self._workspace, rollout_id)
            result = await self._executor.run(list(messages), config=config)
            results.append(result)
            outputs.append(result.final_content)
            exit_codes.append(1 if result.error else 0)
            total_cost += result.total_cost
            total_tokens_in += result.total_tokens_in
            total_tokens_out += result.total_tokens_out

            if should_early_stop(outputs, exit_codes, self._rollout_count):
                logger.info("early stop at rollout %d/%d", rollout_id + 1, self._rollout_count)
                early_stopped = True
                break

        scores = [compute_rollout_score(r) for r in results]
        best_idx = select_best_rollout(results, scores)
        best = results[best_idx]
        await self._publish_rollout_trajectory(
            total_rollouts=len(results), selected_index=best_idx, scores=scores, early_stopped=early_stopped
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
        self, total_rollouts: int, selected_index: int, scores: list[float], early_stopped: bool
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
        except (ConnectionError, TimeoutError, OSError) as exc:
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
        except (httpx.HTTPError, OSError, TimeoutError) as exc:
            if attempt == 0:
                await asyncio.sleep(1)
                continue
            logger.warning("failed to record routing outcome after retries: %s", exc, exc_info=True)
