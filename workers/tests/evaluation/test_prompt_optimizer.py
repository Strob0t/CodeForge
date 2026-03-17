"""Tests for prompt_optimizer — reflection logic and failure analysis.

Covers:
- reflect_on_failures_sync() with sample failures
- reflect_on_failures_sync() with empty failures
- _classify_failure_pattern() heuristic
- _is_failed() edge cases
- reflect_on_failures() async with mocked LLM
- handle_reflect_request() NATS handler
"""

from __future__ import annotations

import json
from unittest.mock import AsyncMock

import pytest

from codeforge.evaluation.prompt_optimizer import (
    PromptAnalysisReport,
    TacticalFix,
    _classify_failure_pattern,
    _is_failed,
    handle_reflect_request,
    reflect_on_failures,
    reflect_on_failures_sync,
)

# ---------------------------------------------------------------------------
# Sample failure fixtures
# ---------------------------------------------------------------------------


def _make_failure(
    task_id: str,
    scores: dict[str, float] | None = None,
    actual_output: str = "",
    expected_output: str = "",
    error: str = "",
    tool_calls: list[dict[str, str]] | None = None,
    trace: str = "",
) -> dict[str, object]:
    result: dict[str, object] = {"task_id": task_id}
    if scores is not None:
        result["scores"] = scores
    if actual_output:
        result["actual_output"] = actual_output
    if expected_output:
        result["expected_output"] = expected_output
    if error:
        result["error"] = error
    if tool_calls is not None:
        result["tool_calls"] = tool_calls
    if trace:
        result["trace"] = trace
    return result


@pytest.fixture
def tool_misuse_failures() -> list[dict[str, object]]:
    return [
        _make_failure(
            "t1",
            scores={"correctness": 0.2},
            error="tool 'Write' called with invalid arguments",
            tool_calls=[{"name": "Write", "args": "bad"}],
        ),
        _make_failure(
            "t2",
            scores={"correctness": 0.1},
            error="tool call failed: wrong tool name 'Writ'",
            tool_calls=[{"name": "Writ", "args": "{}"}],
        ),
    ]


@pytest.fixture
def format_error_failures() -> list[dict[str, object]]:
    return [
        _make_failure(
            "t3",
            scores={"correctness": 0.0},
            error="JSON parse error in output",
            actual_output="not json {[",
        ),
        _make_failure(
            "t4",
            scores={"correctness": 0.3},
            error="syntax error: unexpected token",
            actual_output="def foo( return",
        ),
    ]


@pytest.fixture
def wrong_approach_failures() -> list[dict[str, object]]:
    return [
        _make_failure(
            "t5",
            scores={"correctness": 0.4},
            actual_output="used brute force O(n^2)",
            expected_output="optimal O(n log n)",
            trace="Agent tried brute force approach instead of sorting",
        ),
    ]


@pytest.fixture
def mixed_failures(
    tool_misuse_failures: list[dict[str, object]],
    format_error_failures: list[dict[str, object]],
    wrong_approach_failures: list[dict[str, object]],
) -> list[dict[str, object]]:
    return tool_misuse_failures + format_error_failures + wrong_approach_failures


# ---------------------------------------------------------------------------
# _is_failed() tests
# ---------------------------------------------------------------------------


class TestIsFailed:
    """Test the _is_failed helper for various edge cases."""

    def test_no_scores_key(self) -> None:
        """Result without scores is considered failed."""
        assert _is_failed({}) is True

    def test_empty_scores(self) -> None:
        """Empty scores dict is considered failed."""
        assert _is_failed({"scores": {}}) is True

    def test_scores_not_dict(self) -> None:
        """Non-dict scores is considered failed."""
        assert _is_failed({"scores": "invalid"}) is True
        assert _is_failed({"scores": None}) is True
        assert _is_failed({"scores": []}) is True

    def test_low_average_score(self) -> None:
        """Average score below 0.5 is failed."""
        assert _is_failed({"scores": {"a": 0.3, "b": 0.2}}) is True

    def test_exactly_half(self) -> None:
        """Average of exactly 0.5 is not failed (boundary)."""
        assert _is_failed({"scores": {"a": 0.5}}) is False

    def test_high_scores(self) -> None:
        """High scores are not failed."""
        assert _is_failed({"scores": {"a": 0.9, "b": 0.8}}) is False

    def test_single_zero_score(self) -> None:
        assert _is_failed({"scores": {"a": 0.0}}) is True

    def test_single_perfect_score(self) -> None:
        assert _is_failed({"scores": {"a": 1.0}}) is False

    def test_mixed_scores_above_threshold(self) -> None:
        """Mixed scores that average above 0.5."""
        assert _is_failed({"scores": {"a": 0.2, "b": 0.9}}) is False

    def test_mixed_scores_below_threshold(self) -> None:
        """Mixed scores that average below 0.5."""
        assert _is_failed({"scores": {"a": 0.1, "b": 0.4}}) is True

    def test_string_score_values(self) -> None:
        """Scores with string-convertible values."""
        assert _is_failed({"scores": {"a": "0.8"}}) is False


