"""Agent benchmark runner — full multi-turn agent loop with tools and workspace.

Dispatches tasks through AgentLoopExecutor, captures workspace diffs,
runs test commands, and feeds results into the evaluation pipeline.
"""

from __future__ import annotations

import asyncio
import contextlib
import shutil
import tempfile
import time
from pathlib import Path
from typing import TYPE_CHECKING

import structlog

from codeforge.evaluation.providers.base import ExecutionResult, TaskSpec, ToolCall
from codeforge.evaluation.runners._base import BaseBenchmarkRunner, RunResult

if TYPE_CHECKING:
    from codeforge.agent_loop import AgentLoopExecutor, LoopConfig
    from codeforge.evaluation.pipeline import EvaluationPipeline

logger = structlog.get_logger(__name__)


def _snapshot_files(workspace: Path) -> dict[str, str]:
    """Capture file contents in workspace for diff comparison."""
    snapshot: dict[str, str] = {}
    if not workspace.exists():
        return snapshot
    for fpath in workspace.rglob("*"):
        if fpath.is_file() and not any(p.startswith(".") for p in fpath.relative_to(workspace).parts):
            rel = str(fpath.relative_to(workspace))
            with contextlib.suppress(OSError):
                snapshot[rel] = fpath.read_text(encoding="utf-8", errors="replace")
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
    """Create a temporary workspace and write initial files from task spec."""
    workspace = Path(tempfile.mkdtemp(prefix="bench_agent_", dir=base_dir))
    for rel_path, content in task.initial_files.items():
        fpath = workspace / rel_path
        fpath.parent.mkdir(parents=True, exist_ok=True)
        fpath.write_text(content, encoding="utf-8")
    return workspace


async def _run_test_command(test_command: str, workspace: Path, timeout: int = 60) -> tuple[str, int]:
    """Run a test command in the workspace and return (output, exit_code)."""
    try:
        proc = await asyncio.create_subprocess_shell(
            test_command,
            stdout=asyncio.subprocess.PIPE,
            stderr=asyncio.subprocess.STDOUT,
            cwd=str(workspace),
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
    """
    test_harness = task.metadata.get("test_harness", "")
    if test_harness and "{SOLUTION}" in test_harness:
        harness_content = test_harness.replace("{SOLUTION}", solution)
        (workspace / "solution.py").write_text(harness_content, encoding="utf-8")

    test_patch = task.metadata.get("test_patch", "")
    if test_patch:
        (workspace / "test_patch.diff").write_text(test_patch, encoding="utf-8")


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
        executor: AgentLoopExecutor,
        pipeline: EvaluationPipeline,
        loop_config: LoopConfig | None = None,
        workspace_base: str | None = None,
    ) -> None:
        self._executor = executor
        self._pipeline = pipeline
        self._loop_config = loop_config
        self._workspace_base = workspace_base

    async def run_task(self, task: TaskSpec) -> RunResult:
        """Run a single agent benchmark task."""
        log = logger.bind(task_id=task.id, task_name=task.name)
        log.info("running agent benchmark task")

        workspace = _setup_workspace(task, self._workspace_base)
        log.debug("workspace created", path=str(workspace))

        start = time.monotonic()
        try:
            result = await self._run_agent(task, workspace, log)
        finally:
            # Clean up workspace
            shutil.rmtree(workspace, ignore_errors=True)
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

        # Store original workspace on executor if it supports it
        original_workspace = getattr(self._executor, "_workspace_path", None)
        if hasattr(self._executor, "_workspace_path"):
            self._executor._workspace_path = str(workspace)

        try:
            agent_result = await self._executor.run(messages=messages, config=config)
        except Exception as exc:
            log.error("agent loop failed", error=str(exc))
            execution = ExecutionResult(
                actual_output=f"ERROR: {exc}",
                duration_ms=0,
            )
            eval_score = await self._pipeline.evaluate(task, execution)
            return RunResult(task=task, execution=execution, eval_score=eval_score)
        finally:
            # Restore original workspace
            if original_workspace is not None and hasattr(self._executor, "_workspace_path"):
                self._executor._workspace_path = original_workspace

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
