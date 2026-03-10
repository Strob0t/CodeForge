"""Tests for Layer 3: LLMMetaRouter (Phase 26H)."""

from __future__ import annotations

import json

from codeforge.routing.meta_router import LLMMetaRouter
from codeforge.routing.models import (
    ComplexityTier,
    PromptAnalysis,
    RoutingConfig,
    TaskType,
)


def _analysis(
    tier: ComplexityTier = ComplexityTier.MEDIUM,
    task: TaskType = TaskType.CODE,
) -> PromptAnalysis:
    return PromptAnalysis(
        complexity_tier=tier,
        task_type=task,
        dimensions={},
        confidence=0.7,
    )


def _config() -> RoutingConfig:
    return RoutingConfig(meta_router_model="groq/llama-3.1-8b-instant")


AVAILABLE = ["openai/gpt-4o", "openai/gpt-4o-mini", "groq/llama-3.1-8b-instant"]


# -- Successful classification -----------------------------------------------


def test_valid_json_returns_decision() -> None:
    response = json.dumps(
        {
            "recommended_model": "openai/gpt-4o",
            "needs_tools": True,
            "needs_long_context": False,
            "reasoning": "Complex task needs premium model",
        }
    )

    def llm_call(model: str, prompt: str) -> str:
        return response

    router = LLMMetaRouter(llm_call=llm_call, config=_config())
    decision = router.classify("test prompt", _analysis(), AVAILABLE)

    assert decision is not None
    assert decision.model == "openai/gpt-4o"
    assert decision.routing_layer == "meta"
    assert decision.reasoning == "Complex task needs premium model"


def test_json_embedded_in_text() -> None:
    response = (
        'Here is my recommendation:\n{"recommended_model": "openai/gpt-4o-mini", "reasoning": "Simple task"}\nDone.'
    )

    router = LLMMetaRouter(llm_call=lambda m, p: response, config=_config())
    decision = router.classify("test", _analysis(), AVAILABLE)

    assert decision is not None
    assert decision.model == "openai/gpt-4o-mini"


# -- Malformed responses -----------------------------------------------------


def test_malformed_json_falls_back_to_tier() -> None:
    router = LLMMetaRouter(llm_call=lambda m, p: "not json at all", config=_config())
    decision = router.classify("test", _analysis(ComplexityTier.SIMPLE), AVAILABLE)

    assert decision is not None
    assert decision.routing_layer == "meta"
    # SIMPLE → economy → first available from preference list.
    assert decision.model in AVAILABLE


def test_model_not_in_available_falls_back() -> None:
    response = json.dumps({"recommended_model": "anthropic/claude-opus-4.6", "reasoning": ""})
    router = LLMMetaRouter(llm_call=lambda m, p: response, config=_config())
    decision = router.classify("test", _analysis(ComplexityTier.COMPLEX), AVAILABLE)

    assert decision is not None
    # Should fall back to tier mapping since opus isn't in AVAILABLE.
    assert decision.model in AVAILABLE


# -- LLM call failures -------------------------------------------------------


def test_llm_call_returns_none() -> None:
    router = LLMMetaRouter(llm_call=lambda m, p: None, config=_config())
    decision = router.classify("test", _analysis(), AVAILABLE)
    assert decision is None


def test_llm_call_raises_exception() -> None:
    def failing_call(model: str, prompt: str) -> str:
        raise RuntimeError("API error")

    router = LLMMetaRouter(llm_call=failing_call, config=_config())
    decision = router.classify("test", _analysis(), AVAILABLE)
    assert decision is None


# -- Prompt truncation -------------------------------------------------------


def test_prompt_truncated_at_500_chars() -> None:
    long_prompt = "x" * 1000
    captured_prompts: list[str] = []

    def capture_call(model: str, prompt: str) -> str:
        captured_prompts.append(prompt)
        return json.dumps({"recommended_model": "openai/gpt-4o", "reasoning": ""})

    router = LLMMetaRouter(llm_call=capture_call, config=_config())
    router.classify(long_prompt, _analysis(), AVAILABLE)

    assert len(captured_prompts) == 1
    # The prompt sent to the LLM should contain a truncated preview.
    assert "x" * 500 + "..." in captured_prompts[0]
    assert "x" * 501 not in captured_prompts[0]


# -- Edge cases --------------------------------------------------------------


def test_empty_available_models() -> None:
    router = LLMMetaRouter(llm_call=lambda m, p: None, config=_config())
    decision = router.classify("test", _analysis(), [])
    assert decision is None


def test_tier_to_model_economy() -> None:
    result = LLMMetaRouter._tier_to_model("economy", AVAILABLE)
    assert result == "groq/llama-3.1-8b-instant"


def test_tier_to_model_unknown_tier() -> None:
    result = LLMMetaRouter._tier_to_model("unknown", AVAILABLE)
    assert result is None


def test_tier_models_economy_prefers_copilot() -> None:
    from codeforge.routing.meta_router import _TIER_MODELS

    assert _TIER_MODELS["economy"][0] == "github_copilot/gpt-4o-mini"


def test_tier_models_reasoning_prefers_copilot() -> None:
    from codeforge.routing.meta_router import _TIER_MODELS

    assert _TIER_MODELS["reasoning"][0] == "github_copilot/o3-mini"
