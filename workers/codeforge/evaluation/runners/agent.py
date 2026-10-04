"""Agent benchmark runner — full multi-turn agent loop with tools and workspace.

Dispatches tasks through AgentLoopExecutor, captures workspace diffs,
runs test commands, and feeds results into the evaluation pipeline.
"""

from __future__ import annotations

import asyncio
import contextlib
import tempfile
import time
from pathlib import Path
from typing import TYPE_CHECKING

import structlog

from codeforge.constants import MAX_WORKSPACE_FILE_BYTES
from codeforge.evaluation.providers.base import ExecutionResult, TaskSpec, ToolCall
from codeforge.evaluation.runners._base import BaseBenchmarkRunner, RunResult
from codeforge.subprocess_env import tool_env
from codeforge.tool_process import start_tool_shell, tool_workspace
from codeforge.workspace_fs import WorkspaceRoot

if TYPE_CHECKING:
    from collections.abc import Callable

    from codeforge.agent_loop import AgentLoopExecutor, LoopConfig
    from codeforge.evaluation.pipeline import EvaluationPipeline

logger = structlog.get_logger(__name__)


def _snapshot_files(workspace: Path) -> dict[str, str]:
    """Capture file contents in workspace for diff comparison.

    The agent wrote the workspace: files are read through the workspace helper
    (KI-95), so symlinks that leave it, FIFOs and files over the size cap are
    left out, and hidden entries and symlinked directories are not walked.
    """
    snapshot: dict[str, str] = {}
    try:
        root = WorkspaceRoot(str(workspace))
    except OSError:
        return snapshot
    with root:
        for dirpath, dirnames, filenames, dir_fd in root.walk():
            dirnames[:] = [d for d in dirnames if not d.startswith(".")]
            for name in filenames:
                if name.startswith("."):
                    continue
                rel = name if dirpath == "." else f"{dirpath}/{name}"
                with contextlib.suppress(OSError):
                    data = root.read_entry(dir_fd, name, rel, max_bytes=MAX_WORKSPACE_FILE_BYTES)
                    snapshot[rel] = data.decode("utf-8", errors="replace")
    return snapshot


def _compute_files_changed(before: dict[str, str], after: dict[str, str]) -> list[str]:
    """Compute list of files that were added, modified, or deleted."""
    changed: list[str] = []
    all_keys = set(before) | set(after)
    for key in sorted(all_keys):
        if key not in before:
            changed.append(key)  # added
        elif key not in after:
            changed.append(key)  # deleted
        elif before[key] != after[key]:
            changed.append(key)  # modified
    return changed


def _setup_workspace(task: TaskSpec, base_dir: str | None = None) -> Path:
    """Create a temporary workspace and write initial files from task spec (no tool identity)."""
    workspace = Path(tempfile.mkdtemp(prefix="bench_agent_", dir=base_dir))
    _write_initial_files(task, workspace)
    return workspace


def _write_initial_files(task: TaskSpec, workspace: Path) -> None:
    # Task files stay inside the workspace (a dataset path with ".." or an
    # absolute path is refused).
    with WorkspaceRoot(str(workspace)) as root:
        for rel_path, content in task.initial_files.items():
            root.write_text(rel_path, content, make_parents=True)


async def _run_test_command(test_command: str, workspace: Path, timeout: int = 60) -> tuple[str, int]:
    """Run a test command in the workspace and return (output, exit_code)."""
    try:
        proc = await start_tool_shell(
            test_command,
            stdout=asyncio.subprocess.PIPE,
            stderr=asyncio.subprocess.STDOUT,
            cwd=str(workspace),
            env=tool_env(),
        )
        stdout, _ = await asyncio.wait_for(proc.communicate(), timeout=timeout)
        output = stdout.decode("utf-8", errors="replace") if stdout else ""
        return output, proc.returncode or 0
    except TimeoutError:
        return f"Test command timed out after {timeout}s", 124
    except OSError as exc:
        return f"Test command failed: {exc}", 1


def _prepare_test_files(task: TaskSpec, workspace: Path, solution: str) -> None:
    """Write test harness and patch files to workspace before running tests.

    - HumanEval/MBPP: metadata["test_harness"] with {SOLUTION} placeholder → solution.py
    - SWE-bench: metadata["test_patch"] → test_patch.diff

    The agent ran in the workspace first: the files are put in place as new
    files (replace_bytes), whatever the agent left at those names (a symlink
    out of the workspace, a FIFO), so nothing is written through them (KI-95).
    """
    with WorkspaceRoot(str(workspace)) as root:
        test_harness = task.metadata.get("test_harness", "")
        if test_harness and "{SOLUTION}" in test_harness:
            harness_content = test_harness.replace("{SOLUTION}", solution)
            root.replace_bytes("solution.py", harness_content.encode())

        test_patch = task.metadata.get("test_patch", "")
        if test_patch:
            root.replace_bytes("test_patch.diff", test_patch.encode())


