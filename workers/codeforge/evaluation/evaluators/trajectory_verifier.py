"""Trajectory verifier evaluator — LLM-as-verifier for full trajectory quality.

Inspired by R2E-Gym/EntroPO: unlike LLMJudgeEvaluator (which sees only input/output),
this evaluator sends the complete agent trajectory (all messages, tool calls, diffs,
test output) to a dedicated verifier model for holistic quality scoring.

Produces 5 quality dimensions:
  - solution_quality:       Does the output correctly solve the task?
  - approach_efficiency:    Did the agent take a reasonable path?
  - code_quality:           Is the generated code clean and idiomatic?
  - error_recovery:         Did the agent handle errors well?
  - completeness:           Are all aspects of the task addressed?
"""

from __future__ import annotations

import json
from typing import Any

import structlog

from codeforge.evaluation.evaluators.prompt_compressor import compress_for_context
from codeforge.evaluation.providers.base import EvalDimension, ExecutionResult, TaskSpec

logger = structlog.get_logger()

_SCORE_DIMENSIONS = (
    "solution_quality",
    "approach_efficiency",
    "code_quality",
    "error_recovery",
    "completeness",
)

_VERIFIER_PROMPT = """\
You are an expert code reviewer evaluating an AI coding agent's work.

## Task Description
{task_input}

## Expected Behavior
{expected_output}

## Agent Trajectory
{trajectory}

## Files Changed
{files_changed}

## Test Results
{test_output}

Rate the agent's performance on each dimension using EXACTLY one of: ACHIEVED, PARTIALLY_ACHIEVED, or NOT_ACHIEVED.

Dimension definitions:
1. **solution_quality**: Does the output correctly solve the task?
   - ACHIEVED: Task fully solved, correct output
   - PARTIALLY_ACHIEVED: Partially correct, missing edge cases or minor errors
   - NOT_ACHIEVED: Wrong approach, broken output, or task not addressed

2. **approach_efficiency**: Did the agent take a reasonable path?
   - ACHIEVED: Direct path, no unnecessary steps
   - PARTIALLY_ACHIEVED: Some redundant steps but reached a solution
   - NOT_ACHIEVED: Excessive flailing, loops, or dead-end exploration

3. **code_quality**: Is the generated code clean and idiomatic?
   - ACHIEVED: Clean, readable, follows conventions
   - PARTIALLY_ACHIEVED: Functional but messy or inconsistent style
   - NOT_ACHIEVED: Broken, unreadable, or violates basic conventions

4. **error_recovery**: Did the agent handle errors well?
   - ACHIEVED: Recognized and recovered from errors effectively
   - PARTIALLY_ACHIEVED: Partial recovery or slow adaptation
   - NOT_ACHIEVED: Repeated same errors or ignored them

5. **completeness**: Are all aspects of the task addressed?
   - ACHIEVED: All requirements met
   - PARTIALLY_ACHIEVED: Core requirements met, secondary ones missed
   - NOT_ACHIEVED: Major requirements missing

Respond ONLY with a JSON object:
{{"solution_quality": "ACHIEVED", "approach_efficiency": "PARTIALLY_ACHIEVED", "code_quality": "ACHIEVED", "error_recovery": "NOT_ACHIEVED", "completeness": "ACHIEVED"}}"""

# Max chars per individual message in the formatted trajectory.
_MAX_MSG_CHARS = 500

# Max character budgets for prompt compression (prevents context overflow on local models).
_MAX_TRAJECTORY_CHARS = 4000
_MAX_TASK_INPUT_CHARS = 2000

_CATEGORY_SCORES: dict[str, float] = {
    "ACHIEVED": 1.0,
    "PARTIALLY_ACHIEVED": 0.5,
    "NOT_ACHIEVED": 0.0,
}


