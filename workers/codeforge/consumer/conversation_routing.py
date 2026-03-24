"""Routing helpers for conversation handler: HybridRouter setup, model discovery, fallback chain."""

from __future__ import annotations

import asyncio
from typing import TYPE_CHECKING

import structlog

from codeforge.config import get_settings

if TYPE_CHECKING:
    from codeforge.routing.router import HybridRouter

logger = structlog.get_logger()


async def get_hybrid_router(  # noqa: C901
    litellm_url: str,
    litellm_key: str,
) -> HybridRouter | None:
    """Build a HybridRouter if routing is enabled. Returns None otherwise."""
    from codeforge.llm import load_routing_config

    config = load_routing_config()
    if config is None:
        return None

    from codeforge.routing import ComplexityAnalyzer, HybridRouter, RoutingConfig

    if not isinstance(config, RoutingConfig):
        return None

    complexity = ComplexityAnalyzer()

    # MAB needs a stats loader -- use HTTP API if available, else skip.
    mab = None
    if config.mab_enabled:
        from codeforge.routing.mab import MABModelSelector

        _settings = get_settings()
        core_url = _settings.core_url
        internal_key = _settings.internal_key

        def _load_stats(task_type: str, tier: str) -> list:
            """Synchronous stats loader via Go Core HTTP API."""
            import httpx

            from codeforge.routing.models import ModelStats

            headers: dict[str, str] = {}
            if internal_key:
                headers["X-API-Key"] = internal_key
            try:
                resp = httpx.get(
                    f"{core_url}/api/v1/routing/stats",
                    params={"task_type": task_type, "tier": tier},
                    headers=headers,
                    timeout=5.0,
                )
                if resp.status_code != 200:
                    return []
                data = resp.json()
                if not isinstance(data, list):
                    return []
                return [
                    ModelStats(
                        model_name=s.get("model_name", ""),
                        trial_count=s.get("trial_count", 0),
                        avg_reward=s.get("avg_reward", 0.0),
                        avg_cost_usd=s.get("avg_cost_usd", 0.0),
                        avg_latency_ms=s.get("avg_latency_ms", 0),
                        avg_quality=s.get("avg_quality", 0.0),
                        input_cost_per=s.get("input_cost_per", 0.0),
                        supports_tools=s.get("supports_tools", False),
                        supports_vision=s.get("supports_vision", False),
                        max_context=s.get("max_context", 0),
                    )
                    for s in data
                ]
            except httpx.ConnectError:
                logger.warning("routing stats unavailable (Go Core not reachable)", core_url=core_url)
                return []
            except Exception as exc:
                logger.warning("failed to load routing stats", exc_info=True, error=str(exc))
                return []

        mab = MABModelSelector(stats_loader=_load_stats, config=config)

    # Meta-router needs an LLM call function.
    meta = None
    if config.llm_meta_enabled:
        from codeforge.routing.meta_router import LLMMetaRouter

        def _llm_call(model: str, prompt: str) -> str | None:
            """Synchronous LLM call for meta-router classification."""
            import httpx

            headers: dict[str, str] = {"Content-Type": "application/json"}
            if litellm_key:
                headers["Authorization"] = f"Bearer {litellm_key}"
            try:
                resp = httpx.post(
                    f"{litellm_url}/v1/chat/completions",
                    json={
                        "model": model,
                        "messages": [{"role": "user", "content": prompt}],
                        "temperature": 0.1,
                        "max_tokens": 200,
                    },
                    headers=headers,
                    timeout=30.0,
                )
                if resp.status_code != 200:
                    return None
                data = resp.json()
                choices = data.get("choices", [])
                if not choices:
                    return None
                return choices[0].get("message", {}).get("content", "")
            except Exception as exc:
                logger.warning("meta-router LLM call failed", exc_info=True, error=str(exc))
                return None

        meta = LLMMetaRouter(llm_call=_llm_call, config=config)

    # Get available models from LiteLLM.
    available_models = await get_available_models(litellm_url, litellm_key)

    from codeforge.routing.rate_tracker import get_tracker

    return HybridRouter(
        complexity=complexity,
        mab=mab,
        meta=meta,
        available_models=available_models,
        config=config,
        rate_tracker=get_tracker(),
    )


