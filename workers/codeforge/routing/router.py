"""HybridRouter — three-layer cascade orchestrator.

Coordinates the ComplexityAnalyzer (Layer 1), MABModelSelector (Layer 2),
and LLMMetaRouter (Layer 3) into a single routing decision.

Cascade: L1 (always) → L2 (if enabled + data) → L3 (if enabled + L2 cold) → fallback.
"""

from __future__ import annotations

import logging
from typing import TYPE_CHECKING

from codeforge.routing.models import (
    ComplexityTier,
    PromptAnalysis,
    RoutingConfig,
    RoutingDecision,
)

if TYPE_CHECKING:
    from codeforge.routing.complexity import ComplexityAnalyzer
    from codeforge.routing.mab import MABModelSelector
    from codeforge.routing.meta_router import LLMMetaRouter

logger = logging.getLogger(__name__)

# Cold-start default models per complexity tier (ordered by preference).
COMPLEXITY_DEFAULTS: dict[ComplexityTier, list[str]] = {
    ComplexityTier.SIMPLE: [
        "groq/llama-3.1-8b-instant",
        "openai/gpt-4o-mini",
        "anthropic/claude-haiku-3.5",
    ],
    ComplexityTier.MEDIUM: [
        "groq/llama-3.3-70b",
        "openai/gpt-4o-mini",
        "google/gemini-2.0-flash",
    ],
    ComplexityTier.COMPLEX: [
        "openai/gpt-4o",
        "anthropic/claude-sonnet-4",
        "google/gemini-2.5-pro",
    ],
    ComplexityTier.REASONING: [
        "anthropic/claude-opus-4.6",
        "openai/gpt-4o",
        "google/gemini-2.5-pro",
    ],
}


class HybridRouter:
    """Three-layer routing cascade.

    Layer 1: ComplexityAnalyzer — rule-based, <1ms, always runs.
    Layer 2: MABModelSelector — UCB1 learning, runs if enabled + data exists.
    Layer 3: LLMMetaRouter — cold-start fallback, runs if L2 returned None.
    Fallback: Static tier-to-model mapping.
    """

    def __init__(
        self,
        complexity: ComplexityAnalyzer,
        mab: MABModelSelector | None,
        meta: LLMMetaRouter | None,
        available_models: list[str],
        config: RoutingConfig,
    ) -> None:
        self._complexity = complexity
        self._mab = mab
        self._meta = meta
        self._available_models = available_models
        self._config = config

    def route(
        self,
        prompt: str,
        max_cost: float | None = None,
    ) -> RoutingDecision | None:
        """Route a prompt through the three-layer cascade.

        Returns a RoutingDecision or None if routing is disabled.
        """
        if not self._config.enabled:
            return None

        # Layer 1: Complexity analysis (always runs).
        analysis = self._complexity.analyze(prompt)
        logger.debug(
            "L1 complexity: tier=%s task=%s confidence=%.2f",
            analysis.complexity_tier,
            analysis.task_type,
            analysis.confidence,
        )

        # Layer 2: MAB selection (if enabled).
        if self._config.mab_enabled and self._mab is not None:
            model = self._mab.select(
                analysis.task_type,
                analysis.complexity_tier,
                self._available_models,
                max_cost,
            )
            if model is not None:
                logger.debug("L2 MAB selected: %s", model)
                return RoutingDecision(
                    model=model,
                    routing_layer="mab",
                    complexity_tier=analysis.complexity_tier,
                    task_type=analysis.task_type,
                    confidence=analysis.confidence,
                    reasoning=f"MAB UCB1 selection for {analysis.task_type}/{analysis.complexity_tier}",
                )

        # Layer 3: LLM meta-router (if enabled).
        if self._config.llm_meta_enabled and self._meta is not None:
            decision = self._meta.classify(prompt, analysis, self._available_models)
            if decision is not None:
                logger.debug("L3 meta-router selected: %s", decision.model)
                return decision

        # Fallback: Static tier-to-model mapping.
        return self._complexity_fallback(analysis, max_cost)

    def _complexity_fallback(
        self,
        analysis: PromptAnalysis,
        max_cost: float | None,
    ) -> RoutingDecision | None:
        """Select a model from the static tier-to-model defaults.

        Respects ``max_cost`` by filtering out models whose
        ``input_cost_per_token`` exceeds the budget (requires capabilities lookup).
        """
        if not isinstance(analysis, PromptAnalysis):
            return None

        preferences = COMPLEXITY_DEFAULTS.get(analysis.complexity_tier, [])

        for model in preferences:
            if model not in self._available_models:
                continue
            if max_cost is not None:
                from codeforge.routing.capabilities import enrich_model_capabilities

                caps = enrich_model_capabilities(model)
                if caps["input_cost_per_token"] > max_cost:
                    continue
            return RoutingDecision(
                model=model,
                routing_layer="complexity",
                complexity_tier=analysis.complexity_tier,
                task_type=analysis.task_type,
                confidence=analysis.confidence,
                reasoning=f"Complexity fallback: {analysis.complexity_tier}",
            )

        # No preferred model available — use first available model.
        if self._available_models:
            return RoutingDecision(
                model=self._available_models[0],
                routing_layer="complexity",
                complexity_tier=analysis.complexity_tier,
                task_type=analysis.task_type,
                confidence=0.3,
                reasoning="No preferred model available, using first available",
            )

        return None
