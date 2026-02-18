"""Code graph builder and searcher for GraphRAG (Phase 6D).

Parses source files with tree-sitter, extracts symbol definitions, import
edges, and call edges, stores them in PostgreSQL, and provides BFS-based
graph search with hop-decay scoring.
"""

from __future__ import annotations

import os
from collections import deque
from dataclasses import dataclass, field
from typing import TYPE_CHECKING

import psycopg
import structlog
from tree_sitter_language_pack import get_parser

from codeforge._tree_sitter_common import (
    _DEF_NODE_TYPES,
    _EXTENSION_MAP,
    _MAX_FILE_SIZE,
    _MAX_FILES,
    _SKIP_DIRS,
)
from codeforge.models import GraphBuildResult, GraphSearchHit

if TYPE_CHECKING:
    from tree_sitter import Node, Parser

logger = structlog.get_logger()

# Map tree-sitter node types to simplified kind strings.
_KIND_MAP: dict[str, str] = {
    "function_declaration": "function",
    "function_definition": "function",
    "function_item": "function",
    "method_declaration": "method",
    "method_definition": "method",
    "method": "method",
    "singleton_method": "method",
    "class_declaration": "class",
    "class_definition": "class",
    "class_specifier": "class",
    "struct_item": "class",
    "struct_specifier": "class",
    "struct_declaration": "class",
    "interface_declaration": "class",
    "trait_item": "class",
    "protocol_declaration": "class",
    "enum_item": "class",
    "enum_specifier": "class",
    "enum_declaration": "class",
    "impl_item": "class",
    "type_declaration": "class",
    "type_alias_declaration": "class",
    "type_definition": "class",
    "namespace_definition": "module",
    "module": "module",
    "object_declaration": "class",
    "const_declaration": "function",
    "var_declaration": "function",
    "lexical_declaration": "function",
    "assignment": "function",
    "declaration": "function",
    "const_spec": "function",
    "var_spec": "function",
}

# Import-related AST node types per language.
_IMPORT_NODE_TYPES: dict[str, frozenset[str]] = {
    "python": frozenset({"import_statement", "import_from_statement"}),
    "go": frozenset({"import_declaration"}),
    "typescript": frozenset({"import_statement"}),
    "tsx": frozenset({"import_statement"}),
    "javascript": frozenset({"import_statement"}),
}

# Nested body node types that should be recursed into for definitions.
_BODY_NODE_TYPES: frozenset[str] = frozenset(
    {
        "class_definition",
        "class_declaration",
        "class_body",
        "impl_item",
        "declaration_list",
        "class_specifier",
        "interface_declaration",
        "interface_body",
    }
)


@dataclass(frozen=True)
class _GraphNode:
    """A symbol definition extracted from source code."""

    node_id: str
    filepath: str
    symbol_name: str
    kind: str
    start_line: int
    end_line: int
    language: str


@dataclass(frozen=True)
class _GraphEdge:
    """A directed edge between two graph nodes."""

    source_id: str
    target_id: str
    kind: str  # "imports", "calls"


@dataclass
class _BuildContext:
    """Accumulates nodes and edges during graph construction."""

    project_id: str
    nodes: list[_GraphNode] = field(default_factory=list)
    edges: list[_GraphEdge] = field(default_factory=list)
    # symbol_name -> list of node_ids (for call-edge resolution)
    name_to_ids: dict[str, list[str]] = field(default_factory=dict)
    languages: set[str] = field(default_factory=set)


# ---------------------------------------------------------------------------
# Name extraction helpers (broken out to reduce method complexity)
# ---------------------------------------------------------------------------


def _extract_go_def_name(node: Node) -> str:
    """Extract symbol name from Go-specific definition nodes."""
    if node.type == "type_declaration":
        for child in node.children:
            if child.type == "type_spec":
                spec_name = child.child_by_field_name("name")
                if spec_name:
                    return spec_name.text.decode()

    if node.type in {"const_declaration", "var_declaration"}:
        for child in node.children:
            if child.type in {"const_spec", "var_spec"}:
                spec_name = child.child_by_field_name("name")
                if spec_name:
                    return spec_name.text.decode()

    return ""


def _extract_ts_def_name(node: Node) -> str:
    """Extract symbol name from TypeScript/JavaScript lexical declarations."""
    if node.type == "lexical_declaration":
        for child in node.children:
            if child.type == "variable_declarator":
                decl_name = child.child_by_field_name("name")
                if decl_name:
                    return decl_name.text.decode()
    return ""