class AgentBenchmarkRunner(BaseBenchmarkRunner):
    """Runs agent benchmarks: full multi-turn agent loop with workspace.

    For each task:
    1. Create workspace with initial_files
    2. Run AgentLoopExecutor with task.input as the user prompt
    3. Capture workspace diff (files_changed)
    4. Optionally run test_command in workspace
    5. Evaluate results via EvaluationPipeline
    """

    def __init__(
        self,
        executor_factory: Callable[[str], AgentLoopExecutor],
        pipeline: EvaluationPipeline,
        loop_config: LoopConfig | None = None,
        workspace_base: str | None = None,
    ) -> None:
        # A fresh executor (and so a fresh tool executor) per task, built for the
        # task's workspace: overriding a shared executor's workspace never reached
        # its tools, which ran in the worker's temporary directory (KI-96 S7).
        self._executor_factory = executor_factory
        self._pipeline = pipeline
        self._loop_config = loop_config
        self._workspace_base = workspace_base

    async def run_task(self, task: TaskSpec) -> RunResult:
        """Run a single agent benchmark task."""
        log = logger.bind(task_id=task.id, task_name=task.name)
        log.info("running agent benchmark task")

        start = time.monotonic()
        # The task's workspace: with tool isolation the benchmark tenant's
        # identity works there; leaving it shares and removes it (KI-71 review).
        async with tool_workspace("cf-bench-", self._workspace_base) as path:
            workspace = Path(path)
            _write_initial_files(task, workspace)
            log.debug("workspace created", path=path)
            result = await self._run_agent(task, workspace, log)
        log.debug("workspace cleaned up")

        duration_ms = int((time.monotonic() - start) * 1000)
        log.info(
            "agent task completed",
            task_id=task.id,
            duration_ms=duration_ms,
            files_changed=len(result.execution.files_changed),
            step_count=result.execution.step_count,
        )
        return result

    async def _run_agent(self, task: TaskSpec, workspace: Path, log: structlog.stdlib.BoundLogger) -> RunResult:
        """Execute the agent loop and collect results."""
        # Snapshot before
        before = _snapshot_files(workspace)

        # Build messages for agent loop
        messages = [{"role": "user", "content": task.input}]

        # Build loop config with task-specific overrides
        config = self._build_config(task)

        executor = self._executor_factory(str(workspace))
        try:
            agent_result = await executor.run(messages=messages, config=config)
        except Exception as exc:
            log.error("agent loop failed", error=str(exc))
            execution = ExecutionResult(
                actual_output=f"ERROR: {exc}",
                duration_ms=0,
            )
            eval_score = await self._pipeline.evaluate(task, execution)
            return RunResult(task=task, execution=execution, eval_score=eval_score)

        # Snapshot after and compute diff
        after = _snapshot_files(workspace)
        files_changed = _compute_files_changed(before, after)

        # Write test harness and patch files to workspace before running tests.
        actual_output = agent_result.final_content if hasattr(agent_result, "final_content") else str(agent_result)
        _prepare_test_files(task, workspace, actual_output)

        # Run test command if specified
        test_output = ""
        exit_code = 0
        if task.test_command:
            timeout = int(task.metadata.get("test_timeout", "60"))
            test_output, exit_code = await _run_test_command(task.test_command, workspace, timeout)
            log.debug("test command completed", exit_code=exit_code, output_len=len(test_output))

        # Extract tool calls from agent result
        tool_calls: list[ToolCall] = []
        if hasattr(agent_result, "tool_messages"):
            for msg in agent_result.tool_messages:
                if isinstance(msg, dict) and msg.get("role") == "tool":
                    tool_name = msg.get("name", "")
                    if tool_name:
                        tool_calls.append(ToolCall(name=tool_name))

        execution = ExecutionResult(
            actual_output=agent_result.final_content if hasattr(agent_result, "final_content") else str(agent_result),
            tool_calls=tool_calls,
            files_changed=files_changed,
            test_output=test_output,
            exit_code=exit_code,
            cost_usd=agent_result.total_cost if hasattr(agent_result, "total_cost") else 0.0,
            tokens_in=agent_result.total_tokens_in if hasattr(agent_result, "total_tokens_in") else 0,
            tokens_out=agent_result.total_tokens_out if hasattr(agent_result, "total_tokens_out") else 0,
            duration_ms=0,  # Will be set by caller
            step_count=agent_result.step_count if hasattr(agent_result, "step_count") else 0,
        )

        eval_score = await self._pipeline.evaluate(task, execution)
        return RunResult(task=task, execution=execution, eval_score=eval_score)

    def _build_config(self, task: TaskSpec) -> LoopConfig:
        """Build LoopConfig from task metadata with fallback to default config."""
        from codeforge.agent_loop import LoopConfig

        base = self._loop_config or LoopConfig()

        max_iterations = int(task.metadata.get("max_iterations", str(base.max_iterations)))
        max_cost = float(task.metadata.get("max_cost", str(base.max_cost or 1.0)))

        return LoopConfig(
            max_iterations=max_iterations,
            max_cost=max_cost,
            model=base.model,
            temperature=base.temperature,
            tags=base.tags,
        )
