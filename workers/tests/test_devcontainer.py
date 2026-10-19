"""The dev container starts a working stack (KI-124).

The Go Core and the worker run as processes in the dev container; the dev
compose file runs PostgreSQL, NATS and LiteLLM. They need APP_ENV, one shared
CODEFORGE_INTERNAL_KEY (generated per container, never a fixed value in the
repository) and the same LiteLLM master key and database password as the
compose services.
"""

from __future__ import annotations

import json
import os
import re
import shutil
import stat
import subprocess
from pathlib import Path

import yaml

from codeforge.config import DEV_LITELLM_MASTER_KEY

ROOT = Path(__file__).resolve().parents[2]
DEVCONTAINER = ROOT / ".devcontainer"
REMOTE_ENV: dict[str, str] = json.loads((DEVCONTAINER / "devcontainer.json").read_text())["remoteEnv"]
COMPOSE = yaml.safe_load((ROOT / "docker-compose.yml").read_text())


def _compose_default(service: str, variable: str) -> str:
    value = COMPOSE["services"][service]["environment"][variable]
    match = re.fullmatch(r"\$\{(\w+):-([^}]*)\}", value)
    assert match, f"{service}.{variable} = {value!r} has no default"
    return match.group(2)


def test_core_runs_in_development_mode() -> None:
    assert REMOTE_ENV["APP_ENV"] == "development"


def test_litellm_master_key_matches_the_compose_default() -> None:
    default = _compose_default("litellm", "LITELLM_MASTER_KEY")

    assert default == DEV_LITELLM_MASTER_KEY
    assert REMOTE_ENV["LITELLM_MASTER_KEY"] == "${localEnv:LITELLM_MASTER_KEY:" + default + "}"


def test_database_password_is_the_compose_default_or_the_hosts() -> None:
    default = _compose_default("postgres", "POSTGRES_PASSWORD")
    password = "${localEnv:POSTGRES_PASSWORD:" + default + "}"

    # setup.sh runs docker compose with this value, so PostgreSQL gets the
    # password DATABASE_URL carries.
    assert REMOTE_ENV["POSTGRES_PASSWORD"] == password
    assert REMOTE_ENV["DATABASE_URL"].startswith("postgres://codeforge:" + password + "@codeforge-postgres:5432/")


def test_no_internal_key_in_the_repository() -> None:
    assert "CODEFORGE_INTERNAL_KEY" not in REMOTE_ENV
    assert "CODEFORGE_INTERNAL_KEY=" not in (DEVCONTAINER / "setup.sh").read_text()


def _source_dev_env(home: Path, preset: str | None = None) -> str:
    env = {"HOME": str(home), "PATH": os.environ["PATH"]}
    if preset is not None:
        env["CODEFORGE_INTERNAL_KEY"] = preset
    bash = shutil.which("bash")
    assert bash
    result = subprocess.run(  # noqa: S603 - fixed command on the repository's own script
        [bash, "-c", 'set -u; . "$1"; printf %s "$CODEFORGE_INTERNAL_KEY"', "bash", str(DEVCONTAINER / "dev-env.sh")],
        env=env,
        capture_output=True,
        text=True,
        check=True,
        timeout=30,
    )
    return result.stdout


def test_dev_env_generates_one_shared_internal_key(tmp_path: Path) -> None:
    first = _source_dev_env(tmp_path)
    second = _source_dev_env(tmp_path)

    assert re.fullmatch(r"[0-9a-f]{64}", first)
    assert second == first  # the Core's and the worker's shells get the same key
    key_file = tmp_path / ".config" / "codeforge" / "internal_key"
    assert stat.S_IMODE(key_file.stat().st_mode) == 0o600
    assert stat.S_IMODE(key_file.parent.stat().st_mode) == 0o700


def test_dev_env_key_differs_per_container(tmp_path: Path) -> None:
    assert _source_dev_env(tmp_path / "a") != _source_dev_env(tmp_path / "b")


def test_dev_env_keeps_a_key_set_by_the_user(tmp_path: Path) -> None:
    assert _source_dev_env(tmp_path, preset="mine") == "mine"
    assert not (tmp_path / ".config" / "codeforge" / "internal_key").exists()


def test_setup_and_shells_use_dev_env() -> None:
    setup = (DEVCONTAINER / "setup.sh").read_text()

    assert ". .devcontainer/dev-env.sh" in setup
    assert "/workspaces/CodeForge/.devcontainer/dev-env.sh" in setup  # sourced by ~/.bashrc


def test_devcontainer_files_are_ascii() -> None:
    for path in DEVCONTAINER.iterdir():
        path.read_bytes().decode("ascii")
