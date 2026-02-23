"""Tests for the hybrid retrieval engine."""

from __future__ import annotations

import os
from unittest.mock import AsyncMock, MagicMock, patch

import numpy as np
import pytest

from codeforge.consumer import TaskConsumer
from codeforge.models import (
    RetrievalIndexRequest,
    RetrievalIndexResult,
    RetrievalSearchRequest,
    RetrievalSearchResult,
)
from codeforge.retrieval import CodeChunker, HybridRetriever

# ---------------------------------------------------------------------------
# Fixtures
# ---------------------------------------------------------------------------


@pytest.fixture
def chunker() -> CodeChunker:
    """Create a CodeChunker with default settings."""
    return CodeChunker(max_chunk_lines=100)


@pytest.fixture
def workspace(tmp_path: object) -> str:
    """Create a temporary workspace with sample source files."""
    ws = str(tmp_path)

    # Python file
    src_dir = os.path.join(ws, "src")
    os.makedirs(src_dir)
    with open(os.path.join(src_dir, "service.py"), "w") as f:
        f.write(
            "class UserService:\n"
            "    def get_user(self, user_id):\n"
            "        return None\n"
            "\n"
            "def create_handler():\n"
            "    svc = UserService()\n"
            "    return svc\n"
        )

    # Go file
    pkg_dir = os.path.join(ws, "pkg")
    os.makedirs(pkg_dir)
    with open(os.path.join(pkg_dir, "handler.go"), "w") as f:
        f.write(
            "package pkg\n"
            "\n"
            "func NewHandler() *Handler { return &Handler{} }\n"
            "\n"
            "type Handler struct {\n"
            "    Name string\n"
            "}\n"
        )

    return ws


def _make_embedding_response(texts: list[str], dim: int = 8) -> dict[str, object]:
    """Create a fake LiteLLM /v1/embeddings response."""
    data = []
    for i, _ in enumerate(texts):
        rng = np.random.default_rng(seed=i)
        vec = rng.standard_normal(dim).tolist()
        data.append({"object": "embedding", "index": i, "embedding": vec})
    return {
        "object": "list",
        "data": data,
        "model": "text-embedding-3-small",
        "usage": {"prompt_tokens": len(texts) * 10, "total_tokens": len(texts) * 10},
    }


# ---------------------------------------------------------------------------
# CodeChunker tests
# ---------------------------------------------------------------------------


def test_chunk_file_python(chunker: CodeChunker, tmp_path: object) -> None:
    """Should split a Python file at definition boundaries."""
    ws = str(tmp_path)
    py_file = os.path.join(ws, "example.py")
    with open(py_file, "w") as f:
        f.write("import os\n\nclass MyService:\n    def start(self):\n        pass\n\ndef helper():\n    return 42\n")

    chunks = chunker.chunk_file(py_file, "example.py", "python")

    assert len(chunks) >= 2
    symbol_names = [c.symbol_name for c in chunks if c.symbol_name]
    assert "MyService" in symbol_names
    assert "helper" in symbol_names

    # All chunks should reference the correct file
    for chunk in chunks:
        assert chunk.filepath == "example.py"
        assert chunk.language == "python"


def test_chunk_file_go(chunker: CodeChunker, tmp_path: object) -> None:
    """Should split a Go file at definition boundaries."""
    ws = str(tmp_path)
    go_file = os.path.join(ws, "main.go")
    with open(go_file, "w") as f:
        f.write('package main\n\nfunc Hello() string { return "hello" }\n\ntype Service struct { Name string }\n')

    chunks = chunker.chunk_file(go_file, "main.go", "go")

    assert len(chunks) >= 2
    symbol_names = [c.symbol_name for c in chunks if c.symbol_name]
    assert "Hello" in symbol_names
    assert "Service" in symbol_names


def test_chunk_workspace(chunker: CodeChunker, workspace: str) -> None:
    """Should chunk all recognised files in a workspace."""
    chunks = chunker.chunk_workspace(workspace)

    assert len(chunks) >= 2

    languages = {c.language for c in chunks}
    assert "python" in languages
    assert "go" in languages

    # Should have relative paths
    for chunk in chunks:
        assert not os.path.isabs(chunk.filepath)