# ---------------------------------------------------------------------------
# _classify_failure_pattern() tests
# ---------------------------------------------------------------------------


class TestClassifyFailurePattern:
    """Test heuristic failure classification."""

    def test_tool_misuse_from_error(self) -> None:
        failure = _make_failure("t1", error="tool 'Write' called with invalid arguments")
        assert _classify_failure_pattern(failure) == "tool_misuse"

    def test_tool_misuse_from_tool_calls(self) -> None:
        failure = _make_failure(
            "t1",
            tool_calls=[{"name": "BadTool", "args": "{}"}],
            error="tool call failed",
        )
        assert _classify_failure_pattern(failure) == "tool_misuse"

    def test_format_error_json(self) -> None:
        failure = _make_failure("t1", error="JSON parse error")
        assert _classify_failure_pattern(failure) == "format_error"

    def test_format_error_syntax(self) -> None:
        failure = _make_failure("t1", error="syntax error: unexpected token")
        assert _classify_failure_pattern(failure) == "format_error"

    def test_format_error_parse(self) -> None:
        failure = _make_failure("t1", error="parse error in response")
        assert _classify_failure_pattern(failure) == "format_error"

    def test_wrong_approach_from_trace(self) -> None:
        failure = _make_failure(
            "t1",
            trace="Agent tried brute force approach instead of sorting",
            actual_output="wrong result",
            expected_output="correct result",
        )
        assert _classify_failure_pattern(failure) == "wrong_approach"

    def test_wrong_approach_from_output_mismatch(self) -> None:
        failure = _make_failure(
            "t1",
            actual_output="completely wrong answer",
            expected_output="expected answer",
        )
        assert _classify_failure_pattern(failure) == "wrong_approach"

    def test_other_no_signals(self) -> None:
        failure = _make_failure("t1")
        assert _classify_failure_pattern(failure) == "other"

    def test_other_empty_error(self) -> None:
        failure = _make_failure("t1", error="", actual_output="")
        assert _classify_failure_pattern(failure) == "other"

    def test_tool_misuse_precedence_over_format(self) -> None:
        """Tool misuse keywords should match before format error keywords."""
        failure = _make_failure("t1", error="tool call failed with parse error")
        assert _classify_failure_pattern(failure) == "tool_misuse"


# ---------------------------------------------------------------------------
# reflect_on_failures_sync() tests
# ---------------------------------------------------------------------------


