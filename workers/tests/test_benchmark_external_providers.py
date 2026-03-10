"""Tests for Phase 26E: External Benchmark Providers.

Tests cover:
- Dataset cache system (download, caching, checksum, JSONL/JSON loading)
- HumanEval provider (task conversion, difficulty estimation, test harness)
- MBPP provider (task conversion, test assertions, splits)
- BigCodeBench provider (task conversion, library metadata)
- CRUXEval provider (output/input prediction modes)
- LiveCodeBench provider (date filtering, task conversion)
- Provider registration (all 5 providers registered correctly)
"""

from __future__ import annotations

import hashlib
from typing import TYPE_CHECKING, ClassVar
from unittest.mock import AsyncMock, patch

if TYPE_CHECKING:
    from pathlib import Path

import pytest

from codeforge.evaluation.cache import (
    _verify_checksum,
    get_cache_dir,
    get_cached_path,
    load_json,
    load_jsonl,
)
from codeforge.evaluation.providers.base import (
    BenchmarkType,
    get_provider,
    list_providers,
)

# ---------------------------------------------------------------------------
# Cache tests
# ---------------------------------------------------------------------------


class TestGetCacheDir:
    def test_creates_directory(self, tmp_path: Path) -> None:
        result = get_cache_dir("test_provider", str(tmp_path))
        assert result.exists()
        assert result.name == "test_provider"

    def test_nested_under_base(self, tmp_path: Path) -> None:
        result = get_cache_dir("humaneval", str(tmp_path))
        assert result == tmp_path / "humaneval"

    def test_idempotent(self, tmp_path: Path) -> None:
        r1 = get_cache_dir("p1", str(tmp_path))
        r2 = get_cache_dir("p1", str(tmp_path))
        assert r1 == r2


class TestGetCachedPath:
    def test_returns_none_when_missing(self, tmp_path: Path) -> None:
        result = get_cached_path("provider", "missing.jsonl", str(tmp_path))
        assert result is None

    def test_returns_path_when_exists(self, tmp_path: Path) -> None:
        cache_dir = tmp_path / "provider"
        cache_dir.mkdir()
        f = cache_dir / "data.jsonl"
        f.write_text('{"a": 1}\n')
        result = get_cached_path("provider", "data.jsonl", str(tmp_path))
        assert result == f

    def test_returns_none_for_empty_file(self, tmp_path: Path) -> None:
        cache_dir = tmp_path / "provider"
        cache_dir.mkdir()
        f = cache_dir / "empty.jsonl"
        f.write_text("")
        result = get_cached_path("provider", "empty.jsonl", str(tmp_path))
        assert result is None


class TestVerifyChecksum:
    def test_correct_checksum(self, tmp_path: Path) -> None:
        f = tmp_path / "test.txt"
        content = b"hello world"
        f.write_bytes(content)
        expected = hashlib.sha256(content).hexdigest()
        assert _verify_checksum(f, expected) is True

    def test_wrong_checksum(self, tmp_path: Path) -> None:
        f = tmp_path / "test.txt"
        f.write_bytes(b"hello world")
        assert _verify_checksum(f, "0000") is False


class TestLoadJsonl:
    def test_loads_records(self, tmp_path: Path) -> None:
        f = tmp_path / "data.jsonl"
        f.write_text('{"id": 1}\n{"id": 2}\n')
        records = load_jsonl(f)
        assert len(records) == 2
        assert records[0]["id"] == 1
        assert records[1]["id"] == 2

    def test_skips_blank_lines(self, tmp_path: Path) -> None:
        f = tmp_path / "data.jsonl"
        f.write_text('{"a": 1}\n\n{"b": 2}\n\n')
        records = load_jsonl(f)
        assert len(records) == 2

    def test_empty_file(self, tmp_path: Path) -> None:
        f = tmp_path / "empty.jsonl"
        f.write_text("")
        records = load_jsonl(f)
        assert records == []


