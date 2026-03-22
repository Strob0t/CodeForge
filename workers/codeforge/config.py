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

    # Core / Infrastructure
    core_url: str
    internal_key: str
    app_env: str
    database_url: str
    workspace: str
    config_file: str

    # LLM
    default_model: str

    # Consumer
    consumer_max_errors: int
    consumer_backoff_multiplier: float
    consumer_backoff_max: float

    # Claude Code
    claudecode_enabled: bool
    claudecode_path: str
    claudecode_max_concurrent: int
    claudecode_max_turns: int
    claudecode_timeout: int
    claudecode_tiers: str

    # Routing
    effective_models_cache_ttl: float
    model_block_ttl: float
    model_auth_block_ttl: float

    # Benchmark
    benchmark_max_parallel: int
    benchmark_datasets_dir: str

    # OpenTelemetry
    otel_enabled: bool
    otel_endpoint: str
    otel_service_name: str
    otel_insecure: bool
    otel_sample_rate: float

    # Plan/Act
    plan_act_max_iterations: int

    # Evaluation
    judge_model: str
    early_stop_threshold: float
    early_stop_quorum: int
    hf_token: str

    # Backends
    openhands_poll_interval: float
    openhands_http_timeout: float
    openhands_health_timeout: float
    openhands_cancel_timeout: float

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

        # --- Core / Infrastructure ---
        core_cfg: dict = yaml_cfg.get("core", {}) if isinstance(yaml_cfg.get("core"), dict) else {}
        self.core_url = _resolve_str("CODEFORGE_CORE_URL", core_cfg.get("url"), "http://localhost:8080")
        self.internal_key = _resolve_str("CODEFORGE_INTERNAL_KEY", core_cfg.get("internal_key"), "")
        self.app_env = _resolve_str("APP_ENV", yaml_cfg.get("app_env"), "")
        self.database_url = _resolve_str(
            "DATABASE_URL",
            yaml_cfg.get("postgres", {}).get("dsn") if isinstance(yaml_cfg.get("postgres"), dict) else None,
            "postgresql://codeforge:codeforge_dev@localhost:5432/codeforge",
        )
        self.workspace = _resolve_str("CODEFORGE_WORKSPACE", None, "/workspaces/CodeForge")
        self.config_file = os.environ.get("CODEFORGE_CONFIG_FILE", "")

        # --- LLM ---
        self.default_model = _resolve_str("CODEFORGE_DEFAULT_MODEL", litellm_cfg.get("default_model"), "")

        # --- Consumer ---
        consumer_cfg: dict = yaml_cfg.get("consumer", {}) if isinstance(yaml_cfg.get("consumer"), dict) else {}
        self.consumer_max_errors = _resolve_int("CODEFORGE_CONSUMER_MAX_ERRORS", consumer_cfg.get("max_errors"), 10)
        self.consumer_backoff_multiplier = _resolve_float(
            "CODEFORGE_CONSUMER_BACKOFF_MULTIPLIER", consumer_cfg.get("backoff_multiplier"), 0.5
        )
        self.consumer_backoff_max = _resolve_float(
            "CODEFORGE_CONSUMER_BACKOFF_MAX", consumer_cfg.get("backoff_max"), 5.0
        )

        # --- Claude Code ---
        claude_cfg: dict = yaml_cfg.get("claudecode", {}) if isinstance(yaml_cfg.get("claudecode"), dict) else {}
        self.claudecode_enabled = _resolve_bool("CODEFORGE_CLAUDECODE_ENABLED", claude_cfg.get("enabled"), False)
        self.claudecode_path = _resolve_str("CODEFORGE_CLAUDECODE_PATH", claude_cfg.get("path"), "claude")
        self.claudecode_max_concurrent = _resolve_int(
            "CODEFORGE_CLAUDECODE_MAX_CONCURRENT", claude_cfg.get("max_concurrent"), 5
        )
        self.claudecode_max_turns = _resolve_int("CODEFORGE_CLAUDECODE_MAX_TURNS", claude_cfg.get("max_turns"), 50)
        self.claudecode_timeout = _resolve_int("CODEFORGE_CLAUDECODE_TIMEOUT", claude_cfg.get("timeout"), 300)
        self.claudecode_tiers = _resolve_str("CODEFORGE_CLAUDECODE_TIERS", claude_cfg.get("tiers"), "COMPLEX,REASONING")

        # --- Routing ---
        self.effective_models_cache_ttl = _resolve_float(
            "CODEFORGE_EFFECTIVE_MODELS_CACHE_TTL", routing_cfg.get("effective_models_cache_ttl"), 5.0
        )
        self.model_block_ttl = _resolve_float("CODEFORGE_MODEL_BLOCK_TTL", routing_cfg.get("model_block_ttl"), 300.0)
        self.model_auth_block_ttl = _resolve_float(
            "CODEFORGE_MODEL_AUTH_BLOCK_TTL", routing_cfg.get("model_auth_block_ttl"), 86400.0
        )

        # --- Benchmark ---
        bench_cfg: dict = yaml_cfg.get("benchmark", {}) if isinstance(yaml_cfg.get("benchmark"), dict) else {}
        self.benchmark_max_parallel = _resolve_int("CODEFORGE_BENCHMARK_MAX_PARALLEL", bench_cfg.get("max_parallel"), 3)
        self.benchmark_datasets_dir = _resolve_str(
            "CODEFORGE_BENCHMARK_DATASETS_DIR", bench_cfg.get("datasets_dir"), "configs/benchmarks"
        )

        # --- OpenTelemetry ---
        otel_cfg: dict = yaml_cfg.get("otel", {}) if isinstance(yaml_cfg.get("otel"), dict) else {}
        self.otel_enabled = _resolve_bool("CODEFORGE_OTEL_ENABLED", otel_cfg.get("enabled"), False)
        self.otel_endpoint = _resolve_str("CODEFORGE_OTEL_ENDPOINT", otel_cfg.get("endpoint"), "localhost:4317")
        self.otel_service_name = _resolve_str(
            "CODEFORGE_OTEL_SERVICE_NAME", otel_cfg.get("service_name"), "codeforge-worker"
        )
        self.otel_insecure = _resolve_bool("CODEFORGE_OTEL_INSECURE", otel_cfg.get("insecure"), True)
        self.otel_sample_rate = _resolve_float("CODEFORGE_OTEL_SAMPLE_RATE", otel_cfg.get("sample_rate"), 1.0)

        # --- Plan/Act ---
        self.plan_act_max_iterations = _resolve_int("CODEFORGE_PLAN_ACT_MAX_ITERATIONS", None, 10)

        # --- Evaluation ---
        eval_cfg: dict = yaml_cfg.get("evaluation", {}) if isinstance(yaml_cfg.get("evaluation"), dict) else {}
        self.judge_model = _resolve_str("CODEFORGE_JUDGE_MODEL", eval_cfg.get("judge_model"), "openai/gpt-4o")
        self.early_stop_threshold = _resolve_float(
            "CODEFORGE_EARLY_STOP_THRESHOLD", eval_cfg.get("early_stop_threshold"), 0.9
        )
        self.early_stop_quorum = _resolve_int("CODEFORGE_EARLY_STOP_QUORUM", eval_cfg.get("early_stop_quorum"), 3)
        self.hf_token = _resolve_str("HF_TOKEN", None, "")

        # --- Backends ---
        backends_cfg: dict = yaml_cfg.get("backends", {}) if isinstance(yaml_cfg.get("backends"), dict) else {}
        openhands_cfg: dict = (
            backends_cfg.get("openhands", {}) if isinstance(backends_cfg.get("openhands"), dict) else {}
        )
        self.openhands_poll_interval = _resolve_float(
            "CODEFORGE_OPENHANDS_POLL_INTERVAL", openhands_cfg.get("poll_interval"), 2.0
        )
        self.openhands_http_timeout = _resolve_float(
            "CODEFORGE_OPENHANDS_HTTP_TIMEOUT", openhands_cfg.get("http_timeout"), 30.0
        )
        self.openhands_health_timeout = _resolve_float(
            "CODEFORGE_OPENHANDS_HEALTH_TIMEOUT", openhands_cfg.get("health_timeout"), 5.0
        )
        self.openhands_cancel_timeout = _resolve_float(
            "CODEFORGE_OPENHANDS_CANCEL_TIMEOUT", openhands_cfg.get("cancel_timeout"), 5.0
        )


@lru_cache(maxsize=1)
def get_settings() -> WorkerSettings:
    """Return cached WorkerSettings singleton."""
    return WorkerSettings()
