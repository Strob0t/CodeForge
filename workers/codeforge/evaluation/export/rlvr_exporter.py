"""Export benchmark results as RLVR (Reinforcement Learning from Verifiable Rewards) training data.

RLVR format: each benchmark result becomes a single entry with prompt, response,
computed reward, and metadata. Unlike DPO (chosen/rejected pairs), RLVR exports
every result independently with a scalar reward signal.

Output format (JSONL):
    {"prompt": "...", "response": "...", "reward": 0.85, "metadata": {"task_id": "...", "model": "...", "run_id": "..."}}
"""

from __future__ import annotations

import json
from dataclasses import dataclass
from typing import TYPE_CHECKING

if TYPE_CHECKING:
    from codeforge.models import BenchmarkTaskResult

# functional_test scores receive higher weight since they represent
# verifiable correctness (the core signal for RLVR training).
_FUNCTIONAL_TEST_WEIGHT = 2.0
_DEFAULT_WEIGHT = 1.0


def compute_rlvr_reward(scores: dict[str, float]) -> float:
    """Compute RLVR reward from evaluation scores.

    Strategy: weighted average where functional_test scores get higher weight.
    - functional_test: weight 2.0
    - all others: weight 1.0
    - Clamp result to [0.0, 1.0]
    """
    if not scores:
        return 0.0

    total_weighted = 0.0
    total_weight = 0.0

    for key, value in scores.items():
        weight = _FUNCTIONAL_TEST_WEIGHT if key == "functional_test" else _DEFAULT_WEIGHT
        total_weighted += value * weight
        total_weight += weight

    if total_weight == 0.0:
        return 0.0

    avg = total_weighted / total_weight
    return max(0.0, min(1.0, avg))


@dataclass(frozen=True)
class RLVREntry:
    """One RLVR training entry: prompt + response + scalar reward + metadata."""

    prompt: str
    response: str
    reward: float
    metadata: dict[str, str]

    def to_dict(self) -> dict[str, str | float | dict[str, str]]:
        return {
            "prompt": self.prompt,
            "response": self.response,
            "reward": round(self.reward, 6),
            "metadata": self.metadata,
        }


def format_rlvr_entry(
    task_name: str,
    actual_output: str,
    scores: dict[str, float],
    task_id: str,
    model: str,
    run_id: str,
) -> RLVREntry:
    """Format a single benchmark result as an RLVR training entry."""
    return RLVREntry(
        prompt=task_name,
        response=actual_output,
        reward=compute_rlvr_reward(scores),
        metadata={
            "task_id": task_id,
            "model": model,
            "run_id": run_id,
        },
    )


class RLVRExporter:
    """Exports benchmark results as RLVR training entries."""

    def export_entries(self, results: list[BenchmarkTaskResult], model: str, run_id: str) -> list[RLVREntry]:
        """Convert benchmark results to RLVR entries (one per result)."""
        if not results:
            return []

        return [
            format_rlvr_entry(
                task_name=r.task_name,
                actual_output=r.actual_output,
                scores=dict(r.scores),
                task_id=r.task_id,
                model=model,
                run_id=run_id,
            )
            for r in results
        ]

    def to_jsonl(self, entries: list[RLVREntry]) -> str:
        """Serialize entries to JSONL (one JSON object per line)."""
        if not entries:
            return ""
        lines = [json.dumps(e.to_dict(), ensure_ascii=False) for e in entries]
        return "\n".join(lines) + "\n"
