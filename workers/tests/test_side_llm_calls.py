"""No side LLM calls to models the user did not choose (KI-192, R9-6).

The HybridRouter ran on every turn, also when the run had an explicit model:
its MAB cold start called the meta-router LLM with a preview of the prompt,
and the decision was discarded. Skill selection sent the user's message to
the first available model. Neither call was costed or used the run's key.
Now a run with an explicit model builds no router, and skills are selected
locally (BM25), without an LLM call.
"""

from __future__ import annotations

from unittest.mock import AsyncMock, MagicMock, patch

import pytest

from codeforge.consumer._conversation_prompt_builder import inject_skills
from codeforge.consumer._conversation_routing import resolve_model_and_fallbacks
from codeforge.llm import RoutingResult
from codeforge.models import ConversationMessagePayload
from codeforge.skills.models import Skill
from codeforge.skills.selector import rank_skills

AVAILABLE = ["ollama/qwen3:4b", "openai/gpt-4o", "anthropic/claude-sonnet-4-6", "groq/llama"]


async def test_an_explicit_model_builds_no_router() -> None:
    router = AsyncMock(side_effect=AssertionError("the router must not be built"))
    with (
        patch("codeforge.consumer._conversation_routing.get_hybrid_router", router),
        patch("codeforge.consumer._conversation_routing.get_available_models", AsyncMock(return_value=AVAILABLE)),
    ):
        model, routing, fallbacks = await resolve_model_and_fallbacks(
            "http://litellm",
            "key",
            prompt="refactor the parser",
            scenario="",
            explicit_model="ollama/qwen3:4b",
            max_cost=0.0,
            log=MagicMock(),
        )

    assert model == "ollama/qwen3:4b"
    assert routing.routing_layer == ""
    assert "ollama/qwen3:4b" not in fallbacks
    router.assert_not_awaited()


async def test_without_a_model_the_router_decides() -> None:
    with (
        patch("codeforge.consumer._conversation_routing.get_hybrid_router", AsyncMock(return_value=None)) as router,
        patch("codeforge.llm.resolve_model_with_routing", return_value=RoutingResult(model="openai/gpt-4o")) as routed,
        patch("codeforge.consumer._conversation_routing.get_available_models", AsyncMock(return_value=AVAILABLE)),
    ):
        model, _, _ = await resolve_model_and_fallbacks(
            "http://litellm",
            "key",
            prompt="refactor the parser",
            scenario="",
            explicit_model="",
            max_cost=0.0,
            log=MagicMock(),
        )

    assert model == "openai/gpt-4o"
    router.assert_awaited_once()
    routed.assert_called_once()


SKILLS = [
    Skill(id="s1", name="pytest-fixtures", description="Write pytest fixtures for tests", tags=["pytest"]),
    Skill(id="s2", name="docker-compose", description="Compose services with docker", tags=["docker"]),
]


@pytest.mark.parametrize(
    ("task", "want"),
    [("write pytest fixtures for the parser tests", ["s1"]), ("", []), ("zzz unrelated", [])],
)
def test_rank_skills_is_local(task: str, want: list[str]) -> None:
    assert [s.id for s in rank_skills(SKILLS, task)] == want
    assert rank_skills([], task) == []


async def test_skill_injection_makes_no_llm_call() -> None:
    cursor = AsyncMock()
    cursor.fetchall = AsyncMock(
        return_value=[
            (
                "s1",
                "pytest-fixtures",
                "pattern",
                "Write pytest fixtures",
                "python",
                "Use fixtures.",
                "",
                [],
                "user",
                "active",
            )
        ]
    )
    conn = MagicMock()
    conn.__aenter__ = AsyncMock(return_value=conn)
    conn.__aexit__ = AsyncMock(return_value=None)
    conn.cursor = MagicMock(return_value=MagicMock(__aenter__=AsyncMock(return_value=cursor), __aexit__=AsyncMock()))
    messages = [ConversationMessagePayload(role="user", content="write pytest fixtures for the parser")]

    with (
        patch("psycopg.AsyncConnection.connect", AsyncMock(return_value=conn)),
        patch("codeforge.skills.registry.load_builtin_skills", return_value=[]),
        patch("codeforge.skills.selector.get_available_models", side_effect=AssertionError("no model lookup")),
    ):
        prompt, loaded = await inject_skills("base", "p1", messages, "t1", MagicMock(), "postgresql://fake")

    assert [s.id for s in loaded] == ["s1"]
    assert '<skill name="pytest-fixtures"' in prompt


MIXED = ["ollama/qwen3:4b", "openai/gpt-4o", "groq/llama", "anthropic/claude-haiku-4-5", "anthropic/claude-opus-4"]


@pytest.mark.parametrize(
    ("same_provider_only", "want"),
    [
        (True, ["anthropic/claude-haiku-4-5", "anthropic/claude-opus-4"]),
        (False, ["ollama/qwen3:4b", "openai/gpt-4o", "groq/llama"]),
    ],
)
async def test_explicit_model_fallbacks_are_filtered_by_the_key_before_the_cut(
    same_provider_only: bool, want: list[str]
) -> None:
    """A run with the user's own key falls back to the key's provider: filtered first, then cut to three."""
    with patch("codeforge.consumer._conversation_routing.get_available_models", AsyncMock(return_value=MIXED)):
        _, _, fallbacks = await resolve_model_and_fallbacks(
            "http://litellm",
            "key",
            prompt="refactor the parser",
            scenario="",
            explicit_model="anthropic/claude-sonnet-4-6",
            max_cost=0.0,
            log=MagicMock(),
            same_provider_only=same_provider_only,
        )

    assert fallbacks == want


@pytest.mark.parametrize(("key", "want"), [("sk-user", True), ("", False)])
async def test_the_conversation_run_filters_fallbacks_by_its_own_key(key: str, want: bool) -> None:
    from codeforge.consumer import TaskConsumer
    from codeforge.models import ConversationRunStartMessage

    run_msg = ConversationRunStartMessage(
        run_id="r",
        conversation_id="c",
        project_id="p",
        messages=[],
        system_prompt="s",
        model="anthropic/claude-sonnet-4-6",
        provider_api_key=key,
    )
    resolve = AsyncMock(return_value=("anthropic/claude-sonnet-4-6", RoutingResult(), []))
    with patch("codeforge.consumer._conversation.resolve_model_and_fallbacks", resolve):
        await TaskConsumer(nats_url="nats://test:4222")._resolve_routing_and_fallbacks(run_msg, "x", MagicMock())

    assert resolve.await_args.kwargs["same_provider_only"] is want
