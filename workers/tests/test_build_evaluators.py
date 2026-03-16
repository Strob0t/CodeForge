"""Tests for _build_evaluators() unknown metric rejection (Issue C)."""

from __future__ import annotations

import pytest


def test_unknown_evaluator_raises() -> None:
    """Unknown evaluator name should raise ValueError."""
    from codeforge.consumer._benchmark import _build_evaluators

    with pytest.raises(ValueError, match="unknown evaluator/metric"):
        _build_evaluators(["nonexistent_evaluator"], "gpt-4")


def test_valid_evaluators_pass() -> None:
    """Valid evaluator names should not raise."""
    from codeforge.consumer._benchmark import _build_evaluators

    evaluators = _build_evaluators(["llm_judge", "functional_test"], "gpt-4")
    assert len(evaluators) == 2  # LLMJudge + FunctionalTest


def test_mixed_valid_invalid_raises() -> None:
    """One invalid evaluator in a list should raise even if others are valid."""
    from codeforge.consumer._benchmark import _build_evaluators

    with pytest.raises(ValueError, match="unknown evaluator/metric"):
        _build_evaluators(["llm_judge", "invalid_metric"], "gpt-4")


def test_valid_sub_metrics_pass() -> None:
    """LLM judge sub-metrics (correctness, faithfulness, etc.) should be valid."""
    from codeforge.consumer._benchmark import _build_evaluators

    evaluators = _build_evaluators(["correctness", "faithfulness"], "gpt-4")
    assert len(evaluators) == 1  # Single LLMJudge with 2 metrics


def test_all_four_evaluators_pass() -> None:
    """All four main evaluator names should be valid."""
    from codeforge.consumer._benchmark import _build_evaluators

    evaluators = _build_evaluators(
        ["llm_judge", "functional_test", "sparc", "trajectory_verifier"],
        "gpt-4",
    )
    assert len(evaluators) == 4


def test_empty_string_metric_raises() -> None:
    """Empty string metric name should raise."""
    from codeforge.consumer._benchmark import _build_evaluators

    with pytest.raises(ValueError, match="unknown evaluator/metric"):
        _build_evaluators([""], "gpt-4")


def test_empty_list_raises() -> None:
    """Empty evaluator list should raise because no evaluators are produced."""
    from codeforge.consumer._benchmark import _build_evaluators

    with pytest.raises(ValueError, match="no valid evaluators produced"):
        _build_evaluators([], "gpt-4")