class TestReflectOnFailuresSync:
    """Test the sync reflection function (no LLM)."""

    def test_basic_clustering(self, mixed_failures: list[dict[str, object]]) -> None:
        """Failures are grouped by pattern and produce TacticalFix entries."""
        report = reflect_on_failures_sync(
            failures=mixed_failures,
            current_prompt="You are a coding assistant.",
            mode_id="coder",
            model_family="openai",
        )
        assert isinstance(report, PromptAnalysisReport)
        assert report.mode == "coder"
        assert report.model_family == "openai"
        assert report.total_tasks == 5
        assert report.failed_tasks == 5
        assert report.failure_rate == 1.0
        assert len(report.tactical_fixes) == 5

    def test_tactical_fix_fields(self, tool_misuse_failures: list[dict[str, object]]) -> None:
        """Each TacticalFix has required fields populated."""
        report = reflect_on_failures_sync(
            failures=tool_misuse_failures,
            current_prompt="prompt",
            mode_id="coder",
            model_family="anthropic",
        )
        for fix in report.tactical_fixes:
            assert isinstance(fix, TacticalFix)
            assert fix.task_id != ""
            assert fix.failure_description != ""
            assert fix.root_cause != ""
            assert fix.confidence >= 0.0

    def test_empty_failures(self) -> None:
        """Empty failure list returns an empty report."""
        report = reflect_on_failures_sync(
            failures=[],
            current_prompt="prompt",
            mode_id="coder",
            model_family="openai",
        )
        assert report.total_tasks == 0
        assert report.failed_tasks == 0
        assert report.failure_rate == 0.0
        assert len(report.tactical_fixes) == 0
        assert len(report.strategic_principles) == 0

    def test_pattern_clustering_counts(self, mixed_failures: list[dict[str, object]]) -> None:
        """Verify that strategic_principles mention the identified patterns."""
        report = reflect_on_failures_sync(
            failures=mixed_failures,
            current_prompt="prompt",
            mode_id="coder",
            model_family="openai",
        )
        # Strategic principles should be populated for clusters found
        assert len(report.strategic_principles) > 0

    def test_all_passing_results(self) -> None:
        """Results with passing scores produce no tactical fixes."""
        passing = [
            _make_failure("t1", scores={"a": 0.9}),
            _make_failure("t2", scores={"a": 0.8}),
        ]
        report = reflect_on_failures_sync(
            failures=passing,
            current_prompt="prompt",
            mode_id="coder",
            model_family="openai",
        )
        assert report.total_tasks == 2
        assert report.failed_tasks == 0
        assert len(report.tactical_fixes) == 0

    def test_mixed_pass_fail(self) -> None:
        """Only failed results produce tactical fixes."""
        mixed = [
            _make_failure("pass1", scores={"a": 0.9}),
            _make_failure("fail1", scores={"a": 0.1}, error="tool call failed"),
        ]
        report = reflect_on_failures_sync(
            failures=mixed,
            current_prompt="prompt",
            mode_id="coder",
            model_family="openai",
        )
        assert report.total_tasks == 2
        assert report.failed_tasks == 1
        assert len(report.tactical_fixes) == 1
        assert report.tactical_fixes[0].task_id == "fail1"

    def test_failure_rate_calculation(self) -> None:
        """Failure rate is correctly computed."""
        failures = [
            _make_failure("t1", scores={"a": 0.1}),
            _make_failure("t2", scores={"a": 0.9}),
            _make_failure("t3", scores={"a": 0.2}),
            _make_failure("t4", scores={"a": 0.8}),
        ]
        report = reflect_on_failures_sync(
            failures=failures,
            current_prompt="prompt",
            mode_id="coder",
            model_family="openai",
        )
        assert report.failure_rate == pytest.approx(0.5)

    def test_root_cause_mentions_pattern(self, tool_misuse_failures: list[dict[str, object]]) -> None:
        """Sync root cause should reference the classified pattern."""
        report = reflect_on_failures_sync(
            failures=tool_misuse_failures,
            current_prompt="prompt",
            mode_id="coder",
            model_family="openai",
        )
        for fix in report.tactical_fixes:
            assert "tool_misuse" in fix.root_cause.lower()


# ---------------------------------------------------------------------------
# reflect_on_failures() async tests
# ---------------------------------------------------------------------------


class TestReflectOnFailuresAsync:
    """Test the async reflection function with mocked LLM."""

    @pytest.mark.asyncio
    async def test_calls_llm_and_parses_response(self, mixed_failures: list[dict[str, object]]) -> None:
        """LLM is called, and its JSON response is parsed into TacticalFixes."""
        from codeforge.llm import ChatCompletionResponse

        llm_response_data = {
            "tactical_fixes": [
                {
                    "task_id": "t1",
                    "failure_description": "Tool Write misused",
                    "root_cause": "Agent confused Write argument format",
                    "proposed_addition": "Always pass file_path as first arg to Write",
                    "confidence": 0.85,
                },
            ],
            "strategic_principles": [
                "Add explicit tool argument documentation to system prompt",
            ],
            "few_shot_candidates": [],
        }

        mock_llm = AsyncMock()
        mock_llm.chat_completion.return_value = ChatCompletionResponse(
            content=json.dumps(llm_response_data),
            tool_calls=[],
            finish_reason="stop",
            tokens_in=500,
            tokens_out=200,
            model="gpt-4",
        )

        report = await reflect_on_failures(
            failures=mixed_failures,
            current_prompt="You are a coding agent.",
            mode_id="coder",
            model_family="openai",
            llm_client=mock_llm,
        )

        mock_llm.chat_completion.assert_awaited_once()
        assert len(report.tactical_fixes) == 1
        assert report.tactical_fixes[0].task_id == "t1"
        assert report.tactical_fixes[0].confidence == 0.85
        assert len(report.strategic_principles) == 1
        assert report.mode == "coder"
        assert report.model_family == "openai"

    @pytest.mark.asyncio
    async def test_llm_returns_invalid_json(self, mixed_failures: list[dict[str, object]]) -> None:
        """When LLM returns non-JSON, fall back to sync analysis."""
        from codeforge.llm import ChatCompletionResponse

        mock_llm = AsyncMock()
        mock_llm.chat_completion.return_value = ChatCompletionResponse(
            content="This is not JSON at all",
            tool_calls=[],
            finish_reason="stop",
            tokens_in=100,
            tokens_out=50,
            model="gpt-4",
        )

        report = await reflect_on_failures(
            failures=mixed_failures,
            current_prompt="prompt",
            mode_id="coder",
            model_family="openai",
            llm_client=mock_llm,
        )

        # Should still get a valid report with sync-generated fixes
        assert isinstance(report, PromptAnalysisReport)
        assert report.failed_tasks == 5
        assert len(report.tactical_fixes) > 0

    @pytest.mark.asyncio
    async def test_empty_failures_skips_llm(self) -> None:
        """Empty failures should not call the LLM."""
        mock_llm = AsyncMock()

        report = await reflect_on_failures(
            failures=[],
            current_prompt="prompt",
            mode_id="coder",
            model_family="openai",
            llm_client=mock_llm,
        )

        mock_llm.chat_completion.assert_not_awaited()
        assert report.total_tasks == 0
        assert report.failed_tasks == 0

    @pytest.mark.asyncio
    async def test_llm_exception_falls_back(self, mixed_failures: list[dict[str, object]]) -> None:
        """When LLM raises an exception, fall back to sync analysis."""
        mock_llm = AsyncMock()
        mock_llm.chat_completion.side_effect = RuntimeError("LLM unavailable")

        report = await reflect_on_failures(
            failures=mixed_failures,
            current_prompt="prompt",
            mode_id="coder",
            model_family="openai",
            llm_client=mock_llm,
        )

        assert isinstance(report, PromptAnalysisReport)
        assert report.failed_tasks == 5
        assert len(report.tactical_fixes) > 0


