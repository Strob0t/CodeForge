"""LoopConfig for an agent loop: shared by conversation runs and runs.start."""

from __future__ import annotations

from dataclasses import dataclass, replace
from typing import TYPE_CHECKING

import structlog

from codeforge.agent_loop import DEFAULT_MAX_ITERATIONS, LoopConfig
from codeforge.llm import LiteLLMClient, ModelMetadata
from codeforge.policy_args import is_builtin_tool
from codeforge.tools.capability import CapabilityLevel, classify_model
from codeforge.tools.tool_router import ToolRouter

if TYPE_CHECKING:
    from codeforge.llm import RoutingResult

logger = structlog.get_logger()

_LOCAL_MODEL_PREFIXES = ("lm_studio/", "ollama/")

# Context limits when LiteLLM reports no context window (conservative defaults).
_FALLBACK_CONTEXT_LIMITS: dict[CapabilityLevel, int] = {
    CapabilityLevel.FULL: 120_000,
    CapabilityLevel.API_WITH_TOOLS: 32_000,
    CapabilityLevel.PURE_COMPLETION: 16_000,
}


@dataclass(frozen=True)
class ModelCapability:
    """A model's tool capability level and the context token limit a run uses for it."""

    level: CapabilityLevel
    context_limit: int


async def resolve_model_capability(llm: object, model: str) -> ModelCapability:
    """Classify *model* and size its context from LiteLLM's model metadata (KI-125).

    The metadata comes from the LiteLLM proxy (``/model/info``); another LLM
    client (a test double) has none, and the model is classified by the
    operator's override and its name. The context limit is 85 % of the
    reported window (room for the output), at most the level's default.
    """
    metadata = await llm.model_metadata(model) if isinstance(llm, LiteLLMClient) else ModelMetadata()
    level = classify_model(model, supports_function_calling=metadata.supports_function_calling)
    limit = _FALLBACK_CONTEXT_LIMITS[level]
    if metadata.max_input_tokens:
        limit = min(int(metadata.max_input_tokens * 0.85), limit)
    logger.info(
        "model capability resolved",
        model=model,
        capability_level=level.value,
        supports_function_calling=metadata.supports_function_calling,
        context_window=metadata.max_input_tokens,
        context_limit=limit,
    )
    return ModelCapability(level=level, context_limit=limit)


def build_loop_config(
    *,
    primary_model: str,
    capability_level: CapabilityLevel,
    routing: RoutingResult,
    tool_names: list[str],
    fallback_models: list[str],
    user_prompt: str,
    max_steps: int,
    max_cost: float,
    mode_tools: frozenset[str],
    provider_api_key: str = "",
    plan_act_enabled: bool = False,
    tool_output_max_chars: int = 0,
) -> tuple[LoopConfig, str | None]:
    """Build the LoopConfig of a run with complexity-aware adjustments.

    Carries the fallback chain and the routing decision (the loop reports the
    outcome to the MAB router), the primary model's *capability_level*
    (``resolve_model_capability``), selects the tools for the prompt and applies
    local-model sampling parameters. Returns ``(config, complexity_hint)``;
    the hint is a system message for weak local models on complex tasks, or
    None.
    """
    selected_tools = ToolRouter(all_tool_names=tool_names).select(user_prompt) if user_prompt else None
    if selected_tools is not None:
        logger.info("tool router selected", count=len(selected_tools), tools=selected_tools)

    is_local = primary_model.startswith(_LOCAL_MODEL_PREFIXES)
    loop_cfg = LoopConfig(
        max_iterations=max_steps or DEFAULT_MAX_ITERATIONS,
        max_cost=max_cost or 0.0,
        model=primary_model,
        temperature=0.7 if is_local else routing.temperature,
        tags=routing.tags,
        fallback_models=fallback_models,
        routing_layer=routing.routing_layer,
        complexity_tier=routing.complexity_tier,
        task_type=routing.task_type,
        provider_api_key=provider_api_key,
        plan_act_enabled=plan_act_enabled,
        # The plan phase stays read-only: the mode's built-in tools (Write,
        # Edit, Bash, ...) are not plan tools, only its other tools are.
        extra_plan_tools=frozenset(t for t in mode_tools if not is_builtin_tool(t)),
        routing_metadata=routing.routing_metadata,
        capability_level=str(capability_level),
        mode_tools=mode_tools,
        top_p=0.8 if is_local else None,
        extra_body={"top_k": 20, "repetition_penalty": 1.05} if is_local else None,
        selected_tools=selected_tools,
        tool_output_max_chars=tool_output_max_chars,
    )

    complexity = routing.complexity_tier or "unknown"
    is_weak_model = is_local and capability_level in (CapabilityLevel.PURE_COMPLETION, CapabilityLevel.API_WITH_TOOLS)

    complexity_hint: str | None = None
    if is_weak_model and complexity in ("complex", "reasoning"):
        complexity_hint = (
            "This is a complex task being handled by a local model. "
            "Break it into smaller, sequential subtasks. "
            "Complete each subtask fully (write + test) before moving to the next one."
        )
        logger.info("injected complexity decomposition hint", complexity=complexity, model=primary_model)

    if is_local and complexity == "simple":
        loop_cfg = replace(loop_cfg, max_iterations=min(loop_cfg.max_iterations, 20))
        logger.info(
            "capped iterations for simple local task", max_iterations=loop_cfg.max_iterations, model=primary_model
        )

    return loop_cfg, complexity_hint
