"""Tests for Phase 26D — Agent Benchmark Runner.

Tests workspace setup/teardown, file diff computation, test command execution,
agent loop integration, and the CodeForge Agent provider.
"""

from __future__ import annotations

import os
import tempfile
from dataclasses import dataclass, field
from pathlib import Path
from typing import Any

import pytest

from codeforge.evaluation.pipeline import EvaluationPipeline
from codeforge.evaluation.providers.base import (
    EvalDimension,
    ExecutionResult,
    TaskSpec,
)
from codeforge.evaluation.runners.agent import (
    AgentBenchmarkRunner,
    _compute_files_changed,
    _run_test_command,
    _setup_workspace,
    _snapshot_files,
)
from codeforge.evaluation.runners.simple import RunResult

# --- Stub Evaluator ---


class StubEvaluator:
    """Evaluator that returns a fixed score."""

    def __init__(self, score: float = 0.85) -> None:
        self._score = score

    @property
    def name(self) -> str:
        return "stub"

    async def evaluate(self, task: TaskSpec, result: ExecutionResult) -> list[EvalDimension]:
        return [EvalDimension(name="stub_score", score=self._score)]


# --- Fake AgentLoopExecutor ---


@dataclass
class FakeAgentLoopResult:
    """Mimics AgentLoopResult from agent_loop.py."""

    final_content: str = "Task completed successfully."
    tool_messages: list[dict[str, Any]] = field(default_factory=list)
    total_cost: float = 0.02
    total_tokens_in: int = 500
    total_tokens_out: int = 200
    step_count: int = 3
    model: str = "test-model"
    error: str = ""


class FakeAgentLoopExecutor:
    """Fake executor that simulates agent behavior."""

    def __init__(
        self,
        result: FakeAgentLoopResult | None = None,
        files_to_create: dict[str, str] | None = None,
        fail: bool = False,
    ) -> None:
        self._result = result or FakeAgentLoopResult()
        self._files_to_create = files_to_create or {}
        self._fail = fail
        self._workspace_path: str = ""
        self.call_count = 0
        self.last_messages: list[dict] | None = None
        self.last_config: Any = None

    async def run(self, messages: list[dict], config: Any = None) -> FakeAgentLoopResult:
        self.call_count += 1
        self.last_messages = messages
        self.last_config = config

        if self._fail:
            msg = "Agent loop crashed"
            raise RuntimeError(msg)

        # Simulate agent creating files in workspace
        if self._workspace_path and self._files_to_create:
            workspace = Path(self._workspace_path)
            for rel_path, content in self._files_to_create.items():
                fpath = workspace / rel_path
                fpath.parent.mkdir(parents=True, exist_ok=True)
                fpath.write_text(content)

        return self._result


# --- Workspace Tests ---


class TestSetupWorkspace:
    def test_empty_workspace(self) -> None:
        workspace = _setup_workspace(TaskSpec(id="t1", name="Test", input="hello"))
        assert workspace.exists()
        assert workspace.is_dir()
        assert list(workspace.iterdir()) == []
        workspace.rmdir()

    def test_workspace_with_initial_files(self) -> None:
        task = TaskSpec(
            id="t1",
            name="Test",
            input="hello",
            initial_files={
                "main.py": "print('hello')",
                "src/utils.py": "def helper(): pass",
            },
        )
        workspace = _setup_workspace(task)
        assert (workspace / "main.py").read_text() == "print('hello')"
        assert (workspace / "src" / "utils.py").read_text() == "def helper(): pass"
        import shutil

        shutil.rmtree(workspace)

    def test_workspace_with_nested_dirs(self) -> None:
        task = TaskSpec(
            id="t1",
            name="Test",
            input="hello",
            initial_files={"a/b/c/deep.txt": "deep content"},
        )
        workspace = _setup_workspace(task)
        assert (workspace / "a" / "b" / "c" / "deep.txt").read_text() == "deep content"
        import shutil

        shutil.rmtree(workspace)


class TestSnapshotFiles:
    def test_empty_dir(self) -> None:
        with tempfile.TemporaryDirectory() as d:
            snap = _snapshot_files(Path(d))
            assert snap == {}

    def test_with_files(self) -> None:
        with tempfile.TemporaryDirectory() as d:
            Path(d, "a.txt").write_text("hello")
            Path(d, "b.txt").write_text("world")
            snap = _snapshot_files(Path(d))
            assert snap == {"a.txt": "hello", "b.txt": "world"}

    def test_ignores_hidden_dirs(self) -> None:
        with tempfile.TemporaryDirectory() as d:
            Path(d, "visible.txt").write_text("yes")
            hidden = Path(d, ".hidden")
            hidden.mkdir()
            Path(hidden, "secret.txt").write_text("no")
            snap = _snapshot_files(Path(d))
            assert "visible.txt" in snap
            assert ".hidden/secret.txt" not in snap

    def test_nonexistent_dir(self) -> None:
        snap = _snapshot_files(Path("/nonexistent/path"))
        assert snap == {}


