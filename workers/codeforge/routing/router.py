"""HybridRouter — three-layer cascade orchestrator."""

from __future__ import annotations

import logging
from typing import TYPE_CHECKING

from codeforge.routing.models import (
    CascadePlan,
    CascadeStep,
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
    from codeforge.routing.models import RoutingProfile
    from codeforge.routing.rate_tracker import RateLimitTracker

logger = logging.getLogger(__name__)

COMPLEXITY_DEFAULTS: dict[ComplexityTier, list[str]] = {
    ComplexityTier.SIMPLE: [
        "github_copilot/gpt-4o-mini",
        "gemini/gemini-2.0-flash",
        "groq/llama-3.1-8b-instant",
        "openai/gpt-4o-mini",
        "anthropic/claude-haiku-3.5",
    ],
    ComplexityTier.MEDIUM: [
        "github_copilot/gpt-4o",
        "gemini/gemini-2.5-flash",
        "anthropic/claude-haiku-3.5",
        "openai/gpt-4o-mini",
        "groq/llama-3.3-70b-versatile",
        "gemini/gemini-2.0-flash",
    ],
    ComplexityTier.COMPLEX: [
        "github_copilot/gpt-4o",
        "anthropic/claude-sonnet-4",
        "openai/gpt-4o",
        "gemini/gemini-2.5-pro",
    ],
    ComplexityTier.REASONING: [
        "github_copilot/o3-mini",
        "anthropic/claude-opus-4.6",
        "openai/gpt-4o",
        "gemini/gemini-2.5-pro",
    ],
}

_TIER_ORDER: list[ComplexityTier] = [
    ComplexityTier.SIMPLE,
    ComplexityTier.MEDIUM,
    ComplexityTier.COMPLEX,
    ComplexityTier.REASONING,
]


class HybridRouter:
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

    @property
    def _effective_models(self) -> list[str]:
        from codeforge.routing.blocklist import get_blocklist

        return get_blocklist().filter_available(self._available_models)

    def route(
        self,
        prompt: str,
        max_cost: float | None = None,
        profile: RoutingProfile | None = None,
    ) -> RoutingDecision | None:
        if not self._config.enabled:
            return None

        analysis = self._complexity.analyze(prompt)
        available = self._effective_models

        if self._config.mab_enabled and self._mab is not None:
            model = self._mab.select(
                analysis.task_type,
                analysis.complexity_tier,
                available,
                max_cost,
                profile=profile,
            )
            if model is not None:
                return RoutingDecision(
                    model=model,
                    routing_layer="mab",
                    complexity_tier=analysis.complexity_tier,
                    task_type=analysis.task_type,
                    confidence=analysis.confidence,
                    reasoning=f"MAB UCB1 selection for {analysis.task_type}/{analysis.complexity_tier}",
                )

        if self._config.llm_meta_enabled and self._meta is not None:
            decision = self._meta.classify(prompt, analysis, available)
            if decision is not None:
                return decision

        return self._complexity_fallback(analysis, max_cost)

    def route_with_fallbacks(
        self,
        prompt: str,
        max_cost: float | None = None,
        max_fallbacks: int = 3,
        primary: RoutingDecision | None = None,
        profile: RoutingProfile | None = None,
    ) -> RoutingPlan:
        if primary is None:
            primary = self.route(prompt, max_cost=max_cost, profile=profile)

        available = self._effective_models

        if primary is None:
            return RoutingPlan(
                primary=RoutingDecision(
                    model="",
                    routing_layer="none",
                    complexity_tier=ComplexityTier.MEDIUM,
                    task_type="chat",
                ),
                fallbacks=tuple(available[:max_fallbacks]),
            )

        seen: set[str] = {primary.model}
        fallbacks: list[str] = []

        if self._config.mab_enabled and self._mab is not None:
            diverse = self._mab.select_diverse(
                primary.task_type,
                primary.complexity_tier,
                available,
                n=max_fallbacks + 1,
                max_cost=max_cost,
                profile=profile,
            )
            for m in diverse:
                if m not in seen:
                    fallbacks.append(m)
                    seen.add(m)

        tier_defaults = COMPLEXITY_DEFAULTS.get(primary.complexity_tier, [])
        for m in tier_defaults:
            if m not in seen and m in available:
                if self._rate_tracker is not None:
                    provider = m.split("/")[0] if "/" in m else ""
                    if provider and self._rate_tracker.is_exhausted(provider):
                        continue
                fallbacks.append(m)
                seen.add(m)

        for m in available:
            if m not in seen:
                fallbacks.append(m)
                seen.add(m)

        return RoutingPlan(primary=primary, fallbacks=tuple(fallbacks[:max_fallbacks]))

    def route_cascade(
        self,
        prompt: str,
        max_cost: float | None = None,
        profile: RoutingProfile | None = None,
    ) -> CascadePlan:
        if not self._config.enabled:
            model = self._available_models[0] if self._available_models else ""
            return CascadePlan(
                steps=[
                    CascadeStep(
                        model=model,
                        confidence_threshold=self._config.cascade_confidence_threshold,
                    )
                ]
            )

        if not self._config.cascade_enabled:
            decision = self.route(prompt, max_cost=max_cost, profile=profile)
            model = decision.model if decision else (self._available_models[0] if self._available_models else "")
            return CascadePlan(
                steps=[
                    CascadeStep(
                        model=model,
                        confidence_threshold=self._config.cascade_confidence_threshold,
                    )
                ]
            )

        available = self._effective_models
        max_steps = self._config.cascade_max_steps
        threshold = self._config.cascade_confidence_threshold
        steps: list[CascadeStep] = []
        seen: set[str] = set()

        for tier in _TIER_ORDER:
            if len(steps) >= max_steps:
                break
            for model in COMPLEXITY_DEFAULTS.get(tier, []):
                if len(steps) >= max_steps:
                    break
                if model in seen or model not in available:
                    continue
                if self._rate_tracker is not None:
                    provider = model.split("/")[0] if "/" in model else ""
                    if provider and self._rate_tracker.is_exhausted(provider):
                        continue
                seen.add(model)
                steps.append(CascadeStep(model=model, confidence_threshold=threshold))

        if not steps and available:
            steps.append(CascadeStep(model=available[0], confidence_threshold=threshold))

        return CascadePlan(steps=steps)

    def _complexity_fallback(
        self,
        analysis: PromptAnalysis,
        max_cost: float | None,
    ) -> RoutingDecision | None:
        if not isinstance(analysis, PromptAnalysis):
            return None

        available = self._effective_models
        preferences = COMPLEXITY_DEFAULTS.get(analysis.complexity_tier, [])

        for model in preferences:
            if model not in available:
                continue
            if self._rate_tracker is not None:
                provider = model.split("/")[0] if "/" in model else ""
                if provider and self._rate_tracker.is_exhausted(provider):
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

        if available:
            return RoutingDecision(
                model=available[0],
                routing_layer="complexity",
                complexity_tier=analysis.complexity_tier,
                task_type=analysis.task_type,
                confidence=0.3,
                reasoning="No preferred model available, using first available",
            )

        return None
