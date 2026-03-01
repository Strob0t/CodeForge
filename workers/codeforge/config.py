"""Worker configuration loaded from environment variables."""

from __future__ import annotations

import os


def resolve_backend_path(explicit: str | None, env_var: str, default: str) -> str:
    """Resolve a backend CLI/URL path using explicit value, env var, or default."""
    if explicit:
        return explicit
    return os.environ.get(env_var, default)


class WorkerSettings:
    """Configuration for the Python worker, loaded from environment variables.

    Prefix: CODEFORGE_WORKER_ for worker-specific settings.
    Falls back to shared env vars (NATS_URL, LITELLM_URL) for infrastructure.
    """

    nats_url: str
    litellm_url: str
    litellm_api_key: str
    log_level: str
    log_service: str
    health_port: int

    def __init__(self) -> None:
        self.nats_url = os.environ.get("NATS_URL", "nats://localhost:4222")
        self.litellm_url = os.environ.get("LITELLM_URL", "http://localhost:4000")
        self.litellm_api_key = os.environ.get("LITELLM_MASTER_KEY", "sk-codeforge-dev")
        self.log_level = os.environ.get("CODEFORGE_WORKER_LOG_LEVEL", "info")
        self.log_service = os.environ.get("CODEFORGE_WORKER_LOG_SERVICE", "codeforge-worker")
        self.health_port = int(os.environ.get("CODEFORGE_WORKER_HEALTH_PORT", "8081"))

        # Agent backend CLI paths
        self.aider_path = os.environ.get("CODEFORGE_AIDER_PATH", "aider")
        self.goose_path = os.environ.get("CODEFORGE_GOOSE_PATH", "goose")
        self.opencode_path = os.environ.get("CODEFORGE_OPENCODE_PATH", "opencode")
        self.plandex_path = os.environ.get("CODEFORGE_PLANDEX_PATH", "plandex")
        self.openhands_url = os.environ.get("CODEFORGE_OPENHANDS_URL", "http://localhost:3000")
