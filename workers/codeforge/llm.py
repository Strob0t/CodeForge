"""LiteLLM Proxy client for LLM completions with scenario-based routing."""

from __future__ import annotations

import contextlib
import json
import logging
import os
from dataclasses import dataclass
from typing import TYPE_CHECKING

import httpx

if TYPE_CHECKING:
    from collections.abc import Callable

logger = logging.getLogger(__name__)

# Default model used when no model is specified in the request.
# Set via CODEFORGE_DEFAULT_MODEL env var or falls back to groq/llama-3.3-70b-versatile.
DEFAULT_MODEL: str = os.environ.get("CODEFORGE_DEFAULT_MODEL", "groq/llama-3.3-70b-versatile")


class LLMError(Exception):
    """Raised when the LLM proxy returns an error response."""

    def __init__(self, status_code: int, model: str, body: str) -> None:
        self.status_code = status_code
        self.model = model
        self.body = body
        # Truncate body for the message but keep it accessible via .body
        short = body[:500] if len(body) > 500 else body
        super().__init__(f"LiteLLM {status_code} for model={model}: {short}")


@dataclass(frozen=True)
class CompletionResponse:
    """Parsed response from an LLM completion call."""

    content: str
    tokens_in: int
    tokens_out: int
    model: str
    cost_usd: float = 0.0


@dataclass(frozen=True)
class ToolCallPart:
    """A single tool call from an LLM response."""

    id: str
    name: str
    arguments: str


@dataclass(frozen=True)
class ChatCompletionResponse:
    """Parsed response from a chat completion with tool-calling support."""

    content: str
    tool_calls: list[ToolCallPart]
    finish_reason: str
    tokens_in: int
    tokens_out: int
    model: str
    cost_usd: float = 0.0


@dataclass(frozen=True)
class ScenarioConfig:
    """Per-scenario LLM call defaults (tag for routing, temperature for generation)."""

    tag: str
    temperature: float


# Scenario -> default parameters.  Tags match litellm_params.tags in litellm/config.yaml.
# When no scenario tag is sent, LiteLLM routes by model name without tag filtering
# (with enable_tag_filtering: true, untagged requests can use ALL models).
SCENARIO_DEFAULTS: dict[str, ScenarioConfig] = {
    "background": ScenarioConfig(tag="background", temperature=0.1),
    "think": ScenarioConfig(tag="think", temperature=0.3),
    "longContext": ScenarioConfig(tag="longContext", temperature=0.2),
    "review": ScenarioConfig(tag="review", temperature=0.1),
    "plan": ScenarioConfig(tag="plan", temperature=0.3),
}

_FALLBACK = ScenarioConfig(tag="", temperature=0.2)


