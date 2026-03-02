"""SWE-bench benchmark providers.

SWE-bench contains 2294 real GitHub issue-resolution tasks. Each task
provides an issue description, a repository snapshot at a specific commit,
and a test patch that validates the fix.

Three variants are provided:
- SWEBenchProvider: full dataset (2294 tasks)
- SWEBenchLiteProvider: curated 300-task subset
- SWEBenchVerifiedProvider: 500-task human-verified subset

Source: https://huggingface.co/datasets/princeton-nlp/SWE-bench
"""

from __future__ import annotations

from codeforge.evaluation.cache import download_dataset, load_jsonl
from codeforge.evaluation.providers.base import (
    BenchmarkType,
    Capabilities,
    TaskSpec,
    register_provider,
)

_FULL_URL = "https://huggingface.co/api/datasets/princeton-nlp/SWE-bench/parquet/default/test"
_LITE_URL = "https://huggingface.co/api/datasets/princeton-nlp/SWE-bench_Lite/parquet/default/test"
_VERIFIED_URL = "https://huggingface.co/api/datasets/princeton-nlp/SWE-bench_Verified/parquet/default/test"

_FILENAME_FULL = "swebench_full.jsonl"
_FILENAME_LITE = "swebench_lite.jsonl"
_FILENAME_VERIFIED = "swebench_verified.jsonl"


def _convert_swebench_task(raw: dict) -> TaskSpec:
    """Convert a raw SWE-bench record to TaskSpec."""
    instance_id = raw.get("instance_id", "")
    repo = raw.get("repo", "")
    base_commit = raw.get("base_commit", "")
    problem_statement = raw.get("problem_statement", "")
    hints_text = raw.get("hints_text", "")
    test_patch = raw.get("test_patch", "")
    patch = raw.get("patch", "")
    created_at = raw.get("created_at", "")
    version = raw.get("version", "")

    # Build instruction: the agent sees the issue description
    instruction = f"Fix the following GitHub issue in the repository {repo}.\n\nIssue:\n{problem_statement}"
    if hints_text:
        instruction += f"\n\nHints:\n{hints_text}"

    # The test_patch contains the tests that must pass after the fix
    # In a real run, the AgentBenchmarkRunner sets up the repo at base_commit,
    # the agent modifies files, then the test_patch is applied and tests run.
    test_command = "git apply --check test_patch.diff && pytest -x" if test_patch else ""

    # Estimate difficulty by patch size
    patch_lines = len(patch.splitlines()) if patch else 0
    if patch_lines <= 20:
        difficulty = "easy"
    elif patch_lines <= 80:
        difficulty = "medium"
    else:
        difficulty = "hard"

    return TaskSpec(
        id=instance_id,
        name=instance_id.replace("/", "_").replace("-", "_")[:60],
        input=instruction,
        expected_output="",  # SWE-bench uses test-based evaluation, not output comparison
        test_command=test_command,
        repo_url=f"https://github.com/{repo}.git" if repo else "",
        repo_commit=base_commit,
        difficulty=difficulty,
        metadata={
            "repo": repo,
            "base_commit": base_commit,
            "test_patch": test_patch,
            "gold_patch": patch,
            "version": version,
            "created_at": created_at,
            "language": "python",
            "eval_method": "swe_bench",
        },
    )


class SWEBenchProvider:
    """Full SWE-bench dataset (2294 tasks)."""

    def __init__(self, cache_dir: str = "", tasks: list[dict] | None = None) -> None:
        self._cache_dir = cache_dir
        self._tasks_raw = tasks

    @property
    def name(self) -> str:
        return "swebench"

    @property
    def benchmark_type(self) -> BenchmarkType:
        return BenchmarkType.AGENT

    @property
    def capabilities(self) -> Capabilities:
        return Capabilities(
            functional_tests=True,
            llm_judge=True,
            swe_bench_style=True,
        )

    async def load_tasks(self) -> list[TaskSpec]:
        raw = self._tasks_raw or await self._fetch_tasks()
        return [_convert_swebench_task(t) for t in raw]

    async def task_count(self) -> int:
        raw = self._tasks_raw or await self._fetch_tasks()
        return len(raw)

    async def _fetch_tasks(self) -> list[dict]:
        path = await download_dataset(
            url=_FULL_URL,
            provider_name="swebench",
            filename=_FILENAME_FULL,
            base_dir=self._cache_dir,
        )
        self._tasks_raw = load_jsonl(path)
        return self._tasks_raw


class SWEBenchLiteProvider:
    """SWE-bench Lite: curated 300-task subset."""

    def __init__(self, cache_dir: str = "", tasks: list[dict] | None = None) -> None:
        self._cache_dir = cache_dir
        self._tasks_raw = tasks

    @property
    def name(self) -> str:
        return "swebench_lite"

    @property
    def benchmark_type(self) -> BenchmarkType:
        return BenchmarkType.AGENT

    @property
    def capabilities(self) -> Capabilities:
        return Capabilities(
            functional_tests=True,
            llm_judge=True,
            swe_bench_style=True,
        )

    async def load_tasks(self) -> list[TaskSpec]:
        raw = self._tasks_raw or await self._fetch_tasks()
        return [_convert_swebench_task(t) for t in raw]

    async def task_count(self) -> int:
        raw = self._tasks_raw or await self._fetch_tasks()
        return len(raw)

    async def _fetch_tasks(self) -> list[dict]:
        path = await download_dataset(
            url=_LITE_URL,
            provider_name="swebench",
            filename=_FILENAME_LITE,
            base_dir=self._cache_dir,
        )
        self._tasks_raw = load_jsonl(path)
        return self._tasks_raw


class SWEBenchVerifiedProvider:
    """SWE-bench Verified: 500-task human-verified subset."""

    def __init__(self, cache_dir: str = "", tasks: list[dict] | None = None) -> None:
        self._cache_dir = cache_dir
        self._tasks_raw = tasks

    @property
    def name(self) -> str:
        return "swebench_verified"

    @property
    def benchmark_type(self) -> BenchmarkType:
        return BenchmarkType.AGENT

    @property
    def capabilities(self) -> Capabilities:
        return Capabilities(
            functional_tests=True,
            llm_judge=True,
            swe_bench_style=True,
        )

    async def load_tasks(self) -> list[TaskSpec]:
        raw = self._tasks_raw or await self._fetch_tasks()
        return [_convert_swebench_task(t) for t in raw]

    async def task_count(self) -> int:
        raw = self._tasks_raw or await self._fetch_tasks()
        return len(raw)

    async def _fetch_tasks(self) -> list[dict]:
        path = await download_dataset(
            url=_VERIFIED_URL,
            provider_name="swebench",
            filename=_FILENAME_VERIFIED,
            base_dir=self._cache_dir,
        )
        self._tasks_raw = load_jsonl(path)
        return self._tasks_raw


register_provider("swebench", SWEBenchProvider)
register_provider("swebench_lite", SWEBenchLiteProvider)
register_provider("swebench_verified", SWEBenchVerifiedProvider)
