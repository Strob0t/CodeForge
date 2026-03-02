"""Tests for Phase 26C — Simple & Tool-Use benchmark runners + providers."""

from __future__ import annotations

import asyncio
import json
import os
import tempfile
from dataclasses import dataclass
from typing import Any

import pytest

from codeforge.evaluation.pipeline import EvaluationPipeline
from codeforge.evaluation.providers.base import (
    EvalDimension,
    ExecutionResult,
    TaskSpec,
)
from codeforge.evaluation.runners.simple import RunResult, SimpleBenchmarkRunner
from codeforge.evaluation.runners.tool_use import ToolUseBenchmarkRunner, _parse_tools

# --- Fake LLM client ---


@dataclass
class FakeUsage:
    prompt_tokens: int = 50
    completion_tokens: int = 25


@dataclass
class FakeFunction:
    name: str = "read_file"
    arguments: str = '{"path": "test.py"}'


@dataclass
class FakeToolCall:
    function: Any = None

    def __post_init__(self) -> None:
        if self.function is None:
            self.function = FakeFunction()


@dataclass
class FakeChatResponse:
    content: str = "fake response"
    usage: FakeUsage | None = None
    cost: float = 0.01
    tool_calls: list[FakeToolCall] | None = None

    def __post_init__(self) -> None:
        if self.usage is None:
            self.usage = FakeUsage()


class FakeLLMClient:
    """Fake LLM client that returns predictable responses."""

    def __init__(self, response: FakeChatResponse | None = None, fail: bool = False) -> None:
        self._response = response or FakeChatResponse()
        self._fail = fail
        self.call_count = 0

    async def chat(self, **kwargs: Any) -> FakeChatResponse:
        self.call_count += 1
        if self._fail:
            msg = "LLM unavailable"
            raise ConnectionError(msg)
        return self._response


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


# --- SimpleBenchmarkRunner Tests ---


class TestSimpleBenchmarkRunner:
    def test_run_single_task(self) -> None:
        llm = FakeLLMClient()
        pipeline = EvaluationPipeline([StubEvaluator(0.85)])
        runner = SimpleBenchmarkRunner(llm=llm, pipeline=pipeline, model="test")
        task = TaskSpec(id="t1", name="Test", input="hello")

        result = asyncio.get_event_loop().run_until_complete(runner.run_task(task))

        assert isinstance(result, RunResult)
        assert result.task.id == "t1"
        assert result.execution.actual_output == "fake response"
        assert result.execution.tokens_in == 50
        assert result.execution.tokens_out == 25
        assert result.execution.cost_usd == 0.01
        assert result.execution.duration_ms >= 0
        assert result.eval_score is not None
        assert result.eval_score.average_score() == pytest.approx(0.85)

    @pytest.mark.asyncio
    async def test_run_multiple_tasks(self) -> None:
        llm = FakeLLMClient()
        pipeline = EvaluationPipeline([StubEvaluator(0.9)])
        runner = SimpleBenchmarkRunner(llm=llm, pipeline=pipeline, model="test")
        tasks = [
            TaskSpec(id="t1", name="Task1", input="hello"),
            TaskSpec(id="t2", name="Task2", input="world"),
        ]

        results = await runner.run_tasks(tasks)

        assert len(results) == 2
        assert llm.call_count == 2
        assert all(r.eval_score is not None for r in results)

    @pytest.mark.asyncio
    async def test_llm_failure_handled(self) -> None:
        llm = FakeLLMClient(fail=True)
        pipeline = EvaluationPipeline([StubEvaluator(0.0)])
        runner = SimpleBenchmarkRunner(llm=llm, pipeline=pipeline, model="test")
        task = TaskSpec(id="t1", name="Test", input="hello")

        result = await runner.run_task(task)

        assert "ERROR" in result.execution.actual_output
        assert result.execution.tokens_in == 0

    @pytest.mark.asyncio
    async def test_cost_tracking(self) -> None:
        resp = FakeChatResponse(cost=0.05, usage=FakeUsage(prompt_tokens=100, completion_tokens=50))
        llm = FakeLLMClient(response=resp)
        pipeline = EvaluationPipeline([StubEvaluator(0.7)])
        runner = SimpleBenchmarkRunner(llm=llm, pipeline=pipeline, model="test")
        task = TaskSpec(id="t1", name="Test", input="hello")

        result = await runner.run_task(task)

        assert result.execution.cost_usd == 0.05
        assert result.execution.tokens_in == 100
        assert result.execution.tokens_out == 50