class TestLoadJson:
    def test_loads_list(self, tmp_path: Path) -> None:
        f = tmp_path / "data.json"
        f.write_text('[{"id": 1}, {"id": 2}]')
        data = load_json(f)
        assert isinstance(data, list)
        assert len(data) == 2

    def test_loads_dict(self, tmp_path: Path) -> None:
        f = tmp_path / "data.json"
        f.write_text('{"key": "value"}')
        data = load_json(f)
        assert isinstance(data, dict)
        assert data["key"] == "value"


class TestDownloadHfDataset:
    """Tests for download_hf_dataset (HuggingFace rows API with pagination)."""

    @pytest.mark.asyncio
    async def test_caches_result_as_jsonl(self, tmp_path: Path) -> None:
        from unittest.mock import MagicMock

        from codeforge.evaluation.cache import download_hf_dataset

        page_response = {
            "rows": [
                {"row": {"instance_id": "test/1", "repo": "test/repo", "base_commit": "abc"}},
                {"row": {"instance_id": "test/2", "repo": "test/repo", "base_commit": "def"}},
            ]
        }

        mock_response = MagicMock()
        mock_response.json.return_value = page_response
        mock_response.raise_for_status = lambda: None

        mock_client = AsyncMock()
        mock_client.get = AsyncMock(return_value=mock_response)
        mock_client.__aenter__ = AsyncMock(return_value=mock_client)
        mock_client.__aexit__ = AsyncMock(return_value=False)

        with patch("httpx.AsyncClient", return_value=mock_client):
            result = await download_hf_dataset(
                dataset="test/dataset",
                split="test",
                provider_name="test_hf",
                filename="data.jsonl",
                base_dir=str(tmp_path),
            )

        assert result.exists()
        records = load_jsonl(result)
        assert len(records) == 2
        assert records[0]["instance_id"] == "test/1"
        assert records[1]["instance_id"] == "test/2"

    @pytest.mark.asyncio
    async def test_uses_cache_when_available(self, tmp_path: Path) -> None:
        from codeforge.evaluation.cache import download_hf_dataset

        cache_dir = tmp_path / "test_hf"
        cache_dir.mkdir()
        cached_file = cache_dir / "data.jsonl"
        cached_file.write_text('{"cached": true}\n')

        result = await download_hf_dataset(
            dataset="test/dataset",
            split="test",
            provider_name="test_hf",
            filename="data.jsonl",
            base_dir=str(tmp_path),
        )
        assert result == cached_file
        records = load_jsonl(result)
        assert records[0]["cached"] is True

    @pytest.mark.asyncio
    async def test_paginates_correctly(self, tmp_path: Path) -> None:
        from unittest.mock import MagicMock

        from codeforge.evaluation.cache import download_hf_dataset

        # First page: full (100 rows), second page: partial (1 row)
        page1_rows = [{"row": {"id": i}} for i in range(100)]
        page2_rows = [{"row": {"id": 100}}]

        resp1 = MagicMock()
        resp1.json.return_value = {"rows": page1_rows}
        resp1.raise_for_status = lambda: None

        resp2 = MagicMock()
        resp2.json.return_value = {"rows": page2_rows}
        resp2.raise_for_status = lambda: None

        mock_client = AsyncMock()
        mock_client.get = AsyncMock(side_effect=[resp1, resp2])
        mock_client.__aenter__ = AsyncMock(return_value=mock_client)
        mock_client.__aexit__ = AsyncMock(return_value=False)

        with patch("httpx.AsyncClient", return_value=mock_client):
            result = await download_hf_dataset(
                dataset="test/dataset",
                split="test",
                provider_name="test_hf_pag",
                filename="paginated.jsonl",
                base_dir=str(tmp_path),
            )

        records = load_jsonl(result)
        assert len(records) == 101
        assert records[100]["id"] == 100


