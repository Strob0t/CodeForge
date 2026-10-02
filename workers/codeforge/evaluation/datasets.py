"""Benchmark dataset loading and result persistence."""

from __future__ import annotations

import json
import os
from pathlib import Path

import yaml
from pydantic import BaseModel

from codeforge.config import get_settings
from codeforge.constants import MAX_WORKSPACE_FILE_BYTES
from codeforge.workspace_fs import WorkspacePathError, WorkspaceRoot, workspace_relative


class DatasetPathError(WorkspacePathError):
    """A dataset path outside the benchmark datasets directory (KI-107)."""

    def __init__(self) -> None:
        super().__init__(
            "dataset path is outside the benchmark datasets directory "
            "(benchmark.datasets_dir / CODEFORGE_BENCHMARK_DATASETS_DIR)"
        )


def datasets_dir() -> str:
    """The worker's benchmark datasets directory (benchmark.datasets_dir), absolute.

    A relative setting is looked up below the working directory, its parent
    (the worker runs from workers/ in development, like the config file
    search) and the CODEFORGE_WORKSPACE checkout; the first that exists wins.
    """
    settings = get_settings()
    configured = settings.benchmark_datasets_dir
    if os.path.isabs(configured):
        return os.path.normpath(configured)
    candidates = [
        os.path.abspath(configured),
        os.path.abspath(os.path.join(os.pardir, configured)),
        os.path.normpath(os.path.join(settings.workspace, configured)),
    ]
    return next((c for c in candidates if os.path.isdir(c)), candidates[0])


def dataset_relative(directory: str, path: str) -> str:
    """*path* relative to the datasets *directory*: a relative name as given, an absolute path inside it.

    The Go Core sends absolute paths inside its datasets directory; they are
    matched against the directory as configured and resolved. Anything else
    absolute raises DatasetPathError; ".." and symlinks leading out are
    refused when the name is resolved below the directory.
    """
    if not path.startswith("/"):
        return path
    real = os.path.realpath(directory)
    candidates = [
        (directory, path),
        (real, path),
        (real, os.path.join(os.path.realpath(os.path.dirname(path)), os.path.basename(path))),
    ]
    for base, target in candidates:
        rel = workspace_relative(base, target)
        if not rel.startswith("/"):
            return rel
    raise DatasetPathError


def read_dataset_text(path: str | Path) -> str:
    """The text of a dataset file below the worker's datasets directory (KI-107).

    Resolved through WorkspaceRoot: no symlink leads out of the directory, a
    FIFO never blocks, only regular files up to MAX_WORKSPACE_FILE_BYTES.
    """
    directory = datasets_dir()
    rel = dataset_relative(directory, str(path))
    with WorkspaceRoot.operator_dir(directory) as root:
        return root.read_text(rel, max_bytes=MAX_WORKSPACE_FILE_BYTES)


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
        path: A dataset name or path below the benchmark datasets directory.

    Returns:
        Parsed BenchmarkDataset with all tasks.

    Raises:
        FileNotFoundError: If the dataset file does not exist.
        WorkspacePathError: If the path leaves the datasets directory.
        yaml.YAMLError: If the file is not valid YAML.
    """
    raw = yaml.safe_load(read_dataset_text(path))
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