# --- ToolUseBenchmarkRunner Tests ---


class TestToolUseBenchmarkRunner:
    @pytest.mark.asyncio
    async def test_run_with_tool_calls(self) -> None:
        tc = FakeToolCall()
        resp = FakeChatResponse(content="done", tool_calls=[tc])
        llm = FakeLLMClient(response=resp)
        pipeline = EvaluationPipeline([StubEvaluator(0.8)])
        runner = ToolUseBenchmarkRunner(llm=llm, pipeline=pipeline, model="test")
        task = TaskSpec(
            id="t1",
            name="ToolTest",
            input="read file",
            metadata={"tools": json.dumps([{"type": "function", "function": {"name": "read_file"}}])},
        )

        result = await runner.run_task(task)

        assert result.execution.actual_output == "done"
        assert len(result.execution.tool_calls) == 1

    @pytest.mark.asyncio
    async def test_run_without_tools(self) -> None:
        llm = FakeLLMClient()
        pipeline = EvaluationPipeline([StubEvaluator(0.7)])
        runner = ToolUseBenchmarkRunner(llm=llm, pipeline=pipeline, model="test")
        task = TaskSpec(id="t1", name="NoTools", input="hello")

        result = await runner.run_task(task)

        assert result.execution.actual_output == "fake response"
        assert len(result.execution.tool_calls) == 0

    @pytest.mark.asyncio
    async def test_multiple_tool_calls(self) -> None:
        tc1 = FakeToolCall()
        tc2 = FakeToolCall()
        tc2.function = FakeFunction(name="write_file", arguments='{"path": "out.py", "content": "pass"}')
        resp = FakeChatResponse(content="done", tool_calls=[tc1, tc2])
        llm = FakeLLMClient(response=resp)
        pipeline = EvaluationPipeline([StubEvaluator(0.9)])
        runner = ToolUseBenchmarkRunner(llm=llm, pipeline=pipeline, model="test")
        task = TaskSpec(
            id="t1",
            name="MultiTool",
            input="read and write",
            metadata={
                "tools": json.dumps(
                    [
                        {"type": "function", "function": {"name": "read_file"}},
                        {"type": "function", "function": {"name": "write_file"}},
                    ]
                )
            },
        )

        result = await runner.run_task(task)

        assert len(result.execution.tool_calls) == 2


# --- _parse_tools Tests ---


class TestParseTools:
    def test_parse_valid_tools(self) -> None:
        task = TaskSpec(
            id="t1",
            name="Test",
            input="test",
            metadata={"tools": json.dumps([{"type": "function", "function": {"name": "read_file"}}])},
        )
        tools = _parse_tools(task)
        assert len(tools) == 1
        assert tools[0]["type"] == "function"

    def test_parse_empty_tools(self) -> None:
        task = TaskSpec(id="t1", name="Test", input="test")
        tools = _parse_tools(task)
        assert tools == []

    def test_parse_invalid_json(self) -> None:
        task = TaskSpec(id="t1", name="Test", input="test", metadata={"tools": "not json"})
        tools = _parse_tools(task)
        assert tools == []

    def test_parse_non_list_json(self) -> None:
        task = TaskSpec(id="t1", name="Test", input="test", metadata={"tools": '{"not": "a list"}'})
        tools = _parse_tools(task)
        assert tools == []