class TestDownloadDataset:
    @pytest.mark.asyncio
    async def test_uses_cache_when_available(self, tmp_path: Path) -> None:
        from codeforge.evaluation.cache import download_dataset

        # Pre-populate cache
        cache_dir = tmp_path / "test_prov"
        cache_dir.mkdir()
        cached_file = cache_dir / "data.jsonl"
        cached_file.write_text('{"cached": true}\n')

        result = await download_dataset(
            url="https://example.com/does-not-exist",
            provider_name="test_prov",
            filename="data.jsonl",
            base_dir=str(tmp_path),
        )
        assert result == cached_file

    @pytest.mark.asyncio
    async def test_checksum_mismatch_redownloads(self, tmp_path: Path) -> None:
        from codeforge.evaluation.cache import download_dataset

        # Pre-populate cache with wrong content
        cache_dir = tmp_path / "test_prov"
        cache_dir.mkdir()
        cached_file = cache_dir / "data.jsonl"
        cached_file.write_text("old content\n")

        new_content = b'{"fresh": true}\n'
        expected_sha = hashlib.sha256(new_content).hexdigest()

        mock_response = AsyncMock()
        mock_response.content = new_content
        mock_response.raise_for_status = lambda: None

        mock_client = AsyncMock()
        mock_client.get = AsyncMock(return_value=mock_response)
        mock_client.__aenter__ = AsyncMock(return_value=mock_client)
        mock_client.__aexit__ = AsyncMock(return_value=False)

        with patch("httpx.AsyncClient", return_value=mock_client):
            result = await download_dataset(
                url="https://example.com/data.jsonl",
                provider_name="test_prov",
                filename="data.jsonl",
                expected_sha256=expected_sha,
                base_dir=str(tmp_path),
            )
        assert result.read_bytes() == new_content


# ---------------------------------------------------------------------------
# HumanEval provider tests
# ---------------------------------------------------------------------------


class TestHumanEvalProvider:
    SAMPLE_TASKS: ClassVar[list[dict]] = [
        {
            "task_id": "HumanEval/0",
            "prompt": 'def has_close_elements(numbers, threshold):\n    """Check if any two numbers are close."""\n',
            "canonical_solution": "    for i, n1 in enumerate(numbers):\n        for n2 in numbers[i+1:]:\n            if abs(n1 - n2) < threshold:\n                return True\n    return False\n",
            "test": "def check(candidate):\n    assert candidate([1.0, 2.0], 0.5) == False\n    assert candidate([1.0, 1.1], 0.2) == True\n",
            "entry_point": "has_close_elements",
        },
        {
            "task_id": "HumanEval/1",
            "prompt": 'def add(a, b):\n    """Add two numbers."""\n',
            "canonical_solution": "    return a + b\n",
            "test": "def check(candidate):\n    assert candidate(1, 2) == 3\n",
            "entry_point": "add",
        },
    ]

    def _make_provider(self) -> object:
        from codeforge.evaluation.providers.humaneval import HumanEvalProvider

        return HumanEvalProvider(tasks=self.SAMPLE_TASKS)

    def test_properties(self) -> None:
        p = self._make_provider()
        assert p.name == "humaneval"
        assert p.benchmark_type == BenchmarkType.SIMPLE
        assert p.capabilities.functional_tests is True
        assert p.capabilities.llm_judge is True

    @pytest.mark.asyncio
    async def test_load_tasks(self) -> None:
        p = self._make_provider()
        tasks = await p.load_tasks()
        assert len(tasks) == 2
        assert tasks[0].id == "HumanEval/0"
        assert "has_close_elements" in tasks[0].input
        assert tasks[0].test_command == "python solution.py"

    @pytest.mark.asyncio
    async def test_task_count(self) -> None:
        p = self._make_provider()
        assert await p.task_count() == 2

    @pytest.mark.asyncio
    async def test_difficulty_estimation(self) -> None:
        p = self._make_provider()
        tasks = await p.load_tasks()
        # "return a + b" is 1 line → easy
        assert tasks[1].difficulty == "easy"

    @pytest.mark.asyncio
    async def test_metadata_contains_test_harness(self) -> None:
        p = self._make_provider()
        tasks = await p.load_tasks()
        assert "test_harness" in tasks[0].metadata
        assert "entry_point" in tasks[0].metadata
        assert tasks[0].metadata["language"] == "python"

    @pytest.mark.asyncio
    async def test_entry_point_in_metadata(self) -> None:
        p = self._make_provider()
        tasks = await p.load_tasks()
        assert tasks[0].metadata["entry_point"] == "has_close_elements"

    def test_registration(self) -> None:
        assert "humaneval" in list_providers()
        cls = get_provider("humaneval")
        assert cls is not None


