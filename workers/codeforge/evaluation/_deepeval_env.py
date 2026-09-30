"""Keep deepeval from contacting third parties (KI-54).

deepeval, which backs the LLM-judge metrics, sends usage telemetry to PostHog
and Sentry and looks up the host's public IP address when it is imported,
checks PyPI for updates when asked to, and uploads metric results (with the
evaluated inputs and outputs) and traces to Confident AI when it finds a
Confident API key: CONFIDENT_API_KEY, the generic API_KEY, a .env file or
.deepeval/.deepeval in the working directory. CodeForge uses none of this and
its privacy notice does not cover it, so the opt-outs are forced rather than
defaulted.
"""

from __future__ import annotations

import os
from types import MappingProxyType
from typing import TYPE_CHECKING

if TYPE_CHECKING:
    from collections.abc import MutableMapping

# Settings read by deepeval 3.x; each value switches one kind of traffic off.
DEEPEVAL_PRIVACY_ENV = MappingProxyType(
    {
        "DEEPEVAL_TELEMETRY_OPT_OUT": "YES",  # PostHog, Sentry, public-IP lookup
        "DEEPEVAL_UPDATE_WARNING_OPT_IN": "0",  # PyPI version check
        "CONFIDENT_METRIC_LOGGING_ENABLED": "NO",  # metric uploads to Confident AI
        "CONFIDENT_TRACING_ENABLED": "NO",  # trace uploads to Confident AI
        "DEEPEVAL_DISABLE_DOTENV": "1",  # no keys merged from .env files in the cwd
        "DEEPEVAL_DISABLE_LEGACY_KEYFILE": "1",  # no Confident key from .deepeval/.deepeval
    }
)


def disable_deepeval_phone_home(environ: MutableMapping[str, str] | None = None) -> None:
    """Set the opt-outs. deepeval reads them when it is imported, so call this first."""
    (os.environ if environ is None else environ).update(DEEPEVAL_PRIVACY_ENV)
