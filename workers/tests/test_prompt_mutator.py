"""Tests for prompt_mutator -- GEPA-inspired prompt variant generation."""

from __future__ import annotations

import asyncio
import json

import pytest

from codeforge.evaluation.prompt_mutator import (
    PromptVariant,
    handle_mutate_request,
    mutate_prompt,
    mutate_prompt_sync,
    validate_variant,
)
from codeforge.evaluation.prompt_optimizer import TacticalFix

# -- Fixtures --


@pytest.fixture
def base_content() -> str:
    return "You are a coding assistant. Write clean, tested code."


@pytest.fixture
def fix_a() -> TacticalFix:
    return TacticalFix(
        task_id="task-001",
        failure_description="Missing error handling",
        root_cause="No try/except around file IO",
        proposed_addition="Always wrap file operations in try/except blocks.",
        confidence=0.9,
    )


@pytest.fixture
def fix_b() -> TacticalFix:
    return TacticalFix(
        task_id="task-002",
        failure_description="Type errors in output",
        root_cause="No type annotations",
        proposed_addition="Use explicit type annotations on all function signatures.",
        confidence=0.85,
    )


@pytest.fixture
def fix_duplicate(fix_a: TacticalFix) -> TacticalFix:
    """A fix with the same proposed_addition as fix_a but different task_id."""
    return TacticalFix(
        task_id="task-003",
        failure_description="Different failure",
        root_cause="Different cause",
        proposed_addition=fix_a.proposed_addition,
        confidence=0.7,
    )


@pytest.fixture
def principles() -> list[str]:
    return [
        "Prefer explicit error messages over generic exceptions.",
        "Always validate inputs before processing.",
    ]


# -- mutate_prompt_sync tests --


class TestMutatePromptSyncBasic:
    def test_applies_fixes_to_content(self, base_content: str, fix_a: TacticalFix, fix_b: TacticalFix) -> None:
        variant = mutate_prompt_sync(base_content, [fix_a, fix_b], [], "coder")
        assert fix_a.proposed_addition in variant.content
        assert fix_b.proposed_addition in variant.content
        assert variant.content.startswith(base_content.rstrip())
        assert variant.mutation_source == "tactical"
        assert "task-001" in variant.tactical_fixes_applied
        assert "task-002" in variant.tactical_fixes_applied

    def test_applies_principles(self, base_content: str, principles: list[str]) -> None:
        variant = mutate_prompt_sync(base_content, [], principles, "coder")
        for p in principles:
            assert p in variant.content
        assert variant.mutation_source == "strategic"
        assert variant.strategic_principles == principles

    def test_combined_source(self, base_content: str, fix_a: TacticalFix, principles: list[str]) -> None:
        variant = mutate_prompt_sync(base_content, [fix_a], principles, "coder")
        assert variant.mutation_source == "combined"
        assert fix_a.proposed_addition in variant.content
        for p in principles:
            assert p in variant.content


class TestMutatePromptSyncEmptyFixes:
    def test_returns_variant_with_original_content(self, base_content: str) -> None:
        variant = mutate_prompt_sync(base_content, [], [], "coder")
        assert variant.content == base_content
        assert variant.validation_passed is True
        assert variant.tactical_fixes_applied == []
        assert variant.strategic_principles == []

    def test_empty_proposed_additions_ignored(self, base_content: str) -> None:
        empty_fix = TacticalFix(
            task_id="task-empty",
            failure_description="desc",
            root_cause="cause",
            proposed_addition="",
            confidence=0.5,
        )
        variant = mutate_prompt_sync(base_content, [empty_fix], [], "coder")
        assert variant.content == base_content

    def test_whitespace_only_additions_ignored(self, base_content: str) -> None:
        ws_fix = TacticalFix(
            task_id="task-ws",
            failure_description="desc",
            root_cause="cause",
            proposed_addition="   \n  ",
            confidence=0.5,
        )
        variant = mutate_prompt_sync(base_content, [ws_fix], [], "coder")
        assert variant.content == base_content


