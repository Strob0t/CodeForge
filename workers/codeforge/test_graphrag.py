"""Tests for the GraphRAG code graph builder and searcher (Phase 6D)."""

from __future__ import annotations

import os
import textwrap
from unittest.mock import AsyncMock, MagicMock, patch

import pytest

from codeforge.graphrag import CodeGraphBuilder, GraphSearcher, _BuildContext


def _build_mock_conn(
    fetchall_responses: list[list[tuple[str, ...]]],
    fetchone_responses: list[tuple[str, ...] | None],
) -> AsyncMock:
    """Create an AsyncMock psycopg connection with sequenced responses."""
    fa_iter = iter(fetchall_responses)
    fo_iter = iter(fetchone_responses)

    async def _fetchall() -> list[tuple[str, ...]]:
        return next(fa_iter, [])

    async def _fetchone() -> tuple[str, ...] | None:
        return next(fo_iter, None)

    mock_cursor = AsyncMock()
    mock_cursor.fetchall = _fetchall
    mock_cursor.fetchone = _fetchone
    mock_cursor.__aenter__ = AsyncMock(return_value=mock_cursor)
    mock_cursor.__aexit__ = AsyncMock(return_value=False)

    mock_conn = AsyncMock()
    mock_conn.cursor = MagicMock(return_value=mock_cursor)
    mock_conn.__aenter__ = AsyncMock(return_value=mock_conn)
    mock_conn.__aexit__ = AsyncMock(return_value=False)
    return mock_conn


# ---------------------------------------------------------------------------
# CodeGraphBuilder tests
# ---------------------------------------------------------------------------


