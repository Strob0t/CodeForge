"""The production deployment isolates agent tools and authenticates NATS (KI-71).

Checks docker-compose.prod.yml, Dockerfile.worker and the secret scripts:
the worker starts as root with SETUID, SETGID and KILL only, gets its secrets
as file paths in a directory only it may enter, and each service connects to
NATS with its own user. Runs the real generate-secrets.sh / validate-env.sh
(needs bash and openssl) and, when available, nats-server and docker compose.
"""

from __future__ import annotations

import os
import re
import shutil
import subprocess
from pathlib import Path

import pytest
import yaml

REPO = Path(__file__).resolve().parents[2]
COMPOSE = yaml.safe_load((REPO / "docker-compose.prod.yml").read_text())
WORKER = COMPOSE["services"]["worker"]
CORE = COMPOSE["services"]["core"]
NATS = COMPOSE["services"]["nats"]
BASH = shutil.which("bash")
WORKER_SECRETS = ("DATABASE_URL", "NATS_URL", "LITELLM_MASTER_KEY", "CODEFORGE_INTERNAL_KEY")


def _secret_targets(service: dict[str, object]) -> dict[str, str]:
    targets = {}
    for entry in service["secrets"]:  # type: ignore[attr-defined]
        if isinstance(entry, str):
            targets[entry] = entry
        else:
            targets[entry["source"]] = entry.get("target", entry["source"])
    return targets


def test_worker_starts_as_root_with_three_capabilities() -> None:
    assert WORKER["user"] == "0:0"
    assert WORKER["cap_drop"] == ["ALL"]
    assert sorted(WORKER["cap_add"]) == ["KILL", "SETGID", "SETUID"]
    assert "no-new-privileges:true" in WORKER["security_opt"]
    assert WORKER["read_only"] is True
    # The image entrypoint drops to the worker user; an override would skip it.
    assert "entrypoint" not in WORKER


def test_worker_secrets_are_file_paths_in_a_private_directory() -> None:
    env = WORKER["environment"]
    for key in WORKER_SECRETS:
        assert env[f"{key}_FILE"].startswith("/run/secrets/"), key
        assert key not in env, f"{key} would be in the worker's environment"
    assert "/run/secrets:uid=10001,gid=10001,mode=0700" in WORKER["tmpfs"]
    assert env["CODEFORGE_TOOL_ISOLATION"] == "required"
    assert env["CODEFORGE_WORKSPACE_ROOT"] == "/data/workspaces"


def test_each_service_has_its_own_nats_user() -> None:
    assert _secret_targets(WORKER)["nats-worker-url"] == "nats-url"
    assert _secret_targets(CORE)["nats-core-url"] == "nats-url"
    assert "nats-worker-url" not in _secret_targets(CORE)
    assert "nats-core-url" not in _secret_targets(WORKER)
    assert COMPOSE["configs"]["nats-server-conf"]["file"] == "./configs/nats/nats-server.conf"
    config_target = NATS["configs"][0]["target"]
    passwords_target = _secret_targets(NATS)["nats-passwords"]
    assert Path(passwords_target).parent == Path(config_target).parent, "the config includes passwords.conf"
    assert Path(passwords_target).name == "passwords.conf"
    assert NATS["command"][:2] == ["--config", config_target]


def test_worker_image_isolates_tools() -> None:
    dockerfile = (REPO / "Dockerfile.worker").read_text()
    runtime = dockerfile.split("# --- Runtime stage ---", 1)[1]
    assert not re.search(r"^USER ", runtime, re.MULTILINE), "the entrypoint needs root to drop to the worker user"
    assert 'ENTRYPOINT ["/app/scripts/worker-entrypoint.sh"]' in runtime
    assert "CODEFORGE_TOOL_ISOLATION=required" in runtime
    entrypoint = (REPO / "scripts" / "worker-entrypoint.sh").read_text()
    assert "--ambient-caps=-all,+setuid,+setgid,+kill" in entrypoint
    assert os.access(REPO / "scripts" / "worker-entrypoint.sh", os.X_OK)


needs_tools = pytest.mark.skipif(BASH is None or shutil.which("openssl") is None, reason="needs bash and openssl")


def _script(name: str, *args: str, secrets_dir: Path) -> subprocess.CompletedProcess[str]:
    env = {**os.environ, "SECRETS_DIR": str(secrets_dir), "COMPOSE_ENV_FILE": "/nonexistent"}
    return subprocess.run(  # noqa: S603 - the repository's own scripts
        [str(BASH), str(REPO / "scripts" / name), *args], capture_output=True, text=True, env=env, check=False
    )


