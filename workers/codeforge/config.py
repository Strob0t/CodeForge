"""Worker configuration — hierarchy: defaults < YAML < environment variables.

Mirrors Go Core's config loading (ADR-003). The YAML file path can be set
via ``CODEFORGE_CONFIG_FILE`` env var or auto-discovered from the cwd/parent.
"""

from __future__ import annotations

import logging
import os
from functools import lru_cache
from pathlib import Path

from codeforge.provider_keys import KEYED_PROVIDERS_ENV, parse_keyed_providers
from codeforge.tool_identity import DEFAULT_HOME_BASE, DEFAULT_TOOL_PATH

logger = logging.getLogger(__name__)

_DEFAULT_CONFIG_FILE = "codeforge.yaml"

# LiteLLM master key of the development compose file; never valid in production.
DEV_LITELLM_MASTER_KEY = "sk-codeforge-dev"


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


FILE_ENV_SUFFIX = "_FILE"

# Contents of the secret files read so far. A secret file is read once: the
# worker locks its secrets directory after startup (codeforge.secrets), and
# settings built later must still see the values.
_secret_file_values: dict[str, str] = {}


def read_secret_file(path: str) -> str:
    """Return the content of a secret file without surrounding whitespace (read once, then cached)."""
    if path not in _secret_file_values:
        value = Path(path).read_text().strip()
        if not value:
            msg = f"secret file {path} is empty"
            raise ValueError(msg)
        _secret_file_values[path] = value
    return _secret_file_values[path]


def _resolve_secret(env_key: str, yaml_value: object, default: str) -> str:
    """Resolve a secret: <env_key>_FILE (a Docker secret file) or env var > YAML > default.

    Like the Go Core, a secret set both directly and as a file is rejected.
    """
    file_key = env_key + FILE_ENV_SUFFIX
    path = os.environ.get(file_key, "")
    if not path:
        return _resolve_str(env_key, yaml_value, default)
    if os.environ.get(env_key, ""):
        msg = f"both {env_key} and {file_key} are set, set only one"
        raise ValueError(msg)
    try:
        return read_secret_file(path)
    except (OSError, ValueError) as exc:
        msg = f"{file_key}: {exc}"
        raise ValueError(msg) from exc


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


MODEL_CAPABILITIES_ENV = "CODEFORGE_MODEL_CAPABILITIES"
# The levels of codeforge.tools.capability.CapabilityLevel (not imported here: that package imports this module).
_CAPABILITY_LEVELS = frozenset({"full", "api_with_tools", "pure_completion"})


