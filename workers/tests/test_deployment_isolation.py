"""The production deployment isolates agent tools and authenticates NATS (KI-71).

Checks docker-compose.prod.yml, Dockerfile.worker and the secret scripts:
the worker starts as root with SETUID, SETGID and KILL only, gets its secrets
as file paths in a directory only it may enter, and each service connects to
NATS with its own user. Runs the real generate-secrets.sh / validate-env.sh
(needs bash and openssl) and, when available, nats-server and docker compose.
"""

from __future__ import annotations

import ipaddress
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


def _runtime_stage() -> str:
    return (REPO / "Dockerfile.worker").read_text().split("# --- Runtime stage ---", 1)[1]


def test_worker_image_has_per_tenant_tool_users() -> None:
    """KI-96: passwd entries for 19999 and 20000-29999 written directly (useradd would hand out
    65,536 subordinate IDs per user and run out), the retired shared tool user outside the
    workspace group, and no setting of the KI-71 single tool user."""
    runtime = _runtime_stage()
    assert "seq 20000 29999" in runtime
    assert "/etc/passwd" in runtime
    assert "/etc/group" in runtime
    assert "codeforge-system:x:19999:19999:" in runtime
    assert not re.search(r"useradd[^\n]*-u (19999|2\d{4})", runtime)
    assert "codeforge-tool-legacy" in runtime
    assert not re.search(r"useradd[^&]*-u 10002[^&]*-G codeforge-ws", runtime), "10002 left the workspace group"
    for name in ("CODEFORGE_TOOL_UID", "CODEFORGE_TOOL_GID", "CODEFORGE_TOOL_HOME="):
        assert name not in runtime, name


def test_worker_image_layout_for_landlock_and_acls() -> None:
    runtime = _runtime_stage()
    assert re.search(r"apt-get install[^&]*\bacl\b", runtime), "the acl package (setfacl for operators)"
    assert "python -m compileall -q /usr/local/lib/python3.12" in runtime, "the launch helper imports fast"
    assert "chmod 2771 /data/workspaces" in runtime
    assert "install -d -o 10001 -g 10001 -m 0711 /home/codeforge-tools" in runtime
    assert "/var/lib/codeforge/landlock-canary" in runtime
    assert "user.name 'CodeForge agent'" in runtime
    assert "user.email agent@codeforge.invalid" in runtime
    assert "safe.directory '*'" in runtime


def test_core_image_sets_tool_acls() -> None:
    dockerfile = (REPO / "Dockerfile").read_text()
    runtime = dockerfile.split("# --- Runtime stage ---", 1)[1]
    assert "CODEFORGE_WORKSPACE_TOOL_ACLS=required" in runtime
    assert "chmod 2771 /data/workspaces" in runtime


def test_compose_gives_tools_a_home_volume_and_a_closed_tmp() -> None:
    """KI-96 D7: HOMEs on a disk volume (a tmpfs is noexec and counts against the worker's
    memory); /tmp writable only by the worker; the Core sets tool ACLs; no new capability."""
    assert "tool_homes:/home/codeforge-tools" in WORKER["volumes"]
    assert "tool_homes" in COMPOSE["volumes"]
    assert "/tmp:uid=10001,gid=10010,mode=1771" in WORKER["tmpfs"]
    assert not any(entry.startswith("/home/codeforge-tool") for entry in WORKER["tmpfs"])
    assert "/tmp" not in WORKER["tmpfs"]
    assert CORE["environment"]["CODEFORGE_WORKSPACE_TOOL_ACLS"] == "required"
    assert sorted(WORKER["cap_add"]) == ["KILL", "SETGID", "SETUID"]
    for name in ("CODEFORGE_TOOL_UID", "CODEFORGE_TOOL_GID", "CODEFORGE_TOOL_HOME", "CODEFORGE_TOOL_LANDLOCK"):
        assert name not in WORKER["environment"], name


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
        assert services[name]["environment"]["CODEFORGE_WORKSPACE_TOOL_ACLS"] == "required", name
    worker_targets = {s["source"]: s["target"] for s in services["worker"]["secrets"]}
    assert worker_targets["nats-worker-url"] in ("nats-url", "/run/secrets/nats-url")
    assert sorted(services["worker"]["cap_add"]) == ["KILL", "SETGID", "SETUID"]
    homes = [v for v in services["worker"]["volumes"] if v["target"] == "/home/codeforge-tools"]
    assert homes, services["worker"]["volumes"]
    assert homes[0]["type"] == "volume", homes


