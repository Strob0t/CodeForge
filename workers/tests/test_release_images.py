"""Release images and the version the production compose file pins (KI-116).

A ``v*`` tag on main builds the core, worker and frontend images as the
version and ``latest`` (.github/workflows/docker-build.yml); branch pushes tag
the branch name and the commit SHA only, so a staging push never replaces a
released version. docker-compose.prod.yml pins the images to the version in
VERSION by default, and scripts/sync-version.sh rewrites that pin with every
version bump.
"""

from __future__ import annotations

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
    assert "merge-base --is-ancestor" in script, "a release tag must point at a commit on main"
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
