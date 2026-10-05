"""Per-tenant tool isolation in the built worker image (KI-96, ADR-018), with the production service definition.

Every check runs the worker service of docker-compose.prod.yml (plus
workers/tests/docker/compose.isolation.yml, which only selects the image
under test and mounts the checks): started as root with SETUID, SETGID and
KILL only, no-new-privileges, read-only root, the workspaces and tool_homes
volumes, /tmp 1771, the secrets tmpfs, ``APP_ENV=production`` and
``CODEFORGE_TOOL_ISOLATION=required``. Each test gets its own compose
project (fresh volumes) and removes it.

- the image: 10,001 generated tool users, no subordinate IDs, the layout;
- two tenants (tests.tool_isolation_check): credentials, what each tool
  process is denied, what it can still do, no secret on a command line;
- what agents run (the battery; toolchains with the battery image) and the
  agent backend CLIs of the image (KI-118);
- the host preflight (scripts/check-host.sh) passes here and fails closed
  without Landlock, with Landlock off in production, with the KI-71 compose
  file, with a tmpfs HOME and without POSIX ACLs; the real worker then
  answers /health/ready with 503 and the reason, and no tool call runs;
- the migration of a KI-71 tree (hard links into another tenant's tree,
  planted ACL entries, a tenant directory 10002 replaced), at scale with
  dropped caches while the event loop keeps ticking;
- a rollback to the older worker is detected; two workers on one volume
  keep a migration out of the tenant's running work;
- a project deletion removes a tree a crashed tool locked the worker out of;
- a tool process's /proc rule survives dropped caches (E6).

Needs docker with compose, bash and openssl (scripts/generate-secrets.sh)
and ``CODEFORGE_TEST_WORKER_IMAGE``; ``CODEFORGE_TEST_BATTERY_IMAGE``
(workers/tests/docker/Dockerfile.battery) adds node, go and java to the
battery. Dropping the host's caches needs root or passwordless sudo.
Skipped otherwise; with ``CODEFORGE_ISOLATION_TESTS=required`` (CI) a
missing requirement fails instead. ``CODEFORGE_TEST_EVIDENCE_DIR`` keeps
every report as JSON.
"""

from __future__ import annotations

import json
import os
import shutil
import subprocess
import time
import uuid
from dataclasses import dataclass, field
from pathlib import Path
from typing import TYPE_CHECKING

import pytest

from tests.isolation_requirements import require

if TYPE_CHECKING:
    from collections.abc import Iterator

REPO = Path(__file__).resolve().parents[2]
DOCKER_DIR = Path(__file__).resolve().parent / "docker"
IMAGE = os.environ.get("CODEFORGE_TEST_WORKER_IMAGE", "")
BATTERY_IMAGE = os.environ.get("CODEFORGE_TEST_BATTERY_IMAGE", "")
EVIDENCE = os.environ.get("CODEFORGE_TEST_EVIDENCE_DIR", "")
MIGRATION_ENTRIES = int(os.environ.get("CODEFORGE_TEST_MIGRATION_ENTRIES", "100000"))
DOCKER = shutil.which("docker")
SECRET_FILES = ["database-url", "nats-url", "litellm-master-key", "codeforge-internal-key"]

pytestmark = pytest.mark.docker


def _missing() -> str:
    if not IMAGE:
        return "set CODEFORGE_TEST_WORKER_IMAGE to the worker image under test"
    if DOCKER is None or shutil.which("bash") is None or shutil.which("openssl") is None:
        return "needs docker, bash and openssl"
    if subprocess.run([DOCKER, "compose", "version"], capture_output=True, check=False).returncode:  # noqa: S603
        return "needs the docker compose plugin"
    return ""


@pytest.fixture(autouse=True, scope="module")
def _requirements() -> None:
    missing = _missing()
    require(not missing, missing)


def _record(name: str, report: object) -> None:
    if EVIDENCE:
        Path(EVIDENCE).mkdir(parents=True, exist_ok=True)
        Path(EVIDENCE, f"{name}.json").write_text(json.dumps(report, indent=2, default=str) + "\n")


@pytest.fixture(scope="module")
def secrets_dir(tmp_path_factory: pytest.TempPathFactory) -> Path:
    path = tmp_path_factory.mktemp("docker-secrets") / "secrets"
    env = {**os.environ, "SECRETS_DIR": str(path), "COMPOSE_ENV_FILE": "/nonexistent"}
    done = subprocess.run(  # noqa: S603 - the repository's own script
        [str(shutil.which("bash")), str(REPO / "scripts" / "generate-secrets.sh"), str(path)],
        capture_output=True,
        text=True,
        env=env,
        check=False,
    )
    assert done.returncode == 0, done.stderr
    return path


