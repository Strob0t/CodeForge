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

# Process basics, plus what toolchains need to find themselves, reach the
# network through a proxy and verify TLS.
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
        "CARGO_HOME",
        "RUSTUP_HOME",
        "JAVA_HOME",
        "VIRTUAL_ENV",
    }
)
_ALLOWED_PREFIXES = ("LC_",)


def tool_env(
    *,
    passthrough: Iterable[str] = (),
    extra: Mapping[str, str] | None = None,
) -> dict[str, str]:
    """Return a fresh environment for an agent-controlled subprocess.

    Only allowlisted variables are copied from the worker environment.
    ``passthrough`` names variables a specific caller needs (for example the
    Claude Code CLI's own ANTHROPIC_API_KEY) and copies them when present;
    ``extra`` adds explicit values, such as a backend's configured extra_env.
    """
    env = {
        name: value
        for name, value in os.environ.items()
        if name in _ALLOWED_NAMES or name.startswith(_ALLOWED_PREFIXES)
    }
    for name in passthrough:
        value = os.environ.get(name)
        if value is not None:
            env[name] = value
    if extra:
        env.update(extra)
    return env