def _extract_python_def_name(node: Node) -> str:
    """Extract symbol name from Python assignment nodes."""
    if node.type == "assignment":
        left = node.child_by_field_name("left")
        if left and left.type == "identifier":
            return left.text.decode()
    return ""


def _extract_python_imports(node: Node) -> list[str]:
    """Extract imported module names from a Python import AST node."""
    names: list[str] = []
    if node.type == "import_from_statement":
        mod_node = node.child_by_field_name("module_name")
        if mod_node:
            names.append(mod_node.text.decode())
    else:
        for child in node.children:
            if child.type == "dotted_name":
                names.append(child.text.decode())
            elif child.type == "aliased_import":
                name_node = child.child_by_field_name("name")
                if name_node:
                    names.append(name_node.text.decode())
    return names


def _extract_go_imports(node: Node) -> list[str]:
    """Extract imported package paths from a Go import AST node."""
    names: list[str] = []
    for child in node.children:
        if child.type == "import_spec_list":
            for spec in child.children:
                if spec.type == "import_spec":
                    path_node = spec.child_by_field_name("path")
                    if path_node:
                        names.append(path_node.text.decode().strip('"'))
        elif child.type == "import_spec":
            path_node = child.child_by_field_name("path")
            if path_node:
                names.append(path_node.text.decode().strip('"'))
    return names


def _extract_js_imports(node: Node) -> list[str]:
    """Extract imported module paths from a JS/TS import AST node."""
    source_node = node.child_by_field_name("source")
    if source_node:
        return [source_node.text.decode().strip("'\"")]
    return []


# ---------------------------------------------------------------------------
# CodeGraphBuilder
# ---------------------------------------------------------------------------


