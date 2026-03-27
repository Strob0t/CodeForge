"""Worker healthcheck -- verifies the consumer is connected and processing."""

import sys
from pathlib import Path


def main() -> int:
    """Return 0 if the worker sentinel file exists, 1 otherwise.

    The worker creates a /tmp/codeforge-worker-healthy sentinel file
    when the NATS connection is active and the message loops are running.
    """
    sentinel = Path("/tmp/codeforge-worker-healthy")  # noqa: S108
    if sentinel.exists():
        return 0
    return 1


if __name__ == "__main__":
    try:
        sys.exit(main())
    except Exception:
        sys.exit(1)
