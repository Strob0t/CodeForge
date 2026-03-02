"""Tests for benchmark provider protocol, models, and registry."""

from __future__ import annotations

import pytest

from codeforge.evaluation.providers.base import (
    BenchmarkProvider,
    BenchmarkType,
    Capabilities,
    EvalDimension,
    EvalScore,
    ExecutionResult,
    TaskSpec,
    ToolCall,
    _registry,
    get_provider,
    list_providers,
    register_provider,
)

# --- Concrete test provider ---


class FakeProvider:
    """Minimal provider for testing."""

    @property
    def name(self) -> str:
        return "fake"

    @property
    def benchmark_type(self) -> BenchmarkType:
        return BenchmarkType.SIMPLE

    @property
    def capabilities(self) -> Capabilities:
        return Capabilities(llm_judge=True, functional_tests=True)

    async def load_tasks(self) -> list[TaskSpec]:
        return [
            TaskSpec(id="t1", name="FizzBuzz", input="Write fizzbuzz"),
            TaskSpec(id="t2", name="Sum", input="Write sum function"),
        ]

    async def task_count(self) -> int:
        return 2


# --- Model tests ---


class TestBenchmarkType:
    def test_values(self) -> None:
        assert BenchmarkType.SIMPLE == "simple"
        assert BenchmarkType.TOOL_USE == "tool_use"
        assert BenchmarkType.AGENT == "agent"

    def test_enum_from_string(self) -> None:
        assert BenchmarkType("simple") == BenchmarkType.SIMPLE
        assert BenchmarkType("tool_use") == BenchmarkType.TOOL_USE
        assert BenchmarkType("agent") == BenchmarkType.AGENT

    def test_invalid_raises(self) -> None:
        with pytest.raises(ValueError):
            BenchmarkType("invalid")


class TestTaskSpec:
    def test_minimal(self) -> None:
        task = TaskSpec(id="t1", name="Test", input="hello")
        assert task.id == "t1"
        assert task.expected_output == ""
        assert task.difficulty == "medium"
        assert task.initial_files == {}

    def test_full(self) -> None:
        task = TaskSpec(
            id="swe-001",
            name="Fix bug",
            input="The login crashes",
            expected_output="Fixed code",
            expected_tools=[ToolCall(name="bash", args="pytest")],
            context=["auth.py content"],
            difficulty="hard",
            initial_files={"main.py": "print('hello')"},
            test_command="pytest tests/",
            repo_url="https://github.com/example/repo",
            repo_commit="abc123",
            metadata={"category": "bugfix"},
        )
        assert len(task.expected_tools) == 1
        assert task.initial_files["main.py"] == "print('hello')"
        assert task.test_command == "pytest tests/"

    def test_json_roundtrip(self) -> None:
        task = TaskSpec(id="t1", name="Test", input="hello")
        data = task.model_dump_json()
        restored = TaskSpec.model_validate_json(data)
        assert restored.id == task.id


class TestExecutionResult:
    def test_defaults(self) -> None:
        result = ExecutionResult()
        assert result.actual_output == ""
        assert result.exit_code == 0
        assert result.cost_usd == 0.0
        assert result.step_count == 0

    def test_full(self) -> None:
        result = ExecutionResult(
            actual_output="def fizzbuzz(n): ...",
            tool_calls=[ToolCall(name="write_file", args="main.py")],
            files_changed=["main.py"],
            test_output="1 passed",
            exit_code=0,
            cost_usd=0.05,
            tokens_in=100,
            tokens_out=50,
            duration_ms=1500,
            step_count=3,
        )
        assert result.step_count == 3
        assert len(result.files_changed) == 1


class TestEvalScore:
    def test_average_empty(self) -> None:
        score = EvalScore()
        assert score.average_score() == 0.0

    def test_average_single(self) -> None:
        score = EvalScore(dimensions=[EvalDimension(name="correctness", score=0.8)])
        assert score.average_score() == 0.8

    def test_average_multiple(self) -> None:
        score = EvalScore(
            dimensions=[
                EvalDimension(name="correctness", score=0.9),
                EvalDimension(name="code_quality", score=0.7),
            ]
        )
        assert score.average_score() == pytest.approx(0.8)

    def test_cost_fields(self) -> None:
        score = EvalScore(
            total_cost_usd=0.10,
            cost_per_score_point=0.125,
            token_efficiency=0.005,
        )
        assert score.total_cost_usd == 0.10
        assert score.cost_per_score_point == 0.125


class TestCapabilities:
    def test_defaults(self) -> None:
        caps = Capabilities()
        assert not caps.functional_tests
        assert not caps.llm_judge
        assert not caps.swe_bench_style
        assert not caps.sparc_style

    def test_custom(self) -> None:
        caps = Capabilities(functional_tests=True, sparc_style=True)
        assert caps.functional_tests
        assert caps.sparc_style
        assert not caps.llm_judge


# --- Protocol conformance ---


class TestProviderProtocol:
    def test_fake_is_benchmark_provider(self) -> None:
        provider = FakeProvider()
        assert isinstance(provider, BenchmarkProvider)

    @pytest.mark.asyncio
    async def test_load_tasks(self) -> None:
        provider = FakeProvider()
        tasks = await provider.load_tasks()
        assert len(tasks) == 2
        assert tasks[0].id == "t1"

    @pytest.mark.asyncio
    async def test_task_count(self) -> None:
        provider = FakeProvider()
        count = await provider.task_count()
        assert count == 2


# --- Registry tests ---


class TestRegistry:
    def setup_method(self) -> None:
        """Clear registry before each test."""
        _registry.clear()

    def test_register_and_get(self) -> None:
        register_provider("fake", FakeProvider)
        cls = get_provider("fake")
        assert cls is FakeProvider

    def test_unknown_provider_raises(self) -> None:
        with pytest.raises(KeyError, match="unknown provider"):
            get_provider("nonexistent")

    def test_duplicate_registration_raises(self) -> None:
        register_provider("fake", FakeProvider)
        with pytest.raises(ValueError, match="duplicate registration"):
            register_provider("fake", FakeProvider)

    def test_list_providers(self) -> None:
        register_provider("provider-a", FakeProvider)
        register_provider("provider-b", FakeProvider)
        names = list_providers()
        assert "provider-a" in names
        assert "provider-b" in names
        assert len(names) == 2

    def test_list_providers_empty(self) -> None:
        assert list_providers() == []
