"""Environment for subprocesses that run agent-controlled commands.

Agent tools (bash, grep, quality gates, git in the workspace, benchmark test
commands, external agent CLIs) run commands an LLM chose or code it wrote. The
worker's own environment holds CODEFORGE_INTERNAL_KEY (admin on the core API)
and the database, NATS and LiteLLM credentials, so these subprocesses get an
allowlisted environment instead of inheriting os.environ.

This only removes the credentials from the child's environment. Commands still
run as the worker user and can read what that user can read; isolating them
needs the sandbox execution mode.
"""

from __future__ import annotations

import os
from typing import TYPE_CHECKING

if TYPE_CHECKING:
    from collections.abc import Iterable, Mapping

# Process basics, plus what toolchains need to find themselves, reach package
# indexes and the network (proxies) and verify TLS. Proxy and index URLs are
# passed as they are: credentials embedded in them (http://user:pass@proxy,
# PIP_INDEX_URL with a token) reach agent commands too, so configure those via
# netrc/keyring instead.
_ALLOWED_NAMES = frozenset(
    {
        "PATH",
        "HOME",
        "USER",
        "LOGNAME",
        "SHELL",
        "LANG",
        "LANGUAGE",
        "TERM",
        "TZ",
        "TMPDIR",
        "TEMP",
        "TMP",
        "CI",
        "NO_COLOR",
        "FORCE_COLOR",
        "XDG_CACHE_HOME",
        "XDG_CONFIG_HOME",
        "HTTP_PROXY",
        "HTTPS_PROXY",
        "NO_PROXY",
        "http_proxy",
        "https_proxy",
        "no_proxy",
        "SSL_CERT_FILE",
        "SSL_CERT_DIR",
        "REQUESTS_CA_BUNDLE",
        "CURL_CA_BUNDLE",
        "NODE_EXTRA_CA_CERTS",
        "GIT_SSL_CAINFO",
        "GOPATH",
        "GOROOT",
        "GOCACHE",
        "GOMODCACHE",
        "GOPROXY",
        "GOFLAGS",
        "GOPRIVATE",
        "GONOSUMDB",
        "GONOSUMCHECK",
        "GOINSECURE",
        "GOTOOLCHAIN",
        "CARGO_HOME",
        "RUSTUP_HOME",
        "JAVA_HOME",
        "VIRTUAL_ENV",
        "NODE_OPTIONS",
        "PIP_INDEX_URL",
        "PIP_EXTRA_INDEX_URL",
    }
)
_ALLOWED_PREFIXES = ("LC_", "npm_config_")

# Even an allowed name or prefix is dropped when the name looks like a secret
# (npm_config__authToken, npm_config_//registry/:_auth, ...).
_SECRET_WORDS = ("KEY", "TOKEN", "SECRET", "PASSWORD", "PASSWD", "AUTH", "CREDENTIAL")

# The worker's own credentials and endpoints. Explicit passthrough never copies
# them; only a caller's literal ``extra`` values can set such a name.
_WORKER_NAMES = frozenset({"DATABASE_URL", "NATS_URL"})
_WORKER_PREFIXES = ("CODEFORGE_", "LITELLM_")


def _looks_secret(name: str) -> bool:
    upper = name.upper()
    return any(word in upper for word in _SECRET_WORDS)


def _is_worker_credential(name: str) -> bool:
    return name in _WORKER_NAMES or name.startswith(_WORKER_PREFIXES)


def tool_env(
    *,
    passthrough: Iterable[str] = (),
    passthrough_prefixes: tuple[str, ...] = (),
    extra: Mapping[str, str] | None = None,
) -> dict[str, str]:
    """Return a fresh environment for an agent-controlled subprocess.

    Only allowlisted, non-secret-looking variables are copied from the worker
    environment. ``passthrough`` and ``passthrough_prefixes`` name variables a
    specific caller needs, such as an agent CLI's own provider API keys; they
    are copied when present, except the worker's own credentials
    (CODEFORGE_*, LITELLM_*, DATABASE_URL, NATS_URL). ``extra`` adds explicit
    values, such as a backend's configured extra_env.
    """
    env: dict[str, str] = {}
    wanted = set(passthrough)
    for name, value in os.environ.items():
        allowed = (name in _ALLOWED_NAMES or name.startswith(_ALLOWED_PREFIXES)) and not _looks_secret(name)
        requested = name in wanted or (bool(passthrough_prefixes) and name.startswith(passthrough_prefixes))
        if allowed or (requested and not _is_worker_credential(name)):
            env[name] = value
    if extra:
        env.update(extra)
    return env