# ---------------------------------------------------------------------------
# MBPP provider tests
# ---------------------------------------------------------------------------


class TestMBPPProvider:
    SAMPLE_TASKS: ClassVar[list[dict]] = [
        {
            "task_id": 601,
            "text": "Write a function to find the longest common prefix.",
            "code": "def longest_prefix(strs):\n    if not strs: return ''\n    prefix = strs[0]\n    for s in strs[1:]:\n        while not s.startswith(prefix):\n            prefix = prefix[:-1]\n    return prefix\n",
            "test_list": [
                "assert longest_prefix(['flower','flow','flight']) == 'fl'",
                "assert longest_prefix(['dog','racecar','car']) == ''",
                "assert longest_prefix(['a']) == 'a'",
            ],
            "test_setup_code": "",
        },
        {
            "task_id": 602,
            "text": "Write a function to add two numbers.",
            "code": "def add(a, b):\n    return a + b\n",
            "test_list": ["assert add(1, 2) == 3"],
            "test_setup_code": "",
        },
    ]

    def _make_provider(self) -> object:
        from codeforge.evaluation.providers.mbpp import MBPPProvider

        return MBPPProvider(tasks=self.SAMPLE_TASKS)

    def test_properties(self) -> None:
        p = self._make_provider()
        assert p.name == "mbpp"
        assert p.benchmark_type == BenchmarkType.SIMPLE

    @pytest.mark.asyncio
    async def test_load_tasks(self) -> None:
        p = self._make_provider()
        tasks = await p.load_tasks()
        assert len(tasks) == 2
        assert tasks[0].id == "mbpp_601"
        assert "longest common prefix" in tasks[0].input

    @pytest.mark.asyncio
    async def test_test_assertions_in_metadata(self) -> None:
        p = self._make_provider()
        tasks = await p.load_tasks()
        assert "test_assertions" in tasks[0].metadata
        assert "longest_prefix" in tasks[0].metadata["test_assertions"]

    @pytest.mark.asyncio
    async def test_difficulty_by_test_length(self) -> None:
        p = self._make_provider()
        tasks = await p.load_tasks()
        # Short single assertion → easy
        assert tasks[1].difficulty == "easy"

    def test_registration(self) -> None:
        assert "mbpp" in list_providers()


# ---------------------------------------------------------------------------
# BigCodeBench provider tests
# ---------------------------------------------------------------------------


class TestBigCodeBenchProvider:
    SAMPLE_TASKS: ClassVar[list[dict]] = [
        {
            "task_id": "BCB/0",
            "instruct_prompt": "Write a function that reads a CSV and computes statistics.",
            "complete_prompt": "",
            "canonical_solution": "import pandas as pd\ndef solve():\n    pass\n",
            "test": "def test():\n    assert True\n",
            "libs": ["pandas", "numpy"],
        },
    ]

    def _make_provider(self) -> object:
        from codeforge.evaluation.providers.bigcodebench import BigCodeBenchProvider

        return BigCodeBenchProvider(tasks=self.SAMPLE_TASKS)

    def test_properties(self) -> None:
        p = self._make_provider()
        assert p.name == "bigcodebench"
        assert p.benchmark_type == BenchmarkType.SIMPLE
        assert p.capabilities.functional_tests is True

    @pytest.mark.asyncio
    async def test_load_tasks(self) -> None:
        p = self._make_provider()
        tasks = await p.load_tasks()
        assert len(tasks) == 1
        assert tasks[0].id == "BCB/0"
        assert tasks[0].difficulty == "hard"

    @pytest.mark.asyncio
    async def test_libs_in_metadata(self) -> None:
        p = self._make_provider()
        tasks = await p.load_tasks()
        assert "pandas" in tasks[0].metadata["libs"]
        assert "numpy" in tasks[0].metadata["libs"]

    def test_registration(self) -> None:
        assert "bigcodebench" in list_providers()


