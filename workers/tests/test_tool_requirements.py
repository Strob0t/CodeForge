"""The Python tools of the worker image's tool PATH (KI-96 review).

Tool processes run with the system interpreter (``CODEFORGE_TOOL_PATH``),
never the worker's venv: a venv interpreter cannot run under Landlock. The
default quality gate commands of Python projects (``pytest``, ``ruff check
.``, domain/project/gatecommands.go) and the auto-agent's workspace test
(``python -m pytest <file>``) need pytest and ruff there, so
Dockerfile.worker installs workers/tool-requirements.txt into the system
interpreter, pinned with hashes to poetry.lock (pip --require-hashes: every
dependency must be listed). This test keeps the file and the lock together;
when the lock changes it prints the file's new content.
"""

from __future__ import annotations

import re
import tomllib
from pathlib import Path

REPO = Path(__file__).resolve().parents[2]
REQUIREMENTS = REPO / "workers" / "tool-requirements.txt"
DOCKERFILE = REPO / "Dockerfile.worker"
# What the tool PATH must offer; their dependencies come from the lock.
TOOLS = ("pytest", "ruff")
_HEADER = """\
# The Python packages tool processes get with the worker image's system
# interpreter (Dockerfile.worker): the default quality gate commands of Python
# projects (pytest, ruff check .) and the auto-agent's workspace test
# (python -m pytest). Tool processes never use the worker's venv. Pinned with
# hashes to poetry.lock; workers/tests/test_tool_requirements.py checks that
# and prints this file's new content when the lock changes.
"""


def _lock() -> dict[str, dict[str, object]]:
    with (REPO / "poetry.lock").open("rb") as f:
        return {package["name"]: package for package in tomllib.load(f)["package"]}


def _on_linux(markers: object) -> bool:
    """Whether a dependency's markers can apply to the image (Linux); Windows-only ones cannot."""
    text = str(markers or "")
    return not re.search(r"""(sys_platform|platform_system)\s*==\s*["'](win32|Windows)["']""", text)


def _closure(lock: dict[str, dict[str, object]]) -> list[str]:
    """The tools and every dependency they need on Linux, sorted."""
    needed: set[str] = set()
    pending = list(TOOLS)
    while pending:
        name = pending.pop()
        if name in needed:
            continue
        needed.add(name)
        dependencies = lock[name].get("dependencies") or {}
        for dependency, spec in dependencies.items():  # type: ignore[union-attr]
            markers = spec.get("markers") if isinstance(spec, dict) else None
            if _on_linux(markers):
                pending.append(dependency.lower())
    return sorted(needed)


def expected_requirements() -> str:
    lock = _lock()
    lines = [_HEADER]
    for name in _closure(lock):
        package = lock[name]
        hashes = sorted(str(f["hash"]) for f in package["files"])  # type: ignore[union-attr]
        lines.append(f"{name}=={package['version']} \\")
        lines.extend(f"    --hash={digest} \\" for digest in hashes[:-1])
        lines.append(f"    --hash={hashes[-1]}")
    return "\n".join(lines) + "\n"


def test_the_tool_requirements_match_the_lock() -> None:
    expected = expected_requirements()
    assert REQUIREMENTS.exists(), f"create {REQUIREMENTS} with:\n{expected}"
    assert REQUIREMENTS.read_text() == expected, f"poetry.lock changed: {REQUIREMENTS} must read:\n{expected}"


def test_pytest_and_ruff_and_every_linux_dependency_are_pinned() -> None:
    pinned = re.findall(r"^([a-z0-9_.-]+)==", REQUIREMENTS.read_text(), re.MULTILINE)
    assert set(TOOLS) <= set(pinned)
    assert {"pygments", "pluggy", "iniconfig", "packaging"} <= set(pinned), pinned
    assert "colorama" not in pinned, "Windows only"


def test_the_image_installs_them_into_the_system_interpreter() -> None:
    dockerfile = DOCKERFILE.read_text()
    runtime = dockerfile.split("# --- Runtime stage ---", 1)[1].replace("\\\n", " ")
    install = re.search(r"pip install[^\n]*", runtime)
    assert install, "the runtime stage installs the tool requirements"
    command = install.group(0)
    assert "--require-hashes" in command
    assert "tool-requirements.txt" in command
    assert "/app/.venv" not in command, "into the system interpreter, not the worker's venv"
    # Compiled afterwards: a read-only root keeps Python from writing .pyc files at run time.
    assert runtime.index("pip install") < runtime.index("compileall")
