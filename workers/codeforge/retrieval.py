"""Hybrid retrieval engine combining BM25 keyword search with semantic embeddings.

Provides AST-aware code chunking via tree-sitter and Reciprocal Rank Fusion
to merge BM25 and cosine-similarity rankings into a single result list.
"""

from __future__ import annotations

import os
from dataclasses import dataclass
from typing import TYPE_CHECKING

import bm25s
import httpx
import numpy as np
import structlog
from tree_sitter_language_pack import get_parser

from codeforge._tree_sitter_common import (
    _DEF_NODE_TYPES,
    _EXTENSION_MAP,
    _MAX_FILE_SIZE,
    _MAX_FILES,
    _SKIP_DIRS,
)

if TYPE_CHECKING:
    from tree_sitter import Parser

logger = structlog.get_logger()

_DEFAULT_MAX_CHUNK_LINES = 100


# ---------------------------------------------------------------------------
# Data classes
# ---------------------------------------------------------------------------


@dataclass(frozen=True)
class CodeChunk:
    """A contiguous block of source code extracted from a file."""

    filepath: str
    start_line: int
    end_line: int
    content: str
    language: str
    symbol_name: str


@dataclass
class ProjectIndex:
    """In-memory index for a single project."""

    project_id: str
    chunks: list[CodeChunk]
    bm25: bm25s.BM25
    embeddings: np.ndarray
    file_count: int
    chunk_count: int
    embedding_model: str


@dataclass
class IndexStatus:
    """Status report for a project index."""

    project_id: str
    status: str
    file_count: int = 0
    chunk_count: int = 0
    embedding_model: str = ""
    error: str = ""


@dataclass(frozen=True)
class SearchResult:
    """A single search hit with merged ranking information."""

    filepath: str
    start_line: int
    end_line: int
    content: str
    language: str
    symbol_name: str
    score: float
    bm25_rank: int
    semantic_rank: int


# ---------------------------------------------------------------------------
# CodeChunker -- AST-aware code splitting
# ---------------------------------------------------------------------------