class TrajectoryVerifierEvaluator:
    """Stage 2 (rank) evaluator that scores full agent trajectories."""

    def __init__(
        self,
        model: str = "openai/gpt-4o",
        max_trajectory_tokens: int = 8000,
    ) -> None:
        self._model = model
        self._max_trajectory_tokens = max_trajectory_tokens

    @property
    def name(self) -> str:
        return "trajectory_verifier"

    @property
    def stage(self) -> str:
        return "rank"

    async def evaluate(self, task: TaskSpec, result: ExecutionResult) -> list[EvalDimension]:
        """Score the full trajectory on 5 quality dimensions."""
        trajectory_text = _format_trajectory(task, result)
        compressed_trajectory = compress_for_context(trajectory_text, _MAX_TRAJECTORY_CHARS)
        compressed_task_input = compress_for_context(task.input, _MAX_TASK_INPUT_CHARS)
        compressed_expected = compress_for_context(task.expected_output, _MAX_TASK_INPUT_CHARS // 2) or "N/A"
        prompt = _VERIFIER_PROMPT.format(
            task_input=compressed_task_input,
            expected_output=compressed_expected,
            trajectory=compressed_trajectory,
            files_changed="\n".join(result.files_changed) or "None",
            test_output=result.test_output[:2000] or "N/A",
        )

        try:
            response = await self._call_verifier(prompt)
            content = response.choices[0].message.content
            scores = _parse_scores(content)
        except Exception as exc:
            logger.exception("trajectory verifier failed", task_id=task.id, error=str(exc))
            return [
                EvalDimension(
                    name="trajectory_quality",
                    score=0.0,
                    details={"error": "verifier call failed"},
                )
            ]

        return [
            EvalDimension(
                name=f"trajectory_{dim}",
                score=scores.get(dim, 0.0),
            )
            for dim in _SCORE_DIMENSIONS
        ]

    async def _call_verifier(self, prompt: str) -> object:
        """Call the verifier LLM. Isolated for testability (mock target)."""
        import litellm

        return await litellm.acompletion(
            model=self._model,
            messages=[
                {"role": "system", "content": "You are a precise evaluation model. Return only JSON."},
                {"role": "user", "content": prompt},
            ],
            temperature=0.0,
            max_tokens=128,
        )


def _format_trajectory(task: TaskSpec, result: ExecutionResult) -> str:
    """Format trajectory messages into human-readable text."""
    if not result.trajectory:
        return ""

    lines: list[str] = []
    for entry in result.trajectory:
        role = entry.role
        content = entry.content[:_MAX_MSG_CHARS] if entry.content else ""

        if role == "user":
            lines.append(f"[USER] {content}")
        elif role == "assistant":
            if entry.tool_name:
                lines.append(f"[TOOL CALL] {entry.tool_name}({entry.tool_args[:200]})")
            elif content:
                lines.append(f"[ASSISTANT] {content}")
        elif role == "tool":
            name = entry.tool_name or "unknown"
            lines.append(f"[TOOL RESULT: {name}] {content[:300]}")
        else:
            lines.append(f"[{role.upper()}] {content}")

    return "\n".join(lines)


def _parse_scores(content: str) -> dict[str, float]:
    """Parse categorical JSON scores from LLM response. Returns empty dict on failure."""
    try:
        text = content.strip()
        if text.startswith("```"):
            text = text.split("\n", 1)[-1].rsplit("```", 1)[0].strip()

        raw: dict[str, Any] = json.loads(text)
        result: dict[str, float] = {}
        for k, v in raw.items():
            if k in _SCORE_DIMENSIONS:
                if isinstance(v, str):
                    result[k] = _CATEGORY_SCORES.get(v.upper(), 0.0)
                else:
                    # Backward compat: if someone passes a float, clamp it
                    result[k] = max(0.0, min(1.0, float(v)))
        return result
    except (json.JSONDecodeError, TypeError, ValueError) as exc:
        logger.warning("trajectory verifier parse failed", error=str(exc), content=content[:200])
        raise
