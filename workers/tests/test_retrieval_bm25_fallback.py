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
import pytest
import structlog
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


# --- Which embedding failures mean "model unusable" (KI-130 review) ---


def _failing_retriever(answer: list[httpx.Response | Exception | None]) -> HybridRetriever:
    """A retriever whose /v1/embeddings embeds while answer[0] is None, else answers (or raises) answer[0]."""

    def handler(request: httpx.Request) -> httpx.Response:
        current = answer[0]
        if isinstance(current, Exception):
            raise current
        if current is not None:
            return current
        texts = json.loads(request.content)["input"]
        data = [{"index": i, "embedding": [float(i + 1), 1.0, 0.5]} for i in range(len(texts))]
        return httpx.Response(200, json={"data": data})

    retriever = HybridRetriever(litellm_url="http://litellm.test")
    retriever._client = httpx.AsyncClient(base_url="http://litellm.test", transport=httpx.MockTransport(handler))
    return retriever


_UNUSABLE = [
    pytest.param(httpx.Response(401, json={"error": {"message": "AuthenticationError: no api key"}}), id="401"),
    pytest.param(httpx.Response(403, json={"error": {"message": "forbidden"}}), id="403"),
    pytest.param(httpx.Response(404, json={"error": {"message": "model 'nomic' not found"}}), id="404"),
    pytest.param(
        httpx.Response(
            400,
            json={"error": "/embeddings: Invalid model name passed in model=nomic. Call `/v1/models` to view ..."},
        ),
        id="400-invalid-model-name",
    ),
    pytest.param(
        httpx.Response(400, json={"error": {"message": "litellm.BadRequestError: LLM Provider NOT provided."}}),
        id="400-no-provider",
    ),
    # KI-150: LiteLLM answers an embedding request without the provider's key with a 500.
    pytest.param(
        httpx.Response(
            500,
            json={
                "error": {
                    "message": "litellm.AuthenticationError: AuthenticationError: OpenAIException - The api_key "
                    "client option must be set either by passing api_key to the client or by setting the "
                    "OPENAI_API_KEY environment variable",
                    "code": "500",
                }
            },
        ),
        id="500-authentication-error",
    ),
    pytest.param(
        httpx.Response(400, json={"error": {"message": "Incorrect API key provided: sk-xxxx."}}), id="400-bad-key"
    ),
]

_TRANSIENT = [
    pytest.param(httpx.Response(429, json={"error": {"message": "rate limited"}}), id="429"),
    pytest.param(
        httpx.Response(429, json={"error": {"message": "Rate limit reached for this API key"}}), id="429-naming-the-key"
    ),
    pytest.param(httpx.Response(500, json={"error": {"message": "internal"}}), id="500"),
    pytest.param(httpx.Response(503, json={"error": {"message": "unavailable"}}), id="503"),
    pytest.param(httpx.ReadTimeout("timed out"), id="timeout"),
    pytest.param(httpx.ConnectError("connection refused"), id="connect-error"),
    pytest.param(
        httpx.Response(400, json={"error": {"message": "input is too long for the context window"}}),
        id="400-not-about-the-model",
    ),
]


@pytest.mark.parametrize("failure", _UNUSABLE)
async def test_unusable_model_gives_a_bm25_only_index(failure: httpx.Response, tmp_path: Path) -> None:
    retriever = _failing_retriever([failure])

    status = await retriever.build_index("p1", _workspace(tmp_path))

    assert status.status == "ready"
    assert status.bm25_only is True
    assert retriever._indexes["p1"].embeddings is None
    await retriever.close()


@pytest.mark.parametrize("failure", _TRANSIENT)
async def test_transient_failure_keeps_the_hybrid_index(failure: httpx.Response | Exception, tmp_path: Path) -> None:
    answer: list[httpx.Response | Exception | None] = [None]
    retriever = _failing_retriever(answer)
    workspace = _workspace(tmp_path)
    good = await retriever.build_index("p1", workspace)
    assert good.status == "ready"
    assert good.bm25_only is False
    hybrid = retriever._indexes["p1"]

    answer[0] = failure
    (tmp_path / "calc.py").write_text("def mul(a, b):\n    return a * b\n")  # a changed file needs embeddings
    status = await retriever.build_index("p1", workspace)

    assert status.status == "error"
    assert status.error
    assert retriever._indexes["p1"] is hybrid, "the good hybrid index was replaced"
    assert hybrid.embeddings is not None
    await retriever.close()