def test_chunk_workspace_skips_ignored(chunker: CodeChunker, tmp_path: object) -> None:
    """Should skip .git, node_modules, and other ignored directories."""
    ws = str(tmp_path)

    for skip_dir in [".git", "node_modules", "__pycache__", "vendor"]:
        d = os.path.join(ws, skip_dir)
        os.makedirs(d)
        with open(os.path.join(d, "file.py"), "w") as f:
            f.write("def secret(): pass\n")

    # Valid file
    with open(os.path.join(ws, "main.py"), "w") as f:
        f.write("def main(): pass\n")

    chunks = chunker.chunk_workspace(ws)

    filepaths = {c.filepath for c in chunks}
    assert "main.py" in filepaths
    # None of the ignored dirs should appear
    for chunk in chunks:
        assert ".git" not in chunk.filepath
        assert "node_modules" not in chunk.filepath
        assert "__pycache__" not in chunk.filepath
        assert "vendor" not in chunk.filepath


def test_chunk_large_function_split(tmp_path: object) -> None:
    """A function exceeding max_chunk_lines should be split into sub-chunks."""
    chunker = CodeChunker(max_chunk_lines=10)
    ws = str(tmp_path)
    py_file = os.path.join(ws, "big.py")

    # Generate a 25-line function
    lines = ["def big_function():\n"]
    lines.extend(f"    x_{i} = {i}\n" for i in range(24))

    with open(py_file, "w") as f:
        f.writelines(lines)

    chunks = chunker.chunk_file(py_file, "big.py", "python")

    # 25 lines with max_chunk_lines=10 should produce 3 sub-chunks
    named_chunks = [c for c in chunks if c.symbol_name and "big_function" in c.symbol_name]
    assert len(named_chunks) == 3

    # Check that all lines are covered
    all_lines: set[int] = set()
    for chunk in named_chunks:
        for line in range(chunk.start_line, chunk.end_line + 1):
            all_lines.add(line)
    assert len(all_lines) == 25


# ---------------------------------------------------------------------------
# HybridRetriever internal helper tests
# ---------------------------------------------------------------------------


def test_rrf_fusion() -> None:
    """RRF fusion should merge two rankings and produce correct ordering."""
    bm25_ranking = [0, 2, 1, 3]
    semantic_ranking = [2, 0, 3, 1]

    fused = HybridRetriever._rrf_fuse(bm25_ranking, semantic_ranking, k=60)
    indices = [idx for idx, _ in fused]

    # Chunks 0 and 2 appear in top-2 of both rankings, so they should be top-2 fused
    assert set(indices[:2]) == {0, 2}
    # All 4 chunks should be present
    assert set(indices) == {0, 1, 2, 3}


def test_cosine_similarity() -> None:
    """Cosine similarity should produce correct scores for known vectors."""
    query = np.array([1.0, 0.0, 0.0], dtype=np.float32)
    matrix = np.array(
        [
            [1.0, 0.0, 0.0],  # identical -> 1.0
            [0.0, 1.0, 0.0],  # orthogonal -> 0.0
            [0.7071, 0.7071, 0.0],  # 45 degrees -> ~0.7071
        ],
        dtype=np.float32,
    )

    scores = HybridRetriever._cosine_similarity(query, matrix)

    assert abs(scores[0] - 1.0) < 1e-4
    assert abs(scores[1] - 0.0) < 1e-4
    assert abs(scores[2] - 0.7071) < 1e-3


# ---------------------------------------------------------------------------
# HybridRetriever integration tests (with mocked embeddings)
# ---------------------------------------------------------------------------


async def test_build_index_and_search(workspace: str) -> None:
    """Full pipeline: build index then search, with mocked LiteLLM embeddings."""
    retriever = HybridRetriever(litellm_url="http://test:4000")

    async def _mock_post(url: str, json: dict[str, object] | None = None, **kwargs: object) -> MagicMock:
        texts = json.get("input", []) if json else []
        resp = MagicMock()
        resp.status_code = 200
        resp.raise_for_status = MagicMock()
        resp.json = MagicMock(return_value=_make_embedding_response(texts, dim=8))
        return resp

    with patch.object(retriever._client, "post", side_effect=_mock_post):
        status = await retriever.build_index("proj-1", workspace)

    assert status.status == "ready"
    assert status.file_count >= 2
    assert status.chunk_count >= 2

    # Search
    with patch.object(retriever._client, "post", side_effect=_mock_post):
        results = await retriever.search("proj-1", "handler")

    assert len(results) > 0
    # Results should have valid fields
    for r in results:
        assert r.filepath
        assert r.start_line >= 1
        assert r.language in {"python", "go"}

    await retriever.close()


async def test_search_without_index() -> None:
    """Searching a non-indexed project should return an empty list."""
    retriever = HybridRetriever(litellm_url="http://test:4000")

    results = await retriever.search("nonexistent", "query")
    assert results == []

    await retriever.close()


# ---------------------------------------------------------------------------
# Consumer integration tests (with mocked NATS)
# ---------------------------------------------------------------------------


