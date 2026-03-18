"""Tests for conversation summarization at context exhaustion."""

from __future__ import annotations

import pytest

from codeforge.history import ConversationSummarizer
from codeforge.llm import CompletionResponse
from codeforge.models import ConversationMessagePayload
from tests.fake_llm import FakeLLM


def _msg(role: str, content: str) -> ConversationMessagePayload:
    return ConversationMessagePayload(role=role, content=content)


def _make_history(n: int) -> list[ConversationMessagePayload]:
    """Generate n alternating user/assistant messages."""
    msgs: list[ConversationMessagePayload] = []
    for i in range(n):
        role = "user" if i % 2 == 0 else "assistant"
        msgs.append(_msg(role, f"Message {i}: {'question' if role == 'user' else 'answer'} about topic {i}"))
    return msgs


@pytest.fixture
def summarizer_with_llm() -> tuple[ConversationSummarizer, FakeLLM]:
    llm = FakeLLM(
        responses=[
            CompletionResponse(
                content="Summary: The user discussed topics 0-39. Key decisions: use auth pattern.",
                tokens_in=500,
                tokens_out=50,
                model="test",
            ),
        ]
    )
    return ConversationSummarizer(llm=llm, threshold=40), llm


class TestSummarizeThreshold:
    """A2.1: Summarization triggers only when history exceeds threshold."""

    @pytest.mark.asyncio
    async def test_below_threshold_no_summarization(self) -> None:
        llm = FakeLLM(responses=[])
        summarizer = ConversationSummarizer(llm=llm, threshold=40)
        history = _make_history(20)

        result = await summarizer.summarize_if_needed(history)

        assert result == history  # unchanged
        assert len(llm.calls) == 0

    @pytest.mark.asyncio
    async def test_at_threshold_no_summarization(self) -> None:
        llm = FakeLLM(responses=[])
        summarizer = ConversationSummarizer(llm=llm, threshold=40)
        history = _make_history(40)

        result = await summarizer.summarize_if_needed(history)

        assert result == history
        assert len(llm.calls) == 0

    @pytest.mark.asyncio
    async def test_above_threshold_triggers_summarization(
        self,
        summarizer_with_llm: tuple[ConversationSummarizer, FakeLLM],
    ) -> None:
        summarizer, llm = summarizer_with_llm
        history = _make_history(50)

        result = await summarizer.summarize_if_needed(history)

        assert len(llm.calls) == 1
        assert len(result) < len(history)


class TestSummarizeTailPreservation:
    """A2.2: Recent messages (tail) are always preserved."""

    @pytest.mark.asyncio
    async def test_tail_messages_preserved(
        self,
        summarizer_with_llm: tuple[ConversationSummarizer, FakeLLM],
    ) -> None:
        summarizer, _ = summarizer_with_llm
        history = _make_history(50)
        tail_size = 20  # default min_recent_messages

        result = await summarizer.summarize_if_needed(history)

        # Last tail_size messages should be preserved exactly
        preserved = result[-tail_size:]
        original_tail = history[-tail_size:]
        for i in range(tail_size):
            assert preserved[i].content == original_tail[i].content
            assert preserved[i].role == original_tail[i].role

    @pytest.mark.asyncio
    async def test_summary_is_first_message(
        self,
        summarizer_with_llm: tuple[ConversationSummarizer, FakeLLM],
    ) -> None:
        summarizer, _ = summarizer_with_llm
        history = _make_history(50)

        result = await summarizer.summarize_if_needed(history)

        # First message should be the system-like summary
        assert result[0].role == "system"
        assert "Summary" in result[0].content


class TestSummarizePrompt:
    """A2.3: Summarization prompt includes conversation content."""

    @pytest.mark.asyncio
    async def test_prompt_contains_history(
        self,
        summarizer_with_llm: tuple[ConversationSummarizer, FakeLLM],
    ) -> None:
        summarizer, llm = summarizer_with_llm
        history = _make_history(50)

        await summarizer.summarize_if_needed(history)

        call = llm.calls[0]
        # The prompt should contain the conversation text to summarize
        assert "Message 0" in call.prompt  # head messages included


class TestSummarizeRoutingTags:
    """A2.4: Summarization LLM call uses 'background' routing tag."""

    @pytest.mark.asyncio
    async def test_uses_background_tag(
        self,
        summarizer_with_llm: tuple[ConversationSummarizer, FakeLLM],
    ) -> None:
        summarizer, llm = summarizer_with_llm
        history = _make_history(50)

        await summarizer.summarize_if_needed(history)

        assert llm.calls[0].tags == ["background"]


class TestSummarizeFallback:
    """Fallback: LLM failure returns original history."""

    @pytest.mark.asyncio
    async def test_llm_failure_returns_original(self) -> None:
        llm = FakeLLM(responses=[])  # will raise RuntimeError
        summarizer = ConversationSummarizer(llm=llm, threshold=40)
        history = _make_history(50)

        result = await summarizer.summarize_if_needed(history)

        assert result == history  # unchanged on failure
