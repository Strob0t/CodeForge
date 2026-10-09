"""Functional test evaluator — runs shell commands to verify correctness.

Executes a test command (pytest, go test, npm test, etc.) and parses the
exit code to produce a pass/fail score. The raw output is captured for
debugging and stored in the EvalDimension details.
"""

from __future__ import annotations

import structlog

from codeforge.constants import MAX_OUTPUT_CHARS
from codeforge.evaluation.providers.base import EvalDimension, ExecutionResult, TaskSpec
from codeforge.subprocess_utils import run_tool_shell

logger = structlog.get_logger()

# Default timeout for test commands (seconds).
DEFAULT_TIMEOUT = 120


class FunctionalTestEvaluator:
    """Evaluator that runs a shell test command and scores by exit code."""

    stage = "filter"

    def __init__(
        self,
        timeout: int = DEFAULT_TIMEOUT,
        working_dir: str | None = None,
    ) -> None:
        self._timeout = timeout
        self._working_dir = working_dir

    @property
    def name(self) -> str:
        return "functional_test"

    async def evaluate(self, task: TaskSpec, result: ExecutionResult) -> list[EvalDimension]:
        """Run the task's test_command and score based on exit code."""
        command = task.test_command
        if not command:
            return [
                EvalDimension(
                    name="functional_test",
                    score=0.0,
                    details={"error": "no test_command specified"},
                )
            ]

        try:
            score, output, exit_code = await self._run_command(command)
            return [
                EvalDimension(
                    name="functional_test",
                    score=score,
                    details={
                        "exit_code": str(exit_code),
                        "output": output[:2000],
                        "command": command,
                    },
                )
            ]
        except TimeoutError:
            logger.warning("functional test timed out", task_id=task.id, timeout=self._timeout)
            return [
                EvalDimension(
                    name="functional_test",
                    score=0.0,
                    details={"error": f"timeout after {self._timeout}s", "command": command},
                )
            ]
        except Exception as exc:
            logger.exception("functional test failed", task_id=task.id, error=str(exc))
            return [
                EvalDimension(
                    name="functional_test",
                    score=0.0,
                    details={"error": "execution failed", "command": command},
                )
            ]

    async def _run_command(self, command: str) -> tuple[float, str, int]:
        """Execute a shell command and return (score, output, exit_code).

        A command that does not end in time raises TimeoutError; it and
        everything it started are killed, and its output is capped (KI-194).
        """
        exit_code, output = await run_tool_shell(
            command, cwd=self._working_dir, timeout=self._timeout, max_output=MAX_OUTPUT_CHARS
        )
        score = 1.0 if exit_code == 0 else 0.0
        return score, output, exit_code