# --- Provider Registration Tests ---


class TestProviderRegistration:
    def test_codeforge_simple_registered(self) -> None:
        # Import triggers registration
        import codeforge.evaluation.providers.codeforge_simple  # noqa: F401
        from codeforge.evaluation.providers.base import get_provider

        cls = get_provider("codeforge_simple")
        assert cls is not None

    def test_codeforge_tool_use_registered(self) -> None:
        import codeforge.evaluation.providers.codeforge_tool_use  # noqa: F401
        from codeforge.evaluation.providers.base import get_provider

        cls = get_provider("codeforge_tool_use")
        assert cls is not None


# --- CodeForgeSimpleProvider Tests ---


class TestCodeForgeSimpleProvider:
    def test_properties(self) -> None:
        from codeforge.evaluation.providers.codeforge_simple import CodeForgeSimpleProvider

        p = CodeForgeSimpleProvider()
        assert p.name == "codeforge_simple"
        assert p.benchmark_type == "simple"
        assert p.capabilities.llm_judge is True
        assert p.capabilities.functional_tests is False

    @pytest.mark.asyncio
    async def test_load_tasks_from_yaml(self) -> None:
        from codeforge.evaluation.providers.codeforge_simple import CodeForgeSimpleProvider

        yaml_content = """
name: test-dataset
description: Test
tasks:
  - id: t1
    name: FizzBuzz
    input: Write FizzBuzz
    expected_output: "def fizzbuzz():"
    difficulty: easy
  - id: t2
    name: Sort
    input: Sort a list
    expected_output: "sorted()"
"""
        with tempfile.NamedTemporaryFile(mode="w", suffix=".yaml", delete=False) as f:
            f.write(yaml_content)
            f.flush()
            provider = CodeForgeSimpleProvider(dataset_path=f.name)
            tasks = await provider.load_tasks()

        os.unlink(f.name)
        assert len(tasks) == 2
        assert tasks[0].id == "t1"
        assert tasks[0].difficulty == "easy"
        assert tasks[1].name == "Sort"

    @pytest.mark.asyncio
    async def test_task_count(self) -> None:
        from codeforge.evaluation.providers.codeforge_simple import CodeForgeSimpleProvider

        yaml_content = """
name: test
tasks:
  - id: t1
    name: A
    input: a
  - id: t2
    name: B
    input: b
  - id: t3
    name: C
    input: c
"""
        with tempfile.NamedTemporaryFile(mode="w", suffix=".yaml", delete=False) as f:
            f.write(yaml_content)
            f.flush()
            provider = CodeForgeSimpleProvider(dataset_path=f.name)
            count = await provider.task_count()

        os.unlink(f.name)
        assert count == 3


# --- CodeForgeToolUseProvider Tests ---


class TestCodeForgeToolUseProvider:
    def test_properties(self) -> None:
        from codeforge.evaluation.providers.codeforge_tool_use import CodeForgeToolUseProvider

        p = CodeForgeToolUseProvider()
        assert p.name == "codeforge_tool_use"
        assert p.benchmark_type == "tool_use"
        assert p.capabilities.llm_judge is True
        assert p.capabilities.functional_tests is True

    @pytest.mark.asyncio
    async def test_load_tasks_with_tools(self) -> None:
        from codeforge.evaluation.providers.codeforge_tool_use import CodeForgeToolUseProvider

        yaml_content = """
name: tool-test
tasks:
  - id: t1
    name: ReadFile
    input: Read test.py
    tools:
      - type: function
        function:
          name: read_file
          parameters:
            type: object
            properties:
              path:
                type: string
    expected_tool_sequence:
      - name: read_file
        args: '{"path": "test.py"}'
"""
        with tempfile.NamedTemporaryFile(mode="w", suffix=".yaml", delete=False) as f:
            f.write(yaml_content)
            f.flush()
            provider = CodeForgeToolUseProvider(dataset_path=f.name)
            tasks = await provider.load_tasks()

        os.unlink(f.name)
        assert len(tasks) == 1
        assert tasks[0].id == "t1"
        assert len(tasks[0].expected_tools) == 1
        assert tasks[0].expected_tools[0].name == "read_file"
        assert "tools" in tasks[0].metadata

    def test_load_existing_dataset(self) -> None:
        dataset_path = os.path.join(
            os.path.dirname(__file__), "..", "..", "configs", "benchmarks", "tool-use-basic.yaml"
        )
        if not os.path.exists(dataset_path):
            pytest.skip("tool-use-basic.yaml not found")

        from codeforge.evaluation.providers.codeforge_tool_use import CodeForgeToolUseProvider

        provider = CodeForgeToolUseProvider(dataset_path=dataset_path)
        tasks = asyncio.get_event_loop().run_until_complete(provider.load_tasks())
        assert len(tasks) >= 1