# ---------------------------------------------------------------------------
# CRUXEval provider tests
# ---------------------------------------------------------------------------


class TestCRUXEvalProvider:
    SAMPLE_TASKS: ClassVar[list[dict]] = [
        {
            "id": "crux_001",
            "code": "def f(x):\n    return x * 2",
            "input": "5",
            "output": "10",
        },
        {
            "id": "crux_002",
            "code": "def f(s):\n    return s[::-1]",
            "input": "'hello'",
            "output": "'olleh'",
        },
    ]

    def _make_provider(self, mode: str = "output_prediction") -> object:
        from codeforge.evaluation.providers.cruxeval import CRUXEvalProvider

        return CRUXEvalProvider(tasks=self.SAMPLE_TASKS, mode=mode)

    def test_properties(self) -> None:
        p = self._make_provider()
        assert p.name == "cruxeval"
        assert p.benchmark_type == BenchmarkType.SIMPLE

    @pytest.mark.asyncio
    async def test_output_prediction_mode(self) -> None:
        p = self._make_provider("output_prediction")
        tasks = await p.load_tasks()
        assert len(tasks) == 2
        assert "predict the exact output" in tasks[0].input
        assert tasks[0].expected_output == "10"

    @pytest.mark.asyncio
    async def test_input_prediction_mode(self) -> None:
        p = self._make_provider("input_prediction")
        tasks = await p.load_tasks()
        assert "predict the input" in tasks[0].input
        assert tasks[0].expected_output == "5"

    @pytest.mark.asyncio
    async def test_code_in_metadata(self) -> None:
        p = self._make_provider()
        tasks = await p.load_tasks()
        assert "def f(x)" in tasks[0].metadata["code"]
        assert tasks[0].metadata["mode"] == "output_prediction"

    @pytest.mark.asyncio
    async def test_test_harness_in_metadata(self) -> None:
        p = self._make_provider()
        tasks = await p.load_tasks()
        assert "test_harness" in tasks[0].metadata
        assert "ALL TESTS PASSED" in tasks[0].metadata["test_harness"]

    def test_registration(self) -> None:
        assert "cruxeval" in list_providers()


# ---------------------------------------------------------------------------
# LiveCodeBench provider tests
# ---------------------------------------------------------------------------


