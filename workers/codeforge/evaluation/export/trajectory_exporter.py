"""Export multi-rollout benchmark results as DPO/EntroPO training pairs.

Given benchmark results with multiple rollouts per task, this module generates
chosen/rejected trajectory pairs in JSONL format (HuggingFace-compatible) for
external model fine-tuning.

This closes the feedback loop: CodeForge evaluates → exports training data →
external training → improved model → re-evaluate.
"""

from __future__ import annotations

import json
from collections import defaultdict
from dataclasses import dataclass, field
from typing import TYPE_CHECKING

if TYPE_CHECKING:
    from codeforge.models import BenchmarkTaskResult


@dataclass(frozen=True)
class TrajectoryEntry:
    """One complete trajectory (a single rollout's execution record)."""

    rollout_id: int
    task_id: str
    task_name: str
    output: str
    scores: dict[str, float]
    cost_usd: float = 0.0
    tokens_total: int = 0
    tool_calls: list[dict[str, str]] = field(default_factory=list)
    files_changed: list[str] = field(default_factory=list)

    def avg_score(self) -> float:
        if not self.scores:
            return 0.0
        return sum(self.scores.values()) / len(self.scores)

    def to_dict(self) -> dict:
        return {
            "rollout_id": self.rollout_id,
            "task_id": self.task_id,
            "task_name": self.task_name,
            "output": self.output,
            "scores": self.scores,
            "avg_score": round(self.avg_score(), 6),
            "cost_usd": self.cost_usd,
            "tokens_total": self.tokens_total,
        }


@dataclass(frozen=True)
class TrainingPair:
    """A chosen/rejected pair suitable for DPO training."""

    task_id: str
    prompt: str
    chosen: TrajectoryEntry
    rejected: TrajectoryEntry
    score_gap: float

    def to_dict(self) -> dict:
        return {
            "task_id": self.task_id,
            "prompt": self.prompt,
            "chosen": self.chosen.to_dict(),
            "rejected": self.rejected.to_dict(),
            "score_gap": round(self.score_gap, 6),
        }


class TrajectoryExporter:
    """Exports multi-rollout benchmark results as DPO training pairs."""

    def export_pairs(self, results: list[BenchmarkTaskResult]) -> list[TrainingPair]:
        """Generate chosen/rejected pairs from multi-rollout results.

        Groups results by task_id. For each task with rollout_count >= 2:
        best rollout = chosen, each other rollout = rejected → N-1 pairs per task.
        Single-rollout tasks are skipped.
        """
        if not results:
            return []

        grouped: dict[str, list[BenchmarkTaskResult]] = defaultdict(list)
        for r in results:
            grouped[r.task_id].append(r)

        pairs: list[TrainingPair] = []

        for task_id, task_results in sorted(grouped.items()):
            if len(task_results) < 2:
                continue

            # Find the best rollout.
            best = [r for r in task_results if r.is_best_rollout]
            if not best:
                continue

            chosen_result = best[0]
            chosen_entry = _to_entry(chosen_result)

            # Create a pair for each non-best rollout.
            for r in task_results:
                if r.rollout_id == chosen_result.rollout_id:
                    continue
                rejected_entry = _to_entry(r)
                gap = chosen_entry.avg_score() - rejected_entry.avg_score()
                pairs.append(
                    TrainingPair(
                        task_id=task_id,
                        prompt=chosen_result.task_name,
                        chosen=chosen_entry,
                        rejected=rejected_entry,
                        score_gap=gap,
                    )
                )

        return pairs

    def to_jsonl(self, pairs: list[TrainingPair]) -> str:
        """Serialize pairs to JSONL (one JSON object per line)."""
        if not pairs:
            return ""
        lines = [json.dumps(p.to_dict(), ensure_ascii=False) for p in pairs]
        return "\n".join(lines) + "\n"


def _to_entry(result: BenchmarkTaskResult) -> TrajectoryEntry:
    """Convert a BenchmarkTaskResult into a TrajectoryEntry."""
    return TrajectoryEntry(
        rollout_id=result.rollout_id,
        task_id=result.task_id,
        task_name=result.task_name,
        output=result.actual_output,
        scores=dict(result.scores),
        cost_usd=result.cost_usd,
        tokens_total=result.tokens_in + result.tokens_out,
        tool_calls=list(result.tool_calls),
        files_changed=list(result.files_changed),
    )
