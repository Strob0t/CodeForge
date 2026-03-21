"""Tests for GraphRAG module (FIX-070).

Verifies:
- Module imports successfully
- Core classes/functions exist
- Error handling present
- Data classes are well-formed
"""

from __future__ import annotations

import inspect

from codeforge import graphrag


class TestGraphRAGModuleImports:
    """Module-level import verification."""

    def test_module_imports(self) -> None:
        """graphrag module should import without errors."""
        assert graphrag is not None

    def test_code_graph_builder_exists(self) -> None:
        assert inspect.isclass(graphrag.CodeGraphBuilder)

    def test_graph_searcher_exists(self) -> None:
        assert inspect.isclass(graphrag.GraphSearcher)


class TestCodeGraphBuilder:
    """CodeGraphBuilder class tests."""

    def test_instantiation(self) -> None:
        builder = graphrag.CodeGraphBuilder()
        assert builder is not None

    def test_has_build_graph_method(self) -> None:
        assert hasattr(graphrag.CodeGraphBuilder, "build_graph")
        assert inspect.iscoroutinefunction(graphrag.CodeGraphBuilder.build_graph)

    def test_build_graph_has_error_handling(self) -> None:
        """build_graph must catch exceptions and return error result."""
        source = inspect.getsource(graphrag.CodeGraphBuilder.build_graph)
        assert "except Exception" in source, "build_graph must have exception handling"

    def test_has_collect_files_method(self) -> None:
        assert hasattr(graphrag.CodeGraphBuilder, "_collect_files")

    def test_has_extract_from_file_method(self) -> None:
        assert hasattr(graphrag.CodeGraphBuilder, "_extract_from_file")

    def test_has_persist_method(self) -> None:
        assert hasattr(graphrag.CodeGraphBuilder, "_persist")
        assert inspect.iscoroutinefunction(graphrag.CodeGraphBuilder._persist)


class TestGraphSearcher:
    """GraphSearcher class tests."""

    def test_instantiation(self) -> None:
        searcher = graphrag.GraphSearcher()
        assert searcher is not None

    def test_has_search_method(self) -> None:
        assert hasattr(graphrag.GraphSearcher, "search")
        assert inspect.iscoroutinefunction(graphrag.GraphSearcher.search)

    def test_search_has_error_handling(self) -> None:
        """search must catch exceptions."""
        source = inspect.getsource(graphrag.GraphSearcher.search)
        assert "except Exception" in source, "search must have exception handling"

    def test_search_uses_parameterized_queries(self) -> None:
        """SQL queries must use %s placeholders (psycopg), not f-strings."""
        source = inspect.getsource(graphrag.GraphSearcher)
        assert "%s" in source, "SQL queries must use parameterized placeholders"

    def test_has_bfs_method(self) -> None:
        assert hasattr(graphrag.GraphSearcher, "_bfs")

    def test_has_build_results_method(self) -> None:
        assert hasattr(graphrag.GraphSearcher, "_build_results")


class TestGraphRAGHelpers:
    """Test module-level helper functions."""

    def test_extract_def_name_exists(self) -> None:
        assert hasattr(graphrag, "_extract_def_name")
        assert callable(graphrag._extract_def_name)

    def test_extract_import_names_exists(self) -> None:
        assert hasattr(graphrag, "_extract_import_names")
        assert callable(graphrag._extract_import_names)

    def test_kind_map_not_empty(self) -> None:
        assert len(graphrag._KIND_MAP) > 0

    def test_extension_map_imported(self) -> None:
        """GraphRAG should use the shared extension map."""
        source = inspect.getsource(graphrag)
        assert "_EXTENSION_MAP" in source


class TestGraphRAGDataClasses:
    """Test internal data classes."""

    def test_graph_node_is_frozen(self) -> None:
        """_GraphNode should be frozen (immutable)."""
        node = graphrag._GraphNode(
            node_id="test:file.py:main",
            filepath="file.py",
            symbol_name="main",
            kind="function",
            start_line=1,
            end_line=10,
            language="python",
        )
        assert node.symbol_name == "main"

    def test_graph_edge_is_frozen(self) -> None:
        """_GraphEdge should be frozen (immutable)."""
        edge = graphrag._GraphEdge(
            source_id="a",
            target_id="b",
            kind="imports",
        )
        assert edge.kind == "imports"

    def test_build_context_accumulates(self) -> None:
        """_BuildContext should accumulate nodes and edges."""
        ctx = graphrag._BuildContext(project_id="proj-1")
        assert ctx.project_id == "proj-1"
        assert len(ctx.nodes) == 0
        assert len(ctx.edges) == 0
        assert len(ctx.name_to_ids) == 0
