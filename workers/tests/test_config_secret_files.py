"""The worker reads its secrets from <KEY>_FILE like the Go Core (KI-71).

docker-compose.prod.yml hands the worker file paths only, so no secret is in
its process environment; the files are read once, because the worker locks
its secrets directory after startup.
"""

from __future__ import annotations

from typing import TYPE_CHECKING

import pytest

from codeforge import config
from codeforge.config import WorkerSettings

if TYPE_CHECKING:
    from pathlib import Path

SECRETS = {
    "NATS_URL": "nats_url",
    "DATABASE_URL": "database_url",
    "LITELLM_MASTER_KEY": "litellm_api_key",
    "CODEFORGE_INTERNAL_KEY": "internal_key",
}


@pytest.fixture(autouse=True)
def _no_cached_files(monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.setattr(config, "_secret_file_values", {})
    for key in SECRETS:
        monkeypatch.delenv(key, raising=False)
        monkeypatch.delenv(key + "_FILE", raising=False)


@pytest.mark.parametrize(("key", "attr"), sorted(SECRETS.items()))
def test_secret_from_file(key: str, attr: str, monkeypatch: pytest.MonkeyPatch, tmp_path: Path) -> None:
    secret = tmp_path / key.lower()
    secret.write_text("  s3cret://user:pass@host/x \r\n")
    monkeypatch.setenv(key + "_FILE", str(secret))
    assert getattr(WorkerSettings(), attr) == "s3cret://user:pass@host/x"


@pytest.mark.parametrize("key", sorted(SECRETS))
def test_secret_set_twice_is_rejected(key: str, monkeypatch: pytest.MonkeyPatch, tmp_path: Path) -> None:
    secret = tmp_path / "s"
    secret.write_text("a")
    monkeypatch.setenv(key, "b")
    monkeypatch.setenv(key + "_FILE", str(secret))
    with pytest.raises(ValueError, match=f"both {key} and {key}_FILE are set"):
        WorkerSettings()


@pytest.mark.parametrize("content", ["", " \n\t\n"])
def test_empty_secret_file_is_rejected(content: str, monkeypatch: pytest.MonkeyPatch, tmp_path: Path) -> None:
    secret = tmp_path / "s"
    secret.write_text(content)
    monkeypatch.setenv("NATS_URL_FILE", str(secret))
    with pytest.raises(ValueError, match=r"NATS_URL_FILE: secret file .* is empty"):
        WorkerSettings()


def test_missing_secret_file_is_rejected(monkeypatch: pytest.MonkeyPatch, tmp_path: Path) -> None:
    monkeypatch.setenv("DATABASE_URL_FILE", str(tmp_path / "missing"))
    with pytest.raises(ValueError, match="DATABASE_URL_FILE"):
        WorkerSettings()


def test_secret_file_is_read_once(monkeypatch: pytest.MonkeyPatch, tmp_path: Path) -> None:
    """Settings built after the secrets directory was locked keep the value."""
    secret = tmp_path / "s"
    secret.write_text("nats://worker:p@nats:4222")
    monkeypatch.setenv("NATS_URL_FILE", str(secret))
    assert WorkerSettings().nats_url == "nats://worker:p@nats:4222"
    secret.unlink()
    assert WorkerSettings().nats_url == "nats://worker:p@nats:4222"


def test_without_file_the_environment_and_defaults_apply(monkeypatch: pytest.MonkeyPatch) -> None:
    assert WorkerSettings().nats_url == "nats://localhost:4222"
    monkeypatch.setenv("NATS_URL", "nats://n:4222")
    assert WorkerSettings().nats_url == "nats://n:4222"
