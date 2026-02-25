"""Custom DeepEval LLM wrapper that routes judge calls through LiteLLM proxy."""

from __future__ import annotations

import httpx
from deepeval.models import DeepEvalBaseLLM


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

    async def a_generate(self, prompt: str, **kwargs: object) -> str:
        """Asynchronous generation via LiteLLM proxy."""
        async with httpx.AsyncClient(timeout=self._timeout) as client:
            resp = await client.post(
                f"{self._base_url}/chat/completions",
                json={
                    "model": self.model_name,
                    "messages": [{"role": "user", "content": prompt}],
                    "temperature": 0.0,
                },
                headers={"Authorization": f"Bearer {self._api_key}"},
            )
            resp.raise_for_status()
            data = resp.json()
            return data["choices"][0]["message"]["content"]

    def generate(self, prompt: str, **kwargs: object) -> str:
        """Synchronous generation via LiteLLM proxy."""
        with httpx.Client(timeout=self._timeout) as client:
            resp = client.post(
                f"{self._base_url}/chat/completions",
                json={
                    "model": self.model_name,
                    "messages": [{"role": "user", "content": prompt}],
                    "temperature": 0.0,
                },
                headers={"Authorization": f"Bearer {self._api_key}"},
            )
            resp.raise_for_status()
            data = resp.json()
            return data["choices"][0]["message"]["content"]