def _default(value: str) -> str:
    """The default of a Compose ${VAR:-default} reference (nested ones resolved)."""
    match = re.fullmatch(r"\$\{(\w+):-(.*)\}", value)
    assert match, value
    return _default(match.group(2)) if match.group(2).startswith("${") else match.group(2)


def test_core_trusts_only_the_proxy_addresses_of_the_public_network() -> None:
    """KI-211: nginx (Traefik in blue-green) is the Core's peer; without trusted proxies every client
    shares the proxy's rate-limit bucket. The containers get addresses from ip_range, the Docker
    gateway (the peer of connections through docker-proxy) stays outside it, so it is not trusted."""
    ipam = COMPOSE["networks"]["public"]["ipam"]["config"][0]
    subnet = ipaddress.ip_network(_default(ipam["subnet"]))
    ip_range = ipaddress.ip_network(_default(ipam["ip_range"]))
    gateway = ipaddress.ip_address(_default(ipam["gateway"]))
    assert ip_range.subnet_of(subnet)  # type: ignore[arg-type]
    assert gateway in subnet
    assert gateway not in ip_range
    trusted = CORE["environment"]["CODEFORGE_TRUSTED_PROXIES"]
    assert trusted == "${CODEFORGE_TRUSTED_PROXIES:-" + ipam["ip_range"] + "}"
    assert _default(trusted) == str(ip_range)


def test_public_subnet_is_outside_dockers_default_address_pools() -> None:
    """KI-211: a fixed subnet inside Docker's default pools (172.17-172.31.0.0/16, 192.168.0.0/16)
    can collide with a network Docker allocated earlier ("Pool overlaps with other one")."""
    ipam = COMPOSE["networks"]["public"]["ipam"]["config"][0]
    subnet = ipaddress.ip_network(_default(ipam["subnet"]))
    for pool in (ipaddress.ip_network("172.16.0.0/12"), ipaddress.ip_network("192.168.0.0/16")):
        assert not subnet.overlaps(pool), (subnet, pool)  # type: ignore[arg-type]


class _ComposeLoader(yaml.SafeLoader):
    """Reads Compose's !reset and !override tags as their plain values."""


def _compose_tag(loader: yaml.SafeLoader, _suffix: str, node: yaml.Node) -> object:
    if isinstance(node, yaml.MappingNode):
        return loader.construct_mapping(node)
    if isinstance(node, yaml.SequenceNode):
        return loader.construct_sequence(node)
    return loader.construct_scalar(node)  # type: ignore[arg-type]


_ComposeLoader.add_multi_constructor("!", _compose_tag)


def _blue_green_overlay() -> dict[str, dict[str, dict[str, object]]]:
    return yaml.load((REPO / "docker-compose.blue-green.yml").read_text(), Loader=_ComposeLoader)  # noqa: S506


