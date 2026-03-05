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
    RoutingPlan,
)

if TYPE_CHECKING:
    from codeforge.routing.complexity import ComplexityAnalyzer
    from codeforge.routing.mab import MABModelSelector
    from codeforge.routing.meta_router import LLMMetaRouter
    from codeforge.routing.rate_tracker import RateLimitTracker

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
        "gemini/gemini-2.0-flash",
    ],
    ComplexityTier.COMPLEX: [
        "openai/gpt-4o",
        "anthropic/claude-sonnet-4",
        "gemini/gemini-2.5-pro",
    ],
    ComplexityTier.REASONING: [
        "anthropic/claude-opus-4.6",
        "openai/gpt-4o",
        "gemini/gemini-2.5-pro",
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
        rate_tracker: RateLimitTracker | None = None,
    ) -> None:
        self._complexity = complexity
        self._mab = mab
        self._meta = meta
        self._available_models = available_models
        self._config = config
        self._rate_tracker = rate_tracker

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

    def route_with_fallbacks(
        self,
        prompt: str,
        max_cost: float | None = None,
        max_fallbacks: int = 3,
    ) -> RoutingPlan:
        """Route a prompt and return a primary model plus ranked fallbacks."""
        primary = self.route(prompt, max_cost=max_cost)

        if primary is None:
            return RoutingPlan(
                primary=RoutingDecision(
                    model="",
                    routing_layer="none",
                    complexity_tier=ComplexityTier.MEDIUM,
                    task_type="chat",
                ),
                fallbacks=tuple(self._available_models[:max_fallbacks]),
            )

        seen: set[str] = {primary.model}
        fallbacks: list[str] = []

        if self._config.mab_enabled and self._mab is not None:
            diverse = self._mab.select_diverse(
                primary.task_type,
                primary.complexity_tier,
                self._available_models,
                n=max_fallbacks + 1,
                max_cost=max_cost,
            )
            for m in diverse:
                if m not in seen:
                    fallbacks.append(m)
                    seen.add(m)

        tier_defaults = COMPLEXITY_DEFAULTS.get(primary.complexity_tier, [])
        for m in tier_defaults:
            if m not in seen and m in self._available_models:
                if self._rate_tracker is not None:
                    provider = m.split("/")[0] if "/" in m else ""
                    if provider and self._rate_tracker.is_exhausted(provider):
                        continue
                fallbacks.append(m)
                seen.add(m)

        for m in self._available_models:
            if m not in seen:
                fallbacks.append(m)
                seen.add(m)

        truncated = tuple(fallbacks[:max_fallbacks])
        logger.debug("route_with_fallbacks: primary=%s fallbacks=%s", primary.model, truncated)
        return RoutingPlan(primary=primary, fallbacks=truncated)

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
            if self._rate_tracker is not None:
                provider = model.split("/")[0] if "/" in model else ""
                if provider and self._rate_tracker.is_exhausted(provider):
                    logger.debug("skipping rate-limited provider %s for model %s", provider, model)
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