# ---------------------------------------------------------------------------
# handle_reflect_request() tests
# ---------------------------------------------------------------------------


class TestHandleReflectRequest:
    """Test the NATS handler for reflection requests."""

    @pytest.mark.asyncio
    async def test_publishes_result(self) -> None:
        """Handler calls reflect and publishes result to correct subject."""
        from codeforge.llm import ChatCompletionResponse

        llm_response_data = {
            "tactical_fixes": [
                {
                    "task_id": "t1",
                    "failure_description": "error",
                    "root_cause": "cause",
                    "proposed_addition": "fix",
                    "confidence": 0.9,
                },
            ],
            "strategic_principles": ["principle"],
            "few_shot_candidates": [],
        }

        mock_llm = AsyncMock()
        mock_llm.chat_completion.return_value = ChatCompletionResponse(
            content=json.dumps(llm_response_data),
            tool_calls=[],
            finish_reason="stop",
            tokens_in=100,
            tokens_out=50,
            model="gpt-4",
        )

        mock_nats = AsyncMock()

        payload = {
            "failures": [
                {"task_id": "t1", "scores": {"a": 0.1}, "error": "tool call failed"},
            ],
            "current_prompt": "You are a coder.",
            "mode_id": "coder",
            "model_family": "openai",
        }

        await handle_reflect_request(payload, mock_llm, mock_nats)

        mock_nats.publish.assert_awaited_once()
        call_args = mock_nats.publish.call_args
        assert call_args[0][0] == "prompt.evolution.reflect.complete"
        published_data = json.loads(call_args[0][1])
        assert published_data["mode"] == "coder"
        assert published_data["model_family"] == "openai"

    @pytest.mark.asyncio
    async def test_missing_fields_uses_defaults(self) -> None:
        """Payload with missing optional fields uses safe defaults."""
        mock_llm = AsyncMock()
        mock_nats = AsyncMock()

        payload: dict[str, object] = {
            "failures": [],
        }

        await handle_reflect_request(payload, mock_llm, mock_nats)

        mock_nats.publish.assert_awaited_once()
        call_args = mock_nats.publish.call_args
        published_data = json.loads(call_args[0][1])
        assert published_data["mode"] == ""
        assert published_data["total_tasks"] == 0

    @pytest.mark.asyncio
    async def test_handler_error_publishes_error(self) -> None:
        """When reflection fails, handler publishes an error payload."""
        mock_llm = AsyncMock()
        mock_llm.chat_completion.side_effect = RuntimeError("boom")
        mock_nats = AsyncMock()

        payload = {
            "failures": [
                {"task_id": "t1", "scores": {"a": 0.1}},
            ],
            "current_prompt": "prompt",
            "mode_id": "coder",
            "model_family": "openai",
        }

        # Should not raise — errors are published to NATS
        await handle_reflect_request(payload, mock_llm, mock_nats)

        mock_nats.publish.assert_awaited_once()
        call_args = mock_nats.publish.call_args
        assert call_args[0][0] == "prompt.evolution.reflect.complete"
