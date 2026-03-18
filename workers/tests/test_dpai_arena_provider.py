"""Tests for DPAI Arena benchmark provider.

DPAI Arena provides coding challenges for evaluating LLM coding capabilities.
Tests cover: properties, task loading, conversion, edge cases, registration.
"""

from __future__ import annotations

from typing import ClassVar

import pytest

from codeforge.evaluation.providers.base import (
    BenchmarkType,
    get_provider,
    list_providers,
)

# ---------------------------------------------------------------------------
# Sample DPAI Arena data for testing
# ---------------------------------------------------------------------------

_DPAI_SAMPLES: list[dict] = [
    {
        "id": "dpai_001",
        "question": "Write a Python function `fibonacci(n)` that returns the nth Fibonacci number.",
        "solution": "def fibonacci(n):\n    if n <= 1:\n        return n\n    a, b = 0, 1\n    for _ in range(2, n + 1):\n        a, b = b, a + b\n    return b\n",
        "test_cases": "assert fibonacci(0) == 0\nassert fibonacci(1) == 1\nassert fibonacci(10) == 55\n",
        "difficulty": "easy",
        "tags": "dynamic-programming,math",
        "language": "python",
    },
    {
        "id": "dpai_002",
        "question": "Implement a function `merge_sorted(a, b)` that merges two sorted lists.",
        "solution": "def merge_sorted(a, b):\n    result = []\n    i = j = 0\n    while i < len(a) and j < len(b):\n        if a[i] <= b[j]:\n            result.append(a[i])\n            i += 1\n        else:\n            result.append(b[j])\n            j += 1\n    result.extend(a[i:])\n    result.extend(b[j:])\n    return result\n",
        "test_cases": "assert merge_sorted([1,3,5], [2,4,6]) == [1,2,3,4,5,6]\nassert merge_sorted([], [1,2]) == [1,2]\n",
        "difficulty": "medium",
        "tags": "sorting,arrays",
        "language": "python",
    },
    {
        "id": "dpai_003",
        "question": "Write a function `lcs(s1, s2)` that returns the longest common subsequence.",
        "solution": "def lcs(s1, s2):\n    m, n = len(s1), len(s2)\n    dp = [[''] * (n + 1) for _ in range(m + 1)]\n    for i in range(1, m + 1):\n        for j in range(1, n + 1):\n            if s1[i-1] == s2[j-1]:\n                dp[i][j] = dp[i-1][j-1] + s1[i-1]\n            else:\n                dp[i][j] = max(dp[i-1][j], dp[i][j-1], key=len)\n    return dp[m][n]\n",
        "test_cases": "",
        "difficulty": "hard",
        "tags": "dynamic-programming,strings",
        "language": "python",
    },
]


# ---------------------------------------------------------------------------
# Provider tests
# ---------------------------------------------------------------------------