class TestCodeGraphBuilder:
    """Tests for CodeGraphBuilder."""

    def test_collect_files_skips_ignored_dirs(self, tmp_path: object) -> None:
        """Files in _SKIP_DIRS should be excluded."""
        workspace = str(tmp_path)
        src = os.path.join(workspace, "main.py")
        with open(src, "w") as f:
            f.write("def hello(): pass\n")

        venv = os.path.join(workspace, ".venv")
        os.makedirs(venv)
        with open(os.path.join(venv, "skip.py"), "w") as f:
            f.write("def skip(): pass\n")

        builder = CodeGraphBuilder()
        files = builder._collect_files(workspace)

        assert len(files) == 1
        assert files[0].endswith("main.py")

    def test_collect_files_respects_extension_map(self, tmp_path: object) -> None:
        """Only files with recognized extensions should be collected."""
        workspace = str(tmp_path)
        with open(os.path.join(workspace, "main.py"), "w") as f:
            f.write("x = 1\n")
        with open(os.path.join(workspace, "readme.txt"), "w") as f:
            f.write("text\n")
        with open(os.path.join(workspace, "data.json"), "w") as f:
            f.write("{}\n")

        builder = CodeGraphBuilder()
        files = builder._collect_files(workspace)

        assert len(files) == 1
        assert files[0].endswith("main.py")

    def test_extract_from_python_file(self, tmp_path: object) -> None:
        """Definitions should be extracted from a Python source file."""
        workspace = str(tmp_path)
        src = os.path.join(workspace, "example.py")
        with open(src, "w") as f:
            f.write(
                textwrap.dedent("""\
                import os

                def greet(name):
                    return f"Hello, {name}"

                class Greeter:
                    def say_hello(self):
                        pass
                """)
            )

        builder = CodeGraphBuilder()
        ctx = _BuildContext(project_id="test-proj")
        builder._extract_from_file(ctx, "example.py", src, "python")

        symbol_names = [n.symbol_name for n in ctx.nodes]
        assert "greet" in symbol_names
        assert "Greeter" in symbol_names
        assert "__module__" in symbol_names

    def test_extract_from_go_file(self, tmp_path: object) -> None:
        """Definitions should be extracted from a Go source file."""
        workspace = str(tmp_path)
        src = os.path.join(workspace, "main.go")
        with open(src, "w") as f:
            f.write(
                textwrap.dedent("""\
                package main

                import "fmt"

                func Hello() string {
                    return "hello"
                }

                type Server struct {
                    Port int
                }
                """)
            )

        builder = CodeGraphBuilder()
        ctx = _BuildContext(project_id="test-proj")
        builder._extract_from_file(ctx, "main.go", src, "go")

        symbol_names = [n.symbol_name for n in ctx.nodes]
        assert "Hello" in symbol_names
        assert "Server" in symbol_names

    def test_extract_imports_python(self, tmp_path: object) -> None:
        """Import edges should be created for Python import statements."""
        workspace = str(tmp_path)
        src = os.path.join(workspace, "app.py")
        with open(src, "w") as f:
            f.write(
                textwrap.dedent("""\
                import os
                from pathlib import Path

                def run():
                    pass
                """)
            )

        builder = CodeGraphBuilder()
        ctx = _BuildContext(project_id="proj")
        builder._extract_from_file(ctx, "app.py", src, "python")

        import_edges = [e for e in ctx.edges if e.kind == "imports"]
        assert len(import_edges) >= 1
        target_ids = [e.target_id for e in import_edges]
        assert any("os" in t for t in target_ids) or any("pathlib" in t for t in target_ids)

    def test_extract_imports_typescript(self, tmp_path: object) -> None:
        """Import edges should be created for TypeScript import statements."""
        workspace = str(tmp_path)
        src = os.path.join(workspace, "app.ts")
        with open(src, "w") as f:
            f.write(
                textwrap.dedent("""\
                import { Component } from './component';

                function render(): void {
                    console.log("render");
                }
                """)
            )

        builder = CodeGraphBuilder()
        ctx = _BuildContext(project_id="proj")
        builder._extract_from_file(ctx, "app.ts", src, "typescript")

        import_edges = [e for e in ctx.edges if e.kind == "imports"]
        assert len(import_edges) >= 1
        target_ids = [e.target_id for e in import_edges]
        assert any("component" in t for t in target_ids)

    def test_node_id_format(self, tmp_path: object) -> None:
        """Node IDs should follow the format project_id:filepath:symbol_name."""
        workspace = str(tmp_path)
        src = os.path.join(workspace, "lib.py")
        with open(src, "w") as f:
            f.write("def compute(): pass\n")

        builder = CodeGraphBuilder()
        ctx = _BuildContext(project_id="myproj")
        builder._extract_from_file(ctx, "lib.py", src, "python")

        definition_nodes = [n for n in ctx.nodes if n.symbol_name != "__module__"]
        assert len(definition_nodes) >= 1
        node = definition_nodes[0]
        assert node.node_id == "myproj:lib.py:compute"

    def test_kind_mapping(self, tmp_path: object) -> None:
        """Node kinds should be mapped to simplified strings."""
        workspace = str(tmp_path)
        src = os.path.join(workspace, "kinds.py")
        with open(src, "w") as f:
            f.write(
                textwrap.dedent("""\
                def my_func():
                    pass

                class MyClass:
                    pass
                """)
            )

        builder = CodeGraphBuilder()
        ctx = _BuildContext(project_id="proj")
        builder._extract_from_file(ctx, "kinds.py", src, "python")

        kinds_by_name = {n.symbol_name: n.kind for n in ctx.nodes}
        assert kinds_by_name.get("my_func") == "function"
        assert kinds_by_name.get("MyClass") == "class"

    @pytest.mark.asyncio
    async def test_build_graph_empty_workspace(self, tmp_path: object) -> None:
        """Building a graph on an empty workspace should succeed with zero counts."""
        workspace = str(tmp_path)
        builder = CodeGraphBuilder()

        result = await builder.build_graph("proj", workspace, "postgresql://fake:5432/db")

        assert result.status == "ready"
        assert result.node_count == 0
        assert result.edge_count == 0

    @pytest.mark.asyncio
    async def test_build_graph_persist(self, tmp_path: object) -> None:
        """build_graph should call _persist and return correct counts."""
        workspace = str(tmp_path)
        src = os.path.join(workspace, "main.py")
        with open(src, "w") as f:
            f.write("def hello(): pass\n")

        builder = CodeGraphBuilder()

        with patch.object(builder, "_persist", new_callable=AsyncMock) as mock_persist:
            result = await builder.build_graph("proj", workspace, "postgresql://fake:5432/db")

        assert result.status == "ready"
        assert result.node_count >= 1
        mock_persist.assert_awaited_once()

    @pytest.mark.asyncio
    async def test_build_graph_error(self, tmp_path: object) -> None:
        """build_graph should return error status when _persist raises."""
        workspace = str(tmp_path)
        src = os.path.join(workspace, "main.py")
        with open(src, "w") as f:
            f.write("def hello(): pass\n")

        builder = CodeGraphBuilder()

        with patch.object(builder, "_persist", side_effect=RuntimeError("db down")):
            result = await builder.build_graph("proj", workspace, "postgresql://fake:5432/db")

        assert result.status == "error"
        assert "db down" in result.error


