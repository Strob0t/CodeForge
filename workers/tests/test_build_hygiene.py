"""Build, CI and deployment hygiene (KI-214).

What reaches the images (.dockerignore), what CI runs and pins, and the
compose settings around shutdown and the dev browser container.
"""

from __future__ import annotations

import http.server
import json
import os
import re
import shutil
import subprocess
import threading
from pathlib import Path
from typing import ClassVar

import pytest
import yaml

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


CI = yaml.safe_load((REPO / ".github" / "workflows" / "ci.yml").read_text())
PROD = yaml.safe_load((REPO / "docker-compose.prod.yml").read_text())


def _steps(job: str) -> list[dict[str, object]]:
    return CI["jobs"][job]["steps"]  # type: ignore[no-any-return]


def test_ci_actions_are_pinned_to_commits() -> None:
    """A tag of a third-party action can be moved; gitleaks-action also gets GITHUB_TOKEN."""
    for name, job in CI["jobs"].items():
        for step in job["steps"]:
            uses = step.get("uses")
            if uses:
                assert re.fullmatch(r"[\w.-]+/[\w./-]+@[0-9a-f]{40}", uses), f"{name}: {uses}"


def test_ci_runs_the_nats_version_of_production() -> None:
    prod_image = PROD["services"]["nats"]["image"]
    text = (REPO / ".github" / "workflows" / "ci.yml").read_text()
    assert "nats:latest" not in text
    for image in re.findall(r"\bnats:[\w.-]+", text):
        assert image == prod_image, image


def test_ci_python_job_runs_the_real_sql_tests() -> None:
    """workers/tests/pg_schema.py skips the worker's real-SQL tenant tests without a database."""
    job = CI["jobs"]["test-python"]
    assert job["services"]["postgres"]["image"].startswith("postgres:18")
    url = job["env"]["CODEFORGE_TEST_DATABASE_URL"]
    assert url.startswith("postgresql://")
    assert "@localhost:5432/" in url


def test_ci_smoke_core_listens_on_loopback() -> None:
    assert CI["jobs"]["smoke"]["env"]["CODEFORGE_HOST"] == "127.0.0.1"


GOIMPORTS = shutil.which("goimports")


@pytest.mark.skipif(GOIMPORTS is None or shutil.which("bash") is None, reason="needs goimports and bash")
def test_go_imports_hook_fails_on_a_diff(tmp_path: Path) -> None:
    """`goimports -l -d` exits 0 with a diff, so the hook could never fail."""
    config = yaml.safe_load((REPO / ".pre-commit-config.yaml").read_text())
    hooks = [h for repo in config["repos"] for h in repo["hooks"] if h["id"] == "go-imports-repo"]
    assert len(hooks) == 1
    entry = hooks[0]["entry"]
    bad = tmp_path / "bad.go"
    bad.write_text('package x\nimport ("os"\n"fmt")\nvar _ = fmt.Sprint\nvar _ = os.Args\n')
    good = tmp_path / "good.go"
    good.write_text('package x\n\nimport "fmt"\n\nvar _ = fmt.Sprint\n')

    def run(path: Path) -> subprocess.CompletedProcess[str]:
        return subprocess.run(  # noqa: S603 - the hook's own command line
            [str(shutil.which("bash")), "-c", f'{entry} "$@"', "--", str(path)],
            capture_output=True,
            text=True,
            check=False,
        )

    assert run(bad).returncode != 0
    assert run(good).returncode == 0


_PYTEST_PASSED = """\
........                                                                 [100%]
==================================== PASSES ====================================
=========================== short test summary info ============================
PASSED workers/tests/test_llm_registry.py::test_lists_models
PASSED workers/tests/test_tool_bash.py::test_runs
PASSED workers/tests/test_consumer_runs.py::test_starts
"""


