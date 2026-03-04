"""SPARC evaluator — multi-dimensional quality assessment.

Evaluates agent benchmark results across 7 SPARC dimensions:
1. Correctness — functional test pass rate (delegates to FunctionalTestEvaluator)
2. Steps — normalized step count (fewer is better)
3. Time — normalized wall-clock time (faster is better)
4. Cost — normalized USD cost (cheaper is better)
5. Complexity — step-based categorization (simple/medium/complex/very_complex)
6. Code Quality — modularity, file size, naming conventions
7. Security — no hardcoded secrets, safe commands

Quantitative dimensions (steps, time, cost) are computed directly.
Qualitative dimensions (code_quality, security) use heuristic checks.
"""

from __future__ import annotations

import re

import structlog

from codeforge.evaluation.providers.base import EvalDimension, ExecutionResult, TaskSpec

logger = structlog.get_logger()

# Thresholds for step-based complexity classification.
COMPLEXITY_THRESHOLDS = {"simple": 5, "medium": 15, "complex": 30}

# Max expected values for normalization (configurable baselines).
DEFAULT_MAX_STEPS = 50
DEFAULT_MAX_DURATION_MS = 300_000  # 5 minutes
DEFAULT_MAX_COST_USD = 5.0

# Patterns that indicate hardcoded secrets.
SECRET_PATTERNS = [
    re.compile(r"(?i)(api[_-]?key|secret|password|token)\s*[=:]\s*['\"][^'\"]{8,}"),
    re.compile(r"sk-[a-zA-Z0-9]{20,}"),
    re.compile(r"ghp_[a-zA-Z0-9]{36}"),
]

# Patterns for dangerous shell commands.
UNSAFE_COMMAND_PATTERNS = [
    re.compile(r"\brm\s+-rf\s+/"),
    re.compile(r"\bcurl\s+.*\|\s*sh"),
    re.compile(r"\bwget\s+.*\|\s*sh"),
    re.compile(r"\bchmod\s+777\b"),
]


class SPARCEvaluator:
    """Multi-dimensional evaluator following the SPARC methodology."""

    def __init__(
        self,
        max_steps: int = DEFAULT_MAX_STEPS,
        max_duration_ms: int = DEFAULT_MAX_DURATION_MS,
        max_cost_usd: float = DEFAULT_MAX_COST_USD,
    ) -> None:
        self._max_steps = max_steps
        self._max_duration_ms = max_duration_ms
        self._max_cost_usd = max_cost_usd

    @property
    def name(self) -> str:
        return "sparc"

    @property
    def stage(self) -> str:
        return "rank"

    async def evaluate(self, task: TaskSpec, result: ExecutionResult) -> list[EvalDimension]:
        """Compute all SPARC dimensions for a task execution."""
        dimensions: list[EvalDimension] = []

        dimensions.append(self._score_steps(result))
        dimensions.append(self._score_time(result))
        dimensions.append(self._score_cost(result))
        dimensions.append(self._score_complexity(result))
        dimensions.append(self._score_code_quality(result))
        dimensions.append(self._score_security(result))

        return dimensions

    def _score_steps(self, result: ExecutionResult) -> EvalDimension:
        """Score step efficiency — fewer steps is better."""
        if result.step_count <= 0:
            return EvalDimension(name="sparc_steps", score=1.0, details={"steps": "0"})
        score = max(0.0, 1.0 - (result.step_count / self._max_steps))
        return EvalDimension(
            name="sparc_steps",
            score=round(score, 4),
            details={"steps": str(result.step_count), "max": str(self._max_steps)},
        )

    def _score_time(self, result: ExecutionResult) -> EvalDimension:
        """Score time efficiency — faster is better."""
        if result.duration_ms <= 0:
            return EvalDimension(name="sparc_time", score=1.0, details={"duration_ms": "0"})
        score = max(0.0, 1.0 - (result.duration_ms / self._max_duration_ms))
        return EvalDimension(
            name="sparc_time",
            score=round(score, 4),
            details={"duration_ms": str(result.duration_ms), "max_ms": str(self._max_duration_ms)},
        )

    def _score_cost(self, result: ExecutionResult) -> EvalDimension:
        """Score cost efficiency — cheaper is better."""
        if result.cost_usd <= 0:
            return EvalDimension(name="sparc_cost", score=1.0, details={"cost_usd": "0"})
        score = max(0.0, 1.0 - (result.cost_usd / self._max_cost_usd))
        return EvalDimension(
            name="sparc_cost",
            score=round(score, 4),
            details={"cost_usd": str(result.cost_usd), "max_usd": str(self._max_cost_usd)},
        )

    def _score_complexity(self, result: ExecutionResult) -> EvalDimension:
        """Classify complexity based on step count."""
        steps = result.step_count
        if steps <= COMPLEXITY_THRESHOLDS["simple"]:
            category = "simple"
            score = 1.0
        elif steps <= COMPLEXITY_THRESHOLDS["medium"]:
            category = "medium"
            score = 0.75
        elif steps <= COMPLEXITY_THRESHOLDS["complex"]:
            category = "complex"
            score = 0.5
        else:
            category = "very_complex"
            score = 0.25
        return EvalDimension(
            name="sparc_complexity",
            score=score,
            details={"category": category, "steps": str(steps)},
        )

    def _score_code_quality(self, result: ExecutionResult) -> EvalDimension:
        """Heuristic code quality assessment from execution metadata."""
        score = 1.0
        details: dict[str, str] = {}

        # Penalize if too many files changed (lack of modularity).
        file_count = len(result.files_changed)
        if file_count > 10:
            score -= 0.3
            details["many_files"] = str(file_count)
        elif file_count > 5:
            score -= 0.1
            details["moderate_files"] = str(file_count)

        # Check test output for quality signals.
        if result.test_output:
            lower = result.test_output.lower()
            if "warning" in lower:
                score -= 0.1
                details["warnings"] = "present"
            if "deprecat" in lower:
                score -= 0.1
                details["deprecations"] = "present"

        score = max(0.0, round(score, 4))
        return EvalDimension(name="sparc_code_quality", score=score, details=details)

    def _score_security(self, result: ExecutionResult) -> EvalDimension:
        """Check for hardcoded secrets and unsafe commands."""
        score = 1.0
        details: dict[str, str] = {}

        # Scan actual output and test output for secrets.
        text_to_scan = result.actual_output + "\n" + result.test_output
        for pattern in SECRET_PATTERNS:
            if pattern.search(text_to_scan):
                score -= 0.5
                details["hardcoded_secret"] = "detected"  # noqa: S105
                break

        # Scan for unsafe commands.
        for pattern in UNSAFE_COMMAND_PATTERNS:
            if pattern.search(text_to_scan):
                score -= 0.5
                details["unsafe_command"] = "detected"
                break

        score = max(0.0, round(score, 4))
        return EvalDimension(name="sparc_security", score=score, details=details)
