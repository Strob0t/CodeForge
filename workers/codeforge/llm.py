"""LiteLLM Proxy client for LLM completions with scenario-based routing."""

from __future__ import annotations

import asyncio
import contextlib
import json
import logging
import os
import re
import time
from dataclasses import dataclass, field
from typing import TYPE_CHECKING, cast

import httpx

if TYPE_CHECKING:
    from collections.abc import Awaitable, Callable

logger = logging.getLogger(__name__)

# Default model — empty means auto-discover from LiteLLM.
# Override via CODEFORGE_DEFAULT_MODEL env var if needed.
DEFAULT_MODEL: str = os.environ.get("CODEFORGE_DEFAULT_MODEL", "")


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


@dataclass(frozen=True)
class RoutingResult:
    """Result of model routing — model, temperature, tags, and routing metadata."""

    model: str = ""
    temperature: float = 0.2
    tags: list[str] = field(default_factory=list)
    routing_layer: str = ""
    complexity_tier: str = ""
    task_type: str = ""


# ---------------------------------------------------------------------------
# LLM Client Configuration (retry, backoff, timeout)
# ---------------------------------------------------------------------------


@dataclass(frozen=True)
class LLMClientConfig:
    """Environment-driven configuration for LiteLLMClient retry behaviour."""

    max_retries: int = 2
    backoff_base: float = 2.0
    backoff_max: float = 60.0
    timeout: float = 120.0
    retryable_codes: tuple[int, ...] = (429, 502, 503, 504)


def load_llm_client_config() -> LLMClientConfig:
    """Load LLM client config from environment variables."""

    def _float(key: str, default: float) -> float:
        val = os.environ.get(key, "")
        try:
            return float(val) if val else default
        except ValueError:
            return default

    def _int(key: str, default: int) -> int:
        val = os.environ.get(key, "")
        try:
            return int(val) if val else default
        except ValueError:
            return default

    return LLMClientConfig(
        max_retries=_int("CODEFORGE_LLM_MAX_RETRIES", 2),
        backoff_base=_float("CODEFORGE_LLM_BACKOFF_BASE", 2.0),
        backoff_max=_float("CODEFORGE_LLM_BACKOFF_MAX", 60.0),
        timeout=_float("CODEFORGE_LLM_TIMEOUT", 120.0),
    )


def _extract_provider(model: str) -> str:
    """Extract the provider prefix from a model name.

    ``"groq/llama-3.1-8b"`` -> ``"groq"``
    ``"gpt-4o"``             -> ``"gpt-4o"``
    """
    return model.split("/", 1)[0] if "/" in model else model


def resolve_model_with_routing(
    prompt: str,
    scenario: str,
    router: object | None = None,
    max_cost: float | None = None,
) -> RoutingResult:
    """Resolve model, temperature, and tags — using HybridRouter when available.

    Returns a RoutingResult with model name, temperature, tags, and routing metadata.
    """
    scenario_cfg = resolve_scenario(scenario)

    if router is not None:
        from codeforge.routing.router import HybridRouter

        if isinstance(router, HybridRouter):
            decision = router.route(prompt, max_cost=max_cost)
            if decision is not None:
                logger.info(
                    "routing_decision model=%s layer=%s tier=%s task=%s",
                    decision.model,
                    decision.routing_layer,
                    decision.complexity_tier,
                    decision.task_type,
                )
                return RoutingResult(
                    model=decision.model,
                    temperature=scenario_cfg.temperature,
                    routing_layer=str(decision.routing_layer),
                    complexity_tier=str(decision.complexity_tier),
                    task_type=str(decision.task_type),
                )

    # Fallback: tag-based routing via LiteLLM.
    tags = [scenario_cfg.tag] if scenario_cfg.tag else []
    return RoutingResult(model="", temperature=scenario_cfg.temperature, tags=tags)


