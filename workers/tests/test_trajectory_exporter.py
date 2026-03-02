"""Tests for trajectory export in DPO/EntroPO training format.

Tests cover: pair generation from multi-rollout results, single rollout skip,
JSONL output validity, score_gap computation, empty results, metadata propagation.
"""

from __future__ import annotations

import json

from codeforge.evaluation.export.trajectory_exporter import TrajectoryExporter
from codeforge.models import BenchmarkTaskResult


def _result(
    task_id: str = "t1",
    task_name: str = "Fix bug",
    rollout_id: int = 0,
    rollout_count: int = 1,
    is_best: bool = True,
    actual_output: str = "output",
    scores: dict[str, float] | None = None,
    cost_usd: float = 0.01,
    tokens_in: int = 100,
    tokens_out: int = 50,
) -> BenchmarkTaskResult:
    return BenchmarkTaskResult(
        task_id=task_id,
        task_name=task_name,
        rollout_id=rollout_id,
        rollout_count=rollout_count,
        is_best_rollout=is_best,
        actual_output=actual_output,
        scores=scores or {"correctness": 0.5},
        cost_usd=cost_usd,
        tokens_in=tokens_in,
        tokens_out=tokens_out,
    )


class TestExportPairs:
    def test_two_rollouts_one_pair(self) -> None:
        """2 rollouts for same task → 1 training pair (best vs worst)."""
        results = [
            _result(task_id="t1", rollout_id=0, rollout_count=2, is_best=True, scores={"correctness": 0.9}),
            _result(task_id="t1", rollout_id=1, rollout_count=2, is_best=False, scores={"correctness": 0.3}),
        ]
        exporter = TrajectoryExporter()
        pairs = exporter.export_pairs(results)

        assert len(pairs) == 1
        assert pairs[0].task_id == "t1"
        assert pairs[0].chosen.rollout_id == 0
        assert pairs[0].rejected.rollout_id == 1
        assert pairs[0].score_gap > 0

    def test_four_rollouts_three_pairs(self) -> None:
        """4 rollouts → 3 pairs (best vs each of the 3 others)."""
        results = [
            _result(task_id="t1", rollout_id=0, rollout_count=4, is_best=True, scores={"correctness": 0.9}),
            _result(task_id="t1", rollout_id=1, rollout_count=4, is_best=False, scores={"correctness": 0.7}),
            _result(task_id="t1", rollout_id=2, rollout_count=4, is_best=False, scores={"correctness": 0.5}),
            _result(task_id="t1", rollout_id=3, rollout_count=4, is_best=False, scores={"correctness": 0.2}),
        ]
        exporter = TrajectoryExporter()
        pairs = exporter.export_pairs(results)

        assert len(pairs) == 3
        for pair in pairs:
            assert pair.chosen.rollout_id == 0
            assert pair.rejected.rollout_id != 0

    def test_single_rollout_no_pairs(self) -> None:
        """Single rollout tasks are skipped (need at least 2 for a pair)."""
        results = [
            _result(task_id="t1", rollout_id=0, rollout_count=1, is_best=True),
        ]
        exporter = TrajectoryExporter()
        pairs = exporter.export_pairs(results)

        assert pairs == []

    def test_multiple_tasks_independent_pairs(self) -> None:
        """Multiple tasks each generate their own pairs independently."""
        results = [
            _result(task_id="t1", rollout_id=0, rollout_count=2, is_best=True, scores={"correctness": 0.9}),
            _result(task_id="t1", rollout_id=1, rollout_count=2, is_best=False, scores={"correctness": 0.4}),
            _result(task_id="t2", rollout_id=0, rollout_count=2, is_best=True, scores={"correctness": 0.8}),
            _result(task_id="t2", rollout_id=1, rollout_count=2, is_best=False, scores={"correctness": 0.6}),
        ]
        exporter = TrajectoryExporter()
        pairs = exporter.export_pairs(results)

        assert len(pairs) == 2
        task_ids = {p.task_id for p in pairs}
        assert task_ids == {"t1", "t2"}

    def test_empty_results(self) -> None:
        """No results → no pairs."""
        exporter = TrajectoryExporter()
        pairs = exporter.export_pairs([])
        assert pairs == []

    def test_score_gap_computation(self) -> None:
        """score_gap = chosen_avg_score - rejected_avg_score."""
        results = [
            _result(task_id="t1", rollout_id=0, rollout_count=2, is_best=True, scores={"correctness": 0.8}),
            _result(task_id="t1", rollout_id=1, rollout_count=2, is_best=False, scores={"correctness": 0.3}),
        ]
        exporter = TrajectoryExporter()
        pairs = exporter.export_pairs(results)

        assert len(pairs) == 1
        assert abs(pairs[0].score_gap - 0.5) < 1e-6

    def test_no_best_rollout_skips_task(self) -> None:
        """If no rollout is marked best, skip that task."""
        results = [
            _result(task_id="t1", rollout_id=0, rollout_count=2, is_best=False),
            _result(task_id="t1", rollout_id=1, rollout_count=2, is_best=False),
        ]
        exporter = TrajectoryExporter()
        pairs = exporter.export_pairs(results)

        assert pairs == []

    def test_mixed_single_and_multi_rollout(self) -> None:
        """Mix of single-rollout and multi-rollout tasks → only multi-rollout generates pairs."""
        results = [
            _result(task_id="t1", rollout_id=0, rollout_count=1, is_best=True),
            _result(task_id="t2", rollout_id=0, rollout_count=2, is_best=True, scores={"correctness": 0.9}),
            _result(task_id="t2", rollout_id=1, rollout_count=2, is_best=False, scores={"correctness": 0.3}),
        ]
        exporter = TrajectoryExporter()
        pairs = exporter.export_pairs(results)

        assert len(pairs) == 1
        assert pairs[0].task_id == "t2"


