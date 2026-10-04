"""Retrieval without an embedding provider falls back to BM25 (KI-130).

The default embedding model is a cloud model (text-embedding-3-small); without
its key every clone logged an ERROR with a traceback and the project got no
retrieval index at all, although the keyword (BM25) half needs no model.
"""

from __future__ import annotations

import json
from typing import TYPE_CHECKING

import httpx
import numpy as np
from structlog.testing import capture_logs

from codeforge.retrieval import HybridRetriever

if TYPE_CHECKING:
    from pathlib import Path


def _workspace(root: Path) -> str:
    (root / "calc.py").write_text("def add(a, b):\n    return a + b\n\n\ndef sub(a, b):\n    return a - b\n")
    (root / "shapes.py").write_text("class Circle:\n    def area(self):\n        return 3.14\n")
    return str(root)


def _retriever(embeddings_up: list[bool]) -> tuple[HybridRetriever, list[int]]:
    """A retriever whose /v1/embeddings answers 400 (no such model / key) while embeddings_up[0] is False."""
    calls = [0]

    def handler(request: httpx.Request) -> httpx.Response:
        calls[0] += 1
        if not embeddings_up[0]:
            return httpx.Response(400, json={"error": {"message": "LLM Provider NOT provided"}})
        texts = json.loads(request.content)["input"]
        data = [{"index": i, "embedding": [float(i + 1), 1.0, 0.5]} for i in range(len(texts))]
        return httpx.Response(200, json={"data": data})

    retriever = HybridRetriever(litellm_url="http://litellm.test")
    retriever._client = httpx.AsyncClient(base_url="http://litellm.test", transport=httpx.MockTransport(handler))
    return retriever, calls


async def test_index_without_embeddings_is_bm25_only(tmp_path: Path) -> None:
    retriever, _ = _retriever([False])

    with capture_logs() as logs:
        status = await retriever.build_index("p1", _workspace(tmp_path))
        hits = await retriever.search("p1", "Circle area")

    assert status.status == "ready"
    assert status.chunk_count >= 2
    assert hits
    assert hits[0].filepath == "shapes.py"
    assert not [e for e in logs if e["log_level"] in ("error", "exception")], logs
    await retriever.close()


async def test_unavailable_embeddings_are_reported_once(tmp_path: Path) -> None:
    retriever, _ = _retriever([False])
    workspace = _workspace(tmp_path)

    with capture_logs() as logs:
        for project in ("p1", "p2", "p3"):
            assert (await retriever.build_index(project, workspace)).status == "ready"

    warnings = [e for e in logs if e["log_level"] == "warning"]
    assert len(warnings) == 1, warnings
    assert "BM25" in warnings[0]["event"]
    await retriever.close()


async def test_bm25_only_search_does_not_call_the_embedding_api(tmp_path: Path) -> None:
    retriever, calls = _retriever([False])
    await retriever.build_index("p1", _workspace(tmp_path))
    before = calls[0]

    await retriever.search("p1", "add")

    assert calls[0] == before
    await retriever.close()


async def test_embeddings_are_used_once_available(tmp_path: Path) -> None:
    up = [False]
    retriever, _ = _retriever(up)
    workspace = _workspace(tmp_path)
    await retriever.build_index("p1", workspace)

    up[0] = True
    status = await retriever.build_index("p1", workspace)

    assert status.status == "ready"
    index = retriever._indexes["p1"]
    assert isinstance(index.embeddings, np.ndarray)
    assert index.embeddings.shape[0] == index.chunk_count
    await retriever.close()
