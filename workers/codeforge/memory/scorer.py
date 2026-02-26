"""Composite memory scorer: semantic similarity + recency decay + importance.

Score = (w_semantic * cosine_similarity) + (w_recency * recency_decay) + (w_importance * importance)

Recency decay uses exponential decay: exp(-lambda * hours_since_creation)
with a configurable half-life (default 168 hours = 7 days).
"""

from __future__ import annotations

import math
from datetime import UTC, datetime

import numpy as np

from codeforge.memory.models import ScoreWeights


def _cosine_similarity(a: np.ndarray, b: np.ndarray) -> float:
    """Compute cosine similarity between two vectors."""
    norm_a = np.linalg.norm(a)
    norm_b = np.linalg.norm(b)
    if norm_a == 0 or norm_b == 0:
        return 0.0
    return float(np.dot(a, b) / (norm_a * norm_b))


class CompositeScorer:
    """Scores memories using a weighted combination of three factors."""

    def __init__(
        self,
        weights: ScoreWeights | None = None,
        half_life_hours: float = 168.0,
    ) -> None:
        self.weights = weights or ScoreWeights()
        # lambda = ln(2) / half_life
        self._decay_lambda = math.log(2) / half_life_hours

    def score(
        self,
        query_embedding: np.ndarray,
        memory_embedding: np.ndarray,
        created_at: datetime,
        importance: float,
    ) -> float:
        """Compute the composite score for a single memory."""
        # Semantic similarity (0 to 1)
        semantic = _cosine_similarity(query_embedding, memory_embedding)

        # Recency decay (0 to 1, exponential)
        now = datetime.now(UTC)
        hours_since = max(0.0, (now - created_at).total_seconds() / 3600)
        recency = math.exp(-self._decay_lambda * hours_since)

        # Importance is already 0 to 1

        return self.weights.semantic * semantic + self.weights.recency * recency + self.weights.importance * importance
