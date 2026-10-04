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
    SourceScan,
    iter_source_files,
)
from codeforge.models import RetrievalSearchHit
from codeforge.workspace_fs import PathLeavesWorkspaceError, WorkspaceRoot

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
    # None: the embedding model was unavailable, the index is BM25-only (KI-130).
    embeddings: np.ndarray | None
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


def _open_index_root(path: str, tenant: str, below: str) -> WorkspaceRoot:
    """The directory an index is built from.

    A workspace (no tenant; its directory may not be a symlink), or the
    directory *below* inside the tenant's area <path>/<tenant>/ of the
    knowledge content root *path*. The content root is the operator's, opened
    like the Go Core does (its own path may be a symlink); the area is opened
    inside it first, so ".." or a symlink in *below* cannot leave the area
    (KI-105).
    """
    if not tenant:
        return WorkspaceRoot(path)
    if tenant in (".", "..") or "/" in tenant:
        raise PathLeavesWorkspaceError(tenant)
    with WorkspaceRoot.operator_dir(path) as content, content.subroot(tenant) as area:
        return area.subroot(below)


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
        *,
        tenant: str = "",
        below: str = ".",
    ) -> dict[str, tuple[str, list[CodeChunk]]]:
        """Walk workspace and return {rel_path: (content_hash, chunks)} per file.

        The content hash is the SHA-256 hex digest of the raw file bytes. With
        *tenant*, *workspace_path* is the knowledge content root and only the
        directory *below* in the tenant's area <root>/<tenant>/ is walked (a
        knowledge base, KI-105); paths are relative to it.
        """
        extensions: set[str] = set(_EXTENSION_MAP)
        if file_extensions:
            extensions &= {e if e.startswith(".") else f".{e}" for e in file_extensions}

        result: dict[str, tuple[str, list[CodeChunk]]] = {}
        try:
            root = _open_index_root(workspace_path, tenant, below)
        except OSError as exc:
            logger.warning("cannot open workspace", path=workspace_path, tenant=tenant, below=below, error=str(exc))
            return result

        scan = SourceScan("retrieval")
        with root:
            for rel_path, source in iter_source_files(root, extensions, scan):
                language = _EXTENSION_MAP[os.path.splitext(rel_path)[1]]
                result[rel_path] = (hashlib.sha256(source).hexdigest(), self.chunk_source(source, rel_path, language))
        scan.log()

        return result

    def chunk_source(self, source: bytes, rel_path: str, language: str) -> list[CodeChunk]:  # noqa: C901
        """Parse the source of a single file and split it at definition boundaries."""
        try:
            parser = self._get_parser(language)
            tree = parser.parse(source)
        except Exception as exc:
            logger.warning("parse failed", path=rel_path, language=language, error=str(exc))
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
# CPU-bound index helpers (run in worker threads, they touch no shared state)
# ---------------------------------------------------------------------------


def _build_bm25(corpus: list[str]) -> bm25s.BM25:
    """Tokenize *corpus* and build its BM25 index."""
    bm25 = bm25s.BM25()
    bm25.index(bm25s.tokenize(corpus))
    return bm25


def _decode_embeddings(resp: httpx.Response) -> np.ndarray:
    """Decode a /v1/embeddings response into a matrix ordered by input index."""
    embeddings_data: list[dict[str, object]] = resp.json().get("data", [])
    embeddings_data.sort(key=lambda d: int(d.get("index", 0)))
    return np.array([item["embedding"] for item in embeddings_data], dtype=np.float32)


# ---------------------------------------------------------------------------
# HybridRetriever -- BM25 + semantic search with RRF fusion
# ---------------------------------------------------------------------------


