"""Benchmark dataset loading and result persistence."""

from __future__ import annotations

import json
from pathlib import Path

import yaml
from pydantic import BaseModel


class BenchmarkTask(BaseModel):
    """Single task within a benchmark dataset."""

    id: str
    name: str
    input: str
    expected_output: str
    expected_tools: list[dict[str, str]] = []
    context: list[str] = []
    difficulty: str = "medium"


class BenchmarkDataset(BaseModel):
    """Collection of benchmark tasks loaded from YAML."""

    name: str
    description: str = ""
    tasks: list[BenchmarkTask]


class TaskResult(BaseModel):
    """Result of executing a single benchmark task."""

    task_id: str
    task_name: str
    scores: dict[str, float]
    actual_output: str
    expected_output: str
    tool_calls: list[dict[str, str]] = []
    cost_usd: float = 0.0
    tokens_in: int = 0
    tokens_out: int = 0
    duration_ms: int = 0


def load_dataset(path: str | Path) -> BenchmarkDataset:
    """Load a benchmark dataset from a YAML file.

    Args:
        path: Absolute or relative path to a YAML dataset file.

    Returns:
        Parsed BenchmarkDataset with all tasks.

    Raises:
        FileNotFoundError: If the dataset file does not exist.
        yaml.YAMLError: If the file is not valid YAML.
    """
    p = Path(path)
    raw = yaml.safe_load(p.read_text(encoding="utf-8"))
    return BenchmarkDataset.model_validate(raw)


def save_results(results: list[TaskResult], path: str | Path) -> None:
    """Save benchmark results to a JSON file.

    Args:
        results: List of task results to persist.
        path: Output file path (will be created or overwritten).
    """
    p = Path(path)
    p.parent.mkdir(parents=True, exist_ok=True)
    data = [r.model_dump() for r in results]
    p.write_text(json.dumps(data, indent=2), encoding="utf-8")