class TestMutatePromptSyncDeduplicates:
    def test_same_fix_content_not_duplicated(
        self, base_content: str, fix_a: TacticalFix, fix_duplicate: TacticalFix
    ) -> None:
        variant = mutate_prompt_sync(base_content, [fix_a, fix_duplicate], [], "coder")
        # The proposed_addition should appear exactly once.
        count = variant.content.count(fix_a.proposed_addition)
        assert count == 1
        # Only the first fix's task_id should be in the applied list.
        assert "task-001" in variant.tactical_fixes_applied
        assert "task-003" not in variant.tactical_fixes_applied

    def test_principle_duplicate_of_fix_not_added(self, base_content: str, fix_a: TacticalFix) -> None:
        # Principle text matches a tactical fix -- should be deduped.
        variant = mutate_prompt_sync(base_content, [fix_a], [fix_a.proposed_addition], "coder")
        count = variant.content.count(fix_a.proposed_addition)
        assert count == 1
        # Source should be tactical since the principle was deduped away.
        assert variant.mutation_source == "tactical"

    def test_duplicate_principles_deduped(self, base_content: str) -> None:
        variant = mutate_prompt_sync(base_content, [], ["Be concise.", "Be concise.", "Be concise."], "coder")
        count = variant.content.count("Be concise.")
        assert count == 1


# -- validate_variant tests --


