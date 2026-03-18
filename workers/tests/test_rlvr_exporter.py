"""Tests for RLVR (Reinforcement Learning from Verifiable Rewards) export.

Tests cover: reward computation (weighted average, clamping, edge cases),
entry formatting, exporter (multi-result, zero-results), JSONL serialization.
"""

from __future__ import annotations

import json

from codeforge.evaluation.export.rlvr_exporter import (
    RLVREntry,
    RLVRExporter,
    compute_rlvr_reward,
    format_rlvr_entry,
)
from codeforge.models import BenchmarkTaskResult


def _result(
    task_id: str = "t1",
    task_name: str = "Fix bug",
    actual_output: str = "output",
    scores: dict[str, float] | None = None,
    cost_usd: float = 0.01,
    tokens_in: int = 100,
    tokens_out: int = 50,
) -> BenchmarkTaskResult:
    return BenchmarkTaskResult(
        task_id=task_id,
        task_name=task_name,
        actual_output=actual_output,
        scores=scores or {"correctness": 0.5},
        cost_usd=cost_usd,
        tokens_in=tokens_in,
        tokens_out=tokens_out,
    )


class TestComputeRLVRReward:
    def test_single_score(self) -> None:
        """Single score returns that score as the reward."""
        reward = compute_rlvr_reward({"correctness": 0.8})
        assert abs(reward - 0.8) < 1e-9

    def test_functional_test_weighted(self) -> None:
        """functional_test gets 2x weight; others get 1x.

        functional_test=0.9 (weight 2), correctness=0.5 (weight 1)
        weighted_avg = (0.9*2 + 0.5*1) / (2+1) = 2.3/3 = 0.7666...
        """
        reward = compute_rlvr_reward({"functional_test": 0.9, "correctness": 0.5})
        expected = (0.9 * 2.0 + 0.5 * 1.0) / 3.0
        assert abs(reward - expected) < 1e-9

    def test_empty_scores_returns_zero(self) -> None:
        """Empty scores dict returns 0.0."""
        reward = compute_rlvr_reward({})
        assert reward == 0.0

    def test_clamp_to_one(self) -> None:
        """Scores > 1.0 get clamped to 1.0."""
        reward = compute_rlvr_reward({"correctness": 1.5})
        assert reward == 1.0

    def test_clamp_to_zero(self) -> None:
        """Negative scores get clamped to 0.0."""
        reward = compute_rlvr_reward({"correctness": -0.3})
        assert reward == 0.0

    def test_multiple_scores_unweighted(self) -> None:
        """Multiple scores without functional_test → simple average."""
        reward = compute_rlvr_reward({"correctness": 0.6, "relevance": 0.8})
        expected = (0.6 + 0.8) / 2.0
        assert abs(reward - expected) < 1e-9

    def test_only_functional_test(self) -> None:
        """Only functional_test → its value (weighted avg = value)."""
        reward = compute_rlvr_reward({"functional_test": 0.75})
        assert abs(reward - 0.75) < 1e-9

    def test_all_zero_scores(self) -> None:
        """All zero scores → reward is 0.0."""
        reward = compute_rlvr_reward({"correctness": 0.0, "functional_test": 0.0})
        assert reward == 0.0

    def test_mixed_clamp(self) -> None:
        """Average that exceeds 1.0 is clamped.

        functional_test=1.5 (weight 2) → avg = 1.5 → clamped to 1.0
        """
        reward = compute_rlvr_reward({"functional_test": 1.5})
        assert reward == 1.0


class TestFormatRLVREntry:
    def test_basic_format(self) -> None:
        """Basic formatting produces expected fields."""
        entry = format_rlvr_entry(
            task_name="Fix bug",
            actual_output="fixed code",
            scores={"correctness": 0.8},
            task_id="t1",
            model="gpt-4",
            run_id="run-1",
        )
        assert entry.prompt == "Fix bug"
        assert entry.response == "fixed code"
        assert abs(entry.reward - 0.8) < 1e-9
        assert isinstance(entry, RLVREntry)

    def test_metadata_includes_all_fields(self) -> None:
        """Metadata dict includes task_id, model, and run_id."""
        entry = format_rlvr_entry(
            task_name="Sort array",
            actual_output="sorted",
            scores={"correctness": 0.5},
            task_id="task-42",
            model="claude-3",
            run_id="run-7",
        )
        assert entry.metadata["task_id"] == "task-42"
        assert entry.metadata["model"] == "claude-3"
        assert entry.metadata["run_id"] == "run-7"

    def test_reward_uses_compute_function(self) -> None:
        """Reward in formatted entry matches compute_rlvr_reward."""
        scores = {"functional_test": 0.9, "correctness": 0.5}
        entry = format_rlvr_entry(
            task_name="Test",
            actual_output="out",
            scores=scores,
            task_id="t1",
            model="m",
            run_id="r",
        )
        expected_reward = compute_rlvr_reward(scores)
        assert abs(entry.reward - expected_reward) < 1e-9

    def test_frozen_entry(self) -> None:
        """RLVREntry is frozen (immutable)."""
        entry = format_rlvr_entry(
            task_name="Test",
            actual_output="out",
            scores={"correctness": 0.5},
            task_id="t1",
            model="m",
            run_id="r",
        )
        try:
            entry.reward = 0.99  # type: ignore[misc]
            raise AssertionError("Should not be able to mutate frozen dataclass")
        except AttributeError:
            pass  # Expected: frozen dataclass


