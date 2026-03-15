"""Tests for prompt compressor utility and its integration with evaluators.

Tests cover:
- Short input returned unchanged
- Long input truncated with [truncated] marker
- Head (60%) and tail (40%) preserved
- Zero/negative max_chars returns empty string
- Empty string input
- Exact boundary (len == max_chars)
- Integration: LLM Judge uses compressed inputs
- Integration: Trajectory Verifier uses compressed inputs
- Error fallback enhancement in LLM Judge
"""

from __future__ import annotations

from unittest.mock import AsyncMock, patch

import pytest

from codeforge.evaluation.evaluators.prompt_compressor import compress_for_context
from codeforge.evaluation.providers.base import (
    ExecutionResult,
    TaskSpec,
    TrajectoryMessage,
)

# ---------------------------------------------------------------------------
# Unit tests: compress_for_context
# ---------------------------------------------------------------------------


class TestCompressForContext:
    def test_short_input_unchanged(self) -> None:
        """Input shorter than max_chars is returned verbatim."""
        text = "Hello, world!"
        result = compress_for_context(text, max_chars=100)
        assert result == text

    def test_exact_boundary_unchanged(self) -> None:
        """Input exactly at max_chars is returned verbatim."""
        text = "x" * 50
        result = compress_for_context(text, max_chars=50)
        assert result == text

    def test_long_input_truncated_with_marker(self) -> None:
        """Input longer than max_chars gets head+tail with [truncated] marker."""
        text = "A" * 100
        result = compress_for_context(text, max_chars=50)
        assert "[truncated]" in result
        assert len(result) <= 50 + len("\n[truncated]\n")  # marker adds some overhead

    def test_head_and_tail_preserved(self) -> None:
        """Head gets ~60% budget and tail gets ~40% budget."""
        # Create text with distinct head and tail
        head_part = "H" * 500
        tail_part = "T" * 500
        text = head_part + tail_part
        max_chars = 100

        result = compress_for_context(text, max_chars)

        # Should start with 'H's (head) and end with 'T's (tail)
        assert result.startswith("H")
        assert result.endswith("T")
        assert "[truncated]" in result

        # Verify head is ~60% and tail is ~40% of budget
        parts = result.split("\n[truncated]\n")
        assert len(parts) == 2
        head_len = len(parts[0])
        tail_len = len(parts[1])
        # Budget = 100 - len("\n[truncated]\n") = 87
        # Head = int(87 * 0.6) = 52, Tail = 87 - 52 = 35
        assert head_len == 52, f"head_len={head_len}"
        assert tail_len == 35, f"tail_len={tail_len}"

    def test_zero_max_chars_returns_empty(self) -> None:
        """max_chars=0 returns empty string."""
        result = compress_for_context("some text", max_chars=0)
        assert result == ""

    def test_negative_max_chars_returns_empty(self) -> None:
        """Negative max_chars returns empty string."""
        result = compress_for_context("some text", max_chars=-10)
        assert result == ""

    def test_empty_string_input(self) -> None:
        """Empty string input returns empty string."""
        result = compress_for_context("", max_chars=100)
        assert result == ""

    def test_preserves_content_fidelity(self) -> None:
        """Head and tail contain actual text from the original."""
        text = "START_" + "m" * 200 + "_END"
        result = compress_for_context(text, max_chars=50)
        assert result.startswith("START_")
        assert result.endswith("_END")

    def test_unicode_content(self) -> None:
        """Unicode characters are handled correctly."""
        text = "Hello " + "\u2603" * 200 + " World"
        result = compress_for_context(text, max_chars=50)
        assert "[truncated]" in result


# ---------------------------------------------------------------------------
# Integration: LLM Judge compression
# ---------------------------------------------------------------------------


