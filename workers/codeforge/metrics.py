"""Execution metrics accumulator for agent loops and runtime clients."""

from __future__ import annotations

from dataclasses import dataclass


@dataclass
class ExecutionMetrics:
    """Accumulates cost, token, and step counters during agent execution."""

    model: str = ""
    total_cost: float = 0.0
    total_tokens_in: int = 0
    total_tokens_out: int = 0
    step_count: int = 0

    def record(
        self,
        *,
        cost: float = 0.0,
        tokens_in: int = 0,
        tokens_out: int = 0,
        model: str = "",
    ) -> None:
        """Record metrics from a single LLM call or tool execution."""
        self.total_cost += cost
        self.total_tokens_in += tokens_in
        self.total_tokens_out += tokens_out
        if model:
            self.model = model
