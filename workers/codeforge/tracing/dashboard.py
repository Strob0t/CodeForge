"""Optional AgentNeo dashboard launcher for dev-mode trace visualization."""

from __future__ import annotations

import structlog

logger = structlog.get_logger()


def launch(port: int = 3100) -> None:
    """Start the AgentNeo React dashboard on the given port.

    Only works when ``agentneo`` is installed and ``APP_ENV=development``.
    The dashboard provides:
    - Execution graph visualization
    - Token and cost breakdown per run
    - Trace timeline with tool call details

    Args:
        port: HTTP port for the dashboard (default 3100, 0 = disabled).
    """
    if port == 0:
        logger.info("agentneo dashboard disabled (port=0)")
        return

    try:
        from agentneo import launch_dashboard

        logger.info("launching agentneo dashboard", port=port)
        launch_dashboard(port=port)
    except ImportError:
        logger.warning("agentneo not installed, dashboard unavailable")
    except Exception:
        logger.exception("failed to launch agentneo dashboard")