def test_traefik_has_no_docker_access() -> None:
    """KI-214 (R11-8): the internet-facing Traefik held the raw Docker socket (:ro does not limit
    API calls), and a socket proxy that allows container reads still serves /containers/{id}/archive,
    /export and /logs (every container's files and /run/secrets). Traefik routes through its file
    provider instead: deploy-blue-green.sh writes the active color's routes, so nothing in the
    overlay reaches the Docker API. Traefik runs without capabilities except binding 80/443."""
    overlay = _blue_green_overlay()
    services = overlay["services"]
    traefik = services["traefik"]

    for name, svc in services.items():
        assert not any("docker.sock" in str(v) for v in svc.get("volumes") or []), name  # type: ignore[attr-defined]
    assert "docker-socket-proxy" not in services
    command = traefik["command"]
    assert not any(flag.startswith("--providers.docker") for flag in command)  # type: ignore[attr-defined]
    assert "--providers.file.directory=/etc/traefik/dynamic" in command  # type: ignore[operator]
    assert "--providers.file.watch=true" in command  # type: ignore[operator]
    assert "./traefik/dynamic:/etc/traefik/dynamic:ro" in traefik["volumes"]  # type: ignore[operator]
    assert str(traefik["environment"]["CODEFORGE_DOMAIN"]).startswith("${CODEFORGE_DOMAIN:?")  # type: ignore[index]
    assert traefik["cap_drop"] == ["ALL"]
    assert traefik["cap_add"] == ["NET_BIND_SERVICE"]
    assert "no-new-privileges:true" in traefik["security_opt"]  # type: ignore[operator]
    assert traefik["read_only"] is True
    for color in ("core-blue", "core-green", "frontend-blue", "frontend-green"):
        labels = services[color].get("labels") or []
        assert not any(str(label).startswith("traefik.") for label in labels), color  # type: ignore[attr-defined]
    deploy = (REPO / "scripts" / "deploy-blue-green.sh").read_text()
    assert "change up -d --no-deps traefik" in deploy
    assert "docker-socket-proxy" not in deploy


_DOCKER_STUB = r"""#!/bin/bash
# docker stub: compose services run per $STATE; containers are "id-<service>".
echo "docker $*" >> "$LOG"
routes() { grep -o 'core-[a-z]*:8080' "$ROOT/traefik/dynamic/active-color.yaml" 2>/dev/null || echo none; }
if [ "$1" = inspect ]; then
    case "${!#}" in id-*-"$UNHEALTHY") echo unhealthy;; *) echo healthy;; esac
    exit 0
fi
args=("$@")
for ((i = 0; i < ${#args[@]}; i++)); do
    case "${args[i]}" in
    ps) service="${!#}"; grep -qx "$service" "$STATE" && echo "id-$service"; exit 0 ;;
    up) for s in "${args[@]:i+1}"; do case $s in -*) ;; *) echo "$s" >> "$STATE"; echo "routes at up $s: $(routes)" >> "$LOG";; esac; done; exit 0 ;;
    stop) for s in "${args[@]:i+1}"; do grep -vx "$s" "$STATE" > "$STATE.new"; mv "$STATE.new" "$STATE"; done
          echo "routes at stop: $(routes)" >> "$LOG"; exit 0 ;;
    pull) exit 0 ;;
    esac
done
"""


def _run_deploy(tmp_path: Path, unhealthy: str = "none") -> tuple[subprocess.CompletedProcess[str], Path, str]:
    root = tmp_path / "repo"
    (root / "scripts").mkdir(parents=True)
    (root / "traefik" / "dynamic").mkdir(parents=True)
    shutil.copy(REPO / "scripts" / "deploy-blue-green.sh", root / "scripts")
    bin_dir = tmp_path / "bin"
    bin_dir.mkdir()
    (bin_dir / "docker").write_text(_DOCKER_STUB)
    (bin_dir / "docker").chmod(0o755)
    state = tmp_path / "running"
    state.write_text("postgres\nnats\nlitellm\ntraefik\ncore-blue\nfrontend-blue\n")
    log = tmp_path / "calls.log"
    log.touch()
    result = subprocess.run(  # noqa: S603 - the repository's own script
        [str(BASH), str(root / "scripts" / "deploy-blue-green.sh")],
        capture_output=True,
        text=True,
        env={
            **os.environ,
            "PATH": f"{bin_dir}:{os.environ['PATH']}",
            "ROOT": str(root),
            "STATE": str(state),
            "LOG": str(log),
            "UNHEALTHY": unhealthy,
            "HEALTH_TIMEOUT": "1",
            "HEALTH_INTERVAL": "1",
            "ROUTE_SWITCH_WAIT": "0",
        },
        check=False,
        timeout=60,
    )
    return result, root / "traefik" / "dynamic", log.read_text()


