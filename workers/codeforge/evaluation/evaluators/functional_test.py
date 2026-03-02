"""Functional test evaluator — runs shell commands to verify correctness.

Executes a test command (pytest, go test, npm test, etc.) and parses the
exit code to produce a pass/fail score. The raw output is captured for
debugging and stored in the EvalDimension details.
"""

from __future__ import annotations

import asyncio

import structlog

from codeforge.evaluation.providers.base import EvalDimension, ExecutionResult, TaskSpec

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
        except Exception:
            logger.exception("functional test failed", task_id=task.id)
            return [
                EvalDimension(
                    name="functional_test",
                    score=0.0,
                    details={"error": "execution failed", "command": command},
                )
            ]

    async def _run_command(self, command: str) -> tuple[float, str, int]:
        """Execute a shell command and return (score, output, exit_code)."""
        proc = await asyncio.create_subprocess_shell(
            command,
            stdout=asyncio.subprocess.PIPE,
            stderr=asyncio.subprocess.STDOUT,
            cwd=self._working_dir,
        )
        stdout, _ = await asyncio.wait_for(proc.communicate(), timeout=self._timeout)
        output = stdout.decode("utf-8", errors="replace") if stdout else ""
        exit_code = proc.returncode or 0
        score = 1.0 if exit_code == 0 else 0.0
        return score, output, exit_code
