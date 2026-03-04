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

Rate the agent's performance on these dimensions (0.0 to 1.0):

1. **solution_quality**: Does the final output correctly solve the task?
2. **approach_efficiency**: Did the agent take a reasonable path? (no unnecessary steps, no loops)
3. **code_quality**: Is the generated/modified code clean, idiomatic, and well-structured?
4. **error_recovery**: Did the agent handle errors well? (retry, adapt, not repeat same mistakes)
5. **completeness**: Are all aspects of the task addressed?

Respond ONLY with a JSON object, no other text:
{{"solution_quality": 0.0, "approach_efficiency": 0.0, "code_quality": 0.0, "error_recovery": 0.0, "completeness": 0.0}}"""

# Max chars per individual message in the formatted trajectory.
_MAX_MSG_CHARS = 500


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
        prompt = _VERIFIER_PROMPT.format(
            task_input=task.input[:2000],
            expected_output=task.expected_output[:1000] or "N/A",
            trajectory=trajectory_text,
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

    async def _call_verifier(self, prompt: str) -> Any:
        """Call the verifier LLM. Isolated for testability (mock target)."""
        import litellm

        return await litellm.acompletion(
            model=self._model,
            messages=[
                {"role": "system", "content": "You are a precise evaluation model. Return only JSON."},
                {"role": "user", "content": prompt},
            ],
            temperature=0.0,
            max_tokens=256,
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
    """Parse JSON scores from LLM response. Returns empty dict on failure."""
    try:
        # Strip potential markdown fences.
        text = content.strip()
        if text.startswith("```"):
            text = text.split("\n", 1)[-1].rsplit("```", 1)[0].strip()

        raw: dict[str, Any] = json.loads(text)
        return {k: max(0.0, min(1.0, float(v))) for k, v in raw.items() if k in _SCORE_DIMENSIONS}
    except (json.JSONDecodeError, TypeError, ValueError) as exc:
        logger.warning("trajectory verifier parse failed", error=str(exc), content=content[:200])
        raise