async def test_handle_retrieval_index_message(workspace: str) -> None:
    """Consumer should parse retrieval index request, build index, and publish result."""
    consumer = TaskConsumer(nats_url="nats://test:4222", litellm_url="http://test:4000")

    async def _mock_post(url: str, json: dict[str, object] | None = None, **kwargs: object) -> MagicMock:
        texts = json.get("input", []) if json else []
        resp = MagicMock()
        resp.status_code = 200
        resp.raise_for_status = MagicMock()
        resp.json = MagicMock(return_value=_make_embedding_response(texts, dim=8))
        return resp

    request = RetrievalIndexRequest(
        project_id="proj-1",
        workspace_path=workspace,
    )

    msg = MagicMock()
    msg.data = request.model_dump_json().encode()
    msg.ack = AsyncMock()
    msg.nak = AsyncMock()

    consumer._js = AsyncMock()

    with patch.object(consumer._retriever._client, "post", side_effect=_mock_post):
        await consumer._handle_retrieval_index(msg)

    # Should publish result
    consumer._js.publish.assert_called_once()
    call_args = consumer._js.publish.call_args
    assert call_args.args[0] == "retrieval.index.result"

    result = RetrievalIndexResult.model_validate_json(call_args.args[1])
    assert result.project_id == "proj-1"
    assert result.status == "ready"
    assert result.file_count >= 1
    assert result.chunk_count >= 1

    msg.ack.assert_called_once()
    msg.nak.assert_not_called()

    await consumer._retriever.close()


async def test_handle_retrieval_search_message(workspace: str) -> None:
    """Consumer should parse retrieval search request, search, and publish result."""
    consumer = TaskConsumer(nats_url="nats://test:4222", litellm_url="http://test:4000")

    async def _mock_post(url: str, json: dict[str, object] | None = None, **kwargs: object) -> MagicMock:
        texts = json.get("input", []) if json else []
        resp = MagicMock()
        resp.status_code = 200
        resp.raise_for_status = MagicMock()
        resp.json = MagicMock(return_value=_make_embedding_response(texts, dim=8))
        return resp

    # First build an index
    with patch.object(consumer._retriever._client, "post", side_effect=_mock_post):
        await consumer._retriever.build_index("proj-1", workspace)

    # Now search
    request = RetrievalSearchRequest(
        project_id="proj-1",
        query="handler",
        request_id="req-123",
    )

    msg = MagicMock()
    msg.data = request.model_dump_json().encode()
    msg.ack = AsyncMock()
    msg.nak = AsyncMock()

    consumer._js = AsyncMock()

    with patch.object(consumer._retriever._client, "post", side_effect=_mock_post):
        await consumer._handle_retrieval_search(msg)

    # Should publish result
    consumer._js.publish.assert_called_once()
    call_args = consumer._js.publish.call_args
    assert call_args.args[0] == "retrieval.search.result"

    result = RetrievalSearchResult.model_validate_json(call_args.args[1])
    assert result.project_id == "proj-1"
    assert result.query == "handler"
    assert result.request_id == "req-123"
    assert len(result.results) > 0

    msg.ack.assert_called_once()
    msg.nak.assert_not_called()

    await consumer._retriever.close()


# ---------------------------------------------------------------------------
# Incremental indexing tests
# ---------------------------------------------------------------------------


def _make_mock_post(dim: int = 8):
    """Return an async mock for httpx post that counts embed calls."""
    call_count = {"n": 0, "texts": 0}

    async def _mock_post(url: str, json: dict[str, object] | None = None, **kwargs: object) -> MagicMock:
        texts = json.get("input", []) if json else []
        call_count["n"] += 1
        call_count["texts"] += len(texts)
        resp = MagicMock()
        resp.status_code = 200
        resp.raise_for_status = MagicMock()
        resp.json = MagicMock(return_value=_make_embedding_response(texts, dim=dim))
        return resp

    return _mock_post, call_count


async def test_incremental_no_changes(workspace: str) -> None:
    """Second build with no file changes should skip embedding entirely."""
    retriever = HybridRetriever(litellm_url="http://test:4000")
    mock_post, counts = _make_mock_post()

    with patch.object(retriever._client, "post", side_effect=mock_post):
        status1 = await retriever.build_index("proj-inc", workspace)
    assert status1.status == "ready"
    assert not status1.incremental
    first_embed_calls = counts["n"]
    assert first_embed_calls >= 1

    # Second build — same files, same model.
    counts["n"] = 0
    counts["texts"] = 0
    with patch.object(retriever._client, "post", side_effect=mock_post):
        status2 = await retriever.build_index("proj-inc", workspace)
    assert status2.status == "ready"
    assert status2.incremental is True
    assert status2.files_changed == 0
    assert status2.files_unchanged >= 2
    # No embedding calls needed.
    assert counts["n"] == 0

    await retriever.close()


