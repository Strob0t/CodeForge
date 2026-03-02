"""Built-in CodeForge Simple benchmark provider.

Wraps existing YAML dataset format into the BenchmarkProvider interface.
Auto-registers via module import.
"""

from __future__ import annotations

from typing import TYPE_CHECKING

import yaml

from codeforge.evaluation.providers.base import (
    BenchmarkType,
    Capabilities,
    TaskSpec,
    register_provider,
)

if TYPE_CHECKING:
    from pathlib import Path


class CodeForgeSimpleProvider:
    """Loads simple prompt/response tasks from YAML datasets."""

    def __init__(self, dataset_path: str = "") -> None:
        self._dataset_path = dataset_path

    @property
    def name(self) -> str:
        return "codeforge_simple"

    @property
    def benchmark_type(self) -> BenchmarkType:
        return BenchmarkType.SIMPLE

    @property
    def capabilities(self) -> Capabilities:
        return Capabilities(llm_judge=True)

    async def load_tasks(self) -> list[TaskSpec]:
        from pathlib import Path as _Path

        path: Path = _Path(self._dataset_path)
        raw = yaml.safe_load(path.read_text())
        return [
            TaskSpec(
                id=t["id"],
                name=t["name"],
                input=t["input"],
                expected_output=t.get("expected_output", ""),
                context=t.get("context", []),
                difficulty=t.get("difficulty", "medium"),
            )
            for t in raw.get("tasks", [])
        ]

    async def task_count(self) -> int:
        tasks = await self.load_tasks()
        return len(tasks)


register_provider("codeforge_simple", CodeForgeSimpleProvider)
