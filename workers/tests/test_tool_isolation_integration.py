"""Tool processes really run as the tool user (KI-71), with real setpriv processes.

Needs root (to start processes as other users and to set up a secret owned by
the worker user); skipped otherwise. scripts/check-tool-isolation.sh runs the
same checks in a container with the production settings (worker uid 10001
with ambient capabilities, a tmpfs secrets directory).
"""

from __future__ import annotations

import os
import shutil
import tempfile
from pathlib import Path

import pytest

from tests.tool_isolation_check import run_checks

WORKER_UID = 10001
WORKSPACE_GID = 10010

pytestmark = pytest.mark.skipif(
    not hasattr(os, "geteuid") or os.geteuid() != 0 or shutil.which("setpriv") is None,
    reason="needs root and setpriv (util-linux) to start processes as the tool user",
)


@pytest.fixture
def shared_tmp() -> Path:
    # pytest's tmp_path is private to root; the tool user must reach these.
    base = Path(tempfile.mkdtemp(prefix="cf-ki71-", dir="/tmp"))
    base.chmod(0o755)
    yield base
    shutil.rmtree(base, ignore_errors=True)


async def test_tool_process_runs_isolated(shared_tmp: Path) -> None:
    workspace = shared_tmp / "workspace"
    workspace.mkdir()
    os.chown(workspace, WORKER_UID, WORKSPACE_GID)
    workspace.chmod(0o2775)

    secret = shared_tmp / "secret"
    secret.write_text("internal-admin-key")
    os.chown(secret, WORKER_UID, WORKER_UID)
    secret.chmod(0o400)

    report, problems = await run_checks(str(workspace), [str(secret)])

    assert problems == [], report
    assert report["tool"]["CapEff"] == "0000000000000000"  # type: ignore[index]
    assert report["tool"]["NoNewPrivs"] == "1"  # type: ignore[index]
    assert "Permission denied" in str(report[f"read {secret}"])
    assert "Permission denied" in str(report[f"read /proc/{os.getpid()}/environ"])
