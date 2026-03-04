"""Tests for the LiteLLM health check in the benchmark handler."""

from __future__ import annotations

from unittest.mock import AsyncMock, patch

import pytest
import structlog

from codeforge.consumer._benchmark import (
    _HEALTH_MAX_ATTEMPTS,
    _wait_for_litellm,
)
from codeforge.llm import LiteLLMClient


@pytest.fixture
def log() -> structlog.stdlib.BoundLogger:
    return structlog.get_logger()


def _make_llm(health_sequence: list[bool]) -> LiteLLMClient:
    """Create a LiteLLMClient with a mocked health() returning values in order."""
    llm = LiteLLMClient.__new__(LiteLLMClient)
    llm.health = AsyncMock(side_effect=health_sequence)
    return llm


@pytest.mark.asyncio
@patch("codeforge.consumer._benchmark.asyncio.sleep", new_callable=AsyncMock)
async def test_healthy_immediately(
    mock_sleep: AsyncMock,
    log: structlog.stdlib.BoundLogger,
) -> None:
    """Returns True on first attempt when LiteLLM is already healthy."""
    llm = _make_llm([True])
    result = await _wait_for_litellm(llm, log)
    assert result is True
    assert llm.health.call_count == 1
    mock_sleep.assert_not_called()


@pytest.mark.asyncio
@patch("codeforge.consumer._benchmark.asyncio.sleep", new_callable=AsyncMock)
async def test_healthy_after_retries(
    mock_sleep: AsyncMock,
    log: structlog.stdlib.BoundLogger,
) -> None:
    """Returns True when LiteLLM becomes healthy after a few failures."""
    llm = _make_llm([False, False, True])
    result = await _wait_for_litellm(llm, log)
    assert result is True
    assert llm.health.call_count == 3
    assert mock_sleep.call_count == 2


@pytest.mark.asyncio
@patch("codeforge.consumer._benchmark.asyncio.sleep", new_callable=AsyncMock)
async def test_all_attempts_exhausted(
    mock_sleep: AsyncMock,
    log: structlog.stdlib.BoundLogger,
) -> None:
    """Returns False when all health check attempts fail."""
    llm = _make_llm([False] * _HEALTH_MAX_ATTEMPTS)
    result = await _wait_for_litellm(llm, log)
    assert result is False
    assert llm.health.call_count == _HEALTH_MAX_ATTEMPTS
    assert mock_sleep.call_count == _HEALTH_MAX_ATTEMPTS


@pytest.mark.asyncio
async def test_non_litellm_client_passes(log: structlog.stdlib.BoundLogger) -> None:
    """Non-LiteLLMClient objects skip the health check and return True."""
    result = await _wait_for_litellm(object(), log)
    assert result is True


@pytest.mark.asyncio
@patch("codeforge.consumer._benchmark.asyncio.sleep", new_callable=AsyncMock)
async def test_backoff_increases(
    mock_sleep: AsyncMock,
    log: structlog.stdlib.BoundLogger,
) -> None:
    """Verify exponential backoff intervals: 1, 2, 4, 8s."""
    llm = _make_llm([False, False, False, False, True])
    await _wait_for_litellm(llm, log)
    waits = [call.args[0] for call in mock_sleep.call_args_list]
    assert waits == [1.0, 2.0, 4.0, 8.0]