async def test_incremental_file_added(workspace: str) -> None:
    """Adding a file should only embed the new file's chunks."""
    retriever = HybridRetriever(litellm_url="http://test:4000")
    mock_post, counts = _make_mock_post()

    with patch.object(retriever._client, "post", side_effect=mock_post):
        status1 = await retriever.build_index("proj-add", workspace)
    assert status1.status == "ready"
    original_chunks = status1.chunk_count

    # Add a new Python file.
    new_file = os.path.join(workspace, "src", "extra.py")
    with open(new_file, "w") as f:
        f.write("def extra_function():\n    return 'new'\n")

    counts["n"] = 0
    counts["texts"] = 0
    with patch.object(retriever._client, "post", side_effect=mock_post):
        status2 = await retriever.build_index("proj-add", workspace)
    assert status2.status == "ready"
    assert status2.incremental is True
    assert status2.files_changed == 1  # only the new file
    assert status2.chunk_count > original_chunks
    # Only the new file's chunks should have been embedded.
    assert counts["texts"] >= 1  # at least one new chunk
    assert counts["texts"] < status2.chunk_count  # not all chunks re-embedded

    await retriever.close()


async def test_incremental_file_changed(workspace: str) -> None:
    """Modifying a file should re-embed only that file's chunks."""
    retriever = HybridRetriever(litellm_url="http://test:4000")
    mock_post, counts = _make_mock_post()

    with patch.object(retriever._client, "post", side_effect=mock_post):
        status1 = await retriever.build_index("proj-chg", workspace)
    assert status1.status == "ready"

    # Modify the Go file.
    go_file = os.path.join(workspace, "pkg", "handler.go")
    with open(go_file, "w") as f:
        f.write(
            "package pkg\n\n"
            "func NewHandler() *Handler { return &Handler{} }\n\n"
            'func ExtraMethod() string { return "modified" }\n\n'
            "type Handler struct {\n    Name string\n}\n"
        )

    counts["n"] = 0
    counts["texts"] = 0
    with patch.object(retriever._client, "post", side_effect=mock_post):
        status2 = await retriever.build_index("proj-chg", workspace)
    assert status2.status == "ready"
    assert status2.incremental is True
    assert status2.files_changed == 1  # only the modified Go file
    assert status2.files_unchanged >= 1  # the Python file is unchanged
    # Embedding call happened but only for the changed file's chunks.
    assert counts["n"] == 1
    assert counts["texts"] >= 1

    await retriever.close()


async def test_incremental_file_deleted(workspace: str) -> None:
    """Deleting a file should reduce chunk count, no embed for removed chunks."""
    retriever = HybridRetriever(litellm_url="http://test:4000")
    mock_post, counts = _make_mock_post()

    with patch.object(retriever._client, "post", side_effect=mock_post):
        status1 = await retriever.build_index("proj-del", workspace)
    assert status1.status == "ready"
    original_chunks = status1.chunk_count
    original_files = status1.file_count

    # Delete the Go file.
    go_file = os.path.join(workspace, "pkg", "handler.go")
    os.remove(go_file)

    counts["n"] = 0
    counts["texts"] = 0
    with patch.object(retriever._client, "post", side_effect=mock_post):
        status2 = await retriever.build_index("proj-del", workspace)
    assert status2.status == "ready"
    assert status2.incremental is True
    assert status2.file_count == original_files - 1
    assert status2.chunk_count < original_chunks
    # File deletion + all remaining files unchanged = no new embeds needed.
    assert counts["n"] == 0

    await retriever.close()


async def test_incremental_model_change(workspace: str) -> None:
    """Changing embedding model should trigger a full rebuild."""
    retriever = HybridRetriever(litellm_url="http://test:4000")
    mock_post, counts = _make_mock_post()

    with patch.object(retriever._client, "post", side_effect=mock_post):
        status1 = await retriever.build_index("proj-mdl", workspace, embedding_model="model-a")
    assert status1.status == "ready"
    first_total = counts["texts"]

    counts["n"] = 0
    counts["texts"] = 0
    with patch.object(retriever._client, "post", side_effect=mock_post):
        status2 = await retriever.build_index("proj-mdl", workspace, embedding_model="model-b")
    assert status2.status == "ready"
    assert not status2.incremental  # full rebuild
    # All chunks re-embedded.
    assert counts["texts"] == first_total

    await retriever.close()
