"""Tests for the LiteLLM client."""

from __future__ import annotations

import os
from unittest.mock import AsyncMock, patch

import httpx
import pytest

from codeforge.llm import (
    SCENARIO_DEFAULTS,
    LiteLLMClient,
    LLMClientConfig,
    LLMError,
    _extract_provider,
    load_llm_client_config,
    resolve_scenario,
)

_FAKE_REQUEST = httpx.Request("POST", "http://test:4000/v1/chat/completions")

# Fast retry config for tests (tiny backoff to avoid slow tests).
_TEST_CONFIG = LLMClientConfig(max_retries=2, backoff_base=0.01, backoff_max=0.05)


def _ok_response(**extra_headers: str) -> httpx.Response:
    """Build a 200 OK response with optional extra headers."""
    return httpx.Response(
        200,
        json={
            "choices": [{"message": {"content": "Hello"}}],
            "usage": {"prompt_tokens": 10, "completion_tokens": 5},
        },
        headers=extra_headers,
        request=_FAKE_REQUEST,
    )


def _error_response(status: int, body: str = '{"error": "oops"}', **extra_headers: str) -> httpx.Response:
    """Build an error response."""
    return httpx.Response(status, text=body, headers=extra_headers, request=_FAKE_REQUEST)


@pytest.fixture
def client() -> LiteLLMClient:
    """Create a LiteLLMClient for testing."""
    return LiteLLMClient(base_url="http://test:4000", api_key="test-key", config=_TEST_CONFIG)


# -- existing completion tests ---------------------------------------------


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
        result = await client.completion(prompt="test", model="test-model")

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
    mock_response = _ok_response()

    with patch.object(client._client, "post", new_callable=AsyncMock, return_value=mock_response) as mock_post:
        await client.completion(prompt="test", model="test-model", tags=["think"])

    call_payload = mock_post.call_args.kwargs["json"]
    assert call_payload["tags"] == ["think"]


async def test_completion_without_tags(client: LiteLLMClient) -> None:
    """completion() should not include tags key when tags is None."""
    mock_response = _ok_response()

    with patch.object(client._client, "post", new_callable=AsyncMock, return_value=mock_response) as mock_post:
        await client.completion(prompt="test", model="test-model")

    call_payload = mock_post.call_args.kwargs["json"]
    assert "tags" not in call_payload


def test_resolve_scenario_known() -> None:
    """resolve_scenario() should return correct config for all known scenarios."""
    for name, expected in SCENARIO_DEFAULTS.items():
        cfg = resolve_scenario(name)
        assert cfg.tag == expected.tag
        assert cfg.temperature == expected.temperature


def test_resolve_scenario_unknown_falls_back() -> None:
    """resolve_scenario() should fall back to empty tag for unknown scenarios."""
    cfg = resolve_scenario("nonexistent")
    assert cfg.tag == ""
    assert cfg.temperature == 0.2


def test_resolve_scenario_temperatures() -> None:
    """Verify specific temperature values per scenario."""
    assert resolve_scenario("think").temperature == pytest.approx(0.3)
    assert resolve_scenario("review").temperature == pytest.approx(0.1)
    assert resolve_scenario("default").temperature == pytest.approx(0.2)  # falls back to no-tag default
    assert resolve_scenario("background").temperature == pytest.approx(0.1)
    assert resolve_scenario("plan").temperature == pytest.approx(0.3)
    assert resolve_scenario("longContext").temperature == pytest.approx(0.2)


# -- Retry tests --


async def test_retry_on_429(client: LiteLLMClient) -> None:
    """completion() should retry on 429 and succeed on the second attempt."""
    responses = [_error_response(429), _ok_response()]
    mock_post = AsyncMock(side_effect=responses)

    with patch.object(client._client, "post", mock_post):
        result = await client.completion(prompt="test", model="test-model")

    assert result.content == "Hello"
    assert mock_post.call_count == 2


async def test_retry_on_502(client: LiteLLMClient) -> None:
    """completion() should retry on 502/503/504."""
    for code in (502, 503, 504):
        c = LiteLLMClient(base_url="http://test:4000", api_key="k", config=_TEST_CONFIG)
        responses = [_error_response(code), _ok_response()]
        mock_post = AsyncMock(side_effect=responses)
        with patch.object(c._client, "post", mock_post):
            result = await c.completion(prompt="test", model="test-model")
        assert result.content == "Hello"
        assert mock_post.call_count == 2


async def test_no_retry_on_400(client: LiteLLMClient) -> None:
    """completion() should NOT retry on 400 (not retryable)."""
    mock_post = AsyncMock(return_value=_error_response(400))

    with patch.object(client._client, "post", mock_post), pytest.raises(LLMError) as exc_info:
        await client.completion(prompt="test", model="test-model")

    assert exc_info.value.status_code == 400
    assert mock_post.call_count == 1


async def test_max_retries_exhausted(client: LiteLLMClient) -> None:
    """completion() should raise after max retries are exhausted."""
    mock_post = AsyncMock(return_value=_error_response(429))

    with patch.object(client._client, "post", mock_post), pytest.raises(LLMError) as exc_info:
        await client.completion(prompt="test", model="test-model")

    assert exc_info.value.status_code == 429
    # 1 initial + 2 retries = 3 calls
    assert mock_post.call_count == 3


async def test_backoff_uses_retry_after(client: LiteLLMClient) -> None:
    """completion() should use Retry-After from error body when available."""
    error_body = '{"error": "rate limited", "retry_after": 0.01}'
    responses = [_error_response(429, body=error_body), _ok_response()]
    mock_post = AsyncMock(side_effect=responses)

    with patch.object(client._client, "post", mock_post):
        result = await client.completion(prompt="test", model="test-model")

    assert result.content == "Hello"


