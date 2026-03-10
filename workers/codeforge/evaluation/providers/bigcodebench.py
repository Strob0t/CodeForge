"""BigCodeBench benchmark provider.

1140 tasks requiring composition of multiple Python library APIs.
Tasks include function signatures with docstrings and test suites.

Source: https://huggingface.co/datasets/bigcode/bigcodebench
"""

from __future__ import annotations

from codeforge.evaluation.cache import download_hf_dataset, load_jsonl
from codeforge.evaluation.providers.base import (
    BenchmarkType,
    Capabilities,
    TaskSpec,
    register_provider,
)

_DATASET = "bigcode/bigcodebench"
_CONFIG = "v0.1.2"
_FILENAME = "bigcodebench.jsonl"


class BigCodeBenchProvider:
    """Loads BigCodeBench tasks and converts them to TaskSpec."""

    def __init__(self, cache_dir: str = "", tasks: list[dict] | None = None, config: dict | None = None) -> None:
        self._cache_dir = cache_dir
        self._tasks_raw = tasks
        self._config = config or {}

    @property
    def name(self) -> str:
        return "bigcodebench"

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
        path = await download_hf_dataset(
            dataset=_DATASET,
            split="test",
            provider_name="bigcodebench",
            filename=_FILENAME,
            base_dir=self._cache_dir,
            config=_CONFIG,
        )
        self._tasks_raw = load_jsonl(path)
        return self._tasks_raw

    def _convert_task(self, raw: dict) -> TaskSpec:
        task_id = raw.get("task_id", raw.get("id", ""))
        instruct_prompt = raw.get("instruct_prompt", "")
        complete_prompt = raw.get("complete_prompt", "")
        prompt = instruct_prompt or complete_prompt
        canonical = raw.get("canonical_solution", "")
        test = raw.get("test", "")
        libs = raw.get("libs", [])

        instruction = (
            f"Write a Python function to solve the following problem. Return ONLY the complete function.\n\n{prompt}"
        )

        test_harness = f"{{SOLUTION}}\n\n{test}\n" if test else ""

        return TaskSpec(
            id=str(task_id),
            name=f"BCB_{task_id}",
            input=instruction,
            expected_output=canonical,
            test_command="python solution.py",
            difficulty="hard",
            metadata={
                "test_code": test,
                "test_harness": test_harness,
                "libs": ",".join(libs) if isinstance(libs, list) else str(libs),
                "language": "python",
            },
        )


register_provider("bigcodebench", BigCodeBenchProvider)
