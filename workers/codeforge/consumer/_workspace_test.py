"""Workspace test handler mixin (KI-81, KI-152).

The auto-agent's post-verification used to run pytest in the Go Core, in the
agent-written workspace and with the core's secrets. The worker runs it here
instead, like a quality gate command: allowlisted executable, the tool
environment (no worker credentials), its own process group, a timeout. The
auto-agent's verification of a feature (KI-152) runs the project's test and
lint commands here through the quality gate executor.
"""

from __future__ import annotations

import re
from pathlib import Path
from typing import TYPE_CHECKING

import structlog

from codeforge.constants import DEFAULT_QG_TIMEOUT_SECONDS
from codeforge.consumer._subjects import SUBJECT_CONVERSATION_TEST_RESULT
from codeforge.models import QualityGateRequest, WorkspaceTestRequest, WorkspaceTestResult
from codeforge.tool_identity import ToolIsolationError, tool_tenant

if TYPE_CHECKING:
    import nats.aio.msg

logger = structlog.get_logger()

# The file names the auto-agent extracts from feature descriptions.
_TEST_FILE = re.compile(r"^test_\w+\.py$")

# The result carries the tail of the test output (the summary and the
# failures pytest prints last): the whole output can exceed the NATS max
# payload, and a failed publish re-runs the tests (S3-F review C7).
MAX_OUTPUT_BYTES = 64 * 1024


def _tail(output: str, limit: int = MAX_OUTPUT_BYTES) -> str:
    """The last limit bytes of output, behind a marker when cut."""
    data = output.encode()
    if len(data) <= limit:
        return output
    tail = data[-limit:].decode(errors="ignore")  # may cut into a character
    return f"[... {len(data) - limit} bytes of earlier test output truncated ...]\n{tail}"


def _test_file_error(workspace: str, test_file: str) -> str:
    """Why the test file must not run, "" when it may."""
    if not _TEST_FILE.fullmatch(test_file):
        return f"invalid test file name {test_file!r} (test_<name>.py in the workspace root)"
    root = Path(workspace).resolve()
    path = (root / test_file).resolve()
    if path.parent != root:
        return f"test file {test_file!r} resolves outside the workspace"
    if not path.is_file():
        return f"test file {test_file!r} not found"
    return ""


class WorkspaceTestHandlerMixin:
    """Handles conversation.test.request messages (at-least-once, deduplicated by request_id)."""

    async def _handle_workspace_test(self, msg: nats.aio.msg.Msg) -> None:
        await self._handle_request(
            msg=msg,
            request_model=WorkspaceTestRequest,
            dedup_key=lambda r: f"wstest-{r.request_id}",
            handler=self._do_workspace_test,
            result_subject=SUBJECT_CONVERSATION_TEST_RESULT,
            log_context=lambda r: {"request_id": r.request_id, "conversation_id": r.conversation_id},
        )

    async def _do_workspace_test(
        self, request: WorkspaceTestRequest, log: structlog.BoundLogger
    ) -> WorkspaceTestResult:
        if request.test_file:
            return await self._run_test_file(request, log)
        return await self._run_check_commands(request, log)

    async def _run_check_commands(
        self, request: WorkspaceTestRequest, log: structlog.BoundLogger
    ) -> WorkspaceTestResult:
        """Run the test and lint commands like quality gate checks (KI-152)."""
        result = WorkspaceTestResult(request_id=request.request_id, conversation_id=request.conversation_id)
        run_tests, run_lint = bool(request.test_command.strip()), bool(request.lint_command.strip())
        if not (run_tests or run_lint):
            result.error = "nothing to run: no test file, test command or lint command"
            log.warning("workspace check refused", reason=result.error)
            return result
        gate = QualityGateRequest(
            run_id=request.request_id,
            project_id=request.project_id,
            tenant_id=request.tenant_id,
            workspace_path=request.workspace_path,
            run_tests=run_tests,
            run_lint=run_lint,
            test_command=request.test_command,
            lint_command=request.lint_command,
            timeout_seconds=request.timeout_seconds,
            tool_output_max_chars=request.tool_output_max_chars,
        )
        try:
            # The commands run as the tenant's tool UID (KI-96).
            async with tool_tenant(request.tenant_id, request.tool_uid, request.workspace_path):
                checked = await self._gate_executor.execute(gate)
        except ToolIsolationError as exc:
            log.error("workspace check refused", error=str(exc))
            result.error = str(exc)
            return result
        result.passed, result.output = checked.tests_passed, checked.test_output
        result.lint_passed, result.lint_output = checked.lint_passed, checked.lint_output
        result.error = checked.error
        log.info("workspace check finished", tests_passed=result.passed, lint_passed=result.lint_passed)
        return result

    async def _run_test_file(self, request: WorkspaceTestRequest, log: structlog.BoundLogger) -> WorkspaceTestResult:
        """Run the test file a feature description names."""
        result = WorkspaceTestResult(request_id=request.request_id, conversation_id=request.conversation_id)
        if reason := _test_file_error(request.workspace_path, request.test_file):
            log.warning("workspace test refused", reason=reason)
            result.error = reason
            return result
        timeout = request.timeout_seconds or DEFAULT_QG_TIMEOUT_SECONDS
        command = f"python -m pytest {request.test_file} -v --tb=short"
        try:
            # The test runs as the tenant's tool UID (KI-96).
            async with tool_tenant(request.tenant_id, request.tool_uid, request.workspace_path):
                passed, output = await self._gate_executor.run_command(command, request.workspace_path, log, timeout)
        except ToolIsolationError as exc:
            log.error("workspace test refused", error=str(exc))
            result.error = str(exc)
            return result
        output = _tail(output)
        result.passed, result.output = passed, output
        if passed is None:
            result.error = output
        log.info("workspace test finished", passed=passed)
        return result