@dataclass
class Outcome:
    code: int
    output: str
    report: dict[str, object] = field(default_factory=dict)


@dataclass
class Stack:
    """One compose project of the production worker service with fresh volumes."""

    project: str
    env: dict[str, str]
    env_file: str
    started: list[str] = field(default_factory=list)

    def base(self, files: tuple[str, ...] = ()) -> list[str]:
        command = [str(DOCKER), "compose", "--env-file", self.env_file, "-p", self.project]
        command += ["-f", "docker-compose.prod.yml", "-f", str(DOCKER_DIR / "compose.isolation.yml")]
        for name in files:
            command += ["-f", str(DOCKER_DIR / name)]
        return command

    def compose(self, *args: str, files: tuple[str, ...] = (), image: str = "", timeout: float = 900) -> Outcome:
        env = {**self.env, "CODEFORGE_TEST_WORKER_IMAGE": image or IMAGE}
        done = subprocess.run(  # noqa: S603 - docker compose of this repository
            [*self.base(files), *args], cwd=REPO, capture_output=True, text=True, env=env, timeout=timeout, check=False
        )
        return Outcome(done.returncode, done.stdout + done.stderr)

    def check(self, *args: str, files: tuple[str, ...] = (), image: str = "", timeout: float = 900) -> Outcome:
        """Run ``tests.docker_isolation_checks <args>`` in a one-off worker container."""
        outcome = self.compose(
            "run", "--rm", "--no-deps", "-T", "worker", "python", "-m", "tests.docker_isolation_checks", *args,
            files=files, image=image, timeout=timeout,
        )  # fmt: skip
        outcome.report = _report(outcome.output)
        return outcome

    def start(self, name: str, *command: str, files: tuple[str, ...] = ()) -> str:
        """Start a detached one-off worker container named *name* (removed with the stack)."""
        container = f"{self.project}-{name}"
        outcome = self.compose("run", "-d", "--no-deps", "--name", container, "worker", *command, files=files)
        assert outcome.code == 0, outcome.output
        self.started.append(container)
        return container

    def down(self) -> None:
        for container in self.started:
            subprocess.run([str(DOCKER), "rm", "-f", container], capture_output=True, check=False)  # noqa: S603
        self.compose("down", "-v", "--remove-orphans", timeout=300)


def _report(output: str) -> dict[str, object]:
    for line in reversed(output.splitlines()):
        if line.startswith("CF-REPORT "):
            return json.loads(line[len("CF-REPORT ") :])
    return {}


@pytest.fixture
def stack(secrets_dir: Path, tmp_path: Path) -> Iterator[Stack]:
    env_file = tmp_path / "empty.env"
    env_file.write_text("")
    env = {
        **os.environ,
        "SECRETS_DIR": str(secrets_dir),
        "WORKER_IMAGE": IMAGE,
        "CODEFORGE_TEST_SECCOMP_PROFILE": str(DOCKER_DIR / "seccomp-no-landlock.json"),
    }
    stack = Stack(project=f"cf-ki96-{uuid.uuid4().hex[:8]}", env=env, env_file=str(env_file))
    try:
        yield stack
    finally:
        stack.down()


def _logs(container: str) -> str:
    done = subprocess.run([str(DOCKER), "logs", container], capture_output=True, text=True, check=False)  # noqa: S603
    return done.stdout + done.stderr


def _wait_for(container: str, marker: str, timeout: float = 120) -> None:
    deadline = time.monotonic() + timeout
    while marker not in _logs(container):
        assert time.monotonic() < deadline, f"{container} never printed {marker}: {_logs(container)[-2000:]}"
        time.sleep(0.5)


def _wait_exit(container: str, timeout: float = 300) -> None:
    done = subprocess.run(  # noqa: S603 - docker of the test's own container
        [str(DOCKER), "wait", container], capture_output=True, text=True, timeout=timeout, check=False
    )
    assert done.returncode == 0, done.stderr


def _drop_caches() -> bool:
    """Drop the host's dentry and inode caches (root or passwordless sudo)."""
    command = "sync; echo 3 > /proc/sys/vm/drop_caches"
    if os.geteuid() == 0:
        prefix: list[str] = []
    elif shutil.which("sudo"):
        prefix = ["sudo", "-n"]
    else:
        return False
    return subprocess.run([*prefix, "sh", "-c", command], capture_output=True, check=False).returncode == 0  # noqa: S603


