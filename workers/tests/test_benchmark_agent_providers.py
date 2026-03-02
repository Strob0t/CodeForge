"""Tests for Phase 26F: Agent & Multi-Language External Benchmark Providers.

Tests cover:
- SWEBenchProvider (task conversion, difficulty estimation, repo metadata)
- SWEBenchLiteProvider (properties, task conversion)
- SWEBenchVerifiedProvider (properties, task conversion)
- SPARCBenchProvider (SPARC capabilities, SPARC instruction, metadata)
- AiderPolyglotProvider (multi-language, language filtering, initial_files)
- Provider registration (all providers registered correctly)
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
# Sample SWE-bench data shared by all SWE-bench variants
# ---------------------------------------------------------------------------

_SWEBENCH_SAMPLES: list[dict] = [
    {
        "instance_id": "django__django-11099",
        "repo": "django/django",
        "base_commit": "abc123def456",
        "problem_statement": "UsernameValidator allows trailing newlines in usernames.",
        "hints_text": "Check ASCIIUsernameValidator regex.",
        "test_patch": "diff --git a/tests/auth.py b/tests/auth.py\n+    def test_no_newline(self):\n+        pass\n",
        "patch": "diff --git a/django/contrib/auth/validators.py\n- regex = r'^[\\w.@+-]+$'\n+ regex = r'^[\\w.@+-]+\\Z'\n",
        "created_at": "2023-05-01T12:00:00Z",
        "version": "3.0",
    },
    {
        "instance_id": "scikit-learn__scikit-learn-25638",
        "repo": "scikit-learn/scikit-learn",
        "base_commit": "def789ghi012",
        "problem_statement": "Ridge regression with sample_weight raises ValueError.",
        "hints_text": "",
        "test_patch": "diff --git a/tests/test_ridge.py\n+    def test_sample_weight(self):\n+        pass\n",
        "patch": "--- a/sklearn/linear_model/_ridge.py\n+++ b/sklearn/linear_model/_ridge.py\n@@ -100,6 +100,10 @@\n-    y = y.ravel()\n+    if sample_weight is not None:\n+        sample_weight = np.asarray(sample_weight)\n+    y = y.ravel()\n",
        "created_at": "2024-01-15T10:00:00Z",
        "version": "1.3",
    },
]


# ---------------------------------------------------------------------------
# SWE-bench provider tests
# ---------------------------------------------------------------------------


class TestSWEBenchProvider:
    SAMPLE_TASKS: ClassVar[list[dict]] = _SWEBENCH_SAMPLES

    def _make_provider(self):
        from codeforge.evaluation.providers.swebench import SWEBenchProvider

        return SWEBenchProvider(tasks=self.SAMPLE_TASKS)

    def test_properties(self) -> None:
        p = self._make_provider()
        assert p.name == "swebench"
        assert p.benchmark_type == BenchmarkType.AGENT
        assert p.capabilities.functional_tests is True
        assert p.capabilities.swe_bench_style is True

    @pytest.mark.asyncio
    async def test_load_tasks(self) -> None:
        p = self._make_provider()
        tasks = await p.load_tasks()
        assert len(tasks) == 2
        assert tasks[0].id == "django__django-11099"
        assert "UsernameValidator" in tasks[0].input

    @pytest.mark.asyncio
    async def test_task_count(self) -> None:
        p = self._make_provider()
        assert await p.task_count() == 2

    @pytest.mark.asyncio
    async def test_repo_url_constructed(self) -> None:
        p = self._make_provider()
        tasks = await p.load_tasks()
        assert tasks[0].repo_url == "https://github.com/django/django.git"
        assert tasks[0].repo_commit == "abc123def456"

    @pytest.mark.asyncio
    async def test_difficulty_by_patch_size(self) -> None:
        p = self._make_provider()
        tasks = await p.load_tasks()
        # Small patch → easy
        assert tasks[0].difficulty == "easy"

    @pytest.mark.asyncio
    async def test_metadata_contains_patches(self) -> None:
        p = self._make_provider()
        tasks = await p.load_tasks()
        assert "test_patch" in tasks[0].metadata
        assert "gold_patch" in tasks[0].metadata
        assert tasks[0].metadata["eval_method"] == "swe_bench"

    @pytest.mark.asyncio
    async def test_test_command_when_test_patch_present(self) -> None:
        p = self._make_provider()
        tasks = await p.load_tasks()
        assert tasks[0].test_command != ""

    @pytest.mark.asyncio
    async def test_hints_in_instruction(self) -> None:
        p = self._make_provider()
        tasks = await p.load_tasks()
        assert "ASCIIUsernameValidator" in tasks[0].input
        # Second task has no hints
        assert "Hints" not in tasks[1].input

    def test_registration(self) -> None:
        assert "swebench" in list_providers()


class TestSWEBenchLiteProvider:
    SAMPLE_TASKS: ClassVar[list[dict]] = _SWEBENCH_SAMPLES[:1]

    def _make_provider(self):
        from codeforge.evaluation.providers.swebench import SWEBenchLiteProvider

        return SWEBenchLiteProvider(tasks=self.SAMPLE_TASKS)

    def test_properties(self) -> None:
        p = self._make_provider()
        assert p.name == "swebench_lite"
        assert p.benchmark_type == BenchmarkType.AGENT

    @pytest.mark.asyncio
    async def test_load_tasks(self) -> None:
        p = self._make_provider()
        tasks = await p.load_tasks()
        assert len(tasks) == 1

    def test_registration(self) -> None:
        assert "swebench_lite" in list_providers()


class TestSWEBenchVerifiedProvider:
    SAMPLE_TASKS: ClassVar[list[dict]] = _SWEBENCH_SAMPLES[:1]

    def _make_provider(self):
        from codeforge.evaluation.providers.swebench import SWEBenchVerifiedProvider

        return SWEBenchVerifiedProvider(tasks=self.SAMPLE_TASKS)

    def test_properties(self) -> None:
        p = self._make_provider()
        assert p.name == "swebench_verified"
        assert p.benchmark_type == BenchmarkType.AGENT

    @pytest.mark.asyncio
    async def test_load_tasks(self) -> None:
        p = self._make_provider()
        tasks = await p.load_tasks()
        assert len(tasks) == 1

    def test_registration(self) -> None:
        assert "swebench_verified" in list_providers()


# ---------------------------------------------------------------------------
# SPARC-bench provider tests
# ---------------------------------------------------------------------------


class TestSPARCBenchProvider:
    SAMPLE_TASKS: ClassVar[list[dict]] = _SWEBENCH_SAMPLES

    def _make_provider(self):
        from codeforge.evaluation.providers.sparcbench import SPARCBenchProvider

        return SPARCBenchProvider(tasks=self.SAMPLE_TASKS)

    def test_properties(self) -> None:
        p = self._make_provider()
        assert p.name == "sparcbench"
        assert p.benchmark_type == BenchmarkType.AGENT

    def test_sparc_capabilities(self) -> None:
        p = self._make_provider()
        caps = p.capabilities
        assert caps.functional_tests is True
        assert caps.sparc_style is True
        assert caps.swe_bench_style is True
        assert caps.llm_judge is True

    @pytest.mark.asyncio
    async def test_load_tasks(self) -> None:
        p = self._make_provider()
        tasks = await p.load_tasks()
        assert len(tasks) == 2
        assert tasks[0].id.startswith("sparc_")

    @pytest.mark.asyncio
    async def test_sparc_instruction_contains_methodology(self) -> None:
        p = self._make_provider()
        tasks = await p.load_tasks()
        assert "SPARC methodology" in tasks[0].input
        assert "Specification" in tasks[0].input

    @pytest.mark.asyncio
    async def test_metadata_eval_method(self) -> None:
        p = self._make_provider()
        tasks = await p.load_tasks()
        assert tasks[0].metadata["eval_method"] == "sparc"
        assert "sparc" in tasks[0].metadata["evaluators"]

    @pytest.mark.asyncio
    async def test_repo_url_set(self) -> None:
        p = self._make_provider()
        tasks = await p.load_tasks()
        assert "github.com/django/django" in tasks[0].repo_url

    def test_registration(self) -> None:
        assert "sparcbench" in list_providers()


# ---------------------------------------------------------------------------
# Aider Polyglot provider tests
# ---------------------------------------------------------------------------


class TestAiderPolyglotProvider:
    SAMPLE_TASKS: ClassVar[list[dict]] = [
        {
            "task_id": "poly_001",
            "name": "add_function",
            "language": "python",
            "instruction": "Add a multiply function to the module.",
            "initial_code": "def add(a, b):\n    return a + b\n",
            "test_code": "from solution import multiply\nassert multiply(3, 4) == 12\n",
            "expected_code": "def add(a, b):\n    return a + b\n\ndef multiply(a, b):\n    return a * b\n",
            "filename": "solution.py",
            "test_filename": "test_solution.py",
            "difficulty": "easy",
        },
        {
            "task_id": "poly_002",
            "name": "fix_bug",
            "language": "javascript",
            "instruction": "Fix the off-by-one error in the loop.",
            "initial_code": "function sum(arr) { let s=0; for(let i=0; i<=arr.length; i++) s+=arr[i]; return s; }",
            "test_code": "const {sum} = require('./solution');\nconsole.assert(sum([1,2,3]) === 6);",
            "expected_code": "function sum(arr) { let s=0; for(let i=0; i<arr.length; i++) s+=arr[i]; return s; }",
            "filename": "solution.js",
            "difficulty": "easy",
        },
        {
            "task_id": "poly_003",
            "name": "implement_sort",
            "language": "go",
            "instruction": "Implement a bubble sort function.",
            "initial_code": "package main\n\nfunc BubbleSort(arr []int) []int {\n    return arr\n}\n",
            "test_code": "",
            "expected_code": "",
            "filename": "sort.go",
            "difficulty": "medium",
        },
    ]

    def _make_provider(self, **kwargs):
        from codeforge.evaluation.providers.aider_polyglot import AiderPolyglotProvider

        return AiderPolyglotProvider(tasks=self.SAMPLE_TASKS, **kwargs)

    def test_properties(self) -> None:
        p = self._make_provider()
        assert p.name == "aider_polyglot"
        assert p.benchmark_type == BenchmarkType.AGENT
        assert p.capabilities.functional_tests is True

    @pytest.mark.asyncio
    async def test_load_all_tasks(self) -> None:
        p = self._make_provider()
        tasks = await p.load_tasks()
        assert len(tasks) == 3

    @pytest.mark.asyncio
    async def test_language_filter_python(self) -> None:
        p = self._make_provider(language="python")
        tasks = await p.load_tasks()
        assert len(tasks) == 1
        assert tasks[0].metadata["language"] == "python"

    @pytest.mark.asyncio
    async def test_language_filter_javascript(self) -> None:
        p = self._make_provider(language="javascript")
        tasks = await p.load_tasks()
        assert len(tasks) == 1
        assert tasks[0].metadata["language"] == "javascript"

    @pytest.mark.asyncio
    async def test_initial_files_populated(self) -> None:
        p = self._make_provider(language="python")
        tasks = await p.load_tasks()
        task = tasks[0]
        assert "solution.py" in task.initial_files
        assert "test_solution.py" in task.initial_files
        assert "def add" in task.initial_files["solution.py"]

    @pytest.mark.asyncio
    async def test_instruction_references_filename(self) -> None:
        p = self._make_provider()
        tasks = await p.load_tasks()
        assert "`solution.py`" in tasks[0].input
        assert "`solution.js`" in tasks[1].input

    @pytest.mark.asyncio
    async def test_test_command_by_language(self) -> None:
        p = self._make_provider()
        tasks = await p.load_tasks()
        # Python task → pytest
        py_task = next(t for t in tasks if t.metadata["language"] == "python")
        assert "pytest" in py_task.test_command
        # Go task → go test
        go_task = next(t for t in tasks if t.metadata["language"] == "go")
        assert "go test" in go_task.test_command

    @pytest.mark.asyncio
    async def test_difficulty_preserved(self) -> None:
        p = self._make_provider()
        tasks = await p.load_tasks()
        difficulties = {t.metadata["language"]: t.difficulty for t in tasks}
        assert difficulties["python"] == "easy"
        assert difficulties["go"] == "medium"

    @pytest.mark.asyncio
    async def test_task_count_with_filter(self) -> None:
        p = self._make_provider(language="go")
        assert await p.task_count() == 1

    def test_registration(self) -> None:
        assert "aider_polyglot" in list_providers()


# ---------------------------------------------------------------------------
# Cross-provider registry integration tests
# ---------------------------------------------------------------------------


class TestPhase26FProviderRegistry:
    def test_all_agent_providers_registered(self) -> None:
        import codeforge.evaluation.providers.aider_polyglot
        import codeforge.evaluation.providers.sparcbench
        import codeforge.evaluation.providers.swebench  # noqa: F401

        providers = list_providers()
        expected = [
            "swebench",
            "swebench_lite",
            "swebench_verified",
            "sparcbench",
            "aider_polyglot",
        ]
        for name in expected:
            assert name in providers, f"Provider {name!r} not registered"

    def test_all_agent_providers_are_agent_type(self) -> None:
        import codeforge.evaluation.providers.aider_polyglot
        import codeforge.evaluation.providers.sparcbench
        import codeforge.evaluation.providers.swebench  # noqa: F401

        for name in ("swebench", "swebench_lite", "swebench_verified", "sparcbench", "aider_polyglot"):
            cls = get_provider(name)
            instance = cls(tasks=[])
            assert instance.benchmark_type == BenchmarkType.AGENT, f"{name} should be AGENT type"

    def test_all_agent_providers_have_functional_tests(self) -> None:
        import codeforge.evaluation.providers.aider_polyglot
        import codeforge.evaluation.providers.sparcbench
        import codeforge.evaluation.providers.swebench  # noqa: F401

        for name in ("swebench", "swebench_lite", "swebench_verified", "sparcbench", "aider_polyglot"):
            cls = get_provider(name)
            instance = cls(tasks=[])
            assert instance.capabilities.functional_tests is True, f"{name} should support functional_tests"

    def test_only_sparc_has_sparc_style(self) -> None:
        import codeforge.evaluation.providers.aider_polyglot
        import codeforge.evaluation.providers.sparcbench
        import codeforge.evaluation.providers.swebench  # noqa: F401

        sparc_cls = get_provider("sparcbench")
        assert sparc_cls(tasks=[]).capabilities.sparc_style is True

        aider_cls = get_provider("aider_polyglot")
        assert aider_cls(tasks=[]).capabilities.sparc_style is False