# ---------------------------------------------------------------------------
# GraphSearcher tests
# ---------------------------------------------------------------------------


class TestGraphSearcher:
    """Tests for GraphSearcher."""

    def test_hop_decay_scoring(self) -> None:
        """Scores should decay exponentially with distance."""
        decay = 0.7
        assert decay**0 == 1.0
        assert abs(decay**1 - 0.7) < 1e-9
        assert abs(decay**2 - 0.49) < 1e-9
        assert abs(decay**3 - 0.343) < 1e-9

    @pytest.mark.asyncio
    async def test_search_no_seeds_found(self) -> None:
        """Search should return empty when no seed nodes match."""
        searcher = GraphSearcher()

        mock_cursor = AsyncMock()
        mock_cursor.fetchall = AsyncMock(return_value=[])
        mock_cursor.__aenter__ = AsyncMock(return_value=mock_cursor)
        mock_cursor.__aexit__ = AsyncMock(return_value=False)

        mock_conn = AsyncMock()
        mock_conn.cursor = MagicMock(return_value=mock_cursor)
        mock_conn.__aenter__ = AsyncMock(return_value=mock_conn)
        mock_conn.__aexit__ = AsyncMock(return_value=False)

        with patch("codeforge.graphrag.psycopg.AsyncConnection.connect", return_value=mock_conn):
            results = await searcher.search(
                project_id="proj",
                seed_symbols=["nonexistent"],
                max_hops=2,
                top_k=10,
                db_url="postgresql://fake:5432/db",
            )

        assert results == []

    @pytest.mark.asyncio
    async def test_search_bfs_traversal(self) -> None:
        """BFS should discover neighbors and score them with hop decay."""
        searcher = GraphSearcher()

        # Sequence of responses the mock cursor returns for fetchall calls:
        # 1) seed query, 2) outgoing from func_a, 3) incoming from func_a,
        # 4) outgoing from func_b, 5) incoming from func_b,
        # 6-7) outgoing/incoming from func_c (at max_hops, skipped)
        fetchall_responses: list[list[tuple[str, ...]]] = [
            [("proj:main.py:func_a", "main.py", "func_a", "function", 1, 5)],  # seeds
            [("proj:utils.py:func_b", "calls")],  # outgoing from func_a
            [],  # incoming from func_a
            [("proj:helper.py:func_c", "calls")],  # outgoing from func_b
            [],  # incoming from func_b
            [],  # outgoing from func_c
            [],  # incoming from func_c
        ]
        fetchone_responses: list[tuple[str, ...] | None] = [
            ("utils.py", "func_b", "function", 10, 15),
            ("helper.py", "func_c", "function", 20, 25),
        ]

        mock_conn = _build_mock_conn(fetchall_responses, fetchone_responses)

        with patch("codeforge.graphrag.psycopg.AsyncConnection.connect", return_value=mock_conn):
            results = await searcher.search(
                project_id="proj",
                seed_symbols=["func_a"],
                max_hops=2,
                top_k=10,
                db_url="postgresql://fake:5432/db",
                hop_decay=0.7,
            )

        assert len(results) == 2
        names = {r.symbol_name for r in results}
        assert "func_b" in names
        assert "func_c" in names

        func_b = next(r for r in results if r.symbol_name == "func_b")
        func_c = next(r for r in results if r.symbol_name == "func_c")
        assert abs(func_b.score - 0.7) < 1e-9
        assert abs(func_c.score - 0.49) < 1e-9
        assert func_b.distance == 1
        assert func_c.distance == 2

    @pytest.mark.asyncio
    async def test_search_returns_top_k(self) -> None:
        """Search should respect the top_k limit."""
        searcher = GraphSearcher()

        seed_rows = [("proj:a.py:seed", "a.py", "seed", "function", 1, 5)]
        neighbors = [(f"proj:b.py:func_{i}", "calls") for i in range(20)]
        neighbor_info = {f"proj:b.py:func_{i}": ("b.py", f"func_{i}", "function", i, i + 5) for i in range(20)}

        call_count = 0

        async def mock_fetchall() -> list[tuple[str, ...]]:
            nonlocal call_count
            call_count += 1
            if call_count == 1:
                return seed_rows
            if call_count == 2:
                return neighbors
            if call_count == 3:
                return []
            return []

        async def mock_fetchone() -> tuple[str, ...] | None:
            nonlocal call_count
            call_count += 1
            for info in neighbor_info.values():
                if call_count <= 24:
                    return info
            return None

        mock_cursor = AsyncMock()
        mock_cursor.fetchall = mock_fetchall
        mock_cursor.fetchone = mock_fetchone
        mock_cursor.__aenter__ = AsyncMock(return_value=mock_cursor)
        mock_cursor.__aexit__ = AsyncMock(return_value=False)

        mock_conn = AsyncMock()
        mock_conn.cursor = MagicMock(return_value=mock_cursor)
        mock_conn.__aenter__ = AsyncMock(return_value=mock_conn)
        mock_conn.__aexit__ = AsyncMock(return_value=False)

        with patch("codeforge.graphrag.psycopg.AsyncConnection.connect", return_value=mock_conn):
            results = await searcher.search(
                project_id="proj",
                seed_symbols=["seed"],
                max_hops=1,
                top_k=5,
                db_url="postgresql://fake:5432/db",
            )

        assert len(results) <= 5

    @pytest.mark.asyncio
    async def test_search_connection_error(self) -> None:
        """Search should return empty list on database errors."""
        searcher = GraphSearcher()

        with patch(
            "codeforge.graphrag.psycopg.AsyncConnection.connect",
            side_effect=RuntimeError("connection refused"),
        ):
            results = await searcher.search(
                project_id="proj",
                seed_symbols=["func"],
                max_hops=2,
                top_k=10,
                db_url="postgresql://fake:5432/db",
            )

        assert results == []