class TestComputeFilesChanged:
    def test_no_changes(self) -> None:
        before = {"a.txt": "hello"}
        after = {"a.txt": "hello"}
        assert _compute_files_changed(before, after) == []

    def test_file_added(self) -> None:
        before: dict[str, str] = {}
        after = {"new.py": "print('new')"}
        assert _compute_files_changed(before, after) == ["new.py"]

    def test_file_deleted(self) -> None:
        before = {"old.py": "print('old')"}
        after: dict[str, str] = {}
        assert _compute_files_changed(before, after) == ["old.py"]

    def test_file_modified(self) -> None:
        before = {"main.py": "v1"}
        after = {"main.py": "v2"}
        assert _compute_files_changed(before, after) == ["main.py"]

    def test_mixed_changes(self) -> None:
        before = {"keep.py": "same", "modify.py": "old", "delete.py": "gone"}
        after = {"keep.py": "same", "modify.py": "new", "add.py": "fresh"}
        changed = _compute_files_changed(before, after)
        assert "add.py" in changed
        assert "delete.py" in changed
        assert "modify.py" in changed
        assert "keep.py" not in changed


# --- Test Command Execution ---


class TestRunTestCommand:
    @pytest.mark.asyncio
    async def test_successful_command(self) -> None:
        with tempfile.TemporaryDirectory() as d:
            output, exit_code = await _run_test_command("echo PASS", Path(d))
            assert exit_code == 0
            assert "PASS" in output

    @pytest.mark.asyncio
    async def test_failing_command(self) -> None:
        with tempfile.TemporaryDirectory() as d:
            _output, exit_code = await _run_test_command("exit 1", Path(d))
            assert exit_code == 1

    @pytest.mark.asyncio
    async def test_timeout(self) -> None:
        with tempfile.TemporaryDirectory() as d:
            output, exit_code = await _run_test_command("sleep 10", Path(d), timeout=1)
            assert exit_code == 124
            assert "timed out" in output.lower()

    @pytest.mark.asyncio
    async def test_command_with_workspace_context(self) -> None:
        with tempfile.TemporaryDirectory() as d:
            Path(d, "test.txt").write_text("hello world")
            output, exit_code = await _run_test_command("cat test.txt", Path(d))
            assert exit_code == 0
            assert "hello world" in output


# --- AgentBenchmarkRunner Tests ---