@pytest.mark.parametrize("failure", _TRANSIENT)
async def test_transient_failure_without_an_index_reports_an_error(
    failure: httpx.Response | Exception, tmp_path: Path
) -> None:
    retriever = _failing_retriever([failure])

    status = await retriever.build_index("p1", _workspace(tmp_path))

    assert status.status == "error"
    assert "p1" not in retriever._indexes
    await retriever.close()


async def test_bm25_only_flag_reaches_the_index_result(tmp_path: Path) -> None:
    """The flag crosses NATS in retrieval.index.result (RetrievalIndexResult.bm25_only)."""
    from codeforge.consumer import TaskConsumer
    from codeforge.models import RetrievalIndexRequest

    worker = TaskConsumer(nats_url="nats://test:4222", litellm_url="http://test:4000")
    retriever = _failing_retriever([httpx.Response(401, json={"error": "no key"})])
    worker._retriever = retriever
    request = RetrievalIndexRequest(project_id="p1", workspace_path=_workspace(tmp_path))

    result = await worker._do_retrieval_index(request, structlog.get_logger())

    assert result.status == "ready"
    assert result.bm25_only is True
    assert json.loads(result.model_dump_json())["bm25_only"] is True
    await retriever.close()


# --- KI-150 review: the worker cannot see LiteLLM's routing ---


def _worker_retriever(answer: list[httpx.Response | Exception | None]) -> tuple[object, HybridRetriever, list[int]]:
    """The worker's own retriever, its /v1/embeddings answered like _failing_retriever, counting calls."""
    from codeforge.consumer import TaskConsumer

    worker = TaskConsumer(nats_url="nats://test:4222", litellm_url="http://test:4000")
    retriever = worker._retriever
    calls = [0]
    inner = _failing_retriever(answer)._client

    async def handle(request: httpx.Request) -> httpx.Response:
        calls[0] += 1
        return await inner._transport.handle_async_request(request)  # type: ignore[union-attr]

    retriever._client = httpx.AsyncClient(base_url="http://litellm.test", transport=httpx.MockTransport(handle))
    return worker, retriever, calls


async def test_a_cloud_model_name_routed_to_a_local_server_is_used(
    tmp_path: Path, monkeypatch: pytest.MonkeyPatch
) -> None:
    """text-embedding-3-small may be a LiteLLM alias of a local server (api_base): no key, embeddings work."""
    monkeypatch.delenv("OPENAI_API_KEY", raising=False)
    monkeypatch.delenv("CODEFORGE_LITELLM_KEYED_PROVIDERS", raising=False)
    _, retriever, calls = _worker_retriever([None])

    status = await retriever.build_index("p1", _workspace(tmp_path), embedding_model="text-embedding-3-small")

    assert calls[0] > 0
    assert status.status == "ready"
    assert status.bm25_only is False
    await retriever.close()


async def test_a_cloud_model_without_its_key_is_bm25_only(tmp_path: Path, monkeypatch: pytest.MonkeyPatch) -> None:
    """Without the key LiteLLM answers 500 AuthenticationError: BM25-only, reported once, no error."""
    monkeypatch.delenv("OPENAI_API_KEY", raising=False)
    failure = httpx.Response(
        500,
        json={
            "error": {"message": "litellm.AuthenticationError: OpenAIException - The api_key client option must be set"}
        },
    )
    _, retriever, _ = _worker_retriever([failure])
    workspace = _workspace(tmp_path)

    with capture_logs() as logs:
        first = await retriever.build_index("p1", workspace)
        second = await retriever.build_index("p2", workspace)

    for status in (first, second):
        assert status.status == "ready"
        assert status.bm25_only is True
        assert status.error == ""
    warnings = [e for e in logs if e["log_level"] == "warning"]
    assert len(warnings) == 1, warnings
    assert "AuthenticationError" in warnings[0]["reason"]
    assert not [e for e in logs if e["log_level"] in ("error", "exception")], logs
    await retriever.close()