async def get_available_models(litellm_url: str, litellm_key: str) -> list[str]:
    """Fetch available model names, preferring Go Core's health-checked list."""
    import httpx

    from codeforge.routing.blocklist import get_blocklist

    # --- Primary: Go Core (health-checked, authoritative) ---
    _settings = get_settings()
    core_url = _settings.core_url
    internal_key = _settings.internal_key
    core_headers: dict[str, str] = {}
    if internal_key:
        core_headers["X-API-Key"] = internal_key
    try:
        async with httpx.AsyncClient(timeout=5.0) as client:
            resp = await client.get(f"{core_url}/api/v1/llm/available", headers=core_headers)
        if resp.status_code == 200:
            data = resp.json()
            raw_models = [
                m.get("model_name", "")
                for m in data.get("models", [])
                if m.get("model_name") and m.get("status") != "unreachable"
            ]
            if raw_models:
                from codeforge.model_resolver import expand_wildcard_models
                from codeforge.routing.key_filter import filter_keyless_models

                models = expand_wildcard_models(raw_models)
                models = filter_keyless_models(models)
                models = get_blocklist().filter_available(models)
                await append_claude_code_model(models)
                return models
            logger.warning("Go Core /llm/available returned no reachable models")
    except Exception as exc:
        logger.debug("Go Core /llm/available unavailable, falling back to LiteLLM", error=str(exc))

    # --- Fallback: direct LiteLLM query (no health filtering) ---
    litellm_headers: dict[str, str] = {}
    if litellm_key:
        litellm_headers["Authorization"] = f"Bearer {litellm_key}"
    try:
        async with httpx.AsyncClient(timeout=5.0) as client:
            resp = await client.get(f"{litellm_url}/v1/models", headers=litellm_headers)
        if resp.status_code != 200:
            logger.warning("LiteLLM /v1/models returned status", status_code=resp.status_code)
            return []
        data = resp.json()
        raw_ids = [m.get("id", "") for m in data.get("data", []) if m.get("id")]
        from codeforge.model_resolver import expand_wildcard_models
        from codeforge.routing.key_filter import filter_keyless_models

        models = expand_wildcard_models(raw_ids)
        models = filter_keyless_models(models)
        if not models:
            logger.warning("LiteLLM /v1/models returned empty model list")
        models = get_blocklist().filter_available(models)
        await append_claude_code_model(models)
        return models
    except Exception as exc:
        logger.warning("failed to fetch models from LiteLLM", exc_info=True, error=str(exc))
        return []


async def append_claude_code_model(models: list[str]) -> None:
    """Append ``claudecode/default`` to the model list if CLI is available."""
    from codeforge.claude_code_availability import is_claude_code_available

    if await is_claude_code_available():
        models.append("claudecode/default")


async def build_fallback_chain(
    router: HybridRouter | None,
    user_prompt: str,
    primary_model: str,
    max_cost: float,
    routing_result: object | None,
    get_models_fn: object,
) -> list[str]:
    """Build a ranked list of fallback models from the router or available models."""
    fallbacks: list[str] = []
    if router is not None:
        from codeforge.routing.models import ComplexityTier, RoutingDecision, TaskType
        from codeforge.routing.router import HybridRouter as HybridRouterCls

        if isinstance(router, HybridRouterCls):
            existing: RoutingDecision | None = None
            if routing_result is not None and getattr(routing_result, "routing_layer", ""):
                try:
                    existing = RoutingDecision(
                        model=primary_model,
                        routing_layer=routing_result.routing_layer,
                        complexity_tier=ComplexityTier(routing_result.complexity_tier),
                        task_type=TaskType(routing_result.task_type),
                    )
                except ValueError:
                    existing = None
            plan = await asyncio.to_thread(
                router.route_with_fallbacks,
                prompt=user_prompt,
                max_cost=max_cost if max_cost > 0 else None,
                primary=existing,
            )
            fallbacks = [m for m in plan.fallbacks if m != primary_model]
    if not fallbacks:
        available = await get_models_fn()
        fallbacks = [m for m in available if m != primary_model][:3]
    return fallbacks