class TestValidateVariant:
    def test_within_bounds(self, base_content: str) -> None:
        # Same length -- 100% ratio, well within bounds.
        valid, reason = validate_variant(base_content, base_content)
        assert valid is True
        assert reason == "ok"

    def test_slightly_longer(self, base_content: str) -> None:
        longer = base_content + " Some additional instructions here."
        valid, _reason = validate_variant(base_content, longer)
        assert valid is True

    def test_too_short(self, base_content: str) -> None:
        # Under 50% of original length.
        short = base_content[: len(base_content) // 4]
        valid, reason = validate_variant(base_content, short)
        assert valid is False
        assert "too short" in reason

    def test_too_long(self, base_content: str) -> None:
        # Over 300% of original length.
        long = base_content * 5
        valid, reason = validate_variant(base_content, long)
        assert valid is False
        assert "too long" in reason

    def test_empty_variant(self, base_content: str) -> None:
        valid, reason = validate_variant(base_content, "")
        assert valid is False
        assert "empty" in reason

    def test_whitespace_only_variant(self, base_content: str) -> None:
        valid, reason = validate_variant(base_content, "   \n\t  ")
        assert valid is False
        assert "empty" in reason

    def test_empty_original_with_nonempty_variant(self) -> None:
        valid, reason = validate_variant("", "Some new content")
        assert valid is True
        assert reason == "ok"

    def test_exact_boundary_50_percent(self) -> None:
        original = "a" * 100
        variant = "b" * 50  # exactly 50%
        valid, _reason = validate_variant(original, variant)
        assert valid is True

    def test_exact_boundary_300_percent(self) -> None:
        original = "a" * 100
        variant = "b" * 300  # exactly 300%
        valid, _reason = validate_variant(original, variant)
        assert valid is True

    def test_just_below_50_percent(self) -> None:
        original = "a" * 100
        variant = "b" * 49  # 49% -- below threshold
        valid, reason = validate_variant(original, variant)
        assert valid is False
        assert "too short" in reason

    def test_just_above_300_percent(self) -> None:
        original = "a" * 100
        variant = "b" * 301  # 301% -- above threshold
        valid, reason = validate_variant(original, variant)
        assert valid is False
        assert "too long" in reason


# -- PromptVariant dataclass tests --


class TestPromptVariantDataclass:
    def test_fields_exist(self) -> None:
        variant = PromptVariant(
            content="test prompt",
            version=2,
            parent_id="parent-abc",
            mutation_source="tactical",
            tactical_fixes_applied=["t1", "t2"],
            strategic_principles=["p1"],
            validation_passed=True,
        )
        assert variant.content == "test prompt"
        assert variant.version == 2
        assert variant.parent_id == "parent-abc"
        assert variant.mutation_source == "tactical"
        assert variant.tactical_fixes_applied == ["t1", "t2"]
        assert variant.strategic_principles == ["p1"]
        assert variant.validation_passed is True

    def test_default_values(self) -> None:
        variant = PromptVariant(
            content="x",
            version=1,
            parent_id="",
            mutation_source="strategic",
        )
        assert variant.tactical_fixes_applied == []
        assert variant.strategic_principles == []
        assert variant.validation_passed is False

    def test_independent_default_lists(self) -> None:
        """Default list fields must not share state across instances."""
        v1 = PromptVariant(content="a", version=1, parent_id="", mutation_source="tactical")
        v2 = PromptVariant(content="b", version=1, parent_id="", mutation_source="tactical")
        v1.tactical_fixes_applied.append("x")
        assert "x" not in v2.tactical_fixes_applied


# -- async mutate_prompt tests --


class _FakeResponse:
    """Mimics ChatCompletionResponse with a .content attribute."""

    def __init__(self, content: str) -> None:
        self.content = content


class _FakeLLMClient:
    """Fake LLM client that returns a controlled response."""

    def __init__(self, response_content: str) -> None:
        self._response_content = response_content

    async def chat_completion(self, **kwargs: object) -> _FakeResponse:
        return _FakeResponse(self._response_content)


class TestMutatePromptAsync:
    def test_valid_llm_response(self, base_content: str, fix_a: TacticalFix) -> None:
        # LLM returns a variant within valid length bounds.
        rewritten = base_content + " Always handle errors gracefully."
        client = _FakeLLMClient(rewritten)
        variant = asyncio.get_event_loop().run_until_complete(mutate_prompt(base_content, [fix_a], [], "coder", client))
        assert variant.content == rewritten
        assert variant.validation_passed is True
        assert variant.mutation_source == "tactical"
        assert "task-001" in variant.tactical_fixes_applied

    def test_fallback_on_too_short_response(self, base_content: str, fix_a: TacticalFix) -> None:
        # LLM returns something too short -- should fall back to sync mutation.
        client = _FakeLLMClient("Hi")
        variant = asyncio.get_event_loop().run_until_complete(mutate_prompt(base_content, [fix_a], [], "coder", client))
        # Sync fallback appends the fix, so the content should contain it.
        assert fix_a.proposed_addition in variant.content

    def test_fallback_on_empty_response(self, base_content: str, fix_a: TacticalFix) -> None:
        client = _FakeLLMClient("")
        variant = asyncio.get_event_loop().run_until_complete(mutate_prompt(base_content, [fix_a], [], "coder", client))
        assert fix_a.proposed_addition in variant.content

    def test_combined_source_with_principles(
        self, base_content: str, fix_a: TacticalFix, principles: list[str]
    ) -> None:
        rewritten = base_content + " With error handling and input validation."
        client = _FakeLLMClient(rewritten)
        variant = asyncio.get_event_loop().run_until_complete(
            mutate_prompt(base_content, [fix_a], principles, "coder", client)
        )
        assert variant.mutation_source == "combined"
        assert variant.strategic_principles == principles

    def test_strategic_only(self, base_content: str, principles: list[str]) -> None:
        rewritten = base_content + " Always validate and be explicit."
        client = _FakeLLMClient(rewritten)
        variant = asyncio.get_event_loop().run_until_complete(
            mutate_prompt(base_content, [], principles, "coder", client)
        )
        assert variant.mutation_source == "strategic"
        assert variant.tactical_fixes_applied == []


# -- handle_mutate_request tests --


class _FakeNATSClient:
    """Captures published messages for assertion."""

    def __init__(self) -> None:
        self.published: list[tuple[str, bytes]] = []

    async def publish(self, subject: str, data: bytes) -> None:
        self.published.append((subject, data))


class TestHandleMutateRequest:
    def test_successful_mutation(self, base_content: str) -> None:
        llm = _FakeLLMClient(base_content + " Extra instructions.")
        nats = _FakeNATSClient()
        payload: dict[str, object] = {
            "current_content": base_content,
            "mode_id": "coder",
            "tactical_fixes": [
                {
                    "task_id": "t1",
                    "failure_description": "desc",
                    "root_cause": "cause",
                    "proposed_addition": "fix text",
                    "confidence": 0.8,
                }
            ],
            "strategic_principles": ["principle 1"],
        }

        asyncio.get_event_loop().run_until_complete(handle_mutate_request(payload, llm, nats))

        assert len(nats.published) == 1
        subject, data = nats.published[0]
        assert subject == "prompt.evolution.mutate.complete"
        result = json.loads(data)
        assert result["status"] == "complete"
        assert result["mode_id"] == "coder"
        assert "variant" in result
        assert result["variant"]["validation_passed"] is True

    def test_error_published_on_failure(self) -> None:
        class _BrokenLLM:
            async def chat_completion(self, **kwargs: object) -> dict[str, object]:
                raise RuntimeError("LLM is down")

        nats = _FakeNATSClient()
        payload: dict[str, object] = {
            "current_content": "some prompt",
            "mode_id": "coder",
            "tactical_fixes": [],
            "strategic_principles": [],
        }

        asyncio.get_event_loop().run_until_complete(handle_mutate_request(payload, _BrokenLLM(), nats))

        assert len(nats.published) == 1
        result = json.loads(nats.published[0][1])
        assert result["status"] == "error"
        assert "LLM is down" in result["error"]

    def test_empty_payload_fields_handled(self) -> None:
        llm = _FakeLLMClient("some prompt with additions")
        nats = _FakeNATSClient()
        payload: dict[str, object] = {}

        asyncio.get_event_loop().run_until_complete(handle_mutate_request(payload, llm, nats))

        assert len(nats.published) == 1
        result = json.loads(nats.published[0][1])
        assert result["status"] == "complete"
