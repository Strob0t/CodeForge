"""AgentNeo-based evaluation metrics for agent observability.

These metrics leverage AgentNeo's built-in evaluation capabilities
to measure tool selection, goal decomposition, and plan adaptability.
Only available when AgentNeo is installed and tracing is enabled.
"""

from __future__ import annotations

from typing import Any

import structlog

logger = structlog.get_logger()


async def evaluate_tool_selection_accuracy(session: Any) -> float:
    """Evaluate how well the agent selects appropriate tools for the task.

    Uses AgentNeo's built-in metric evaluation with an LLM judge to
    assess whether the tools chosen were optimal for the task at hand.

    Args:
        session: AgentNeo session object containing the execution trace.

    Returns:
        Score in [0, 1] where 1.0 means perfect tool selection.
    """
    try:
        from agentneo import evaluate

        result = evaluate(session, metric="tool_selection_accuracy")
        return float(result.get("score", 0.0))
    except (ImportError, Exception):
        logger.warning("tool_selection_accuracy evaluation failed")
        return 0.0


async def evaluate_goal_decomposition(session: Any) -> float:
    """Evaluate how well the agent breaks down complex tasks into sub-goals.

    Args:
        session: AgentNeo session object.

    Returns:
        Score in [0, 1] where 1.0 means excellent decomposition.
    """
    try:
        from agentneo import evaluate

        result = evaluate(session, metric="goal_decomposition_efficiency")
        return float(result.get("score", 0.0))
    except (ImportError, Exception):
        logger.warning("goal_decomposition evaluation failed")
        return 0.0


async def evaluate_plan_adaptability(session: Any) -> float:
    """Evaluate how well the agent adapts when tools fail or return unexpected results.

    Args:
        session: AgentNeo session object.

    Returns:
        Score in [0, 1] where 1.0 means excellent adaptability.
    """
    try:
        from agentneo import evaluate

        result = evaluate(session, metric="plan_adaptability")
        return float(result.get("score", 0.0))
    except (ImportError, Exception):
        logger.warning("plan_adaptability evaluation failed")
        return 0.0
