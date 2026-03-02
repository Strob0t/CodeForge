"""Built-in CodeForge Tool-Use benchmark provider.

Extended YAML format with tools and expected_tool_sequence.
Auto-registers via module import.
"""

from __future__ import annotations

import json
from typing import TYPE_CHECKING

import yaml

from codeforge.evaluation.providers.base import (
    BenchmarkType,
    Capabilities,
    TaskSpec,
    ToolCall,
    register_provider,
)

if TYPE_CHECKING:
    from pathlib import Path


class CodeForgeToolUseProvider:
    """Loads tool-use tasks from YAML datasets with tool definitions."""

    def __init__(self, dataset_path: str = "") -> None:
        self._dataset_path = dataset_path

    @property
    def name(self) -> str:
        return "codeforge_tool_use"

    @property
    def benchmark_type(self) -> BenchmarkType:
        return BenchmarkType.TOOL_USE

    @property
    def capabilities(self) -> Capabilities:
        return Capabilities(llm_judge=True, functional_tests=True)

    async def load_tasks(self) -> list[TaskSpec]:
        from pathlib import Path as _Path

        path: Path = _Path(self._dataset_path)
        raw = yaml.safe_load(path.read_text())
        tasks: list[TaskSpec] = []
        for t in raw.get("tasks", []):
            expected_tools = [
                ToolCall(name=tc["name"], args=tc.get("args", "")) for tc in t.get("expected_tool_sequence", [])
            ]
            metadata: dict[str, str] = {}
            if "tools" in t:
                metadata["tools"] = json.dumps(t["tools"])

            tasks.append(
                TaskSpec(
                    id=t["id"],
                    name=t["name"],
                    input=t["input"],
                    expected_output=t.get("expected_output", ""),
                    expected_tools=expected_tools,
                    context=t.get("context", []),
                    difficulty=t.get("difficulty", "medium"),
                    metadata=metadata,
                )
            )
        return tasks

    async def task_count(self) -> int:
        tasks = await self.load_tasks()
        return len(tasks)


register_provider("codeforge_tool_use", CodeForgeToolUseProvider)