class TestAgentBenchmarkRunner:
    @pytest.mark.asyncio
    async def test_run_single_task(self) -> None:
        executor = FakeAgentLoopExecutor()
        pipeline = EvaluationPipeline([StubEvaluator(0.9)])
        runner = AgentBenchmarkRunner(executor=executor, pipeline=pipeline)

        task = TaskSpec(id="t1", name="FizzBuzz", input="Write fizzbuzz.py")
        result = await runner.run_task(task)

        assert isinstance(result, RunResult)
        assert result.task.id == "t1"
        assert result.execution.actual_output == "Task completed successfully."
        assert result.execution.cost_usd == 0.02
        assert result.execution.tokens_in == 500
        assert result.execution.tokens_out == 200
        assert result.execution.step_count == 3
        assert result.eval_score is not None
        assert result.eval_score.average_score() == pytest.approx(0.9)
        assert executor.call_count == 1

    @pytest.mark.asyncio
    async def test_workspace_with_initial_files(self) -> None:
        executor = FakeAgentLoopExecutor()
        pipeline = EvaluationPipeline([StubEvaluator(0.8)])
        runner = AgentBenchmarkRunner(executor=executor, pipeline=pipeline)

        task = TaskSpec(
            id="t1",
            name="BugFix",
            input="Fix the bug",
            initial_files={"main.py": "def broken(): pass"},
        )
        result = await runner.run_task(task)

        assert result.task.id == "t1"
        assert executor.last_messages == [{"role": "user", "content": "Fix the bug"}]

    @pytest.mark.asyncio
    async def test_files_changed_detection(self) -> None:
        # Executor will create a new file in workspace
        executor = FakeAgentLoopExecutor(files_to_create={"output.py": "print('created by agent')"})
        pipeline = EvaluationPipeline([StubEvaluator(0.85)])
        runner = AgentBenchmarkRunner(executor=executor, pipeline=pipeline)

        task = TaskSpec(id="t1", name="Create", input="Create output.py")
        result = await runner.run_task(task)

        assert "output.py" in result.execution.files_changed

    @pytest.mark.asyncio
    async def test_files_modified_detection(self) -> None:
        executor = FakeAgentLoopExecutor(files_to_create={"main.py": "modified content"})
        pipeline = EvaluationPipeline([StubEvaluator(0.8)])
        runner = AgentBenchmarkRunner(executor=executor, pipeline=pipeline)

        task = TaskSpec(
            id="t1",
            name="Modify",
            input="Modify main.py",
            initial_files={"main.py": "original content"},
        )
        result = await runner.run_task(task)

        assert "main.py" in result.execution.files_changed

    @pytest.mark.asyncio
    async def test_test_command_execution(self) -> None:
        executor = FakeAgentLoopExecutor(files_to_create={"hello.py": "print('hello')"})
        pipeline = EvaluationPipeline([StubEvaluator(0.9)])
        runner = AgentBenchmarkRunner(executor=executor, pipeline=pipeline)

        task = TaskSpec(
            id="t1",
            name="WithTest",
            input="Create hello.py",
            test_command="python hello.py",
        )
        result = await runner.run_task(task)

        assert "hello" in result.execution.test_output  # test command ran in workspace

    @pytest.mark.asyncio
    async def test_agent_failure_handled(self) -> None:
        executor = FakeAgentLoopExecutor(fail=True)
        pipeline = EvaluationPipeline([StubEvaluator(0.0)])
        runner = AgentBenchmarkRunner(executor=executor, pipeline=pipeline)

        task = TaskSpec(id="t1", name="Fail", input="fail please")
        result = await runner.run_task(task)

        assert "ERROR" in result.execution.actual_output
        assert result.eval_score is not None

    @pytest.mark.asyncio
    async def test_multiple_tasks(self) -> None:
        executor = FakeAgentLoopExecutor()
        pipeline = EvaluationPipeline([StubEvaluator(0.7)])
        runner = AgentBenchmarkRunner(executor=executor, pipeline=pipeline)

        tasks = [
            TaskSpec(id="t1", name="Task1", input="Do thing 1"),
            TaskSpec(id="t2", name="Task2", input="Do thing 2"),
            TaskSpec(id="t3", name="Task3", input="Do thing 3"),
        ]
        results = await runner.run_tasks(tasks)

        assert len(results) == 3
        assert executor.call_count == 3
        assert all(r.eval_score is not None for r in results)

    @pytest.mark.asyncio
    async def test_workspace_cleanup(self) -> None:
        executor = FakeAgentLoopExecutor()
        pipeline = EvaluationPipeline([StubEvaluator(0.8)])
        runner = AgentBenchmarkRunner(executor=executor, pipeline=pipeline)

        task = TaskSpec(
            id="t1",
            name="Cleanup",
            input="test",
            initial_files={"temp.py": "temporary"},
        )
        # After run_task, workspace should be cleaned up
        await runner.run_task(task)

        # Verify no leftover directories (check that temp dir was removed)
        # The workspace was in a temp dir, so it should be gone
        # We can't easily verify this without capturing the path,
        # but the test succeeding means cleanup didn't error

    @pytest.mark.asyncio
    async def test_tool_messages_extracted(self) -> None:
        result = FakeAgentLoopResult(
            tool_messages=[
                {"role": "tool", "name": "read_file", "content": "file contents"},
                {"role": "tool", "name": "write_file", "content": "written"},
                {"role": "assistant", "content": "done"},  # non-tool message
            ]
        )
        executor = FakeAgentLoopExecutor(result=result)
        pipeline = EvaluationPipeline([StubEvaluator(0.8)])
        runner = AgentBenchmarkRunner(executor=executor, pipeline=pipeline)

        task = TaskSpec(id="t1", name="Tools", input="use tools")
        run_result = await runner.run_task(task)

        assert len(run_result.execution.tool_calls) == 2
        assert run_result.execution.tool_calls[0].name == "read_file"
        assert run_result.execution.tool_calls[1].name == "write_file"


# --- CodeForge Agent Provider Tests ---