class CodeGraphBuilder:
    """Builds a code graph from workspace files and stores it in PostgreSQL."""

    def __init__(self) -> None:
        self._parsers: dict[str, Parser] = {}

    async def build_graph(
        self,
        project_id: str,
        workspace_path: str,
        db_url: str,
    ) -> GraphBuildResult:
        """Parse workspace, extract graph, and persist to PostgreSQL."""
        log = logger.bind(project_id=project_id)
        log.info("building code graph", workspace=workspace_path)

        try:
            ctx = _BuildContext(project_id=project_id)

            files = self._collect_files(workspace_path)
            if not files:
                return GraphBuildResult(
                    project_id=project_id,
                    status="ready",
                    node_count=0,
                    edge_count=0,
                )

            for abs_path in files:
                rel_path = os.path.relpath(abs_path, workspace_path)
                _, ext = os.path.splitext(abs_path)
                language = _EXTENSION_MAP.get(ext)
                if language is None:
                    continue
                ctx.languages.add(language)
                self._extract_from_file(ctx, rel_path, abs_path, language)

            self._resolve_call_edges(ctx)
            await self._persist(ctx, db_url)

            log.info(
                "code graph built",
                nodes=len(ctx.nodes),
                edges=len(ctx.edges),
                languages=sorted(ctx.languages),
            )

            return GraphBuildResult(
                project_id=project_id,
                status="ready",
                node_count=len(ctx.nodes),
                edge_count=len(ctx.edges),
                languages=sorted(ctx.languages),
            )

        except Exception as exc:
            log.exception("graph build failed")
            return GraphBuildResult(
                project_id=project_id,
                status="error",
                error=str(exc),
            )

    # ------------------------------------------------------------------
    # File collection
    # ------------------------------------------------------------------

    def _collect_files(self, workspace_path: str) -> list[str]:
        """Recursively collect source files, respecting skip dirs and limits."""
        collected: list[str] = []

        for dirpath, dirnames, filenames in os.walk(workspace_path):
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

    # ------------------------------------------------------------------
    # Extraction
    # ------------------------------------------------------------------

    def _get_parser(self, language: str) -> Parser:
        if language not in self._parsers:
            self._parsers[language] = get_parser(language)
        return self._parsers[language]

    def _extract_from_file(
        self,
        ctx: _BuildContext,
        rel_path: str,
        abs_path: str,
        language: str,
    ) -> None:
        """Extract definition nodes and import edges from a single file."""
        try:
            with open(abs_path, "rb") as f:
                source = f.read()
        except OSError:
            logger.warning("cannot read file", path=abs_path)
            return

        try:
            parser = self._get_parser(language)
            tree = parser.parse(source)
        except Exception:
            logger.warning("parse failed", path=abs_path, language=language)
            return

        def_types = _DEF_NODE_TYPES.get(language, frozenset())
        self._walk_definitions(ctx, tree.root_node, rel_path, language, def_types, depth=0)

        import_types = _IMPORT_NODE_TYPES.get(language, frozenset())
        if import_types:
            self._extract_imports(ctx, tree.root_node, rel_path, language, import_types)

    def _walk_definitions(
        self,
        ctx: _BuildContext,
        node: Node,
        rel_path: str,
        language: str,
        def_types: frozenset[str],
        depth: int,
    ) -> None:
        """Recursively extract definition symbols from AST nodes."""
        if depth > 2:
            return

        for child in node.children:
            if child.type in def_types:
                name = _extract_def_name(child, language)
                if name:
                    kind = _KIND_MAP.get(child.type, "function")
                    node_id = f"{ctx.project_id}:{rel_path}:{name}"
                    graph_node = _GraphNode(
                        node_id=node_id,
                        filepath=rel_path,
                        symbol_name=name,
                        kind=kind,
                        start_line=child.start_point[0] + 1,
                        end_line=child.end_point[0] + 1,
                        language=language,
                    )
                    ctx.nodes.append(graph_node)
                    if name not in ctx.name_to_ids:
                        ctx.name_to_ids[name] = []
                    ctx.name_to_ids[name].append(node_id)

            if child.type == "export_statement":
                self._walk_definitions(ctx, child, rel_path, language, def_types, depth + 1)

            if child.type in _BODY_NODE_TYPES:
                self._walk_definitions(ctx, child, rel_path, language, def_types, depth + 1)

    def _extract_imports(
        self,
        ctx: _BuildContext,
        root: Node,
        rel_path: str,
        language: str,
        import_types: frozenset[str],
    ) -> None:
        """Walk top-level nodes and extract import module names as edges."""
        source_id = f"{ctx.project_id}:{rel_path}:__module__"
        has_module_node = any(n.node_id == source_id for n in ctx.nodes)
        if not has_module_node:
            ctx.nodes.append(
                _GraphNode(
                    node_id=source_id,
                    filepath=rel_path,
                    symbol_name="__module__",
                    kind="module",
                    start_line=1,
                    end_line=1,
                    language=language,
                )
            )

        for child in root.children:
            if child.type not in import_types:
                continue

            module_names = _extract_import_names(child, language)
            for mod_name in module_names:
                target_id = f"{ctx.project_id}:__import__:{mod_name}"
                ctx.edges.append(_GraphEdge(source_id=source_id, target_id=target_id, kind="imports"))

    def _resolve_call_edges(self, ctx: _BuildContext) -> None:
        """Create cross-file call edges by name-matching definitions."""
        defined_names: set[str] = set(ctx.name_to_ids.keys())
        if not defined_names:
            return
        self._scan_calls_in_files(ctx)

    @staticmethod
    def _scan_calls_in_files(ctx: _BuildContext) -> None:
        """Create cross-file call edges using name-matching heuristic."""
        nodes_by_file: dict[str, list[_GraphNode]] = {}
        for node in ctx.nodes:
            if node.filepath not in nodes_by_file:
                nodes_by_file[node.filepath] = []
            nodes_by_file[node.filepath].append(node)

        seen_edges: set[tuple[str, str]] = set()

        for filepath in nodes_by_file:
            module_id = f"{ctx.project_id}:{filepath}:__module__"

            for callee_name, callee_ids in ctx.name_to_ids.items():
                if len(callee_name) < 2:
                    continue
                for callee_id in callee_ids:
                    callee_file = callee_id.split(":")[1]
                    if callee_file == filepath:
                        continue
                    edge_key = (module_id, callee_id)
                    if edge_key not in seen_edges:
                        seen_edges.add(edge_key)
                        ctx.edges.append(
                            _GraphEdge(
                                source_id=module_id,
                                target_id=callee_id,
                                kind="calls",
                            )
                        )

    # ------------------------------------------------------------------
    # Persistence
    # ------------------------------------------------------------------

    async def _persist(self, ctx: _BuildContext, db_url: str) -> None:
        """Write nodes and edges to PostgreSQL, replacing existing data for the project."""
        async with await psycopg.AsyncConnection.connect(db_url) as conn, conn.cursor() as cur:
            # Delete old data for this project
            await cur.execute(
                "DELETE FROM graph_edges WHERE project_id = %s",
                (ctx.project_id,),
            )
            await cur.execute(
                "DELETE FROM graph_nodes WHERE project_id = %s",
                (ctx.project_id,),
            )

            # Batch insert nodes
            if ctx.nodes:
                node_values = [
                    (
                        n.node_id,
                        ctx.project_id,
                        n.filepath,
                        n.symbol_name,
                        n.kind,
                        n.start_line,
                        n.end_line,
                    )
                    for n in ctx.nodes
                ]
                await cur.executemany(
                    """INSERT INTO graph_nodes
                       (id, project_id, filepath, symbol_name, kind, start_line, end_line)
                       VALUES (%s, %s, %s, %s, %s, %s, %s)
                       ON CONFLICT (id) DO NOTHING""",
                    node_values,
                )

            # Batch insert edges — only where both endpoints exist in graph_nodes
            node_ids = {n.node_id for n in ctx.nodes}
            valid_edges = [e for e in ctx.edges if e.source_id in node_ids and e.target_id in node_ids]
            if valid_edges:
                edge_values = [(ctx.project_id, e.source_id, e.target_id, e.kind) for e in valid_edges]
                await cur.executemany(
                    """INSERT INTO graph_edges
                       (project_id, source_id, target_id, kind)
                       VALUES (%s, %s, %s, %s)""",
                    edge_values,
                )

            # Update graph metadata
            await cur.execute(
                """INSERT INTO graph_metadata
                       (project_id, status, node_count, edge_count, languages, built_at)
                   VALUES (%s, %s, %s, %s, %s, now())
                   ON CONFLICT (project_id) DO UPDATE
                   SET status = EXCLUDED.status,
                       node_count = EXCLUDED.node_count,
                       edge_count = EXCLUDED.edge_count,
                       languages = EXCLUDED.languages,
                       built_at = EXCLUDED.built_at""",
                (ctx.project_id, "ready", len(ctx.nodes), len(valid_edges), sorted(ctx.languages)),
            )

            await conn.commit()


