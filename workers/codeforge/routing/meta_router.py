"""Layer 3: LLM-as-Router for cold-start fallback.

Uses a small, cheap model to classify prompts and recommend a model when
the MAB (Layer 2) has insufficient data. This is the most expensive routing
layer and only runs when Layers 1-2 cannot make a confident decision.
"""

from __future__ import annotations

import json
import logging
from typing import TYPE_CHECKING

from codeforge.routing.models import (
    ComplexityTier,
    PromptAnalysis,
    RoutingConfig,
    RoutingDecision,
)

if TYPE_CHECKING:
    from collections.abc import Callable

logger = logging.getLogger(__name__)

# Prompt truncation limit for the meta-router.
_MAX_PROMPT_CHARS = 500

# Tier-to-model preference lists (used as fallback when LLM returns a tier).
_TIER_MODELS: dict[str, list[str]] = {
    "economy": ["groq/llama-3.1-8b-instant", "openai/gpt-4o-mini", "anthropic/claude-haiku-3.5"],
    "standard": ["openai/gpt-4o-mini", "gemini/gemini-2.0-flash", "anthropic/claude-sonnet-4"],
    "premium": ["openai/gpt-4o", "anthropic/claude-sonnet-4", "gemini/gemini-2.5-pro"],
    "reasoning": ["anthropic/claude-opus-4.6", "openai/gpt-4o", "gemini/gemini-2.5-pro"],
}

_META_PROMPT = """\
Classify this task for model routing. Select the best model from the available list.

Prompt preview: {preview}

Pre-analysis: complexity={tier}, task_type={task_type}

Available models: {models}

Output ONLY valid JSON: {{"recommended_model": "...", "needs_tools": true/false, "needs_long_context": true/false, "reasoning": "..."}}"""


class LLMMetaRouter:
    """LLM-based model classifier for cold-start routing.

    Uses a small, cheap LLM to classify the prompt and recommend a model
    from the available list. Falls back to tier-based mapping if the LLM
    call fails or returns invalid output.
    """

    def __init__(
        self,
        llm_call: Callable[[str, str], str | None],
        config: RoutingConfig,
    ) -> None:
        """Initialize with an LLM call function and routing config.

        Args:
            llm_call: Function(model, prompt) -> response_text or None on failure.
            config: Routing configuration.
        """
        self._llm_call = llm_call
        self._config = config

    def classify(
        self,
        prompt: str,
        analysis: PromptAnalysis,
        available_models: list[str],
    ) -> RoutingDecision | None:
        """Classify a prompt and recommend a model.

        Returns None if the LLM call fails or no suitable model is found.
        """
        if not available_models:
            return None

        classification_prompt = self._build_prompt(prompt, analysis, available_models)

        router_model = self._config.meta_router_model
        if not router_model:
            from codeforge.model_resolver import resolve_model

            router_model = resolve_model()

        try:
            response = self._llm_call(router_model, classification_prompt)
        except Exception as exc:
            logger.warning("Meta-router LLM call failed: %s", exc, exc_info=True)
            return None

        if response is None:
            return None

        return self._parse_response(response, analysis, available_models)

    def _build_prompt(
        self,
        prompt: str,
        analysis: PromptAnalysis,
        available_models: list[str],
    ) -> str:
        preview = prompt[:_MAX_PROMPT_CHARS]
        if len(prompt) > _MAX_PROMPT_CHARS:
            preview += "..."

        return _META_PROMPT.format(
            preview=preview,
            tier=analysis.complexity_tier.value,
            task_type=analysis.task_type.value,
            models=", ".join(available_models),
        )

    def _parse_response(
        self,
        response: str,
        analysis: PromptAnalysis,
        available_models: list[str],
    ) -> RoutingDecision | None:
        """Parse the LLM response. Falls back to tier mapping on failure."""
        try:
            # Try to extract JSON from the response.
            start = response.find("{")
            end = response.rfind("}") + 1
            data = json.loads(response[start:end]) if start >= 0 and end > start else json.loads(response)

            recommended = data.get("recommended_model", "")
            reasoning = data.get("reasoning", "")

            # If the recommended model is in our available list, use it.
            if recommended in available_models:
                return RoutingDecision(
                    model=recommended,
                    routing_layer="meta",
                    complexity_tier=analysis.complexity_tier,
                    task_type=analysis.task_type,
                    confidence=0.7,
                    reasoning=reasoning,
                )

            # If it's a tier name, map to model.
            model = self._tier_to_model(recommended, available_models)
            if model:
                return RoutingDecision(
                    model=model,
                    routing_layer="meta",
                    complexity_tier=analysis.complexity_tier,
                    task_type=analysis.task_type,
                    confidence=0.6,
                    reasoning=reasoning,
                )

        except (json.JSONDecodeError, KeyError, TypeError):
            logger.debug("Meta-router response not valid JSON: %s", response[:200])

        # Final fallback: map complexity tier to tier category.
        tier_map = {
            ComplexityTier.SIMPLE: "economy",
            ComplexityTier.MEDIUM: "standard",
            ComplexityTier.COMPLEX: "premium",
            ComplexityTier.REASONING: "reasoning",
        }
        category = tier_map.get(analysis.complexity_tier, "standard")
        model = self._tier_to_model(category, available_models)
        if model:
            return RoutingDecision(
                model=model,
                routing_layer="meta",
                complexity_tier=analysis.complexity_tier,
                task_type=analysis.task_type,
                confidence=0.5,
                reasoning="Tier-based fallback from meta-router",
            )

        return None

    @staticmethod
    def _tier_to_model(tier: str, available_models: list[str]) -> str | None:
        """Map a tier name to the first available model from preference list."""
        preferences = _TIER_MODELS.get(tier.lower())
        if preferences is None:
            logger.debug("Unrecognised tier %r — no preference list available", tier)
            return None
        for model in preferences:
            if model in available_models:
                return model
        return None
