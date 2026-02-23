"""Hybrid retrieval engine combining BM25 keyword search with semantic embeddings.

Provides AST-aware code chunking via tree-sitter and Reciprocal Rank Fusion
to merge BM25 and cosine-similarity rankings into a single result list.
"""

from __future__ import annotations

import asyncio
import hashlib
import os
from dataclasses import dataclass, field
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
from codeforge.models import RetrievalSearchHit

if TYPE_CHECKING:
    from tree_sitter import Parser

    from codeforge.llm import LiteLLMClient

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


@dataclass(frozen=True)
class FileHashRecord:
    """Tracks a file's content hash and its chunk span in the index."""

    filepath: str
    content_hash: str
    chunk_start: int  # index into chunks list
    chunk_count: int  # number of chunks from this file


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
    file_hashes: dict[str, FileHashRecord] = field(default_factory=dict)


@dataclass
class IndexStatus:
    """Status report for a project index."""

    project_id: str
    status: str
    file_count: int = 0
    chunk_count: int = 0
    embedding_model: str = ""
    error: str = ""
    incremental: bool = False
    files_changed: int = 0
    files_unchanged: int = 0


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
        per_file = self.chunk_workspace_by_file(workspace_path, file_extensions)
        chunks: list[CodeChunk] = []
        for _rel, file_chunks in per_file.values():
            chunks.extend(file_chunks)
        return chunks

    def chunk_workspace_by_file(
        self,
        workspace_path: str,
        file_extensions: list[str] | None = None,
    ) -> dict[str, tuple[str, list[CodeChunk]]]:
        """Walk workspace and return {rel_path: (content_hash, chunks)} per file.

        The content hash is the SHA-256 hex digest of the raw file bytes.
        """
        ext_filter: set[str] | None = None
        if file_extensions:
            ext_filter = {e if e.startswith(".") else f".{e}" for e in file_extensions}

        result: dict[str, tuple[str, list[CodeChunk]]] = {}
        file_count = 0

        for dirpath, dirnames, filenames in os.walk(workspace_path):
            dirnames[:] = [d for d in dirnames if d not in _SKIP_DIRS]

            for fname in filenames:
                if file_count >= _MAX_FILES:
                    return result

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
                content_hash = _file_sha256(abs_path)
                file_chunks = self.chunk_file(abs_path, rel_path, language)
                result[rel_path] = (content_hash, file_chunks)
                file_count += 1

        return result

    def chunk_file(self, abs_path: str, rel_path: str, language: str) -> list[CodeChunk]:  # noqa: C901
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
    def _extract_name(node: object, language: str) -> str:  # noqa: C901
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
# File hashing helpers
# ---------------------------------------------------------------------------