def resolve_scenario(scenario: str) -> ScenarioConfig:
    """Look up scenario config, falling back to no-tag routing for unknown values."""
    return SCENARIO_DEFAULTS.get(scenario, _FALLBACK)


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
        model: str = DEFAULT_MODEL,
        system: str = "",
        temperature: float = 0.2,
        tags: list[str] | None = None,
    ) -> CompletionResponse:
        """Send a chat completion request to LiteLLM.

        When *tags* is provided, LiteLLM routes to a model whose
        ``litellm_params.tags`` include at least one matching tag.
        """
        messages: list[dict[str, str]] = []
        if system:
            messages.append({"role": "system", "content": system})
        messages.append({"role": "user", "content": prompt})

        payload: dict[str, object] = {
            "model": model,
            "messages": messages,
            "temperature": temperature,
        }

        if tags:
            payload["tags"] = tags

        logger.debug(
            "llm_completion_request model=%s temperature=%.2f tags=%s prompt_len=%d",
            model,
            temperature,
            tags,
            len(prompt),
        )

        resp = await self._client.post("/v1/chat/completions", json=payload)
        if resp.status_code >= 400:
            body = resp.text
            logger.error(
                "LiteLLM error status=%d model=%s body=%s",
                resp.status_code,
                model,
                body[:1000],
            )
            raise LLMError(resp.status_code, model, body)
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

    async def chat_completion(
        self,
        messages: list[dict[str, object]],
        model: str = DEFAULT_MODEL,
        tools: list[dict[str, object]] | None = None,
        tool_choice: str | dict[str, object] | None = None,
        temperature: float = 0.2,
        tags: list[str] | None = None,
        max_tokens: int | None = None,
        response_format: dict[str, object] | None = None,
    ) -> ChatCompletionResponse:
        """Send a chat completion with tool-calling support.

        Returns a ChatCompletionResponse that includes parsed tool_calls
        and finish_reason alongside the standard content and usage fields.

        When *response_format* is provided, it is forwarded to the LLM API
        to request structured JSON output (e.g. ``{"type": "json_schema", ...}``).
        """
        payload: dict[str, object] = {
            "model": model,
            "messages": messages,
            "temperature": temperature,
        }
        if tools:
            payload["tools"] = tools
        if tool_choice is not None:
            payload["tool_choice"] = tool_choice
        if tags:
            payload["tags"] = tags
        if max_tokens is not None:
            payload["max_tokens"] = max_tokens
        if response_format is not None:
            payload["response_format"] = response_format

        logger.debug(
            "chat_completion model=%s tools=%d temperature=%.2f",
            model,
            len(tools) if tools else 0,
            temperature,
        )

        resp = await self._client.post("/v1/chat/completions", json=payload)
        if resp.status_code >= 400:
            body = resp.text
            logger.error(
                "LiteLLM error status=%d model=%s body=%s",
                resp.status_code,
                model,
                body[:1000],
            )
            raise LLMError(resp.status_code, model, body)
        data: dict[str, object] = resp.json()

        try:
            cost = float(resp.headers.get("x-litellm-response-cost", "0"))
        except (ValueError, TypeError):
            cost = 0.0

        choices = data.get("choices", [])
        if not isinstance(choices, list) or len(choices) == 0:
            return ChatCompletionResponse(
                content="",
                tool_calls=[],
                finish_reason="stop",
                tokens_in=0,
                tokens_out=0,
                model=model,
                cost_usd=cost,
            )

        choice = choices[0]
        finish_reason = choice.get("finish_reason", "stop") if isinstance(choice, dict) else "stop"
        message = choice.get("message", {}) if isinstance(choice, dict) else {}
        content = message.get("content", "") or "" if isinstance(message, dict) else ""

        tool_calls = _parse_tool_calls(message.get("tool_calls")) if isinstance(message, dict) else []

        usage = data.get("usage", {})
        tokens_in = usage.get("prompt_tokens", 0) if isinstance(usage, dict) else 0
        tokens_out = usage.get("completion_tokens", 0) if isinstance(usage, dict) else 0

        return ChatCompletionResponse(
            content=str(content),
            tool_calls=tool_calls,
            finish_reason=str(finish_reason),
            tokens_in=int(tokens_in),
            tokens_out=int(tokens_out),
            model=model,
            cost_usd=cost,
        )

    async def chat_completion_stream(
        self,
        messages: list[dict[str, object]],
        model: str = DEFAULT_MODEL,
        tools: list[dict[str, object]] | None = None,
        tool_choice: str | dict[str, object] | None = None,
        temperature: float = 0.2,
        tags: list[str] | None = None,
        max_tokens: int | None = None,
        on_chunk: Callable[[str], None] | None = None,
        on_tool_call: Callable[[ToolCallPart], None] | None = None,
    ) -> ChatCompletionResponse:
        """Stream a chat completion, accumulating tool_call deltas.

        *on_chunk* is called for each text delta.
        *on_tool_call* is called for each fully accumulated tool call.
        Returns the final assembled ChatCompletionResponse.
        """
        payload: dict[str, object] = {
            "model": model,
            "messages": messages,
            "temperature": temperature,
            "stream": True,
        }
        if tools:
            payload["tools"] = tools
        if tool_choice is not None:
            payload["tool_choice"] = tool_choice
        if tags:
            payload["tags"] = tags
        if max_tokens is not None:
            payload["max_tokens"] = max_tokens

        logger.debug(
            "chat_completion_stream model=%s tools=%d temperature=%.2f",
            model,
            len(tools) if tools else 0,
            temperature,
        )

        acc = _StreamAccumulator()

        async with self._client.stream("POST", "/v1/chat/completions", json=payload) as resp:
            if resp.status_code >= 400:
                body = (await resp.aread()).decode(errors="replace")
                logger.error(
                    "LiteLLM error status=%d model=%s body=%s",
                    resp.status_code,
                    model,
                    body[:1000],
                )
                raise LLMError(resp.status_code, model, body)
            with contextlib.suppress(ValueError, TypeError):
                acc.cost = float(resp.headers.get("x-litellm-response-cost", "0"))

            async for line in resp.aiter_lines():
                if not line.startswith("data: "):
                    continue
                raw = line[6:]
                if raw.strip() == "[DONE]":
                    break
                acc.process_chunk(raw, on_chunk)

        tool_calls = acc.build_tool_calls(on_tool_call)

        return ChatCompletionResponse(
            content="".join(acc.content_parts),
            tool_calls=tool_calls,
            finish_reason=acc.finish_reason,
            tokens_in=int(acc.tokens_in),
            tokens_out=int(acc.tokens_out),
            model=model,
            cost_usd=acc.cost,
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


class _StreamAccumulator:
    """Accumulates SSE stream chunks for chat completion responses."""

    __slots__ = ("content_parts", "cost", "finish_reason", "tc_accum", "tokens_in", "tokens_out")

    def __init__(self) -> None:
        self.content_parts: list[str] = []
        self.tc_accum: dict[int, dict[str, str]] = {}
        self.finish_reason = "stop"
        self.tokens_in = 0
        self.tokens_out = 0
        self.cost = 0.0

    def process_chunk(self, raw: str, on_chunk: Callable[[str], None] | None) -> None:
        """Parse a single SSE data line and accumulate into state."""
        try:
            chunk = json.loads(raw)
        except json.JSONDecodeError:
            return

        choices = chunk.get("choices", [])
        if not choices:
            return
        delta = choices[0].get("delta", {})
        chunk_finish = choices[0].get("finish_reason")
        if chunk_finish:
            self.finish_reason = chunk_finish

        text = delta.get("content")
        if text:
            self.content_parts.append(text)
            if on_chunk:
                on_chunk(text)

        for tc_delta in delta.get("tool_calls", []):
            idx = tc_delta.get("index", 0)
            if idx not in self.tc_accum:
                self.tc_accum[idx] = {"id": "", "name": "", "arguments": ""}
            acc = self.tc_accum[idx]
            if "id" in tc_delta:
                acc["id"] = tc_delta["id"]
            func = tc_delta.get("function", {})
            if "name" in func:
                acc["name"] = func["name"]
            if "arguments" in func:
                acc["arguments"] += func["arguments"]

        usage = chunk.get("usage")
        if isinstance(usage, dict):
            self.tokens_in = usage.get("prompt_tokens", self.tokens_in)
            self.tokens_out = usage.get("completion_tokens", self.tokens_out)

    def build_tool_calls(self, on_tool_call: Callable[[ToolCallPart], None] | None) -> list[ToolCallPart]:
        """Build final ToolCallPart list from accumulated deltas."""
        result: list[ToolCallPart] = []
        for idx in sorted(self.tc_accum):
            acc = self.tc_accum[idx]
            tc = ToolCallPart(id=acc["id"], name=acc["name"], arguments=acc["arguments"])
            result.append(tc)
            if on_tool_call:
                on_tool_call(tc)
        return result


def _parse_tool_calls(raw: object) -> list[ToolCallPart]:
    """Parse tool_calls from a non-streaming chat completion response."""
    if not isinstance(raw, list):
        return []
    result: list[ToolCallPart] = []
    for tc in raw:
        if not isinstance(tc, dict):
            continue
        func = tc.get("function")
        if not isinstance(func, dict) or "name" not in func:
            continue
        result.append(
            ToolCallPart(
                id=str(tc.get("id", "")),
                name=str(func["name"]),
                arguments=str(func.get("arguments", "")),
            )
        )
    return result
