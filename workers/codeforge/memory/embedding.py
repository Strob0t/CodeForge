"""Shared embedding computation helper for memory modules.

Extracted to a shared helper to avoid code duplication (FIX-056).
"""

from __future__ import annotations

from typing import TYPE_CHECKING

import numpy as np
import structlog

if TYPE_CHECKING:
    from codeforge.llm import LiteLLMClient

_logger = structlog.get_logger()


async def compute_embedding(llm: LiteLLMClient, text: str) -> np.ndarray | None:
    """Compute an embedding vector for the given text via LiteLLM.

    Shared helper used by ExperiencePool and memory consumer to avoid duplication.
    """
    try:
        resp = await llm.embedding(text)
        if resp and len(resp) > 0:
            return np.array(resp, dtype=np.float32)
    except Exception as exc:
        _logger.warning("embedding computation failed", exc_info=True, error=str(exc))
    return None
