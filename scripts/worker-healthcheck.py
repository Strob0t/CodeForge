"""Worker container healthcheck: exit 0 when GET /health/ready answers 200.

The worker serves its health endpoints on CODEFORGE_WORKER_HEALTH_PORT
(default 8081); /health/ready is 200 while it is connected to NATS and
every consumer loop runs (codeforge/health.py). Standard library only.
"""

import os
import sys
import urllib.request

TIMEOUT_SECONDS = 3


def main() -> int:
    port = os.environ.get("CODEFORGE_WORKER_HEALTH_PORT") or "8081"
    url = f"http://127.0.0.1:{port}/health/ready"
    with urllib.request.urlopen(url, timeout=TIMEOUT_SECONDS) as response:  # noqa: S310 - fixed local http URL
        return 0 if response.status == 200 else 1


if __name__ == "__main__":
    try:
        sys.exit(main())
    except Exception as exc:  # not ready (503), refused, timed out, bad port
        # docker inspect shows this output with the health status.
        sys.stderr.write(f"worker not ready: {exc}\n")
        sys.exit(1)
