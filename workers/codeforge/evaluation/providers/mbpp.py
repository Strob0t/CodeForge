"""MBPP (Mostly Basic Python Problems) benchmark provider.

974 crowd-sourced Python programming problems, each with a task
description and 3 test assertions. The sanitized split (427 tasks)
is used by default for reliable evaluation.

Source: https://huggingface.co/datasets/google-research-datasets/mbpp
"""

from __future__ import annotations

from codeforge.evaluation.cache import download_dataset, load_jsonl
from codeforge.evaluation.providers.base import (
    BenchmarkType,
    Capabilities,
    TaskSpec,
    register_provider,
)

_JSONL_URL = "https://huggingface.co/api/datasets/google-research-datasets/mbpp/parquet/sanitized/test"
_FULL_URL = "https://huggingface.co/api/datasets/google-research-datasets/mbpp/parquet/full/test"
_FILENAME_SANITIZED = "mbpp_sanitized.jsonl"
_FILENAME_FULL = "mbpp_full.jsonl"


class MBPPProvider:
    """Loads MBPP tasks and converts them to TaskSpec."""

    def __init__(
        self,
        cache_dir: str = "",
        split: str = "sanitized",
        tasks: list[dict] | None = None,
    ) -> None:
        self._cache_dir = cache_dir
        self._split = split
        self._tasks_raw = tasks

    @property
    def name(self) -> str:
        return "mbpp"

    @property
    def benchmark_type(self) -> BenchmarkType:
        return BenchmarkType.SIMPLE

    @property
    def capabilities(self) -> Capabilities:
        return Capabilities(functional_tests=True, llm_judge=True)

    async def load_tasks(self) -> list[TaskSpec]:
        raw = self._tasks_raw or await self._fetch_tasks()
        return [self._convert_task(t) for t in raw]

    async def task_count(self) -> int:
        raw = self._tasks_raw or await self._fetch_tasks()
        return len(raw)

    async def _fetch_tasks(self) -> list[dict]:
        if self._split == "full":
            url, filename = _FULL_URL, _FILENAME_FULL
        else:
            url, filename = _JSONL_URL, _FILENAME_SANITIZED

        path = await download_dataset(
            url=url,
            provider_name="mbpp",
            filename=filename,
            base_dir=self._cache_dir,
        )
        self._tasks_raw = load_jsonl(path)
        return self._tasks_raw

    def _convert_task(self, raw: dict) -> TaskSpec:
        task_id = str(raw.get("task_id", ""))
        text = raw.get("text", "")
        code = raw.get("code", "")
        test_list = raw.get("test_list", [])
        test_setup = raw.get("test_setup_code", "")

        instruction = (
            f"Write a Python function to solve the following problem. "
            f"Return ONLY the complete function definition.\n\n{text}"
        )

        # Build test assertions as a runnable script
        test_lines = []
        if test_setup:
            test_lines.append(test_setup)
        test_lines.append("{SOLUTION}")
        test_lines.extend(test_list)
        test_lines.append('print("ALL TESTS PASSED")')
        test_script = "\n".join(test_lines)

        return TaskSpec(
            id=f"mbpp_{task_id}",
            name=f"MBPP_{task_id}",
            input=instruction,
            expected_output=code,
            test_command="python solution.py",
            difficulty=self._estimate_difficulty(test_list),
            metadata={
                "test_assertions": "\n".join(test_list),
                "test_setup": test_setup,
                "test_harness": test_script,
                "language": "python",
            },
        )

    @staticmethod
    def _estimate_difficulty(test_list: list[str]) -> str:
        total_len = sum(len(t) for t in test_list)
        if total_len < 100:
            return "easy"
        if total_len < 300:
            return "medium"
        return "hard"


register_provider("mbpp", MBPPProvider)