@pytest.mark.skipif(BASH is None, reason="needs bash")
def test_deploy_routes_traefik_to_the_new_color_before_stopping_the_old(tmp_path: Path) -> None:
    result, dynamic, calls = _run_deploy(tmp_path)
    assert result.returncode == 0, result.stdout + result.stderr
    # Traefik is (re)started with the active color's routes, the new color gets the
    # traffic once it is healthy, and only then the old color stops.
    assert "routes at up traefik: core-blue:8080" in calls, calls
    assert "routes at stop: core-green:8080" in calls, calls
    assert "docker-socket-proxy" not in calls
    assert sorted(p.name for p in dynamic.iterdir()) == ["active-color.yaml"]

    text = (dynamic / "active-color.yaml").read_text()
    routes = yaml.safe_load(text.replace('{{ env "CODEFORGE_DOMAIN" }}', "x.example"))["http"]
    middlewares = yaml.safe_load((REPO / "traefik" / "dynamic" / "middleware.yaml").read_text())["http"]["middlewares"]
    core, frontend = routes["routers"]["core"], routes["routers"]["frontend"]
    assert core["rule"].startswith("Host(`x.example`) && (PathPrefix(`/api`)")
    for prefix in ("/api", "/health", "/ws", "/.well-known", "/a2a"):
        assert f"PathPrefix(`{prefix}`)" in core["rule"], prefix
    assert frontend["rule"] == "Host(`x.example`)"
    assert core["priority"] > frontend["priority"]
    for router in (core, frontend):
        assert router["entryPoints"] == ["websecure"]
        assert router["tls"] == {"certResolver": "letsencrypt"}
        assert set(router["middlewares"]) <= set(middlewares)
    assert "rate-limit" in core["middlewares"]
    assert routes["services"]["core"]["loadBalancer"]["servers"] == [{"url": "http://core-green:8080"}]
    assert routes["services"]["frontend"]["loadBalancer"]["servers"] == [{"url": "http://frontend-green:8080"}]


@pytest.mark.skipif(BASH is None, reason="needs bash")
def test_deploy_keeps_routing_to_the_active_color_when_the_new_one_fails(tmp_path: Path) -> None:
    result, dynamic, calls = _run_deploy(tmp_path, unhealthy="green")
    assert result.returncode == 1
    assert "routes at stop: core-blue:8080" in calls, calls
    assert "http://core-blue:8080" in (dynamic / "active-color.yaml").read_text()


def test_core_api_port_is_published_on_loopback_only() -> None:
    """KI-213: clients use the frontend's port; the plain-HTTP API port is for checks on the host."""
    assert CORE["ports"] == ["127.0.0.1:${CORE_PORT:-8080}:8080"]


def test_nginx_does_not_cut_long_api_requests() -> None:
    """KI-213: the Core bounds API requests itself; clone, setup and pull may take minutes."""
    conf = (REPO / "frontend" / "nginx.conf").read_text()
    api = conf.split("location /api/ {", 1)[1].split("\n    }", 1)[0]
    assert re.search(r"^\s*proxy_read_timeout 1h;", api, re.MULTILINE), api


def _nginx_location(conf: str, match: str) -> str:
    return conf.split(f"location {match} {{", 1)[1].split("\n    }", 1)[0]


@pytest.mark.parametrize("match", ["= /a2a", "= /.well-known/agent-card.json"])
def test_nginx_proxies_a2a_to_the_core(match: str) -> None:
    """KI-213: the Core port is loopback-only, so external A2A callers come through nginx."""
    conf = (REPO / "frontend" / "nginx.conf").read_text()
    assert f"location {match} {{" in conf
    block = _nginx_location(conf, match)
    for line in (
        "proxy_pass http://${CORE_UPSTREAM};",
        "proxy_set_header Host $host;",
        "proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;",
        "proxy_set_header X-Forwarded-Proto $scheme;",
    ):
        assert line in block, (match, line)