@pytest.mark.skipif(shutil.which("bash") is None, reason="needs bash")
@pytest.mark.parametrize(
    ("pytest_output", "pytest_exit", "want_exit"),
    [
        (_PYTEST_PASSED, 0, 0),
        (_PYTEST_PASSED + "FAILED workers/tests/test_llm_registry.py::test_default - AssertionError\n", 1, 1),
        ("ERROR workers/tests/test_tool_bash.py - ImportError: cannot import\nInterrupted: 1 error\n", 2, 1),
    ],
    ids=["all-pass", "critical-failure", "collection-error"],
)
def test_feature_verification_python_half_can_fail(
    tmp_path: Path, pytest_output: str, pytest_exit: int, want_exit: int
) -> None:
    """Without pytest-json-report the script wrote an empty report and -q printed no test lines,
    so a failing Python test never failed the Feature Verification job."""
    scripts = tmp_path / "repo" / "scripts"
    scripts.mkdir(parents=True)
    shutil.copy(REPO / "scripts" / "verify-features.sh", scripts / "verify-features.sh")
    output = tmp_path / "pytest-output.txt"
    output.write_text(pytest_output)
    bin_dir = tmp_path / "bin"
    bin_dir.mkdir()
    (bin_dir / "poetry").write_text(
        "#!/bin/bash\n"
        'case "$*" in *test_nats_contracts*) echo "1 passed"; exit 0;; esac\n'
        f'cat "{output}"\nexit {pytest_exit}\n'
    )
    (bin_dir / "go").write_text("#!/bin/bash\nexit 0\n")
    for stub in bin_dir.iterdir():
        stub.chmod(0o755)
    work = tmp_path / "tmp"
    work.mkdir()
    result = subprocess.run(  # noqa: S603 - the repository's own script with stub tools
        [str(shutil.which("bash")), str(scripts / "verify-features.sh")],
        capture_output=True,
        text=True,
        env={"PATH": f"{bin_dir}:/usr/bin:/bin", "TMPDIR": str(work), "HOME": str(tmp_path)},
        check=False,
        timeout=120,
    )
    assert result.returncode == want_exit, result.stdout + result.stderr
    if want_exit == 0:
        assert "| 4 | LLM Model Registry | NONE | PASS |" in result.stdout, result.stdout


def _seconds(duration: str) -> int:
    match = re.fullmatch(r"(?:(\d+)m)?(?:(\d+)s)?", duration)
    assert match, duration
    return int(match.group(1) or 0) * 60 + int(match.group(2) or 0)


def test_core_gets_time_for_its_graceful_shutdown() -> None:
    """Docker's default of 10 s killed the Core in phase 1 (HTTP shutdown, up to 30 s), before the
    NATS drain (up to 30 s) published its pending messages."""
    assert _seconds(PROD["services"]["core"]["stop_grace_period"]) >= 65


DEV = yaml.safe_load((REPO / "docker-compose.yml").read_text())


def test_dev_browser_has_its_own_ipc_namespace() -> None:
    """playwright-mcp loads arbitrary sites; shm_size already gives Chromium its shared memory."""
    browser = DEV["services"]["playwright-mcp"]
    assert "ipc" not in browser
    assert browser["shm_size"]


class _FakeCore(http.server.BaseHTTPRequestHandler):
    """The API calls of scripts/run-agent-eval.sh; POST bodies must be JSON."""

    bodies: ClassVar[list[dict[str, object]]] = []

    def log_message(self, *_args: object) -> None:
        pass

    def _send(self, payload: object, status: int = 200) -> None:
        data = json.dumps(payload).encode()
        self.send_response(status)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(data)))
        self.end_headers()
        self.wfile.write(data)

    def do_POST(self) -> None:
        raw = self.rfile.read(int(self.headers.get("Content-Length", "0")))
        try:
            body = json.loads(raw) if raw else {}
        except json.JSONDecodeError:
            self._send({"error": "invalid JSON"}, 400)
            return
        type(self).bodies.append({"path": self.path, "body": body})
        if self.path.endswith("/auth/login"):
            self._send({"access_token": "t"})
        elif self.path.endswith("/projects"):
            self._send({"id": "p1"})
        elif self.path.endswith("/conversations"):
            self._send({"id": "c1"})
        else:
            self._send({})

    def do_GET(self) -> None:
        self._send([{"role": "assistant", "content": "done " * 20, "tool_calls": []}])


