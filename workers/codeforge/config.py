"""Worker configuration — hierarchy: defaults < YAML < environment variables.

Mirrors Go Core's config loading (ADR-003). The YAML file path can be set
via ``CODEFORGE_CONFIG_FILE`` env var or auto-discovered from the cwd/parent.
"""

from __future__ import annotations

import logging
import os
from functools import lru_cache
from pathlib import Path

logger = logging.getLogger(__name__)

_DEFAULT_CONFIG_FILE = "codeforge.yaml"


def _find_config_file() -> Path | None:
    """Locate the YAML config file via env var or auto-discovery."""
    explicit = os.environ.get("CODEFORGE_CONFIG_FILE", "")
    if explicit:
        p = Path(explicit)
        return p if p.is_file() else None

    for candidate in (
        Path(_DEFAULT_CONFIG_FILE),
        Path("..") / _DEFAULT_CONFIG_FILE,
    ):
        resolved = candidate.resolve()
        if resolved.is_file():
            return resolved
    return None


@lru_cache(maxsize=1)
def load_yaml_config() -> dict:
    """Load the YAML config dict. Cached after first call. Returns {} if not found."""
    path = _find_config_file()
    if path is None:
        return {}
    try:
        import yaml

        with open(path) as f:
            data = yaml.safe_load(f)
        logger.info("loaded config from %s", path)
        return data if isinstance(data, dict) else {}
    except Exception as exc:
        logger.warning("failed to load config from %s: %s", path, exc)
        return {}


def _resolve_str(env_key: str, yaml_value: object, default: str) -> str:
    """Resolve a string setting: env var > YAML > default."""
    env = os.environ.get(env_key, "")
    if env:
        return env
    if yaml_value is not None and isinstance(yaml_value, str) and yaml_value:
        return yaml_value
    return default


def _resolve_bool(env_key: str, yaml_value: object, default: bool) -> bool:
    """Resolve a bool setting: env var > YAML > default."""
    env = os.environ.get(env_key, "")
    if env:
        return env.lower() in ("1", "true", "yes")
    if isinstance(yaml_value, bool):
        return yaml_value
    return default


def _resolve_int(env_key: str, yaml_value: object, default: int) -> int:
    """Resolve an int setting: env var > YAML > default."""
    env = os.environ.get(env_key, "")
    if env:
        try:
            return int(env)
        except ValueError:
            pass
    if isinstance(yaml_value, int):
        return yaml_value
    return default


def _resolve_float(env_key: str, yaml_value: object, default: float) -> float:
    """Resolve a float setting: env var > YAML > default."""
    env = os.environ.get(env_key, "")
    if env:
        try:
            return float(env)
        except ValueError:
            pass
    if isinstance(yaml_value, (int, float)):
        return float(yaml_value)
    return default


def resolve_backend_path(explicit: str | None, env_var: str, default: str) -> str:
    """Resolve a backend CLI/URL path using explicit value, env var, or default."""
    if explicit:
        return explicit
    return os.environ.get(env_var, default)


class WorkerSettings:
    """Configuration for the Python worker.

    Hierarchy: defaults < codeforge.yaml < environment variables (ADR-003).
    """

    nats_url: str
    litellm_url: str
    litellm_api_key: str
    log_level: str
    log_service: str
    health_port: int
    routing_enabled: bool
    trust_min_level: str

    def __init__(self) -> None:
        yaml_cfg = load_yaml_config()

        nats_cfg: dict = yaml_cfg.get("nats", {}) if isinstance(yaml_cfg.get("nats"), dict) else {}
        litellm_cfg: dict = yaml_cfg.get("litellm", {}) if isinstance(yaml_cfg.get("litellm"), dict) else {}
        logging_cfg: dict = yaml_cfg.get("logging", {}) if isinstance(yaml_cfg.get("logging"), dict) else {}
        routing_cfg: dict = yaml_cfg.get("routing", {}) if isinstance(yaml_cfg.get("routing"), dict) else {}
        trust_cfg: dict = yaml_cfg.get("trust", {}) if isinstance(yaml_cfg.get("trust"), dict) else {}

        self.nats_url = _resolve_str("NATS_URL", nats_cfg.get("url"), "nats://localhost:4222")
        self.litellm_url = _resolve_str("LITELLM_BASE_URL", litellm_cfg.get("url"), "http://localhost:4000")
        self.litellm_api_key = _resolve_str("LITELLM_MASTER_KEY", litellm_cfg.get("master_key"), "sk-codeforge-dev")
        if self.litellm_api_key == "sk-codeforge-dev":
            logger.warning("using default LiteLLM key 'sk-codeforge-dev' - set LITELLM_MASTER_KEY for production")
        self.log_level = _resolve_str("CODEFORGE_WORKER_LOG_LEVEL", logging_cfg.get("level"), "info")
        self.log_service = _resolve_str("CODEFORGE_WORKER_LOG_SERVICE", None, "codeforge-worker")
        self.health_port = _resolve_int("CODEFORGE_WORKER_HEALTH_PORT", None, 8081)

        self.routing_enabled = _resolve_bool("CODEFORGE_ROUTING_ENABLED", routing_cfg.get("enabled"), True)
        self.trust_min_level = _resolve_str("CODEFORGE_TRUST_MIN_LEVEL", trust_cfg.get("min_level"), "untrusted")
