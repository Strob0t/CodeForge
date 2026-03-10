"""LiveCodeBench benchmark provider.

Competition programming problems collected from LeetCode, Codeforces,
and AtCoder. Supports temporal splits so models can be evaluated on
problems released after their training cutoff.

Source: https://huggingface.co/datasets/livecodebench/code_generation_lite
"""

from __future__ import annotations

from datetime import datetime

from codeforge.evaluation.cache import download_hf_dataset, load_jsonl
from codeforge.evaluation.providers.base import (
    BenchmarkType,
    Capabilities,
    TaskSpec,
    register_provider,
)

_DATASET = "livecodebench/code_generation_lite"
_CONFIG = "release_v5"
_FILENAME = "livecodebench.jsonl"


class LiveCodeBenchProvider:
    """Loads LiveCodeBench tasks with optional date filtering."""

    def __init__(
        self,
        cache_dir: str = "",
        after_date: str = "",
        before_date: str = "",
        tasks: list[dict] | None = None,
        config: dict | None = None,
    ) -> None:
        self._cache_dir = cache_dir
        self._after_date = after_date
        self._before_date = before_date
        self._tasks_raw = tasks
        self._config = config or {}

    @property
    def name(self) -> str:
        return "livecodebench"

    @property
    def benchmark_type(self) -> BenchmarkType:
        return BenchmarkType.SIMPLE

    @property
    def capabilities(self) -> Capabilities:
        return Capabilities(functional_tests=True, llm_judge=True)

    async def load_tasks(self) -> list[TaskSpec]:
        raw = self._tasks_raw or await self._fetch_tasks()
        filtered = self._apply_date_filter(raw)
        return [self._convert_task(t) for t in filtered]

    async def task_count(self) -> int:
        raw = self._tasks_raw or await self._fetch_tasks()
        return len(self._apply_date_filter(raw))

    async def _fetch_tasks(self) -> list[dict]:
        path = await download_hf_dataset(
            dataset=_DATASET,
            split="test",
            provider_name="livecodebench",
            filename=_FILENAME,
            base_dir=self._cache_dir,
            config=_CONFIG,
        )
        self._tasks_raw = load_jsonl(path)
        return self._tasks_raw

    def _apply_date_filter(self, tasks: list[dict]) -> list[dict]:
        """Filter tasks by contest date range."""
        if not self._after_date and not self._before_date:
            return tasks

        after = _parse_date(self._after_date) if self._after_date else None
        before = _parse_date(self._before_date) if self._before_date else None

        filtered = []
        for t in tasks:
            date_str = t.get("contest_date", t.get("date", ""))
            if not date_str:
                filtered.append(t)
                continue
            task_date = _parse_date(date_str)
            if task_date is None:
                filtered.append(t)
                continue
            if after and task_date < after:
                continue
            if before and task_date > before:
                continue
            filtered.append(t)
        return filtered

    def _convert_task(self, raw: dict) -> TaskSpec:
        task_id = raw.get("question_id", raw.get("id", raw.get("task_id", "")))
        title = raw.get("question_title", raw.get("title", f"LCB_{task_id}"))
        content = raw.get("question_content", raw.get("prompt", ""))
        difficulty = raw.get("difficulty", "medium").lower()
        contest_date = raw.get("contest_date", raw.get("date", ""))
        platform = raw.get("platform", "")
        starter_code = raw.get("starter_code", "")

        instruction = (
            f"Solve the following competitive programming problem in Python. "
            f"Return ONLY the complete solution code.\n\n"
            f"Problem: {title}\n\n{content}"
        )
        if starter_code:
            instruction += f"\n\nStarter code:\n```python\n{starter_code}\n```"

        # Test cases are typically in input/output format
        public_tests = raw.get("public_test_cases", "")
        private_tests = raw.get("private_test_cases", "")

        return TaskSpec(
            id=f"lcb_{task_id}",
            name=title.replace(" ", "_")[:60],
            input=instruction,
            expected_output="",
            test_command="python solution.py",
            difficulty=difficulty if difficulty in ("easy", "medium", "hard") else "medium",
            metadata={
                "platform": platform,
                "contest_date": contest_date,
                "starter_code": starter_code,
                "public_tests": str(public_tests),
                "private_tests": str(private_tests),
                "language": "python",
            },
        )


def _parse_date(date_str: str) -> datetime | None:
    """Parse a date string in common formats."""
    for fmt in ("%Y-%m-%d", "%Y-%m-%dT%H:%M:%S", "%Y/%m/%d"):
        try:
            return datetime.strptime(date_str, fmt)
        except ValueError:
            continue
    return None


register_provider("livecodebench", LiveCodeBenchProvider)
