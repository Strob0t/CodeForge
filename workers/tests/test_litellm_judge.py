"""Tests for LiteLLMJudge wrapper (Phase 20A).

deepeval is not installed in the dev/test environment, so we mock the entire
module tree before importing production code that depends on it.
"""

from __future__ import annotations

import sys
from types import ModuleType
from unittest.mock import AsyncMock, MagicMock, patch

import httpx
import pytest

# --- Mock deepeval module tree before importing production code ---
_deepeval = ModuleType("deepeval")
_deepeval_models = ModuleType("deepeval.models")
_deepeval_models.DeepEvalBaseLLM = type("DeepEvalBaseLLM", (), {})

sys.modules.setdefault("deepeval", _deepeval)
sys.modules.setdefault("deepeval.models", _deepeval_models)

from codeforge.evaluation.litellm_judge import LiteLLMJudge  # noqa: E402


def _make_response_json() -> dict:
    """Build a fake OpenAI-style chat completion response."""
    return {
        "choices": [
            {
                "message": {
                    "content": "The answer is correct.",
                }
            }
        ]
    }


def test_get_model_name() -> None:
    """get_model_name should return the configured model string."""
    judge = LiteLLMJudge(model="test/model-v1")
    assert judge.get_model_name() == "test/model-v1"


def test_load_model() -> None:
    """load_model should return the model name (no actual loading needed)."""
    judge = LiteLLMJudge(model="test/model-v1")
    assert judge.load_model() == "test/model-v1"


@pytest.mark.asyncio
async def test_a_generate_success() -> None:
    """a_generate should POST to LiteLLM and return the message content."""
    judge = LiteLLMJudge(model="test/model", base_url="http://localhost:4000/v1")

    mock_response = MagicMock()
    mock_response.json.return_value = _make_response_json()
    mock_response.raise_for_status = MagicMock()

    with patch("httpx.AsyncClient") as mock_client_cls:
        mock_client = AsyncMock()
        mock_client.post = AsyncMock(return_value=mock_response)
        mock_client.__aenter__ = AsyncMock(return_value=mock_client)
        mock_client.__aexit__ = AsyncMock(return_value=False)
        mock_client_cls.return_value = mock_client

        result = await judge.a_generate("Is 2+2=4?")

    assert result == "The answer is correct."
    mock_client.post.assert_awaited_once()
    call_args = mock_client.post.call_args
    assert "chat/completions" in call_args[0][0]


@pytest.mark.asyncio
async def test_a_generate_http_error() -> None:
    """a_generate should propagate HTTP errors from the proxy."""
    judge = LiteLLMJudge(model="test/model", base_url="http://localhost:4000/v1")

    mock_response = MagicMock()
    mock_response.raise_for_status.side_effect = httpx.HTTPStatusError(
        "Server Error",
        request=MagicMock(),
        response=MagicMock(status_code=500),
    )

    with patch("httpx.AsyncClient") as mock_client_cls:
        mock_client = AsyncMock()
        mock_client.post = AsyncMock(return_value=mock_response)
        mock_client.__aenter__ = AsyncMock(return_value=mock_client)
        mock_client.__aexit__ = AsyncMock(return_value=False)
        mock_client_cls.return_value = mock_client

        with pytest.raises(httpx.HTTPStatusError):
            await judge.a_generate("test prompt")


def test_generate_success() -> None:
    """generate (sync) should POST to LiteLLM and return the message content."""
    judge = LiteLLMJudge(model="test/model", base_url="http://localhost:4000/v1")

    mock_response = MagicMock()
    mock_response.json.return_value = _make_response_json()
    mock_response.raise_for_status = MagicMock()

    with patch("httpx.Client") as mock_client_cls:
        mock_client = MagicMock()
        mock_client.post.return_value = mock_response
        mock_client.__enter__ = MagicMock(return_value=mock_client)
        mock_client.__exit__ = MagicMock(return_value=False)
        mock_client_cls.return_value = mock_client

        result = judge.generate("Is 2+2=4?")

    assert result == "The answer is correct."
    mock_client.post.assert_called_once()


def test_generate_http_error() -> None:
    """generate (sync) should propagate HTTP errors from the proxy."""
    judge = LiteLLMJudge(model="test/model", base_url="http://localhost:4000/v1")

    mock_response = MagicMock()
    mock_response.raise_for_status.side_effect = httpx.HTTPStatusError(
        "Server Error",
        request=MagicMock(),
        response=MagicMock(status_code=500),
    )

    with patch("httpx.Client") as mock_client_cls:
        mock_client = MagicMock()
        mock_client.post.return_value = mock_response
        mock_client.__enter__ = MagicMock(return_value=mock_client)
        mock_client.__exit__ = MagicMock(return_value=False)
        mock_client_cls.return_value = mock_client

        with pytest.raises(httpx.HTTPStatusError):
            judge.generate("test prompt")