class TestRLVRExporter:
    def test_export_entries(self) -> None:
        """Exporter converts results to RLVR entries."""
        results = [
            _result(task_id="t1", task_name="Fix bug", actual_output="code1", scores={"correctness": 0.8}),
            _result(task_id="t2", task_name="Sort", actual_output="code2", scores={"functional_test": 0.9}),
        ]
        exporter = RLVRExporter()
        entries = exporter.export_entries(results, model="gpt-4", run_id="run-1")

        assert len(entries) == 2
        assert entries[0].prompt == "Fix bug"
        assert entries[0].response == "code1"
        assert entries[0].metadata["model"] == "gpt-4"
        assert entries[0].metadata["run_id"] == "run-1"
        assert entries[1].prompt == "Sort"

    def test_zero_results(self) -> None:
        """Empty results list returns empty entries list."""
        exporter = RLVRExporter()
        entries = exporter.export_entries([], model="gpt-4", run_id="run-1")
        assert entries == []

    def test_to_jsonl_format(self) -> None:
        """JSONL output has one JSON object per line with correct fields."""
        results = [
            _result(task_id="t1", task_name="Fix bug", actual_output="code1", scores={"correctness": 0.8}),
        ]
        exporter = RLVRExporter()
        entries = exporter.export_entries(results, model="gpt-4", run_id="run-1")
        jsonl = exporter.to_jsonl(entries)

        lines = [line for line in jsonl.strip().split("\n") if line]
        assert len(lines) == 1
        obj = json.loads(lines[0])
        assert obj["prompt"] == "Fix bug"
        assert obj["response"] == "code1"
        assert "reward" in obj
        assert obj["metadata"]["task_id"] == "t1"
        assert obj["metadata"]["model"] == "gpt-4"
        assert obj["metadata"]["run_id"] == "run-1"

    def test_to_jsonl_multiple_entries(self) -> None:
        """Multiple entries produce multiple JSONL lines."""
        results = [
            _result(task_id="t1", task_name="Task A", scores={"correctness": 0.7}),
            _result(task_id="t2", task_name="Task B", scores={"correctness": 0.9}),
            _result(task_id="t3", task_name="Task C", scores={"correctness": 0.5}),
        ]
        exporter = RLVRExporter()
        entries = exporter.export_entries(results, model="gpt-4", run_id="run-1")
        jsonl = exporter.to_jsonl(entries)

        lines = [line for line in jsonl.strip().split("\n") if line]
        assert len(lines) == 3
        for line in lines:
            obj = json.loads(line)
            assert "prompt" in obj
            assert "response" in obj
            assert "reward" in obj
            assert "metadata" in obj

    def test_to_jsonl_empty_entries(self) -> None:
        """Empty entries list produces empty string."""
        exporter = RLVRExporter()
        jsonl = exporter.to_jsonl([])
        assert jsonl == ""

    def test_multi_run_results(self) -> None:
        """Results from different tasks all get exported independently."""
        results = [
            _result(task_id="t1", task_name="A", scores={"correctness": 0.9}),
            _result(task_id="t1", task_name="A", scores={"correctness": 0.3}),
            _result(task_id="t2", task_name="B", scores={"functional_test": 0.7}),
        ]
        exporter = RLVRExporter()
        entries = exporter.export_entries(results, model="gpt-4", run_id="run-1")

        # RLVR exports every result as its own entry (unlike DPO which pairs them).
        assert len(entries) == 3
        task_ids = [e.metadata["task_id"] for e in entries]
        assert task_ids.count("t1") == 2
        assert task_ids.count("t2") == 1