def load_routing_config() -> object | None:
    """Load routing config from environment variables. Returns RoutingConfig or None."""
    enabled = os.environ.get("CODEFORGE_ROUTING_ENABLED", "").lower() in ("1", "true", "yes")
    if not enabled:
        return None

    from codeforge.routing.models import RoutingConfig

    def _float(key: str, default: float) -> float:
        val = os.environ.get(key, "")
        try:
            return float(val) if val else default
        except ValueError:
            return default

    def _int(key: str, default: int) -> int:
        val = os.environ.get(key, "")
        try:
            return int(val) if val else default
        except ValueError:
            return default

    def _bool(key: str, default: bool) -> bool:
        val = os.environ.get(key, "")
        if not val:
            return default
        return val.lower() in ("1", "true", "yes")

    return RoutingConfig(
        enabled=True,
        complexity_enabled=_bool("CODEFORGE_ROUTING_COMPLEXITY_ENABLED", True),
        mab_enabled=_bool("CODEFORGE_ROUTING_MAB_ENABLED", True),
        llm_meta_enabled=_bool("CODEFORGE_ROUTING_LLM_META_ENABLED", True),
        mab_min_trials=_int("CODEFORGE_ROUTING_MAB_MIN_TRIALS", 10),
        mab_exploration_rate=_float("CODEFORGE_ROUTING_MAB_EXPLORATION_RATE", 1.414),
        cost_weight=_float("CODEFORGE_ROUTING_COST_WEIGHT", 0.3),
        quality_weight=_float("CODEFORGE_ROUTING_QUALITY_WEIGHT", 0.5),
        latency_weight=_float("CODEFORGE_ROUTING_LATENCY_WEIGHT", 0.2),
        meta_router_model=os.environ.get("CODEFORGE_ROUTING_META_MODEL", ""),
        stats_refresh_interval=os.environ.get("CODEFORGE_ROUTING_STATS_INTERVAL", "5m"),
    )


# ---------------------------------------------------------------------------
# Rate-limit header parsing
# ---------------------------------------------------------------------------

_DURATION_RE = re.compile(r"(?:(\d+)h)?(?:(\d+)m)?(?:(\d+(?:\.\d+)?)s)?(?:(\d+)ms)?")


def _parse_duration(value: str) -> float | None:
    """Parse a duration string like ``'1m30s'``, ``'500ms'``, or plain seconds."""
    try:
        return float(value)
    except ValueError:
        pass
    m = _DURATION_RE.fullmatch(value.strip())
    if m is None:
        return None
    h, mi, s, ms = m.groups()
    total = 0.0
    if h:
        total += int(h) * 3600
    if mi:
        total += int(mi) * 60
    if s:
        total += float(s)
    if ms:
        total += int(ms) / 1000
    return total if total > 0 else None


