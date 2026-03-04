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
    from agentneo import AgentNeo

logger = structlog.get_logger()


def _run_metric(session: AgentNeo, metric_name: str) -> float:
    """Run a single AgentNeo evaluation metric and return its score.

    Args:
        session: AgentNeo session object containing the execution trace.
        metric_name: Name of the metric to evaluate.

    Returns:
        Score in [0, 1] where 1.0 is best.
    """
    try:
        from agentneo import Evaluation

        evaluation = Evaluation(session=session, metrics=[metric_name])
        evaluation.run()
        results = evaluation.result()
        if isinstance(results, dict):
            return float(results.get(metric_name, {}).get("score", 0.0))
        return 0.0
    except ImportError:
        logger.warning("%s unavailable — agentneo not installed", metric_name)
        return 0.0
    except Exception as exc:
        logger.exception("%s evaluation failed at runtime", metric_name, error=str(exc))
        return 0.0


async def evaluate_tool_selection_accuracy(session: AgentNeo) -> float:
    """Evaluate how well the agent selects appropriate tools for the task.

    Args:
        session: AgentNeo session object containing the execution trace.

    Returns:
        Score in [0, 1] where 1.0 means perfect tool selection.
    """
    return _run_metric(session, "tool_selection_accuracy")


async def evaluate_goal_decomposition(session: AgentNeo) -> float:
    """Evaluate how well the agent breaks down complex tasks into sub-goals.

    Args:
        session: AgentNeo session object.

    Returns:
        Score in [0, 1] where 1.0 means excellent decomposition.
    """
    return _run_metric(session, "goal_decomposition_efficiency")


async def evaluate_plan_adaptability(session: AgentNeo) -> float:
    """Evaluate how well the agent adapts when tools fail or return unexpected results.

    Args:
        session: AgentNeo session object.

    Returns:
        Score in [0, 1] where 1.0 means excellent adaptability.
    """
    return _run_metric(session, "plan_adaptability")