class TestLLMJudgeCompression:
    def test_llm_judge_compresses_long_inputs(self) -> None:
        """LLM Judge should compress task/result fields before metric calls."""
        from codeforge.evaluation.evaluators.llm_judge import (
            _MAX_EXPECTED_CHARS,
            _MAX_INPUT_CHARS,
            _MAX_OUTPUT_CHARS,
        )

        # Verify constants are defined
        assert _MAX_INPUT_CHARS == 4000
        assert _MAX_OUTPUT_CHARS == 4000
        assert _MAX_EXPECTED_CHARS == 2000

    @pytest.mark.asyncio
    async def test_llm_judge_passes_compressed_to_metrics(self) -> None:
        """The evaluate() method should compress before calling _run_metric."""
        from codeforge.evaluation.evaluators.llm_judge import LLMJudgeEvaluator

        long_input = "I" * 10000
        long_output = "O" * 10000
        long_expected = "E" * 5000

        task = TaskSpec(
            id="t1",
            name="Test",
            input=long_input,
            expected_output=long_expected,
            difficulty="hard",
            metadata={"key": "value"},
        )
        result = ExecutionResult(actual_output=long_output, cost_usd=0.01)

        evaluator = LLMJudgeEvaluator(metrics=["correctness"])

        captured_args: dict[str, object] = {}

        async def fake_run_metric(name: str, t: TaskSpec, r: ExecutionResult) -> float:
            captured_args["task_input"] = t.input
            captured_args["task_expected"] = t.expected_output
            captured_args["result_output"] = r.actual_output
            # Preserve all other fields
            captured_args["task_difficulty"] = t.difficulty
            captured_args["task_metadata"] = t.metadata
            return 0.9

        with patch.object(evaluator, "_run_metric", side_effect=fake_run_metric):
            dims = await evaluator.evaluate(task, result)

        # Fields should be compressed
        assert len(str(captured_args["task_input"])) < len(long_input)
        assert len(str(captured_args["task_expected"])) < len(long_expected)
        assert len(str(captured_args["result_output"])) < len(long_output)
        # Other fields preserved via model_copy
        assert captured_args["task_difficulty"] == "hard"
        assert captured_args["task_metadata"] == {"key": "value"}
        # Score should still come through
        assert len(dims) == 1
        assert dims[0].score == 0.9

    @pytest.mark.asyncio
    async def test_llm_judge_short_inputs_unchanged(self) -> None:
        """Short inputs should pass through without modification."""
        from codeforge.evaluation.evaluators.llm_judge import LLMJudgeEvaluator

        task = TaskSpec(
            id="t1",
            name="Test",
            input="short input",
            expected_output="short expected",
        )
        result = ExecutionResult(actual_output="short output")

        evaluator = LLMJudgeEvaluator(metrics=["correctness"])

        captured_args: dict[str, object] = {}

        async def fake_run_metric(name: str, t: TaskSpec, r: ExecutionResult) -> float:
            captured_args["task_input"] = t.input
            captured_args["task_expected"] = t.expected_output
            captured_args["result_output"] = r.actual_output
            return 0.8

        with patch.object(evaluator, "_run_metric", side_effect=fake_run_metric):
            await evaluator.evaluate(task, result)

        assert captured_args["task_input"] == "short input"
        assert captured_args["task_expected"] == "short expected"
        assert captured_args["result_output"] == "short output"


# ---------------------------------------------------------------------------
# Integration: LLM Judge error fallback enhancement
# ---------------------------------------------------------------------------


