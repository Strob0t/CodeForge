"""Logprob verifier evaluator — calibrated ranking via P(YES) logprobs.

Uses a single-token YES/NO classification with logprob extraction to produce
a mathematically grounded confidence score for Best-of-N selection.
Cheapest possible verifier: max_tokens=1.
"""

from __future__ import annotations

import math
import re
from typing import TYPE_CHECKING

from codeforge.evaluation.evaluators.base import EvaluatorError
from codeforge.evaluation.evaluators.prompt_compressor import compress_for_context
from codeforge.evaluation.evaluators.trajectory_verifier import VerifierClient, _format_trajectory
from codeforge.evaluation.providers.base import EvalDimension, ExecutionResult, TaskSpec

if TYPE_CHECKING:
    from codeforge.llm import ChatCompletionResponse, LiteLLMClient, TokenLogprob

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
# Words of a text answer (letters only, so punctuation and markup split them).
_WORD = re.compile(r"[A-Za-z]+")


class LogprobVerifierEvaluator:
    """Stage 2 (rank) evaluator using logprob P(YES) as calibrated confidence.

    The verifier model is called through the LiteLLM proxy with the worker's
    client (*llm*; by default one for the configured proxy).
    """

    def __init__(
        self,
        model: str = "openai/gpt-4o",
        max_trajectory_tokens: int = 8000,
        llm: LiteLLMClient | None = None,
    ) -> None:
        self._model = model
        self._max_trajectory_tokens = max_trajectory_tokens
        self._client = VerifierClient(llm)

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
        except Exception as exc:
            msg = f"logprob verifier failed: {exc}"
            raise EvaluatorError(msg) from exc
        score, details = _extract_score(response)

        return [EvalDimension(name="logprob_verification", score=score, details=details)]

    async def aclose(self) -> None:
        """Close the client this evaluator created (an injected one stays open)."""
        await self._client.aclose()

    async def _call_verifier(self, prompt: str) -> ChatCompletionResponse:
        """Call the verifier model through the LiteLLM proxy."""
        return await self._client.get().chat_completion(
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


def _extract_score(response: ChatCompletionResponse) -> tuple[float, dict[str, str]]:
    """Extract P(YES) from logprobs, falling back to text parsing.

    An answer that is neither (no YES/NO logprob, no YES/NO text, e.g. an
    empty one) is an evaluation error, not a 0.5 score.
    """
    if response.top_logprobs:
        top = response.top_logprobs
        yes_lp = _find_token_logprob(top, _YES_TOKENS)
        no_lp = _find_token_logprob(top, _NO_TOKENS)

        if yes_lp is not None and no_lp is not None:
            p_yes = math.exp(yes_lp) / (math.exp(yes_lp) + math.exp(no_lp))
            return p_yes, {"method": "logprob"}

        if yes_lp is not None:
            return math.exp(yes_lp), {"method": "logprob_partial"}

        if no_lp is not None:
            return 1.0 - math.exp(no_lp), {"method": "logprob_partial"}

    answer = _text_answer(response.content)
    if answer is not None:
        return (1.0 if answer else 0.0), {"method": "text_fallback"}
    msg = f"logprob verifier gave no usable answer: {response.content[:80]!r}"
    raise EvaluatorError(msg)


def _text_answer(content: str) -> bool | None:
    """The leading yes/no of an answer (case-insensitive, punctuation ignored), or None.

    "Yes." and "No, because ..." count; the one-letter forms only as the whole
    answer ("Y", "n."), so "N/A" does not read as NO. Anything else is None.
    """
    words = _WORD.findall(content)
    if not words:
        return None
    first = words[0].lower()
    if first in ("yes", "no") or (first in ("y", "n") and len(words) == 1):
        return first.startswith("y")
    return None


def _find_token_logprob(top_logprobs: list[TokenLogprob], token_set: set[str]) -> float | None:
    """Find the logprob for any token matching the given set."""
    for entry in top_logprobs:
        if entry.token.strip() in token_set:
            return entry.logprob
    return None