class CodeChunker:
    """Splits source files into chunks at definition boundaries using tree-sitter."""

    def __init__(self, max_chunk_lines: int = _DEFAULT_MAX_CHUNK_LINES) -> None:
        self._max_chunk_lines = max_chunk_lines
        self._parsers: dict[str, Parser] = {}

    def chunk_workspace(
        self,
        workspace_path: str,
        file_extensions: list[str] | None = None,
    ) -> list[CodeChunk]:
        """Walk the workspace and chunk all recognised source files."""
        ext_filter: set[str] | None = None
        if file_extensions:
            ext_filter = {e if e.startswith(".") else f".{e}" for e in file_extensions}

        chunks: list[CodeChunk] = []
        file_count = 0

        for dirpath, dirnames, filenames in os.walk(workspace_path):
            dirnames[:] = [d for d in dirnames if d not in _SKIP_DIRS]

            for fname in filenames:
                if file_count >= _MAX_FILES:
                    return chunks

                abs_path = os.path.join(dirpath, fname)
                _, ext = os.path.splitext(fname)

                if ext not in _EXTENSION_MAP:
                    continue
                if ext_filter is not None and ext not in ext_filter:
                    continue

                try:
                    if os.path.getsize(abs_path) > _MAX_FILE_SIZE:
                        continue
                except OSError:
                    continue

                rel_path = os.path.relpath(abs_path, workspace_path)
                language = _EXTENSION_MAP[ext]
                chunks.extend(self.chunk_file(abs_path, rel_path, language))
                file_count += 1

        return chunks

    def chunk_file(self, abs_path: str, rel_path: str, language: str) -> list[CodeChunk]:
        """Parse a single file and split at definition boundaries."""
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

        lines = source.decode(errors="replace").splitlines(keepends=True)
        if not lines:
            return []

        def_types = _DEF_NODE_TYPES.get(language, frozenset())
        # Collect top-level definition spans: (start_line_0idx, end_line_0idx, name)
        definitions: list[tuple[int, int, str]] = []
        for child in tree.root_node.children:
            if child.type in def_types:
                name = self._extract_name(child, language)
                definitions.append((child.start_point[0], child.end_point[0], name))
            # Handle export_statement wrappers (TS/JS)
            elif child.type == "export_statement":
                for grandchild in child.children:
                    if grandchild.type in def_types:
                        name = self._extract_name(grandchild, language)
                        definitions.append((grandchild.start_point[0], grandchild.end_point[0], name))

        definitions.sort(key=lambda d: d[0])

        chunks: list[CodeChunk] = []
        covered_up_to = 0  # 0-indexed line we have covered so far

        for start_0, end_0, sym_name in definitions:
            # Gap code before this definition
            if start_0 > covered_up_to:
                gap_text = "".join(lines[covered_up_to:start_0])
                if gap_text.strip():
                    chunks.append(
                        CodeChunk(
                            filepath=rel_path,
                            start_line=covered_up_to + 1,
                            end_line=start_0,
                            content=gap_text,
                            language=language,
                            symbol_name="",
                        )
                    )

            # Definition chunk (may need splitting if oversized)
            def_lines = lines[start_0 : end_0 + 1]
            def_text = "".join(def_lines)
            num_lines = end_0 - start_0 + 1

            if num_lines > self._max_chunk_lines:
                # Split oversized definition into sub-chunks
                for offset in range(0, num_lines, self._max_chunk_lines):
                    sub_start = start_0 + offset
                    sub_end = min(start_0 + offset + self._max_chunk_lines - 1, end_0)
                    sub_text = "".join(lines[sub_start : sub_end + 1])
                    suffix = (
                        f" (part {offset // self._max_chunk_lines + 1})" if num_lines > self._max_chunk_lines else ""
                    )
                    chunks.append(
                        CodeChunk(
                            filepath=rel_path,
                            start_line=sub_start + 1,
                            end_line=sub_end + 1,
                            content=sub_text,
                            language=language,
                            symbol_name=f"{sym_name}{suffix}" if sym_name else "",
                        )
                    )
            else:
                chunks.append(
                    CodeChunk(
                        filepath=rel_path,
                        start_line=start_0 + 1,
                        end_line=end_0 + 1,
                        content=def_text,
                        language=language,
                        symbol_name=sym_name,
                    )
                )

            covered_up_to = end_0 + 1

        # Trailing gap after last definition
        if covered_up_to < len(lines):
            tail_text = "".join(lines[covered_up_to:])
            if tail_text.strip():
                chunks.append(
                    CodeChunk(
                        filepath=rel_path,
                        start_line=covered_up_to + 1,
                        end_line=len(lines),
                        content=tail_text,
                        language=language,
                        symbol_name="",
                    )
                )

        # Fallback: if no definitions found, emit the whole file as a single chunk
        if not definitions and lines:
            full_text = "".join(lines)
            if full_text.strip():
                chunks.append(
                    CodeChunk(
                        filepath=rel_path,
                        start_line=1,
                        end_line=len(lines),
                        content=full_text,
                        language=language,
                        symbol_name="",
                    )
                )

        return chunks

    # ------------------------------------------------------------------
    # Internal helpers
    # ------------------------------------------------------------------

    def _get_parser(self, language: str) -> Parser:
        if language not in self._parsers:
            self._parsers[language] = get_parser(language)
        return self._parsers[language]

    @staticmethod
    def _extract_name(node: object, language: str) -> str:
        """Extract the symbol name from a definition AST node."""
        name_node = node.child_by_field_name("name")  # type: ignore[union-attr]
        if name_node:
            return name_node.text.decode()  # type: ignore[union-attr]

        # Go: type_declaration -> type_spec -> name
        if node.type == "type_declaration":  # type: ignore[union-attr]
            for child in node.children:  # type: ignore[union-attr]
                if child.type == "type_spec":
                    spec_name = child.child_by_field_name("name")
                    if spec_name:
                        return spec_name.text.decode()

        # Go: const_declaration / var_declaration -> *_spec -> name
        if node.type in {"const_declaration", "var_declaration"}:  # type: ignore[union-attr]
            for child in node.children:  # type: ignore[union-attr]
                if child.type in {"const_spec", "var_spec"}:
                    spec_name = child.child_by_field_name("name")
                    if spec_name:
                        return spec_name.text.decode()

        # TS/JS: lexical_declaration -> variable_declarator -> name
        if node.type == "lexical_declaration":  # type: ignore[union-attr]
            for child in node.children:  # type: ignore[union-attr]
                if child.type == "variable_declarator":
                    decl_name = child.child_by_field_name("name")
                    if decl_name:
                        return decl_name.text.decode()

        # Python: assignment -> left
        if node.type == "assignment":  # type: ignore[union-attr]
            left = node.child_by_field_name("left")  # type: ignore[union-attr]
            if left and left.type == "identifier":
                return left.text.decode()

        return ""