def _file_sha256(path: str) -> str:
    """Compute the SHA-256 hex digest of a file's contents."""
    h = hashlib.sha256()
    with open(path, "rb") as f:
        for block in iter(lambda: f.read(8192), b""):
            h.update(block)
    return h.hexdigest()


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
        """Chunk workspace, build BM25 index and compute embeddings.

        Supports incremental builds: if a prior index exists with the same
        embedding model, only changed/new files are re-chunked and re-embedded.
        Unchanged files reuse their chunks and embedding rows from the prior index.
        """
        log = logger.bind(project_id=project_id)
        log.info("building retrieval index", workspace=workspace_path)

        try:
            # Collect files with per-file content hashes.
            per_file = self._chunker.chunk_workspace_by_file(workspace_path, file_extensions)
            if not per_file:
                log.info("index empty, no files found")
                return IndexStatus(
                    project_id=project_id,
                    status="empty",
                    embedding_model=embedding_model,
                )

            # Check for a prior index with the same embedding model.
            prior = self._indexes.get(project_id)
            can_incremental = (
                prior is not None
                and prior.embedding_model == embedding_model
                and prior.file_hashes  # non-empty hash map
            )

            if can_incremental and prior is not None:
                return await self._build_incremental(
                    project_id,
                    per_file,
                    prior,
                    embedding_model,
                    log,
                )

            # Full build — no prior index or model changed.
            return await self._build_full(project_id, per_file, embedding_model, log)

        except Exception as exc:
            log.exception("index build failed")
            return IndexStatus(
                project_id=project_id,
                status="error",
                error=str(exc),
            )

    async def _build_full(
        self,
        project_id: str,
        per_file: dict[str, tuple[str, list[CodeChunk]]],
        embedding_model: str,
        log: structlog.stdlib.BoundLogger,
    ) -> IndexStatus:
        """Perform a full index build from scratch."""
        chunks: list[CodeChunk] = []
        file_hashes: dict[str, FileHashRecord] = {}

        for rel_path, (content_hash, file_chunks) in per_file.items():
            chunk_start = len(chunks)
            chunks.extend(file_chunks)
            file_hashes[rel_path] = FileHashRecord(
                filepath=rel_path,
                content_hash=content_hash,
                chunk_start=chunk_start,
                chunk_count=len(file_chunks),
            )

        if not chunks:
            log.info("index empty, no chunks found")
            return IndexStatus(
                project_id=project_id,
                status="empty",
                embedding_model=embedding_model,
            )

        # Build BM25
        corpus = [c.content for c in chunks]
        corpus_tokens = bm25s.tokenize(corpus)
        bm25 = bm25s.BM25()
        bm25.index(corpus_tokens)

        # Embed all chunks
        embeddings = await self._embed_texts(corpus, embedding_model)

        index = ProjectIndex(
            project_id=project_id,
            chunks=chunks,
            bm25=bm25,
            embeddings=embeddings,
            file_count=len(per_file),
            chunk_count=len(chunks),
            embedding_model=embedding_model,
            file_hashes=file_hashes,
        )
        self._indexes[project_id] = index

        log.info("index built (full)", files=index.file_count, chunks=index.chunk_count)
        return IndexStatus(
            project_id=project_id,
            status="ready",
            file_count=index.file_count,
            chunk_count=index.chunk_count,
            embedding_model=embedding_model,
        )

    async def _build_incremental(
        self,
        project_id: str,
        per_file: dict[str, tuple[str, list[CodeChunk]]],
        prior: ProjectIndex,
        embedding_model: str,
        log: structlog.stdlib.BoundLogger,
    ) -> IndexStatus:
        """Incremental build: reuse chunks/embeddings for unchanged files."""
        old_hashes = prior.file_hashes
        current_files = set(per_file.keys())
        old_files = set(old_hashes.keys())

        unchanged = {f for f in current_files & old_files if per_file[f][0] == old_hashes[f].content_hash}
        changed = (current_files & old_files) - unchanged
        added = current_files - old_files
        # deleted files are simply not included

        files_changed = len(changed) + len(added)
        files_unchanged = len(unchanged)

        if files_changed == 0 and len(current_files) == len(old_files):
            # Nothing changed — return current status.
            log.info("incremental: no changes detected", files=len(current_files))
            return IndexStatus(
                project_id=project_id,
                status="ready",
                file_count=prior.file_count,
                chunk_count=prior.chunk_count,
                embedding_model=embedding_model,
                incremental=True,
                files_changed=0,
                files_unchanged=files_unchanged,
            )

        # Assemble merged chunk list and embedding rows.
        chunks: list[CodeChunk] = []
        embedding_rows: list[np.ndarray] = []
        file_hashes: dict[str, FileHashRecord] = {}
        new_chunks: list[CodeChunk] = []  # chunks that need fresh embeddings

        # 1. Reuse unchanged files.
        for rel_path in sorted(unchanged):
            rec = old_hashes[rel_path]
            chunk_start = len(chunks)
            old_chunks = prior.chunks[rec.chunk_start : rec.chunk_start + rec.chunk_count]
            old_embeds = prior.embeddings[rec.chunk_start : rec.chunk_start + rec.chunk_count]
            chunks.extend(old_chunks)
            embedding_rows.append(old_embeds)
            file_hashes[rel_path] = FileHashRecord(
                filepath=rel_path,
                content_hash=rec.content_hash,
                chunk_start=chunk_start,
                chunk_count=rec.chunk_count,
            )

        # 2. Add changed + new files (need re-embedding).
        for rel_path in sorted(changed | added):
            content_hash, file_chunks = per_file[rel_path]
            chunk_start = len(chunks) + len(new_chunks)
            new_chunks.extend(file_chunks)
            file_hashes[rel_path] = FileHashRecord(
                filepath=rel_path,
                content_hash=content_hash,
                chunk_start=chunk_start,
                chunk_count=len(file_chunks),
            )

        # Embed only new/changed chunks.
        if new_chunks:
            new_corpus = [c.content for c in new_chunks]
            new_embeddings = await self._embed_texts(new_corpus, embedding_model)
            embedding_rows.append(new_embeddings)
            chunks.extend(new_chunks)

        if not chunks:
            log.info("incremental: index empty after rebuild")
            return IndexStatus(
                project_id=project_id,
                status="empty",
                embedding_model=embedding_model,
                incremental=True,
                files_changed=files_changed,
                files_unchanged=files_unchanged,
            )

        # Concatenate embeddings.
        all_embeddings = np.concatenate(embedding_rows, axis=0) if embedding_rows else np.empty((0, 0))

        # Rebuild BM25 (always full — it's fast).
        corpus = [c.content for c in chunks]
        corpus_tokens = bm25s.tokenize(corpus)
        bm25 = bm25s.BM25()
        bm25.index(corpus_tokens)

        index = ProjectIndex(
            project_id=project_id,
            chunks=chunks,
            bm25=bm25,
            embeddings=all_embeddings,
            file_count=len(per_file),
            chunk_count=len(chunks),
            embedding_model=embedding_model,
            file_hashes=file_hashes,
        )
        self._indexes[project_id] = index

        log.info(
            "index built (incremental)",
            files=index.file_count,
            chunks=index.chunk_count,
            files_changed=files_changed,
            files_unchanged=files_unchanged,
        )
        return IndexStatus(
            project_id=project_id,
            status="ready",
            file_count=index.file_count,
            chunk_count=index.chunk_count,
            embedding_model=embedding_model,
            incremental=True,
            files_changed=files_changed,
            files_unchanged=files_unchanged,
        )

    async def search(
        self,
        project_id: str,
        query: str,
        top_k: int = 20,
        bm25_weight: float = 0.5,
        semantic_weight: float = 0.5,
        query_embedding: np.ndarray | None = None,
    ) -> list[RetrievalSearchHit]:
        """Search an indexed project using hybrid BM25 + semantic retrieval.

        If *query_embedding* is provided, it is used directly for the semantic
        branch instead of calling the embedding API.  This allows callers to
        batch-embed multiple queries in a single request.
        """
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
        if query_embedding is None:
            query_embedding = (await self._embed_texts([query], index.embedding_model))[0]
        query_vec = query_embedding
        cosine_scores = self._cosine_similarity(query_vec, index.embeddings)
        semantic_ranking: list[int] = list(np.argsort(-cosine_scores))

        # RRF fusion
        fused = self._rrf_fuse(bm25_ranking, semantic_ranking)

        # Pre-build rank maps for O(1) lookup (#14)
        bm25_rank_map = {idx: rank for rank, idx in enumerate(bm25_ranking)}
        sem_rank_map = {idx: rank for rank, idx in enumerate(semantic_ranking)}

        # Build results
        results: list[RetrievalSearchHit] = []
        for chunk_idx, score in fused[:effective_k]:
            chunk = index.chunks[chunk_idx]
            results.append(
                RetrievalSearchHit(
                    filepath=chunk.filepath,
                    start_line=chunk.start_line,
                    end_line=chunk.end_line,
                    content=chunk.content,
                    language=chunk.language,
                    symbol_name=chunk.symbol_name,
                    score=score,
                    bm25_rank=bm25_rank_map.get(chunk_idx, n_chunks) + 1,
                    semantic_rank=sem_rank_map.get(chunk_idx, n_chunks) + 1,
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


# ---------------------------------------------------------------------------
# RetrievalSubAgent -- LLM-guided multi-query retrieval (Phase 6C)
# ---------------------------------------------------------------------------

_EXPAND_SYSTEM = (
    "You are a code search query expander. Given a task description, generate "
    "focused search queries that would help find relevant code. Output one query "
    "per line. Do not number them or add any other text."
)

_RERANK_SYSTEM = (
    "You are a code relevance ranker. Given a query and a numbered list of code "
    "snippets, output the numbers of the most relevant snippets in order of "
    "relevance, one number per line. Output only numbers, nothing else."
)


class RetrievalSubAgent:
    """LLM-guided multi-query retrieval agent.

    Composes a HybridRetriever with an LLM client to provide:
    1. Query expansion (task prompt -> N focused queries)
    2. Parallel hybrid searches
    3. Deduplication by (filepath, start_line)
    4. LLM re-ranking for relevance
    """

    def __init__(self, retriever: HybridRetriever, llm: LiteLLMClient) -> None:
        self._retriever = retriever
        self._llm = llm

    _MAX_RERANK_CANDIDATES = 30

    async def search(
        self,
        project_id: str,
        query: str,
        top_k: int = 20,
        max_queries: int = 5,
        model: str = "",
        rerank: bool = True,
    ) -> tuple[list[RetrievalSearchHit], list[str], int]:
        """Multi-step retrieval: expand -> parallel search -> dedup -> rerank.

        Returns (results, expanded_queries, total_candidates_before_dedup).
        """
        # 1. LLM query expansion
        expanded = await self._expand_queries(query, max_queries, model)
        if not expanded:
            expanded = [query]

        # 2. Parallel hybrid searches
        all_hits = await self._parallel_search(project_id, expanded, top_k)
        total_candidates = len(all_hits)

        # 3. Deduplicate by (filepath, start_line)
        deduped = self._deduplicate(all_hits)

        # 4. Optional LLM re-ranking
        if rerank and len(deduped) > top_k:
            deduped = await self._rerank(query, deduped, top_k, model)
        else:
            deduped = sorted(deduped, key=lambda r: r.score, reverse=True)[:top_k]

        return deduped, expanded, total_candidates

    async def _expand_queries(
        self,
        query: str,
        max_queries: int,
        model: str,
    ) -> list[str]:
        """Use LLM to expand a task prompt into focused search queries."""
        prompt = f"Expand this task description into {max_queries} focused code search queries:\n\n{query}"
        try:
            resp = await self._llm.completion(
                prompt=prompt,
                model=model,
                system=_EXPAND_SYSTEM,
                temperature=0.3,
            )
            lines = [line.strip() for line in resp.content.splitlines() if line.strip()]
            return lines[:max_queries]
        except Exception:
            logger.warning("query expansion failed, using original query", query=query[:80], exc_info=True)
            return [query]

    async def _parallel_search(
        self,
        project_id: str,
        queries: list[str],
        top_k: int,
    ) -> list[RetrievalSearchHit]:
        """Run hybrid searches in parallel for each expanded query.

        Batch-embeds all queries in a single API call, then passes the
        pre-computed vectors to each search to avoid N separate embedding
        round-trips.
        """
        # Each query returns at least top_k results for a rich candidate set (#13).
        per_query_k = top_k

        # Batch-embed all queries in one call if index exists.
        embeddings: list[np.ndarray | None] = [None] * len(queries)
        index = self._retriever._indexes.get(project_id)
        if index is not None:
            try:
                all_vecs = await self._retriever._embed_texts(queries, index.embedding_model)
                embeddings = list(all_vecs)
            except Exception:
                logger.warning("batch embedding failed, falling back to per-query embedding", exc_info=True)

        tasks = [
            self._retriever.search(project_id, q, per_query_k, query_embedding=emb)
            for q, emb in zip(queries, embeddings, strict=False)
        ]
        results_lists = await asyncio.gather(*tasks, return_exceptions=True)
        all_hits: list[RetrievalSearchHit] = []
        for result in results_lists:
            if isinstance(result, list):
                all_hits.extend(result)
            elif isinstance(result, Exception):
                logger.warning("parallel search failed for one query", error=str(result))
        return all_hits

    @staticmethod
    def _deduplicate(hits: list[RetrievalSearchHit]) -> list[RetrievalSearchHit]:
        """Group by (filepath, start_line) and keep the highest-scored hit per group."""
        best: dict[tuple[str, int], RetrievalSearchHit] = {}
        for hit in hits:
            key = (hit.filepath, hit.start_line)
            if key not in best or hit.score > best[key].score:
                best[key] = hit
        return list(best.values())

    async def _rerank(
        self,
        query: str,
        hits: list[RetrievalSearchHit],
        top_k: int,
        model: str,
    ) -> list[RetrievalSearchHit]:
        """Use LLM to re-rank candidates by relevance to the original query."""
        # Cap candidates to avoid exceeding context window
        max_candidates = min(top_k * 2, self._MAX_RERANK_CANDIDATES)
        candidates = sorted(hits, key=lambda r: r.score, reverse=True)[:max_candidates]

        # Format numbered list for LLM
        snippets: list[str] = []
        for i, hit in enumerate(candidates):
            preview = hit.content[:200].replace("\n", " ")
            snippets.append(f"{i + 1}. {hit.filepath}:{hit.start_line} — {preview}")

        prompt = f"Query: {query}\n\nRank these code snippets by relevance (most relevant first):\n\n" + "\n".join(
            snippets
        )

        try:
            resp = await self._llm.completion(
                prompt=prompt,
                model=model,
                system=_RERANK_SYSTEM,
                temperature=0.0,
            )
            # Parse ranking: extract numbers from response lines
            ranked_indices: list[int] = []
            seen: set[int] = set()
            for line in resp.content.splitlines():
                line = line.strip().rstrip(".")
                try:
                    idx = int(line) - 1  # Convert 1-based to 0-based
                    if 0 <= idx < len(candidates) and idx not in seen:
                        ranked_indices.append(idx)
                        seen.add(idx)
                except ValueError:
                    continue

            if ranked_indices:
                # Append unranked candidates so we always return up to top_k.
                ranked_indices.extend(i for i in range(len(candidates)) if i not in seen)
                return [candidates[i] for i in ranked_indices[:top_k]]
        except Exception:
            logger.warning("LLM reranking failed, falling back to score-based ranking", exc_info=True)

        # Fallback: score-based sorting
        return sorted(candidates, key=lambda r: r.score, reverse=True)[:top_k]