def test_compute_backoff_uses_retry_hint_without_excessive_buffer() -> None:
    """When Gemini says 'retry in 9.27s', backoff should be ~14s (hint + 5s buffer), not 32s exponential."""
    config = LLMClientConfig(max_retries=5, backoff_base=2.0, backoff_max=90.0)
    client = LiteLLMClient(config=config)

    # Simulate a Gemini 429 with retry hint at attempt 4 (would normally be 2^5=32s).
    exc = LLMError(
        429,
        "gemini/gemini-2.5-flash",
        "retry in 9.273757488s.",
    )
    backoff = client._compute_backoff(exc, attempt=4)
    # Should use the hint (9.27s) + 5s buffer = ~14.27s, NOT the exponential 32s.
    assert backoff >= 9.27, f"Backoff {backoff}s is less than the hint"
    assert backoff < 20.0, f"Backoff {backoff}s is too high, hint was 9.27s"


async def test_chat_completion_retries_on_429(client: LiteLLMClient) -> None:
    """chat_completion() should also retry on 429."""
    responses = [_error_response(429), _ok_response()]
    mock_post = AsyncMock(side_effect=responses)

    with patch.object(client._client, "post", mock_post):
        result = await client.chat_completion(
            messages=[{"role": "user", "content": "test"}],
            model="test-model",
        )

    assert result.content == "Hello"
    assert mock_post.call_count == 2


# -- Config tests --


def test_config_from_env() -> None:
    """load_llm_client_config() should read from environment variables."""
    env = {
        "CODEFORGE_LLM_MAX_RETRIES": "5",
        "CODEFORGE_LLM_BACKOFF_BASE": "3.0",
        "CODEFORGE_LLM_BACKOFF_MAX": "120.0",
        "CODEFORGE_LLM_TIMEOUT": "300.0",
    }
    with patch.dict(os.environ, env):
        cfg = load_llm_client_config()
    assert cfg.max_retries == 5
    assert cfg.backoff_base == pytest.approx(3.0)
    assert cfg.backoff_max == pytest.approx(120.0)
    assert cfg.timeout == pytest.approx(300.0)


def test_config_defaults() -> None:
    """load_llm_client_config() should use defaults when no env vars set."""
    env_keys = [
        "CODEFORGE_LLM_MAX_RETRIES",
        "CODEFORGE_LLM_BACKOFF_BASE",
        "CODEFORGE_LLM_BACKOFF_MAX",
        "CODEFORGE_LLM_TIMEOUT",
    ]
    clean_env = dict.fromkeys(env_keys, "")
    with patch.dict(os.environ, clean_env):
        cfg = load_llm_client_config()
    assert cfg.max_retries == 2
    assert cfg.backoff_base == pytest.approx(2.0)
    assert cfg.backoff_max == pytest.approx(60.0)
    assert cfg.timeout == pytest.approx(120.0)


# -- Rate-info extraction tests --


async def test_rate_info_extracted(client: LiteLLMClient) -> None:
    """Rate-limit headers should be parsed into a dict."""
    resp = _ok_response(
        **{
            "x-ratelimit-remaining-requests": "5",
            "x-ratelimit-limit-requests": "60",
            "x-ratelimit-reset-requests": "30",
        },
    )

    with patch.object(client._client, "post", new_callable=AsyncMock, return_value=resp):
        await client.completion(prompt="test", model="groq/llama-3.1-8b")

    # Verify the tracker was updated via the singleton.
    from codeforge.routing.rate_tracker import get_tracker

    tracker = get_tracker()
    assert not tracker.is_exhausted("groq")


async def test_rate_info_ms_parsing(client: LiteLLMClient) -> None:
    """Rate-limit reset header with ms suffix should be parsed."""
    info = LiteLLMClient._extract_rate_info(
        httpx.Headers(
            {
                "x-ratelimit-remaining-requests": "0",
                "x-ratelimit-reset-requests": "500ms",
            }
        ),
        "groq/llama-3.1-8b",
    )
    assert info is not None
    assert info["remaining_requests"] == 0
    assert info["reset_after_seconds"] == pytest.approx(0.5)


async def test_rate_info_missing_headers_ignored(client: LiteLLMClient) -> None:
    """When no rate-limit headers are present, extraction returns None."""
    info = LiteLLMClient._extract_rate_info(httpx.Headers({}), "groq/llama-3.1-8b")
    assert info is None


async def test_rate_info_updates_tracker(client: LiteLLMClient) -> None:
    """Rate info should update the global tracker when remaining=0."""
    resp = _ok_response(
        **{
            "x-ratelimit-remaining-requests": "0",
            "x-ratelimit-limit-requests": "60",
            "x-ratelimit-reset-requests": "30",
        },
    )

    with patch.object(client._client, "post", new_callable=AsyncMock, return_value=resp):
        await client.completion(prompt="test", model="mistral/mistral-small-latest")

    from codeforge.routing.rate_tracker import get_tracker

    assert get_tracker().is_exhausted("mistral")


# -- _extract_provider tests --


def test_extract_provider_with_slash() -> None:
    """_extract_provider should return the prefix before the slash."""
    assert _extract_provider("groq/llama-3.1-8b") == "groq"
    assert _extract_provider("mistral/mistral-small-latest") == "mistral"
    assert _extract_provider("ollama/llama3.2") == "ollama"


def test_extract_provider_without_slash() -> None:
    """_extract_provider should return the full name when no slash."""
    assert _extract_provider("gpt-4o") == "gpt-4o"
    assert _extract_provider("claude-3-opus") == "claude-3-opus"