@needs_tools
def test_generated_nats_secrets(tmp_path: Path) -> None:
    secrets_dir = tmp_path / "secrets"
    result = _script("generate-secrets.sh", str(secrets_dir), secrets_dir=secrets_dir)
    assert result.returncode == 0, result.stderr
    core_pass = (secrets_dir / "nats-core-pass").read_text()
    worker_pass = (secrets_dir / "nats-worker-pass").read_text()
    assert core_pass != worker_pass
    assert (secrets_dir / "nats-core-url").read_text() == f"nats://core:{core_pass}@nats:4222"
    assert (secrets_dir / "nats-worker-url").read_text() == f"nats://worker:{worker_pass}@nats:4222"
    passwords = (secrets_dir / "nats-passwords.conf").read_text()
    assert f'CORE_PASSWORD: "{core_pass}"' in passwords
    assert f'WORKER_PASSWORD: "{worker_pass}"' in passwords
    for name in ("nats-core-url", "nats-worker-url", "nats-passwords.conf"):
        assert (secrets_dir / name).stat().st_mode & 0o777 == 0o644, name

    result = _script("validate-env.sh", secrets_dir=secrets_dir)
    assert result.returncode == 0, result.stderr

    (secrets_dir / "nats-worker-url").write_text(f"nats://worker:{core_pass}@nats:4222")
    result = _script("validate-env.sh", secrets_dir=secrets_dir)
    assert result.returncode == 1
    assert "nats-worker-url and nats-passwords.conf hold different passwords" in result.stderr
    (secrets_dir / "nats-worker-url").write_text(f"nats://core:{worker_pass}@nats:4222")
    result = _script("validate-env.sh", secrets_dir=secrets_dir)
    assert "expected the NATS user 'worker'" in result.stderr


@needs_tools
def test_old_single_user_files_are_reported_not_used(tmp_path: Path) -> None:
    secrets_dir = tmp_path / "secrets"
    secrets_dir.mkdir(mode=0o700)
    (secrets_dir / "nats-url").write_text("nats://codeforge-1:p@nats:4222")
    result = _script("generate-secrets.sh", str(secrets_dir), secrets_dir=secrets_dir)
    assert result.returncode == 1  # an in-use directory without the JWT secret is refused, as before
    for name in ("codeforge-auth-jwt-secret",):
        (secrets_dir / name).write_text("a" * 64)
    (secrets_dir / "litellm-master-key").write_text("sk-" + "b" * 64)
    (secrets_dir / "postgres-password").write_text("c" * 64)
    result = _script("generate-secrets.sh", str(secrets_dir), secrets_dir=secrets_dir)
    assert result.returncode == 0, result.stderr
    assert "nats-url is no longer used" in result.stdout
    assert (secrets_dir / "nats-worker-url").is_file()


NATS_SERVER = os.environ.get("NATS_SERVER_BIN") or shutil.which("nats-server")


@needs_tools
@pytest.mark.skipif(NATS_SERVER is None, reason="needs a nats-server binary (NATS_SERVER_BIN)")
def test_nats_server_accepts_the_config_with_generated_passwords(tmp_path: Path) -> None:
    secrets_dir = tmp_path / "secrets"
    assert _script("generate-secrets.sh", str(secrets_dir), secrets_dir=secrets_dir).returncode == 0
    conf_dir = tmp_path / "nats"
    conf_dir.mkdir()
    shutil.copy(REPO / "configs" / "nats" / "nats-server.conf", conf_dir / "nats-server.conf")
    shutil.copy(secrets_dir / "nats-passwords.conf", conf_dir / "passwords.conf")
    assert NATS_SERVER is not None
    result = subprocess.run(  # noqa: S603 - the configured nats-server
        [NATS_SERVER, "-t", "-c", str(conf_dir / "nats-server.conf")], capture_output=True, text=True, check=False
    )
    assert result.returncode == 0, result.stderr + result.stdout


@needs_tools
@pytest.mark.skipif(shutil.which("docker") is None, reason="needs docker compose")
@pytest.mark.parametrize("overlay", [False, True], ids=["prod", "blue-green"])
def test_compose_config_is_valid(overlay: bool, tmp_path: Path) -> None:
    secrets_dir = tmp_path / "secrets"
    assert _script("generate-secrets.sh", str(secrets_dir), secrets_dir=secrets_dir).returncode == 0
    files = ["-f", "docker-compose.prod.yml"]
    if overlay:
        files += ["-f", "docker-compose.blue-green.yml", "--profile", "blue", "--profile", "green"]
    env = {
        **os.environ,
        "SECRETS_DIR": str(secrets_dir),
        "ACME_EMAIL": "ops@example.com",
        "CODEFORGE_DOMAIN": "x.example",
    }
    result = subprocess.run(  # noqa: S603 - docker compose of this repository
        [str(shutil.which("docker")), "compose", *files, "config", "--format", "json"],
        cwd=REPO,
        capture_output=True,
        text=True,
        env=env,
        check=False,
        timeout=120,
    )
    if result.returncode != 0 and "docker: 'compose' is not a docker command" in result.stderr:
        pytest.skip("docker compose plugin not installed")
    assert result.returncode == 0, result.stderr
    services = yaml.safe_load(result.stdout)["services"]
    cores = ["core-blue", "core-green"] if overlay else ["core"]
    for name in cores:
        targets = {s["source"]: s["target"] for s in services[name]["secrets"]}
        assert targets["nats-core-url"] in ("nats-url", "/run/secrets/nats-url"), name
    worker_targets = {s["source"]: s["target"] for s in services["worker"]["secrets"]}
    assert worker_targets["nats-worker-url"] in ("nats-url", "/run/secrets/nats-url")
    assert sorted(services["worker"]["cap_add"]) == ["KILL", "SETGID", "SETUID"]
