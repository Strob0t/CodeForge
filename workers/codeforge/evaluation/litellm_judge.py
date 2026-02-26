"""Custom DeepEval LLM wrapper that routes judge calls through LiteLLM proxy."""

from __future__ import annotations

from typing import TYPE_CHECKING

import httpx
from deepeval.models import DeepEvalBaseLLM

if TYPE_CHECKING:
    from pydantic import BaseModel


class LiteLLMJudge(DeepEvalBaseLLM):
    """Judge LLM that calls the local LiteLLM proxy for evaluation scoring.

    Uses the OpenAI-compatible ``/v1/chat/completions`` endpoint exposed by
    the LiteLLM sidecar container.
    """

    def __init__(
        self,
        model: str = "openai/gpt-4o",
        base_url: str = "http://codeforge-litellm:4000/v1",
        api_key: str = "sk-codeforge",
        timeout: float = 120.0,
    ) -> None:
        self.model_name = model
        self._base_url = base_url.rstrip("/")
        self._api_key = api_key
        self._timeout = timeout

    def get_model_name(self) -> str:
        return self.model_name

    def load_model(self) -> str:
        return self.model_name

    async def a_generate(self, prompt: str, schema: type[BaseModel] | None = None, **kwargs: object) -> str | BaseModel:
        """Asynchronous generation via LiteLLM proxy."""
        payload: dict[str, object] = {
            "model": self.model_name,
            "messages": [{"role": "user", "content": prompt}],
            "temperature": 0.0,
        }
        if schema is not None:
            payload["response_format"] = {"type": "json_object"}
        async with httpx.AsyncClient(timeout=self._timeout) as client:
            resp = await client.post(
                f"{self._base_url}/chat/completions",
                json=payload,
                headers={"Authorization": f"Bearer {self._api_key}"},
            )
            resp.raise_for_status()
            data = resp.json()
            content = data["choices"][0]["message"]["content"]
            if schema is not None:
                return schema.model_validate_json(content)
            return content

    def generate(self, prompt: str, schema: type[BaseModel] | None = None, **kwargs: object) -> str | BaseModel:
        """Synchronous generation via LiteLLM proxy."""
        payload: dict[str, object] = {
            "model": self.model_name,
            "messages": [{"role": "user", "content": prompt}],
            "temperature": 0.0,
        }
        if schema is not None:
            payload["response_format"] = {"type": "json_object"}
        with httpx.Client(timeout=self._timeout) as client:
            resp = client.post(
                f"{self._base_url}/chat/completions",
                json=payload,
                headers={"Authorization": f"Bearer {self._api_key}"},
            )
            resp.raise_for_status()
            data = resp.json()
            content = data["choices"][0]["message"]["content"]
            if schema is not None:
                return schema.model_validate_json(content)
            return content
