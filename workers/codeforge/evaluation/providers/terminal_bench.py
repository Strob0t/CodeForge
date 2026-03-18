"""Terminal-Bench benchmark provider.

Evaluates terminal/CLI skills of AI agents. Tasks include initial filesystem
state, a command instruction, and expected filesystem state after execution.
Evaluation is via filesystem state verification.

Source: Local JSONL cache or HuggingFace dataset (if available).
"""

from __future__ import annotations

import json

from codeforge.evaluation.cache import download_hf_dataset, load_jsonl
from codeforge.evaluation.providers.base import (
    BenchmarkType,
    Capabilities,
    TaskSpec,
    register_provider,
)

_DATASET = "terminal-bench/terminal-bench"
_CONFIG = "default"
_FILENAME = "terminal_bench.jsonl"


def _build_expected_state_summary(
    expected_files: dict[str, str],
    expected_missing: list[str],
) -> str:
    """Build a human-readable summary of expected filesystem state."""
    parts: list[str] = []
    if expected_files:
        parts.append("Expected files:")
        for path, content in expected_files.items():
            if content:
                parts.append(f"  {path} (with specific content)")
            else:
                parts.append(f"  {path} (exists)")
    if expected_missing:
        parts.append("Files that should NOT exist:")
        parts.extend(f"  {path}" for path in expected_missing)
    return "\n".join(parts)


class TerminalBenchProvider:
    """Loads Terminal-Bench tasks and converts them to TaskSpec."""

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
        return "terminal_bench"

    @property
    def benchmark_type(self) -> BenchmarkType:
        return BenchmarkType.AGENT

    @property
    def capabilities(self) -> Capabilities:
        return Capabilities(functional_tests=True)

    async def load_tasks(self) -> list[TaskSpec]:
        raw = self._tasks_raw if self._tasks_raw is not None else await self._fetch_tasks()
        return [self._convert_task(t) for t in raw]

    async def task_count(self) -> int:
        raw = self._tasks_raw if self._tasks_raw is not None else await self._fetch_tasks()
        return len(raw)

    async def _fetch_tasks(self) -> list[dict]:
        """Download and cache the Terminal-Bench dataset."""
        path = await download_hf_dataset(
            dataset=_DATASET,
            split="test",
            provider_name="terminal_bench",
            filename=_FILENAME,
            base_dir=self._cache_dir,
            config=_CONFIG,
        )
        self._tasks_raw = load_jsonl(path)
        return self._tasks_raw

    def _convert_task(self, raw: dict) -> TaskSpec:
        """Convert a raw Terminal-Bench record to TaskSpec."""
        task_id = raw.get("id", "")
        task_name = raw.get("name", task_id)
        instruction = raw.get("instruction", "")
        initial_files: dict[str, str] = raw.get("initial_files", {})
        expected_files: dict[str, str] = raw.get("expected_files", {})
        expected_missing: list[str] = raw.get("expected_missing", [])
        difficulty = raw.get("difficulty", "medium")
        tags = raw.get("tags", "")

        expected_output = _build_expected_state_summary(expected_files, expected_missing)

        metadata: dict[str, str] = {
            "expected_files": json.dumps(expected_files),
            "expected_missing": json.dumps(expected_missing),
            "eval_method": "filesystem_state",
        }
        if tags:
            metadata["tags"] = tags

        return TaskSpec(
            id=task_id,
            name=task_name,
            input=instruction,
            expected_output=expected_output,
            initial_files=initial_files,
            test_command="verify_filesystem_state",
            difficulty=difficulty,
            metadata=metadata,
        )


register_provider("terminal_bench", TerminalBenchProvider)
