"""Tree-sitter based repository map generator.

Extracts symbol definitions and references from source files, builds a
cross-file dependency graph, ranks files via PageRank, and formats a
compact text map that fits within a token budget.
"""

from __future__ import annotations

import os
from dataclasses import dataclass
from typing import TYPE_CHECKING

import structlog
from tree_sitter_language_pack import get_parser

from codeforge._tree_sitter_common import (
    _CHARS_PER_TOKEN,
    _DEF_NODE_TYPES,
    _EXTENSION_MAP,
    _MAX_FILE_SIZE,
    _MAX_FILES,
    _SKIP_DIRS,
)

if TYPE_CHECKING:
    from tree_sitter import Node, Parser

    from codeforge.models import RepoMapResult

logger = structlog.get_logger()

# PageRank parameters
_PAGERANK_DAMPING = 0.85
_PAGERANK_ITERATIONS = 100
_PAGERANK_TOLERANCE = 1e-6


@dataclass(frozen=True)
class SymbolTag:
    """A single symbol extracted from source code."""

    rel_path: str
    line: int  # 1-indexed
    name: str
    kind: str  # "def" or "ref"
    scope: str  # "public" or "private"


class _DiGraph:
    """Minimal weighted directed graph using adjacency dicts."""

    def __init__(self) -> None:
        self._nodes: set[str] = set()
        # edges[source][target] = weight
        self._edges: dict[str, dict[str, float]] = {}

    def add_node(self, node: str) -> None:
        self._nodes.add(node)

    def add_edge(self, source: str, target: str, weight: float) -> None:
        self._nodes.add(source)
        self._nodes.add(target)
        if source not in self._edges:
            self._edges[source] = {}
        self._edges[source][target] = weight

    def has_edge(self, source: str, target: str) -> bool:
        return source in self._edges and target in self._edges[source]

    def edge_weight(self, source: str, target: str) -> float:
        return self._edges.get(source, {}).get(target, 0.0)

    def set_edge_weight(self, source: str, target: str, weight: float) -> None:
        if source in self._edges and target in self._edges[source]:
            self._edges[source][target] = weight

    @property
    def nodes(self) -> set[str]:
        return self._nodes

    def out_edges(self, node: str) -> dict[str, float]:
        """Return {target: weight} for all outgoing edges from node."""
        return self._edges.get(node, {})

    def in_edges(self, node: str) -> dict[str, float]:
        """Return {source: weight} for all incoming edges to node."""
        result: dict[str, float] = {}
        for source, targets in self._edges.items():
            if node in targets:
                result[source] = targets[node]
        return result

    def __len__(self) -> int:
        return len(self._nodes)


def _pagerank(graph: _DiGraph, damping: float = _PAGERANK_DAMPING) -> dict[str, float]:
    """Pure-Python weighted PageRank implementation.

    Iterative power method: no numpy/scipy dependency.
    """
    nodes = list(graph.nodes)
    n = len(nodes)
    if n == 0:
        return {}

    # Initialize uniform
    rank: dict[str, float] = dict.fromkeys(nodes, 1.0 / n)

    # Precompute outgoing weight sums per node
    out_weight: dict[str, float] = {}
    for node in nodes:
        edges = graph.out_edges(node)
        out_weight[node] = sum(edges.values()) if edges else 0.0

    for _ in range(_PAGERANK_ITERATIONS):
        new_rank: dict[str, float] = {}
        # Collect dangling node mass (nodes with no outgoing edges)
        dangling_sum = sum(rank[node] for node in nodes if out_weight[node] == 0.0)

        for node in nodes:
            # Sum of contributions from incoming edges
            incoming = graph.in_edges(node)
            s = 0.0
            for source, weight in incoming.items():
                total_out = out_weight[source]
                if total_out > 0.0:
                    s += rank[source] * weight / total_out

            new_rank[node] = (1.0 - damping) / n + damping * (s + dangling_sum / n)

        # Check convergence
        diff = sum(abs(new_rank[node] - rank[node]) for node in nodes)
        rank = new_rank
        if diff < _PAGERANK_TOLERANCE * n:
            break

    return rank


