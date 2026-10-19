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
import tempfile
import time
from pathlib import Path
from typing import TYPE_CHECKING

import pytest
import yaml

if TYPE_CHECKING:
    from collections.abc import Iterator

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


# --- backup-postgres.sh / restore-postgres.sh (KI-212) -------------------------
#
# The PostgreSQL client tools are stubs on PATH that log their calls; a "dump"
# is a file starting with pg_dump's custom-format magic (PGDMP), which the
# pg_restore stub's --list checks like the real one. A dump containing
# TRUNCATED has a readable table of contents but data pg_restore cannot read
# to the end. gpg is the real one.

BASH = shutil.which("bash")
GPG = shutil.which("gpg")
needs_backup_tools = pytest.mark.skipif(BASH is None or GPG is None, reason="needs bash and gpg")

_STUBS = {
    "pg_dump": 'echo "pg_dump $*" >> "$OPS_LOG"\n'
    'for a in "$@"; do case $a in --file=*) printf "PGDMP %s\\n" "$OPS_DUMP" > "${a#--file=}";; esac; done\n',
    "pg_restore": 'echo "pg_restore $*" >> "$OPS_LOG"\nfile="${!#}"\n'
    'if [ "$(head -c 5 "$file")" != PGDMP ]; then\n'
    '  echo "pg_restore: error: input file does not appear to be a valid archive" >&2; exit 1\nfi\n'
    'if [ "$1" = --list ]; then exit 0; fi\n'
    'if grep -q TRUNCATED "$file"; then\n'
    '  echo "pg_restore: error: could not read from input file: end of file" >&2; exit 1\nfi\n'
    "case $1 in --file=*) exit 0;; esac\n"
    'cp "$file" "$OPS_RESTORED"\n',
    "psql": 'echo "psql $*" >> "$OPS_LOG"\ncat > /dev/null\necho "0|"\n',
    "dropdb": 'echo "dropdb $*" >> "$OPS_LOG"\n',
    "createdb": 'echo "createdb $*" >> "$OPS_LOG"\n',
}


class _Ops:
    def __init__(self, root: Path) -> None:
        self.root = root
        self.backups = root / "backups"
        self.backups.mkdir()
        self.log = root / "calls.log"
        self.log.touch()
        self.restored = root / "restored"
        self.tmp = root / "tmp"
        self.tmp.mkdir()
        self.key = root / "backup.key"
        self.key.write_text("correct horse battery staple\n")
        bin_dir = root / "bin"
        bin_dir.mkdir()
        for name, body in _STUBS.items():
            stub = bin_dir / name
            stub.write_text("#!/bin/bash\n" + body)
            stub.chmod(0o755)
        self.env = {
            **os.environ,
            "PATH": f"{bin_dir}:{os.environ['PATH']}",
            "BACKUP_DIR": str(self.backups),
            "OPS_LOG": str(self.log),
            "OPS_RESTORED": str(self.restored),
            "OPS_DUMP": "fresh",
            "TMPDIR": str(self.tmp),
            "PGDATABASE": "codeforge",
            "GNUPGHOME": str(root / "gnupg"),
        }
        (root / "gnupg").mkdir(mode=0o700)
        self.env.pop("BACKUP_ENCRYPTION_KEY_FILE", None)

    def run(
        self, script: str, *args: str, stdin: str = "", cwd: Path | None = None, **env: str
    ) -> subprocess.CompletedProcess[str]:
        return subprocess.run(  # noqa: S603 - the repository's own scripts
            [str(BASH), str(SCRIPTS / script), *args],
            input=stdin,
            capture_output=True,
            text=True,
            env={**self.env, **env},
            cwd=cwd or self.root,
            check=False,
            timeout=60,
        )

    def calls(self) -> str:
        return self.log.read_text()

    def dump(self, name: str, content: str, days: float = 0, encrypt: bool = False) -> Path:
        path = self.backups / name
        if encrypt:
            plain = self.root / "plain.tmp"
            plain.write_text(f"PGDMP {content}\n")
            subprocess.run(  # noqa: S603 - gpg as the backup script runs it
                [
                    str(GPG),
                    *("--symmetric", "--batch", "--yes", "--passphrase-file", str(self.key)),
                    *("--output", str(path), str(plain)),
                ],
                env=self.env,
                check=True,
                capture_output=True,
            )
            plain.unlink()
        else:
            path.write_text(f"PGDMP {content}\n")
        _age(path, days)
        return path