def _need_drop_caches() -> None:
    require(_drop_caches(), "dropping the host's caches needs root or passwordless sudo")


# ---------------------------------------------------------------------------
# The image and two tenants
# ---------------------------------------------------------------------------


def test_the_image_generates_one_user_per_tool_uid(stack: Stack) -> None:
    outcome = stack.check("image")
    _record("image", outcome.report)
    assert outcome.report.get("problems") == [], outcome.output[-3000:]
    assert outcome.report["generated passwd entries"] == 10_001
    assert outcome.code == 0


def test_two_tenants_are_isolated_in_the_production_container(stack: Stack) -> None:
    secrets = [f"/run/secrets/{name}" for name in SECRET_FILES]
    outcome = stack.compose(
        "run", "--rm", "--no-deps", "-T", "worker",
        "python", "-m", "tests.tool_isolation_check", "/data/workspaces", "/home/codeforge-tools", *secrets,
    )  # fmt: skip
    start = outcome.output.find("{")
    report = json.loads(outcome.output[start : outcome.output.rfind("}") + 1])
    _record("two-tenants", report)
    assert report["problems"] == [], report["problems"]
    assert report["ready"] is True
    assert report["landlock_abi"] >= 2
    for name, uid in (("A", 20000), ("B", 20001)):
        tenant = next(value for key, value in report.items() if key.startswith(f"tenant {name} "))
        credentials = tenant["credentials"]
        assert credentials["Uid"].split() == [str(uid)] * 4
        assert credentials["Groups"] == ""
        assert credentials["CapEff"] == "0000000000000000"
        assert credentials["NoNewPrivs"] == "1"
        assert "list the other tenant's directory" in tenant["denied"]
        assert "read the other tenant's command line" in tenant["denied"]
    assert outcome.code == 0


def test_what_agents_run_works_confined(stack: Stack) -> None:
    for name, image in (("battery", IMAGE), ("battery-toolchains", BATTERY_IMAGE)):
        if not image:
            continue
        outcome = stack.check("battery", image=image)
        _record(name, outcome.report)
        assert outcome.report.get("problems") == [], outcome.output[-4000:]
        if image == BATTERY_IMAGE:
            assert len(outcome.report["toolchains"]) == 4, outcome.report


def test_the_backend_clis_run_as_a_tenants_tool_user(stack: Stack) -> None:
    """KI-118: the image's agent backend CLIs are found on the tool PATH and run confined."""
    outcome = stack.check("backends")
    _record("backends", outcome.report)
    assert outcome.report.get("problems") == [], outcome.output[-4000:]
    assert outcome.code == 0


# ---------------------------------------------------------------------------
# Failing closed and visibly
# ---------------------------------------------------------------------------


def _preflight(stack: Stack, *files: str) -> Outcome:
    extra: list[str] = []
    for name in ("compose.isolation.yml", *files):
        extra += ["-f", str(DOCKER_DIR / name)]
    env = {**stack.env, "COMPOSE_PROJECT_NAME": stack.project, "CODEFORGE_TEST_WORKER_IMAGE": IMAGE}
    done = subprocess.run(  # noqa: S603 - the repository's own script
        [str(shutil.which("bash")), str(REPO / "scripts" / "check-host.sh"), "--env-file", stack.env_file, *extra],
        cwd=REPO,
        capture_output=True,
        text=True,
        env=env,
        timeout=600,
        check=False,
    )
    return Outcome(done.returncode, done.stdout + done.stderr)


def test_the_preflight_passes_on_this_host(stack: Stack) -> None:
    outcome = _preflight(stack)
    _record("preflight", outcome.output.splitlines())
    assert outcome.code == 0, outcome.output
    assert "result: OK" in outcome.output
    assert "tool isolation: ready (Landlock ABI" in outcome.output


@pytest.mark.parametrize(
    ("scenario", "reason"),
    [
        ("compose.no-landlock.yml", "ENOSYS"),
        ("compose.landlock-off.yml", "not allowed with APP_ENV=production"),
        ("compose.old-tool-home.yml", "tool_homes volume"),
        ("compose.tmpfs-home.yml", "noexec"),
        ("compose.no-acl-homes.yml", "no POSIX ACLs on /home/codeforge-tools"),
    ],
)
def test_a_host_that_cannot_isolate_fails_closed(stack: Stack, scenario: str, reason: str) -> None:
    outcome = _preflight(stack, scenario)
    _record(f"preflight-{scenario}", outcome.output.splitlines())
    assert outcome.code == 1, outcome.output
    assert "result: FAILED" in outcome.output
    assert reason in outcome.output
    refused = stack.check("refused-call", files=(scenario,))
    _record(f"refused-{scenario}", refused.report)
    assert refused.report.get("problems") == [], refused.output[-3000:]
    assert refused.report["marker exists"] is False
    assert "ToolIsolationError" in str(refused.report["tool call"])


