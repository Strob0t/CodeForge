"""HybridRouter -- three-layer cascade orchestrator.

Routing cascade order (route() method):
1. ComplexityAnalyzer -- always runs first. Rule-based, <1ms. Produces
   a PromptAnalysis with complexity_tier and task_type.
2. MABModelSelector (UCB1) -- primary selection when mab_enabled=True and
   the selector has been instantiated. Uses bandit statistics to pick the
   best model for the analyzed tier/task.
3. LLMMetaRouter -- fallback for cold-start or when MAB has no candidate.
   Calls a cheap LLM to classify the prompt and pick a model.
4. Complexity defaults -- final fallback. Maps the complexity tier to a
   static list of preferred models (COMPLEXITY_DEFAULTS).

Routing is ENABLED by default (RoutingConfig.enabled=True). Disable with
CODEFORGE_ROUTING_ENABLED=false when all providers are unhealthy.
"""

from __future__ import annotations

import logging
import os
import time
from typing import TYPE_CHECKING

from codeforge.routing.models import (
    CascadePlan,
    CascadeStep,
    ComplexityTier,
    PromptAnalysis,
    RoutingConfig,
    RoutingDecision,
    RoutingMetadata,
    RoutingPlan,
)

if TYPE_CHECKING:
    from codeforge.routing.blocklist import ModelBlocklist
    from codeforge.routing.complexity import ComplexityAnalyzer
    from codeforge.routing.key_filter import KeyFilter
    from codeforge.routing.mab import MABModelSelector
    from codeforge.routing.meta_router import LLMMetaRouter
    from codeforge.routing.models import RoutingProfile
    from codeforge.routing.rate_tracker import RateLimitTracker

logger = logging.getLogger(__name__)

# TTL for caching the effective (non-blocked) models list.
_EFFECTIVE_MODELS_CACHE_TTL = float(os.environ.get("CODEFORGE_EFFECTIVE_MODELS_CACHE_TTL", "5.0"))

# Maximum retries for fallback routing when primary + fallback selection fails.
MAX_ROUTING_RETRIES: int = 3

# Hard deadline (seconds) for the entire route_with_fallbacks call.
ROUTING_TIMEOUT_SECONDS: float = 30.0

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
        "claudecode/default",
        "github_copilot/gpt-4o",
        "anthropic/claude-sonnet-4",
        "openai/gpt-4o",
        "gemini/gemini-2.5-pro",
    ],
    ComplexityTier.REASONING: [
        "claudecode/default",
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
        blocklist: ModelBlocklist | None = None,
        key_filter: KeyFilter | None = None,
    ) -> None:
        self._complexity = complexity
        self._mab = mab
        self._meta = meta
        self._available_models = available_models
        self._config = config
        self._rate_tracker = rate_tracker
        self._blocklist = blocklist
        self._key_filter = key_filter
        self._effective_cache: list[str] | None = None
        self._effective_cache_ts: float = 0.0

    @property
    def _effective_models(self) -> list[str]:
        """Return available models with blocked ones filtered out.

        Uses the injected blocklist if provided, otherwise falls back to the
        module-level default instance for backward compatibility.

        Caches the result for _EFFECTIVE_MODELS_CACHE_TTL seconds to avoid
        re-fetching the blocklist on every call.
        """
        now = time.monotonic()
        if self._effective_cache is not None and (now - self._effective_cache_ts) < _EFFECTIVE_MODELS_CACHE_TTL:
            return self._effective_cache

        if self._blocklist is not None:
            bl = self._blocklist
        else:
            from codeforge.routing.blocklist import get_blocklist

            bl = get_blocklist()

        self._effective_cache = bl.filter_available(self._available_models)
        self._effective_cache_ts = now
        return self._effective_cache

    def _is_provider_exhausted(self, model: str) -> bool:
        """Return True if the model's provider is currently rate-limited."""
        if self._rate_tracker is None:
            return False
        provider = model.split("/")[0] if "/" in model else ""
        return bool(provider and self._rate_tracker.is_exhausted(provider))

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
            if model is not None and not self._is_provider_exhausted(model):
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
            if decision is not None and not self._is_provider_exhausted(decision.model):
                return decision

        return self._complexity_fallback(analysis, max_cost)

    def route_with_metadata(
        self,
        prompt: str,
        max_cost: float | None = None,
        profile: RoutingProfile | None = None,
    ) -> tuple[RoutingDecision | None, RoutingMetadata | None]:
        """Route and return both decision and transparent metadata (C1)."""
        if not self._config.enabled:
            return None, None

        decision = self.route(prompt, max_cost=max_cost, profile=profile)
        if decision is None:
            return None, None

        # Build alternatives from tier defaults excluding selected model.
        available = self._effective_models
        tier_defaults = COMPLEXITY_DEFAULTS.get(decision.complexity_tier, [])
        alternatives: list[dict[str, object]] = []
        for model in tier_defaults:
            if model == decision.model or model not in available:
                continue
            score = 0.5  # default score for complexity-layer alternatives
            if self._mab is not None:
                mab_score = self._mab.score(model, decision.task_type, decision.complexity_tier)
                if mab_score is not None:
                    score = mab_score
            alternatives.append({"model": model, "score": score})

        metadata = RoutingMetadata(
            complexity_tier=decision.complexity_tier,
            selected_model=decision.model,
            reason=decision.reasoning,
            mab_score=decision.confidence,
            alternatives=tuple(alternatives),
        )
        return decision, metadata

    def route_with_fallbacks(
        self,
        prompt: str,
        max_cost: float | None = None,
        max_fallbacks: int = 3,
        primary: RoutingDecision | None = None,
        profile: RoutingProfile | None = None,
        timeout: float = ROUTING_TIMEOUT_SECONDS,
        max_retries: int = MAX_ROUTING_RETRIES,
    ) -> RoutingPlan:
        deadline = time.monotonic() + timeout
        retries = 0

        while primary is None and retries < max_retries:
            if time.monotonic() > deadline:
                logger.warning("route_with_fallbacks: timeout after %.1fs", timeout)
                break
            primary = self.route(prompt, max_cost=max_cost, profile=profile)
            retries += 1

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

        if time.monotonic() > deadline:
            logger.warning("route_with_fallbacks: timeout reached, returning primary only")
            return RoutingPlan(primary=primary, fallbacks=())

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
            if m not in seen and m in available and not self._is_provider_exhausted(m):
                fallbacks.append(m)
                seen.add(m)

        for m in available:
            if m not in seen and not self._is_provider_exhausted(m):
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
                if model in seen or model not in available or self._is_provider_exhausted(model):
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
            if model not in available or self._is_provider_exhausted(model):
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