def _resolve_model_capabilities(yaml_value: object) -> tuple[tuple[str, str], ...]:
    """Resolve the operator's tool-capability overrides: env var > YAML > none.

    The env var holds ``pattern=level`` entries separated by commas, the YAML
    key (``litellm.model_capabilities``) a mapping of pattern to level.
    Patterns are shell-style globs on the model name; the first match wins.
    An entry without a pattern or with an unknown level is refused.
    """
    env = os.environ.get(MODEL_CAPABILITIES_ENV, "")
    entries: list[tuple[str, str]] = []
    if env.strip():
        for item in env.split(","):
            if not item.strip():
                continue
            pattern, sep, level = item.partition("=")
            if not sep:
                msg = f"{MODEL_CAPABILITIES_ENV}: entry {item.strip()!r} is not pattern=level"
                raise ValueError(msg)
            entries.append((pattern.strip(), level.strip()))
    elif isinstance(yaml_value, dict):
        entries = [(str(pattern).strip(), str(level).strip()) for pattern, level in yaml_value.items()]
    for pattern, level in entries:
        if not pattern or level not in _CAPABILITY_LEVELS:
            msg = (
                f"{MODEL_CAPABILITIES_ENV} / litellm.model_capabilities: entry {pattern!r}={level!r} needs a "
                f"pattern and one of the levels {', '.join(sorted(_CAPABILITY_LEVELS))}"
            )
            raise ValueError(msg)
    return tuple(entries)


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
    workspace_root: str
    config_file: str

    # Tool isolation (KI-71, KI-96: codeforge.tool_process, codeforge.tool_identity)
    tool_isolation: str
    workspace_gid: int
    tool_home_base: str
    tool_path: str
    tool_landlock: str
    tool_landlock_min_abi: int
    tool_read_paths: str
    tool_cache_max_mb: int

    # LLM
    default_model: str
    model_capabilities: tuple[tuple[str, str], ...]
    keyed_providers: frozenset[str]

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

    # Knowledge bases: indexed below this directory only (KI-105)
    knowledge_content_root: str

    # OpenTelemetry
    otel_enabled: bool
    otel_endpoint: str
    otel_service_name: str
    otel_insecure: bool
    otel_sample_rate: float

    # Plan/Act
    plan_act_max_iterations: int

    # Experience pool (same keys and env vars as the Go config)
    experience_enabled: bool
    experience_confidence_threshold: float
    experience_max_entries: int

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

        self.nats_url = _resolve_secret("NATS_URL", nats_cfg.get("url"), "nats://localhost:4222")
        self.litellm_url = _resolve_str("LITELLM_BASE_URL", litellm_cfg.get("url"), "http://localhost:4000")
        # The worker entry point warns about the development key once logging is set up.
        self.litellm_api_key = _resolve_secret(
            "LITELLM_MASTER_KEY", litellm_cfg.get("master_key"), DEV_LITELLM_MASTER_KEY
        )
        self.log_level = _resolve_str("CODEFORGE_WORKER_LOG_LEVEL", logging_cfg.get("level"), "info")
        self.log_service = _resolve_str("CODEFORGE_WORKER_LOG_SERVICE", None, "codeforge-worker")
        self.health_port = _resolve_int("CODEFORGE_WORKER_HEALTH_PORT", None, 8081)

        self.routing_enabled = _resolve_bool("CODEFORGE_ROUTING_ENABLED", routing_cfg.get("enabled"), True)
        self.trust_min_level = _resolve_str("CODEFORGE_TRUST_MIN_LEVEL", trust_cfg.get("min_level"), "untrusted")

        # --- Core / Infrastructure ---
        core_cfg: dict = yaml_cfg.get("core", {}) if isinstance(yaml_cfg.get("core"), dict) else {}
        self.core_url = _resolve_str("CODEFORGE_CORE_URL", core_cfg.get("url"), "http://localhost:8080")
        self.internal_key = _resolve_secret("CODEFORGE_INTERNAL_KEY", core_cfg.get("internal_key"), "")
        self.app_env = _resolve_str("APP_ENV", yaml_cfg.get("app_env"), "")
        self.database_url = _resolve_secret(
            "DATABASE_URL",
            yaml_cfg.get("postgres", {}).get("dsn") if isinstance(yaml_cfg.get("postgres"), dict) else None,
            "postgresql://codeforge:codeforge_dev@localhost:5432/codeforge",
        )
        self.workspace = _resolve_str("CODEFORGE_WORKSPACE", None, "/workspaces/CodeForge")
        # The Go Core's workspace root (same variable); with tool isolation the
        # worker opens workspaces created before it to the workspace group.
        self.workspace_root = _resolve_str("CODEFORGE_WORKSPACE_ROOT", None, "")
        self.config_file = os.environ.get("CODEFORGE_CONFIG_FILE", "")

        # --- Tool isolation (KI-71, KI-96): who agent tool processes run as ---
        # "required" in the worker image and docker-compose.prod.yml, "off" elsewhere.
        # With it every tenant's tool processes run as the tenant's tool UID (the
        # Go Core sends it), with a HOME below the base and the tool PATH.
        self.tool_isolation = _resolve_str("CODEFORGE_TOOL_ISOLATION", None, "off")
        self.workspace_gid = _resolve_int("CODEFORGE_WORKSPACE_GID", None, 10010)
        self.tool_home_base = _resolve_str("CODEFORGE_TOOL_HOME_BASE", None, DEFAULT_HOME_BASE)
        self.tool_path = _resolve_str("CODEFORGE_TOOL_PATH", None, DEFAULT_TOOL_PATH)
        # Landlock per tool call (KI-96 D6): unset follows CODEFORGE_TOOL_ISOLATION;
        # "off" is refused with APP_ENV=production. Read paths: operator
        # toolchains tools may read and run (colon-separated, validated).
        self.tool_landlock = _resolve_str("CODEFORGE_TOOL_LANDLOCK", None, "")
        self.tool_landlock_min_abi = _resolve_int("CODEFORGE_TOOL_LANDLOCK_MIN_ABI", None, 2)
        self.tool_read_paths = _resolve_str("CODEFORGE_TOOL_READ_PATHS", None, "")
        # A tenant's HOME cache (<HOME>/.cache) larger than this is removed when the tenant goes idle.
        self.tool_cache_max_mb = _resolve_int("CODEFORGE_TOOL_CACHE_MAX_MB", None, 4096)

        # --- LLM ---
        self.default_model = _resolve_str("CODEFORGE_DEFAULT_MODEL", litellm_cfg.get("default_model"), "")
        # Tool capability per model, above LiteLLM's metadata and the name patterns (KI-125).
        self.model_capabilities = _resolve_model_capabilities(litellm_cfg.get("model_capabilities"))
        # Providers whose API key LiteLLM holds, as the Go Core reads them (KI-125).
        self.keyed_providers = parse_keyed_providers(
            os.environ.get(KEYED_PROVIDERS_ENV, ""), litellm_cfg.get("keyed_providers")
        )

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

        # --- Experience pool ---
        experience_cfg: dict = yaml_cfg.get("experience", {}) if isinstance(yaml_cfg.get("experience"), dict) else {}
        self.experience_enabled = _resolve_bool("CODEFORGE_EXPERIENCE_ENABLED", experience_cfg.get("enabled"), False)
        self.experience_confidence_threshold = _resolve_float(
            "CODEFORGE_EXPERIENCE_CONFIDENCE_THRESHOLD", experience_cfg.get("confidence_threshold"), 0.85
        )
        self.experience_max_entries = _resolve_int(
            "CODEFORGE_EXPERIENCE_MAX_ENTRIES", experience_cfg.get("max_entries"), 1000
        )

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

        # --- Knowledge bases (same key and env var as the Go config, KI-105) ---
        knowledge_cfg: dict = yaml_cfg.get("knowledge", {}) if isinstance(yaml_cfg.get("knowledge"), dict) else {}
        self.knowledge_content_root = _resolve_str(
            "CODEFORGE_KNOWLEDGE_CONTENT_ROOT", knowledge_cfg.get("content_root"), "data/knowledge"
        )

        # --- OpenTelemetry ---
        otel_cfg: dict = yaml_cfg.get("otel", {}) if isinstance(yaml_cfg.get("otel"), dict) else {}
        self.otel_enabled = _resolve_bool("CODEFORGE_OTEL_ENABLED", otel_cfg.get("enabled"), False)
        self.otel_endpoint = _resolve_str("CODEFORGE_OTEL_ENDPOINT", otel_cfg.get("endpoint"), "localhost:4317")
        self.otel_service_name = _resolve_str(
            "CODEFORGE_OTEL_SERVICE_NAME", otel_cfg.get("service_name"), "codeforge-worker"
        )
        # Same default as the Go core (TLS); the dev Jaeger needs CODEFORGE_OTEL_INSECURE=true.
        self.otel_insecure = _resolve_bool("CODEFORGE_OTEL_INSECURE", otel_cfg.get("insecure"), False)
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