class HybridRetriever:
    """Combines BM25 keyword search with LiteLLM embedding cosine similarity."""

    def __init__(self, litellm_url: str = "http://localhost:4000", litellm_key: str = "") -> None:
        self._indexes: dict[str, ProjectIndex] = {}
        self._chunker = CodeChunker()
        self._litellm_url = litellm_url.rstrip("/")
        self._litellm_key = litellm_key
        self._client: httpx.AsyncClient | None = None
        # Embedding models found unavailable: reported once, not on every index build.
        self._unavailable_embedding_models: set[str] = set()

    def _get_client(self) -> httpx.AsyncClient:
        if self._client is None:
            headers: dict[str, str] = {"Content-Type": "application/json"}
            if self._litellm_key:
                headers["Authorization"] = f"Bearer {self._litellm_key}"
            self._client = httpx.AsyncClient(base_url=self._litellm_url, headers=headers, timeout=120.0)
        return self._client

    # ------------------------------------------------------------------
    # Public API
    # ------------------------------------------------------------------

    async def build_index(
        self,
        project_id: str,
        workspace_path: str,
        embedding_model: str = "text-embedding-3-small",
        file_extensions: list[str] | None = None,
        tenant: str = "",
        below: str = ".",
    ) -> IndexStatus:
        """Chunk workspace (only its directory *below*), build BM25 index and compute embeddings.

        Supports incremental builds: if a prior index exists with the same
        embedding model, only changed/new files are re-chunked and re-embedded.
        Unchanged files reuse their chunks and embedding rows from the prior index.
        """
        log = logger.bind(project_id=project_id)
        log.info("building retrieval index", workspace=workspace_path)

        try:
            # Collect files with per-file content hashes. Walking and parsing the
            # workspace is CPU-bound: run it off the event loop, which also keeps
            # the in-progress acks of this request flowing.
            per_file = await asyncio.to_thread(
                self._chunker.chunk_workspace_by_file, workspace_path, file_extensions, tenant=tenant, below=below
            )
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
                and prior.embeddings is not None  # a BM25-only index is rebuilt in full
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

        # Build BM25 off the event loop: it scales with the repository.
        corpus = [c.content for c in chunks]
        bm25 = await asyncio.to_thread(_build_bm25, corpus)

        embeddings = await self._embed_or_none(corpus, embedding_model, log)

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

        # Embed only new/changed chunks; without the embedding model the
        # index becomes BM25-only.
        embeddings_available = True
        if new_chunks:
            new_corpus = [c.content for c in new_chunks]
            new_embeddings = await self._embed_or_none(new_corpus, embedding_model, log)
            if new_embeddings is None:
                embeddings_available = False
            else:
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

        # Concatenate embeddings and rebuild BM25 (always full) off the event
        # loop: both scale with the repository.
        all_embeddings: np.ndarray | None = None
        if embeddings_available:
            all_embeddings = (
                await asyncio.to_thread(np.concatenate, embedding_rows, axis=0) if embedding_rows else np.empty((0, 0))
            )
        corpus = [c.content for c in chunks]
        bm25 = await asyncio.to_thread(_build_bm25, corpus)

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

        # Semantic retrieval (none for a BM25-only index)
        semantic_ranking: list[int] = []
        if index.embeddings is not None:
            if query_embedding is None:
                query_embedding = (await self._embed_texts([query], index.embedding_model))[0]
            cosine_scores = self._cosine_similarity(query_embedding, index.embeddings)
            semantic_ranking = list(np.argsort(-cosine_scores))

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
        if self._client is not None:
            await self._client.aclose()
            self._client = None

    # ------------------------------------------------------------------
    # Internal helpers
    # ------------------------------------------------------------------

    async def _embed_or_none(
        self, texts: list[str], model: str, log: structlog.stdlib.BoundLogger
    ) -> np.ndarray | None:
        """Embed *texts*, or return None when the embedding model cannot be used (KI-130).

        Without an embedding provider (no key for the default cloud model, a
        local-only installation) the index is BM25-only; that is reported once
        per model, not on every index build.
        """
        try:
            embeddings = await self._embed_texts(texts, model)
        except httpx.HTTPError as exc:
            if model not in self._unavailable_embedding_models:
                self._unavailable_embedding_models.add(model)
                log.warning(
                    "embedding model unavailable, retrieval indexes are BM25 only "
                    "(set orchestrator.default_embedding_model / CODEFORGE_ORCH_EMBEDDING_MODEL)",
                    embedding_model=model,
                    error=str(exc),
                )
            return None
        self._unavailable_embedding_models.discard(model)
        return embeddings

    async def _embed_texts(self, texts: list[str], model: str = "text-embedding-3-small") -> np.ndarray:
        """Batch-embed texts via the LiteLLM /v1/embeddings endpoint."""
        resp = await self._get_client().post(
            "/v1/embeddings",
            json={"input": texts, "model": model},
        )
        resp.raise_for_status()
        # Decoding a whole corpus' vectors is CPU-bound: keep it off the event loop.
        return await asyncio.to_thread(_decode_embeddings, resp)

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
        self.last_cost = self.CostAccumulator()

    _MAX_RERANK_CANDIDATES = 30

    @dataclass
    class CostAccumulator:
        """Tracks aggregate LLM cost across sub-agent calls."""

        model: str = ""
        tokens_in: int = 0
        tokens_out: int = 0
        cost_usd: float = 0.0

        def add(self, resp: object) -> None:
            """Add cost from a CompletionResponse (duck-typed)."""
            self.tokens_in += getattr(resp, "tokens_in", 0)
            self.tokens_out += getattr(resp, "tokens_out", 0)
            self.cost_usd += getattr(resp, "cost_usd", 0.0)
            if not self.model and getattr(resp, "model", ""):
                self.model = resp.model  # type: ignore[union-attr]

    async def search(
        self,
        project_id: str,
        query: str,
        top_k: int = 20,
        max_queries: int = 5,
        model: str = "",
        rerank: bool = True,
        expansion_prompt: str = "",
    ) -> tuple[list[RetrievalSearchHit], list[str], int]:
        """Multi-step retrieval: expand -> parallel search -> dedup -> rerank.

        Returns (results, expanded_queries, total_candidates_before_dedup).
        After completion, ``self.last_cost`` holds the accumulated LLM cost.
        """
        cost = self.CostAccumulator()

        # 1. LLM query expansion
        expanded = await self._expand_queries(query, max_queries, model, expansion_prompt, cost)
        if not expanded:
            expanded = [query]

        # 2. Parallel hybrid searches
        all_hits = await self._parallel_search(project_id, expanded, top_k)
        total_candidates = len(all_hits)

        # 3. Deduplicate by (filepath, start_line)
        deduped = self._deduplicate(all_hits)

        # 4. Optional LLM re-ranking
        if rerank and len(deduped) > top_k:
            deduped = await self._rerank(query, deduped, top_k, model, cost)
        else:
            deduped = sorted(deduped, key=lambda r: r.score, reverse=True)[:top_k]

        self.last_cost = cost
        return deduped, expanded, total_candidates

    async def _expand_queries(
        self,
        query: str,
        max_queries: int,
        model: str,
        expansion_prompt: str = "",
        cost: CostAccumulator | None = None,
    ) -> list[str]:
        """Use LLM to expand a task prompt into focused search queries.

        If *expansion_prompt* is non-empty, it replaces the default system prompt.
        """
        system = expansion_prompt or _EXPAND_SYSTEM
        prompt = f"Expand this task description into {max_queries} focused code search queries:\n\n{query}"
        try:
            resp = await self._llm.completion(
                prompt=prompt,
                model=model,
                system=system,
                temperature=0.3,
            )
            if cost is not None:
                cost.add(resp)
            lines = [line.strip() for line in resp.content.splitlines() if line.strip()]
            return lines[:max_queries]
        except Exception as exc:
            logger.warning(
                "query expansion failed, using original query", query=query[:80], exc_info=True, error=str(exc)
            )
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
        if index is not None and index.embeddings is not None:
            try:
                all_vecs = await self._retriever._embed_texts(queries, index.embedding_model)
                embeddings = list(all_vecs)
            except Exception as exc:
                logger.warning(
                    "batch embedding failed, falling back to per-query embedding", exc_info=True, error=str(exc)
                )

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
        cost: CostAccumulator | None = None,
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
            if cost is not None:
                cost.add(resp)
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
        except Exception as exc:
            logger.warning("LLM reranking failed, falling back to score-based ranking", exc_info=True, error=str(exc))

        # Fallback: score-based sorting
        return sorted(candidates, key=lambda r: r.score, reverse=True)[:top_k]