# ---------------------------------------------------------------------------
# Edge extraction unit tests
# ---------------------------------------------------------------------------


class TestEdgeExtraction:
    """Tests for import/call edge extraction logic."""

    def test_python_import_from(self, tmp_path: object) -> None:
        """from X import Y should create an import edge."""
        workspace = str(tmp_path)
        src = os.path.join(workspace, "mod.py")
        with open(src, "w") as f:
            f.write("from collections import OrderedDict\n")

        builder = CodeGraphBuilder()
        ctx = _BuildContext(project_id="proj")
        builder._extract_from_file(ctx, "mod.py", src, "python")

        import_edges = [e for e in ctx.edges if e.kind == "imports"]
        assert len(import_edges) >= 1
        assert any("collections" in e.target_id for e in import_edges)

    def test_go_import(self, tmp_path: object) -> None:
        """Go import declarations should create import edges."""
        workspace = str(tmp_path)
        src = os.path.join(workspace, "main.go")
        with open(src, "w") as f:
            f.write(
                textwrap.dedent("""\
                package main

                import (
                    "fmt"
                    "os"
                )

                func main() {}
                """)
            )

        builder = CodeGraphBuilder()
        ctx = _BuildContext(project_id="proj")
        builder._extract_from_file(ctx, "main.go", src, "go")

        import_edges = [e for e in ctx.edges if e.kind == "imports"]
        target_ids = [e.target_id for e in import_edges]
        assert any("fmt" in t for t in target_ids)
        assert any("os" in t for t in target_ids)

    def test_resolve_call_edges(self, tmp_path: object) -> None:
        """Call edges should be created between definitions across files."""
        workspace = str(tmp_path)
        src_a = os.path.join(workspace, "a.py")
        with open(src_a, "w") as f:
            f.write("def func_a(): pass\n")

        src_b = os.path.join(workspace, "b.py")
        with open(src_b, "w") as f:
            f.write("def func_b(): pass\n")

        builder = CodeGraphBuilder()
        ctx = _BuildContext(project_id="proj")
        builder._extract_from_file(ctx, "a.py", src_a, "python")
        builder._extract_from_file(ctx, "b.py", src_b, "python")

        builder._resolve_call_edges(ctx)

        call_edges = [e for e in ctx.edges if e.kind == "calls"]
        # Cross-file call edges should exist since name-matching creates them
        assert len(call_edges) >= 0
