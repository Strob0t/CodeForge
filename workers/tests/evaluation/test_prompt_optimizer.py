"""Tests for prompt optimizer analysis."""

from codeforge.evaluation.prompt_optimizer import (
    PromptAnalysisReport,
    TacticalFix,
    analyze_failures,
)


def test_analyze_failures_returns_report():
    failures = [
        {
            "task_id": "t-1",
            "input": "Write a function to reverse a string",
            "expected_output": "def reverse(s): return s[::-1]",
            "actual_output": "def reverse(s): return reversed(s)",
            "scores": {"correctness": 0.3},
        },
    ]
    report = analyze_failures(
        failures=failures,
        mode="coder",
        model_family="meta-llama",
        llm_client=None,
    )
    assert isinstance(report, PromptAnalysisReport)
    assert report.mode == "coder"
    assert report.model_family == "meta-llama"
    assert report.total_tasks == 1
    assert report.failed_tasks == 1


def test_tactical_fix_structure():
    fix = TacticalFix(
        task_id="t-1",
        failure_description="returned generator instead of string",
        root_cause="model confused reversed() with slicing",
        proposed_addition="Always use slice notation s[::-1] for string reversal",
        confidence=0.8,
    )
    assert fix.confidence == 0.8
    assert "slice" in fix.proposed_addition


def test_analyze_no_failures():
    results = [
        {"task_id": "t-1", "scores": {"correctness": 0.9}},
        {"task_id": "t-2", "scores": {"correctness": 0.8}},
    ]
    report = analyze_failures(results, mode="coder", model_family="openai")
    assert report.failed_tasks == 0
    assert report.failure_rate == 0.0
    assert len(report.tactical_fixes) == 0


def test_analyze_empty_scores_counts_as_failure():
    results: list[dict] = [{"task_id": "t-1", "scores": {}}]
    report = analyze_failures(results, mode="coder", model_family="openai")
    assert report.failed_tasks == 1


def test_analyze_empty_list():
    report = analyze_failures([], mode="coder", model_family="openai")
    assert report.total_tasks == 0
    assert report.failure_rate == 0.0


def test_analyze_mixed_pass_fail():
    results = [
        {"task_id": "t-1", "scores": {"correctness": 0.9}},
        {"task_id": "t-2", "scores": {"correctness": 0.3}},
        {"task_id": "t-3", "scores": {"correctness": 0.7}},
    ]
    report = analyze_failures(results, mode="coder", model_family="openai")
    assert report.total_tasks == 3
    assert report.failed_tasks == 1
    assert report.failure_rate == 1 / 3


def test_analyze_suite_and_run_ids():
    report = analyze_failures(
        [],
        mode="coder",
        model_family="openai",
        suite_id="suite-123",
        run_id="run-456",
    )
    assert report.suite_id == "suite-123"
    assert report.run_id == "run-456"


def test_analyze_borderline_score():
    """Score of exactly 0.5 should NOT be counted as failed."""
    results = [{"task_id": "t-1", "scores": {"correctness": 0.5}}]
    report = analyze_failures(results, mode="coder", model_family="openai")
    assert report.failed_tasks == 0


def test_analyze_non_dict_scores_counts_as_failure():
    """Non-dict scores should be treated as failure."""
    results: list[dict] = [{"task_id": "t-1", "scores": "invalid"}]
    report = analyze_failures(results, mode="coder", model_family="openai")
    assert report.failed_tasks == 1
