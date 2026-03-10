"""HumanEval benchmark provider.

164 Python function completion tasks from OpenAI's HumanEval dataset.
Each task provides a function signature + docstring; the model must
generate the function body. Evaluation is via functional tests.

Source: https://huggingface.co/datasets/openai/openai_humaneval
"""

from __future__ import annotations

import textwrap

from codeforge.evaluation.cache import download_hf_dataset, load_jsonl
from codeforge.evaluation.providers.base import (
    BenchmarkType,
    Capabilities,
    TaskSpec,
    register_provider,
)

_DATASET = "openai/openai_humaneval"
_CONFIG = "openai_humaneval"
_FILENAME = "humaneval.jsonl"


def _build_test_harness(entry_point: str, test_code: str, solution: str) -> str:
    """Build a Python test script that runs the generated solution against test cases.

    The harness writes the solution to a temp module, imports it, and runs tests.
    """
    return textwrap.dedent(f"""\
        {solution}

        {test_code}

        check({entry_point})
        print("ALL TESTS PASSED")
    """)


class HumanEvalProvider:
    """Loads HumanEval tasks and converts them to TaskSpec."""

    def __init__(self, cache_dir: str = "", tasks: list[dict] | None = None, config: dict | None = None) -> None:
        self._cache_dir = cache_dir
        self._tasks_raw = tasks  # Allow injection for testing
        self._config = config or {}

    @property
    def name(self) -> str:
        return "humaneval"

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
        """Download and cache the HumanEval dataset."""
        path = await download_hf_dataset(
            dataset=_DATASET,
            split="test",
            provider_name="humaneval",
            filename=_FILENAME,
            base_dir=self._cache_dir,
            config=_CONFIG,
        )
        self._tasks_raw = load_jsonl(path)
        return self._tasks_raw

    def _convert_task(self, raw: dict) -> TaskSpec:
        """Convert a raw HumanEval record to TaskSpec."""
        task_id = raw.get("task_id", "")
        prompt = raw.get("prompt", "")
        test = raw.get("test", "")
        entry_point = raw.get("entry_point", "")
        canonical = raw.get("canonical_solution", "")

        # The prompt IS the input — model should complete the function
        instruction = (
            f"Complete the following Python function. "
            f"Return ONLY the function body (no signature, no markdown).\n\n{prompt}"
        )

        # Build test command: write solution + tests to file, run with python
        test_script = _build_test_harness(entry_point, test, f"{prompt}{{SOLUTION}}")

        return TaskSpec(
            id=task_id,
            name=task_id.replace("/", "_"),
            input=instruction,
            expected_output=canonical,
            test_command="python solution.py",
            difficulty=self._estimate_difficulty(canonical),
            metadata={
                "entry_point": entry_point,
                "prompt": prompt,
                "test_code": test,
                "test_harness": test_script,
                "language": "python",
            },
        )

    @staticmethod
    def _estimate_difficulty(solution: str) -> str:
        lines = len(solution.strip().splitlines())
        if lines <= 5:
            return "easy"
        if lines <= 15:
            return "medium"
        return "hard"


register_provider("humaneval", HumanEvalProvider)
