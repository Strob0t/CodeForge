"""Tests for benchmark dataset loading and result persistence."""

from __future__ import annotations

import json
from typing import TYPE_CHECKING

import pytest

if TYPE_CHECKING:
    from pathlib import Path

from codeforge.evaluation.datasets import BenchmarkDataset, TaskResult, load_dataset, save_results


@pytest.fixture
def sample_dataset_file(tmp_path: Path) -> Path:
    """Create a minimal benchmark dataset YAML file."""
    content = """
name: test-dataset
description: A test dataset
tasks:
  - id: task-1
    name: Hello World
    input: "Write a function that prints hello world"
    expected_output: "def hello(): print('hello world')"
    difficulty: easy
  - id: task-2
    name: Add Two Numbers
    input: "Write a function that adds two numbers"
    expected_output: "def add(a, b): return a + b"
    expected_tools:
      - name: write_file
        args: "main.py"
"""
    p = tmp_path / "test.yaml"
    p.write_text(content)
    return p


def test_load_dataset(sample_dataset_file: Path) -> None:
    ds = load_dataset(sample_dataset_file)
    assert isinstance(ds, BenchmarkDataset)
    assert ds.name == "test-dataset"
    assert ds.description == "A test dataset"
    assert len(ds.tasks) == 2
    assert ds.tasks[0].id == "task-1"
    assert ds.tasks[0].difficulty == "easy"
    assert ds.tasks[1].expected_tools == [{"name": "write_file", "args": "main.py"}]


def test_load_dataset_not_found() -> None:
    with pytest.raises(FileNotFoundError):
        load_dataset("/nonexistent/path.yaml")


def test_save_results(tmp_path: Path) -> None:
    results = [
        TaskResult(
            task_id="t1",
            task_name="Test Task",
            scores={"correctness": 0.85},
            actual_output="output",
            expected_output="expected",
            duration_ms=1234,
        ),
    ]
    out = tmp_path / "results.json"
    save_results(results, out)

    data = json.loads(out.read_text())
    assert len(data) == 1
    assert data[0]["task_id"] == "t1"
    assert data[0]["scores"]["correctness"] == 0.85
