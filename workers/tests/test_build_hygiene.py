"""Build, CI and deployment hygiene (KI-214).

What reaches the images (.dockerignore), what CI runs and pins, and the
compose settings around shutdown and the dev browser container.
"""

from __future__ import annotations

import re
import shutil
import subprocess
from pathlib import Path

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
