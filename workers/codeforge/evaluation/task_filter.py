"""Universal task filter for benchmark providers.

Applies difficulty filtering, shuffling, and task count capping
to any list of TaskSpec objects. Used by all providers uniformly.
"""

from __future__ import annotations

import math
import random
from typing import TYPE_CHECKING

if TYPE_CHECKING:
    from codeforge.evaluation.providers.base import TaskSpec


def apply_task_filters(tasks: list[TaskSpec], config: dict) -> list[TaskSpec]:
    """Apply universal filters to a task list based on provider config.

    Supports:
    - difficulty_filter: list of allowed difficulty levels
    - shuffle: bool (default True)
    - seed: int for reproducible shuffle (default 42)
    - max_tasks: cap absolute count (0 = unlimited)
    - task_percentage: cap as percentage of total (100 = all)

    When both max_tasks and task_percentage are set, the more restrictive wins.
    """
    if not tasks:
        return []

    result = list(tasks)

    # 1. Difficulty filter
    difficulties: list[str] = config.get("difficulty_filter", [])
    if difficulties:
        result = [t for t in result if t.difficulty in difficulties]

    # 2. Shuffle (not cryptographic — deterministic benchmark ordering)
    if config.get("shuffle", True):
        rng = random.Random(config.get("seed", 42))  # noqa: S311
        rng.shuffle(result)

    # 3. Cap by percentage first, then by max_tasks — more restrictive wins
    total = len(result)
    percentage: int | float = config.get("task_percentage", 100)
    if 0 < percentage < 100:
        cap_by_pct = max(1, math.ceil(total * percentage / 100))
        result = result[:cap_by_pct]

    max_tasks: int = config.get("max_tasks", 0)
    if max_tasks > 0:
        result = result[:max_tasks]

    return result