class TestLiveCodeBenchProvider:
    SAMPLE_TASKS: ClassVar[list[dict]] = [
        {
            "question_id": "lcb_100",
            "question_title": "Two Sum",
            "question_content": "Given an array, find two numbers that add up to target.",
            "difficulty": "Easy",
            "platform": "leetcode",
            "contest_date": "2024-01-15",
            "starter_code": "class Solution:\n    def twoSum(self, nums, target):\n        pass",
            "public_test_cases": "[[2,7,11,15], 9] -> [0,1]",
        },
        {
            "question_id": "lcb_200",
            "question_title": "Hard Graph Problem",
            "question_content": "Solve a complex graph problem.",
            "difficulty": "Hard",
            "platform": "codeforces",
            "contest_date": "2025-06-01",
        },
        {
            "question_id": "lcb_300",
            "question_title": "Medium DP",
            "question_content": "Dynamic programming problem.",
            "difficulty": "Medium",
            "contest_date": "2024-06-15",
        },
    ]

    def _make_provider(self, **kwargs) -> object:
        from codeforge.evaluation.providers.livecodebench import LiveCodeBenchProvider

        return LiveCodeBenchProvider(tasks=self.SAMPLE_TASKS, **kwargs)

    def test_properties(self) -> None:
        p = self._make_provider()
        assert p.name == "livecodebench"
        assert p.benchmark_type == BenchmarkType.SIMPLE

    @pytest.mark.asyncio
    async def test_load_all_tasks(self) -> None:
        p = self._make_provider()
        tasks = await p.load_tasks()
        assert len(tasks) == 3

    @pytest.mark.asyncio
    async def test_date_filter_after(self) -> None:
        p = self._make_provider(after_date="2025-01-01")
        tasks = await p.load_tasks()
        assert len(tasks) == 1
        assert tasks[0].id == "lcb_lcb_200"

    @pytest.mark.asyncio
    async def test_date_filter_before(self) -> None:
        p = self._make_provider(before_date="2024-03-01")
        tasks = await p.load_tasks()
        assert len(tasks) == 1
        assert tasks[0].id == "lcb_lcb_100"

    @pytest.mark.asyncio
    async def test_date_filter_range(self) -> None:
        p = self._make_provider(after_date="2024-01-01", before_date="2024-12-31")
        tasks = await p.load_tasks()
        assert len(tasks) == 2  # lcb_100 and lcb_300

    @pytest.mark.asyncio
    async def test_difficulty_preserved(self) -> None:
        p = self._make_provider()
        tasks = await p.load_tasks()
        difficulties = {t.id: t.difficulty for t in tasks}
        assert difficulties["lcb_lcb_100"] == "easy"
        assert difficulties["lcb_lcb_200"] == "hard"

    @pytest.mark.asyncio
    async def test_starter_code_in_prompt(self) -> None:
        p = self._make_provider()
        tasks = await p.load_tasks()
        task = next(t for t in tasks if t.id == "lcb_lcb_100")
        assert "twoSum" in task.input

    @pytest.mark.asyncio
    async def test_metadata_platform(self) -> None:
        p = self._make_provider()
        tasks = await p.load_tasks()
        task = next(t for t in tasks if t.id == "lcb_lcb_100")
        assert task.metadata["platform"] == "leetcode"

    def test_registration(self) -> None:
        assert "livecodebench" in list_providers()


# ---------------------------------------------------------------------------
# Provider registry integration tests
# ---------------------------------------------------------------------------


class TestProviderRegistry:
    def test_all_external_providers_registered(self) -> None:
        # Force import of all provider modules
        import codeforge.evaluation.providers.bigcodebench
        import codeforge.evaluation.providers.cruxeval
        import codeforge.evaluation.providers.humaneval
        import codeforge.evaluation.providers.livecodebench
        import codeforge.evaluation.providers.mbpp  # noqa: F401

        providers = list_providers()
        for name in ("humaneval", "mbpp", "bigcodebench", "cruxeval", "livecodebench"):
            assert name in providers, f"Provider {name!r} not registered"

    def test_get_provider_returns_class(self) -> None:
        import codeforge.evaluation.providers.humaneval  # noqa: F401

        cls = get_provider("humaneval")
        instance = cls(tasks=[])
        assert instance.name == "humaneval"

    def test_all_providers_have_correct_capabilities(self) -> None:
        import codeforge.evaluation.providers.bigcodebench
        import codeforge.evaluation.providers.cruxeval
        import codeforge.evaluation.providers.humaneval
        import codeforge.evaluation.providers.livecodebench
        import codeforge.evaluation.providers.mbpp  # noqa: F401

        for name in ("humaneval", "mbpp", "bigcodebench", "cruxeval", "livecodebench"):
            cls = get_provider(name)
            instance = cls(tasks=[])
            assert instance.capabilities.functional_tests is True
            assert instance.capabilities.llm_judge is True