class TestLLMJudgeErrorFallback:
    @pytest.mark.asyncio
    async def test_context_overflow_error_detected(self) -> None:
        """Context overflow errors should be tagged as context_overflow."""
        from codeforge.evaluation.evaluators.llm_judge import LLMJudgeEvaluator

        task = TaskSpec(id="t1", name="Test", input="test")
        result = ExecutionResult(actual_output="some output")

        evaluator = LLMJudgeEvaluator(metrics=["correctness"])

        with patch.object(
            evaluator,
            "_run_metric",
            side_effect=RuntimeError("context length exceeded - 400 error"),
        ):
            dims = await evaluator.evaluate(task, result)

        assert len(dims) == 1
        assert dims[0].score == 0.0
        assert dims[0].details["error"] == "context_overflow"
        assert "context length exceeded" in dims[0].details["error_message"]

    @pytest.mark.asyncio
    async def test_400_error_detected_as_context_overflow(self) -> None:
        """Errors containing '400' should be tagged as context_overflow."""
        from codeforge.evaluation.evaluators.llm_judge import LLMJudgeEvaluator

        task = TaskSpec(id="t1", name="Test", input="test")
        result = ExecutionResult(actual_output="some output")

        evaluator = LLMJudgeEvaluator(metrics=["correctness"])

        with patch.object(
            evaluator,
            "_run_metric",
            side_effect=RuntimeError("HTTP 400 Bad Request"),
        ):
            dims = await evaluator.evaluate(task, result)

        assert dims[0].details["error"] == "context_overflow"

    @pytest.mark.asyncio
    async def test_generic_error_fallback(self) -> None:
        """Non-context errors should be tagged as evaluation_failed."""
        from codeforge.evaluation.evaluators.llm_judge import LLMJudgeEvaluator

        task = TaskSpec(id="t1", name="Test", input="test")
        result = ExecutionResult(actual_output="some output")

        evaluator = LLMJudgeEvaluator(metrics=["correctness"])

        with patch.object(
            evaluator,
            "_run_metric",
            side_effect=RuntimeError("some random error"),
        ):
            dims = await evaluator.evaluate(task, result)

        assert dims[0].details["error"] == "evaluation_failed"
        assert "some random error" in dims[0].details["error_message"]

    @pytest.mark.asyncio
    async def test_error_message_truncated(self) -> None:
        """Long error messages should be truncated to 200 chars."""
        from codeforge.evaluation.evaluators.llm_judge import LLMJudgeEvaluator

        task = TaskSpec(id="t1", name="Test", input="test")
        result = ExecutionResult(actual_output="some output")

        evaluator = LLMJudgeEvaluator(metrics=["correctness"])

        long_error = "e" * 500
        with patch.object(
            evaluator,
            "_run_metric",
            side_effect=RuntimeError(long_error),
        ):
            dims = await evaluator.evaluate(task, result)

        assert len(dims[0].details["error_message"]) <= 200


# ---------------------------------------------------------------------------
# Integration: Trajectory Verifier compression
# ---------------------------------------------------------------------------


class TestTrajectoryVerifierCompression:
    @pytest.mark.asyncio
    async def test_trajectory_verifier_compresses_long_inputs(self) -> None:
        """Trajectory verifier should compress task input, expected output, and trajectory."""
        from codeforge.evaluation.evaluators.trajectory_verifier import (
            _MAX_TASK_INPUT_CHARS,
            _MAX_TRAJECTORY_CHARS,
        )

        # Verify constants exist
        assert _MAX_TASK_INPUT_CHARS == 2000
        assert _MAX_TRAJECTORY_CHARS == 4000

    @pytest.mark.asyncio
    async def test_trajectory_verifier_uses_compression(self) -> None:
        """The evaluate() method should compress inputs before building the prompt."""
        from codeforge.evaluation.evaluators.trajectory_verifier import (
            TrajectoryVerifierEvaluator,
        )

        long_input = "I" * 10000
        task = TaskSpec(
            id="t1",
            name="Test",
            input=long_input,
            expected_output="E" * 5000,
        )
        result = ExecutionResult(
            actual_output="output",
            files_changed=["test.py"],
            test_output="1 passed",
            trajectory=[
                TrajectoryMessage(role="user", content="x" * 3000),
                TrajectoryMessage(role="assistant", content="y" * 3000),
            ],
        )

        mock_response = AsyncMock()
        mock_response.choices = [AsyncMock()]
        mock_response.choices[0].message.content = (
            '{"solution_quality": 0.8, "approach_efficiency": 0.7, '
            '"code_quality": 0.6, "error_recovery": 0.5, "completeness": 0.9}'
        )

        evaluator = TrajectoryVerifierEvaluator(model="test-model")

        captured_prompt: list[str] = []

        async def capture_call(prompt: str) -> object:
            captured_prompt.append(prompt)
            return mock_response

        with patch.object(evaluator, "_call_verifier", side_effect=capture_call):
            dims = await evaluator.evaluate(task, result)

        assert len(dims) == 5
        # The prompt should NOT contain the full 10000-char input
        assert len(captured_prompt[0]) < len(long_input)
