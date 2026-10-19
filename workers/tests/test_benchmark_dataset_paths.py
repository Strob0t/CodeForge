"""Benchmark datasets are read below the worker's datasets directory only (KI-107)."""

from __future__ import annotations

import os
import threading
from types import SimpleNamespace
from typing import TYPE_CHECKING

import pytest

from codeforge.consumer import _benchmark_runners
from codeforge.evaluation import datasets
from codeforge.evaluation.datasets import DatasetPathError, load_dataset, read_dataset_text
from codeforge.evaluation.providers.codeforge_agent import CodeForgeAgentProvider
from codeforge.evaluation.providers.codeforge_simple import CodeForgeSimpleProvider
from codeforge.evaluation.providers.codeforge_tool_use import CodeForgeToolUseProvider
from codeforge.workspace_fs import NotRegularFileError, PathLeavesWorkspaceError

if TYPE_CHECKING:
    from pathlib import Path

_DATASET = "name: {name}\ntasks:\n  - id: t1\n    name: T\n    input: x\n    expected_output: y\n"


@pytest.fixture
def layout(tmp_path: Path, monkeypatch: pytest.MonkeyPatch) -> tuple[Path, Path]:
    root = tmp_path / "datasets"
    outside = tmp_path / "outside"
    (root / "sub").mkdir(parents=True)
    outside.mkdir()
    (root / "basic.yaml").write_text(_DATASET.format(name="basic"))
    (root / "sub" / "more.yml").write_text(_DATASET.format(name="more"))
    (outside / "secret.yaml").write_text(_DATASET.format(name="outside-secret"))
    os.symlink(str(outside / "secret.yaml"), root / "abs.yaml")
    os.symlink("../outside/secret.yaml", root / "rel.yaml")
    os.symlink(str(outside), root / "out")
    os.symlink("basic.yaml", root / "in.yaml")
    os.mkfifo(root / "fifo.yaml")
    monkeypatch.setattr(datasets, "datasets_dir", lambda: str(root))
    return root, outside


def test_reads_datasets_inside_the_directory(layout: tuple[Path, Path]) -> None:
    root, _ = layout
    for path in (
        "basic.yaml",
        str(root / "basic.yaml"),
        "in.yaml",
        "sub/more.yml",
        str(root / "sub" / "x" / ".." / "more.yml"),
    ):
        assert "name:" in read_dataset_text(path), path
    assert load_dataset(str(root / "basic.yaml")).name == "basic"


@pytest.mark.parametrize(
    ("path", "error"),
    [
        ("/etc/passwd", DatasetPathError),
        ("OUTSIDE", DatasetPathError),
        ("../outside/secret.yaml", PathLeavesWorkspaceError),
        ("abs.yaml", PathLeavesWorkspaceError),
        ("rel.yaml", PathLeavesWorkspaceError),
        ("out/secret.yaml", PathLeavesWorkspaceError),
        ("fifo.yaml", NotRegularFileError),
        ("missing.yaml", FileNotFoundError),
    ],
)
def test_refuses_datasets_outside_the_directory(layout: tuple[Path, Path], path: str, error: type[Exception]) -> None:
    _, outside = layout
    if path == "OUTSIDE":
        path = str(outside / "secret.yaml")
    result: list[BaseException] = []

    def read() -> None:
        try:
            read_dataset_text(path)
        except BaseException as exc:
            result.append(exc)

    worker = threading.Thread(target=read, daemon=True)
    worker.start()
    worker.join(5)
    assert not worker.is_alive(), "reading the dataset blocked"
    assert len(result) == 1
    assert isinstance(result[0], error), result[0]
    assert str(outside) not in str(result[0])


@pytest.mark.parametrize("provider_cls", [CodeForgeSimpleProvider, CodeForgeToolUseProvider, CodeForgeAgentProvider])
async def test_providers_read_only_inside_the_directory(layout: tuple[Path, Path], provider_cls: type) -> None:
    root, outside = layout
    tasks = await provider_cls(dataset_path=str(root / "basic.yaml")).load_tasks()
    assert [t.id for t in tasks] == ["t1"]
    for path in (str(outside / "secret.yaml"), "abs.yaml"):
        with pytest.raises(OSError):
            await provider_cls(dataset_path=path).load_tasks()


def test_legacy_dataset_loader_refuses_outside(layout: tuple[Path, Path]) -> None:
    _, outside = layout
    with pytest.raises(DatasetPathError):
        _benchmark_runners._dataset_to_task_specs(str(outside / "secret.yaml"))


def test_default_datasets_are_names() -> None:
    assert _benchmark_runners._resolve_default_dataset("codeforge_simple") == "basic-coding.yaml"
    assert _benchmark_runners._resolve_default_dataset("unknown") == ""


def test_datasets_dir_resolution(tmp_path: Path, monkeypatch: pytest.MonkeyPatch) -> None:
    repo = tmp_path / "repo"
    (repo / "configs" / "benchmarks").mkdir(parents=True)
    (repo / "workers").mkdir()
    checkout = tmp_path / "checkout"
    (checkout / "data" / "sets").mkdir(parents=True)

    def settings(directory: str, workspace: str = "/nonexistent") -> None:
        monkeypatch.setattr(
            datasets,
            "get_settings",
            lambda: SimpleNamespace(benchmark_datasets_dir=directory, workspace=workspace),
        )

    settings("/srv/datasets")
    assert datasets.datasets_dir() == "/srv/datasets"
    monkeypatch.chdir(repo)
    settings("configs/benchmarks")
    assert datasets.datasets_dir() == str(repo / "configs" / "benchmarks")
    monkeypatch.chdir(repo / "workers")  # the worker runs from workers/ in development
    assert datasets.datasets_dir() == str(repo / "configs" / "benchmarks")
    settings("data/sets", workspace=str(checkout))
    assert datasets.datasets_dir() == str(checkout / "data" / "sets")
