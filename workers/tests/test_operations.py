"""Operations scripts and the production database settings (KI-210, KI-212).

WAL archiving is off unless the operator turns it on (an archive without base
backups and pruning only fills the disk); the archive cleanup runs with the
POSIX sh of the postgres image. Runs the repository's scripts against scratch
directories.
"""

from __future__ import annotations

import os
import shutil
import subprocess
import time
from pathlib import Path

import pytest
import yaml

REPO = Path(__file__).resolve().parents[2]
SCRIPTS = REPO / "scripts"
SH = shutil.which("sh")
DOCKER = shutil.which("docker")
DAY = 86400


def _postgres_settings(command: list[str]) -> dict[str, str]:
    settings = {}
    for i, arg in enumerate(command):
        if arg == "-c":
            key, _, value = command[i + 1].partition("=")
            settings[key] = value
    return settings


def test_wal_archiving_is_off_unless_the_operator_turns_it_on() -> None:
    compose = yaml.safe_load((REPO / "docker-compose.prod.yml").read_text())
    settings = _postgres_settings(compose["services"]["postgres"]["command"])
    assert settings["archive_mode"] == "${POSTGRES_ARCHIVE_MODE:-off}"
    # The archive volume and command stay, for the opt-in.
    assert settings["archive_command"] == "test ! -f /archive/%f && cp %p /archive/%f"


@pytest.mark.skipif(DOCKER is None, reason="needs docker compose")
@pytest.mark.parametrize(("env", "want"), [({}, "off"), ({"POSTGRES_ARCHIVE_MODE": "on"}, "on")])
def test_compose_resolves_the_archive_mode(env: dict[str, str], want: str, tmp_path: Path) -> None:
    secrets = tmp_path / "secrets"
    secrets.mkdir()
    for name in (
        "postgres-password",
        "postgres-tls.crt",
        "postgres-tls.key",
        "database-url",
        "nats-passwords.conf",
        "nats-core-url",
        "nats-worker-url",
        "litellm-master-key",
        "codeforge-auth-jwt-secret",
        "codeforge-auth-llm-key-encryption-secret",
        "codeforge-internal-key",
    ):
        (secrets / name).write_text("x")
    base = {k: v for k, v in os.environ.items() if k != "POSTGRES_ARCHIVE_MODE"}
    result = subprocess.run(  # noqa: S603 - docker compose of this repository
        [str(DOCKER), "compose", "-f", "docker-compose.prod.yml", "config", "--format", "json"],
        cwd=REPO,
        capture_output=True,
        text=True,
        env={**base, "SECRETS_DIR": str(secrets), **env},
        check=False,
        timeout=120,
    )
    if result.returncode != 0 and "'compose' is not a docker command" in result.stderr:
        pytest.skip("docker compose plugin not installed")
    assert result.returncode == 0, result.stderr
    command = yaml.safe_load(result.stdout)["services"]["postgres"]["command"]
    assert _postgres_settings(command)["archive_mode"] == want


def _age(path: Path, days: float) -> None:
    stamp = time.time() - days * DAY
    os.utime(path, (stamp, stamp))


@pytest.mark.skipif(SH is None, reason="needs sh")
def test_wal_cleanup_removes_only_old_archive_files(tmp_path: Path) -> None:
    archive = tmp_path / "archive"
    archive.mkdir()
    files = {
        "000000010000000000000001": 10,
        "000000010000000000000002.00000028.backup": 10,
        "000000010000000000000003": 1,
        "000000010000000000000004.00000028.backup": 1,
        "notes.txt": 10,
    }
    for name, days in files.items():
        (archive / name).write_text(name)
        _age(archive / name, days)

    # sh, not bash: the postgres image (alpine) has no bash.
    result = subprocess.run(  # noqa: S603 - the repository's own script
        [str(SH), str(SCRIPTS / "cleanup-wal-archives.sh"), "7"],
        capture_output=True,
        text=True,
        env={**os.environ, "ARCHIVE_DIR": str(archive)},
        check=False,
    )

    assert result.returncode == 0, result.stderr
    assert sorted(p.name for p in archive.iterdir()) == [
        "000000010000000000000003",
        "000000010000000000000004.00000028.backup",
        "notes.txt",
    ]
    assert "Cleaned up 2 WAL archive files" in result.stdout


@pytest.mark.skipif(SH is None, reason="needs sh")
@pytest.mark.parametrize("days", ["7d", "-1", "1.5"])
def test_wal_cleanup_refuses_an_invalid_retention(days: str, tmp_path: Path) -> None:
    (tmp_path / "000000010000000000000001").write_text("x")
    _age(tmp_path / "000000010000000000000001", 30)
    result = subprocess.run(  # noqa: S603 - the repository's own script
        [str(SH), str(SCRIPTS / "cleanup-wal-archives.sh"), days],
        capture_output=True,
        text=True,
        env={**os.environ, "ARCHIVE_DIR": str(tmp_path)},
        check=False,
    )
    assert result.returncode == 2, result.stdout
    assert (tmp_path / "000000010000000000000001").exists()
