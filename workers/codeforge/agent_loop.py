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
import re
import time
from dataclasses import dataclass, field
from datetime import UTC, datetime
from typing import TYPE_CHECKING, Literal

import httpx
from opentelemetry import trace
from opentelemetry.trace import StatusCode

from codeforge.history import DEFAULT_TOOL_OUTPUT_MAX_CHARS
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
from codeforge.policy_args import canonical_tool
from codeforge.pricing import resolve_cost
from codeforge.provider_keys import fallbacks_for_key, model_provider
from codeforge.quality_tracking import (
    IterationQualityTracker,
    compute_rollout_score,
    select_best_rollout,
    should_early_stop,
)
from codeforge.routing.blocklist import get_blocklist
from codeforge.routing.rate_tracker import RateLimitTracker, get_tracker
from codeforge.stall_detection import StallDetector, stall_error
from codeforge.subprocess_env import tool_env
from codeforge.tool_executor import ToolExecutor
from codeforge.tool_process import start_tool_process
from codeforge.tools.capability import ALWAYS_OFFERED_TOOLS, TOOLS_BY_CAPABILITY, CapabilityLevel
from codeforge.tools.text_protocol import (
    EXTRA_CALLS_NOTE,
    REPAIR_NOTICE,
    TURN_MAX_TOKENS,
    TextProtocolError,
    TextToolCall,
    TextToolProtocol,
    grammar_rejected,
    native_response,
)
from codeforge.tools.text_protocol_stream import ProtocolStreamFilter
from codeforge.tracing import metrics as otel_metrics
from codeforge.tracing import tracing_manager

if TYPE_CHECKING:
    from codeforge.llm import ChatCompletionResponse, LiteLLMClient, ToolCallPart
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
    # agent.tool_output_max_chars: tool results added in the loop are
    # truncated to it (0 = DEFAULT_TOOL_OUTPUT_MAX_CHARS).
    tool_output_max_chars: int = 0
    # A turn that implements (runs.start, the auto-agent's feature turns):
    # an announced action without a tool call is nudged once (KI-153).
    implementation_turn: bool = False
    # Pure-completion models: constrain text tool protocol replies with a
    # JSON-schema grammar (litellm.text_tool_grammar, S9-C).
    text_tool_grammar: bool = True


# An announced next step in the last part of a reply ("I will now ...",
# "Let me ...") - intent without action, which weak models end turns with.
_ANNOUNCED_ACTION = re.compile(
    r"\b(?:i will|i'll|i am going to|i'm going to|i shall|let me|let's|next,? i)\s+(?!know\b)\w+",
    re.IGNORECASE,
)
# How much of the reply's end is searched for an announcement.
_ANNOUNCEMENT_TAIL_CHARS = 400

CONTINUE_NUDGE = (
    "Continue: call the tool now instead of announcing it. If the work is already done, say so in one sentence."
)


def announces_action(content: str) -> bool:
    """Whether a reply without tool calls ends with an announced action."""
    return bool(_ANNOUNCED_ACTION.search(content.strip()[-_ANNOUNCEMENT_TAIL_CHARS:]))


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
    tool_output_max_chars: int = DEFAULT_TOOL_OUTPUT_MAX_CHARS
    nudged: bool = False  # the turn got its "continue" nudge (KI-153)
    # The text tool protocol of a pure-completion model (S9-C), and how many
    # unusable replies in a row were sent back to the model.
    tool_protocol: TextToolProtocol | None = None
    protocol_repairs: int = 0


# Unusable text protocol replies in a row the model is asked again for.
_MAX_PROTOCOL_REPAIRS = 1


@dataclass(frozen=True, slots=True)
class _LLMReply:
    """A streamed completion and the text the user saw of it."""

    response: ChatCompletionResponse
    text: str


@dataclass(frozen=True, slots=True)
class _LLMRequest:
    """What a completion request sends: messages, tools, grammar and output limit."""

    messages: list[dict[str, object]]
    tools: list[dict[str, object]] | None
    response_format: dict[str, object] | None = None
    max_tokens: int | None = None


