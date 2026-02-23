"""Tests for the LiteLLM client."""

from __future__ import annotations

from unittest.mock import AsyncMock, patch

import httpx
import pytest

from codeforge.llm import SCENARIO_DEFAULTS, LiteLLMClient, resolve_scenario

_FAKE_REQUEST = httpx.Request("POST", "http://test:4000/v1/chat/completions")


@pytest.fixture
def client() -> LiteLLMClient:
    """Create a LiteLLMClient for testing."""
    return LiteLLMClient(base_url="http://test:4000", api_key="test-key")


async def test_completion_parses_response(client: LiteLLMClient) -> None:
    """completion() should parse a valid OpenAI-format response."""
    mock_response = httpx.Response(
        200,
        json={
            "choices": [{"message": {"content": "Hello world"}}],
            "usage": {"prompt_tokens": 10, "completion_tokens": 5},
        },
        request=_FAKE_REQUEST,
    )

    with patch.object(client._client, "post", new_callable=AsyncMock, return_value=mock_response):
        result = await client.completion(prompt="Say hello", model="test-model")

    assert result.content == "Hello world"
    assert result.tokens_in == 10
    assert result.tokens_out == 5
    assert result.model == "test-model"
    assert result.cost_usd == 0.0


async def test_completion_extracts_cost_from_header(client: LiteLLMClient) -> None:
    """completion() should extract cost_usd from x-litellm-response-cost header."""
    mock_response = httpx.Response(
        200,
        json={
            "choices": [{"message": {"content": "Hello"}}],
            "usage": {"prompt_tokens": 100, "completion_tokens": 50},
        },
        headers={"x-litellm-response-cost": "0.00325"},
        request=_FAKE_REQUEST,
    )

    with patch.object(client._client, "post", new_callable=AsyncMock, return_value=mock_response):
        result = await client.completion(prompt="Say hello", model="gpt-4o")

    assert result.cost_usd == pytest.approx(0.00325)


async def test_completion_handles_invalid_cost_header(client: LiteLLMClient) -> None:
    """completion() should default to 0 when cost header is invalid."""
    mock_response = httpx.Response(
        200,
        json={
            "choices": [{"message": {"content": "Hello"}}],
            "usage": {"prompt_tokens": 10, "completion_tokens": 5},
        },
        headers={"x-litellm-response-cost": "not-a-number"},
        request=_FAKE_REQUEST,
    )

    with patch.object(client._client, "post", new_callable=AsyncMock, return_value=mock_response):
        result = await client.completion(prompt="Say hello", model="gpt-4o")

    assert result.cost_usd == 0.0


async def test_completion_empty_choices(client: LiteLLMClient) -> None:
    """completion() should handle empty choices gracefully."""
    mock_response = httpx.Response(200, json={"choices": [], "usage": {}}, request=_FAKE_REQUEST)

    with patch.object(client._client, "post", new_callable=AsyncMock, return_value=mock_response):
        result = await client.completion(prompt="test")

    assert result.content == ""
    assert result.tokens_in == 0
    assert result.tokens_out == 0
    assert result.cost_usd == 0.0


async def test_health_returns_true(client: LiteLLMClient) -> None:
    """health() should return True when the proxy responds with 200."""
    mock_response = httpx.Response(200, request=httpx.Request("GET", "http://test:4000/health"))

    with patch.object(client._client, "get", new_callable=AsyncMock, return_value=mock_response):
        assert await client.health() is True


async def test_health_returns_false_on_error(client: LiteLLMClient) -> None:
    """health() should return False on connection errors."""
    with patch.object(client._client, "get", new_callable=AsyncMock, side_effect=httpx.ConnectError("refused")):
        assert await client.health() is False


async def test_close_calls_aclose(client: LiteLLMClient) -> None:
    """close() should properly close the HTTP client."""
    with patch.object(client._client, "aclose", new_callable=AsyncMock) as mock_close:
        await client.close()
        mock_close.assert_called_once()


# -- Scenario routing tests --


async def test_completion_passes_tags(client: LiteLLMClient) -> None:
    """completion() should include tags in the request payload when provided."""
    mock_response = httpx.Response(
        200,
        json={
            "choices": [{"message": {"content": "Hello"}}],
            "usage": {"prompt_tokens": 10, "completion_tokens": 5},
        },
        request=_FAKE_REQUEST,
    )

    with patch.object(client._client, "post", new_callable=AsyncMock, return_value=mock_response) as mock_post:
        await client.completion(prompt="test", tags=["think"])

    call_payload = mock_post.call_args.kwargs["json"]
    assert call_payload["tags"] == ["think"]


async def test_completion_without_tags(client: LiteLLMClient) -> None:
    """completion() should not include tags key when tags is None."""
    mock_response = httpx.Response(
        200,
        json={
            "choices": [{"message": {"content": "Hello"}}],
            "usage": {"prompt_tokens": 10, "completion_tokens": 5},
        },
        request=_FAKE_REQUEST,
    )

    with patch.object(client._client, "post", new_callable=AsyncMock, return_value=mock_response) as mock_post:
        await client.completion(prompt="test")

    call_payload = mock_post.call_args.kwargs["json"]
    assert "tags" not in call_payload


def test_resolve_scenario_known() -> None:
    """resolve_scenario() should return correct config for all known scenarios."""
    for name, expected in SCENARIO_DEFAULTS.items():
        cfg = resolve_scenario(name)
        assert cfg.tag == expected.tag
        assert cfg.temperature == expected.temperature


def test_resolve_scenario_unknown_falls_back() -> None:
    """resolve_scenario() should fall back to 'default' for unknown scenarios."""
    cfg = resolve_scenario("nonexistent")
    assert cfg.tag == "default"
    assert cfg.temperature == 0.2


def test_resolve_scenario_temperatures() -> None:
    """Verify specific temperature values per scenario."""
    assert resolve_scenario("think").temperature == pytest.approx(0.3)
    assert resolve_scenario("review").temperature == pytest.approx(0.1)
    assert resolve_scenario("default").temperature == pytest.approx(0.2)
    assert resolve_scenario("background").temperature == pytest.approx(0.1)
    assert resolve_scenario("plan").temperature == pytest.approx(0.3)
    assert resolve_scenario("longContext").temperature == pytest.approx(0.2)