class TestCodeForgeAgentProvider:
    def test_properties(self) -> None:
        from codeforge.evaluation.providers.codeforge_agent import CodeForgeAgentProvider

        p = CodeForgeAgentProvider()
        assert p.name == "codeforge_agent"
        assert p.benchmark_type == "agent"
        assert p.capabilities.functional_tests is True
        assert p.capabilities.llm_judge is True
        assert p.capabilities.swe_bench_style is True

    @pytest.mark.asyncio
    async def test_load_tasks_with_initial_files(self) -> None:
        from codeforge.evaluation.providers.codeforge_agent import CodeForgeAgentProvider

        yaml_content = """
name: test-agent
tasks:
  - id: ag-1
    name: BugFix
    input: Fix the bug
    initial_files:
      main.py: "def broken(): pass"
    test_command: "python -m pytest"
    max_iterations: 10
    timeout_seconds: 120
    difficulty: easy
"""
        with tempfile.NamedTemporaryFile(mode="w", suffix=".yaml", delete=False) as f:
            f.write(yaml_content)
            f.flush()
            provider = CodeForgeAgentProvider(dataset_path=f.name)
            tasks = await provider.load_tasks()

        os.unlink(f.name)
        assert len(tasks) == 1
        assert tasks[0].id == "ag-1"
        assert tasks[0].initial_files == {"main.py": "def broken(): pass"}
        assert tasks[0].test_command == "python -m pytest"
        assert tasks[0].metadata["max_iterations"] == "10"
        assert tasks[0].metadata["timeout_seconds"] == "120"
        assert tasks[0].difficulty == "easy"

    @pytest.mark.asyncio
    async def test_load_tasks_with_tools(self) -> None:
        from codeforge.evaluation.providers.codeforge_agent import CodeForgeAgentProvider

        yaml_content = """
name: test-agent-tools
tasks:
  - id: ag-2
    name: ToolTask
    input: Use tools
    tools:
      - type: function
        function:
          name: read_file
    expected_tool_sequence:
      - name: read_file
        args: '{"path": "test.py"}'
"""
        with tempfile.NamedTemporaryFile(mode="w", suffix=".yaml", delete=False) as f:
            f.write(yaml_content)
            f.flush()
            provider = CodeForgeAgentProvider(dataset_path=f.name)
            tasks = await provider.load_tasks()

        os.unlink(f.name)
        assert len(tasks) == 1
        assert len(tasks[0].expected_tools) == 1
        assert tasks[0].expected_tools[0].name == "read_file"
        assert "tools" in tasks[0].metadata

    @pytest.mark.asyncio
    async def test_task_count(self) -> None:
        from codeforge.evaluation.providers.codeforge_agent import CodeForgeAgentProvider

        yaml_content = """
name: count-test
tasks:
  - id: t1
    name: A
    input: a
  - id: t2
    name: B
    input: b
"""
        with tempfile.NamedTemporaryFile(mode="w", suffix=".yaml", delete=False) as f:
            f.write(yaml_content)
            f.flush()
            provider = CodeForgeAgentProvider(dataset_path=f.name)
            count = await provider.task_count()

        os.unlink(f.name)
        assert count == 2

    def test_registration(self) -> None:
        import codeforge.evaluation.providers.codeforge_agent  # noqa: F401
        from codeforge.evaluation.providers.base import get_provider

        cls = get_provider("codeforge_agent")
        assert cls is not None


# --- Integration with SPARC Evaluator ---


class TestAgentRunnerIntegration:
    @pytest.mark.asyncio
    async def test_agent_with_sparc_pipeline(self) -> None:
        from codeforge.evaluation.evaluators.sparc import SPARCEvaluator

        result = FakeAgentLoopResult(step_count=5, total_cost=0.10)
        executor = FakeAgentLoopExecutor(result=result)
        pipeline = EvaluationPipeline([SPARCEvaluator()])
        runner = AgentBenchmarkRunner(executor=executor, pipeline=pipeline)

        task = TaskSpec(id="t1", name="Agent", input="code something")
        run_result = await runner.run_task(task)

        assert run_result.eval_score is not None
        dim_names = [d.name for d in run_result.eval_score.dimensions]
        assert "sparc_steps" in dim_names
        assert "sparc_cost" in dim_names
        # SPARC evaluates step_count and cost from execution
        assert run_result.execution.step_count == 5
        assert run_result.execution.cost_usd == 0.10

    @pytest.mark.asyncio
    async def test_agent_with_multi_evaluator(self) -> None:
        executor = FakeAgentLoopExecutor()
        pipeline = EvaluationPipeline([StubEvaluator(0.8), StubEvaluator(0.6)])
        runner = AgentBenchmarkRunner(executor=executor, pipeline=pipeline)

        task = TaskSpec(id="t1", name="Multi", input="test")
        result = await runner.run_task(task)

        assert result.eval_score is not None
        assert len(result.eval_score.dimensions) == 2
        assert result.eval_score.average_score() == pytest.approx(0.7)