class TestToJsonl:
    def test_jsonl_format_valid(self) -> None:
        """Each line in JSONL output is valid JSON."""
        results = [
            _result(task_id="t1", rollout_id=0, rollout_count=2, is_best=True, scores={"correctness": 0.8}),
            _result(task_id="t1", rollout_id=1, rollout_count=2, is_best=False, scores={"correctness": 0.3}),
        ]
        exporter = TrajectoryExporter()
        pairs = exporter.export_pairs(results)
        jsonl = exporter.to_jsonl(pairs)

        lines = [line for line in jsonl.strip().split("\n") if line]
        assert len(lines) == 1
        parsed = json.loads(lines[0])
        assert parsed["task_id"] == "t1"
        assert "chosen" in parsed
        assert "rejected" in parsed
        assert "score_gap" in parsed

    def test_jsonl_multiple_pairs(self) -> None:
        """Multiple pairs → one JSON object per line."""
        results = [
            _result(task_id="t1", rollout_id=0, rollout_count=3, is_best=True, scores={"correctness": 0.9}),
            _result(task_id="t1", rollout_id=1, rollout_count=3, is_best=False, scores={"correctness": 0.5}),
            _result(task_id="t1", rollout_id=2, rollout_count=3, is_best=False, scores={"correctness": 0.2}),
        ]
        exporter = TrajectoryExporter()
        pairs = exporter.export_pairs(results)
        jsonl = exporter.to_jsonl(pairs)

        lines = [line for line in jsonl.strip().split("\n") if line]
        assert len(lines) == 2
        for line in lines:
            obj = json.loads(line)
            assert obj["task_id"] == "t1"

    def test_empty_pairs_empty_output(self) -> None:
        """No pairs → empty string."""
        exporter = TrajectoryExporter()
        jsonl = exporter.to_jsonl([])
        assert jsonl == ""

    def test_jsonl_contains_prompt_and_output(self) -> None:
        """JSONL entries contain prompt (from task_name) and actual_output."""
        results = [
            _result(
                task_id="t1",
                task_name="Fix login bug",
                rollout_id=0,
                rollout_count=2,
                is_best=True,
                actual_output="fixed code",
                scores={"correctness": 0.9},
            ),
            _result(
                task_id="t1",
                task_name="Fix login bug",
                rollout_id=1,
                rollout_count=2,
                is_best=False,
                actual_output="broken code",
                scores={"correctness": 0.2},
            ),
        ]
        exporter = TrajectoryExporter()
        pairs = exporter.export_pairs(results)
        jsonl = exporter.to_jsonl(pairs)

        obj = json.loads(jsonl.strip())
        assert obj["chosen"]["output"] == "fixed code"
        assert obj["rejected"]["output"] == "broken code"
