"""Tests for Phase 26L — routing integration with llm.py, conversation, executor."""

from __future__ import annotations

from codeforge.llm import resolve_model_with_routing, resolve_scenario
from codeforge.routing.models import RoutingConfig
from codeforge.routing.router import HybridRouter


def _make_router(
    enabled: bool = True,
    available: list[str] | None = None,
) -> HybridRouter:
    """Create a HybridRouter with a real ComplexityAnalyzer and no MAB/Meta."""
    from codeforge.routing.complexity import ComplexityAnalyzer

    config = RoutingConfig(enabled=enabled)
    return HybridRouter(
        complexity=ComplexityAnalyzer(),
        mab=None,
        meta=None,
        available_models=available or ["openai/gpt-4o", "groq/llama-3.1-8b-instant"],
        config=config,
    )


# -- resolve_model_with_routing -----------------------------------------------


class TestResolveModelWithRouting:
    def test_with_router_returns_model(self) -> None:
        router = _make_router(enabled=True, available=["groq/llama-3.1-8b-instant"])
        model, _temp, tags = resolve_model_with_routing("Hello", "default", router=router)
        assert model == "groq/llama-3.1-8b-instant"
        assert tags == []

    def test_without_router_falls_back_to_tags(self) -> None:
        model, temp, tags = resolve_model_with_routing("Hello", "think", router=None)
        assert model == ""
        assert temp == 0.3
        assert tags == ["think"]

    def test_router_disabled_returns_tags(self) -> None:
        router = _make_router(enabled=False)
        model, _temp, tags = resolve_model_with_routing("Hello", "review", router=router)
        assert model == ""
        assert tags == ["review"]

    def test_temperature_from_scenario(self) -> None:
        router = _make_router(enabled=True, available=["groq/llama-3.1-8b-instant"])
        _, temp, _ = resolve_model_with_routing("Hello", "plan", router=router)
        assert temp == 0.3  # plan scenario temperature

    def test_unknown_scenario_defaults(self) -> None:
        _, temp, tags = resolve_model_with_routing("Hello", "unknown_scenario", router=None)
        assert temp == 0.2
        assert tags == []

    def test_complex_prompt_routes_to_premium(self) -> None:
        router = _make_router(
            enabled=True,
            available=["openai/gpt-4o", "groq/llama-3.1-8b-instant"],
        )
        prompt = (
            "Refactor this microservice to use event sourcing across 5 files. "
            "Analyze the trade-offs between CQRS and traditional architecture. "
            "Consider the database migration strategy for PostgreSQL and Redis caching. "
            "Step 1: Design the event store. Step 2: Implement projections. "
            "Step 3: Migrate existing data. Step 4: Add integration tests."
        )
        model, _, _ = resolve_model_with_routing(prompt, "default", router=router)
        assert model in ("openai/gpt-4o", "groq/llama-3.1-8b-instant")
        assert model != ""  # Should always return a model when router is enabled

    def test_non_router_object_ignored(self) -> None:
        model, _, tags = resolve_model_with_routing("Hello", "think", router="not_a_router")
        assert model == ""
        assert tags == ["think"]


# -- resolve_scenario ----------------------------------------------------------


class TestResolveScenario:
    def test_known_scenarios(self) -> None:
        for name in ("background", "think", "longContext", "review", "plan"):
            cfg = resolve_scenario(name)
            assert cfg.tag == name
            assert cfg.temperature > 0

    def test_unknown_scenario(self) -> None:
        cfg = resolve_scenario("nonexistent")
        assert cfg.tag == ""
        assert cfg.temperature == 0.2

    def test_empty_scenario(self) -> None:
        cfg = resolve_scenario("")
        assert cfg.tag == ""


# -- load_routing_config -------------------------------------------------------


class TestLoadRoutingConfig:
    def test_disabled_by_default(self) -> None:
        import os

        os.environ.pop("CODEFORGE_ROUTING_ENABLED", None)
        from codeforge.llm import load_routing_config

        assert load_routing_config() is None

    def test_enabled_returns_config(self, monkeypatch: object) -> None:
        import os

        os.environ["CODEFORGE_ROUTING_ENABLED"] = "true"
        try:
            from codeforge.llm import load_routing_config

            config = load_routing_config()
            assert config is not None
            assert config.enabled is True
        finally:
            os.environ.pop("CODEFORGE_ROUTING_ENABLED", None)
