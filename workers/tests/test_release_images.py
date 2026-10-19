"""Release images and the version the production compose file pins (KI-116).

A ``v*`` tag on main builds the core, worker and frontend images as the
version and ``latest`` (.github/workflows/docker-build.yml); branch pushes tag
the branch name and the commit SHA only, so a staging push never replaces a
released version. docker-compose.prod.yml pins the images to the version in
VERSION by default, and scripts/sync-version.sh rewrites that pin with every
version bump.
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
VERSION = (REPO / "VERSION").read_text().strip()
IMAGES = {"core": "CORE_IMAGE", "worker": "WORKER_IMAGE", "frontend": "FRONTEND_IMAGE"}
BUILD_JOBS = ("build-core", "build-worker", "build-frontend")
BASH = shutil.which("bash")
# The files scripts/sync-version.sh reads and writes.
SYNCED_FILES = (
    "VERSION",
    "scripts/sync-version.sh",
    "pyproject.toml",
    "frontend/package.json",
    "frontend/package-lock.json",
    "docker-compose.prod.yml",
)


def _image_defaults(compose_text: str) -> dict[str, str]:
    services = yaml.safe_load(compose_text)["services"]
    defaults = {}
    for service, variable in IMAGES.items():
        match = re.fullmatch(rf"\$\{{{variable}:-([^}}]+)\}}", services[service]["image"])
        assert match, f"{service}: image {services[service]['image']!r} has no {variable} default"
        defaults[service] = match.group(1)
    return defaults


def test_compose_pins_the_images_to_the_version() -> None:
    defaults = _image_defaults((REPO / "docker-compose.prod.yml").read_text())

    for service, image in defaults.items():
        assert image == f"ghcr.io/strob0t/codeforge-{service}:{VERSION}", service


@pytest.mark.skipif(BASH is None, reason="needs bash")
def test_sync_version_rewrites_the_compose_pin(tmp_path: Path) -> None:
    for name in SYNCED_FILES:
        (tmp_path / name).parent.mkdir(parents=True, exist_ok=True)
        shutil.copy2(REPO / name, tmp_path / name)
    (tmp_path / "VERSION").write_text("9.10.11-rc.1\n")

    subprocess.run([BASH, str(tmp_path / "scripts" / "sync-version.sh")], check=True, capture_output=True)  # noqa: S603

    compose = (tmp_path / "docker-compose.prod.yml").read_text()
    for service, image in _image_defaults(compose).items():
        assert image == f"ghcr.io/strob0t/codeforge-{service}:9.10.11-rc.1", service
    # Only the three image lines change.
    original = (REPO / "docker-compose.prod.yml").read_text().splitlines()
    changed = [line for line, new in zip(original, compose.splitlines(), strict=True) if line != new]
    assert len(changed) == 3, changed


def test_scripts_default_to_the_pinned_worker_image() -> None:
    """check-host.sh and check-tool-isolation.sh name the image the compose file runs."""
    for script in ("check-host.sh", "check-tool-isolation.sh"):
        text = (REPO / "scripts" / script).read_text()
        assert "codeforge-worker:latest" not in text, script
        assert "VERSION" in text, script


def _workflow() -> dict[str, object]:
    return yaml.safe_load((REPO / ".github" / "workflows" / "docker-build.yml").read_text())


def _metadata_step(job: dict[str, object]) -> dict[str, object]:
    steps = job["steps"]  # type: ignore[index]
    return next(s for s in steps if str(s.get("uses", "")).startswith("docker/metadata-action"))  # type: ignore[union-attr]


def test_release_tag_builds_version_and_latest() -> None:
    workflow = _workflow()
    on = workflow[True] if True in workflow else workflow["on"]  # PyYAML reads "on" as True
    assert on["push"]["tags"] == ["v*"]  # type: ignore[index]
    release = workflow["jobs"]["release"]  # type: ignore[index]
    script = "\n".join(str(step.get("run", "")) for step in release["steps"])
    assert "rev-list --first-parent origin/main" in script, "a release tag must point at a commit of main"
    assert "VERSION" in script, "a release tag must name the version in VERSION"
    for name in BUILD_JOBS:
        job = workflow["jobs"][name]  # type: ignore[index]
        assert "release" in job["needs"], name
        meta = _metadata_step(job)["with"]
        tags = [line.strip() for line in meta["tags"].splitlines() if line.strip()]
        assert "type=semver,pattern={{version}}" in tags, name
        assert "type=raw,value=latest,enable=${{ needs.release.outputs.latest == 'true' }}" in tags, name
        assert meta["flavor"].strip() == "latest=false", f"{name}: latest only through the release job"


def test_branch_pushes_keep_branch_and_sha_tags_only() -> None:
    """A branch push must not move the version tag the compose file pins."""
    workflow = _workflow()
    for name in BUILD_JOBS:
        tags = [line.strip() for line in _metadata_step(workflow["jobs"][name])["with"]["tags"].splitlines()]  # type: ignore[index]
        assert "type=ref,event=branch" in tags, name
        assert "type=sha,prefix=" in tags, name
        assert not [tag for tag in tags if "steps.version.outputs.value" in tag], (
            f"{name}: VERSION tag on branch pushes"
        )


def test_published_images_have_no_claude_code() -> None:
    """Claude Code is a build option (proprietary licence): the release workflow never sets it."""
    workflow = _workflow()
    assert "INSTALL_CLAUDE_CODE" not in (REPO / ".github" / "workflows" / "docker-build.yml").read_text()
    worker = workflow["jobs"]["build-worker"]  # type: ignore[index]
    build = next(s for s in worker["steps"] if str(s.get("uses", "")).startswith("docker/build-push-action"))
    assert build["with"]["file"] == "Dockerfile.worker"
    assert "INSTALL_CLAUDE_CODE" not in build["with"].get("build-args", "")


def test_jobs_that_attest_may_write_attestations() -> None:
    """Without attestations: write the provenance step fails ("Resource not accessible by integration")."""
    attesting = []
    for name, job in _workflow()["jobs"].items():  # type: ignore[union-attr]
        if any(str(s.get("uses", "")).startswith("actions/attest-build-provenance") for s in job["steps"]):
            attesting.append(name)
            assert job["permissions"].get("attestations") == "write", name
            assert job["permissions"].get("id-token") == "write", name
    assert sorted(attesting) == sorted(BUILD_JOBS)


# The release job's check, run in a scratch repository: a v* tag must name
# VERSION and point at a commit of main's first-parent history; latest moves
# only forward (a release whose version is at least the highest v* release).

GIT = shutil.which("git")
_GIT_ENV = {
    "GIT_AUTHOR_NAME": "t",
    "GIT_AUTHOR_EMAIL": "t@example.invalid",
    "GIT_COMMITTER_NAME": "t",
    "GIT_COMMITTER_EMAIL": "t@example.invalid",
    "GIT_CONFIG_GLOBAL": "/dev/null",
    "GIT_CONFIG_NOSYSTEM": "1",
    "PATH": os.environ.get("PATH", ""),
}


def _git(cwd: Path, *args: str) -> str:
    done = subprocess.run([GIT, *args], cwd=cwd, env=_GIT_ENV, capture_output=True, text=True, check=True)  # type: ignore[list-item]  # noqa: S603
    return done.stdout.strip()


class _Repo:
    """A clone ("the checkout") of a bare origin whose main gets commits."""

    def __init__(self, tmp_path: Path) -> None:
        self.origin = tmp_path / "origin.git"
        self.work = tmp_path / "work"
        _git(tmp_path, "init", "-q", "--bare", "-b", "main", str(self.origin))
        _git(tmp_path, "init", "-q", "-b", "main", str(self.work))
        _git(self.work, "remote", "add", "origin", str(self.origin))

    def commit(self, version: str, branch: str = "main") -> str:
        current = subprocess.run(  # noqa: S603 - git of the scratch repository
            [GIT, "symbolic-ref", "-q", "--short", "HEAD"],  # type: ignore[list-item]
            cwd=self.work,
            env=_GIT_ENV,
            capture_output=True,
            text=True,
            check=False,
        ).stdout.strip()
        if current != branch:
            _git(self.work, "checkout", "-q", branch)
        (self.work / "VERSION").write_text(version + "\n")
        _git(self.work, "add", "VERSION")
        _git(self.work, "commit", "-q", "--allow-empty", "-m", f"version {version}")
        _git(self.work, "push", "-q", "origin", branch)
        return _git(self.work, "rev-parse", "HEAD")

    def check(self, tag: str, sha: str, ref_type: str = "tag") -> tuple[int, str, dict[str, str]]:
        """Run the release check for a push of tag at sha; returns (exit code, output, the step's outputs)."""
        if ref_type == "tag":
            _git(self.work, "tag", "-a", "-m", tag, tag, sha)
        _git(self.work, "checkout", "-q", "--detach", sha)
        output = self.work.parent / "github-output"
        output.write_text("")
        env = {**_GIT_ENV, "REF_TYPE": ref_type, "TAG": tag, "GITHUB_SHA": sha, "GITHUB_OUTPUT": str(output)}
        done = subprocess.run(  # noqa: S603 - the workflow's own script
            [BASH, "-c", _release_script()],  # type: ignore[list-item]
            cwd=self.work,
            env=env,
            capture_output=True,
            text=True,
            check=False,
        )
        outputs = dict(line.split("=", 1) for line in output.read_text().splitlines())
        return done.returncode, done.stdout + done.stderr, outputs


def _release_script() -> str:
    release = _workflow()["jobs"]["release"]  # type: ignore[index]
    return next(str(s["run"]) for s in release["steps"] if s.get("id") == "check")


@pytest.fixture
def repo(tmp_path: Path) -> _Repo:
    if GIT is None or BASH is None:
        pytest.skip("needs git and bash")
    return _Repo(tmp_path)


def test_release_check_branch_push(repo: _Repo) -> None:
    sha = repo.commit("0.9.0")
    code, output, outputs = repo.check("main", sha, ref_type="branch")
    assert (code, outputs) == (0, {"latest": "false", "minor": "false"}), output


def test_release_check_first_release_moves_latest(repo: _Repo) -> None:
    repo.commit("0.8.0")
    sha = repo.commit("0.9.0")
    code, output, outputs = repo.check("v0.9.0", sha)
    assert (code, outputs) == (0, {"latest": "true", "minor": "true"}), output


def test_release_check_tag_must_name_version(repo: _Repo) -> None:
    sha = repo.commit("0.9.0")
    code, output, outputs = repo.check("v0.9.1", sha)
    assert code == 1
    assert outputs == {}
    assert "does not name the version in VERSION (0.9.0)" in output


def test_release_check_commit_of_a_merged_branch_is_refused(repo: _Repo) -> None:
    base = repo.commit("0.8.0")
    _git(repo.work, "checkout", "-q", "-b", "feature", base)
    feature = repo.commit("0.9.0", branch="feature")
    _git(repo.work, "checkout", "-q", "main")
    _git(repo.work, "merge", "-q", "--no-ff", "-m", "merge feature", "feature")
    _git(repo.work, "push", "-q", "origin", "main")

    assert _git(repo.work, "merge-base", "--is-ancestor", feature, "main") == ""  # reachable, not first-parent
    code, output, outputs = repo.check("v0.9.0", feature)
    assert code == 1
    assert outputs == {}
    assert "first-parent" in output


def test_release_check_commit_off_main_is_refused(repo: _Repo) -> None:
    base = repo.commit("0.8.0")
    _git(repo.work, "checkout", "-q", "-b", "staging", base)
    sha = repo.commit("0.9.0", branch="staging")
    code, _, outputs = repo.check("v0.9.0", sha)
    assert code == 1
    assert outputs == {}


@pytest.mark.parametrize(
    ("existing", "version", "latest", "minor"),
    [
        (["v1.0.0"], "0.9.1", "false", "true"),  # a patch release of an older line
        (["v0.9.0"], "0.10.0", "true", "true"),  # semver, not text order
        (["v0.9.0", "v0.10.0"], "0.9.1", "false", "true"),
        (["v0.9.0"], "0.9.0-rc.2", "false", "false"),  # a prerelease moves neither
        (["v1.0.0-rc.1"], "0.9.0", "true", "true"),  # prereleases do not count as released
        (["v0.9.2"], "0.9.1", "false", "false"),  # 0.9 stays on 0.9.2
    ],
)
def test_release_check_tags_move_only_forward(
    repo: _Repo, existing: list[str], version: str, latest: str, minor: str
) -> None:
    for tag in existing:
        repo.check(tag, repo.commit(tag[1:]))
    sha = repo.commit(version)
    code, output, outputs = repo.check(f"v{version}", sha)
    assert (code, outputs) == (0, {"latest": latest, "minor": minor}), output


def test_release_check_an_older_commit_tagged_later(repo: _Repo) -> None:
    """v0.9.0 tagged on its commit after v0.9.1 was released: neither latest nor 0.9 moves back."""
    old = repo.commit("0.9.0")
    repo.check("v0.9.1", repo.commit("0.9.1"))
    code, output, outputs = repo.check("v0.9.0", old)
    assert (code, outputs) == (0, {"latest": "false", "minor": "false"}), output


def test_release_check_only_tags_on_main_count(repo: _Repo) -> None:
    """A stray v* tag off main's first-parent history (a failed or mistaken tag) does not hold latest back."""
    base = repo.commit("0.8.0")
    _git(repo.work, "checkout", "-q", "-b", "staging", base)
    stray = repo.commit("2.0.0", branch="staging")
    _git(repo.work, "tag", "-a", "-m", "v2.0.0", "v2.0.0", stray)
    _git(repo.work, "checkout", "-q", "main")
    code, output, outputs = repo.check("v0.9.0", repo.commit("0.9.0"))
    assert (code, outputs) == (0, {"latest": "true", "minor": "true"}), output


def test_build_jobs_gate_the_minor_tag() -> None:
    workflow = _workflow()
    assert workflow["jobs"]["release"]["outputs"]["minor"] == "${{ steps.check.outputs.minor }}"  # type: ignore[index]
    for name in BUILD_JOBS:
        tags = [line.strip() for line in _metadata_step(workflow["jobs"][name])["with"]["tags"].splitlines()]  # type: ignore[index]
        assert "type=semver,pattern={{major}}.{{minor}},enable=${{ needs.release.outputs.minor == 'true' }}" in tags, (
            name
        )
