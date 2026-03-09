"""SPARC-bench benchmark provider.

Uses SWE-bench tasks but evaluates with the SPARC methodology:
7 dimensions — correctness (functional), steps, time, cost,
complexity, code_quality, and security.

The SPARCEvaluator handles multi-dimensional scoring; this provider
handles task loading and declares SPARC-specific capabilities.

Source: Uses SWE-bench data with SPARC evaluation methodology.
"""

from __future__ import annotations

from codeforge.evaluation.cache import download_dataset, load_jsonl
from codeforge.evaluation.providers.base import (
    BenchmarkType,
    Capabilities,
    TaskSpec,
    register_provider,
)

_LITE_URL = "https://huggingface.co/api/datasets/princeton-nlp/SWE-bench_Lite/parquet/default/test"
_FILENAME = "swebench_lite.jsonl"


class SPARCBenchProvider:
    """SPARC-bench: SWE-bench tasks with multi-dimensional SPARC evaluation.

    Uses SWE-bench Lite (300 tasks) by default. The key difference from
    SWEBenchProvider is the evaluation method: SPARC scores across 7
    quality dimensions rather than just pass/fail.
    """

    def __init__(self, cache_dir: str = "", tasks: list[dict] | None = None, config: dict | None = None) -> None:
        self._cache_dir = cache_dir
        self._tasks_raw = tasks
        self._config = config or {}

    @property
    def name(self) -> str:
        return "sparcbench"

    @property
    def benchmark_type(self) -> BenchmarkType:
        return BenchmarkType.AGENT

    @property
    def capabilities(self) -> Capabilities:
        return Capabilities(
            functional_tests=True,
            llm_judge=True,
            swe_bench_style=True,
            sparc_style=True,
        )

    async def load_tasks(self) -> list[TaskSpec]:
        raw = self._tasks_raw or await self._fetch_tasks()
        return [self._convert_task(t) for t in raw]

    async def task_count(self) -> int:
        raw = self._tasks_raw or await self._fetch_tasks()
        return len(raw)

    async def _fetch_tasks(self) -> list[dict]:
        path = await download_dataset(
            url=_LITE_URL,
            provider_name="sparcbench",
            filename=_FILENAME,
            base_dir=self._cache_dir,
        )
        self._tasks_raw = load_jsonl(path)
        return self._tasks_raw

    def _convert_task(self, raw: dict) -> TaskSpec:
        instance_id = raw.get("instance_id", "")
        repo = raw.get("repo", "")
        base_commit = raw.get("base_commit", "")
        problem_statement = raw.get("problem_statement", "")
        hints_text = raw.get("hints_text", "")
        test_patch = raw.get("test_patch", "")
        patch = raw.get("patch", "")

        instruction = (
            f"Fix the following GitHub issue in the repository {repo}.\n"
            f"Follow SPARC methodology: Specification, Pseudocode, Architecture, "
            f"Refinement, Completion.\n\n"
            f"Issue:\n{problem_statement}"
        )
        if hints_text:
            instruction += f"\n\nHints:\n{hints_text}"

        test_command = "git apply --check test_patch.diff && pytest -x" if test_patch else ""

        patch_lines = len(patch.splitlines()) if patch else 0
        if patch_lines <= 20:
            difficulty = "easy"
        elif patch_lines <= 80:
            difficulty = "medium"
        else:
            difficulty = "hard"

        return TaskSpec(
            id=f"sparc_{instance_id}",
            name=instance_id.replace("/", "_").replace("-", "_")[:60],
            input=instruction,
            expected_output="",
            test_command=test_command,
            repo_url=f"https://github.com/{repo}.git" if repo else "",
            repo_commit=base_commit,
            difficulty=difficulty,
            metadata={
                "repo": repo,
                "base_commit": base_commit,
                "test_patch": test_patch,
                "gold_patch": patch,
                "language": "python",
                "eval_method": "sparc",
                "evaluators": "sparc,functional_test",
            },
        )


register_provider("sparcbench", SPARCBenchProvider)