def test_the_worker_answers_503_with_the_reason(stack: Stack) -> None:
    """The real worker (python -m codeforge.consumer) next to the production NATS service."""
    up = stack.compose("up", "-d", "nats")
    assert up.code == 0, up.output
    container = stack.start("worker", files=("compose.no-landlock.yml",))
    probe = (
        "import urllib.request, urllib.error\n"
        "try:\n"
        "    r = urllib.request.urlopen('http://127.0.0.1:8081/health/ready', timeout=3); print(r.status, r.read())\n"
        "except urllib.error.HTTPError as e:\n"
        "    print(e.code, e.read())\n"
    )
    deadline = time.monotonic() + 120
    answer = ""
    while time.monotonic() < deadline:
        done = subprocess.run(  # noqa: S603 - docker exec of the test's own container
            [str(DOCKER), "exec", container, "python", "-c", probe], capture_output=True, text=True, check=False
        )
        answer = done.stdout.strip()
        if "tool isolation not ready" in answer:
            break
        time.sleep(1)
    _record("health-503", {"answer": answer, "logs": _logs(container)[-3000:]})
    assert answer.startswith("503"), answer + _logs(container)[-3000:]
    assert "tool isolation not ready" in answer
    assert "Landlock" in answer


# ---------------------------------------------------------------------------
# Migration, rollback, two workers
# ---------------------------------------------------------------------------


def test_a_tree_from_before_the_upgrade_is_migrated(stack: Stack) -> None:
    outcome = stack.check("migration")
    _record("migration", outcome.report)
    assert outcome.report.get("problems") == [], outcome.output[-4000:]
    assert "10002" in str(outcome.report["tenant directory of 10002"])


def test_a_large_tree_migrates_while_the_worker_keeps_ticking(stack: Stack) -> None:
    seeded = stack.check("scale-seed", str(MIGRATION_ENTRIES), timeout=1800)
    assert seeded.report.get("problems") == [], seeded.output[-3000:]
    _need_drop_caches()
    outcome = stack.check("scale-migrate", timeout=1800)
    _record("migration-scale", {"seed": seeded.report, "migrate": outcome.report})
    assert outcome.report.get("problems") == [], outcome.output[-3000:]


def test_a_rollback_to_the_older_worker_is_detected(stack: Stack) -> None:
    for step in ("prepare-a", "ki71-walk", "after-rollback"):
        outcome = stack.check(step)
        _record(f"rollback-{step}", outcome.report)
        assert outcome.report.get("problems") == [], f"{step}: {outcome.output[-3000:]}"


def test_a_migration_waits_for_the_tenants_work_in_another_worker(stack: Stack) -> None:
    holder = stack.start("hold", "python", "-m", "tests.docker_isolation_checks", "hold", "40")
    _wait_for(holder, "CF-HOLDING")
    refused = stack.check("contend", "3")
    assert "waiting for the tenant's other work" in str(refused.report.get("refused")), refused.output[-3000:]
    _wait_exit(holder)
    held = _report(_logs(holder))
    assert held.get("problems") == [], _logs(holder)[-3000:]
    entered = stack.check("contend", "3")
    _record("two-workers", {"while held": refused.report, "after": entered.report})
    assert entered.report.get("entered") is True, entered.output[-3000:]


# ---------------------------------------------------------------------------
# Deletion and /proc
# ---------------------------------------------------------------------------


def test_a_deleted_projects_locked_out_tree_is_removed(stack: Stack) -> None:
    outcome = stack.check("deletion")
    _record("deletion", outcome.report)
    assert outcome.report.get("problems") == [], outcome.output[-3000:]
    assert outcome.report["workspace exists after the deletion"] is False


def test_the_proc_rule_survives_dropped_caches(stack: Stack) -> None:
    _need_drop_caches()
    container = stack.start("pin", "python", "-m", "tests.docker_isolation_checks", "pin", "12")
    _wait_for(container, "CF-PINNING")
    for _ in range(3):
        time.sleep(2)
        assert _drop_caches()
    _wait_exit(container)
    report = _report(_logs(container))
    _record("proc-pin", report)
    assert report.get("problems") == [], _logs(container)[-3000:]
