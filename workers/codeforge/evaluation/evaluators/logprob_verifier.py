"""Logprob verifier evaluator — calibrated ranking via P(YES) logprobs.

Uses a single-token YES/NO classification with logprob extraction to produce
a mathematically grounded confidence score for Best-of-N selection.
Cheapest possible verifier: max_tokens=1.
"""

from __future__ import annotations

import math

import structlog

from codeforge.evaluation.evaluators.prompt_compressor import compress_for_context
from codeforge.evaluation.evaluators.trajectory_verifier import _format_trajectory
from codeforge.evaluation.providers.base import EvalDimension, ExecutionResult, TaskSpec

logger = structlog.get_logger()

_MAX_TRAJECTORY_CHARS = 8000
_MAX_TASK_INPUT_CHARS = 2000

_VERIFIER_PROMPT = """\
You are evaluating an AI coding agent's work.

## Task
{task_input}

## Agent Trajectory
{trajectory}

## Files Changed
{files_changed}

Did the assistant successfully resolve the task? Answer with a single word: YES or NO."""

# Token variants considered for YES/NO matching.
_YES_TOKENS = {"YES", "yes", "Yes"}
_NO_TOKENS = {"NO", "no", "No"}


class LogprobVerifierEvaluator:
    """Stage 2 (rank) evaluator using logprob P(YES) as calibrated confidence."""

    def __init__(
        self,
        model: str = "openai/gpt-4o",
        max_trajectory_tokens: int = 8000,
    ) -> None:
        self._model = model
        self._max_trajectory_tokens = max_trajectory_tokens

    @property
    def name(self) -> str:
        return "logprob_verifier"

    @property
    def stage(self) -> str:
        return "rank"

    async def evaluate(self, task: TaskSpec, result: ExecutionResult) -> list[EvalDimension]:
        """Produce a single logprob_verification dimension."""
        trajectory_text = _format_trajectory(task, result)
        compressed_trajectory = compress_for_context(trajectory_text, _MAX_TRAJECTORY_CHARS)
        compressed_task_input = compress_for_context(task.input, _MAX_TASK_INPUT_CHARS)
        prompt = _VERIFIER_PROMPT.format(
            task_input=compressed_task_input,
            trajectory=compressed_trajectory or "N/A",
            files_changed="\n".join(result.files_changed) or "None",
        )

        try:
            response = await self._call_verifier(prompt)
            score, details = _extract_score(response)
        except Exception as exc:
            logger.exception("logprob verifier failed", task_id=task.id, error=str(exc))
            return [
                EvalDimension(
                    name="logprob_verification",
                    score=0.0,
                    details={"error": str(exc)},
                )
            ]

        return [EvalDimension(name="logprob_verification", score=score, details=details)]

    async def _call_verifier(self, prompt: str) -> object:
        """Call the verifier LLM. Isolated for testability."""
        import litellm

        return await litellm.acompletion(
            model=self._model,
            messages=[
                {"role": "system", "content": "Answer YES or NO only."},
                {"role": "user", "content": prompt},
            ],
            temperature=0.0,
            max_tokens=1,
            logprobs=True,
            top_logprobs=20,
        )


def _extract_score(response: object) -> tuple[float, dict[str, str]]:
    """Extract P(YES) from logprobs, falling back to text parsing."""
    logprobs = getattr(response.choices[0], "logprobs", None)
    if logprobs is not None:
        content = getattr(logprobs, "content", None)
        if content and len(content) > 0:
            top = content[0].top_logprobs
            yes_lp = _find_token_logprob(top, _YES_TOKENS)
            no_lp = _find_token_logprob(top, _NO_TOKENS)

            if yes_lp is not None and no_lp is not None:
                p_yes = math.exp(yes_lp) / (math.exp(yes_lp) + math.exp(no_lp))
                return p_yes, {"method": "logprob"}

            if yes_lp is not None:
                return math.exp(yes_lp), {"method": "logprob_partial"}

            if no_lp is not None:
                return 1.0 - math.exp(no_lp), {"method": "logprob_partial"}

    # Text fallback
    text = response.choices[0].message.content.strip().upper()
    if text in ("YES", "Y"):
        return 1.0, {"method": "text_fallback"}
    if text in ("NO", "N"):
        return 0.0, {"method": "text_fallback"}
    return 0.5, {"method": "text_fallback"}


def _find_token_logprob(top_logprobs: list, token_set: set[str]) -> float | None:
    """Find the logprob for any token matching the given set."""
    for entry in top_logprobs:
        token = entry.token if hasattr(entry, "token") else str(entry)
        if token in token_set:
            return entry.logprob
    return None
