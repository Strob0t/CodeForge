"""Evaluation metrics for role-based agent testing."""

from __future__ import annotations

from dataclasses import dataclass


@dataclass
class EvaluationMetrics:
    """Lightweight metrics container for a single evaluation run."""

    passed: bool
    tokens_in: int = 0
    tokens_out: int = 0
    step_count: int = 0
    cost_usd: float = 0.0
    artifact_quality: float = 0.0  # 0.0 - 1.0
