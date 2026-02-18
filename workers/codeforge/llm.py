"""LiteLLM Proxy client for LLM completions."""

from __future__ import annotations

from dataclasses import dataclass

import httpx


@dataclass(frozen=True)
class CompletionResponse:
    """Parsed response from an LLM completion call."""

    content: str
    tokens_in: int
    tokens_out: int
    model: str
    cost_usd: float = 0.0


class LiteLLMClient:
    """HTTP client for the LiteLLM Proxy (OpenAI-compatible API)."""

    def __init__(self, base_url: str = "http://localhost:4000", api_key: str = "") -> None:
        self._base_url = base_url.rstrip("/")
        headers: dict[str, str] = {"Content-Type": "application/json"}
        if api_key:
            headers["Authorization"] = f"Bearer {api_key}"
        self._client = httpx.AsyncClient(base_url=self._base_url, headers=headers, timeout=120.0)

    async def completion(
        self,
        prompt: str,
        model: str = "ollama/llama3.2",
        system: str = "",
        temperature: float = 0.2,
    ) -> CompletionResponse:
        """Send a chat completion request to LiteLLM."""
        messages: list[dict[str, str]] = []
        if system:
            messages.append({"role": "system", "content": system})
        messages.append({"role": "user", "content": prompt})

        payload: dict[str, object] = {
            "model": model,
            "messages": messages,
            "temperature": temperature,
        }

        resp = await self._client.post("/v1/chat/completions", json=payload)
        resp.raise_for_status()
        data: dict[str, object] = resp.json()

        # Extract cost from LiteLLM response header (if available).
        try:
            litellm_cost = float(resp.headers.get("x-litellm-response-cost", "0"))
        except (ValueError, TypeError):
            litellm_cost = 0.0

        choices = data.get("choices", [])
        if not isinstance(choices, list) or len(choices) == 0:
            return CompletionResponse(content="", tokens_in=0, tokens_out=0, model=model, cost_usd=litellm_cost)

        message = choices[0].get("message", {})
        content = message.get("content", "") if isinstance(message, dict) else ""

        usage = data.get("usage", {})
        tokens_in = usage.get("prompt_tokens", 0) if isinstance(usage, dict) else 0
        tokens_out = usage.get("completion_tokens", 0) if isinstance(usage, dict) else 0

        return CompletionResponse(
            content=str(content),
            tokens_in=int(tokens_in),
            tokens_out=int(tokens_out),
            model=model,
            cost_usd=litellm_cost,
        )

    async def health(self) -> bool:
        """Check if the LiteLLM Proxy is healthy."""
        try:
            resp = await self._client.get("/health")
            return resp.status_code == 200
        except httpx.HTTPError:
            return False

    async def close(self) -> None:
        """Close the HTTP client."""
        await self._client.aclose()
