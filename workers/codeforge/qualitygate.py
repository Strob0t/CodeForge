"""Quality gate executor for running test and lint commands (Phase 4C).

Receives requests from the Go control plane via NATS, executes the specified
commands in the project workspace, and reports results back.
"""

from __future__ import annotations

import asyncio

import structlog

from codeforge.models import QualityGateRequest, QualityGateResult

logger = structlog.get_logger()

DEFAULT_TIMEOUT_SECONDS = 120


class QualityGateExecutor:
    """Executes test and lint commands and returns pass/fail results."""

    def __init__(self, timeout_seconds: int = DEFAULT_TIMEOUT_SECONDS) -> None:
        self._timeout = timeout_seconds

    async def execute(self, request: QualityGateRequest) -> QualityGateResult:
        """Run the requested quality gate checks and return the result."""
        log = logger.bind(run_id=request.run_id, project_id=request.project_id)
        log.info("quality gate execution started")

        result = QualityGateResult(run_id=request.run_id)

        if request.run_tests and request.test_command:
            passed, output = await self._run_command(
                request.test_command,
                request.workspace_path,
                log,
            )
            result.tests_passed = passed
            result.test_output = output

        if request.run_lint and request.lint_command:
            passed, output = await self._run_command(
                request.lint_command,
                request.workspace_path,
                log,
            )
            result.lint_passed = passed
            result.lint_output = output

        log.info(
            "quality gate execution completed",
            tests_passed=result.tests_passed,
            lint_passed=result.lint_passed,
        )
        return result

    async def _run_command(
        self,
        command: str,
        cwd: str,
        log: structlog.stdlib.BoundLogger,
    ) -> tuple[bool, str]:
        """Run a shell command and return (passed, output)."""
        log.debug("running gate command", command=command, cwd=cwd)
        try:
            proc = await asyncio.create_subprocess_shell(
                command,
                cwd=cwd,
                stdout=asyncio.subprocess.PIPE,
                stderr=asyncio.subprocess.STDOUT,
            )
            stdout, _ = await asyncio.wait_for(
                proc.communicate(),
                timeout=self._timeout,
            )
            output = stdout.decode(errors="replace") if stdout else ""
            passed = proc.returncode == 0
            log.info(
                "gate command finished",
                command=command,
                passed=passed,
                returncode=proc.returncode,
            )
            return passed, output
        except TimeoutError:
            log.warning("gate command timed out", command=command)
            return False, f"command timed out after {self._timeout}s"
        except Exception as exc:
            log.error("gate command error", command=command, error=str(exc))
            return False, str(exc)
