"""Repo map, retrieval and GraphRAG collectors stay inside the workspace (KI-95).

They read workspace files in the worker's own process: symlinks that leave the
workspace, FIFOs, sockets and devices are skipped (and counted once per run),
symlinked directories are never walked, and a FIFO never blocks a run.
"""

from __future__ import annotations

import os
import threading
from typing import TYPE_CHECKING

import pytest
from structlog.testing import capture_logs

from codeforge._tree_sitter_common import _EXTENSION_MAP, collect_source_files
from codeforge.graphrag import CodeGraphBuilder
from codeforge.repomap import RepoMapGenerator
from codeforge.retrieval import CodeChunker
from codeforge.workspace_fs import WorkspaceRoot

if TYPE_CHECKING:
    from collections.abc import Callable
    from pathlib import Path

OUTSIDE_SYMBOL = "outside_only_function"


@pytest.fixture
def ws(tmp_path: Path) -> Path:
    workspace = tmp_path / "ws"
    outside = tmp_path / "out"
    (workspace / "pkg").mkdir(parents=True)
    (outside / "deep").mkdir(parents=True)
    (outside / "secret.py").write_text(f"def {OUTSIDE_SYMBOL}():\n    return 1\n")
    (outside / "deep" / "more.py").write_text(f"def {OUTSIDE_SYMBOL}_deep():\n    return 2\n")
    (workspace / "pkg" / "mod.py").write_text("def inside_function():\n    return 0\n")
    (workspace / "leak.py").symlink_to("../out/secret.py")  # leaves the workspace
    (workspace / "leak_abs.py").symlink_to(outside / "secret.py")  # absolute: leaves it too
    (workspace / "dangling.py").symlink_to("gone.py")
    (workspace / "outdir").symlink_to("../out")  # a directory outside: never walked
    (workspace / "alias.py").symlink_to("pkg/mod.py")  # inside: indexed under its own path
    os.mkfifo(workspace / "pipe.py")
    return workspace


def _without_blocking[T](fn: Callable[[], T], seconds: float = 20.0) -> T:
    result: list[T] = []
    thread = threading.Thread(target=lambda: result.append(fn()), daemon=True)
    thread.start()
    thread.join(seconds)
    assert not thread.is_alive(), "the indexer blocked"
    return result[0]


def test_collect_source_files(ws: Path) -> None:
    with WorkspaceRoot(str(ws)) as root:
        files, skipped = collect_source_files(root, _EXTENSION_MAP)
    assert sorted(files) == ["alias.py", "pkg/mod.py"]
    assert skipped == 4  # leak.py, leak_abs.py, dangling.py, pipe.py


def test_collect_source_files_size_cap_is_not_a_skip(ws: Path) -> None:
    (ws / "big.py").write_bytes(b"x = 1\n" * 30_000)
    with WorkspaceRoot(str(ws)) as root:
        files, skipped = collect_source_files(root, _EXTENSION_MAP)
    assert "big.py" not in files
    assert skipped == 4


def _skip_logs(logs: list[dict[str, object]], indexer: str) -> list[dict[str, object]]:
    return [
        entry for entry in logs if entry.get("event") == "skipped workspace entries" and entry["indexer"] == indexer
    ]


def test_repomap(ws: Path) -> None:
    with capture_logs() as logs:
        result = _without_blocking(lambda: RepoMapGenerator()._generate_sync(str(ws), None))
    assert OUTSIDE_SYMBOL not in result.map_text
    assert "inside_function" in result.map_text
    assert result.file_count == 2
    assert [entry["skipped"] for entry in _skip_logs(logs, "repomap")] == [4]


def test_retrieval(ws: Path) -> None:
    with capture_logs() as logs:
        per_file = _without_blocking(lambda: CodeChunker().chunk_workspace_by_file(str(ws)))
    assert sorted(per_file) == ["alias.py", "pkg/mod.py"]
    text = "".join(c.content for _, chunks in per_file.values() for c in chunks)
    assert OUTSIDE_SYMBOL not in text
    assert [entry["skipped"] for entry in _skip_logs(logs, "retrieval")] == [4]


def test_graphrag(ws: Path) -> None:
    with capture_logs() as logs:
        ctx = _without_blocking(lambda: CodeGraphBuilder()._extract_graph("p1", str(ws)))
    symbols = {node.symbol_name for node in ctx.nodes}
    assert "inside_function" in symbols
    assert not [s for s in symbols if s.startswith(OUTSIDE_SYMBOL)]
    assert [entry["skipped"] for entry in _skip_logs(logs, "graphrag")] == [4]


def test_nothing_skipped_logs_nothing(tmp_path: Path) -> None:
    (tmp_path / "a.py").write_text("def f():\n    return 1\n")
    with capture_logs() as logs:
        RepoMapGenerator()._generate_sync(str(tmp_path), None)
    assert _skip_logs(logs, "repomap") == []


@pytest.mark.parametrize("build", ["repomap", "retrieval", "graphrag"])
def test_symlinked_workspace_is_not_indexed(ws: Path, tmp_path: Path, build: str) -> None:
    """A workspace directory an agent replaced by a symlink is not indexed at all."""
    swapped = tmp_path / "swapped"
    swapped.symlink_to(tmp_path / "out")
    if build == "repomap":
        assert RepoMapGenerator()._generate_sync(str(swapped), None).file_count == 0
    elif build == "retrieval":
        assert CodeChunker().chunk_workspace_by_file(str(swapped)) == {}
    else:
        assert CodeGraphBuilder()._extract_graph("p1", str(swapped)) is None
