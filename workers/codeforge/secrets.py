"""Secret provider with Docker Secrets file fallback to env vars.

In production, secrets are mounted as files under /run/secrets/ by Docker.
In development, they are read from environment variables.

Usage::

    from codeforge.secrets import get_secret

    litellm_key = get_secret("LITELLM_MASTER_KEY")
    database_url = get_secret("DATABASE_URL")
"""

from __future__ import annotations

import logging
import os
from pathlib import Path

logger = logging.getLogger(__name__)

SECRETS_DIR = Path(os.getenv("DOCKER_SECRETS_DIR", "/run/secrets"))


def get_secret(key: str, default: str = "") -> str:
    """Read secret from Docker Secrets file, fall back to env var.

    File lookup: converts KEY_NAME to key-name (lowercase, hyphens).
    """
    file_path = SECRETS_DIR / key.lower().replace("_", "-")
    if file_path.is_file():
        value = file_path.read_text().strip()
        logger.debug("loaded secret from file: %s", key)
        return value
    value = os.getenv(key, default)
    if value:
        logger.debug("loaded secret from env: %s", key)
    return value