def _llm_request(
    protocol: TextToolProtocol | None, tools_array: list[dict[str, object]], messages: list[dict[str, object]]
) -> _LLMRequest:
    """The request of an iteration: native tools, or the text tool protocol's text and grammar (S9-C)."""
    if protocol is None:
        return _LLMRequest(messages=messages, tools=tools_array or None)
    return _LLMRequest(
        messages=protocol.wire_messages(messages),
        tools=None,
        response_format=protocol.response_format(),
        max_tokens=TURN_MAX_TOKENS,
    )


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
    ) -> None:
        self._llm = llm
        self._tools = tool_registry
        self._runtime = runtime
        self._workspace = workspace_path
        self._tool_executor = ToolExecutor(tool_registry, runtime, workspace_path)

    _MCP_READONLY_KEYWORDS: frozenset[str] = frozenset({"search", "list", "find", "get", "fetch_url"})

    @staticmethod
    def _filter_tools_for_capability(
        tools_array: list[dict[str, object]],
        capability: CapabilityLevel,
        mode_tools: frozenset[str] | None = None,
        selected_tools: list[str] | None = None,
    ) -> list[dict[str, object]]:
        """Filter tools based on model capability level and ToolRouter selection.

        The mode's tools and ALWAYS_OFFERED_TOOLS are always offered on top.
        Go sends the mode's tools as canonical policy names (Read, Edit,
        Bash, ...), so they are compared canonically.
        """
        if selected_tools is not None:
            allowed: frozenset[str] = frozenset(selected_tools)
        else:
            allowed = TOOLS_BY_CAPABILITY.get(capability, frozenset())
            if not allowed:
                return tools_array
        mode_canonical = frozenset(canonical_tool(t) for t in mode_tools or ())

        def _is_allowed(tool: dict[str, object]) -> bool:
            name = tool.get("function", {}).get("name", "")
            if name in allowed or name in ALWAYS_OFFERED_TOOLS or canonical_tool(name) in mode_canonical:
                return True
            if selected_tools is None and name.startswith("mcp__"):
                tool_action = name.rsplit("__", 1)[-1]
                return any(kw in tool_action for kw in AgentLoopExecutor._MCP_READONLY_KEYWORDS)
            return False

        return [t for t in tools_array if _is_allowed(t)]

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
        """Return the next untried fallback model, or None if exhausted.

        With a user's own key (``cfg.provider_api_key``) only models of the
        provider of the current model (the key's provider) qualify: the key
        is sent with every call and must never reach another provider.
        """
        same_provider = fallbacks_for_key(cfg.model, cfg.fallback_models) if cfg.provider_api_key else None
        for m in cfg.fallback_models:
            if m in state.failed_models:
                continue
            if same_provider is not None and m not in same_provider:
                logger.warning(
                    "skipping fallback model %s of another provider: the run uses the user's own key for %r",
                    m,
                    model_provider(cfg.model),
                )
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

    @_tracer.trace_agent("agent_loop")
    async def run(self, messages: list[dict[str, object]], config: LoopConfig | None = None) -> AgentLoopResult:  # noqa: C901
        """Execute the agentic loop until the LLM stops or limits are hit."""
        cfg = config or LoopConfig()
        quality_tracker = IterationQualityTracker()
        state = _LoopState(
            model=cfg.model,
            quality_tracker=quality_tracker,
            tool_output_max_chars=cfg.tool_output_max_chars or DEFAULT_TOOL_OUTPUT_MAX_CHARS,
        )
        stall_detector = StallDetector()
        error_tracker = ToolErrorTracker()

        # No experience cache here: an agentic turn's result is the work it
        # does in the workspace, which a cached answer cannot replace (KI-16).
        plan_act = init_plan_act(cfg, messages)
        tools_array = self._tools.get_openai_tools()
        cap_level = CapabilityLevel(cfg.capability_level) if cfg.capability_level else CapabilityLevel.FULL
        tools_array = self._filter_tools_for_capability(
            tools_array, cap_level, cfg.mode_tools or None, cfg.selected_tools
        )

        # A model without function calling gets no tools parameter: it calls
        # the offered tools through the text tool protocol (S9-C).
        if cap_level == CapabilityLevel.PURE_COMPLETION and tools_array:
            state.tool_protocol = TextToolProtocol(
                tools_array, plan_act=plan_act.enabled, grammar=cfg.text_tool_grammar
            )

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

            stored_before = len(state.tool_messages)
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

            # Only an iteration that called tools feeds the stall detector: a
            # retry (a protocol repair, a dropped grammar, a fallback model)
            # would count the previous call again.
            if len(state.tool_messages) > stored_before:
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
            state.error = stall_error(abort_info["repeated_action"], abort_info["escape_count"])
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
        """Run one LLM iteration. Returns typed IterationOutcome.

        With the text tool protocol (state.tool_protocol) a reply without
        native tool calls is parsed: a call continues as a native one, an
        unusable reply is sent back to the model once (S9-C).
        """
        llm_decision = await self._runtime.request_tool_call(tool="LLM", command="chat_completion")
        if llm_decision.decision != "allow":
            logger.warning("LLM call denied by policy: %s", llm_decision.reason)
            return IterationError(f"LLM call denied: {llm_decision.reason}")

        reply = await self._call_llm(cfg, tools_array, messages, state, iteration)
        if not isinstance(reply, _LLMReply):
            return reply
        response, full_text = reply.response, reply.text

        protocol = state.tool_protocol
        ignored_calls = 0
        if protocol is not None and not response.tool_calls:
            turn = protocol.parse(response.content, truncated=response.finish_reason == "length")
            if isinstance(turn, TextProtocolError):
                return await self._handle_protocol_error(cfg, state, response, llm_decision, full_text, messages, turn)
            response = native_response(response, turn)
            ignored_calls = turn.ignored_calls if isinstance(turn, TextToolCall) else 0
        state.protocol_repairs = 0

        outcome = await self._process_llm_response(
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
        if ignored_calls and isinstance(outcome, IterationContinue):
            messages.append({"role": "user", "content": EXTRA_CALLS_NOTE})
        return outcome

    async def _call_llm(
        self,
        cfg: LoopConfig,
        tools_array: list[dict[str, object]],
        messages: list[dict[str, object]],
        state: _LoopState,
        iteration: int,
    ) -> _LLMReply | IterationOutcome:
        """Stream one completion; the reply and its visible text, or the outcome of a failed call.

        With the text tool protocol the request carries the protocol's wire
        messages, grammar and max_tokens instead of tools, and the stream
        shows only the reply's prose, thought and final text.
        """
        request = _llm_request(state.tool_protocol, tools_array, messages)
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

        stream_filter = ProtocolStreamFilter(_on_chunk) if state.tool_protocol is not None else None

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
                    messages=request.messages,
                    model=model_name,
                    tools=request.tools,
                    temperature=cfg.temperature,
                    tags=cfg.tags or None,
                    on_chunk=stream_filter.feed if stream_filter is not None else _on_chunk,
                    provider_api_key=cfg.provider_api_key,
                    top_p=cfg.top_p,
                    extra_body=cfg.extra_body,
                    response_format=request.response_format,
                    max_tokens=request.max_tokens,
                )
            except asyncio.CancelledError:
                raise
            except (LLMError, httpx.HTTPError, OSError, RuntimeError, ValueError) as exc:
                llm_span.set_status(StatusCode.ERROR, str(exc))
                llm_span.record_exception(exc)
                return await self._llm_call_failed(cfg, state, exc, iteration, model_name)
            llm_span.set_attribute("gen_ai.usage.input_tokens", response.tokens_in)
            llm_span.set_attribute("gen_ai.usage.output_tokens", response.tokens_out)
            if response.model:
                llm_span.set_attribute("gen_ai.response.model", response.model)

        if stream_filter is not None:
            stream_filter.finish()
        if pending_sends:
            await asyncio.gather(*pending_sends, return_exceptions=True)
        otel_metrics.llm_call_duration.record(time.monotonic() - llm_start)
        otel_metrics.llm_tokens.add(response.tokens_in + response.tokens_out)
        full_text = "".join(streamed_text)
        if full_text and not pending_sends:
            await self._runtime.send_output(full_text)
        return _LLMReply(response=response, text=full_text)

    async def _llm_call_failed(
        self, cfg: LoopConfig, state: _LoopState, exc: Exception, iteration: int, model_name: str
    ) -> IterationOutcome:
        """The outcome of a failed completion: retry without a rejected grammar, a fallback model, or an error."""
        if not isinstance(exc, LLMError):
            logger.exception("LLM call failed on iteration %d (unexpected)", iteration)
            exc = LLMError(status_code=500, model=model_name, body=str(exc))
        elif self._drop_rejected_grammar(state.tool_protocol, exc):
            return IterationContinue()
        err = await self._handle_llm_error(cfg, state, exc, iteration)
        return IterationError(err) if err else IterationContinue()

    @staticmethod
    def _drop_rejected_grammar(protocol: TextToolProtocol | None, exc: LLMError) -> bool:
        """Turn the text protocol's grammar off for the run when the server rejected it.

        Returns True when the iteration should be retried without it: there
        is no cache across runs, a later run tries the grammar again.
        """
        if protocol is None or not protocol.grammar or not grammar_rejected(exc.status_code, exc.body):
            return False
        protocol.grammar = False
        logger.warning(
            "the server rejected the text tool grammar (status %d), continuing without it: %s",
            exc.status_code,
            exc.body[:200],
        )
        return True

    async def _handle_protocol_error(
        self,
        cfg: LoopConfig,
        state: _LoopState,
        response: ChatCompletionResponse,
        llm_decision: ToolCallDecision,
        full_text: str,
        messages: list[dict[str, object]],
        error: TextProtocolError,
    ) -> IterationOutcome:
        """An unusable text protocol reply: costed, then asked once more; a second in a row ends the run.

        The repair message goes into the loop's messages only (not
        state.tool_messages), without the malformed reply.
        """
        await self._record_llm_turn(cfg, state, response, llm_decision, full_text)
        if state.protocol_repairs >= _MAX_PROTOCOL_REPAIRS:
            return IterationError(f"text tool protocol: {error.message}")
        state.protocol_repairs += 1
        logger.warning("unusable text tool protocol reply, asking the model again: %s", error.message)
        messages.append({"role": "user", "content": TextToolProtocol.repair_message(error)})
        await self._runtime.send_output(REPAIR_NOTICE)
        return IterationContinue()

    async def _record_llm_turn(
        self,
        cfg: LoopConfig,
        state: _LoopState,
        response: ChatCompletionResponse,
        llm_decision: ToolCallDecision,
        full_text: str,
    ) -> None:
        """Account an LLM reply: cost and tokens, its LLM tool result, trajectory and routing outcome."""
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
        await self._record_llm_turn(cfg, state, response, llm_decision, full_text)

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
            if cfg.implementation_turn and not state.nudged and announces_action(response.content):
                state.nudged = True
                messages.append({"role": "assistant", "content": response.content})
                messages.append({"role": "user", "content": CONTINUE_NUDGE})
                logger.warning(
                    "announced action without a tool call at iteration %d, nudging the agent once", iteration
                )
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

    No shell, to avoid injection risks; git runs as a tool process (it
    executes the workspace's hooks and config).
    """
    proc = await start_tool_process(
        "git",
        *args,
        cwd=workspace_path,
        stdout=asyncio.subprocess.PIPE,
        stderr=asyncio.subprocess.PIPE,
        env=tool_env(),
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