# --- Benchmark Models Tests ---


class TestBenchmarkModels:
    def test_benchmark_run_request_phase26_fields(self) -> None:
        from codeforge.models import BenchmarkRunRequest

        req = BenchmarkRunRequest(
            run_id="r1",
            dataset_path="test.yaml",
            model="gpt-4",
            benchmark_type="tool_use",
            suite_id="suite-1",
            exec_mode="sandbox",
            evaluators=["llm_judge", "sparc"],
        )
        assert req.benchmark_type == "tool_use"
        assert req.suite_id == "suite-1"
        assert req.exec_mode == "sandbox"
        assert len(req.evaluators) == 2

    def test_benchmark_run_request_defaults(self) -> None:
        from codeforge.models import BenchmarkRunRequest

        req = BenchmarkRunRequest(run_id="r1", dataset_path="test.yaml", model="gpt-4")
        assert req.benchmark_type == "simple"
        assert req.suite_id == ""
        assert req.exec_mode == ""
        assert req.evaluators == []

    def test_benchmark_task_result_phase26_fields(self) -> None:
        from codeforge.models import BenchmarkTaskResult

        result = BenchmarkTaskResult(
            task_id="t1",
            task_name="Test",
            evaluator_scores={"llm_judge": {"correctness": 0.9}},
            files_changed=["main.py"],
            functional_test_output="1 passed",
        )
        assert result.evaluator_scores["llm_judge"]["correctness"] == 0.9
        assert result.files_changed == ["main.py"]
        assert result.functional_test_output == "1 passed"


# --- Integration Tests ---


class TestRunnerPipelineIntegration:
    @pytest.mark.asyncio
    async def test_simple_runner_with_sparc_pipeline(self) -> None:
        from codeforge.evaluation.evaluators.sparc import SPARCEvaluator

        llm = FakeLLMClient()
        pipeline = EvaluationPipeline([SPARCEvaluator()])
        runner = SimpleBenchmarkRunner(llm=llm, pipeline=pipeline, model="test")
        task = TaskSpec(id="t1", name="Test", input="hello", expected_output="world")

        result = await runner.run_task(task)

        assert result.eval_score is not None
        assert len(result.eval_score.dimensions) > 0
        dim_names = [d.name for d in result.eval_score.dimensions]
        assert "sparc_steps" in dim_names
        assert "sparc_cost" in dim_names

    @pytest.mark.asyncio
    async def test_tool_use_runner_with_multi_evaluator_pipeline(self) -> None:
        pipeline = EvaluationPipeline([StubEvaluator(0.8), StubEvaluator(0.6)])
        llm = FakeLLMClient()
        runner = ToolUseBenchmarkRunner(llm=llm, pipeline=pipeline, model="test")
        task = TaskSpec(id="t1", name="Test", input="hello")

        result = await runner.run_task(task)

        assert result.eval_score is not None
        assert len(result.eval_score.dimensions) == 2
        assert result.eval_score.average_score() == pytest.approx(0.7)
