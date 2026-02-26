"""Backend executor framework for agent backends."""

from codeforge.backends.aider import AiderExecutor
from codeforge.backends.goose import GooseExecutor
from codeforge.backends.opencode import OpenCodeExecutor
from codeforge.backends.openhands import OpenHandsExecutor
from codeforge.backends.plandex import PlandexExecutor
from codeforge.backends.router import BackendRouter


def build_default_router() -> BackendRouter:
    """Create a BackendRouter with all known backend executors registered."""
    router = BackendRouter()
    router.register(AiderExecutor())
    router.register(GooseExecutor())
    router.register(OpenHandsExecutor())
    router.register(OpenCodeExecutor())
    router.register(PlandexExecutor())
    return router


__all__ = [
    "AiderExecutor",
    "BackendRouter",
    "GooseExecutor",
    "OpenCodeExecutor",
    "OpenHandsExecutor",
    "PlandexExecutor",
    "build_default_router",
]
