"""CRUXEval benchmark provider.

800 code reasoning tasks: given a Python function and an input,
predict the output (CRUXEval-O), or given a function and output,
predict the input (CRUXEval-I).

Source: https://huggingface.co/datasets/cruxeval/cruxeval
"""

from __future__ import annotations

from codeforge.evaluation.cache import download_hf_dataset, load_jsonl
from codeforge.evaluation.providers.base import (
    BenchmarkType,
    Capabilities,
    TaskSpec,
    register_provider,
)

_DATASET = "cruxeval/cruxeval"
_FILENAME = "cruxeval.jsonl"


class CRUXEvalProvider:
    """Loads CRUXEval tasks and converts them to TaskSpec.

    Supports two modes:
    - output_prediction (default): given function + input → predict output
    - input_prediction: given function + output → predict input
    """

    def __init__(
        self,
        cache_dir: str = "",
        mode: str = "output_prediction",
        tasks: list[dict] | None = None,
        config: dict | None = None,
    ) -> None:
        self._cache_dir = cache_dir
        self._mode = mode
        self._tasks_raw = tasks
        self._config = config or {}

    @property
    def name(self) -> str:
        return "cruxeval"

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
            provider_name="cruxeval",
            filename=_FILENAME,
            base_dir=self._cache_dir,
        )
        self._tasks_raw = load_jsonl(path)
        return self._tasks_raw

    def _convert_task(self, raw: dict) -> TaskSpec:
        task_id = raw.get("id", raw.get("task_id", ""))
        code = raw.get("code", "")
        sample_input = raw.get("input", "")
        expected_output = raw.get("output", "")

        if self._mode == "input_prediction":
            instruction = (
                f"Given the following Python function and its expected output, "
                f"predict the input that produces this output.\n\n"
                f"Function:\n```python\n{code}\n```\n\n"
                f"Expected output: {expected_output}\n\n"
                f"Return ONLY the input value, nothing else."
            )
            expected = sample_input
        else:
            instruction = (
                f"Given the following Python function and input, "
                f"predict the exact output.\n\n"
                f"Function:\n```python\n{code}\n```\n\n"
                f"Input: {sample_input}\n\n"
                f"Return ONLY the output value, nothing else."
            )
            expected = expected_output

        # Build a test harness that verifies the prediction
        test_harness = (
            f"{code}\n\n"
            f"result = f({sample_input})\n"
            f"expected = {expected_output}\n"
            f"assert str(result) == str(expected), "
            f'f"Expected {{expected}}, got {{result}}"\n'
            f'print("ALL TESTS PASSED")\n'
        )

        return TaskSpec(
            id=f"cruxeval_{task_id}",
            name=f"CRUXEval_{task_id}",
            input=instruction,
            expected_output=expected,
            test_command="python solution.py",
            difficulty="medium",
            metadata={
                "code": code,
                "sample_input": sample_input,
                "expected_output": expected_output,
                "mode": self._mode,
                "test_harness": test_harness,
                "language": "python",
            },
        )


register_provider("cruxeval", CRUXEvalProvider)