class TestDPAIArenaProvider:
    SAMPLE_TASKS: ClassVar[list[dict]] = _DPAI_SAMPLES

    def _make_provider(self, **kwargs):
        from codeforge.evaluation.providers.dpai_arena import DPAIArenaProvider

        return DPAIArenaProvider(tasks=self.SAMPLE_TASKS, **kwargs)

    def test_name(self) -> None:
        p = self._make_provider()
        assert p.name == "dpai_arena"

    def test_benchmark_type(self) -> None:
        p = self._make_provider()
        assert p.benchmark_type == BenchmarkType.SIMPLE

    def test_capabilities(self) -> None:
        p = self._make_provider()
        caps = p.capabilities
        assert caps.functional_tests is True
        assert caps.llm_judge is True
        assert caps.swe_bench_style is False
        assert caps.sparc_style is False

    @pytest.mark.asyncio
    async def test_load_tasks_from_injected(self) -> None:
        p = self._make_provider()
        tasks = await p.load_tasks()
        assert len(tasks) == 3

    @pytest.mark.asyncio
    async def test_task_count(self) -> None:
        p = self._make_provider()
        assert await p.task_count() == 3

    @pytest.mark.asyncio
    async def test_task_ids(self) -> None:
        p = self._make_provider()
        tasks = await p.load_tasks()
        assert tasks[0].id == "dpai_001"
        assert tasks[1].id == "dpai_002"
        assert tasks[2].id == "dpai_003"

    @pytest.mark.asyncio
    async def test_task_input_contains_question(self) -> None:
        p = self._make_provider()
        tasks = await p.load_tasks()
        assert "fibonacci" in tasks[0].input
        assert "merge_sorted" in tasks[1].input

    @pytest.mark.asyncio
    async def test_expected_output_is_solution(self) -> None:
        p = self._make_provider()
        tasks = await p.load_tasks()
        assert "def fibonacci" in tasks[0].expected_output
        assert "def merge_sorted" in tasks[1].expected_output

    @pytest.mark.asyncio
    async def test_difficulty_preserved(self) -> None:
        p = self._make_provider()
        tasks = await p.load_tasks()
        assert tasks[0].difficulty == "easy"
        assert tasks[1].difficulty == "medium"
        assert tasks[2].difficulty == "hard"

    @pytest.mark.asyncio
    async def test_metadata_contains_tags(self) -> None:
        p = self._make_provider()
        tasks = await p.load_tasks()
        assert "tags" in tasks[0].metadata
        assert "dynamic-programming" in tasks[0].metadata["tags"]

    @pytest.mark.asyncio
    async def test_metadata_contains_language(self) -> None:
        p = self._make_provider()
        tasks = await p.load_tasks()
        assert tasks[0].metadata["language"] == "python"

    @pytest.mark.asyncio
    async def test_test_command_set(self) -> None:
        p = self._make_provider()
        tasks = await p.load_tasks()
        assert tasks[0].test_command == "python solution.py"

    @pytest.mark.asyncio
    async def test_test_cases_in_metadata(self) -> None:
        p = self._make_provider()
        tasks = await p.load_tasks()
        assert "test_cases" in tasks[0].metadata
        assert "fibonacci" in tasks[0].metadata["test_cases"]

    @pytest.mark.asyncio
    async def test_empty_test_cases_handled(self) -> None:
        """Task with empty test_cases should still load without error."""
        p = self._make_provider()
        tasks = await p.load_tasks()
        task3 = tasks[2]
        assert task3.metadata.get("test_cases", "") == ""

    @pytest.mark.asyncio
    async def test_name_derived_from_id(self) -> None:
        p = self._make_provider()
        tasks = await p.load_tasks()
        assert tasks[0].name == "dpai_001"

    def test_convert_task_with_minimal_fields(self) -> None:
        """Task with only required fields should not raise."""
        from codeforge.evaluation.providers.dpai_arena import DPAIArenaProvider

        minimal = {"id": "min_001", "question": "Do something."}
        p = DPAIArenaProvider(tasks=[minimal])
        task = p._convert_task(minimal)
        assert task.id == "min_001"
        assert "Do something" in task.input
        assert task.difficulty == "medium"  # default

    def test_convert_task_missing_id_uses_fallback(self) -> None:
        """Task missing 'id' field should get empty string id."""
        from codeforge.evaluation.providers.dpai_arena import DPAIArenaProvider

        raw = {"question": "Something"}
        p = DPAIArenaProvider(tasks=[raw])
        task = p._convert_task(raw)
        assert task.id == ""

    @pytest.mark.asyncio
    async def test_empty_tasks_list(self) -> None:
        """Provider with empty tasks list should return empty list."""
        from codeforge.evaluation.providers.dpai_arena import DPAIArenaProvider

        p = DPAIArenaProvider(tasks=[])
        tasks = await p.load_tasks()
        assert tasks == []
        assert await p.task_count() == 0


# ---------------------------------------------------------------------------
# Registration tests
# ---------------------------------------------------------------------------


class TestDPAIArenaRegistration:
    def test_registered_in_provider_registry(self) -> None:
        import codeforge.evaluation.providers.dpai_arena  # noqa: F401

        assert "dpai_arena" in list_providers()

    def test_get_provider_returns_class(self) -> None:
        import codeforge.evaluation.providers.dpai_arena  # noqa: F401

        cls = get_provider("dpai_arena")
        instance = cls(tasks=[])
        assert instance.name == "dpai_arena"
        assert instance.benchmark_type == BenchmarkType.SIMPLE