@pytest.fixture
def ops() -> Iterator[_Ops]:
    # A short path: gpg-agent's socket lives in GNUPGHOME (sun_path limit).
    root = Path(tempfile.mkdtemp(prefix="cf-ops-"))
    try:
        yield _Ops(root)
    finally:
        subprocess.run(  # noqa: S603 - stop the scratch keyring's agent
            [shutil.which("gpgconf") or "gpgconf", "--kill", "gpg-agent"],
            env={**os.environ, "GNUPGHOME": str(root / "gnupg")},
            check=False,
            capture_output=True,
        )
        shutil.rmtree(root, ignore_errors=True)


@needs_backup_tools
def test_restore_latest_without_backups_drops_nothing(ops: _Ops) -> None:
    """Without -r, xargs ran ls in the current directory and "restored" a file found there."""
    (ops.root / "VERSION").write_text("0.8.0\n")
    result = ops.run("restore-postgres.sh", "latest", stdin="y\n")
    assert result.returncode == 1
    assert "No backups found" in result.stdout + result.stderr
    assert "dropdb" not in ops.calls()


@needs_backup_tools
def test_restore_latest_decrypts_the_newest_backup(ops: _Ops) -> None:
    ops.dump("codeforge_20260101_000000.sql.gz", "older", days=2)
    ops.dump("codeforge_20260102_000000.sql.gz.gpg", "newest", days=1, encrypt=True)
    result = ops.run("restore-postgres.sh", "latest", stdin="y\n", BACKUP_ENCRYPTION_KEY_FILE=str(ops.key))
    assert result.returncode == 0, result.stdout + result.stderr
    assert ops.restored.read_text() == "PGDMP newest\n"
    calls = ops.calls()
    assert calls.index("pg_restore --file=/dev/null") < calls.index("dropdb"), calls
    assert not list(ops.tmp.iterdir()), "the decrypted dump is removed"


@needs_backup_tools
@pytest.mark.parametrize(
    ("setup", "message"),
    [
        ("not-a-dump", "not a pg_dump archive"),
        ("truncated", "cannot read all of"),
        ("no-key", "BACKUP_ENCRYPTION_KEY_FILE"),
        ("wrong-key", "cannot decrypt"),
    ],
)
def test_restore_refuses_before_dropping(ops: _Ops, setup: str, message: str) -> None:
    env: dict[str, str] = {}
    if setup == "not-a-dump":
        target = ops.backups / "codeforge_20260101_000000.sql.gz"
        target.write_text("-- plain SQL, not a custom-format dump\n")
    elif setup == "truncated":
        # The table of contents reads; the data ends early (KI-212).
        target = ops.dump("codeforge_20260101_000000.sql.gz", "TRUNCATED")
    else:
        target = ops.dump("codeforge_20260101_000000.sql.gz.gpg", "data", encrypt=True)
        if setup == "wrong-key":
            other = ops.root / "other.key"
            other.write_text("not the key\n")
            env["BACKUP_ENCRYPTION_KEY_FILE"] = str(other)
    result = ops.run("restore-postgres.sh", str(target), stdin="y\n", **env)
    assert result.returncode == 1, result.stdout
    assert message in result.stdout + result.stderr
    assert "dropdb" not in ops.calls()
    assert not ops.restored.exists()
    assert not list(ops.tmp.iterdir())


@needs_backup_tools
def test_backup_retention_prunes_encrypted_backups(ops: _Ops) -> None:
    ops.dump("codeforge_20260101_000000.sql.gz", "old", days=10)
    ops.dump("codeforge_20260102_000000.sql.gz.gpg", "old", days=10, encrypt=True)
    ops.dump("codeforge_20260110_000000.sql.gz.gpg", "recent", days=1, encrypt=True)
    notes = ops.backups / "notes.txt"
    notes.write_text("keep me")
    _age(notes, 10)

    result = ops.run("backup-postgres.sh", "--cleanup", BACKUP_ENCRYPTION_KEY_FILE=str(ops.key), BACKUP_RETAIN_DAYS="7")

    assert result.returncode == 0, result.stdout + result.stderr
    names = sorted(p.name for p in ops.backups.iterdir())
    assert "codeforge_20260101_000000.sql.gz" not in names
    assert "codeforge_20260102_000000.sql.gz.gpg" not in names
    assert "codeforge_20260110_000000.sql.gz.gpg" in names
    assert "notes.txt" in names
    new = [n for n in names if n.endswith(".gpg") and n != "codeforge_20260110_000000.sql.gz.gpg"]
    assert len(new) == 1, names
    assert not any(n.endswith(".sql.gz") for n in names), "the plain dump is removed after encryption"
    assert "removed 2 backups" in result.stdout

    # The new encrypted backup restores.
    restore = ops.run("restore-postgres.sh", "latest", stdin="y\n", BACKUP_ENCRYPTION_KEY_FILE=str(ops.key))
    assert restore.returncode == 0, restore.stdout + restore.stderr
    assert ops.restored.read_text() == "PGDMP fresh\n"
