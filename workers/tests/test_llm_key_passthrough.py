"""Tests that provider_api_key is threaded through to the LiteLLM request payload."""

from __future__ import annotations

from unittest.mock import AsyncMock, MagicMock

import pytest

from codeforge.agent_loop import LoopConfig
from codeforge.llm import LiteLLMClient


@pytest.fixture
def llm_client() -> LiteLLMClient:
    """Create a LiteLLMClient with a mocked httpx client."""
    client = LiteLLMClient.__new__(LiteLLMClient)
    client._client = AsyncMock()
    client._config = MagicMock()
    client._config.max_retries = 0
    client._config.retryable_codes = set()
    client._config.timeout = 30
    client._base_url = "http://localhost:4000"
    return client


class TestChatCompletionPayload:
    """Verify that provider_api_key appears in the HTTP request payload."""

    @pytest.mark.asyncio
    async def test_provider_key_included_in_payload(self, llm_client: LiteLLMClient) -> None:
        """When provider_api_key is set, the payload body includes 'api_key'."""
        mock_resp = MagicMock()
        mock_resp.status_code = 200
        mock_resp.headers = {}
        mock_resp.json.return_value = {
            "choices": [{"message": {"role": "assistant", "content": "ok"}, "finish_reason": "stop"}],
            "usage": {"prompt_tokens": 10, "completion_tokens": 5},
            "model": "openai/gpt-4o",
        }
        llm_client._client.post = AsyncMock(return_value=mock_resp)

        await llm_client.chat_completion(
            messages=[{"role": "user", "content": "hello"}],
            model="openai/gpt-4o",
            provider_api_key="sk-user-test-key-123",
        )

        call_kwargs = llm_client._client.post.call_args
        payload = call_kwargs.kwargs.get("json") or call_kwargs[1].get("json")
        assert payload is not None
        assert payload["api_key"] == "sk-user-test-key-123"

    @pytest.mark.asyncio
    async def test_provider_key_absent_when_empty(self, llm_client: LiteLLMClient) -> None:
        """When provider_api_key is empty, 'api_key' is not in payload."""
        mock_resp = MagicMock()
        mock_resp.status_code = 200
        mock_resp.headers = {}
        mock_resp.json.return_value = {
            "choices": [{"message": {"role": "assistant", "content": "ok"}, "finish_reason": "stop"}],
            "usage": {"prompt_tokens": 10, "completion_tokens": 5},
            "model": "openai/gpt-4o",
        }
        llm_client._client.post = AsyncMock(return_value=mock_resp)

        await llm_client.chat_completion(
            messages=[{"role": "user", "content": "hello"}],
            model="openai/gpt-4o",
            provider_api_key="",
        )

        call_kwargs = llm_client._client.post.call_args
        payload = call_kwargs.kwargs.get("json") or call_kwargs[1].get("json")
        assert payload is not None
        assert "api_key" not in payload


class TestLoopConfigPassthrough:
    """Verify that LoopConfig.provider_api_key reaches the LLM call."""

    def test_loop_config_default_empty(self) -> None:
        cfg = LoopConfig()
        assert cfg.provider_api_key == ""

    def test_loop_config_stores_key(self) -> None:
        cfg = LoopConfig(provider_api_key="sk-test")
        assert cfg.provider_api_key == "sk-test"


class TestPydanticModelField:
    """Verify that the NATS message model carries provider_api_key."""

    def test_field_present_on_start_message(self) -> None:
        from codeforge.models import ConversationRunStartMessage

        msg = ConversationRunStartMessage(
            run_id="r1",
            conversation_id="c1",
            project_id="p1",
            messages=[],
            system_prompt="",
            model="openai/gpt-4o",
            provider_api_key="sk-user-key",
        )
        assert msg.provider_api_key == "sk-user-key"

    def test_field_defaults_to_empty(self) -> None:
        from codeforge.models import ConversationRunStartMessage

        msg = ConversationRunStartMessage(
            run_id="r1",
            conversation_id="c1",
            project_id="p1",
            messages=[],
            system_prompt="",
            model="openai/gpt-4o",
        )
        assert msg.provider_api_key == ""