def test_nginx_a2a_streams_are_not_buffered_or_cut() -> None:
    """KI-213: message/stream and tasks/resubscribe answer with SSE that may run for a long time."""
    block = _nginx_location((REPO / "frontend" / "nginx.conf").read_text(), "= /a2a")
    for directive in (
        "proxy_http_version 1.1;",
        'proxy_set_header Connection "";',
        "proxy_buffering off;",
        "proxy_cache off;",
        "proxy_read_timeout 1h;",
    ):
        assert re.search(rf"^\s*{re.escape(directive)}", block, re.MULTILINE), directive


def test_live_e2e_core_listens_on_loopback() -> None:
    """KI-213: live E2E runs with public dev credentials and tool isolation off."""
    env = (REPO / "scripts" / "live-e2e" / "env.example.sh").read_text()
    assert "export CODEFORGE_HOST=${CODEFORGE_HOST:-127.0.0.1}" in env


def _dummy_secrets(directory: Path) -> Path:
    directory.mkdir()
    for entry in COMPOSE["secrets"].values():
        (directory / Path(entry["file"]).name).write_text("x")
    return directory


@pytest.mark.skipif(shutil.which("docker") is None, reason="needs docker compose")
@pytest.mark.parametrize("overlay", [False, True], ids=["prod", "blue-green"])
@pytest.mark.parametrize(
    ("env", "want"),
    [
        ({}, "10.250.240.128/25"),
        ({"CODEFORGE_PUBLIC_IP_RANGE": "10.77.0.128/25", "CODEFORGE_PUBLIC_SUBNET": "10.77.0.0/24"}, "10.77.0.128/25"),
        ({"CODEFORGE_TRUSTED_PROXIES": "10.1.2.3"}, "10.1.2.3"),
    ],
    ids=["default", "own-range", "explicit"],
)
def test_compose_resolves_the_trusted_proxies(overlay: bool, env: dict[str, str], want: str, tmp_path: Path) -> None:
    files = ["-f", "docker-compose.prod.yml"]
    if overlay:
        files += ["-f", "docker-compose.blue-green.yml", "--profile", "blue", "--profile", "green"]
    base = {k: v for k, v in os.environ.items() if not k.startswith(("CODEFORGE_PUBLIC_", "CODEFORGE_TRUSTED_"))}
    result = subprocess.run(  # noqa: S603 - docker compose of this repository
        [str(shutil.which("docker")), "compose", *files, "config", "--format", "json"],
        cwd=REPO,
        capture_output=True,
        text=True,
        env={
            **base,
            "SECRETS_DIR": str(_dummy_secrets(tmp_path / "secrets")),
            "ACME_EMAIL": "ops@example.com",
            "CODEFORGE_DOMAIN": "x.example",
            **env,
        },
        check=False,
        timeout=120,
    )
    if result.returncode != 0 and "'compose' is not a docker command" in result.stderr:
        pytest.skip("docker compose plugin not installed")
    assert result.returncode == 0, result.stderr
    config = yaml.safe_load(result.stdout)
    for name in ["core-blue", "core-green"] if overlay else ["core"]:
        assert config["services"][name]["environment"]["CODEFORGE_TRUSTED_PROXIES"] == want, name
    ipam = config["networks"]["public"]["ipam"]["config"][0]
    if "CODEFORGE_TRUSTED_PROXIES" not in env:
        assert ipam["ip_range"] == want


def test_core_image_has_svn_and_no_gh() -> None:
    """KI-117: SVN projects need svn in the Go Core (hardened SVN adapter); GitHub goes through its REST API."""
    runtime = (REPO / "Dockerfile").read_text().split("# --- Runtime stage ---", 1)[1]
    assert re.search(r"apk add[^\n]*\bsubversion\b", runtime)
    assert not re.search(r"\b(github-cli|gh)\b", runtime.split("RUN apk add", 1)[1].split("\n", 1)[0])
