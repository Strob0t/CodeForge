"""DPAI Arena benchmark provider.

Coding challenges from the DPAI Arena dataset for evaluating LLM coding
capabilities. Each task provides a question, optional test cases, and a
reference solution. Evaluation is via functional tests and LLM judge.

Source: https://huggingface.co/datasets/DPAI/arena (if available)
Fallback: Local JSONL cache file.
"""

from __future__ import annotations

from codeforge.evaluation.cache import download_hf_dataset, load_jsonl
from codeforge.evaluation.providers.base import (
    BenchmarkType,
    Capabilities,
    TaskSpec,
    register_provider,
)

_DATASET = "DPAI/arena"
_CONFIG = "default"
_FILENAME = "dpai_arena.jsonl"


class DPAIArenaProvider:
    """Loads DPAI Arena coding challenge tasks and converts them to TaskSpec."""

    def __init__(
        self,
        cache_dir: str = "",
        tasks: list[dict] | None = None,
        config: dict | None = None,
    ) -> None:
        self._cache_dir = cache_dir
        self._tasks_raw = tasks
        self._config = config or {}

    @property
    def name(self) -> str:
        return "dpai_arena"

    @property
    def benchmark_type(self) -> BenchmarkType:
        return BenchmarkType.SIMPLE

    @property
    def capabilities(self) -> Capabilities:
        return Capabilities(functional_tests=True, llm_judge=True)

    async def load_tasks(self) -> list[TaskSpec]:
        raw = self._tasks_raw if self._tasks_raw is not None else await self._fetch_tasks()
        return [self._convert_task(t) for t in raw]

    async def task_count(self) -> int:
        raw = self._tasks_raw if self._tasks_raw is not None else await self._fetch_tasks()
        return len(raw)

    async def _fetch_tasks(self) -> list[dict]:
        """Download and cache the DPAI Arena dataset."""
        path = await download_hf_dataset(
            dataset=_DATASET,
            split="test",
            provider_name="dpai_arena",
            filename=_FILENAME,
            base_dir=self._cache_dir,
            config=_CONFIG,
        )
        self._tasks_raw = load_jsonl(path)
        return self._tasks_raw

    def _convert_task(self, raw: dict) -> TaskSpec:
        """Convert a raw DPAI Arena record to TaskSpec."""
        task_id = raw.get("id", "")
        question = raw.get("question", "")
        solution = raw.get("solution", "")
        test_cases = raw.get("test_cases", "")
        difficulty = raw.get("difficulty", "medium")
        tags = raw.get("tags", "")
        language = raw.get("language", "python")

        instruction = (
            f"Solve the following coding challenge. "
            f"Return ONLY the solution code (no markdown, no explanation).\n\n{question}"
        )

        metadata: dict[str, str] = {
            "language": language,
        }
        if tags:
            metadata["tags"] = tags
        if test_cases:
            metadata["test_cases"] = test_cases

        return TaskSpec(
            id=task_id,
            name=task_id,
            input=instruction,
            expected_output=solution,
            test_command="python solution.py",
            difficulty=difficulty,
            metadata=metadata,
        )


register_provider("dpai_arena", DPAIArenaProvider)
