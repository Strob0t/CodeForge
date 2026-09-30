"""Quality gate executor for running test and lint commands (Phase 4C).

Receives requests from the Go control plane via NATS, executes the specified
commands in the project workspace, and reports results back.
"""

from __future__ import annotations

import asyncio
import contextlib
import os
import shlex
import signal

import structlog

from codeforge.constants import DEFAULT_QG_TIMEOUT_SECONDS
from codeforge.models import QualityGateRequest, QualityGateResult
from codeforge.subprocess_env import tool_env

logger = structlog.get_logger()

DEFAULT_TIMEOUT_SECONDS = DEFAULT_QG_TIMEOUT_SECONDS

# Allowlist of approved base commands for quality gate execution.
# Only the first token (executable name) of each command is checked.
_ALLOWED_COMMANDS: frozenset[str] = frozenset(
    {
        "pytest",
        "python",
        "ruff",
        "mypy",
        "pylint",
        "flake8",
        "black",
        "isort",
        "go",
        "golangci-lint",
        "npm",
        "npx",
        "yarn",
        "pnpm",
        "eslint",
        "prettier",
        "tsc",
        "cargo",
        "make",
        "pre-commit",
    }
)


def _split_command(command: str) -> list[str] | None:
    """Split a command like a POSIX shell; None when it cannot be split."""
    try:
        return shlex.split(command)
    except ValueError:
        return None


def _is_command_allowed(command: str) -> bool:
    """Return True if the command splits and its executable is on the allowlist."""
    parts = _split_command(command)
    if not parts:
        return False
    return parts[0] in _ALLOWED_COMMANDS


class QualityGateExecutor:
    """Executes test and lint commands and returns pass/fail results.

    A check that ran reports whether it passed (the command's exit code). A
    check that could not run or did not finish (no command, a command that
    is invalid, not allowed or cannot start, a timeout) has no verdict: its
    ``*_passed`` stays None and the reason goes to ``error``, which fails
    the gate without counting as a failed check - the Go Core then neither
    rolls the workspace back nor counts it against the agent.
    """

    def __init__(self, timeout_seconds: int = DEFAULT_TIMEOUT_SECONDS) -> None:
        self._timeout = timeout_seconds

    async def execute(self, request: QualityGateRequest) -> QualityGateResult:
        """Run the requested quality gate checks and return the result."""
        log = logger.bind(run_id=request.run_id, project_id=request.project_id)
        log.info("quality gate execution started")

        result = QualityGateResult(run_id=request.run_id)
        timeout = request.timeout_seconds or self._timeout
        errors: list[str] = []

        # A requested check is never skipped: without a command it fails the
        # gate instead of passing it (KI-29).
        if request.run_tests:
            result.tests_passed, result.test_output = await self._run_check(
                "test", request.test_command, request.workspace_path, log, timeout
            )
            if result.tests_passed is None:
                errors.append(f"test check could not run: {result.test_output}")

        if request.run_lint:
            result.lint_passed, result.lint_output = await self._run_check(
                "lint", request.lint_command, request.workspace_path, log, timeout
            )
            if result.lint_passed is None:
                errors.append(f"lint check could not run: {result.lint_output}")

        result.error = "; ".join(errors)
        log.info(
            "quality gate execution completed",
            tests_passed=result.tests_passed,
            lint_passed=result.lint_passed,
            error=result.error,
        )
        return result

    async def _run_check(
        self,
        check: str,
        command: str,
        cwd: str,
        log: structlog.stdlib.BoundLogger,
        timeout_seconds: int,
    ) -> tuple[bool | None, str]:
        """Run one requested check; a check without a command has no verdict."""
        if not command.strip():
            log.warning("quality gate check has no command", check=check)
            return None, f"no command for the {check} check"
        return await self._run_command(command, cwd, log, timeout_seconds)

    async def run_command(
        self,
        command: str,
        cwd: str,
        log: structlog.stdlib.BoundLogger,
        timeout_seconds: int | None = None,
    ) -> tuple[bool | None, str]:
        """Run an allowlisted command like a gate check (the auto-agent's workspace tests)."""
        return await self._run_command(command, cwd, log, timeout_seconds)

    async def _run_command(
        self,
        command: str,
        cwd: str,
        log: structlog.stdlib.BoundLogger,
        timeout_seconds: int | None = None,
    ) -> tuple[bool | None, str]:
        """Run a command and return (passed, output); passed is None without a verdict.

        The command runs in a process group of its own (start_new_session):
        on timeout, and whenever the gate stops waiting for it, the whole group
        is killed, so neither the command nor anything it started keeps running
        or keeps its output pipe open (KI-28).
        """
        timeout = timeout_seconds or self._timeout
        argv = _split_command(command)
        if argv is None:
            log.warning("quality gate command rejected: invalid", command=command)
            return None, f"invalid command: {command!r}"
        if not _is_command_allowed(command):
            log.warning("quality gate command rejected: not on allowlist", command=command)
            return None, f"command not allowed: {command!r}. Only approved commands may run."
        log.debug("running gate command", command=command, cwd=cwd, timeout_seconds=timeout)
        try:
            proc = await asyncio.create_subprocess_exec(
                *argv,
                cwd=cwd,
                stdout=asyncio.subprocess.PIPE,
                stderr=asyncio.subprocess.STDOUT,
                env=tool_env(),
                start_new_session=True,
            )
        except Exception as exc:
            log.error("gate command error", command=command, error=str(exc))
            return None, f"command could not start: {exc}"

        try:
            stdout, _ = await asyncio.wait_for(proc.communicate(), timeout=timeout)
        except TimeoutError:
            log.warning("gate command timed out, killing its process group", command=command, timeout_seconds=timeout)
            await _kill_process_group(proc, log)
            return None, f"command timed out after {timeout}s"
        except Exception as exc:
            log.error("gate command error", command=command, error=str(exc))
            await _kill_process_group(proc, log)
            return None, str(exc)
        finally:
            # Cancelled (worker shutdown) or failed while waiting: leave nothing behind.
            if proc.returncode is None:
                _signal_process_group(proc)

        output = stdout.decode(errors="replace") if stdout else ""
        passed = proc.returncode == 0
        log.info("gate command finished", command=command, passed=passed, returncode=proc.returncode)
        return passed, output


# How long a killed gate command may take to release its output pipe.
_REAP_TIMEOUT_SECONDS = 5


def _signal_process_group(proc: asyncio.subprocess.Process) -> None:
    """Send SIGKILL to the command's process group (its pid: start_new_session)."""
    with contextlib.suppress(ProcessLookupError):  # the whole group already exited
        os.killpg(proc.pid, signal.SIGKILL)


async def _kill_process_group(proc: asyncio.subprocess.Process, log: structlog.stdlib.BoundLogger) -> None:
    """Kill the command's process group and reap the command."""
    _signal_process_group(proc)
    try:
        await asyncio.wait_for(proc.wait(), timeout=_REAP_TIMEOUT_SECONDS)
    except TimeoutError:
        # A process that left the group (setsid) still holds the output pipe.
        log.warning("killed gate command still holds its output open", pid=proc.pid)
