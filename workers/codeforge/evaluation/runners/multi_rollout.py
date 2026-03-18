"""Multi-rollout runner — runs N independent rollouts and selects the best.

Inspired by R2E-Gym/EntroPO test-time scaling: running multiple independent
agent attempts on the same task and selecting the best via hybrid verification
dramatically improves results (59.8% @16 rollouts vs 51.6% @1).
"""

from __future__ import annotations

from dataclasses import dataclass
from typing import TYPE_CHECKING, Protocol

import structlog

from codeforge.evaluation.runners._similarity import normalized_edit_distance
from codeforge.evaluation.runners.early_stopping import EarlyStopChecker

# Backward-compatible alias (used by compute_diversity and external callers).
_normalized_edit_distance = normalized_edit_distance

if TYPE_CHECKING:
    from codeforge.evaluation.hybrid_pipeline import HybridEvaluationPipeline, VerificationResult
    from codeforge.evaluation.providers.base import EvalScore, ExecutionResult, TaskSpec
    from codeforge.evaluation.runners._base import RunResult

logger = structlog.get_logger()


def _trajectory_length(result: ExecutionResult) -> int:
    """Compute trajectory length for selection strategies.

    Falls back to step_count, then actual_output length if trajectory is empty.
    """
    if result.trajectory:
        return len(result.trajectory)
    if result.step_count > 0:
        return result.step_count
    return len(result.actual_output)


class _InnerRunner(Protocol):
    """Protocol for any runner that can execute a single task.

    Matches the interface of SimpleBenchmarkRunner, AgentBenchmarkRunner,
    and ToolUseBenchmarkRunner which all expose ``run_task()``.
    """

    async def run_task(self, task: TaskSpec) -> RunResult: ...


@dataclass
class RolloutOutcome:
    """Result of a single rollout within a multi-rollout execution."""

    rollout_id: int
    result: ExecutionResult
    eval_score: EvalScore | None = None
    verification: VerificationResult | None = None
    diversity_score: float = 0.0
    is_best: bool = False


@dataclass
class MultiRolloutMetadata:
    """Metadata about the multi-rollout execution, including early stopping."""

    early_stopped: bool = False
    completed_rollouts: int = 0
    skipped_rollouts: int = 0
    total_rollouts: int = 0