# ---------------------------------------------------------------------------
# HybridRetriever -- BM25 + semantic search with RRF fusion
# ---------------------------------------------------------------------------


@dataclass
class _RetrieverConfig:
    """Internal configuration for the retriever HTTP client."""

    base_url: str
    api_key: str
    embedding_model: str = "text-embedding-3-small"


class HybridRetriever:
    """Combines BM25 keyword search with LiteLLM embedding cosine similarity."""

    def __init__(self, litellm_url: str = "http://localhost:4000", litellm_key: str = "") -> None:
        self._indexes: dict[str, ProjectIndex] = {}
        self._chunker = CodeChunker()
        base_url = litellm_url.rstrip("/")
        headers: dict[str, str] = {"Content-Type": "application/json"}
        if litellm_key:
            headers["Authorization"] = f"Bearer {litellm_key}"
        self._client = httpx.AsyncClient(base_url=base_url, headers=headers, timeout=120.0)

    # ------------------------------------------------------------------
    # Public API
    # ------------------------------------------------------------------

    async def build_index(
        self,
        project_id: str,
        workspace_path: str,
        embedding_model: str = "text-embedding-3-small",
        file_extensions: list[str] | None = None,
    ) -> IndexStatus:
        """Chunk workspace, build BM25 index and compute embeddings."""
        log = logger.bind(project_id=project_id)
        log.info("building retrieval index", workspace=workspace_path)

        try:
            chunks = self._chunker.chunk_workspace(workspace_path, file_extensions)
            if not chunks:
                status = IndexStatus(
                    project_id=project_id,
                    status="empty",
                    embedding_model=embedding_model,
                )
                log.info("index empty, no chunks found")
                return status

            # Build BM25 index
            corpus = [chunk.content for chunk in chunks]
            corpus_tokens = bm25s.tokenize(corpus)
            bm25 = bm25s.BM25()
            bm25.index(corpus_tokens)

            # Compute embeddings via LiteLLM
            embeddings = await self._embed_texts(corpus, embedding_model)

            # Count unique files
            file_set: set[str] = set()
            for chunk in chunks:
                file_set.add(chunk.filepath)

            index = ProjectIndex(
                project_id=project_id,
                chunks=chunks,
                bm25=bm25,
                embeddings=embeddings,
                file_count=len(file_set),
                chunk_count=len(chunks),
                embedding_model=embedding_model,
            )
            self._indexes[project_id] = index

            log.info("index built", files=index.file_count, chunks=index.chunk_count)
            return IndexStatus(
                project_id=project_id,
                status="ready",
                file_count=index.file_count,
                chunk_count=index.chunk_count,
                embedding_model=embedding_model,
            )

        except Exception as exc:
            log.exception("index build failed")
            return IndexStatus(
                project_id=project_id,
                status="error",
                error=str(exc),
            )

    async def search(
        self,
        project_id: str,
        query: str,
        top_k: int = 20,
        bm25_weight: float = 0.5,
        semantic_weight: float = 0.5,
    ) -> list[SearchResult]:
        """Search an indexed project using hybrid BM25 + semantic retrieval."""
        index = self._indexes.get(project_id)
        if index is None:
            logger.warning("no index for project", project_id=project_id)
            return []

        n_chunks = len(index.chunks)
        if n_chunks == 0:
            return []

        effective_k = min(top_k, n_chunks)

        # BM25 retrieval
        query_tokens = bm25s.tokenize([query])
        bm25_results, _bm25_scores = index.bm25.retrieve(query_tokens, k=min(n_chunks, n_chunks))
        # bm25_results shape: (1, k) -- indices into chunks
        bm25_ranking: list[int] = [int(idx) for idx in bm25_results[0]]

        # Semantic retrieval
        query_embedding = await self._embed_texts([query], index.embedding_model)
        query_vec = query_embedding[0]
        cosine_scores = self._cosine_similarity(query_vec, index.embeddings)
        semantic_ranking: list[int] = list(np.argsort(-cosine_scores))

        # RRF fusion
        fused = self._rrf_fuse(bm25_ranking, semantic_ranking)

        # Build results
        results: list[SearchResult] = []
        for chunk_idx, score in fused[:effective_k]:
            chunk = index.chunks[chunk_idx]
            bm25_rank = bm25_ranking.index(chunk_idx) + 1 if chunk_idx in bm25_ranking else n_chunks
            sem_rank = semantic_ranking.index(chunk_idx) + 1
            results.append(
                SearchResult(
                    filepath=chunk.filepath,
                    start_line=chunk.start_line,
                    end_line=chunk.end_line,
                    content=chunk.content,
                    language=chunk.language,
                    symbol_name=chunk.symbol_name,
                    score=score,
                    bm25_rank=bm25_rank,
                    semantic_rank=sem_rank,
                )
            )

        return results

    def get_index_status(self, project_id: str) -> IndexStatus:
        """Return the status of a project's index."""
        index = self._indexes.get(project_id)
        if index is None:
            return IndexStatus(project_id=project_id, status="not_found")
        return IndexStatus(
            project_id=project_id,
            status="ready",
            file_count=index.file_count,
            chunk_count=index.chunk_count,
            embedding_model=index.embedding_model,
        )

    def drop_index(self, project_id: str) -> bool:
        """Remove a project's index from memory."""
        if project_id in self._indexes:
            del self._indexes[project_id]
            return True
        return False

    async def close(self) -> None:
        """Close the HTTP client."""
        await self._client.aclose()

    # ------------------------------------------------------------------
    # Internal helpers
    # ------------------------------------------------------------------

    async def _embed_texts(self, texts: list[str], model: str = "text-embedding-3-small") -> np.ndarray:
        """Batch-embed texts via the LiteLLM /v1/embeddings endpoint."""
        resp = await self._client.post(
            "/v1/embeddings",
            json={"input": texts, "model": model},
        )
        resp.raise_for_status()
        data = resp.json()

        # Sort by index to ensure correct ordering
        embeddings_data: list[dict[str, object]] = data.get("data", [])
        embeddings_data.sort(key=lambda d: int(d.get("index", 0)))

        vectors = [item["embedding"] for item in embeddings_data]
        return np.array(vectors, dtype=np.float32)

    @staticmethod
    def _cosine_similarity(query_vec: np.ndarray, matrix: np.ndarray) -> np.ndarray:
        """Compute cosine similarity between a query vector and a matrix of vectors."""
        query_norm = np.linalg.norm(query_vec)
        if query_norm == 0.0:
            return np.zeros(matrix.shape[0], dtype=np.float32)
        matrix_norms = np.linalg.norm(matrix, axis=1)
        # Avoid division by zero
        matrix_norms = np.where(matrix_norms == 0.0, 1.0, matrix_norms)
        return np.dot(matrix, query_vec) / (matrix_norms * query_norm)

    @staticmethod
    def _rrf_fuse(
        bm25_ranking: list[int],
        semantic_ranking: list[int],
        k: int = 60,
    ) -> list[tuple[int, float]]:
        """Reciprocal Rank Fusion of two rankings.

        Returns a list of (chunk_index, score) sorted by descending score.
        """
        scores: dict[int, float] = {}

        for rank, chunk_idx in enumerate(bm25_ranking):
            scores[chunk_idx] = scores.get(chunk_idx, 0.0) + 1.0 / (k + rank + 1)

        for rank, chunk_idx in enumerate(semantic_ranking):
            scores[chunk_idx] = scores.get(chunk_idx, 0.0) + 1.0 / (k + rank + 1)

        fused = sorted(scores.items(), key=lambda item: item[1], reverse=True)
        return fused
