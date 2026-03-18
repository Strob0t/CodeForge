"""LLM-based re-ranking for context entries."""

from __future__ import annotations

import logging
import re
from dataclasses import dataclass
from typing import TYPE_CHECKING

if TYPE_CHECKING:
    from codeforge.llm import LiteLLMClient

logger = logging.getLogger(__name__)

_RERANK_SYSTEM = (
    "You are a code relevance ranker. Given a developer's query and a numbered list "
    "of context entries (files, snippets, summaries), output the numbers of the most "
    "relevant entries in order of relevance, one number per line. Output only numbers."
)

_MAX_PREVIEW_CHARS = 300
_MAX_RERANK_ENTRIES = 30


@dataclass(frozen=True)
class RerankEntry:
    """A context entry to be re-ranked."""

    path: str
    kind: str
    content: str
    priority: int
    tokens: int


@dataclass(frozen=True)
class RerankResult:
    """Result of re-ranking context entries."""

    entries: list[RerankEntry]
    fallback_used: bool = False
    tokens_in: int = 0
    tokens_out: int = 0
    cost_usd: float = 0.0


class ContextReranker:
    """Re-ranks context entries using an LLM for relevance scoring.

    Falls back to original priority ordering on any LLM failure.
    """

    def __init__(self, llm: LiteLLMClient, model: str = "") -> None:
        self._llm = llm
        self._model = model

    async def rerank(
        self,
        entries: list[RerankEntry],
        query: str,
    ) -> RerankResult:
        """Re-rank entries by LLM-judged relevance to query."""
        if len(entries) <= 1:
            return RerankResult(entries=list(entries))

        # Cap candidates
        candidates = entries[:_MAX_RERANK_ENTRIES]

        # Build numbered prompt
        prompt = self._build_prompt(candidates, query)

        try:
            resp = await self._llm.completion(
                prompt=prompt,
                model=self._model,
                system=_RERANK_SYSTEM,
                temperature=0.0,
                tags=["background"],
            )
        except Exception:
            logger.warning("context rerank LLM call failed, using original order")
            return RerankResult(entries=list(entries), fallback_used=True)

        # Parse ranking
        ranked = self._parse_ranking(resp.content, len(candidates))
        if not ranked:
            logger.warning("context rerank: could not parse LLM output, using original order")
            return RerankResult(
                entries=list(entries),
                fallback_used=True,
                tokens_in=resp.tokens_in,
                tokens_out=resp.tokens_out,
                cost_usd=resp.cost_usd,
            )

        # Reorder entries and assign new priorities (85 down to 60)
        reordered = [candidates[i] for i in ranked]
        # Append any entries beyond MAX that weren't ranked
        remaining = entries[_MAX_RERANK_ENTRIES:]

        priority_max = 85
        priority_min = 60
        step = (priority_max - priority_min) / max(len(reordered) - 1, 1)
        updated = [
            RerankEntry(
                path=e.path,
                kind=e.kind,
                content=e.content,
                priority=int(priority_max - i * step),
                tokens=e.tokens,
            )
            for i, e in enumerate(reordered)
        ]
        updated.extend(remaining)

        return RerankResult(
            entries=updated,
            tokens_in=resp.tokens_in,
            tokens_out=resp.tokens_out,
            cost_usd=resp.cost_usd,
        )

    def _build_prompt(self, entries: list[RerankEntry], query: str) -> str:
        lines = [f"Query: {query}\n\nContext entries:\n"]
        for i, e in enumerate(entries, 1):
            preview = e.content[:_MAX_PREVIEW_CHARS]
            lines.append(f"{i}. [{e.kind}] {e.path}\n{preview}\n")
        return "\n".join(lines)

    @staticmethod
    def _parse_ranking(output: str, n: int) -> list[int]:
        """Parse LLM output as list of 1-based indices, return 0-based."""
        indices: list[int] = []
        seen: set[int] = set()
        for token in re.findall(r"\d+", output):
            idx = int(token)
            if 1 <= idx <= n and idx not in seen:
                seen.add(idx)
                indices.append(idx - 1)  # 0-based
        # Need at least 2 valid indices to consider it a valid ranking
        if len(indices) < 2:
            return []
        return indices