class RepoMapGenerator:
    """Generates a ranked, token-budgeted map of repository symbols."""

    def __init__(self, token_budget: int = 1024) -> None:
        self._token_budget = token_budget
        self._parsers: dict[str, Parser] = {}

    async def generate(
        self,
        workspace_path: str,
        active_files: list[str] | None = None,
    ) -> RepoMapResult:
        """Generate a repo map for the given workspace."""
        from codeforge.models import RepoMapResult

        active = active_files or []
        files = self._collect_files(workspace_path)

        if not files:
            return RepoMapResult(
                project_id="",
                map_text="",
                token_count=0,
                file_count=0,
                symbol_count=0,
                languages=[],
            )

        # Extract tags from all files
        all_tags: list[SymbolTag] = []
        languages_seen: set[str] = set()

        for abs_path in files:
            rel_path = os.path.relpath(abs_path, workspace_path)
            language = self._detect_language(abs_path)
            if language is None:
                continue
            languages_seen.add(language)
            tags = self._extract_tags(rel_path, abs_path, language)
            all_tags.extend(tags)

        if not all_tags:
            return RepoMapResult(
                project_id="",
                map_text="",
                token_count=0,
                file_count=len(files),
                symbol_count=0,
                languages=sorted(languages_seen),
            )

        # Build cross-file dependency graph and rank
        graph = self._build_graph(all_tags, active)
        ranked = self._rank_files(graph)

        # Sort tags by file rank, then by line number
        def_tags = [t for t in all_tags if t.kind == "def"]
        def_tags.sort(key=lambda t: (-ranked.get(t.rel_path, 0.0), t.line))

        # Format within token budget
        map_text = self._format_map(def_tags, self._token_budget)
        token_count = len(map_text) // _CHARS_PER_TOKEN

        return RepoMapResult(
            project_id="",
            map_text=map_text,
            token_count=token_count,
            file_count=len(files),
            symbol_count=len(def_tags),
            languages=sorted(languages_seen),
        )

    def _collect_files(self, workspace_path: str) -> list[str]:
        """Recursively collect source files, skipping ignored directories and large files."""
        collected: list[str] = []

        for dirpath, dirnames, filenames in os.walk(workspace_path):
            # Filter out ignored directories in-place to prevent descending
            dirnames[:] = [d for d in dirnames if d not in _SKIP_DIRS]

            for fname in filenames:
                if len(collected) >= _MAX_FILES:
                    return collected

                abs_path = os.path.join(dirpath, fname)
                _, ext = os.path.splitext(fname)

                if ext not in _EXTENSION_MAP:
                    continue

                try:
                    if os.path.getsize(abs_path) > _MAX_FILE_SIZE:
                        continue
                except OSError:
                    continue

                collected.append(abs_path)

        return collected

    def _detect_language(self, file_path: str) -> str | None:
        """Detect the tree-sitter language name from file extension."""
        _, ext = os.path.splitext(file_path)
        return _EXTENSION_MAP.get(ext)

    def _get_parser(self, language: str) -> Parser:
        """Get or create a cached parser for the given language."""
        if language not in self._parsers:
            self._parsers[language] = get_parser(language)
        return self._parsers[language]

    def _extract_tags(self, rel_path: str, abs_path: str, language: str) -> list[SymbolTag]:
        """Extract definition and reference tags from a single source file."""
        try:
            with open(abs_path, "rb") as f:
                source = f.read()
        except OSError:
            logger.warning("cannot read file", path=abs_path)
            return []

        try:
            parser = self._get_parser(language)
            tree = parser.parse(source)
        except Exception:
            logger.warning("parse failed", path=abs_path, language=language)
            return []

        def_types = _DEF_NODE_TYPES.get(language, frozenset())
        tags: list[SymbolTag] = []
        def_names: set[str] = set()

        # Walk top-level and one level of nesting for definitions
        self._walk_definitions(tree.root_node, rel_path, language, def_types, tags, def_names, depth=0)

        # Collect references: all identifier nodes not in the definition set
        self._walk_references(tree.root_node, rel_path, def_names, tags)

        return tags

    def _walk_definitions(
        self,
        node: Node,
        rel_path: str,
        language: str,
        def_types: frozenset[str],
        tags: list[SymbolTag],
        def_names: set[str],
        depth: int,
    ) -> None:
        """Recursively extract definition symbols from AST nodes."""
        if depth > 2:
            return

        for child in node.children:
            if child.type in def_types:
                name = self._extract_def_name(child, language)
                if name:
                    scope = "private" if name.startswith("_") else "public"
                    tags.append(
                        SymbolTag(
                            rel_path=rel_path,
                            line=child.start_point[0] + 1,
                            name=name,
                            kind="def",
                            scope=scope,
                        )
                    )
                    def_names.add(name)

            # Also look for definitions inside export_statement wrappers (TS/JS)
            if child.type == "export_statement":
                self._walk_definitions(child, rel_path, language, def_types, tags, def_names, depth + 1)

            # Recurse into class/struct/impl bodies for method definitions
            if child.type in {
                "class_definition",
                "class_declaration",
                "class_body",
                "impl_item",
                "declaration_list",
                "class_specifier",
                "interface_declaration",
                "interface_body",
            }:
                self._walk_definitions(child, rel_path, language, def_types, tags, def_names, depth + 1)

    def _extract_def_name(self, node: Node, language: str) -> str:  # noqa: C901
        """Extract the symbol name from a definition node."""
        # Try 'name' field first (works for most languages)
        name_node = node.child_by_field_name("name")
        if name_node:
            return name_node.text.decode()

        # Go: type_declaration -> type_spec -> name
        if node.type == "type_declaration":
            for child in node.children:
                if child.type == "type_spec":
                    spec_name = child.child_by_field_name("name")
                    if spec_name:
                        return spec_name.text.decode()

        # Go: const_declaration / var_declaration -> const_spec / var_spec -> name
        if node.type in {"const_declaration", "var_declaration"}:
            for child in node.children:
                if child.type in {"const_spec", "var_spec"}:
                    spec_name = child.child_by_field_name("name")
                    if spec_name:
                        return spec_name.text.decode()

        # TypeScript/JS: lexical_declaration -> variable_declarator -> name
        if node.type == "lexical_declaration":
            for child in node.children:
                if child.type == "variable_declarator":
                    decl_name = child.child_by_field_name("name")
                    if decl_name:
                        return decl_name.text.decode()

        # Python: assignment -> left
        if node.type == "assignment":
            left = node.child_by_field_name("left")
            if left and left.type == "identifier":
                return left.text.decode()

        return ""

    def _walk_references(
        self,
        node: Node,
        rel_path: str,
        def_names: set[str],
        tags: list[SymbolTag],
    ) -> None:
        """Walk the AST to collect identifier references (non-definition identifiers)."""
        if node.type == "identifier" and node.parent is not None:
            name = node.text.decode()
            # Only collect references to names that are NOT defined in this file
            # (cross-file references are more useful for the graph)
            if name not in def_names and len(name) >= 2:
                tags.append(
                    SymbolTag(
                        rel_path=rel_path,
                        line=node.start_point[0] + 1,
                        name=name,
                        kind="ref",
                        scope="public",
                    )
                )

        for child in node.children:
            self._walk_references(child, rel_path, def_names, tags)

    def _build_graph(self, tags: list[SymbolTag], active_files: list[str]) -> _DiGraph:  # noqa: C901
        """Build a directed graph where edges point from referencing files to defining files."""
        graph = _DiGraph()

        # Index definitions by name -> file
        defs_by_name: dict[str, set[str]] = {}
        for tag in tags:
            if tag.kind == "def":
                if tag.name not in defs_by_name:
                    defs_by_name[tag.name] = set()
                defs_by_name[tag.name].add(tag.rel_path)
                graph.add_node(tag.rel_path)

        # Add edges from referencing files to defining files
        active_set = frozenset(active_files)
        for tag in tags:
            if tag.kind != "ref":
                continue
            if tag.name not in defs_by_name:
                continue

            for def_file in defs_by_name[tag.name]:
                if def_file == tag.rel_path:
                    continue

                # Calculate edge weight
                weight = 1.0
                if len(tag.name) >= 8:
                    weight *= 10.0
                if tag.name.startswith("_"):
                    weight *= 0.1
                if tag.rel_path in active_set or def_file in active_set:
                    weight *= 50.0

                if graph.has_edge(tag.rel_path, def_file):
                    old_weight = graph.edge_weight(tag.rel_path, def_file)
                    graph.set_edge_weight(tag.rel_path, def_file, old_weight + weight)
                else:
                    graph.add_edge(tag.rel_path, def_file, weight=weight)

        return graph

    def _rank_files(self, graph: _DiGraph) -> dict[str, float]:
        """Rank files using PageRank on the dependency graph."""
        if len(graph) == 0:
            return {}
        return _pagerank(graph)

    def _format_map(self, ranked_tags: list[SymbolTag], token_budget: int) -> str:
        """Format the repo map text, fitting within the token budget via binary search."""
        if not ranked_tags:
            return ""

        char_budget = token_budget * _CHARS_PER_TOKEN

        # Group tags by file, preserving rank order
        files_order: list[str] = []
        tags_by_file: dict[str, list[SymbolTag]] = {}
        seen_files: set[str] = set()

        for tag in ranked_tags:
            if tag.rel_path not in seen_files:
                seen_files.add(tag.rel_path)
                files_order.append(tag.rel_path)
                tags_by_file[tag.rel_path] = []
            tags_by_file[tag.rel_path].append(tag)

        # Binary search: find the maximum number of files that fit
        lo, hi = 1, len(files_order)
        best = self._render(files_order[:1], tags_by_file)

        while lo <= hi:
            mid = (lo + hi) // 2
            rendered = self._render(files_order[:mid], tags_by_file)
            if len(rendered) <= char_budget:
                best = rendered
                lo = mid + 1
            else:
                hi = mid - 1

        return best

    @staticmethod
    def _render(files: list[str], tags_by_file: dict[str, list[SymbolTag]]) -> str:
        """Render the map text for a subset of files."""
        lines: list[str] = []
        for filepath in files:
            lines.append(filepath)
            lines.extend(f"    {tag.name}" for tag in tags_by_file.get(filepath, []))
            lines.append("")
        return "\n".join(lines)