class LiteLLMClient:
    """HTTP client for the LiteLLM Proxy (OpenAI-compatible API)."""

    def __init__(
        self,
        base_url: str = "http://localhost:4000",
        api_key: str = "",
        config: LLMClientConfig | None = None,
    ) -> None:
        self._config = config or load_llm_client_config()
        self._base_url = base_url.rstrip("/")
        headers: dict[str, str] = {"Content-Type": "application/json"}
        if api_key:
            headers["Authorization"] = f"Bearer {api_key}"
        self._client = httpx.AsyncClient(
            base_url=self._base_url,
            headers=headers,
            timeout=self._config.timeout,
        )

    # -- retry / resilience helpers -----------------------------------------

    def _is_retryable(self, exc: LLMError) -> bool:
        return exc.status_code in self._config.retryable_codes

    @staticmethod
    def _parse_retry_after(exc: LLMError) -> float | None:
        """Extract a Retry-After hint from the error body or well-known JSON fields."""
        try:
            data = json.loads(exc.body)
        except (json.JSONDecodeError, TypeError):
            return None
        for key in ("retry_after", "Retry-After", "retry-after"):
            val = data.get(key)
            if val is not None:
                try:
                    return float(val)
                except (ValueError, TypeError):
                    pass
        return None

    def _compute_backoff(self, exc: LLMError, attempt: int) -> float:
        hint = self._parse_retry_after(exc)
        if hint is not None:
            return min(hint, self._config.backoff_max)
        return min(self._config.backoff_base ** (attempt + 1), self._config.backoff_max)

    async def _with_retry(self, fn: Callable[..., Awaitable[object]], *args: object, **kwargs: object) -> object:
        last_exc: LLMError | None = None
        for attempt in range(self._config.max_retries + 1):
            try:
                return await fn(*args, **kwargs)
            except LLMError as exc:
                last_exc = exc
                if not self._is_retryable(exc) or attempt == self._config.max_retries:
                    raise
                wait = self._compute_backoff(exc, attempt)
                logger.warning(
                    "LLM %d, retry %d/%d in %.1fs (model=%s)",
                    exc.status_code,
                    attempt + 1,
                    self._config.max_retries,
                    wait,
                    exc.model,
                )
                await asyncio.sleep(wait)
        raise last_exc  # type: ignore[misc]  # unreachable

    # -- rate-limit header extraction ---------------------------------------

    @staticmethod
    def _extract_rate_info(headers: httpx.Headers, model: str) -> dict[str, object] | None:
        """Parse ``x-ratelimit-*`` headers into a dict suitable for RateLimitTracker."""
        remaining_raw = headers.get("x-ratelimit-remaining-requests")
        if remaining_raw is None:
            return None
        try:
            remaining = int(remaining_raw)
        except (ValueError, TypeError):
            return None
        limit_raw = headers.get("x-ratelimit-limit-requests")
        limit: int | None = None
        if limit_raw is not None:
            with contextlib.suppress(ValueError, TypeError):
                limit = int(limit_raw)
        reset_raw = headers.get("x-ratelimit-reset-requests") or headers.get("retry-after")
        reset_seconds: float | None = None
        if reset_raw is not None:
            reset_seconds = _parse_duration(reset_raw)
        return {
            "remaining_requests": remaining,
            "limit_requests": limit,
            "reset_after_seconds": reset_seconds,
            "provider": _extract_provider(model),
            "timestamp": time.monotonic(),
        }

    @staticmethod
    def _report_rate_info(info: dict[str, object] | None) -> None:
        if info is None:
            return
        from codeforge.routing.rate_tracker import RateLimitInfo, get_tracker

        get_tracker().update(
            str(info["provider"]),
            RateLimitInfo(
                remaining_requests=info["remaining_requests"],  # type: ignore[arg-type]
                limit_requests=info["limit_requests"],  # type: ignore[arg-type]
                reset_after_seconds=info["reset_after_seconds"],  # type: ignore[arg-type]
                provider=str(info["provider"]),
                timestamp=float(info["timestamp"]),  # type: ignore[arg-type]
            ),
        )

    # -- public API ---------------------------------------------------------

    async def completion(
        self,
        prompt: str,
        model: str = "",
        system: str = "",
        temperature: float = 0.2,
        tags: list[str] | None = None,
    ) -> CompletionResponse:
        """Send a chat completion request to LiteLLM with automatic retry."""
        if not model:
            from codeforge.model_resolver import resolve_model

            model = resolve_model()

        async def _inner() -> CompletionResponse:
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

            self._report_rate_info(self._extract_rate_info(resp.headers, model))

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

        return cast("CompletionResponse", await self._with_retry(_inner))

    async def chat_completion(
        self,
        messages: list[dict[str, object]],
        model: str = "",
        tools: list[dict[str, object]] | None = None,
        tool_choice: str | dict[str, object] | None = None,
        temperature: float = 0.2,
        tags: list[str] | None = None,
        max_tokens: int | None = None,
        response_format: dict[str, object] | None = None,
    ) -> ChatCompletionResponse:
        """Send a chat completion with tool-calling support and automatic retry."""
        if not model:
            from codeforge.model_resolver import resolve_model

            model = resolve_model()

        async def _inner() -> ChatCompletionResponse:
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

            self._report_rate_info(self._extract_rate_info(resp.headers, model))

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
            msg = choice.get("message", {}) if isinstance(choice, dict) else {}
            content = msg.get("content", "") or "" if isinstance(msg, dict) else ""

            tool_calls = _parse_tool_calls(msg.get("tool_calls")) if isinstance(msg, dict) else []

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

        return cast("ChatCompletionResponse", await self._with_retry(_inner))

    async def chat_completion_stream(
        self,
        messages: list[dict[str, object]],
        model: str = "",
        tools: list[dict[str, object]] | None = None,
        tool_choice: str | dict[str, object] | None = None,
        temperature: float = 0.2,
        tags: list[str] | None = None,
        max_tokens: int | None = None,
        on_chunk: Callable[[str], None] | None = None,
        on_tool_call: Callable[[ToolCallPart], None] | None = None,
    ) -> ChatCompletionResponse:
        """Stream a chat completion with automatic retry on transient errors."""
        if not model:
            from codeforge.model_resolver import resolve_model

            model = resolve_model()

        async def _inner() -> ChatCompletionResponse:
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
                self._report_rate_info(self._extract_rate_info(resp.headers, model))

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

        return cast("ChatCompletionResponse", await self._with_retry(_inner))

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