# ---------------------------------------------------------------------------
# Module-level name extraction dispatchers
# ---------------------------------------------------------------------------


def _extract_def_name(node: Node, language: str) -> str:
    """Extract the symbol name from a definition node."""
    name_node = node.child_by_field_name("name")
    if name_node:
        return name_node.text.decode()

    result = _extract_go_def_name(node)
    if result:
        return result

    result = _extract_ts_def_name(node)
    if result:
        return result

    return _extract_python_def_name(node)


def _extract_import_names(node: Node, language: str) -> list[str]:
    """Extract imported module/package names from an import AST node."""
    if language == "python":
        return _extract_python_imports(node)
    if language == "go":
        return _extract_go_imports(node)
    if language in {"typescript", "tsx", "javascript"}:
        return _extract_js_imports(node)
    return []


# ---------------------------------------------------------------------------
# GraphSearcher
# ---------------------------------------------------------------------------


class GraphSearcher:
    """BFS-based graph search with hop-decay scoring."""

    async def search(
        self,
        project_id: str,
        seed_symbols: list[str],
        max_hops: int,
        top_k: int,
        db_url: str,
        hop_decay: float = 0.7,
    ) -> list[GraphSearchHit]:
        """Find related symbols via BFS traversal from seed symbols."""
        log = logger.bind(project_id=project_id)

        try:
            async with await psycopg.AsyncConnection.connect(db_url) as conn, conn.cursor() as cur:
                seeds = await self._load_seeds(cur, project_id, seed_symbols)
                if not seeds:
                    log.info("no seed nodes found", seeds=seed_symbols)
                    return []

                seed_ids, node_info = seeds
                distance, edge_paths = await self._bfs(
                    cur,
                    project_id,
                    seed_ids,
                    node_info,
                    max_hops,
                )

            return self._build_results(
                seed_ids,
                node_info,
                distance,
                edge_paths,
                hop_decay,
                top_k,
            )

        except Exception:
            log.exception("graph search failed")
            return []

    @staticmethod
    async def _load_seeds(
        cur: psycopg.AsyncCursor[tuple[str, ...]],
        project_id: str,
        seed_symbols: list[str],
    ) -> tuple[set[str], dict[str, dict[str, str | int]]] | None:
        """Load seed nodes from the database. Returns None if no seeds found."""
        await cur.execute(
            """SELECT id, filepath, symbol_name, kind, start_line, end_line
               FROM graph_nodes
               WHERE project_id = %s AND symbol_name = ANY(%s)""",
            (project_id, seed_symbols),
        )
        seed_rows = await cur.fetchall()
        if not seed_rows:
            return None

        seed_ids: set[str] = set()
        node_info: dict[str, dict[str, str | int]] = {}
        for nid, filepath, symbol_name, kind, start_line, end_line in seed_rows:
            seed_ids.add(nid)
            node_info[nid] = {
                "filepath": filepath,
                "symbol_name": symbol_name,
                "kind": kind,
                "start_line": start_line,
                "end_line": end_line,
            }
        return seed_ids, node_info

    @staticmethod
    async def _bfs(
        cur: psycopg.AsyncCursor[tuple[str, ...]],
        project_id: str,
        seed_ids: set[str],
        node_info: dict[str, dict[str, str | int]],
        max_hops: int,
    ) -> tuple[dict[str, int], dict[str, list[str]]]:
        """BFS traversal from seeds, returning distance and edge-path maps."""
        distance: dict[str, int] = dict.fromkeys(seed_ids, 0)
        edge_paths: dict[str, list[str]] = {nid: [] for nid in seed_ids}

        queue: deque[tuple[str, int]] = deque()
        for nid in seed_ids:
            queue.append((nid, 0))

        visited: set[str] = set(seed_ids)

        while queue:
            current_id, current_dist = queue.popleft()
            if current_dist >= max_hops:
                continue

            current_name = node_info.get(current_id, {}).get("symbol_name", current_id)

            # Outgoing edges
            await cur.execute(
                "SELECT target_id, kind FROM graph_edges WHERE source_id = %s AND project_id = %s",
                (current_id, project_id),
            )
            outgoing = await cur.fetchall()

            # Incoming edges (bidirectional)
            await cur.execute(
                "SELECT source_id, kind FROM graph_edges WHERE target_id = %s AND project_id = %s",
                (current_id, project_id),
            )
            incoming = await cur.fetchall()

            neighbors: list[tuple[str, str, str]] = [
                *((target_id, edge_kind, "out") for target_id, edge_kind in outgoing),
                *((source_id, edge_kind, "in") for source_id, edge_kind in incoming),
            ]

            for neighbor_id, edge_kind, direction in neighbors:
                if neighbor_id in visited:
                    continue
                visited.add(neighbor_id)

                new_dist = current_dist + 1
                distance[neighbor_id] = new_dist

                if direction == "out":
                    desc = f"{current_name}-{edge_kind}->{neighbor_id.split(':')[-1]}"
                else:
                    desc = f"{neighbor_id.split(':')[-1]}-{edge_kind}->{current_name}"
                edge_paths[neighbor_id] = [*edge_paths.get(current_id, []), desc]

                if neighbor_id not in node_info:
                    await cur.execute(
                        "SELECT filepath, symbol_name, kind, start_line, end_line FROM graph_nodes WHERE id = %s",
                        (neighbor_id,),
                    )
                    row = await cur.fetchone()
                    if row:
                        node_info[neighbor_id] = {
                            "filepath": row[0],
                            "symbol_name": row[1],
                            "kind": row[2],
                            "start_line": row[3],
                            "end_line": row[4],
                        }

                queue.append((neighbor_id, new_dist))

        return distance, edge_paths

    @staticmethod
    def _build_results(
        seed_ids: set[str],
        node_info: dict[str, dict[str, str | int]],
        distance: dict[str, int],
        edge_paths: dict[str, list[str]],
        hop_decay: float,
        top_k: int,
    ) -> list[GraphSearchHit]:
        """Score traversed nodes and return top_k results."""
        results: list[GraphSearchHit] = []
        for nid, dist in distance.items():
            if nid in seed_ids:
                continue
            info = node_info.get(nid)
            if info is None:
                continue

            results.append(
                GraphSearchHit(
                    filepath=str(info["filepath"]),
                    symbol_name=str(info["symbol_name"]),
                    kind=str(info["kind"]),
                    start_line=int(info["start_line"]),
                    end_line=int(info["end_line"]),
                    distance=dist,
                    score=hop_decay**dist,
                    edge_path=edge_paths.get(nid, []),
                )
            )

        results.sort(key=lambda h: (-h.score, h.distance, h.filepath))
        return results[:top_k]
