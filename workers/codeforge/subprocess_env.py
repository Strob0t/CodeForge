"""Environment for subprocesses that run agent-controlled commands.

Agent tools (bash, grep, quality gates, git in the workspace, benchmark test
commands, external agent CLIs) run commands an LLM chose or code it wrote. The
worker's own environment holds CODEFORGE_INTERNAL_KEY (admin on the core API)
and the database, NATS and LiteLLM credentials, so these subprocesses get an
allowlisted environment instead of inheriting os.environ.

With tool isolation (codeforge.tool_process, KI-71) these commands also run
as the tool user, whose HOME, USER and LOGNAME they get, and cannot read the
worker's secret files or its process environment.
"""

from __future__ import annotations

import logging
import os
from typing import TYPE_CHECKING

from codeforge.tool_process import tool_identity_env

if TYPE_CHECKING:
    from collections.abc import Iterable, Mapping

logger = logging.getLogger(__name__)

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

# The worker's own credentials and endpoints (and the files holding them).
# Explicit passthrough never copies them; only a caller's literal ``extra``
# values can set such a name.
_WORKER_NAMES = frozenset({"DATABASE_URL", "NATS_URL", "DATABASE_URL_FILE", "NATS_URL_FILE"})
_WORKER_PREFIXES = ("CODEFORGE_", "LITELLM_")


def _looks_secret(name: str) -> bool:
    upper = name.upper()
    return any(word in upper for word in _SECRET_WORDS)


def _is_worker_credential(name: str) -> bool:
    return name in _WORKER_NAMES or name.startswith(_WORKER_PREFIXES)


# Variables that make the dynamic loader or an interpreter run other code;
# never taken from a declared environment.
_CODE_LOADING_PREFIXES = ("LD_", "PYTHON", "PERL5", "RUBY", "MALLOC_")
_CODE_LOADING_NAMES = frozenset(
    {
        "NODE_OPTIONS",
        "NODE_PATH",
        "BASH_ENV",
        "ENV",
        "GCONV_PATH",
        "GLIBC_TUNABLES",
        "LOCPATH",
        "NLSPATH",
        "HOSTALIASES",
    }
)


def declared_tool_env(declared: Mapping[str, str] | None) -> dict[str, str]:
    """The variables a tool process's definition declares (an MCP server's env), safe to pass on.

    Drops the worker's own credentials and variables that make the dynamic
    loader or an interpreter load other code (LD_*, PYTHON*, NODE_OPTIONS,
    ...). Credentials the definition declares for the tool itself (an API
    token of an MCP server) are kept.
    """
    if not declared:
        return {}
    kept: dict[str, str] = {}
    for name, value in declared.items():
        upper = name.upper()
        if _is_worker_credential(upper) or upper.startswith(_CODE_LOADING_PREFIXES) or upper in _CODE_LOADING_NAMES:
            logger.warning("dropping the declared environment variable %s of a tool process", name)
            continue
        kept[name] = value
    return kept


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
    (CODEFORGE_*, LITELLM_*, DATABASE_URL, NATS_URL). With tool isolation
    HOME, USER and LOGNAME are the tool user's. ``extra`` adds explicit
    values, such as a backend's configured extra_env.
    """
    env: dict[str, str] = {}
    wanted = set(passthrough)
    for name, value in os.environ.items():
        allowed = (name in _ALLOWED_NAMES or name.startswith(_ALLOWED_PREFIXES)) and not _looks_secret(name)
        requested = name in wanted or (bool(passthrough_prefixes) and name.startswith(passthrough_prefixes))
        if allowed or (requested and not _is_worker_credential(name)):
            env[name] = value
    env.update(tool_identity_env())
    if extra:
        env.update(extra)
    return env
