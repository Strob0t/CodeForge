"""Built-in CodeForge Agent benchmark provider.

Loads agent tasks from YAML datasets with initial_files, test_command,
and workspace setup instructions. Auto-registers via module import.
"""

from __future__ import annotations

import json

import yaml

from codeforge.evaluation.providers.base import (
    BenchmarkType,
    Capabilities,
    TaskSpec,
    ToolCall,
    register_provider,
)


class CodeForgeAgentProvider:
    """Loads agent benchmark tasks from YAML datasets."""

    def __init__(self, dataset_path: str = "") -> None:
        self._dataset_path = dataset_path

    @property
    def name(self) -> str:
        return "codeforge_agent"

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
        from pathlib import Path as _Path

        path: _Path = _Path(self._dataset_path)
        raw = yaml.safe_load(path.read_text())
        tasks: list[TaskSpec] = []
        for t in raw.get("tasks", []):
            expected_tools = [
                ToolCall(name=tc["name"], args=tc.get("args", "")) for tc in t.get("expected_tool_sequence", [])
            ]
            metadata: dict[str, str] = {}
            if "max_iterations" in t:
                metadata["max_iterations"] = str(t["max_iterations"])
            if "timeout_seconds" in t:
                metadata["timeout_seconds"] = str(t["timeout_seconds"])
            if "max_cost" in t:
                metadata["max_cost"] = str(t["max_cost"])
            if "test_timeout" in t:
                metadata["test_timeout"] = str(t["test_timeout"])
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
                    initial_files=t.get("initial_files", {}),
                    test_command=t.get("test_command", ""),
                    metadata=metadata,
                )
            )
        return tasks

    async def task_count(self) -> int:
        tasks = await self.load_tasks()
        return len(tasks)


register_provider("codeforge_agent", CodeForgeAgentProvider)
