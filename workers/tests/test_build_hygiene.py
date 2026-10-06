"""Build, CI and deployment hygiene (KI-214).

What reaches the images (.dockerignore), what CI runs and pins, and the
compose settings around shutdown and the dev browser container.
"""

from __future__ import annotations

import shutil
import subprocess
from pathlib import Path

import pytest

REPO = Path(__file__).resolve().parents[2]
DOCKER = shutil.which("docker")


@pytest.mark.skipif(DOCKER is None, reason="needs docker (BuildKit)")
def test_dockerignore_applies_in_every_directory(tmp_path: Path) -> None:
    """Root-only patterns let a host's frontend/node_modules overlay the npm ci result, and
    workers/.env and workers/tests reach the worker image."""
    ctx = tmp_path / "ctx"
    files = {
        "frontend/node_modules/seroval/index.js": False,
        "frontend/.env.local": False,
        "frontend/src/main.ts": True,
        "workers/.env": False,
        "workers/codeforge/__pycache__/x.cpython-312.pyc": False,
        "workers/codeforge/agent_loop.py": True,
        "workers/tests/test_x.py": False,
        "node_modules/x.js": False,
        ".env": False,
        ".env.example": True,
        "scripts/worker-healthcheck.py": True,
        "scripts/backup-postgres.sh": False,
    }
    for name in files:
        path = ctx / name
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text("x")
    shutil.copy(REPO / ".dockerignore", ctx / ".dockerignore")
    (ctx / "Dockerfile").write_text("FROM scratch\nCOPY . /ctx\n")
    out = tmp_path / "out"
    result = subprocess.run(  # noqa: S603 - a local build of a scratch context
        [str(DOCKER), "build", "-q", "--output", f"type=local,dest={out}", str(ctx)],
        capture_output=True,
        text=True,
        check=False,
        timeout=180,
    )
    if result.returncode != 0 and "unknown flag: --output" in result.stderr:
        pytest.skip("docker build without BuildKit")
    assert result.returncode == 0, result.stderr
    for name, kept in files.items():
        assert (out / "ctx" / name).exists() == kept, name