@pytest.mark.skipif(shutil.which("bash") is None or shutil.which("git") is None, reason="needs bash and git")
def test_agent_eval_sends_valid_json(tmp_path: Path) -> None:
    """The prompt went into hand-built JSON with raw newlines: the POST failed silently and every
    scenario waited out its timeout."""
    _FakeCore.bodies = []
    server = http.server.ThreadingHTTPServer(("127.0.0.1", 0), _FakeCore)
    thread = threading.Thread(target=server.serve_forever, daemon=True)
    thread.start()
    bin_dir = tmp_path / "bin"
    bin_dir.mkdir()
    (bin_dir / "sleep").write_text("#!/bin/sh\nexit 0\n")
    (bin_dir / "sleep").chmod(0o755)
    workspaces = tmp_path / 'ws "quoted"'
    try:
        result = subprocess.run(  # noqa: S603 - the repository's own script against a fake Core
            [str(shutil.which("bash")), str(REPO / "scripts" / "run-agent-eval.sh"), "--scenario", "S1"],
            capture_output=True,
            text=True,
            env={
                **os.environ,
                "PATH": f"{bin_dir}:{os.environ['PATH']}",
                "BASE_URL": f"http://127.0.0.1:{server.server_address[1]}",
                "WORKSPACE_ROOT": str(workspaces),
                "MODEL": 'model "x"',
                "GIT_AUTHOR_NAME": "t",
                "GIT_AUTHOR_EMAIL": "t@example.com",
                "GIT_COMMITTER_NAME": "t",
                "GIT_COMMITTER_EMAIL": "t@example.com",
            },
            cwd=tmp_path,
            check=False,
            timeout=120,
        )
    finally:
        server.shutdown()
    assert "Agent completed" in result.stdout, result.stdout + result.stderr
    messages = [b["body"] for b in _FakeCore.bodies if str(b["path"]).endswith("/messages")]
    assert len(messages) == 1, _FakeCore.bodies
    message = messages[0]
    assert isinstance(message, dict)
    assert "\n2. Count lines" in str(message["content"])
    assert str(workspaces) in str(message["content"])
    assert message["model"] == 'model "x"'
    assert message["agentic"] is True
    projects = [b["body"] for b in _FakeCore.bodies if str(b["path"]).endswith("/projects")]
    assert isinstance(projects[0], dict)
    assert str(projects[0]["local_path"]).startswith(str(workspaces))
    assert "\n{" in result.stdout, result.stdout + result.stderr
    summary = json.loads(result.stdout[result.stdout.rindex("\n{") :])
    assert summary["scenario"] == "S1"
    assert summary["model"] == 'model "x"'


def test_alert_rules_use_only_exported_metrics() -> None:
    """CodeForge exports no Prometheus metrics: rules on http_requests_total or
    nats_consumer_pending could never fire. The rest need cAdvisor, node_exporter and
    blackbox_exporter."""
    rules = yaml.safe_load((REPO / "configs" / "prometheus" / "alerts.yml").read_text())
    exporters = ("container_", "node_", "probe_")
    for group in rules["groups"]:
        for rule in group["rules"]:
            expr = re.sub(r"\"[^\"]*\"", '""', rule["expr"])  # label matchers are no metrics
            metrics = set(re.findall(r"\b([a-z_][a-z0-9_]*)\s*(?:\{|\[|/|>|<|=|\)|$)", expr, re.MULTILINE))
            metrics -= {"by", "le", "and", "or", "unless", "on", "sum", "rate", "time", "predict_linear"}
            for metric in metrics:
                assert metric.startswith(exporters), f"{rule['alert']}: {metric}"


SETUP = (REPO / ".devcontainer" / "setup.sh").read_text()
SESSION_START = (REPO / ".claude" / "hooks" / "session-start.sh").read_text()
# golangci-lint-2.11.4-checksums.txt of the release (linux-amd64, linux-arm64).
GOLANGCI_SHA256 = {
    "amd64": "200c5b7503f67b59a6743ccf32133026c174e272b930ee79aa2aa6f37aca7ef1",
    "arm64": "3bcfa2e6f3d32b2bf5cd75eaa876447507025e0303698633f722a05331988db4",
}


def _assigned(text: str, name: str) -> str:
    match = re.search(rf'^{name}="?([^"\s]+)"?', text, re.MULTILINE)
    assert match, name
    return match.group(1)


def test_dev_toolchain_versions_match_ci() -> None:
    ci_lint = next(s["with"]["version"] for s in _steps("test-go") if "golangci-lint-action" in s.get("uses", ""))
    assert ci_lint == "v" + _assigned(SETUP, "GOLANGCI_LINT_VERSION")
    assert ci_lint == "v" + _assigned(SESSION_START, "GOLANGCI_LINT_VERSION")
    assert _assigned(SETUP, "GOIMPORTS_VERSION") == _assigned(SESSION_START, "GOIMPORTS_VERSION")
    assert "goimports@latest" not in SETUP


def test_golangci_lint_downloads_are_verified() -> None:
    """The devcontainer piped the install script of golangci-lint's HEAD to sh; session-start used
    the tarball without a checksum."""
    assert "install.sh | sh" not in SETUP
    for script in (SETUP, SESSION_START):
        assert "sha256sum -c" in script
        assert GOLANGCI_SHA256["amd64"] in script
    assert GOLANGCI_SHA256["arm64"] in SETUP


def test_dev_and_live_e2e_run_the_litellm_of_production() -> None:
    prod_image = PROD["services"]["litellm"]["image"]
    tag = prod_image.rsplit(":", 1)[1]
    assert DEV["services"]["litellm"]["image"].endswith(f"/berriai/litellm:{tag}")
    env = (REPO / "scripts" / "live-e2e" / "env.example.sh").read_text()
    match = re.search(r'LIVE_LITELLM_IMAGE:=([^}"]+)', env)
    assert match
    assert match.group(1).endswith(f"/berriai/litellm:{tag}")
