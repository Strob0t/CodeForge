"""FakeLLM test harness — duck-types LiteLLMClient for deterministic testing."""

from __future__ import annotations

import json
from dataclasses import dataclass
from typing import TYPE_CHECKING

from codeforge.llm import CompletionResponse

if TYPE_CHECKING:
    from pathlib import Path


@dataclass
class LLMCall:
    """Record of a single call made to the FakeLLM."""

    prompt: str
    model: str
    system: str
    temperature: float
    tags: list[str] | None


class FakeLLM:
    """Deterministic LLM stand-in that returns pre-configured responses.

    Responses are consumed in order.  Raises RuntimeError if exhausted.
    All calls are recorded in ``calls`` for assertion.
    """

    def __init__(self, responses: list[CompletionResponse]) -> None:
        self._responses = list(responses)
        self._index = 0
        self.calls: list[LLMCall] = []

    @classmethod
    def from_fixture(cls, path: Path) -> FakeLLM:
        """Load responses from a ``llm_responses.json`` fixture file.

        Expected format::

            { "responses": [
                { "content": "...", "tokens_in": 10, "tokens_out": 5,
                  "model": "test-model", "cost_usd": 0.0 }
            ] }
        """
        data = json.loads(path.read_text())
        responses = [
            CompletionResponse(
                content=r["content"],
                tokens_in=r.get("tokens_in", 0),
                tokens_out=r.get("tokens_out", 0),
                model=r.get("model", "fake-model"),
                cost_usd=r.get("cost_usd", 0.0),
            )
            for r in data["responses"]
        ]
        return cls(responses)

    async def completion(
        self,
        prompt: str,
        model: str = "fake-model",
        system: str = "",
        temperature: float = 0.2,
        tags: list[str] | None = None,
    ) -> CompletionResponse:
        """Return the next canned response and record the call."""
        self.calls.append(LLMCall(prompt=prompt, model=model, system=system, temperature=temperature, tags=tags))
        if self._index >= len(self._responses):
            msg = f"FakeLLM exhausted: {len(self._responses)} responses consumed, but call #{self._index + 1} requested"
            raise RuntimeError(msg)
        resp = self._responses[self._index]
        self._index += 1
        return resp

    async def health(self) -> bool:
        """Always healthy."""
        return True

    async def close(self) -> None:
        """No-op."""
