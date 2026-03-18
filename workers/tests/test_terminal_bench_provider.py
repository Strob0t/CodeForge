"""Tests for Terminal-Bench benchmark provider.

Terminal-Bench evaluates terminal/CLI skills of AI agents. Tasks include
initial filesystem state, a command to execute, and expected filesystem
state after execution.

Tests cover: properties, task loading, filesystem state, edge cases, registration.
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
# Sample Terminal-Bench data for testing
# ---------------------------------------------------------------------------

_TERMINAL_BENCH_SAMPLES: list[dict] = [
    {
        "id": "tb_001",
        "name": "create_directory_structure",
        "instruction": "Create a directory called 'project' with subdirectories 'src' and 'tests'.",
        "initial_files": {},
        "expected_files": {
            "project/src/.gitkeep": "",
            "project/tests/.gitkeep": "",
        },
        "expected_missing": [],
        "difficulty": "easy",
        "tags": "filesystem,mkdir",
    },
    {
        "id": "tb_002",
        "name": "move_and_rename",
        "instruction": "Move 'old_name.txt' to 'new_dir/new_name.txt'.",
        "initial_files": {
            "old_name.txt": "hello world",
        },
        "expected_files": {
            "new_dir/new_name.txt": "hello world",
        },
        "expected_missing": ["old_name.txt"],
        "difficulty": "easy",
        "tags": "filesystem,mv",
    },
    {
        "id": "tb_003",
        "name": "find_and_replace",
        "instruction": "Replace all occurrences of 'foo' with 'bar' in config.txt.",
        "initial_files": {
            "config.txt": "foo=1\nfoo=2\nbaz=3\n",
        },
        "expected_files": {
            "config.txt": "bar=1\nbar=2\nbaz=3\n",
        },
        "expected_missing": [],
        "difficulty": "medium",
        "tags": "sed,text-processing",
    },
    {
        "id": "tb_004",
        "name": "compress_files",
        "instruction": "Create a tar.gz archive called 'backup.tar.gz' containing all .log files, then delete the .log files.",
        "initial_files": {
            "app.log": "log entry 1\n",
            "error.log": "error entry 1\n",
            "config.yaml": "key: value\n",
        },
        "expected_files": {
            "backup.tar.gz": "",
            "config.yaml": "key: value\n",
        },
        "expected_missing": ["app.log", "error.log"],
        "difficulty": "hard",
        "tags": "tar,compression,cleanup",
    },
]


# ---------------------------------------------------------------------------
# Provider tests
# ---------------------------------------------------------------------------


class TestTerminalBenchProvider:
    SAMPLE_TASKS: ClassVar[list[dict]] = _TERMINAL_BENCH_SAMPLES

    def _make_provider(self, **kwargs):
        from codeforge.evaluation.providers.terminal_bench import TerminalBenchProvider

        return TerminalBenchProvider(tasks=self.SAMPLE_TASKS, **kwargs)

    def test_name(self) -> None:
        p = self._make_provider()
        assert p.name == "terminal_bench"

    def test_benchmark_type(self) -> None:
        p = self._make_provider()
        assert p.benchmark_type == BenchmarkType.AGENT

    def test_capabilities(self) -> None:
        p = self._make_provider()
        caps = p.capabilities
        assert caps.functional_tests is True
        assert caps.llm_judge is False
        assert caps.swe_bench_style is False
        assert caps.sparc_style is False

    @pytest.mark.asyncio
    async def test_load_tasks_from_injected(self) -> None:
        p = self._make_provider()
        tasks = await p.load_tasks()
        assert len(tasks) == 4

    @pytest.mark.asyncio
    async def test_task_count(self) -> None:
        p = self._make_provider()
        assert await p.task_count() == 4

    @pytest.mark.asyncio
    async def test_task_ids(self) -> None:
        p = self._make_provider()
        tasks = await p.load_tasks()
        assert tasks[0].id == "tb_001"
        assert tasks[1].id == "tb_002"
        assert tasks[2].id == "tb_003"
        assert tasks[3].id == "tb_004"

    @pytest.mark.asyncio
    async def test_task_input_contains_instruction(self) -> None:
        p = self._make_provider()
        tasks = await p.load_tasks()
        assert "Create a directory" in tasks[0].input
        assert "Move" in tasks[1].input

    @pytest.mark.asyncio
    async def test_initial_files_populated(self) -> None:
        p = self._make_provider()
        tasks = await p.load_tasks()
        # First task has no initial files
        assert tasks[0].initial_files == {}
        # Second task has initial files
        assert "old_name.txt" in tasks[1].initial_files
        assert tasks[1].initial_files["old_name.txt"] == "hello world"

    @pytest.mark.asyncio
    async def test_expected_files_in_metadata(self) -> None:
        p = self._make_provider()
        tasks = await p.load_tasks()
        import json

        expected = json.loads(tasks[1].metadata["expected_files"])
        assert "new_dir/new_name.txt" in expected
        assert expected["new_dir/new_name.txt"] == "hello world"

    @pytest.mark.asyncio
    async def test_expected_missing_in_metadata(self) -> None:
        p = self._make_provider()
        tasks = await p.load_tasks()
        import json

        missing = json.loads(tasks[1].metadata["expected_missing"])
        assert "old_name.txt" in missing

    @pytest.mark.asyncio
    async def test_difficulty_preserved(self) -> None:
        p = self._make_provider()
        tasks = await p.load_tasks()
        assert tasks[0].difficulty == "easy"
        assert tasks[2].difficulty == "medium"
        assert tasks[3].difficulty == "hard"

    @pytest.mark.asyncio
    async def test_tags_in_metadata(self) -> None:
        p = self._make_provider()
        tasks = await p.load_tasks()
        assert "tags" in tasks[0].metadata
        assert "filesystem" in tasks[0].metadata["tags"]

    @pytest.mark.asyncio
    async def test_test_command_is_verify_filesystem(self) -> None:
        p = self._make_provider()
        tasks = await p.load_tasks()
        # Terminal-bench uses filesystem state verification, not a shell command
        assert tasks[0].test_command == "verify_filesystem_state"

    @pytest.mark.asyncio
    async def test_expected_output_contains_expected_state(self) -> None:
        """Expected output should summarize expected filesystem state."""
        p = self._make_provider()
        tasks = await p.load_tasks()
        # Task with expected files should have them mentioned
        assert "new_dir/new_name.txt" in tasks[1].expected_output

    @pytest.mark.asyncio
    async def test_name_field_preserved(self) -> None:
        p = self._make_provider()
        tasks = await p.load_tasks()
        assert tasks[0].name == "create_directory_structure"
        assert tasks[1].name == "move_and_rename"

    def test_convert_task_minimal(self) -> None:
        """Task with only required fields should not raise."""
        from codeforge.evaluation.providers.terminal_bench import TerminalBenchProvider

        minimal = {"id": "min_001", "instruction": "Do something."}
        p = TerminalBenchProvider(tasks=[minimal])
        task = p._convert_task(minimal)
        assert task.id == "min_001"
        assert "Do something" in task.input
        assert task.difficulty == "medium"

    def test_convert_task_missing_id(self) -> None:
        from codeforge.evaluation.providers.terminal_bench import TerminalBenchProvider

        raw = {"instruction": "Do something"}
        p = TerminalBenchProvider(tasks=[raw])
        task = p._convert_task(raw)
        assert task.id == ""

    @pytest.mark.asyncio
    async def test_empty_tasks_list(self) -> None:
        from codeforge.evaluation.providers.terminal_bench import TerminalBenchProvider

        p = TerminalBenchProvider(tasks=[])
        tasks = await p.load_tasks()
        assert tasks == []
        assert await p.task_count() == 0

    @pytest.mark.asyncio
    async def test_complex_initial_files(self) -> None:
        """Task with multiple initial files should preserve all of them."""
        p = self._make_provider()
        tasks = await p.load_tasks()
        task4 = tasks[3]  # compress_files
        assert "app.log" in task4.initial_files
        assert "error.log" in task4.initial_files
        assert "config.yaml" in task4.initial_files


# ---------------------------------------------------------------------------
# Registration tests
# ---------------------------------------------------------------------------


class TestTerminalBenchRegistration:
    def test_registered_in_provider_registry(self) -> None:
        import codeforge.evaluation.providers.terminal_bench  # noqa: F401

        assert "terminal_bench" in list_providers()

    def test_get_provider_returns_class(self) -> None:
        import codeforge.evaluation.providers.terminal_bench  # noqa: F401

        cls = get_provider("terminal_bench")
        instance = cls(tasks=[])
        assert instance.name == "terminal_bench"
        assert instance.benchmark_type == BenchmarkType.AGENT