class MultiRolloutRunner:
    """Wraps any benchmark runner to execute N independent rollouts per task.

    Args:
        inner_runner: Runner implementing ``run_single_task(task) -> ExecutionResult``.
        hybrid_pipeline: Optional hybrid verifier for best-of-N selection.
        rollout_count: Number of independent rollouts per task.
        strategy: Selection strategy — ``"best"`` (hybrid), ``"majority"`` (vote),
            ``"longest"`` (max trajectory), or ``"shortest"`` (min non-empty trajectory).
    """

    def __init__(
        self,
        inner_runner: _InnerRunner,
        hybrid_pipeline: HybridEvaluationPipeline | None,
        rollout_count: int = 1,
        strategy: str = "best",
    ) -> None:
        self._inner = inner_runner
        self._hybrid = hybrid_pipeline
        self._rollout_count = max(1, rollout_count)
        self._strategy = strategy

    async def run_task(self, task: TaskSpec) -> list[RolloutOutcome]:
        """Run the task N times and return all outcomes with selection markers."""
        # Phase 1: Execute N independent rollouts with early stopping.
        outcomes: list[RolloutOutcome] = []
        checker = EarlyStopChecker()
        early_stopped = False

        for i in range(self._rollout_count):
            run_result = await self._inner.run_task(task)
            outcome = RolloutOutcome(
                rollout_id=i,
                result=run_result.execution,
                eval_score=run_result.eval_score,
            )
            outcomes.append(outcome)

            # Feed the checker with completed rollout data.
            eval_avg = run_result.eval_score.average_score() if run_result.eval_score else 0.0
            checker.add_rollout(
                i,
                run_result.execution.actual_output,
                run_result.execution.exit_code,
                eval_avg,
            )

            # Check for early stop (only meaningful when rollout_count > 3,
            # since with <= 3 all rollouts run anyway).
            if self._rollout_count > 3 and checker.should_stop():
                early_stopped = True
                logger.info(
                    "early-stop triggered",
                    task_id=task.id,
                    completed=checker.completed_count,
                    total=self._rollout_count,
                    skipped=self._rollout_count - checker.completed_count,
                )
                break

        if self._rollout_count == 1:
            outcomes[0].is_best = True
            self._metadata = MultiRolloutMetadata(
                early_stopped=False,
                completed_rollouts=1,
                skipped_rollouts=0,
                total_rollouts=1,
            )
            return outcomes

        # Phase 2: Compute diversity scores.
        outputs = [o.result.actual_output for o in outcomes]
        diversity_scores = compute_diversity(outputs)
        for outcome, div_score in zip(outcomes, diversity_scores, strict=True):
            outcome.diversity_score = div_score

        # Phase 3: Select best.
        if early_stopped:
            # Use the checker's cluster-based selection.
            best_id = checker.best_from_cluster()
            for o in outcomes:
                if o.rollout_id == best_id:
                    o.is_best = True
                    break
        elif self._strategy == "best" and self._hybrid is not None:
            await self._select_best_hybrid(task, outcomes)
        elif self._strategy == "majority":
            self._select_majority(outcomes)
        elif self._strategy == "longest":
            self._select_longest(outcomes)
        elif self._strategy == "shortest":
            self._select_shortest(outcomes)
        else:
            # No pipeline or unknown strategy: first rollout is best (fallback).
            outcomes[0].is_best = True

        self._metadata = MultiRolloutMetadata(
            early_stopped=early_stopped,
            completed_rollouts=len(outcomes),
            skipped_rollouts=self._rollout_count - len(outcomes),
            total_rollouts=self._rollout_count,
        )

        return outcomes

    @property
    def last_run_metadata(self) -> MultiRolloutMetadata:
        """Return metadata from the most recent run_task() call."""
        return getattr(self, "_metadata", MultiRolloutMetadata())

    async def _select_best_hybrid(self, task: TaskSpec, outcomes: list[RolloutOutcome]) -> None:
        """Use hybrid verification to select the best rollout."""
        best_idx = -1
        best_score = -1.0

        for i, outcome in enumerate(outcomes):
            vr = await self._hybrid.verify(task, outcome.result)  # type: ignore[union-attr]
            outcome.verification = vr
            score = vr.combined_score.average_score() if vr.combined_score else -1.0
            if score > best_score:
                best_score = score
                best_idx = i

        if best_idx >= 0:
            outcomes[best_idx].is_best = True

        logger.info(
            "best-of-N selected",
            task_id=task.id,
            best_rollout=best_idx,
            best_score=best_score,
            rollout_count=len(outcomes),
        )

    def _select_majority(self, outcomes: list[RolloutOutcome]) -> None:
        """Select best via majority vote on pass/fail (exit_code == 0)."""
        passing = [o for o in outcomes if o.result.exit_code == 0]
        failing = [o for o in outcomes if o.result.exit_code != 0]

        if len(passing) >= len(failing):
            # Majority passes — pick first passing rollout as best.
            if passing:
                passing[0].is_best = True
            elif outcomes:
                outcomes[0].is_best = True
        else:
            # Majority fails — still pick first passing if any, else first.
            if passing:
                passing[0].is_best = True
            elif outcomes:
                outcomes[0].is_best = True

    def _select_longest(self, outcomes: list[RolloutOutcome]) -> None:
        """Select rollout with longest trajectory."""
        best_idx = 0
        best_len = -1
        for i, o in enumerate(outcomes):
            tl = _trajectory_length(o.result)
            if tl > best_len:
                best_len = tl
                best_idx = i
        outcomes[best_idx].is_best = True

    def _select_shortest(self, outcomes: list[RolloutOutcome]) -> None:
        """Select rollout with shortest non-empty trajectory."""
        non_empty = [(i, _trajectory_length(o.result)) for i, o in enumerate(outcomes) if o.result.trajectory]
        if not non_empty:
            outcomes[0].is_best = True
            return
        best_idx = min(non_empty, key=lambda x: x[1])[0]
        outcomes[best_idx].is_best = True


def compute_diversity(outputs: list[str]) -> list[float]:
    """Compute diversity score for each output using normalized edit distance.

    Each output's diversity score is the average normalized edit distance
    to all other outputs. Higher = more different from peers.

    Returns:
        List of diversity scores (0.0-1.0), one per output.
    """
    n = len(outputs)
    if n <= 1:
        return [0.0] * n

    scores: list[float] = []
    for i in range(n):
        total_dist = 0.0
        for j in range(n):
            if i != j:
                total_dist += _normalized_edit_distance(outputs[i], outputs[j])
        scores.append(total_dist / (n - 1))

    return scores
