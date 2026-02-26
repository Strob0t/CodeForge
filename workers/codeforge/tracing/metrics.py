"""AgentNeo-based evaluation metrics for agent observability.

These metrics leverage AgentNeo's built-in evaluation capabilities
to measure tool selection, goal decomposition, and plan adaptability.
Only available when AgentNeo is installed and tracing is enabled.

When AgentNeo is unavailable (ImportError) or evaluation fails at runtime,
each metric gracefully degrades to 0.0 with a specific warning log. This
is intentional — the metrics are supplementary observability signals, not
hard requirements for the benchmark pipeline.
"""

from __future__ import annotations

from typing import TYPE_CHECKING

import structlog

if TYPE_CHECKING:
    from codeforge.tracing.setup import TracerProtocol

logger = structlog.get_logger()


async def evaluate_tool_selection_accuracy(session: TracerProtocol) -> float:
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
    except ImportError:
        logger.warning("tool_selection_accuracy unavailable — agentneo not installed")
        return 0.0
    except Exception:
        logger.exception("tool_selection_accuracy evaluation failed at runtime")
        return 0.0


async def evaluate_goal_decomposition(session: TracerProtocol) -> float:
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
    except ImportError:
        logger.warning("goal_decomposition unavailable — agentneo not installed")
        return 0.0
    except Exception:
        logger.exception("goal_decomposition evaluation failed at runtime")
        return 0.0


async def evaluate_plan_adaptability(session: TracerProtocol) -> float:
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
    except ImportError:
        logger.warning("plan_adaptability unavailable — agentneo not installed")
        return 0.0
    except Exception:
        logger.exception("plan_adaptability evaluation failed at runtime")
        return 0.0
